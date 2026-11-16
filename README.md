# Helwan Screen Recorder

**A simple, lightweight, and professional screen and audio recorder for Linux.**

Helwan Screen Recorder is a GTK3 desktop application developed in Go for recording screen activity and audio on Linux systems, tailored specifically for **Helwan Linux** and content creators.

Designed with simplicity and practicality in mind, it provides robust recording features without unnecessary complexity.

## Features

- **Flexible Recording:** Full screen or custom selected area (using `slop`).
- **Professional Audio:** Simultaneous microphone and system audio recording.
- **Pre-start Mute Check:** Automatically detects if your microphone is muted and offers instant unmuting before recording.
- **Pause & Resume:** Clean segment-based recording with instant audio muting/unmuting on pause.
- **Webcam PiP Overlay:** Built-in picture-in-picture webcam integration (`/dev/video0`) via FFmpeg.
- **Keystrokes Overlay:** Visual feedback for keyboard shortcuts using `screenkey`.
- **Hardware Acceleration:** Automatic VAAPI support with intelligent CPU fallback.
- **Timer & Countdown:** OSD countdown delay before recording starts.
- **Magnifier Support:** Quick toggle screen magnifier (`Ctrl+G`).
- **Automatic Window Management:** Auto-minimize to taskbar on start and restore on finish.

## Technology

- **Language:** Go
- **GUI:** GTK3 (`gotk3`)
- **Recording Engine:** FFmpeg & FFprobe
- **Display Server:** X11
- **Architecture:** x86_64

## Requirements

- Linux with X11
- GTK3
- FFmpeg
- PulseAudio (`pactl`)
- **Optional (for advanced features):** `slop` (area selection), `screenkey` (keystrokes overlay)

## Build

Clone the repository:

```bash
git clone https://github.com/helwan-linux/hel-recorder.git
cd hel-recorder
```

Build the application:

```bash
go build -o hel-recorder .
```

Run:

```bash
./hel-recorder
```

## Keyboard Shortcuts

- **`Ctrl + R`**: Start recording / Cancel countdown
- **`Ctrl + E`**: Pause / Resume recording
- **`Ctrl + Q`**: Stop and save recording
- **`Ctrl + G`**: Toggle Magnifier

## Design Philosophy

Helwan Screen Recorder follows a simple principle:

**Essential features. No unnecessary complexity.**

It aims to provide the recording capabilities users actually need through a straightforward desktop interface.

## Project

Part of **Helwan Linux**.

Website: https://helwan-linux.github.io/helwanlinux/

GitHub: https://github.com/helwan-linux

## License

See the LICENSE file for licensing information.
