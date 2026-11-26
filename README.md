<div align="center">

<img src="./App.jpg" width="400" alt="Helwan Screen Recorder">

# Helwan Screen Recorder

**A simple, lightweight, and practical screen and audio recorder for Linux.**

</div>

# Helwan Screen Recorder

Helwan Screen Recorder is a GTK3 desktop application written in **Go** for recording the desktop, microphone, system audio, and webcam on **Linux/X11**.

It is designed around a simple idea:

> **Press Record. Capture what you need. Pause when you want. Save a clean recording.**

Helwan Screen Recorder uses **FFmpeg** for capture, encoding, audio processing, and finalization while providing a simple GTK3 interface without the complexity of a full production-oriented recording suite.

---

## Features

### Screen Recording

- Full-screen recording at the native X11 desktop resolution.
- Custom recording-area selection using `slop`.
- Window selection using `slop`.
- Monitor selection when multiple X11 monitors are available.
- 30 FPS or 60 FPS.
- X11 screen capture through FFmpeg `x11grab`.
- Automatic desktop-resolution detection using `xrandr`.
- `xdpyinfo` fallback when `xrandr` cannot provide the resolution.
- Automatic validation of the selected recording resolution.
- Mouse cursor capture.

### Audio Recording

- Microphone recording.
- System audio recording.
- Microphone + system audio recording at the same time.
- Selection of the microphone source.
- Selection of the system-audio monitor source.
- PulseAudio-compatible source detection through `pactl`.
- Automatic microphone mute detection.
- Option to unmute a muted microphone before recording.
- Separate microphone and system-audio inputs.
- Automatic mixing of microphone and system audio.
- AAC audio for MP4/MKV.
- Opus audio for WebM.
- 192 kbps audio encoding.
- Independent audio processing from the video recording process.

---

## Reliable Audio Recording

Audio reliability is an important part of Helwan Screen Recorder.

The recorder:

- Detects available PulseAudio-compatible sources.
- Clearly separates microphone and system-audio sources.
- Detects muted microphones before recording.
- Can restore microphone input when required.
- Keeps microphone and system audio as separate FFmpeg inputs.
- Mixes both sources only when both are enabled.
- Stops audio processing cleanly when recording is paused or stopped.
- Preserves recorded audio parts for final merging.

The goal is to avoid silent recordings and unexpected audio interruptions caused by an incorrectly selected or muted input.

---

## Low-Lag Recording Architecture

Helwan Screen Recorder is designed to reduce unnecessary interaction between the video and audio pipelines.

### Separate Video and Audio Processes

When audio is enabled, the recorder uses:

```text
X11 Screen ───────────────► FFmpeg Video
                                  │
                                  ▼
                           Video Recording Part


Microphone ────────┐
                   ├────────────► FFmpeg Audio
System Audio ──────┘
                                  │
                                  ▼
                           Audio Recording Part
```

The video FFmpeg process does not wait for PulseAudio processing or audio filters.

This separation helps prevent audio processing from unnecessarily blocking the video capture pipeline.

### Input Queue Detection

The recorder checks whether the installed FFmpeg version supports:

```text
-thread_queue_size
```

before using it.

If the option is not supported by the installed FFmpeg build, the recorder automatically avoids using it instead of starting with an invalid command.

### Hardware Encoder Detection

Hardware acceleration is tested before it is used.

The recorder performs an actual FFmpeg encoding test rather than assuming that a GPU encoder is available.

---

## Hardware Acceleration

Helwan Screen Recorder automatically detects usable hardware H.264 encoders.

The detection order is:

```text
VAAPI
  │
  └── Intel / AMD

NVENC
  │
  └── NVIDIA

CPU
  │
  └── libx264 fallback
```

### VAAPI

The recorder searches for available DRM render devices:

```text
/dev/dri/renderD*
```

Each candidate is tested using FFmpeg before being selected.

When a working VAAPI encoder is found:

```text
h264_vaapi
```

is used.

The recorder also tests the available pixel-format conversion path and can use GPU-side conversion when supported.

### NVENC

On NVIDIA systems, the recorder checks for:

```text
/dev/nvidiactl
```

and performs an actual `h264_nvenc` encoding test.

When usable:

```text
h264_nvenc
```

is selected.

### CPU Fallback

If no working hardware encoder is detected, the recorder automatically uses:

```text
libx264
```

with a low-latency configuration.

The application therefore does not simply assume that hardware encoding works.

---

## Quality and Bitrate

The Advanced panel provides three quality levels:

