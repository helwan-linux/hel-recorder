// recorder.go
package main

import (
	"bytes"
	"errors"
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
	stopTimeout = 10 * time.Second
)

type RecorderManager struct {
	ui *AppUI

	cmd       *exec.Cmd
	stdinPipe io.WriteCloser

	isRecording bool
	isPaused    bool
	stopping    bool
	finalizing  bool
	stopDone    chan struct{}

	mu sync.Mutex

	startTime      time.Time
	pausedDuration time.Duration
	pauseStart     time.Time

	stopTicker  chan struct{}
	processDone chan struct{}

	outputFile      string
	tempPartFiles   []string
	currentPartFile string
	stderr          *bytes.Buffer

	countdownCancel chan struct{}
	countdownActive bool

	magnifierCmd  *exec.Cmd
	screenkeyCmd  *exec.Cmd
	camPreviewCmd *exec.Cmd

	activeRes    string
	activeOffset string

	suppressNotifications bool
}

func NewRecorderManager(ui *AppUI) *RecorderManager {
	m := &RecorderManager{ui: ui}

	ui.BtnStart.Connect("clicked", m.StartRecording)
	ui.BtnPause.Connect("clicked", m.PauseResumeRecording)
	ui.BtnStop.Connect("clicked", m.StopRecording)

	return m
}

