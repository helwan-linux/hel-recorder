// camera.go
package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// findWebcamDevice returns the first existing V4L2 video device.
func findWebcamDevice() string {
	matches, err := filepath.Glob("/dev/video*")
	if err != nil {
		return ""
	}
	for _, device := range matches {
		info, err := os.Stat(device)
		if err == nil && !info.IsDir() {
			return device
		}
	}
	return ""
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

// startWebcamPreviewLocked starts a preview. Caller must hold m.mu.
func (m *RecorderManager) startWebcamPreviewLocked(size, pos string) {
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
