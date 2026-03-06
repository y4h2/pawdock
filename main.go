package main

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"image"
	_ "image/png"
	"log"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	width  = 430
	height = 430
)

//go:embed material/*
var f embed.FS

// Config holds application settings loaded from deskpet.ini.
type Config struct {
	Speed int
	Scale float64
}

func loadConfig(path string) Config {
	cfg := Config{Speed: 1, Scale: 0.5} // defaults
	file, err := os.Open(path)
	if err != nil {
		return cfg
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "Speed":
			if v, err := strconv.Atoi(val); err == nil {
				cfg.Speed = v
			}
		case "Scale":
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				cfg.Scale = v
			}
		}
	}
	return cfg
}

// State represents the pet's current behavior.
type State int

const (
	StateIdle State = iota
	StateWalk
	StateHappy
	StatePick
	StatePlay
)

// deskpet implements the ebiten.Game interface.
type deskpet struct {
	x, y       int     // window position
	state      State   // current state
	frameIndex int     // current animation frame
	frameCount int     // tick counter for frame timing
	facingLeft  bool // true when pet faces left
	idleFrames  []*ebiten.Image
	walkFrames  []*ebiten.Image
	happyFrames []*ebiten.Image
	pickFrames  []*ebiten.Image
	playFrames  []*ebiten.Image
	cfg           Config
	tickCount     int // global tick counter
	lastClickTick int // tick of last click for double-click detection
	dragStartX  int // mouse X at drag start (screen coords)
	dragStartY  int // mouse Y at drag start (screen coords)
	dragWinX    int // window X at drag start
	dragWinY    int // window Y at drag start
}

// loadImage decodes an embedded PNG file into an ebiten.Image.
func loadImage(path string) *ebiten.Image {
	data, err := f.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		log.Fatal(err)
	}
	return ebiten.NewImageFromImage(img)
}

// loadSheetFrames splits a sprite sheet into individual frames.
// The sheet is arranged in a grid of cols x rows, with each frame being frameW x frameH.
func loadSheetFrames(path string, frameW, frameH, cols, total int) []*ebiten.Image {
	sheet := loadImage(path)
	frames := make([]*ebiten.Image, total)
	for i := 0; i < total; i++ {
		col := i % cols
		row := i / cols
		x := col * frameW
		y := row * frameH
		rect := image.Rect(x, y, x+frameW, y+frameH)
		frames[i] = sheet.SubImage(rect).(*ebiten.Image)
	}
	return frames
}

func loadSprites() (idle, walk, happy, pick, play []*ebiten.Image) {
	idle = loadSheetFrames("material/idle_sheet.png", 256, 256, 9, 121)
	walk = loadSheetFrames("material/walk_sheet.png", 256, 256, 9, 73)
	happy = loadSheetFrames("material/happy_sheet.png", 256, 256, 9, 73)
	pick = loadSheetFrames("material/pick_sheet.png", 256, 256, 9, 73)
	play = loadSheetFrames("material/play_sheet.png", 256, 256, 9, 121)
	return idle, walk, happy, pick, play
}

func (d *deskpet) Update() error {
	// Press Escape to quit
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}

	// Handle mouse interactions
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		wx, wy := ebiten.WindowPosition()
		d.dragStartX = wx + mx
		d.dragStartY = wy + my
		d.dragWinX = wx
		d.dragWinY = wy
	}

	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		wx, wy := ebiten.WindowPosition()
		screenX := wx + mx
		screenY := wy + my
		dx := screenX - d.dragStartX
		dy := screenY - d.dragStartY
		if dx < 0 {
			dx = -dx
		}
		if dy < 0 {
			dy = -dy
		}
		// Start pick if dragged more than 5 pixels
		if d.state != StatePick && (dx > 5 || dy > 5) {
			d.state = StatePick
			d.frameIndex = 0
			d.frameCount = 0
		}
		if d.state == StatePick {
			newX := d.dragWinX + (screenX - d.dragStartX)
			newY := d.dragWinY + (screenY - d.dragStartY)
			d.x = newX
			d.y = newY
			ebiten.SetWindowPosition(newX, newY)
		}
	}

	d.tickCount++

	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		if d.state == StatePick {
			d.state = StateIdle
			d.frameIndex = 0
			d.frameCount = 0
		} else {
			// Double-click detection (within 20 ticks ≈ 400ms at TPS=50)
			if d.tickCount-d.lastClickTick < 20 {
				d.state = StatePlay
				d.frameIndex = 0
				d.frameCount = 0
			} else {
				// Single click -> happy
				if d.state != StateHappy && d.state != StatePlay {
					d.state = StateHappy
					d.frameIndex = 0
					d.frameCount = 0
				}
			}
			d.lastClickTick = d.tickCount
		}
	}

	switch d.state {
	case StatePlay:
		d.frameCount++
		if d.frameCount >= 3 {
			d.frameCount = 0
			d.frameIndex++
			if d.frameIndex >= len(d.playFrames) {
				d.state = StateIdle
				d.frameIndex = 0
			}
		}
	case StateHappy:
		d.frameCount++
		if d.frameCount >= 3 {
			d.frameCount = 0
			d.frameIndex++
			if d.frameIndex >= len(d.happyFrames) {
				d.state = StateIdle
				d.frameIndex = 0
			}
		}
	case StatePick:
		d.frameCount++
		if d.frameCount >= 3 {
			d.frameCount = 0
			d.frameIndex++
			if d.frameIndex >= len(d.pickFrames) {
				d.frameIndex = 0 // loop pick animation
			}
		}
	case StateWalk:
		d.frameCount++
		if d.frameCount >= 3 {
			d.frameCount = 0
			d.frameIndex++
			if d.frameIndex >= len(d.walkFrames) {
				d.frameIndex = 0 // loop walk animation
			}
		}
		d.catchCursor()
	case StateIdle:
		d.frameCount++
		if d.frameCount >= 3 {
			d.frameCount = 0
			d.frameIndex++
			if d.frameIndex >= len(d.idleFrames) {
				d.frameIndex = 0 // loop idle animation
			}
		}
		d.catchCursor()
	}

	return nil
}

