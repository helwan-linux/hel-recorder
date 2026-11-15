
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gotk3/gotk3/glib"
)

const (
	inputQueueSize = "64"
	stopTimeout    = 10 * time.Second
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

	stopTicker  chan struct{}
	processDone chan struct{}

	outputFile string
	stderr     *bytes.Buffer

	countdownCancel chan struct{}
	countdownActive bool

	magnifierCmd *exec.Cmd

	suppressNotifications bool
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
		if len(fields) >= 2 {
			sources = append(sources, fields[1])
		}
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

func detectVAAPI() string {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return ""
	}

	devices, err := filepath.Glob("/dev/dri/renderD*")
	if err != nil {
		return ""
	}

	for _, device := range devices {
		if _, err := os.Stat(device); err != nil {
			continue
		}

		args := []string{
			"-hide_banner",
			"-loglevel", "error",
			"-vaapi_device", device,
			"-f", "lavfi",
			"-i", "color=c=black:s=128x128:d=0.1",
			"-vf", "format=nv12,hwupload",
			"-frames:v", "1",
			"-c:v", "h264_vaapi",
			"-qp", "20",
			"-f", "null",
			"-",
		}

		if exec.Command(ffmpeg, args...).Run() == nil {
			return device
		}
	}

	return ""
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

	if m.countdownActive {
		m.cancelCountdownLocked()
		return
	}

	if !m.isRecording {
		seconds := m.selectedCountdown()

		if seconds > 0 {
			m.startCountdownLocked(seconds)
			return
		}

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

	recordMic := m.ui.ChkAudio.GetActive()
	recordSystem := m.ui.ChkSystemAudio.GetActive()

	if recordMic || recordSystem {
		if _, err := exec.LookPath("pactl"); err != nil {
			m.setStatus("Error: pactl is required for audio recording")
			return
		}
	}

	audioInputs := make([]string, 0, 2)

	if recordMic {
		audioInputs = append(audioInputs, m.selectedMicrophone())
	}

	if recordSystem {
		source := m.selectedSystemAudio()
		if source == "" {
			m.setStatus("Error: Select a system audio source")
			return
		}
		audioInputs = append(audioInputs, source)
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
	}

	vaapiDevice := detectVAAPI()

	if vaapiDevice != "" {
		args = append(args, "-vaapi_device", vaapiDevice)
	}

	args = append(args,
		"-thread_queue_size", inputQueueSize,
		"-f", "x11grab",
		"-framerate", fps,
		"-video_size", res,
		"-i", display,
	)

	for _, source := range audioInputs {
		args = append(args,
			"-thread_queue_size", inputQueueSize,
			"-f", "pulse",
			"-i", source,
		)
	}

	args = append(args, "-map", "0:v:0")

	for index := range audioInputs {
		args = append(args, "-map", fmt.Sprintf("%d:a:0", index+1))
	}

	if vaapiDevice != "" {
		args = append(args,
			"-vf", "format=nv12,hwupload",
			"-c:v", "h264_vaapi",
			"-qp", "20",
		)
	} else {
		args = append(args,
			"-c:v", "libx264",
			"-preset", "ultrafast",
			"-crf", "20",
			"-pix_fmt", "yuv420p",
		)
	}

	args = append(args, "-r", fps)

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

	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

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
	m.stderr = stderr

	m.isRecording = true
	m.isPaused = false
	m.stopping = false
	m.suppressNotifications = false

	m.startTime = time.Now()
	m.pausedDuration = 0

	m.stopTicker = make(chan struct{})
	m.processDone = make(chan struct{})

	ticker := m.stopTicker

	mode := "CPU"
	if vaapiDevice != "" {
		mode = "VAAPI"
	}

	glib.IdleAdd(func() {
		m.ui.BtnStart.SetLabel("Pause (Ctrl+R)")
		m.ui.BtnStop.SetSensitive(true)
		m.setControlsSensitive(false)
		m.ui.LblStatus.SetText("Status: Recording (" + mode + ")")
	})

	go m.runTimer(ticker)
	go m.watchProcess(cmd, stderr)
}

func (m *RecorderManager) setControlsSensitive(enabled bool) {
	m.ui.ChkAudio.SetSensitive(enabled)

	m.ui.ComboMic.SetSensitive(
		enabled && m.ui.ChkAudio.GetActive(),
	)

	m.ui.ChkSystemAudio.SetSensitive(enabled)

	m.ui.ComboSystemAudio.SetSensitive(
		enabled &&
			m.ui.ChkSystemAudio.GetActive() &&
			m.ui.ComboSystemAudio.GetActiveID() != "",
	)

	m.ui.ComboFPS.SetSensitive(enabled)
	m.ui.ComboRes.SetSensitive(enabled)
	m.ui.ComboCountdown.SetSensitive(enabled)
	m.ui.FileChooser.SetSensitive(enabled)
}

// verifyOutput checks the MP4 metadata and decodes its video stream.
func verifyOutput(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("output file is missing: %w", err)
	}

	if info.Size() == 0 {
		return fmt.Errorf("output file is empty")
	}

	ffprobe, err := exec.LookPath("ffprobe")
	if err == nil {
		output, probeErr := exec.Command(
			ffprobe,
			"-v", "error",
			"-show_entries", "stream=codec_type:format=duration",
			"-of", "default=noprint_wrappers=1",
			path,
		).CombinedOutput()

		if probeErr != nil {
			return fmt.Errorf("MP4 validation failed: %s", strings.TrimSpace(string(output)))
		}

		text := string(output)
		if !strings.Contains(text, "codec_type=video") {
			return fmt.Errorf("video stream was not found in the MP4 file")
		}

		durationValid := false

		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)

			if strings.HasPrefix(line, "duration=") {
				value := strings.TrimPrefix(line, "duration=")
				duration, parseErr := strconv.ParseFloat(value, 64)

				if parseErr == nil && duration > 0 {
					durationValid = true
				}
			}
		}

		if !durationValid {
			return fmt.Errorf("MP4 has no valid recording duration")
		}
	}

	// Decode the entire video stream to catch damaged or incomplete data.
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("FFmpeg is unavailable for video verification")
	}

	output, decodeErr := exec.Command(
		ffmpeg,
		"-v", "error",
		"-i", path,
		"-map", "0:v:0",
		"-f", "null",
		"-",
	).CombinedOutput()

	if decodeErr != nil {
		details := strings.TrimSpace(string(output))
		if details == "" {
			details = decodeErr.Error()
		}
		return fmt.Errorf("video decoding failed: %s", details)
	}

	return nil
}

