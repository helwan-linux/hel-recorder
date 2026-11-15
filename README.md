# Helwan Screen Recorder

**A simple, lightweight screen and audio recorder for Linux.**

Helwan Screen Recorder is a GTK3 desktop application developed in Go for recording screen activity and audio on Linux systems.

Designed with simplicity and practicality in mind, it provides the essential recording features without unnecessary complexity.

## Features

- Screen recording using FFmpeg.
- Microphone audio recording.
- System audio recording.
- Microphone and system audio recording simultaneously.
- MP4 video output.
- Configurable frame rates (30/60 FPS).
- Recording pause and resume.
- Recording timer.
- CPU-based encoding with hardware acceleration support where available.
- Countdown before recording.
- Screen magnifier support.

## Technology

- **Language:** Go
- **GUI:** GTK3
- **Recording Engine:** FFmpeg
- **Display Server:** X11
- **Architecture:** x86_64

## Requirements

- Linux with X11.
- GTK3.
- FFmpeg.
- Go (for building from source).

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