func (m *RecorderManager) updateWindowState(state string) {
	glib.IdleAdd(func() {
		switch state {
		case "recording":
			m.ui.Window.SetTitle("🔴 [REC] Helwan Screen Recorder")
		case "paused":
			m.ui.Window.SetTitle("⏸ [PAUSED] Helwan Screen Recorder")
		default:
			m.ui.Window.SetTitle("Helwan Screen Recorder")
		}
	})
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

func selectAreaWithSlop() (string, string, error) {
	slopPath, err := exec.LookPath("slop")
	if err != nil {
		return "", "", fmt.Errorf("'slop' is not installed (install with: sudo pacman -S slop)")
	}

	out, err := exec.Command(slopPath, "-f", "%wx%h+%x,%y").Output()
	if err != nil {
		return "", "", fmt.Errorf("area selection cancelled")
	}

	raw := strings.TrimSpace(string(out))
	parts := strings.SplitN(raw, "+", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid area format from slop")
	}

	dims := strings.SplitN(parts[0], "x", 2)
	if len(dims) != 2 {
		return "", "", fmt.Errorf("invalid dimensions from slop")
	}

	w, err1 := strconv.Atoi(dims[0])
	h, err2 := strconv.Atoi(dims[1])

	if err1 != nil || err2 != nil || w < 16 || h < 16 {
		return "", "", fmt.Errorf("selected area is too small")
	}

	if w%2 != 0 {
		w--
	}

	if h%2 != 0 {
		h--
	}

	return fmt.Sprintf("%dx%d", w, h), "+" + parts[1], nil
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

func isSourceMuted(source string) bool {
	target := source

	if target == "" || target == "default" {
		target = "@DEFAULT_SOURCE@"
	}

	out, err := exec.Command(
		"pactl",
		"get-source-mute",
		target,
	).Output()

	if err != nil {
		return false
	}

	return strings.Contains(
		strings.ToLower(string(out)),
		"yes",
	)
}

func setSourceMute(source string, mute bool) {
	target := source

	if target == "" || target == "default" {
		target = "@DEFAULT_SOURCE@"
	}

	value := "0"

	if mute {
		value = "1"
	}

	_ = exec.Command(
		"pactl",
		"set-source-mute",
		target,
		value,
	).Run()
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

	source := strings.TrimSpace(m.ui.ComboMic.GetActiveID())

	if source == "" {
		return "default"
	}

	return source
}

func (m *RecorderManager) selectedSystemAudio() string {
	if m.ui.ComboSystemAudio == nil {
		return ""
	}

	source := strings.TrimSpace(
		m.ui.ComboSystemAudio.GetActiveID(),
	)

	if source == "" {
		output, err := exec.Command(
			"pactl",
			"get-default-sink",
		).Output()

		if err == nil {
			sink := strings.TrimSpace(string(output))

			if sink != "" {
				return sink + ".monitor"
			}
		}

		return "alsa_output.pci-0000_00_1f.3.analog-stereo.monitor"
	}

	return source
}

func (m *RecorderManager) StartRecording() {
	m.mu.Lock()

	if m.stopping || m.finalizing || m.isRecording || m.countdownActive {
		m.mu.Unlock()
		return
	}

	if m.ui.ChkAudio != nil && m.ui.ChkAudio.GetActive() {
		micSource := m.selectedMicrophone()

		if isSourceMuted(micSource) {
			m.mu.Unlock()

			choice := showMicMutedDialog(m.ui.Window)

			if choice == "cancel" {
				m.setStatus("Status: Recording cancelled (mic muted)")
				return
			}

			if choice == "unmute" {
				setSourceMute(micSource, false)
			}

			m.mu.Lock()

			if m.stopping || m.finalizing || m.isRecording || m.countdownActive {
				m.mu.Unlock()
				return
			}
		}
	}

	resMode := m.ui.ComboRes.GetActiveID()

	if resMode == "area" {
		m.mu.Unlock()

		m.setStatus(
			"Status: Drag mouse to select recording area...",
		)

		areaRes, areaOffset, err := selectAreaWithSlop()

		if err != nil {
			m.setStatus("Error: " + err.Error())
			return
		}

		m.mu.Lock()

		if m.stopping || m.finalizing || m.isRecording || m.countdownActive {
			m.mu.Unlock()
			return
		}

		m.activeRes = areaRes
		m.activeOffset = areaOffset
	} else {
		nativeRes, err := getNativeResolution()

		if err != nil {
			m.mu.Unlock()
			m.setStatus("Error: Cannot detect desktop resolution")
			return
		}

		m.activeRes = nativeRes
		m.activeOffset = ""
	}

	if m.ui.ChkMinimize != nil &&
		m.ui.ChkMinimize.GetActive() {

		glib.IdleAdd(func() {
			m.ui.Window.Iconify()
		})
	}

	seconds := m.selectedCountdown()

	if seconds > 0 {
		m.startCountdownLocked(seconds)
		m.mu.Unlock()
		return
	}

	m.startRecordingLocked()
	m.mu.Unlock()
}

func (m *RecorderManager) PauseResumeRecording() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopping || !m.isRecording || m.countdownActive {
		return
	}

	if m.isPaused {
		if m.cmd != nil {
			return
		}

		m.resumeRecordingLocked()
		return
	}

	m.pauseRecordingLocked()
}

func (m *RecorderManager) resetFailedStartLocked() {
	if m.currentPartFile != "" {
		_ = os.Remove(m.currentPartFile)
	}

	m.cmd = nil
	m.stdinPipe = nil
	m.stderr = nil

	m.currentPartFile = ""
	m.outputFile = ""
	m.tempPartFiles = nil

	m.isRecording = false
	m.isPaused = false
	m.stopping = false

	m.activeRes = ""
	m.activeOffset = ""

	m.startTime = time.Time{}
	m.pausedDuration = 0
	m.pauseStart = time.Time{}
}

func (m *RecorderManager) startRecordingLocked() {
	saveDir := strings.TrimSpace(m.ui.FileChooser.GetFilename())

	if saveDir == "" {
		m.setStatus("Error: Select a save directory")
		m.restoreWindow()
		return
	}

	info, err := os.Stat(saveDir)
	if err != nil || !info.IsDir() {
		m.setStatus("Error: Invalid save directory")
		m.restoreWindow()
		return
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		m.setStatus("Error: FFmpeg is not installed")
		m.restoreWindow()
		return
	}

	display := strings.TrimSpace(os.Getenv("DISPLAY"))
	if display == "" {
		m.setStatus("Error: X11 DISPLAY is not available")
		m.restoreWindow()
		return
	}

	fps := m.ui.ComboFPS.GetActiveID()
	if fps != "30" && fps != "60" {
		fps = "30"
	}

	formatExt := "mp4"
	if m.ui.ComboFormat != nil {
		selectedFormat := strings.ToLower(strings.TrimSpace(m.ui.ComboFormat.GetActiveID()))
		if selectedFormat != "" {
			formatExt = selectedFormat
		}
	}

	if !isSupportedVideoFormat(formatExt) {
		m.setStatus("Error: Unsupported recording format")
		m.restoreWindow()
		return
	}

	res := m.activeRes
	if res == "" {
		res, err = getNativeResolution()
		if err != nil {
			m.setStatus("Error: Cannot detect desktop resolution")
			m.restoreWindow()
			return
		}
	}

	validResolution, _ := regexp.MatchString(
		`^[1-9][0-9]{1,4}x[1-9][0-9]{1,4}$`,
		res,
	)
	if !validResolution {
		m.setStatus("Error: Invalid resolution")
		m.restoreWindow()
		return
	}

	recordMic := m.ui.ChkAudio != nil && m.ui.ChkAudio.GetActive()
	recordSystem := m.ui.ChkSystemAudio != nil && m.ui.ChkSystemAudio.GetActive()

	if (recordMic || recordSystem) && formatExt != "gif" {
		if _, err := exec.LookPath("pactl"); err != nil {
			m.setStatus("Error: pactl is required for audio recording")
			m.restoreWindow()
			return
		}
	}

	audioInputs := make([]string, 0, 2)
	if formatExt != "gif" {
		if recordMic {
			audioInputs = append(audioInputs, m.selectedMicrophone())
		}
		if recordSystem {
			if source := m.selectedSystemAudio(); source != "" {
				audioInputs = append(audioInputs, source)
			}
		}
	}

	useWebcam := m.ui.ChkWebcam != nil &&
		m.ui.ChkWebcam.GetActive() &&
		hasWebcamDevice() &&
		formatExt != "gif"

	useScreenkey := m.ui.ChkScreenkey != nil && m.ui.ChkScreenkey.GetActive()

	camSize := "320x240"
	camPos := "bottom-right"
	webcamDevice := ""

	if useWebcam {
		if m.ui.ComboCamSize != nil {
			if selectedSize := strings.TrimSpace(m.ui.ComboCamSize.GetActiveID()); selectedSize != "" {
				camSize = selectedSize
			}
		}

		if m.ui.ComboCamPos != nil {
			if selectedPos := strings.TrimSpace(m.ui.ComboCamPos.GetActiveID()); selectedPos != "" {
				camPos = selectedPos
			}
		}

		webcamDevice = findWebcamDevice()
		if webcamDevice == "" {
			useWebcam = false
		} else {
			m.startWebcamPreviewLocked(camSize, camPos)
		}
	}

	if useScreenkey {
		m.startScreenkeyLocked()
	}

	timestamp := time.Now().Format("2006-01-02_15-04-05")

	if m.outputFile == "" {
		m.outputFile = filepath.Join(
			saveDir,
			fmt.Sprintf(
				"Recording_%s_%d.%s",
				timestamp,
				time.Now().UnixNano(),
				formatExt,
			),
		)
		m.tempPartFiles = nil
	}

	partExt := formatExt
	if formatExt == "gif" {
		// Record GIF sessions into a reliable intermediate video container.
		// GIF is generated only after recording stops.
		partExt = "mkv"
	}

	partFile := generatePartFileName(saveDir, partExt)
	m.currentPartFile = partFile

	args := []string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
	}

	vaapiDevice := ""
	if formatExt == "mp4" && !useWebcam {
		vaapiDevice = detectVAAPI()
	}

	if vaapiDevice != "" {
		args = append(args, "-vaapi_device", vaapiDevice)
	}

	grabInput := display + m.activeOffset

	args = append(
		args,
		"-f", "x11grab",
		"-framerate", fps,
		"-video_size", res,
		"-i", grabInput,
	)

	nextInputIdx := 1
	webcamInputIdx := -1

	if useWebcam {
		webcamInputIdx = nextInputIdx
		args = append(
			args,
			"-f", "v4l2",
			"-framerate", "30",
			"-video_size", camSize,
			"-i", webcamDevice,
		)
		nextInputIdx++
	}

	audioInputIndices := make([]int, 0, 2)

	for _, source := range audioInputs {
		args = append(
			args,
			"-f", "pulse",
			"-i", source,
		)

		audioInputIndices = append(audioInputIndices, nextInputIdx)
		nextInputIdx++
	}

	filterParts := make([]string, 0, 2)
	videoMap := "0:v:0"

	if useWebcam {
		filterParts = append(
			filterParts,
			fmt.Sprintf(
				"[0:v:0][%d:v:0]overlay=%s:shortest=1[vout]",
				webcamInputIdx,
				webcamOverlayPosition(camPos),
			),
		)
		videoMap = "[vout]"
	}

	audioMap := ""

	switch len(audioInputIndices) {
	case 1:
		audioMap = fmt.Sprintf("%d:a:0", audioInputIndices[0])

	case 2:
		filterParts = append(
			filterParts,
			fmt.Sprintf(
				"[%d:a:0][%d:a:0]amix=inputs=2:duration=longest:dropout_transition=2[aout]",
				audioInputIndices[0],
				audioInputIndices[1],
			),
		)
		audioMap = "[aout]"
	}

	if len(filterParts) > 0 {
		args = append(
			args,
			"-filter_complex",
			strings.Join(filterParts, ";"),
		)
	}

	args = append(args, "-map", videoMap)

	if audioMap != "" {
		args = append(args, "-map", audioMap)
	}

	args = append(
		args,
		getFormatSettings(
			formatExt,
			fps,
			res,
			vaapiDevice,
			len(audioInputIndices) > 0 && formatExt != "gif",
		)...,
	)

	args = append(args, partFile)

	cmd := exec.Command("ffmpeg", args...)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		m.stopScreenkeyLocked()
		m.stopWebcamPreviewLocked()
		m.resetFailedStartLocked()
		m.setStatus("Error: Cannot create FFmpeg input")
		m.restoreWindow()
		return
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		m.stopScreenkeyLocked()
		m.stopWebcamPreviewLocked()
		m.resetFailedStartLocked()
		m.setStatus("Error: Cannot start FFmpeg: " + err.Error())
		m.restoreWindow()
		return
	}

	m.cmd = cmd
	m.stdinPipe = stdin
	m.stderr = stderr

	m.isRecording = true
	m.isPaused = false
	m.stopping = false
	m.suppressNotifications = false

	if m.startTime.IsZero() {
		m.startTime = time.Now()
		m.pausedDuration = 0
	}

	m.stopTicker = make(chan struct{})
	m.processDone = make(chan struct{})

	ticker := m.stopTicker
	mode := "CPU"

	if vaapiDevice != "" {
		mode = "VAAPI"
	}

	if useWebcam {
		mode += " + Webcam PiP"
	}

	m.updateWindowState("recording")
	m.showFloatingOSD()

	glib.IdleAdd(func() {
		m.ui.BtnStart.SetSensitive(false)
		m.ui.BtnPause.SetSensitive(true)
		m.ui.BtnPause.SetLabel("Pause (Ctrl+E)")
		m.ui.BtnStop.SetSensitive(true)
		m.setControlsSensitive(false)
		m.ui.LblStatus.SetText("Status: Recording (" + mode + ")")
	})

	go m.runTimer(ticker)
	go m.watchProcess(cmd, stderr)
}

