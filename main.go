package main

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"image/color"

	"github.com/hajimehoshi/bitmapfont/v3"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	width  = 430
	height = 430
)

//go:embed material/*
var f embed.FS

// WorkState represents the pomodoro work mode state.
type WorkState int

const (
	WorkIdle       WorkState = iota
	WorkFocus                // 专注工作中
	WorkShortBreak           // 短休息
	WorkLongBreak            // 长休息
)

// Bubble represents a speech bubble above the pet.
type Bubble struct {
	Text      string
	Remaining int     // remaining ticks
	Alpha     float64 // opacity for fade-out
}

func (b *Bubble) Show(text string, durationSec int) {
	b.Text = text
	b.Remaining = durationSec * 50 // TPS=50
	b.Alpha = 1.0
}

func (b *Bubble) Update() {
	if b.Remaining <= 0 {
		return
	}
	b.Remaining--
	if b.Remaining < 50 {
		b.Alpha = float64(b.Remaining) / 50.0
	}
}

func (b *Bubble) Active() bool {
	return b.Remaining > 0
}

// Random bubble text pools
var (
	focusStartTexts = []string{
		"Focus time!",
		"Let's go!",
		"You got this~",
	}
	breakTexts = []string{
		"Take a break!",
		"Drink water~",
		"Stretch a bit!",
		"Rest your eyes~",
	}
	longBreakTexts = []string{
		"Great job! Long break~",
		"Amazing! Rest well!",
	}
	workDoneTexts = []string{
		"Well done today!",
		"Nice work!",
	}
)

func randomText(pool []string) string {
	return pool[rand.Intn(len(pool))]
}

// Config holds application settings loaded from deskpet.ini.
type Config struct {
	Speed            int
	Scale            float64
	FocusDuration    int // minutes, default 25
	ShortBreak       int // minutes, default 5
	LongBreak        int // minutes, default 15
	CyclesBeforeLong int // default 4
}

func loadConfig(path string) Config {
	cfg := Config{Speed: 1, Scale: 0.5, FocusDuration: 25, ShortBreak: 5, LongBreak: 15, CyclesBeforeLong: 4}
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
		case "FocusDuration":
			if v, err := strconv.Atoi(val); err == nil && v > 0 {
				cfg.FocusDuration = v
			}
		case "ShortBreak":
			if v, err := strconv.Atoi(val); err == nil && v > 0 {
				cfg.ShortBreak = v
			}
		case "LongBreak":
			if v, err := strconv.Atoi(val); err == nil && v > 0 {
				cfg.LongBreak = v
			}
		case "CyclesBeforeLong":
			if v, err := strconv.Atoi(val); err == nil && v > 0 {
				cfg.CyclesBeforeLong = v
			}
		}
	}
	return cfg
}

// State represents the pet's current behavior.
type State int