- **High** — better quality, larger files.
- **Standard** — recommended default.
- **Small** — smaller files, lower quality.

A fixed bitrate can also be selected:

- Auto
- 2 Mbps
- 4 Mbps
- 8 Mbps
- 12 Mbps
- 20 Mbps

When a fixed bitrate is selected, it overrides the quality-level setting.

---

## Recording Formats

### MP4

- H.264 video.
- AAC audio.
- `faststart` enabled.
- Suitable for general playback and sharing.

### MKV

- H.264 video.
- AAC audio.
- Designed to be more resilient during recording.
- Useful when recording stability is more important than immediate MP4 compatibility.

### WebM

- VP9 video.
- Opus audio.
- Realtime-oriented VP9 settings.
- Suitable for web-oriented recordings.

### Animated GIF

- 15 FPS.
- Maximum output width of 720 pixels.
- Automatic palette generation.
- Palette-based conversion for improved GIF quality.
- Intended for short clips.
- Audio is disabled.

---

## Safe Recording Parts

During recording, the application does not write directly to the final MP4 file.

Instead, recording parts are created in the selected destination directory.

For example:

```text
.hel_rec_part_123456789.mkv
```

This provides a safer intermediate recording format.

For MP4 output, the final file is created using stream-copy remuxing with:

```text
-movflags +faststart
```

The video is not unnecessarily re-encoded during MP4 finalization.

If finalization fails, the raw recording part is kept so that recorded footage is not silently lost.

---

## Pause & Resume

Helwan Screen Recorder uses a **segment-based Pause/Resume system**.

When recording is paused:

1. The current video FFmpeg process is cleanly closed.
2. The current video part is preserved.
3. The current audio part is preserved when audio is enabled.
4. Audio input is stopped cleanly.
5. Recording remains in the paused state.
6. Resume starts a new recording segment.
7. Multiple segments can be created during one recording session.
8. The segments are automatically merged when recording finishes.

The final result is presented as a single recording file.

This approach avoids depending on a fragile in-process FFmpeg pause mechanism.

---

## Webcam Overlay

Helwan Screen Recorder supports a webcam **PiP overlay**.

Enable:

```text
Webcam Overlay (PiP)
```

The webcam is composited into the recorded video itself.

Available sizes:

- 240 × 180
- 320 × 240
- 480 × 360

Available positions:

- Bottom-Right
- Bottom-Left
- Top-Right
- Top-Left

The recorder automatically detects a usable V4L2 webcam under:

```text
/dev/video*
```

It prefers the primary capture device when multiple video nodes are present.

If the webcam supports MJPEG, the recorder can use MJPEG input to reduce unnecessary camera-side processing.

The webcam overlay is not a separate floating window and is not simply captured from the desktop.

---

## Keystroke Display

Optional on-screen keyboard visualization is provided through **Screenkey**.

Enable:

```text
Show Keystrokes on screen
```

This is useful for:

- Tutorials
- Programming videos
- Software demonstrations
- Linux tutorials
- Educational recordings
- Keyboard-driven workflows

---

## Floating Recording OSD

During recording, Helwan Screen Recorder provides a floating status control.

Example:

```text
🔴 REC 00:12:34   ⏸   ⏹
```

While paused:

```text
⏸ PAUSED 00:12:34
```

The OSD provides quick access to:

- Pause / Resume
- Stop

The OSD can remain available while the main application window is minimized or hidden.

Its position is remembered for later recording sessions.

---

## System Tray

Helwan Screen Recorder provides a system-tray control when supported by the desktop environment.

The tray menu can provide:

- Show Window
- Start Recording
- Pause / Resume
- Stop and Save
- Quit

The tray state changes according to the current recording state.

For example:

```text
Ready
  ↓
Recording
  ↓
Paused
```

The application supports both traditional GTK status icons and compatible AppIndicator libraries when available.

---

## Countdown

Recording can start after an optional countdown:

- No Countdown
- 3 seconds
- 5 seconds
- 10 seconds

The countdown is displayed in a large on-screen window.

The countdown can be cancelled before recording begins.

---

## Automatic Minimize

Helwan Screen Recorder can automatically minimize its main window when recording starts.

This prevents the recorder's own interface from appearing in the captured desktop.

The main window is restored when recording finishes.

---

## Automatic Stop

The Advanced panel provides optional automatic recording limits:

- No limit
- 5 minutes
- 10 minutes
- 15 minutes
- 30 minutes
- 1 hour
- 2 hours

When the selected limit is reached, the recorder automatically follows the normal stop-and-save workflow.

---

## Low Disk Space Protection