func (m *RecorderManager) watchProcess(cmd *exec.Cmd, stderr *bytes.Buffer) {
	// Wait also waits for os/exec to finish copying stderr into the buffer.
	waitErr := cmd.Wait()

	stderrText := strings.TrimSpace(stderr.String())

	m.mu.Lock()

	outputFile := m.outputFile
	ticker := m.stopTicker
	done := m.processDone
	suppressNotification := m.suppressNotifications

	m.isRecording = false
	m.isPaused = false
	m.stopping = false

	m.cmd = nil
	m.stdinPipe = nil
	m.stopTicker = nil

	m.mu.Unlock()

	if ticker != nil {
		close(ticker)
	}

	var validationErr error

	if waitErr == nil {
		validationErr = verifyOutput(outputFile)
	} else {
		validationErr = waitErr

		if stderrText != "" {
			validationErr = fmt.Errorf("%w: %s", waitErr, stderrText)
		}
	}

	fileInfo, statErr := os.Stat(outputFile)
	fileExists := statErr == nil && fileInfo != nil && fileInfo.Size() > 0

	success := waitErr == nil && validationErr == nil

	errorMessage := ""
	if validationErr != nil {
		errorMessage = validationErr.Error()
	}

	glib.IdleAdd(func() {
		m.ui.BtnStart.SetLabel("Start (Ctrl+R)")
		m.ui.BtnStop.SetSensitive(false)
		m.setControlsSensitive(true)

		m.ui.LblTimer.SetMarkup(
			"<span font='28' weight='bold' color='#3584e4'>00:00:00</span>",
		)

		if success {
			m.ui.LblStatus.SetText("Status: Video saved successfully!")

			if !suppressNotification {
				showRecordingResult(m.ui.Window, "success", outputFile, "")
			}
		} else if fileExists {
			m.ui.LblStatus.SetText("Warning: Video may be incomplete")

			if !suppressNotification {
				showRecordingResult(m.ui.Window, "warning", outputFile, errorMessage)
			}
		} else {
			m.ui.LblStatus.SetText("Error: Video could not be saved")

			if !suppressNotification {
				showRecordingResult(m.ui.Window, "error", outputFile, errorMessage)
			}
		}
	})

	if done != nil {
		close(done)
	}
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

	if m.countdownActive {
		m.cancelCountdownLocked()
		m.mu.Unlock()
		return
	}

	if !m.isRecording || m.stopping {
		m.mu.Unlock()
		return
	}

	m.stopping = true

	cmd := m.cmd
	stdin := m.stdinPipe
	done := m.processDone

	if m.isPaused && cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGCONT)
		m.pausedDuration += time.Since(m.pauseStart)
		m.isPaused = false
	}

	m.mu.Unlock()

	m.setStatus("Status: Stopping and saving...")

	if stdin != nil {
		_, err := stdin.Write([]byte("q\n"))
		_ = stdin.Close()

		if err != nil && cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGINT)
		}
	} else if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGINT)
	}

	if cmd != nil && cmd.Process != nil && done != nil {
		go func() {
			select {
			case <-done:
				return
			case <-time.After(stopTimeout):
				_ = cmd.Process.Kill()
			}
		}()
	}
}

func (m *RecorderManager) WaitForStop() {
	m.mu.Lock()

	m.suppressNotifications = true

	if m.countdownActive {
		m.cancelCountdownLocked()
	}

	recording := m.isRecording
	done := m.processDone

	m.mu.Unlock()

	if recording {
		m.StopRecording()
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

			if elapsed < 0 {
				elapsed = 0
			}

			hours := int(elapsed.Hours())
			minutes := int(elapsed.Minutes()) % 60
			seconds := int(elapsed.Seconds()) % 60

			timeStr := fmt.Sprintf(
				"<span font='28' weight='bold' color='#e01b24'>%02d:%02d:%02d</span>",
				hours, minutes, seconds,
			)

			glib.IdleAdd(func() {
				m.ui.LblTimer.SetMarkup(timeStr)
			})
		}
	}
}