const (
	StateIdle  State = iota
	StateWalk
	StateHappy
	StatePick
	StatePlay
	StateWork  // working animation (focus mode)
	StateHide  // peekaboo: hiding animation
	StatePeek  // peekaboo: peeking from screen edge
	StateFound // peekaboo: found by user
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
	cfg          Config
	showMenu     bool
	following    bool // true=跟随鼠标, false=待命原地
	menuConsumed bool // true when menu handled the click
	iconHappy    *ebiten.Image
	iconPlay     *ebiten.Image
	iconFollow   *ebiten.Image
	iconStay     *ebiten.Image
	iconPeekaboo *ebiten.Image
	dragStartX int // mouse X at drag start (screen coords)
	dragStartY int // mouse Y at drag start (screen coords)
	dragWinX   int // window X at drag start
	dragWinY   int // window Y at drag start
	workFrames      []*ebiten.Image
	hideFrames      []*ebiten.Image
	peekFrames      []*ebiten.Image
	peekTopFrames   []*ebiten.Image // upper half: for peeking from top edge
	peekBottomFrames []*ebiten.Image // lower half: for peeking from bottom edge
	foundFrames     []*ebiten.Image
	peekEdge    int  // 0=left, 1=right, 2=top, 3=bottom
	peekCount   int  // number of times peeked (found after 3)
	savedX      int  // saved position before peekaboo
	savedY      int  // saved position before peekaboo
	// Work mode (pomodoro)
	workState         WorkState
	workEndTime       time.Time     // when current phase ends
	workTotalDuration time.Duration // total duration of current phase
	workCycle         int           // completed focus cycles
	workFollowing     bool          // saved following state before work mode
	bubble            Bubble
	iconWork          *ebiten.Image
	iconStop          *ebiten.Image
	iconQuit          *ebiten.Image
	showQuitConfirm   bool
	audioContext      *audio.Context
	notifyPCM         []byte // decoded PCM for notification sound
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

// cropFramesHalf crops each frame to its top or bottom half.
// If top is true, keeps the upper half; otherwise keeps the lower half.
func cropFramesHalf(frames []*ebiten.Image, top bool) []*ebiten.Image {
	result := make([]*ebiten.Image, len(frames))
	for i, f := range frames {
		b := f.Bounds()
		halfH := b.Dy() / 2
		var rect image.Rectangle
		if top {
			rect = image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+halfH)
		} else {
			rect = image.Rect(b.Min.X, b.Min.Y+halfH, b.Max.X, b.Max.Y)
		}
		result[i] = f.SubImage(rect).(*ebiten.Image)
	}
	return result
}

func loadSprites() (idle, walk, happy, pick, play, work, hide, peek, peekTop, peekBottom, found []*ebiten.Image) {
	idle = loadSheetFrames("material/idle_sheet.png", 256, 256, 9, 121)
	walk = loadSheetFrames("material/walk_sheet.png", 256, 256, 9, 73)
	happy = loadSheetFrames("material/happy_sheet.png", 256, 256, 9, 73)
	pick = loadSheetFrames("material/pick_sheet.png", 256, 256, 9, 73)
	play = loadSheetFrames("material/play_sheet.png", 256, 256, 9, 121)
	work = loadSheetFrames("material/work_sheet.png", 256, 256, 9, 145)
	hide = loadSheetFrames("material/hide_sheet.png", 256, 256, 9, 73)
	peek = loadSheetFrames("material/peek_sheet.png", 256, 256, 9, 73)
	found = loadSheetFrames("material/found_sheet.png", 256, 256, 9, 73)
	peekTop = cropFramesHalf(peek, true)       // head part: show when peeking from top
	peekBottom = cropFramesHalf(peek, false)    // feet part: show when peeking from bottom
	return
}

// --- Work mode methods ---

func (d *deskpet) startWorkMode() {
	d.workFollowing = d.following
	d.workCycle = 0
	d.startFocusPhase()
}

func (d *deskpet) startFocusPhase() {
	d.workState = WorkFocus
	dur := time.Duration(d.cfg.FocusDuration) * time.Minute
	d.workEndTime = time.Now().Add(dur)
	d.workTotalDuration = dur
	d.following = false
	d.state = StateWork
	d.frameIndex = 0
	d.frameCount = 0
	d.bubble.Show(randomText(focusStartTexts), 4)
}

func (d *deskpet) startShortBreak() {
	d.workState = WorkShortBreak
	dur := time.Duration(d.cfg.ShortBreak) * time.Minute
	d.workEndTime = time.Now().Add(dur)
	d.workTotalDuration = dur
	d.following = true
	d.state = StateHappy
	d.frameIndex = 0
	d.frameCount = 0
	d.bubble.Show(randomText(breakTexts), 4)
	d.playNotify()
}