Helwan Screen Recorder can monitor available disk space during recording.

Available settings:

- Off
- Keep 500 MB free
- Keep 1 GB free
- Keep 2 GB free
- Keep 5 GB free

The disk guard is designed around the actual temporary recording files.

Because finalization may require additional space, the recorder considers both:

```text
Required free-space margin
+
Space already occupied by the recording
```

When the available space becomes too low, the recorder automatically stops and saves the recording instead of continuing until the filesystem is full.

The default setting is:

```text
Keep 1 GB free
```

---

## Global Keyboard Shortcuts

The recorder provides global X11 keyboard shortcuts.

| Shortcut | Action |
|---|---|
| `Ctrl + R` | Start recording |
| `Ctrl + E` | Pause / Resume |
| `Ctrl + Q` | Stop and save |
| `Ctrl + G` | Toggle magnifier |

These shortcuts work outside the main application window.

They are implemented directly through X11 global key grabs.

The hotkey handling also accounts for:

- Caps Lock
- Num Lock
- Different keyboard layouts

The shortcuts are matched using X11 keycodes, so they continue to work when the active keyboard layout changes.

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
Select screen / monitor / area / window
          │
          ▼
Select FPS
          │
          ▼
Select video format
          │
          ▼
Select quality / bitrate
          │
          ▼
Select microphone / system audio
          │
          ▼
Optional webcam / keystrokes
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
Merge video/audio parts
          │
          ▼
Finalize output
          │
          ▼
      Save recording
```

---

## Audio Architecture

Audio sources are discovered through PulseAudio-compatible `pactl` commands.

The recorder distinguishes between:

```text
Microphone Sources
        │
        └── Physical/input sources


System Audio Sources
        │
        └── .monitor sources
```

When both are selected:

```text
Microphone ──────┐
                 │
                 ├── FFmpeg amix ──► Audio Output
                 │
System Audio ────┘
```

The audio recording process is independent from the video recording process.

After recording:

```text
Video Parts ──────► Video Merge
                         │
                         ├──────► Final Video
                         │
Audio Parts ──────► Audio Merge
                         │
                         └──────► Final Audio
