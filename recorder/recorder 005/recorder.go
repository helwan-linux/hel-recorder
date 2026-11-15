
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"syscall"
	"time"

	"github.com/gotk3/gotk3/glib"
)

type RecorderManager struct {
	ui             *AppUI
	cmd            *exec.Cmd
	stdinPipe      io.WriteCloser

	isRecording    bool
	isPaused       bool
	stopping       bool

	mu             sync.Mutex

	startTime      time.Time
	pausedDuration time.Duration
	pauseStart     time.Time

	stopTicker     chan bool
	done           chan struct{}
	outputFile     string
}

func NewRecorderManager(ui *AppUI) *RecorderManager {
	m := &RecorderManager{ui: ui}

	ui.BtnStart.Connect("clicked", m.ToggleRecordPause)
	ui.BtnStop.Connect("clicked", m.StopRecording)

	return m
}

func (m *RecorderManager) setStatus(status string) {
	glib.IdleAdd(func() {
		m.ui.LblStatus.SetText(status)
	})
}

func (m *RecorderManager) ToggleRecordPause() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopping {
		return
	}

	if !m.isRecording {
		m.startRecordingLocked()
		return
	}

	if m.isPaused {
		m.resumeRecordingLocked()
	} else {
		m.pauseRecordingLocked()
	}
}

func getNativeResolution() (string, error) {
	output, err := exec.Command("xrandr", "--current").Output()
	if err != nil {
		return "", fmt.Errorf("cannot detect screen resolution: %w", err)
	}

	re := regexp.MustCompile(`(?m)^\s*(\d+x\d+)\s+[\d.]+\*`)
	match := re.FindStringSubmatch(string(output))

	if len(match) < 2 {
		return "", fmt.Errorf("cannot detect native resolution")
	}

	return match[1], nil
}

func (m *RecorderManager) startRecordingLocked() {
	saveDir := m.ui.FileChooser.GetFilename()

	if saveDir == "" {
		m.setStatus("Error: Select a save directory")
		return
	}

	info, err := os.Stat(saveDir)
	if err != nil || !info.IsDir() {
		m.setStatus("Error: Invalid save directory")
		return
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		m.setStatus("Error: FFmpeg is not installed")
		return
	}

	display := os.Getenv("DISPLAY")
	if display == "" {
		display = ":0.0"
	}

	fps := m.ui.ComboFPS.GetActiveID()
	if fps != "30" && fps != "60" {
		fps = "30"
	}

	res := m.ui.ComboRes.GetActiveID()

	if res == "auto" {
		res, err = getNativeResolution()
		if err != nil {
			m.setStatus("Error: Cannot detect screen resolution")
			return
		}
	}

	if res != "1920x1080" &&
		res != "1280x720" {
		if _, err := regexp.MatchString(`^\d+x\d+$`, res); err != nil {
			m.setStatus("Error: Invalid resolution")
			return
		}
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	outputFile := filepath.Join(
		saveDir,
		fmt.Sprintf("Recording_%s.mp4", timestamp),
	)

	if _, err := os.Stat(outputFile); err == nil {
		outputFile = filepath.Join(
			saveDir,
			fmt.Sprintf("Recording_%s_%d.mp4", timestamp, time.Now().UnixNano()),
		)
	}

	args := []string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-thread_queue_size", "1024",
		"-f", "x11grab",
		"-framerate", fps,
		"-video_size", res,
		"-i", display,
	}

	recordAudio := m.ui.ChkAudio.GetActive()

	if recordAudio {
		args = append(args,
			"-thread_queue_size", "1024",
			"-f", "pulse",
			"-i", "default",
		)
	}

	args = append(args,
		"-map", "0:v:0",
	)

	if recordAudio {
		args = append(args,
			"-map", "1:a:0",
			"-c:a", "aac",
			"-b:a", "160k",
		)
	}

	args = append(args,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "20",
		"-pix_fmt", "yuv420p",
		"-r", fps,
		"-movflags", "+faststart",
		outputFile,
	)

	cmd := exec.Command("ffmpeg", args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		m.setStatus("Error: Cannot create FFmpeg input")
		return
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		m.setStatus("Error: Cannot start FFmpeg")
		return
	}

	m.cmd = cmd
	m.stdinPipe = stdin
	m.outputFile = outputFile

	m.isRecording = true
	m.isPaused = false
	m.stopping = false

	m.startTime = time.Now()
	m.pausedDuration = 0

	m.stopTicker = make(chan bool)
	m.done = nil

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
	if m.cmd == nil || m.cmd.Process == nil {
		return
	}

	err := m.cmd.Process.Signal(syscall.SIGSTOP)
	if err != nil {
		m.setStatus("Error: Cannot pause recording")
		return
	}

	m.isPaused = true
	m.pauseStart = time.Now()

	glib.IdleAdd(func() {
		m.ui.BtnStart.SetLabel("Resume (Ctrl+R)")
		m.ui.LblStatus.SetText("Status: Paused")
	})
}

