// camera.go
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// findWebcamDevice returns the first V4L2 *capture* device.
//
// Many cameras expose two nodes (for example /dev/video0 for video and
// /dev/video1 for metadata). The sysfs "index" file is 0 for the primary node,
// so it is preferred over a plain first-match.
func findWebcamDevice() string {
	matches, err := filepath.Glob("/dev/video*")
	if err != nil {
		return ""
	}

	// Natural order: video2 before video10.
	sort.Slice(matches, func(i, j int) bool {
		if len(matches[i]) != len(matches[j]) {
			return len(matches[i]) < len(matches[j])
		}
		return matches[i] < matches[j]
	})

	fallback := ""

	for _, device := range matches {
		info, err := os.Stat(device)
		if err != nil || info.IsDir() {
			continue
		}

		if fallback == "" {
			fallback = device
		}

		indexPath := filepath.Join(
			"/sys/class/video4linux",
			filepath.Base(device),
			"index",
		)

		if data, err := os.ReadFile(indexPath); err == nil &&
			strings.TrimSpace(string(data)) == "0" {
			return device
		}
	}

	return fallback
}

var mjpegSupport sync.Map // device path -> bool

// webcamSupportsMJPEG reports whether the camera can deliver MJPEG frames.
// The answer is cached per device.
func webcamSupportsMJPEG(device string) bool {
	if cached, ok := mjpegSupport.Load(device); ok {
		return cached.(bool)
	}

	out, _ := exec.Command(
		"ffmpeg",
		"-hide_banner",
		"-f", "v4l2",
		"-list_formats", "all",
		"-i", device,
	).CombinedOutput()

	supported := strings.Contains(strings.ToLower(string(out)), "mjpeg")
	mjpegSupport.Store(device, supported)

	return supported
}

func webcamOverlayPosition(pos string) string {
	switch pos {
	case "bottom-left":
		return "20:main_h-overlay_h-20"
	case "top-right":
		return "main_w-overlay_w-20:20"
	case "top-left":
		return "20:20"
	default:
		return "main_w-overlay_w-20:main_h-overlay_h-20"
	}
}

// startWebcamPreviewLocked starts an ffplay preview window. Caller must hold
// m.mu.
//
// NOTE: it is intentionally NOT used while recording. A V4L2 camera normally
// allows only one process to stream from it, so opening it in ffplay and in
// the recording FFmpeg at the same time makes one of them fail, and the
// preview window would also be burned into the screen capture. It is kept for
// future use (for example a preview fed from a v4l2loopback device).
func (m *RecorderManager) startWebcamPreviewLocked(size, pos string) {
	_ = pos

	if m.camPreviewCmd != nil {
		return
	}

	device := findWebcamDevice()
	if device == "" {
		return
	}

	ffplay, err := exec.LookPath("ffplay")
	if err != nil {
		return
	}

	if size == "" {
		size = "320x240"
	}

	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-fflags", "nobuffer", "-flags", "low_delay",
		"-f", "v4l2", "-framerate", "30",
		"-video_size", size, "-i", device, "-an",
		"-window_title", "Helwan Webcam Preview",
	}

	cmd := exec.Command(ffplay, args...)
	if err := cmd.Start(); err != nil {
		return
	}

	m.camPreviewCmd = cmd

	go func(c *exec.Cmd) {
		_ = c.Wait()

		m.mu.Lock()
		if m.camPreviewCmd == c {
			m.camPreviewCmd = nil
		}
		m.mu.Unlock()
	}(cmd)
}

// stopWebcamPreviewLocked stops the preview. Caller must hold m.mu.
func (m *RecorderManager) stopWebcamPreviewLocked() {
	if m.camPreviewCmd == nil {
		return
	}

	cmd := m.camPreviewCmd
	m.camPreviewCmd = nil

	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