func (m *RecorderManager) restoreWindow() {
	glib.IdleAdd(func() {
		hideFloatingOSD()
		m.ui.Window.Deiconify()
		m.ui.Window.Present()
	})
}

func (m *RecorderManager) setControlsSensitive(
	enabled bool,
) {
	m.ui.ChkAudio.SetSensitive(enabled)

	m.ui.ComboMic.SetSensitive(
		enabled &&
			m.ui.ChkAudio.GetActive(),
	)

	m.ui.ChkSystemAudio.SetSensitive(enabled)

	m.ui.ComboSystemAudio.SetSensitive(
		enabled &&
			m.ui.ChkSystemAudio.GetActive(),
	)

	m.ui.ComboFPS.SetSensitive(enabled)
	m.ui.ComboRes.SetSensitive(enabled)
	m.ui.ComboFormat.SetSensitive(enabled)
	m.ui.ComboCountdown.SetSensitive(enabled)

	m.ui.ChkMinimize.SetSensitive(enabled)
	m.ui.ChkScreenkey.SetSensitive(enabled)

	if hasWebcamDevice() {
		m.ui.ChkWebcam.SetSensitive(enabled)

		camActive :=
			enabled &&
				m.ui.ChkWebcam.GetActive()

		m.ui.ComboCamPos.SetSensitive(camActive)
		m.ui.ComboCamSize.SetSensitive(camActive)
	}

	m.ui.FileChooser.SetSensitive(enabled)
}

