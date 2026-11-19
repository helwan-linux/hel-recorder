<div align="center">

<img src="./App.jpg" width="280" alt="Helwan Screen Recorder">

# Helwan Screen Recorder

**A simple, lightweight, and practical screen and audio recorder for Linux.**

</div>


# Helwan Screen Recorder

**A simple, lightweight, and practical screen and audio recorder for Linux.**

Helwan Screen Recorder is a GTK3 desktop application written in **Go** for recording the desktop, microphone, and system audio on Linux.

It is designed around a simple idea:

> **Press Record. Capture what you need. Pause when you want. Save a clean recording.**

Helwan Screen Recorder uses **FFmpeg** for recording and encoding, while providing a simple GTK3 interface without the complexity of a full production-oriented recording suite.

---

## Features

### Screen Recording

- Full-screen recording at the native X11 desktop resolution.
- Custom recording-area selection using `slop`.
- 30 FPS or 60 FPS.
- X11 screen capture through FFmpeg `x11grab`.
- Automatic desktop-resolution detection using `xrandr`, with `xdpyinfo` fallback.
- Automatic validation of the selected recording resolution.

### Audio Recording

- Microphone recording.
- System audio recording.
- Microphone + system audio recording at the same time.
- Selection of the microphone source.
- Selection of the system-audio monitor source.
- PulseAudio/Pulse-compatible source detection through `pactl`.
- Automatic detection of muted microphones.
- Option to unmute the selected microphone before recording.
- Separate microphone and system-audio inputs.
- Automatic mixing of microphone and system audio when both are enabled.
- AAC audio for MP4/MKV.
- Opus audio for WebM.
- 192 kbps audio encoding.

## Reliable Audio Recording, Without Guesswork

Audio reliability is a core design priority in Helwan Screen Recorder.

- Explicit microphone and system-audio source selection.
- Microphone mute detection before recording starts.
- Clear handling of muted microphone states.
- Independent audio inputs with controlled mixing.
- Audio muting and restoration during Pause and Resume.
- Clear recording status and error reporting.

The goal is to help prevent silent recordings and unexpected audio interruptions through proactive source detection and validation.

## Designed for Everyday Use

You don't need to understand FFmpeg or complicated recording settings.

1. Choose what you want to record.
2. Select your audio sources.
3. Press Record.
4. Pause, resume, or stop whenever you need.

**Simple controls. Clear feedback. No unnecessary complexity.**

### Recording Formats

#### MP4

- H.264 video.
- AAC audio.
- `faststart` enabled for better playback/streaming compatibility.

#### MKV

- H.264 video.
- AAC audio.
- Useful when recording stability and container resilience are important.

#### WebM

- VP9 video.
- Opus audio.
- Suitable for web-oriented recordings.

#### Animated GIF

- 15 FPS.
- 720px output width.
- Automatic palette generation and palette usage for better GIF quality.
- Intended for short clips.

> Audio recording is disabled for GIF output.

---

## Pause & Resume

Helwan Screen Recorder uses a **segment-based Pause/Resume system**.

When recording is paused:

1. The current FFmpeg recording process is cleanly closed.
2. The current recording part is preserved.
3. Audio sources are muted when applicable.
4. Recording can later be resumed as a new segment.
5. When recording ends, the recorded segments are joined automatically.

This avoids relying on a fragile in-process pause mechanism and allows the recording session to continue across multiple segments.

The final result is presented as a single recording file.

---

## Hardware Acceleration

Helwan Screen Recorder automatically checks for a usable **VAAPI H.264 encoder**.

It searches available DRM render devices under:

`/dev/dri/renderD*`

and performs an actual FFmpeg encoding test before selecting VAAPI.

If a working VAAPI device is found:

`h264_vaapi`

is used.

Otherwise, the recorder automatically falls back to:

`libx264`

This means the application does not simply assume that hardware encoding is available.

---

## Webcam

If `/dev/video0` is available, Helwan Screen Recorder provides a webcam preview option.

Available preview sizes:

