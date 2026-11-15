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
	ui          *AppUI
	cmd         *exec.Cmd
	stdinPipe   io.WriteCloser
	isRecording bool
	mu          sync.Mutex
	startTime   time.Time
	stopTicker  chan bool
}

func NewRecorderManager(ui *AppUI) *RecorderManager {
	manager := &RecorderManager{ui: ui}

	ui.BtnStart.Connect("clicked", manager.StartRecording)
	ui.BtnStop.Connect("clicked", manager.StopRecording)

	return manager
}

func (m *RecorderManager) StartRecording() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRecording {
		return
	}

	saveDir := m.ui.FileChooser.GetFilename()
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	outputFile := filepath.Join(saveDir, fmt.Sprintf("Recording_%s.mp4", timestamp))

	fps := m.ui.ComboFPS.GetActiveID()
	res := m.ui.ComboRes.GetActiveID()
	recordAudio := m.ui.ChkAudio.GetActive()

	var args []string

	if runtime.GOOS == "windows" {
		args = []string{
			"-y",
			"-f", "gdigrab",
			"-framerate", fps,
			"-i", "desktop",
		}
		if recordAudio {
			args = append(args,
				"-f", "dshow",
				"-i", "audio=Microphone",
				"-c:a", "aac",
				"-b:a", "192k",
			)
		}
	} else {
		display := os.Getenv("DISPLAY")
		if display == "" {
			display = ":0.0"
		}
		args = []string{
			"-y",
			"-framerate", fps,
		}
		if res != "auto" {
			args = append(args, "-video_size", res)
		}
		args = append(args,
			"-f", "x11grab",
			"-i", display,
		)
		if recordAudio {
			args = append(args,
				"-f", "pulse",
				"-i", "default",
				"-c:a", "aac",
				"-b:a", "192k",
			)
		}
	}

	args = append(args,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "18",
		"-pix_fmt", "yuv420p",
		outputFile,
	)

	m.cmd = exec.Command("ffmpeg", args...)
	var err error
	m.stdinPipe, err = m.cmd.StdinPipe()
	if err != nil {
		m.ui.LblStatus.SetText("Error: Failed to create FFmpeg pipe")
		return
	}

	if err := m.cmd.Start(); err != nil {
		m.ui.LblStatus.SetText("Error: Ensure FFmpeg is installed and in PATH")
		return
	}

	m.isRecording = true
	m.startTime = time.Now()
	m.stopTicker = make(chan bool)

	m.ui.BtnStart.SetSensitive(false)
	m.ui.BtnStop.SetSensitive(true)
	m.ui.ChkAudio.SetSensitive(false)
	m.ui.ComboFPS.SetSensitive(false)
	m.ui.ComboRes.SetSensitive(false)
	m.ui.FileChooser.SetSensitive(false)
	m.ui.LblStatus.SetText("Status: Recording...")

	go m.runTimer()
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
	close(m.stopTicker)

	glib.IdleAdd(func() {
		m.ui.BtnStart.SetSensitive(true)
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
			elapsed := time.Since(m.startTime)
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