func (m *RecorderManager) watchProcess(
	cmd *exec.Cmd,
	stderr *bytes.Buffer,
) {
	waitErr := cmd.Wait()

	stderrText := strings.TrimSpace(
		stderr.String(),
	)

	m.mu.Lock()

	stopping := m.stopping
	isPaused := m.isPaused

	currentPart := m.currentPartFile
	ticker := m.stopTicker
	done := m.processDone

	if m.cmd == cmd {
		m.cmd = nil
		m.stdinPipe = nil
		m.stopTicker = nil
	}

	m.mu.Unlock()

	if ticker != nil {
		close(ticker)
	}

	if currentPart != "" {
		if info, err := os.Stat(currentPart); err == nil &&
			info.Size() > 0 {

			m.mu.Lock()

			alreadyAdded := false

			for _, p := range m.tempPartFiles {
				if p == currentPart {
					alreadyAdded = true
					break
				}
			}

			if !alreadyAdded {
				m.tempPartFiles = append(
					m.tempPartFiles,
					currentPart,
				)
			}

			if m.currentPartFile == currentPart {
				m.currentPartFile = ""
			}

			m.mu.Unlock()
		}
	}

	if stopping {
		// StopRecording owns finalization and waits for this process.
		if done != nil {
			close(done)
		}
		return
	}

	if isPaused {
		glib.IdleAdd(func() {
			m.ui.BtnPause.SetSensitive(true)
			m.ui.BtnPause.SetLabel("Resume (Ctrl+E)")
			m.ui.LblStatus.SetText("Status: Paused")
		})

		m.updateWindowState("paused")

		if done != nil {
			close(done)
		}

		return
	}

	if waitErr != nil {
		m.mu.Lock()

		m.stopScreenkeyLocked()
		m.stopWebcamPreviewLocked()

		m.isRecording = false
		m.isPaused = false
		m.stopping = false

		m.mu.Unlock()

		hideFloatingOSD()
		m.updateWindowState("ready")
		m.restoreWindow()

		glib.IdleAdd(func() {
			m.ui.BtnStart.SetSensitive(true)
			m.ui.BtnPause.SetSensitive(false)
			m.ui.BtnStop.SetSensitive(false)

			m.setControlsSensitive(true)

			if stderrText == "" {
				stderrText = "FFmpeg exited unexpectedly"
			}

			m.ui.LblStatus.SetText(
				"Error: Recording interrupted: " +
					stderrText,
			)
		})
	}

	if done != nil {
		close(done)
	}
}

