# Pawdock

A cute desktop pet that lives on your screen. Built with Go and [Ebitengine](https://ebitengine.org/).

**Website**: [https://y4h2.github.io/pawdock/](https://y4h2.github.io/pawdock/)

![Pawdock](docs/idle.png)

## Features

- **Mouse Following** - Your pet follows your cursor around the screen with 8-directional movement
- **Drag & Play** - Pick up and move your pet anywhere you like
- **Peekaboo** - Play hide & seek as your pet peeks from screen edges
- **Pomodoro Timer** - Built-in focus timer (25min work / 5min break) with speech bubble reminders
- **Work Mode** - Stay focused while your pet keeps you company, with countdown display and break notifications

## Getting Started

### Download

Grab the latest release from the [Releases](https://github.com/y4h2/pawdock/releases) page.

### Build from Source

```bash
go run .
```

Requires Go 1.23+ and CGO enabled (for Ebitengine).

## Configuration

Create a `deskpet.ini` file in the same directory:

```ini
Speed = 1              # Movement speed
Scale = 0.5            # Window scale
FocusDuration = 25     # Focus duration (minutes)
ShortBreak = 5         # Short break (minutes)
LongBreak = 15         # Long break (minutes)
CyclesBeforeLong = 4   # Cycles before long break
```

## Controls

- **Click** pet to open the menu
- **Drag** to move the pet around
- **Esc** to quit

### Menu

| Button | Action |
|--------|--------|
| Happy | Play happy animation |
| Play | Play the play animation |
| Peekaboo | Start hide & seek game |
| Follow/Stay | Toggle mouse following |
| Work | Start/stop pomodoro timer |

## Tech Stack

- [Go](https://go.dev/)
- [Ebitengine](https://ebitengine.org/) - 2D game engine
- Sprite sheet animations (256x256, 9 columns, 12fps)

## License

MIT