func (d *deskpet) startLongBreak() {
	d.workState = WorkLongBreak
	dur := time.Duration(d.cfg.LongBreak) * time.Minute
	d.workEndTime = time.Now().Add(dur)
	d.workTotalDuration = dur
	d.following = true
	d.state = StatePlay
	d.frameIndex = 0
	d.frameCount = 0
	d.bubble.Show(randomText(longBreakTexts), 5)
	d.playNotify()
}

func (d *deskpet) stopWorkMode() {
	d.workState = WorkIdle
	d.following = d.workFollowing
	d.state = StateHappy
	d.frameIndex = 0
	d.frameCount = 0
	d.bubble.Show(randomText(workDoneTexts), 4)
}

func (d *deskpet) updateWorkMode() {
	if d.workState == WorkIdle {
		return
	}
	if time.Now().Before(d.workEndTime) {
		return
	}
	// Phase ended
	switch d.workState {
	case WorkFocus:
		d.workCycle++
		if d.workCycle >= d.cfg.CyclesBeforeLong {
			d.startLongBreak()
		} else {
			d.startShortBreak()
		}
	case WorkShortBreak:
		d.startFocusPhase()
	case WorkLongBreak:
		d.stopWorkMode()
	}
}

func (d *deskpet) playNotify() {
	if d.audioContext == nil || d.notifyPCM == nil {
		return
	}
	player := d.audioContext.NewPlayerFromBytes(d.notifyPCM)
	player.Play()
}

func (d *deskpet) drawBubble(screen *ebiten.Image) {
	if !d.bubble.Active() {
		return
	}

	face := text.NewGoXFace(bitmapfont.Face)
	msg := d.bubble.Text
	tw, th := text.Measure(msg, face, 0)

	lw, lh := d.Layout(0, 0)
	// Position: above center, slightly right
	bubbleX := float64(lw)/2 - tw/2
	bubbleY := float64(lh)*0.15 - th/2
	padX := 6.0
	padY := 4.0

	alpha := d.bubble.Alpha

	// Background rect
	bgColor := color.RGBA{30, 30, 30, uint8(200 * alpha)}
	vector.DrawFilledRect(screen,
		float32(bubbleX-padX), float32(bubbleY-padY),
		float32(tw+padX*2), float32(th+padY*2),
		bgColor, false)

	// Triangle pointer (pointing down toward character)
	triX := float32(bubbleX + tw/2)
	triY := float32(bubbleY + th + padY)
	triSize := float32(6)
	vector.StrokeLine(screen, triX-triSize, triY, triX, triY+triSize, 2, bgColor, false)
	vector.StrokeLine(screen, triX+triSize, triY, triX, triY+triSize, 2, bgColor, false)
	vector.StrokeLine(screen, triX-triSize, triY, triX+triSize, triY, 2, bgColor, false)

	// Text
	top := &text.DrawOptions{}
	top.GeoM.Translate(bubbleX, bubbleY)
	top.ColorScale.ScaleWithColor(color.RGBA{255, 255, 255, uint8(255 * alpha)})
	text.Draw(screen, msg, face, top)
}

func (d *deskpet) drawTimerBadge(screen *ebiten.Image) {
	if d.workState == WorkIdle {
		return
	}

	remaining := time.Until(d.workEndTime)
	if remaining < 0 {
		remaining = 0
	}
	totalSec := int(remaining.Seconds())
	mm := totalSec / 60
	ss := totalSec % 60

	lw, lh := d.Layout(0, 0)
	face := text.NewGoXFace(bitmapfont.Face)
	label := fmt.Sprintf("%d:%02d", mm, ss)
	tw, th := text.Measure(label, face, 0)

	// Scale up for large text
	scale := 3.0
	cx := float64(lw)/2 + 40 + 20
	cy := float64(lh) * 0.12

	// Warm color based on work state
	var clr color.RGBA
	if d.workState == WorkFocus {
		clr = color.RGBA{180, 60, 30, 255} // deep warm red
	} else {
		clr = color.RGBA{60, 140, 100, 255} // calm green
	}

	// Draw multiple times with slight offsets for bold effect
	for _, off := range [][2]float64{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
		top := &text.DrawOptions{}
		top.GeoM.Translate(-tw/2, -th/2)
		top.GeoM.Scale(scale, scale)
		top.GeoM.Translate(cx+off[0], cy+off[1])
		top.ColorScale.ScaleWithColor(clr)
		text.Draw(screen, label, face, top)
	}
}