// moveFile moves a completed recording to its final destination.
// os.Rename cannot move files between different filesystems (for example
// /tmp and ~/Videos), so this function falls back to a safe copy-and-rename.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	dir := filepath.Dir(dst)
	base := filepath.Base(dst)
	tmp, err := os.CreateTemp(dir, "."+base+".part-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	srcFile, err := os.Open(src)
	if err != nil {
		_ = tmp.Close()
		return err
	}

	_, copyErr := io.Copy(tmp, srcFile)
	closeSrcErr := srcFile.Close()
	if copyErr != nil {
		_ = tmp.Close()
		return copyErr
	}
	if closeSrcErr != nil {
		_ = tmp.Close()
		return closeSrcErr
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		return err
	}

	return os.Remove(src)
}

func (m *RecorderManager) finalizeRecording() {
	m.mu.Lock()

	parts := append(
		[]string(nil),
		m.tempPartFiles...,
	)

	outputFile := m.outputFile

	suppressNotification :=
		m.suppressNotifications

	m.stopScreenkeyLocked()
	m.stopWebcamPreviewLocked()

	hideFloatingOSD()

	m.isRecording = false
	m.isPaused = false
	m.stopping = false

	m.outputFile = ""
	m.tempPartFiles = nil
	m.currentPartFile = ""

	m.activeRes = ""
	m.activeOffset = ""

	m.startTime = time.Time{}
	m.pausedDuration = 0
	m.pauseStart = time.Time{}

	m.cmd = nil
	m.stdinPipe = nil
	m.stderr = nil

	m.mu.Unlock()

	m.updateWindowState("ready")
	m.restoreWindow()

	if len(parts) == 0 {
		glib.IdleAdd(func() {
			m.ui.BtnStart.SetSensitive(true)
			m.ui.BtnPause.SetSensitive(false)
			m.ui.BtnStop.SetSensitive(false)

			m.setControlsSensitive(true)

			m.ui.LblStatus.SetText(
				"Error: No recorded video parts found",
			)
		})

		return
	}

	if len(parts) == 1 {
		err := moveFile(parts[0], outputFile)

		if err == nil {
			err = verifyOutput(outputFile)
		}

		success := err == nil

		if !success {
			_ = os.Remove(outputFile)
			_ = os.Remove(parts[0])
		}

		glib.IdleAdd(func() {
			m.ui.BtnStart.SetSensitive(true)
			m.ui.BtnPause.SetSensitive(false)
			m.ui.BtnPause.SetLabel("Pause (Ctrl+E)")
			m.ui.BtnStop.SetSensitive(false)

			m.setControlsSensitive(true)

			m.ui.LblTimer.SetMarkup(
				"<span font='28' weight='bold' color='#3584e4'>00:00:00</span>",
			)

			if success {
				m.ui.LblStatus.SetText(
					"Status: Video saved successfully!",
				)

				if !suppressNotification {
					showRecordingResult(
						m.ui.Window,
						"success",
						outputFile,
						"",
					)
				}
			} else {
				message := "Output file could not be saved"

				if err != nil {
					message = err.Error()
				}

				m.ui.LblStatus.SetText(
					"Error: " + message,
				)

				if !suppressNotification {
					showRecordingResult(
						m.ui.Window,
						"error",
						outputFile,
						message,
					)
				}
			}
		})

		return
	}

	m.setStatus("Status: Merging recorded parts...")

	mergeErr := mergeRecordingParts(parts, outputFile)

	for _, p := range parts {
		_ = os.Remove(p)
	}

	if mergeErr == nil {
		mergeErr = verifyOutput(outputFile)
	}

	success := mergeErr == nil

	if !success {
		_ = os.Remove(outputFile)
	}

	glib.IdleAdd(func() {
		m.ui.BtnStart.SetSensitive(true)
		m.ui.BtnPause.SetSensitive(false)
		m.ui.BtnPause.SetLabel("Pause (Ctrl+E)")
		m.ui.BtnStop.SetSensitive(false)

		m.setControlsSensitive(true)

		m.ui.LblTimer.SetMarkup(
			"<span font='28' weight='bold' color='#3584e4'>00:00:00</span>",
		)

		if success {
			m.ui.LblStatus.SetText("Status: Video saved successfully!")

			if !suppressNotification {
				showRecordingResult(
					m.ui.Window,
					"success",
					outputFile,
					"",
				)
			}

			return
		}

		message := "Merge failed"
		if mergeErr != nil {
			message = mergeErr.Error()
		}

		m.ui.LblStatus.SetText("Error: Video could not be saved")

		if !suppressNotification {
			showRecordingResult(
				m.ui.Window,
				"error",
				outputFile,
				message,
			)
		}
	})
}

