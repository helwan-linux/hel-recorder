
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gotk3/gotk3/glib"
)

type RecorderManager struct {
	ui *AppUI

	cmd       *exec.Cmd
	stdinPipe io.WriteCloser

	isRecording bool
	isPaused    bool
	stopping    bool

	mu sync.Mutex

	startTime      time.Time
	pausedDuration time.Duration
	pauseStart     time.Time

	stopTicker chan struct{}
	done       chan struct{}
	outputFile string
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

func getNativeResolution() (string, error) {
	output, err := exec.Command("xrandr", "--current").Output()
	if err == nil {
		re := regexp.MustCompile(`current\s+(\d+)\s+x\s+(\d+)`)
		match := re.FindStringSubmatch(string(output))
		if len(match) == 3 {
			return match[1] + "x" + match[2], nil
		}
	}

	output, err = exec.Command("xdpyinfo").Output()
	if err == nil {
		re := regexp.MustCompile(`dimensions:\s+(\d+x\d+)`)
		match := re.FindStringSubmatch(string(output))
		if len(match) == 2 {
			return match[1], nil
		}
	}

	return "", fmt.Errorf("cannot detect X11 desktop dimensions")
}

func getPulseSources() ([]string, error) {
	output, err := exec.Command("pactl", "list", "short", "sources").Output()
	if err != nil {
		return nil, fmt.Errorf("cannot list audio sources: %w", err)
	}

	var sources []string

	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sources = append(sources, fields[1])
	}

	return sources, nil
}

func GetMicrophoneSources() ([]string, error) {
	sources, err := getPulseSources()
	if err != nil {
		return nil, err
	}

	var result []string
	for _, source := range sources {
		if !strings.HasSuffix(source, ".monitor") {
			result = append(result, source)
		}
	}

	return result, nil
}

func GetSystemAudioSources() ([]string, error) {
	sources, err := getPulseSources()
	if err != nil {
		return nil, err
	}

	var result []string
	for _, source := range sources {
		if strings.HasSuffix(source, ".monitor") {
			result = append(result, source)
		}
	}

	return result, nil
}

func (m *RecorderManager) selectedMicrophone() string {
	if m.ui.ComboMic == nil {
		return "default"
	}

	source := m.ui.ComboMic.GetActiveID()
	if strings.TrimSpace(source) == "" {
		return "default"
	}

	return source
}