func (d *deskpet) catchCursor() {
	mx, my := ebiten.CursorPosition()
	lw, lh := d.Layout(0, 0)
	x := mx - (lw / 2)
	y := my - (lh / 2)

	// Calculate Euclidean distance from pet center to cursor
	distance := math.Sqrt(float64(x*x + y*y))

	// Update facing direction based on cursor position
	if x < 0 {
		d.facingLeft = true
	} else if x > 0 {
		d.facingLeft = false
	}

	// Stop when close enough to cursor
	if distance < 100 {
		if d.state == StateWalk {
			d.state = StateIdle
			d.frameIndex = 0
			d.frameCount = 0
		}
		return
	}

	// Switch to walk state if idle
	if d.state == StateIdle {
		d.state = StateWalk
		d.frameIndex = 0
		d.frameCount = 0
	}

	speed := d.cfg.Speed

	// Calculate angle for 8-directional movement
	r := math.Atan2(float64(y), float64(x))
	a := (r / math.Pi * 180) + 360
	a = math.Mod(a, 360)

	wx, wy := ebiten.WindowPosition()

	switch {
	case a >= 337.5 || a < 22.5: // Right
		wx += speed
	case a >= 22.5 && a < 67.5: // Down-Right
		wx += speed
		wy += speed
	case a >= 67.5 && a < 112.5: // Down
		wy += speed
	case a >= 112.5 && a < 157.5: // Down-Left
		wx -= speed
		wy += speed
	case a >= 157.5 && a < 202.5: // Left
		wx -= speed
	case a >= 202.5 && a < 247.5: // Up-Left
		wx -= speed
		wy -= speed
	case a >= 247.5 && a < 292.5: // Up
		wy -= speed
	case a >= 292.5 && a < 337.5: // Up-Right
		wx += speed
		wy -= speed
	}

	d.x = wx
	d.y = wy
	ebiten.SetWindowPosition(wx, wy)
}

func (d *deskpet) Draw(screen *ebiten.Image) {
	screen.Clear()

	var frames []*ebiten.Image
	switch d.state {
	case StatePlay:
		frames = d.playFrames
	case StateHappy:
		frames = d.happyFrames
	case StatePick:
		frames = d.pickFrames
	case StateWalk:
		frames = d.walkFrames
	default:
		frames = d.idleFrames
	}

	var img *ebiten.Image
	if d.frameIndex < len(frames) {
		img = frames[d.frameIndex]
	} else if len(frames) > 0 {
		img = frames[0]
	}

	if img == nil {
		return
	}

	op := &ebiten.DrawImageOptions{}
	scale := d.cfg.Scale
	iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
	sx := float64(width) / float64(iw) * scale
	sy := float64(height) / float64(ih) * scale

	if !d.facingLeft {
		op.GeoM.Scale(-sx, sy)
		op.GeoM.Translate(float64(iw)*sx, 0)
	} else {
		op.GeoM.Scale(sx, sy)
	}

	screen.DrawImage(img, op)
}

func (d *deskpet) Layout(outsideWidth, outsideHeight int) (int, int) {
	return int(float64(width) * d.cfg.Scale), int(float64(height) * d.cfg.Scale)
}

func main() {
	cfg := loadConfig("deskpet.ini")

	idleFrames, walkFrames, happyFrames, pickFrames, playFrames := loadSprites()

	fmt.Printf("Loaded: %d idle + %d walk + %d happy + %d pick + %d play frames\n",
		len(idleFrames), len(walkFrames), len(happyFrames), len(pickFrames), len(playFrames))
	fmt.Printf("Config: Speed=%d, Scale=%.1f\n", cfg.Speed, cfg.Scale)

	pet := &deskpet{
		idleFrames:  idleFrames,
		walkFrames:  walkFrames,
		happyFrames: happyFrames,
		pickFrames:  pickFrames,
		playFrames:  playFrames,
		cfg:         cfg,
	}

	scaledW := int(float64(width) * cfg.Scale)
	scaledH := int(float64(height) * cfg.Scale)

	ebiten.SetRunnableOnUnfocused(true)
	ebiten.SetScreenClearedEveryFrame(false)
	ebiten.SetTPS(50)
	ebiten.SetVsyncEnabled(true)
	ebiten.SetWindowDecorated(false)
	ebiten.SetWindowFloating(true)
	ebiten.SetWindowSize(scaledW, scaledH)
	ebiten.SetWindowTitle("deskpet")

	err := ebiten.RunGameWithOptions(pet, &ebiten.RunGameOptions{
		InitUnfocused:     true,
		ScreenTransparent: true,
		SkipTaskbar:       true,
	})
	if err != nil {
		log.Fatal(err)
	}
}
