package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/gotk3/gotk3/glib"
)

type RecorderManager struct {
	ui             *AppUI
	cmd            *exec.Cmd
	stdinPipe      io.WriteCloser
	isRecording    bool
	isPaused       bool
	mu             sync.Mutex
	startTime      time.Time
	pausedDuration time.Duration
	pauseStart     time.Time
	stopTicker     chan bool
}

func NewRecorderManager(ui *AppUI) *RecorderManager {
	manager := &RecorderManager{ui: ui}
	ui.BtnStart.Connect("clicked", manager.ToggleRecordPause)
	ui.BtnStop.Connect("clicked", manager.StopRecording)
	return manager
}

func (m *RecorderManager) ToggleRecordPause() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRecording {
		m.startRecordingLocked()
	} else if m.isPaused {
		m.resumeRecordingLocked()
	} else {
		m.pauseRecordingLocked()
	}
}

func (m *RecorderManager) startRecordingLocked() {
	saveDir := m.ui.FileChooser.GetFilename()
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	outputFile := filepath.Join(saveDir, fmt.Sprintf("Recording_%s.mp4", timestamp))

	fps := m.ui.ComboFPS.GetActiveID()
	res := m.ui.ComboRes.GetActiveID()
	recordAudio := m.ui.ChkAudio.GetActive()

	var args []string

	if runtime.GOOS == "windows" {
		args = []string{"-y", "-f", "gdigrab", "-framerate", fps, "-i", "desktop"}
		if recordAudio {
			args = append(args, "-f", "dshow", "-i", "audio=Microphone", "-c:a", "aac", "-b:a", "192k")
		}
	} else {
		display := os.Getenv("DISPLAY")
		if display == "" {
			display = ":0.0"
		}
		args = []string{"-y", "-framerate", fps}
		if res != "auto" {
			args = append(args, "-video_size", res)
		}
		args = append(args, "-f", "x11grab", "-i", display)
		if recordAudio {
			args = append(args, "-f", "pulse", "-i", "default", "-c:a", "aac", "-b:a", "192k")
		}
	}

	args = append(args, "-c:v", "libx264", "-preset", "ultrafast", "-crf", "18", "-pix_fmt", "yuv420p", outputFile)

	var err error
	m.cmd = exec.Command("ffmpeg", args...)
	m.stdinPipe, err = m.cmd.StdinPipe()
	if err != nil {
		m.ui.LblStatus.SetText("Error: Pipe creation failed")
		return
	}

	if err := m.cmd.Start(); err != nil {
		m.ui.LblStatus.SetText("Error: FFmpeg not found in PATH")
		return
	}

	m.isRecording = true
	m.isPaused = false
	m.startTime = time.Now()
	m.pausedDuration = 0
	m.stopTicker = make(chan bool)

	glib.IdleAdd(func() {
		m.ui.BtnStart.SetLabel("Pause (Ctrl+R)")
		m.ui.BtnStop.SetSensitive(true)
		m.ui.ChkAudio.SetSensitive(false)
		m.ui.ComboFPS.SetSensitive(false)
		m.ui.ComboRes.SetSensitive(false)
		m.ui.FileChooser.SetSensitive(false)
		m.ui.LblStatus.SetText("Status: Recording...")
	})

	go m.runTimer()
}

func (m *RecorderManager) pauseRecordingLocked() {
	if m.stdinPipe != nil {
		_, _ = m.stdinPipe.Write([]byte("p\n"))
	}
	m.isPaused = true
	m.pauseStart = time.Now()

	glib.IdleAdd(func() {
		m.ui.BtnStart.SetLabel("Resume (Ctrl+R)")
		m.ui.LblStatus.SetText("Status: Paused")
	})
}

func (m *RecorderManager) resumeRecordingLocked() {
	if m.stdinPipe != nil {
		_, _ = m.stdinPipe.Write([]byte("p\n"))
	}
	m.pausedDuration += time.Since(m.pauseStart)
	m.isPaused = false

	glib.IdleAdd(func() {
		m.ui.BtnStart.SetLabel("Pause (Ctrl+R)")
		m.ui.LblStatus.SetText("Status: Recording...")
	})
}

func (m *RecorderManager) StopRecording() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.isRecording {
		return
	}

	if m.stdinPipe != nil {
		_, _ = m.stdinPipe.Write([]byte("q\n"))
		_ = m.stdinPipe.Close()
	}

	_ = m.cmd.Wait()
	m.isRecording = false
	m.isPaused = false
	close(m.stopTicker)

	glib.IdleAdd(func() {
		m.ui.BtnStart.SetLabel("Start (Ctrl+R)")
		m.ui.BtnStop.SetSensitive(false)
		m.ui.ChkAudio.SetSensitive(true)
		m.ui.ComboFPS.SetSensitive(true)
		m.ui.ComboRes.SetSensitive(true)
		m.ui.FileChooser.SetSensitive(true)
		m.ui.LblStatus.SetText("Status: Saved successfully!")
		m.ui.LblTimer.SetMarkup("<span font='28' weight='bold' color='#3584e4'>00:00:00</span>")
	})
}

func (m *RecorderManager) runTimer() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopTicker:
			return
		case <-ticker.C:
			m.mu.Lock()
			if m.isPaused {
				m.mu.Unlock()
				continue
			}
			elapsed := time.Since(m.startTime) - m.pausedDuration
			m.mu.Unlock()

			hours := int(elapsed.Hours())
			minutes := int(elapsed.Minutes()) % 60
			seconds := int(elapsed.Seconds()) % 60
			timeStr := fmt.Sprintf("<span font='28' weight='bold' color='#e01b24'>%02d:%02d:%02d</span>", hours, minutes, seconds)
			glib.IdleAdd(func() {
				m.ui.LblTimer.SetMarkup(timeStr)
			})
		}
	}
}