func (m *RecorderManager) selectedSystemAudio() string {
	if m.ui.ComboSystemAudio == nil {
		return ""
	}

	return strings.TrimSpace(m.ui.ComboSystemAudio.GetActiveID())
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
		m.setStatus("Error: X11 DISPLAY is not available")
		return
	}

	fps := m.ui.ComboFPS.GetActiveID()
	if fps != "30" && fps != "60" {
		fps = "30"
	}

	res := m.ui.ComboRes.GetActiveID()
	if res == "" || res == "auto" {
		res, err = getNativeResolution()
		if err != nil {
			m.setStatus("Error: Cannot detect desktop resolution")
			return
		}
	}

	validResolution, _ := regexp.MatchString(
		`^[1-9][0-9]{1,4}x[1-9][0-9]{1,4}$`,
		res,
	)
	if !validResolution {
		m.setStatus("Error: Invalid resolution")
		return
	}

	if _, err := exec.LookPath("pactl"); err != nil {
		if m.ui.ChkAudio.GetActive() || m.ui.ChkSystemAudio.GetActive() {
			m.setStatus("Error: pactl is required for audio recording")
			return
		}
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	outputFile := filepath.Join(
		saveDir,
		fmt.Sprintf("Recording_%s_%d.mp4", timestamp, time.Now().UnixNano()),
	)

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

	audioInputs := make([]string, 0, 2)

	if m.ui.ChkAudio.GetActive() {
		audioInputs = append(audioInputs, m.selectedMicrophone())
	}

	if m.ui.ChkSystemAudio.GetActive() {
		systemSource := m.selectedSystemAudio()
		if systemSource == "" {
			m.setStatus("Error: Select a system audio source")
			return
		}
		audioInputs = append(audioInputs, systemSource)
	}

	for _, source := range audioInputs {
		args = append(args,
			"-thread_queue_size", "1024",
			"-f", "pulse",
			"-i", source,
		)
	}

	args = append(args, "-map", "0:v:0")

	for index := range audioInputs {
		inputIndex := index + 1
		args = append(args, "-map", fmt.Sprintf("%d:a:0", inputIndex))
	}

	args = append(args,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "20",
		"-pix_fmt", "yuv420p",
		"-r", fps,
	)

	if len(audioInputs) > 0 {
		args = append(args,
			"-c:a", "aac",
			"-b:a", "192k",
		)
	}

	args = append(args,
		"-movflags", "+faststart",
		outputFile,
	)

	cmd := exec.Command("ffmpeg", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		m.setStatus("Error: Cannot create FFmpeg input")
		return
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		m.setStatus("Error: Cannot start FFmpeg: " + err.Error())
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
	m.stopTicker = make(chan struct{})
	m.done = nil

	ticker := m.stopTicker

	glib.IdleAdd(func() {
		m.ui.BtnStart.SetLabel("Pause (Ctrl+R)")
		m.ui.BtnStop.SetSensitive(true)
		m.setControlsSensitive(false)
		m.ui.LblStatus.SetText("Status: Recording...")
	})

	go m.runTimer(ticker)
	go m.watchProcess(cmd, &stderr)
}

func (m *RecorderManager) setControlsSensitive(enabled bool) {
	m.ui.ChkAudio.SetSensitive(enabled)
	m.ui.ComboMic.SetSensitive(enabled && m.ui.ChkAudio.GetActive())

	m.ui.ChkSystemAudio.SetSensitive(enabled)
	m.ui.ComboSystemAudio.SetSensitive(
		enabled &&
			m.ui.ChkSystemAudio.GetActive() &&
			m.ui.ComboSystemAudio.GetActiveID() != "",
	)

	m.ui.ComboFPS.SetSensitive(enabled)
	m.ui.ComboRes.SetSensitive(enabled)
	m.ui.FileChooser.SetSensitive(enabled)
}

func (m *RecorderManager) watchProcess(cmd *exec.Cmd, stderr *bytes.Buffer) {
	// The process is waited for only by StopRecording.
	// This goroutine intentionally does not call Wait.
}

func (m *RecorderManager) pauseRecordingLocked() {
	if m.cmd == nil || m.cmd.Process == nil {
		return
	}

	if err := m.cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		m.setStatus("Error: Cannot pause recording: " + err.Error())
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

	if err := m.cmd.Process.Signal(syscall.SIGCONT); err != nil {
		m.setStatus("Error: Cannot resume recording: " + err.Error())
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

	if m.isPaused && m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Signal(syscall.SIGCONT)
		m.pausedDuration += time.Since(m.pauseStart)
		m.isPaused = false
	}

	cmd := m.cmd
	stdin := m.stdinPipe
	outputFile := m.outputFile
	ticker := m.stopTicker

	m.done = make(chan struct{})
	done := m.done

	if stdin != nil {
		_, writeErr := stdin.Write([]byte("q\n"))
		_ = stdin.Close()

		if writeErr != nil && cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGINT)
		}
	}

	m.mu.Unlock()

	if ticker != nil {
		close(ticker)
	}

	go func() {
		var waitErr error
		if cmd != nil {
			waitErr = cmd.Wait()
		}

		fileInfo, statErr := os.Stat(outputFile)

		success := waitErr == nil &&
			statErr == nil &&
			fileInfo.Size() > 0

		errorMessage := ""
		if waitErr != nil {
			errorMessage = waitErr.Error()
		}

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
			m.setControlsSensitive(true)

			if success {
				m.ui.LblStatus.SetText("Status: Saved successfully!")
			} else if errorMessage != "" {
				m.ui.LblStatus.SetText("Error: " + errorMessage)
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

func (m *RecorderManager) runTimer(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
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

			elapsed := time.Since(m.startTime) - m.pausedDuration
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