func (m *RecorderManager) pauseRecordingLocked() {
	if m.stdinPipe == nil || m.cmd == nil {
		return
	}

	if m.ui.ChkAudio != nil &&
		m.ui.ChkAudio.GetActive() {

		setSourceMute(
			m.selectedMicrophone(),
			true,
		)
	}

	if m.ui.ChkSystemAudio != nil &&
		m.ui.ChkSystemAudio.GetActive() {

		sysSource := m.selectedSystemAudio()

		if sysSource != "" {
			setSourceMute(
				sysSource,
				true,
			)
		}
	}

	m.isPaused = true
	m.pauseStart = time.Now()

	_, _ = m.stdinPipe.Write([]byte("q\n"))
	_ = m.stdinPipe.Close()

	m.updateWindowState("paused")

	glib.IdleAdd(func() {
		m.ui.BtnPause.SetSensitive(false)
		m.ui.BtnPause.SetLabel("Resume (Ctrl+E)")
		m.ui.LblStatus.SetText(
			"Status: Pausing...",
		)
	})
}

func (m *RecorderManager) resumeRecordingLocked() {
	if m.cmd != nil {
		return
	}

	m.isPaused = false

	if !m.pauseStart.IsZero() {
		m.pausedDuration += time.Since(
			m.pauseStart,
		)

		m.pauseStart = time.Time{}
	}

	if m.ui.ChkAudio != nil &&
		m.ui.ChkAudio.GetActive() {

		setSourceMute(
			m.selectedMicrophone(),
			false,
		)
	}

	if m.ui.ChkSystemAudio != nil &&
		m.ui.ChkSystemAudio.GetActive() {

		sysSource := m.selectedSystemAudio()

		if sysSource != "" {
			setSourceMute(
				sysSource,
				false,
			)
		}
	}

	glib.IdleAdd(func() {
		m.ui.BtnPause.SetSensitive(false)
		m.ui.BtnPause.SetLabel("Pause (Ctrl+E)")
		m.ui.LblStatus.SetText(
			"Status: Recording...",
		)
	})

	go func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		if m.stopping ||
			!m.isRecording ||
			m.cmd != nil {

			return
		}

		m.startRecordingLocked()
	}()
}