func (m *RecorderManager) resumeRecordingLocked() {
	if m.cmd == nil || m.cmd.Process == nil {
		return
	}

	err := m.cmd.Process.Signal(syscall.SIGCONT)
	if err != nil {
		m.setStatus("Error: Cannot resume recording")
		return
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

	if !m.isRecording || m.stopping {
		m.mu.Unlock()
		return
	}

	m.stopping = true

	if m.isPaused && m.cmd != nil {
		_ = m.cmd.Process.Signal(syscall.SIGCONT)
		m.isPaused = false
	}

	cmd := m.cmd
	stdin := m.stdinPipe
	outputFile := m.outputFile
	ticker := m.stopTicker

	m.done = make(chan struct{})
	done := m.done

	if stdin != nil {
		_, _ = stdin.Write([]byte("q\n"))
	}

	m.mu.Unlock()

	if ticker != nil {
		close(ticker)
	}

	go func() {
		err := cmd.Wait()

		if stdin != nil {
			_ = stdin.Close()
		}

		fileInfo, statErr := os.Stat(outputFile)

		success := err == nil &&
			statErr == nil &&
			fileInfo.Size() > 0

		m.mu.Lock()

		m.isRecording = false
		m.isPaused = false
		m.stopping = false
		m.cmd = nil
		m.stdinPipe = nil
		m.stopTicker = nil

		m.mu.Unlock()

		glib.IdleAdd(func() {
			m.ui.BtnStart.SetLabel("Start (Ctrl+R)")
			m.ui.BtnStop.SetSensitive(false)
			m.ui.ChkAudio.SetSensitive(true)
			m.ui.ComboFPS.SetSensitive(true)
			m.ui.ComboRes.SetSensitive(true)
			m.ui.FileChooser.SetSensitive(true)

			if success {
				m.ui.LblStatus.SetText("Status: Saved successfully!")
			} else {
				m.ui.LblStatus.SetText("Error: Recording or saving failed")
			}

			m.ui.LblTimer.SetMarkup(
				"<span font='28' weight='bold' color='#3584e4'>00:00:00</span>",
			)
		})

		close(done)
	}()
}

func (m *RecorderManager) WaitForStop() {
	m.mu.Lock()

	recording := m.isRecording
	stopping := m.stopping
	done := m.done

	m.mu.Unlock()

	if recording && !stopping {
		m.StopRecording()

		m.mu.Lock()
		done = m.done
		m.mu.Unlock()
	}

	if done != nil {
		<-done
	}
}

func (m *RecorderManager) runTimer() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopTicker:
			return

		case <-ticker.C:
			m.mu.Lock()

			if !m.isRecording {
				m.mu.Unlock()
				return
			}

			if m.isPaused {
				m.mu.Unlock()
				continue
			}

			elapsed := time.Since(m.startTime) -
				m.pausedDuration

			m.mu.Unlock()

			hours := int(elapsed.Hours())
			minutes := int(elapsed.Minutes()) % 60
			seconds := int(elapsed.Seconds()) % 60

			timeStr := fmt.Sprintf(
				"<span font='28' weight='bold' color='#e01b24'>%02d:%02d:%02d</span>",
				hours,
				minutes,
				seconds,
			)

			glib.IdleAdd(func() {
				m.ui.LblTimer.SetMarkup(timeStr)
			})
		}
	}
}