func (d *deskpet) Update() error {
	// Press Escape to quit
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		return ebiten.Termination
	}

	// Update bubble and work mode timer
	d.bubble.Update()
	d.updateWorkMode()

	// During peekaboo states, only handle click-to-find
	if d.state == StateHide || d.state == StatePeek || d.state == StateFound {
		if d.state == StatePeek && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			// Move window on-screen with offset for sprite padding
			scaledW := int(float64(width) * d.cfg.Scale)
			pad := scaledW * 30 / 100
			switch d.peekEdge {
			case 0: // left edge
				d.x = -pad
			case 1: // right edge
				sw, _ := d.screenSize()
				d.x = sw - scaledW + pad
			}
			ebiten.SetWindowPosition(d.x, d.y)
			d.state = StateFound
			d.frameIndex = 0
			d.frameCount = 0
		}
		goto stateUpdate
	}

	// Handle mouse interactions
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		wx, wy := ebiten.WindowPosition()
		d.dragStartX = wx + mx
		d.dragStartY = wy + my
		d.dragWinX = wx
		d.dragWinY = wy

		// Check quit confirm click first
		if d.showQuitConfirm {
			if btn := d.quitConfirmHitTest(mx, my); btn >= 0 {
				if btn == 0 { // Yes
					os.Exit(0)
				}
				d.showQuitConfirm = false // No
			}
			d.menuConsumed = true
			return nil
		}

		// Check menu click first
		d.menuConsumed = false
		if d.showMenu {
			if item := d.menuHitTest(mx, my); item >= 0 {
				switch item {
				case 0: // Happy
					d.showMenu = false
					d.state = StateHappy
					d.frameIndex = 0
					d.frameCount = 0
				case 1: // Play
					d.showMenu = false
					d.state = StatePlay
					d.frameIndex = 0
					d.frameCount = 0
				case 2: // Peekaboo
					d.showMenu = false
					d.savedX, d.savedY = ebiten.WindowPosition()
					d.state = StateHide
					d.frameIndex = 0
					d.frameCount = 0
					d.peekCount = 0
				case 3: // Follow/Stay toggle
					d.showMenu = false
					if d.workState == WorkIdle {
						d.following = !d.following
						if !d.following && d.state == StateWalk {
							d.state = StateIdle
							d.frameIndex = 0
							d.frameCount = 0
						}
					}
				case 4: // Work mode toggle
					d.showMenu = false
					if d.workState == WorkIdle {
						d.startWorkMode()
					} else {
						d.stopWorkMode()
					}
				case 5: // Quit
					d.showMenu = false
					d.showQuitConfirm = true
				}
				d.menuConsumed = true
				return nil
			}
		}
	}

	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && !d.showMenu {
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

	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) && !d.menuConsumed {
		if d.state == StatePick {
			if d.workState != WorkIdle {
				d.state = StateWork
			} else {
				d.state = StateIdle
			}
			d.frameIndex = 0
			d.frameCount = 0
		} else {
			// Toggle menu on click
			d.showMenu = !d.showMenu
		}
	}