func (m *RecorderManager) StopRecording() {
	m.mu.Lock()

	if m.countdownActive {
		m.cancelCountdownLocked()
		m.restoreWindow()
		m.mu.Unlock()
		return
	}

	if m.stopping || m.finalizing {
		m.mu.Unlock()
		return
	}
	if !m.isRecording && !m.isPaused {
		m.mu.Unlock()
		return
	}

	m.stopping = true
	m.finalizing = true
	m.stopDone = make(chan struct{})
	m.isPaused = false

	cmd := m.cmd
	stdin := m.stdinPipe
	processDone := m.processDone
	done := m.stopDone
	m.mu.Unlock()

	m.setStatus("Status: Stopping and saving...")

	if stdin != nil {
		_, _ = stdin.Write([]byte("q\n"))
		_ = stdin.Close()
	} else if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGINT)
	}

	go func() {
		if processDone != nil {
			select {
			case <-processDone:
			case <-time.After(stopTimeout):
				if cmd != nil && cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				<-processDone
			}
		}

		m.finalizeRecording()

		m.mu.Lock()
		if m.stopDone == done {
			m.stopDone = nil
		}
		m.finalizing = false
		m.mu.Unlock()
		close(done)
	}()
}

// WaitForStop blocks until the recording process and output finalization finish.
func (m *RecorderManager) WaitForStop() {
	m.mu.Lock()
	m.suppressNotifications = true
	if m.countdownActive {
		m.cancelCountdownLocked()
	}
	recording := m.isRecording || m.isPaused
	m.mu.Unlock()

	if recording {
		m.StopRecording()
	}

	m.mu.Lock()
	done := m.stopDone
	m.mu.Unlock()
	if done != nil {
		<-done
	}

	m.mu.Lock()
	m.stopScreenkeyLocked()
	m.stopWebcamPreviewLocked()
	m.mu.Unlock()
	hideFloatingOSD()
}

func (m *RecorderManager) runTimer(
	stop <-chan struct{},
) {
	ticker := time.NewTicker(
		time.Second,
	)

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

			isPaused := m.isPaused

			elapsed :=
				time.Since(m.startTime) -
					m.pausedDuration

			m.mu.Unlock()

			if elapsed < 0 {
				elapsed = 0
			}

			hours := int(
				elapsed.Hours(),
			)

			minutes := int(
				elapsed.Minutes(),
			) % 60

			seconds := int(
				elapsed.Seconds(),
			) % 60

			timeStr := fmt.Sprintf(
				"%02d:%02d:%02d",
				hours,
				minutes,
				seconds,
			)

			if isPaused {
				updateFloatingOSDText(
					timeStr,
					true,
				)

				continue
			}

			markupStr := fmt.Sprintf(
				"<span font='28' weight='bold' color='#e01b24'>%s</span>",
				timeStr,
			)

			updateFloatingOSDText(
				timeStr,
				false,
			)

			glib.IdleAdd(func() {
				m.ui.LblTimer.SetMarkup(
					markupStr,
				)
			})
		}
	}
}