- 240 × 180
- 320 × 240
- 480 × 360

Available positions:

- Bottom-Right
- Bottom-Left
- Top-Right
- Top-Left

The webcam preview uses `ffplay` and remains on top of other windows.

> **Note:** The current C003 implementation provides a live webcam preview window. The webcam stream is not composited into the recorded video file itself.

---

## Keystroke Display

Optional on-screen keyboard visualization is provided through **Screenkey**.

Enable:

`Show Keystrokes on screen`

when you want keyboard actions to appear during a recording.

This is useful for:

- Tutorials
- Software demonstrations
- Programming videos
- Linux tutorials
- Educational recordings

---

## Floating Recording OSD

During recording, Helwan Screen Recorder displays a small floating status control.

Example:

`🔴 REC 00:12:34   ⏸   ⏹`

While paused:

`⏸ PAUSED 00:12:34`

The OSD provides quick access to:

- Pause / Resume
- Stop

without requiring the main application window to remain visible.

---

## Countdown

Recording can start after an optional countdown:

- No Countdown
- 3 seconds
- 5 seconds
- 10 seconds

The countdown is displayed in a large on-screen window.

The countdown can also be cancelled before recording begins.

---

## Automatic Minimize

Helwan Screen Recorder can automatically minimize its main window when recording starts.

This keeps the recorder itself out of the captured desktop area.

The window is restored automatically when the recording finishes.

---

## Global Keyboard Shortcuts

The recorder provides global X11 shortcuts.

| Shortcut | Action |
|---|---|
| `Ctrl + R` | Start recording |
| `Ctrl + E` | Pause / Resume |
| `Ctrl + Q` | Stop and save |
| `Ctrl + G` | Toggle magnifier |

These shortcuts work outside the recorder's main window.

The shortcuts are implemented directly through X11 global key grabs.

---

## Magnifier

`Ctrl + G` launches or closes **KMag**.

This can be useful when demonstrating:

- Small UI elements
- Desktop settings
- Terminal commands
- Accessibility features
- Detailed interface operations

---

## Recording Workflow

A typical recording session looks like this:

```text
Launch Helwan Screen Recorder
          │
          ▼
Select screen / area
          │
          ▼
Select video format
          │
          ▼
Select microphone / system audio
          │
          ▼
Optional countdown
          │
          ▼
        RECORD
          │
          ├───────────────┐
          │               │
          ▼               ▼
       PAUSE           CONTINUE
          │
          ▼
      RESUME
          │
          ▼
        STOP
          │
          ▼
 Merge recording parts
          │
          ▼
      Save video
```

---

## Audio Architecture

Audio sources are discovered through PulseAudio-compatible `pactl` commands.

The recorder distinguishes between:

```text
Microphone sources
        │
        └── physical/input sources

System Audio sources
        │
        └── .monitor sources
```

When both microphone and system audio are selected:

```text
Microphone ──────┐
                 ├── FFmpeg amix ──► Audio Output
System Audio ────┘
```

This keeps the two audio paths explicit instead of treating the entire audio system as a single unknown input.

---

## Requirements

Helwan Screen Recorder is designed for **Linux/X11**.

Required runtime components:

- GTK3
- FFmpeg
- PulseAudio-compatible audio interface / `pactl`
- X11
- KMag
- `slop`
- Screenkey

For webcam preview:

- `ffplay`
- V4L2 webcam available as `/dev/video0`

On Arch Linux / Helwan Linux, the required packages can be installed with:

```bash
sudo pacman -S ffmpeg gtk3 libpulse libx11 kmag slop screenkey
```

---

## Building From Source

### Requirements

For building:

- Go
- GTK3 development libraries
- X11 development libraries
- FFmpeg
- PulseAudio development/runtime support

Clone the repository:

```bash
git clone https://github.com/helwan-linux/hel-recorder.git
cd hel-recorder/hel-recorder
```

Build:

```bash
go build -o hel-recorder .
```

Run:

```bash
./hel-recorder
```

---

## Arch Linux Package