```

The final video and audio streams are then muxed without unnecessary re-encoding.

---

## Requirements

Helwan Screen Recorder is designed for:

```text
Linux + X11
```

Required runtime components:

- GTK3
- FFmpeg
- PulseAudio-compatible `pactl`
- X11
- KMag
- `slop`
- Screenkey

For webcam overlay:

- FFmpeg V4L2 support
- A V4L2-compatible webcam under `/dev/video*`

On Arch Linux / Helwan Linux:

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
- `libx11`

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

## Debug Mode

For troubleshooting FFmpeg and recording behavior, enable:

```bash
HSR_DEBUG=1 ./hel-recorder
```

Debug mode prints information such as:

- Detected hardware encoder.
- FFmpeg command lines.
- Input queue support.
- FFmpeg runtime statistics.
- Recording pipeline information.

Example:

```text
[hsr] encoder detected: VAAPI ...
[hsr] input -thread_queue_size supported: true
```

This is useful when diagnosing recording performance, encoder selection, or FFmpeg compatibility.

---

## Force CPU Encoding

For testing or troubleshooting hardware acceleration, CPU encoding can be forced with:

```bash
HSR_FORCE_CPU=1 ./hel-recorder
```

This disables automatic VAAPI/NVENC selection and uses the CPU encoder.

---

## Arch Linux Package

The project includes a `PKGBUILD` for Arch-based systems.

Build and install:

```bash
makepkg -si
```

The package installs:

```text
/usr/bin/hel-recorder
```

Application icons are installed under:

```text
/usr/share/icons/hicolor/
```

The GTK3 stylesheet is installed under:

```text
/usr/share/helwan-screen-recorder/style.css
```

The desktop entry is installed as:

```text
/usr/share/applications/helwan-screen-recorder.desktop
```

---

## Output Files

Recordings are automatically generated with timestamped filenames such as:

```text
Recording_2026-11-17_09-30-15_123456789.mp4
```

The default save location is:

```text
~/Videos
```

when that directory exists.

Otherwise, the user's home directory is used.

Temporary recording parts are stored in the selected recording directory and are removed after successful finalization.

---

## Technology Stack

| Component | Technology |
|---|---|
| Language | Go |
| GUI | GTK3 |
| Screen Capture | FFmpeg `x11grab` |
| Audio Capture | FFmpeg `pulse` |
| Audio Mixing | FFmpeg `amix` |
| H.264 CPU | `libx264` |
| H.264 GPU | `h264_vaapi` / `h264_nvenc` |
| VP9 | `libvpx-vp9` |
| Audio | AAC / Opus |
| Webcam | V4L2 |
| Webcam Overlay | FFmpeg `overlay` |
| Screen Area / Window Selection | `slop` |
| Magnifier | KMag |
| Keystroke Display | Screenkey |
| Global Shortcuts | X11 |
| System Tray | GTK StatusIcon / AppIndicator |

---

## Project Structure

```text
hel-recorder/
├── assets/
│   └── icon.png
├── camera.go
├── config.go
├── features.go
├── formats.go
├── go.mod
├── go.sum
├── hel-recorder.desktop
├── main.go
├── monitors.go
├── options.go
├── recorder.go
├── style.css
├── tray.go
├── ui.go
└── x11_hotkeys.go
```

### Main Components

`main.go`

Initializes GTK, the recorder manager, global shortcuts, system tray integration, and application shutdown handling.

`ui.go`

Contains the GTK3 interface, recording controls, video settings, audio settings, Advanced settings, webcam options, dialogs, Help, and About.

`recorder.go`

Contains the main recording engine, FFmpeg command construction, separate video/audio pipelines, Pause/Resume, output handling, hardware encoder usage, disk protection, automatic stop, and recording lifecycle.

`formats.go`

Contains:

- Supported output formats.
- VAAPI detection.
- NVENC detection.
- CPU fallback.
- FFmpeg input queue compatibility detection.
- Encoder settings.
- Video part merging.
- MP4 remuxing.
- GIF conversion.
- Audio/video finalization.

`options.go`

Contains:

- Quality settings.
- Bitrate settings.
- Auto-stop settings.
- Disk-space protection.

`camera.go`

Contains webcam detection, MJPEG capability detection, and webcam overlay positioning.

`features.go`

Contains optional recording features such as:

- Floating OSD.
- Countdown.
- KMag integration.
- Screenkey integration.
- Webcam-related functionality.

`tray.go`

Provides system-tray integration and recording-state controls.

`monitors.go`

Handles monitor detection and monitor selection.

`x11_hotkeys.go`

Provides global X11 keyboard shortcuts.

`config.go`

Handles persistent recorder configuration.

---

## Design Goals

Helwan Screen Recorder is intentionally built around a small number of principles.

### Simple

The interface exposes important recording controls without overwhelming the user.

### Practical

Features are focused on real recording workflows rather than adding complexity for its own sake.

### Lightweight

The application is written in Go with GTK3 and delegates video/audio processing to FFmpeg.

### Reliable

The recorder validates important dependencies, recording parameters, hardware encoders, audio sources, and available disk space before and during recording.

### Low-Latency

The recording pipeline is designed to minimize unnecessary synchronization between screen capture, audio processing, and encoding.

### Transparent

The application clearly reports recording state, errors, and the final output location.

### Linux-Focused

The current implementation is designed specifically around Linux technologies such as:

- X11
- FFmpeg
- PulseAudio-compatible `pactl`
- V4L2
- VAAPI
- NVENC
- GTK3

---

## What Helwan Screen Recorder Is Not

Helwan Screen Recorder is not intended to reproduce every feature of large production-oriented recording suites.

It focuses on the features that are useful for everyday desktop recording:

- Screen capture
- Audio capture
- Webcam overlay
- Pause / Resume
- Multiple formats
- Hardware acceleration
- Low-lag recording architecture
- Global shortcuts
- Automatic stopping
- Disk protection
- Practical recording controls

The goal is to keep the application understandable while still providing the features that matter for real recording workflows.

---

## Status

Helwan Screen Recorder is an actively developed Linux desktop application.

The current **1.2.0** release focuses on:

- Reliable X11 screen recording.
- Separate video and audio recording pipelines.
- Reduced recording latency.
- VAAPI hardware acceleration.
- NVENC hardware acceleration.
- CPU fallback encoding.
- Runtime FFmpeg capability detection.
- Microphone and system audio recording.
- Webcam PiP overlay.
- Multiple output formats.
- Segment-based Pause/Resume.
- Floating recording OSD.
- System-tray controls.
- Global keyboard shortcuts.
- Screenkey integration.
- KMag integration.
- Automatic recording limits.
- Low-disk-space protection.
- Quality and bitrate controls.
- Crash-resilient intermediate recording parts.
- Lightweight GTK3 interface.

The goal is simple:

> **Fast, practical, reliable, and understandable screen recording for Linux.**

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