stateUpdate:
	switch d.state {
	case StatePlay:
		d.frameCount++
		if d.frameCount >= 3 {
			d.frameCount = 0
			d.frameIndex++
			if d.frameIndex >= len(d.playFrames) {
				if d.workState != WorkIdle {
					d.frameIndex = 0 // loop during work break
				} else {
					d.state = StateIdle
					d.frameIndex = 0
				}
			}
		}
	case StateHappy:
		d.frameCount++
		if d.frameCount >= 3 {
			d.frameCount = 0
			d.frameIndex++
			if d.frameIndex >= len(d.happyFrames) {
				if d.workState != WorkIdle {
					d.frameIndex = 0 // loop during work break
				} else {
					d.state = StateIdle
					d.frameIndex = 0
				}
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
	case StateWork:
		d.frameCount++
		if d.frameCount >= 3 {
			d.frameCount = 0
			d.frameIndex++
			if d.frameIndex >= len(d.workFrames) {
				d.frameIndex = 0 // loop work animation
			}
		}
		// Face toward screen center
		screenW, _ := d.screenSize()
		wx, _ := ebiten.WindowPosition()
		scaledW := int(float64(width) * d.cfg.Scale)
		d.facingLeft = wx+scaledW/2 > screenW/2
	case StateHide:
		d.frameCount++
		if d.frameCount >= 3 {
			d.frameCount = 0
			d.frameIndex++
			if d.frameIndex >= len(d.hideFrames) {
				// Hide animation done, move to edge and start peeking
				d.pickRandomEdge()
				d.state = StatePeek
				d.frameIndex = 0
				d.frameCount = 0
			}
		}
	case StatePeek:
		d.frameCount++
		if d.frameCount >= 3 {
			d.frameCount = 0
			d.frameIndex++
			if d.frameIndex >= len(d.peekFrames) {
				d.frameIndex = 0 // loop peek animation
			}
		}
		// Check if mouse is close to the window
		d.checkPeekMouse()
	case StateFound:
		d.frameCount++
		if d.frameCount >= 3 {
			d.frameCount = 0
			d.frameIndex++
			if d.frameIndex >= len(d.foundFrames) {
				// Found animation done, return to saved position and idle
				d.x = d.savedX
				d.y = d.savedY
				ebiten.SetWindowPosition(d.savedX, d.savedY)
				d.state = StateIdle
				d.frameIndex = 0
				d.frameCount = 0
			}
		}
	}

	return nil
}

// Menu layout
var menuLabels = []string{"Happy", "Play", "Peekaboo", "Follow", "Work", "Quit"}

// Warm gradient button colors
var menuColors = []color.RGBA{
	{235, 150, 120, 230}, // warm coral
	{230, 160, 135, 230}, // soft salmon
	{200, 160, 210, 230}, // soft purple
	{225, 170, 145, 230}, // muted peach
	{240, 140, 100, 230}, // warm orange (work)
	{180, 100, 100, 230}, // muted red (quit)
}
var menuHoverColors = []color.RGBA{
	{245, 170, 140, 250},
	{240, 180, 155, 250},
	{220, 180, 230, 250},
	{235, 190, 165, 250},
	{250, 160, 120, 250}, // warm orange hover
	{210, 120, 120, 250}, // muted red hover
}

const (
	menuBtnSize = 28 // circle diameter
	menuBtnGap  = 6
	menuIconSize = 16 // icon draw size inside button
)

// menuBtnCenter returns the center (cx, cy) of menu button i.
func (d *deskpet) menuBtnCenter(i int) (cx, cy int) {
	lw, lh := d.Layout(0, 0)
	n := len(menuLabels)
	totalH := n*menuBtnSize + (n-1)*menuBtnGap
	startX := lw/2 + 30
	startY := (lh - totalH) / 2
	cx = startX + menuBtnSize/2
	cy = startY + i*(menuBtnSize+menuBtnGap) + menuBtnSize/2
	return cx, cy
}

func (d *deskpet) menuHitTest(mx, my int) int {
	for i := range menuLabels {
		cx, cy := d.menuBtnCenter(i)
		dx := mx - cx
		dy := my - cy
		r := menuBtnSize / 2
		if dx*dx+dy*dy <= r*r {
			return i
		}
	}
	return -1
}

func (d *deskpet) drawMenu(screen *ebiten.Image) {
	mx, my := ebiten.CursorPosition()
	face := text.NewGoXFace(bitmapfont.Face)

	for i := range menuLabels {
		cx, cy := d.menuBtnCenter(i)
		r := float32(menuBtnSize) / 2

		// Determine hover
		dx := mx - cx
		dy := my - cy
		hovered := dx*dx+dy*dy <= int(r)*int(r)

		// Draw circle button
		btnColor := menuColors[i]
		if hovered {
			btnColor = menuHoverColors[i]
		}
		vector.DrawFilledCircle(screen, float32(cx), float32(cy), r, btnColor, true)

		// Draw icon centered in circle
		var icon *ebiten.Image
		switch i {
		case 0:
			icon = d.iconHappy
		case 1:
			icon = d.iconPlay
		case 2:
			icon = d.iconPeekaboo
		case 3:
			if d.following {
				icon = d.iconStay
			} else {
				icon = d.iconFollow
			}
		case 4:
			if d.workState != WorkIdle {
				icon = d.iconStop
			} else {
				icon = d.iconWork
			}
		case 5:
			icon = d.iconQuit
		}
		if icon != nil {
			iop := &ebiten.DrawImageOptions{}
			iw, ih := icon.Bounds().Dx(), icon.Bounds().Dy()
			sx := float64(menuIconSize) / float64(iw)
			sy := float64(menuIconSize) / float64(ih)
			iop.GeoM.Scale(sx, sy)
			iop.GeoM.Translate(float64(cx)-float64(menuIconSize)/2, float64(cy)-float64(menuIconSize)/2)
			screen.DrawImage(icon, iop)
		}

		// Show tooltip on hover
		if hovered {
			label := menuLabels[i]
			if i == 3 {
				if d.following {
					label = "Stay"
				} else {
					label = "Follow"
				}
			} else if i == 4 {
				if d.workState != WorkIdle {
					label = "Stop"
				} else {
					label = "Work"
				}
			}
			tw, th := text.Measure(label, face, 0)
			tipX := float64(cx) + float64(r) + 6
			tipY := float64(cy) - th/2

			// Tooltip background
			vector.DrawFilledRect(screen,
				float32(tipX)-2, float32(tipY)-2,
				float32(tw)+4, float32(th)+4,
				color.RGBA{30, 30, 30, 210}, false)

			// Tooltip text
			top := &text.DrawOptions{}
			top.GeoM.Translate(tipX, tipY)
			top.ColorScale.ScaleWithColor(color.White)
			text.Draw(screen, label, face, top)
		}
	}
}

// Quit confirm dialog
const (
	quitDialogW = 90
	quitDialogH = 40
	quitBtnW    = 30
	quitBtnH    = 14
)

func (d *deskpet) quitConfirmPos() (dx, dy int) {
	lw, lh := d.Layout(0, 0)
	return (lw - quitDialogW) / 2, (lh - quitDialogH) / 2
}

func (d *deskpet) quitConfirmHitTest(mx, my int) int {
	dx, dy := d.quitConfirmPos()
	btnY := dy + 22
	// Yes button
	yesX := dx + quitDialogW/2 - quitBtnW - 4
	if mx >= yesX && mx <= yesX+quitBtnW && my >= btnY && my <= btnY+quitBtnH {
		return 0
	}
	// No button
	noX := dx + quitDialogW/2 + 4
	if mx >= noX && mx <= noX+quitBtnW && my >= btnY && my <= btnY+quitBtnH {
		return 1
	}
	return -1
}

func (d *deskpet) drawQuitConfirm(screen *ebiten.Image) {
	dx, dy := d.quitConfirmPos()
	face := text.NewGoXFace(bitmapfont.Face)
	mx, my := ebiten.CursorPosition()

	// Dialog background
	vector.DrawFilledRect(screen, float32(dx), float32(dy), quitDialogW, quitDialogH,
		color.RGBA{40, 40, 40, 230}, false)
	vector.StrokeRect(screen, float32(dx), float32(dy), quitDialogW, quitDialogH,
		1, color.RGBA{180, 180, 180, 200}, false)

	// Title
	tw, _ := text.Measure("Quit?", face, 0)
	top := &text.DrawOptions{}
	top.GeoM.Translate(float64(dx)+(quitDialogW-tw)/2, float64(dy)+4)
	top.ColorScale.ScaleWithColor(color.White)
	text.Draw(screen, "Quit?", face, top)

	// Buttons
	btnY := dy + 22
	yesX := dx + quitDialogW/2 - quitBtnW - 4
	noX := dx + quitDialogW/2 + 4

	for i, label := range []string{"Yes", "No"} {
		bx := yesX
		if i == 1 {
			bx = noX
		}
		hovered := mx >= bx && mx <= bx+quitBtnW && my >= btnY && my <= btnY+quitBtnH
		btnColor := color.RGBA{180, 100, 100, 230}
		if i == 1 {
			btnColor = color.RGBA{100, 140, 100, 230}
		}
		if hovered {
			btnColor.R = min(btnColor.R+30, 255)
			btnColor.G = min(btnColor.G+30, 255)
			btnColor.B = min(btnColor.B+30, 255)
		}
		vector.DrawFilledRect(screen, float32(bx), float32(btnY), quitBtnW, quitBtnH, btnColor, false)

		lw, _ := text.Measure(label, face, 0)
		lt := &text.DrawOptions{}
		lt.GeoM.Translate(float64(bx)+(quitBtnW-lw)/2, float64(btnY)+2)
		lt.ColorScale.ScaleWithColor(color.White)
		text.Draw(screen, label, face, lt)
	}
}

func (d *deskpet) screenSize() (int, int) {
	m := ebiten.Monitor()
	return m.Size()
}

func (d *deskpet) pickRandomEdge() {
	screenW, screenH := d.screenSize()
	scaledW := int(float64(width) * d.cfg.Scale)
	scaledH := int(float64(height) * d.cfg.Scale)

	// Pick left or right edge, different from current
	for {
		edge := rand.Intn(2) // 0=left, 1=right
		if edge != d.peekEdge || d.peekCount == 0 {
			d.peekEdge = edge
			break
		}
	}

	// Offset to account for transparent padding in sprite (about 18% of cell on each side)
	pad := scaledW * 30 / 100
	switch d.peekEdge {
	case 0: // left edge: face right (into screen)
		d.x = -pad
		d.y = rand.Intn(screenH - scaledH)
		d.facingLeft = false
	case 1: // right edge: face left (into screen)
		d.x = screenW - scaledW + pad
		d.y = rand.Intn(screenH - scaledH)
		d.facingLeft = true
	}
	ebiten.SetWindowPosition(d.x, d.y)
}

func (d *deskpet) checkPeekMouse() {
	mx, my := ebiten.CursorPosition()
	wx, wy := ebiten.WindowPosition()
	scaledW := int(float64(width) * d.cfg.Scale)
	scaledH := int(float64(height) * d.cfg.Scale)

	// Mouse position in screen coords
	screenMX := wx + mx
	screenMY := wy + my

	// Window center
	cx := wx + scaledW/2
	cy := wy + scaledH/2

	dx := screenMX - cx
	dy := screenMY - cy
	dist := math.Sqrt(float64(dx*dx + dy*dy))

	// If mouse gets close, retreat and pick new edge
	if dist < float64(scaledW) {
		d.peekCount++
		if d.peekCount >= 3 {
			// After 3 peeks, stay and let user click
			return
		}
		d.pickRandomEdge()
		d.frameIndex = 0
		d.frameCount = 0
	}
}

func (d *deskpet) catchCursor() {
	mx, my := ebiten.CursorPosition()
	lw, lh := d.Layout(0, 0)
	x := mx - (lw / 2)
	y := my - (lh / 2)

	// Always face toward cursor, even when staying
	if x < 0 {
		d.facingLeft = true
	} else if x > 0 {
		d.facingLeft = false
	}

	if !d.following {
		return
	}

	// Calculate Euclidean distance from pet center to cursor
	distance := math.Sqrt(float64(x*x + y*y))

	// Stop distance must be larger than menu right edge from center
	if distance < 200 {
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
	case StateWork:
		frames = d.workFrames
	case StateHide:
		frames = d.hideFrames
	case StatePeek:
		frames = d.peekFrames
	case StateFound:
		frames = d.foundFrames
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

	if d.showMenu {
		d.drawMenu(screen)
	}

	if d.showQuitConfirm {
		d.drawQuitConfirm(screen)
	}

	d.drawBubble(screen)
	d.drawTimerBadge(screen)
}

func (d *deskpet) Layout(outsideWidth, outsideHeight int) (int, int) {
	return int(float64(width) * d.cfg.Scale), int(float64(height) * d.cfg.Scale)
}

func main() {
	cfg := loadConfig("deskpet.ini")

	idleFrames, walkFrames, happyFrames, pickFrames, playFrames, workFrames, hideFrames, peekFrames, peekTopFrames, peekBottomFrames, foundFrames := loadSprites()

	fmt.Printf("Loaded: %d idle + %d walk + %d happy + %d pick + %d play + %d work + %d hide + %d peek + %d found frames\n",
		len(idleFrames), len(walkFrames), len(happyFrames), len(pickFrames), len(playFrames),
		len(workFrames), len(hideFrames), len(peekFrames), len(foundFrames))
	fmt.Printf("Config: Speed=%d, Scale=%.1f, Focus=%dmin, Break=%d/%dmin, Cycles=%d\n",
		cfg.Speed, cfg.Scale, cfg.FocusDuration, cfg.ShortBreak, cfg.LongBreak, cfg.CyclesBeforeLong)

	// Init audio
	var audioCtx *audio.Context
	var notifyPCM []byte
	audioCtx = audio.NewContext(44100)
	if wavData, err := f.ReadFile("material/notify.wav"); err == nil {
		if decoded, err := wav.DecodeWithSampleRate(44100, bytes.NewReader(wavData)); err == nil {
			notifyPCM, _ = io.ReadAll(decoded)
		}
	}

	pet := &deskpet{
		idleFrames:   idleFrames,
		walkFrames:   walkFrames,
		happyFrames:  happyFrames,
		pickFrames:   pickFrames,
		playFrames:   playFrames,
		workFrames:   workFrames,
		hideFrames:       hideFrames,
		peekFrames:       peekFrames,
		peekTopFrames:    peekTopFrames,
		peekBottomFrames: peekBottomFrames,
		foundFrames:      foundFrames,
		cfg:          cfg,
		following:    true,
		iconHappy:    loadImage("material/icon_happy.png"),
		iconPlay:     loadImage("material/icon_play.png"),
		iconFollow:   loadImage("material/icon_follow.png"),
		iconStay:     loadImage("material/icon_stay.png"),
		iconPeekaboo: loadImage("material/icon_peekaboo.png"),
		iconWork:     loadImage("material/icon_work.png"),
		iconStop:     loadImage("material/icon_stop.png"),
		iconQuit:     loadImage("material/icon_quit.png"),
		audioContext: audioCtx,
		notifyPCM:    notifyPCM,
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
	ebiten.SetWindowTitle("Pawdock")

	err := ebiten.RunGameWithOptions(pet, &ebiten.RunGameOptions{
		InitUnfocused:     true,
		ScreenTransparent: true,
		SkipTaskbar:       true,
	})
	if err != nil {
		log.Fatal(err)
	}
}