The project includes a `PKGBUILD` for Arch-based systems.

Build the package with:

```bash
makepkg -si
```

The package installs:

`/usr/bin/hel-recorder`

and the desktop entry:

`/usr/share/applications/helwan-screen-recorder.desktop`

---

## Output Files

Recordings are automatically generated with timestamped filenames such as:

`Recording_2026-11-17_09-30-15_123456789.mp4`

The default save location is:

`~/Videos`

if the directory exists.

Otherwise, the user's home directory is used.

---

## Design Goals

Helwan Screen Recorder is intentionally built around a small number of principles:

### Simple

The interface exposes the important recording controls without overwhelming the user.

### Practical

Features are focused on real recording workflows rather than adding complexity for its own sake.

### Lightweight

The application itself is written in Go with GTK3 and delegates video/audio processing to FFmpeg.

### Reliable

The recorder validates important dependencies and recording parameters before starting.

### Transparent

The application clearly reports recording state, errors, and the final output location.

### Linux-focused

The current implementation is designed specifically around Linux technologies such as:

- X11
- FFmpeg
- PulseAudio-compatible `pactl`
- V4L2
- VAAPI
- GTK3

---

## Designed for Everyday Use

Helwan Screen Recorder is designed to make screen recording straightforward.

Users can focus on their work instead of dealing with complicated recording configurations.

The application combines simple recording controls with practical features for everyday desktop recording.

**Record. Pause. Resume. Stop. That's it.**

---

## Technology Stack

| Component | Technology |
|---|---|
| Language | Go |
| GUI | GTK3 |
| Screen Capture | FFmpeg `x11grab` |
| Audio Capture | FFmpeg `pulse` |
| Audio Mixing | FFmpeg `amix` |
| H.264 | `libx264` / `h264_vaapi` |
| VP9 | `libvpx-vp9` |
| Audio | AAC / Opus |
| Webcam Preview | FFmpeg `ffplay` + V4L2 |
| Screen Area Selection | `slop` |
| Magnifier | KMag |
| Keystroke Display | Screenkey |
| Global Shortcuts | X11 |

---

## Project Structure

```text
📁 hel-recorder/
├── 📁 assets
│   └── 📄 icon.png
├── 📄 camera.go
├── 📄 features.go
├── 📄 formats.go
├── 📄 go.mod
├── 📄 go.sum
├── 📄 hel-recorder.desktop
├── 📄 main.go
├── 📄 recorder.go
├── 📄 style.css
├── 📄 ui.go
└── 📄 x11_hotkeys.go
```

### Main Components

`main.go`

Initializes GTK, the recorder manager, global shortcuts, and application shutdown handling.

`ui.go`

Contains the GTK3 interface, controls, dialogs, microphone warnings, help, and About dialog.

`recorder.go`

Contains the recording engine, FFmpeg command construction, audio handling, Pause/Resume, output handling, VAAPI detection, and recording lifecycle.

`features.go`

Contains optional recording features such as:

- Floating OSD
- Countdown
- KMag integration
- Screenkey integration
- Webcam preview

`x11_hotkeys.go`

Provides global X11 keyboard shortcuts.

---

## License

Helwan Screen Recorder is released under the **MIT License**.

See `LICENSE` for the complete license text.

---

## Helwan Linux

Helwan Screen Recorder is part of the Helwan Linux ecosystem.

**Helwan Linux — Arch Power, Helwan Simplicity.**

Project:

https://github.com/helwan-linux/hel-recorder

Helwan Linux:

https://github.com/helwan-linux

---

## Status

Helwan Screen Recorder is an actively developed Linux desktop application.

The current 1.1.0 version focuses on:

- Reliable screen recording
- Practical audio capture
- Simple Pause/Resume workflow
- Multiple output formats
- Hardware acceleration when available
- Useful recording overlays
- Global keyboard controls
- Lightweight GTK3 interface

The goal is not to reproduce every feature of large recording suites.

The goal is to provide a **fast, practical, and understandable recorder for Linux users**.
