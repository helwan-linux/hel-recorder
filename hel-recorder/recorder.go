// recorder.go
package main

import (
	"errors"
	"fmt"
	"io"
	"log"
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

// Set HSR_DEBUG=1 to print the exact FFmpeg command line and live statistics
// (fps, speed, dropped frames) to the terminal. Set HSR_FORCE_CPU=1 to skip
// hardware encoder detection.
var debugEnabled = os.Getenv("HSR_DEBUG") != ""

func debugf(format string, a ...interface{}) {
	if debugEnabled {
		log.Printf("[hsr] "+format, a...)
	}
}

func ffmpegLogLevel() string {
	if debugEnabled {
		return "info"
	}
	return "warning"
}

// tailBuffer keeps only the last max bytes written to it. FFmpeg can emit
// warnings continuously during a long recording; an unbounded buffer would
// slowly grow in memory.
type tailBuffer struct {
	mu   sync.Mutex
	max  int
	data []byte
}

func newTailBuffer(max int) *tailBuffer {
	return &tailBuffer{max: max}
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.data = append(t.data, p...)

	if len(t.data) > t.max {
		t.data = append([]byte(nil), t.data[len(t.data)-t.max:]...)
	}

	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	return string(t.data)
}

// lastLine returns the last non-empty line of text.
func lastLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")

	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}

	return ""
}

// makeEvenResolution rounds both dimensions down to even numbers, which
// yuv420p/H.264 encoders require.
func makeEvenResolution(res string) string {
	parts := strings.SplitN(res, "x", 2)
	if len(parts) != 2 {
		return res
	}

	w, err1 := strconv.Atoi(parts[0])
	h, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return res
	}

	return fmt.Sprintf("%dx%d", w&^1, h&^1)
}

type RecorderManager struct {
	ui *AppUI

	cmd            *exec.Cmd
	stdinPipe      io.WriteCloser
	audioCmd       *exec.Cmd
	audioStdinPipe io.WriteCloser

	isRecording bool
	isPaused    bool
	stopping    bool
	finalizing  bool
	stopDone    chan struct{}

	mu sync.Mutex

	startTime      time.Time
	pausedDuration time.Duration
	pauseStart     time.Time

	stopTicker       chan struct{}
	processDone      chan struct{}
	audioProcessDone chan struct{}

	outputFile           string
	tempPartFiles        []string
	currentPartFile      string
	audioPartFiles       []string
	currentAudioPartFile string
	stderr               *tailBuffer
	audioStderr          *tailBuffer

	countdownCancel chan struct{}
	countdownActive bool

	magnifierCmd  *exec.Cmd
	screenkeyCmd  *exec.Cmd
	camPreviewCmd *exec.Cmd

	activeRes    string
	activeOffset string

	suppressNotifications bool

	// Auto-stop settings, captured whenever a recording part starts.
	recordDir      string
	autoStopAfter  time.Duration
	minFreeBytes   uint64
	autoStopReason string
}

func NewRecorderManager(ui *AppUI) *RecorderManager {
	m := &RecorderManager{ui: ui}

	// Probe VAAPI/NVENC in the background so the first recording (and every
	// resume after a pause) starts instantly.
	go getHWEncoder()

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

		trayApplyState(state)
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

// selectWindowWithSlop lets the user click a window and returns its geometry
// as (resolution, offset) in the same format as selectAreaWithSlop. The very
// large tolerance makes slop treat every click as "select the window under the
// cursor". The rectangle is clamped to the desktop so x11grab never receives a
// negative offset or an area that exceeds the screen.
func selectWindowWithSlop() (string, string, error) {
	slopPath, err := exec.LookPath("slop")
	if err != nil {
		return "", "", fmt.Errorf("'slop' is not installed (install with: sudo pacman -S slop)")
	}

	out, err := exec.Command(slopPath, "-t", "100000", "-f", "%x %y %w %h").Output()
	if err != nil {
		return "", "", fmt.Errorf("window selection cancelled")
	}

	fields := strings.Fields(string(out))
	if len(fields) != 4 {
		return "", "", fmt.Errorf("invalid window geometry from slop")
	}

	values := make([]int, 4)
	for i, field := range fields {
		n, convErr := strconv.Atoi(field)
		if convErr != nil {
			return "", "", fmt.Errorf("invalid window geometry from slop")
		}
		values[i] = n
	}

	x, y, w, h := values[0], values[1], values[2], values[3]

	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}

	if native, nativeErr := getNativeResolution(); nativeErr == nil {
		dims := strings.SplitN(native, "x", 2)
		if len(dims) == 2 {
			sw, e1 := strconv.Atoi(dims[0])
			sh, e2 := strconv.Atoi(dims[1])

			if e1 == nil && e2 == nil {
				if x+w > sw {
					w = sw - x
				}
				if y+h > sh {
					h = sh - y
				}
			}
		}
	}

	w &^= 1
	h &^= 1

	if w < 16 || h < 16 {
		return "", "", fmt.Errorf("selected window is too small or off-screen")
	}

	return fmt.Sprintf("%dx%d", w, h), fmt.Sprintf("+%d,%d", x, y), nil
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

	if resMode == "area" || resMode == "window" {
		m.mu.Unlock()

		var (
			areaRes    string
			areaOffset string
			err        error
		)

		if resMode == "window" {
			m.setStatus(
				"Status: Click the window you want to record...",
			)

			areaRes, areaOffset, err = selectWindowWithSlop()
		} else {
			m.setStatus(
				"Status: Drag mouse to select recording area...",
			)

			areaRes, areaOffset, err = selectAreaWithSlop()
		}

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
	} else if strings.HasPrefix(resMode, "monitor:") {
		mon, found := findMonitor(strings.TrimPrefix(resMode, "monitor:"))
		if !found {
			m.mu.Unlock()
			m.setStatus("Error: The selected monitor is not connected")
			return
		}

		m.activeRes = fmt.Sprintf("%dx%d", mon.W&^1, mon.H&^1)
		m.activeOffset = fmt.Sprintf("+%d,%d", mon.X, mon.Y)
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
	if m.currentAudioPartFile != "" {
		_ = os.Remove(m.currentAudioPartFile)
	}

	m.cmd = nil
	m.stdinPipe = nil
	m.audioCmd = nil
	m.audioStdinPipe = nil
	m.stderr = nil
	m.audioStderr = nil

	m.currentPartFile = ""
	m.currentAudioPartFile = ""
	m.outputFile = ""
	m.tempPartFiles = nil
	m.audioPartFiles = nil

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

	minFree := m.selectedMinFreeBytes()
	if minFree > 0 {
		if free, freeErr := freeDiskBytes(saveDir); freeErr == nil && free < minFree {
			m.setStatus(fmt.Sprintf(
				"Error: Not enough free disk space (%s free, %s required)",
				formatBytes(free),
				formatBytes(minFree),
			))
			m.restoreWindow()
			return
		}
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

	res = makeEvenResolution(res)

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
	micInputIdx, sysInputIdx := -1, -1

	if formatExt != "gif" {
		if recordMic {
			micInputIdx = len(audioInputs)
			audioInputs = append(audioInputs, m.selectedMicrophone())
		}
		if recordSystem {
			if source := m.selectedSystemAudio(); source != "" {
				sysInputIdx = len(audioInputs)
				audioInputs = append(audioInputs, source)
			}
		}
	}

	// Audio levels and processing, read here because this runs on the GTK
	// main thread.
	mixOptions := audioMixOptions{
		MicIndex:  micInputIdx,
		SysIndex:  sysInputIdx,
		MicVolume: parseVolumePercent(m.ui.ComboMicVol.GetActiveID()),
		SysVolume: parseVolumePercent(m.ui.ComboSysVol.GetActiveID()),
		Denoise: m.ui.ChkDenoise.GetActive() &&
			ffmpegHasFilter("afftdn"),
		Separate: m.ui.ChkSeparate.GetActive(),
	}

	drawMouse := "1"
	if m.ui.ChkCursor != nil && !m.ui.ChkCursor.GetActive() {
		drawMouse = "0"
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
		m.audioPartFiles = nil
		m.autoStopReason = ""
	}

	m.recordDir = saveDir
	m.autoStopAfter = m.selectedAutoStop()
	m.minFreeBytes = minFree
	quality := m.selectedQuality()

	// Video and audio are deliberately written by separate FFmpeg processes.
	// The video process never waits for PulseAudio or an audio filter/muxer.
	videoPartFile := generatePartFileName(saveDir, internalPartExt(formatExt))
	m.currentPartFile = videoPartFile

	var audioPartFile string
	if len(audioInputs) > 0 {
		audioPartFile = generatePartFileName(saveDir, "mka")
		m.currentAudioPartFile = audioPartFile
	}

	hw := getHWEncoder()
	if formatExt == "webm" {
		hw = hwEncoder{}
	}

	// ----------------------------
	// Video FFmpeg process
	// ----------------------------
	videoArgs := []string{
		"-y",
		"-hide_banner",
		"-loglevel", ffmpegLogLevel(),
	}

	if debugEnabled {
		videoArgs = append(videoArgs, "-stats")
	}

	videoArgs = append(videoArgs, hw.InitArgs()...)

	grabInput := display + m.activeOffset

	videoArgs = append(videoArgs, inputQueueArgs("1024")...)
	videoArgs = append(
		videoArgs,
		"-f", "x11grab",
		"-framerate", fps,
		"-draw_mouse", drawMouse,
		"-video_size", res,
		"-i", grabInput,
	)

	webcamInputIdx := -1
	if useWebcam {
		webcamInputIdx = 1

		videoArgs = append(videoArgs, inputQueueArgs("512")...)
		videoArgs = append(
			videoArgs,
			"-use_wallclock_as_timestamps", "1",
			"-f", "v4l2",
		)

		if webcamSupportsMJPEG(webcamDevice) {
			videoArgs = append(videoArgs, "-input_format", "mjpeg")
		}

		videoArgs = append(
			videoArgs,
			"-framerate", "30",
			"-video_size", camSize,
			"-i", webcamDevice,
		)
	}

	videoFilter := ""
	videoMap := "0:v:0"
	vfInGraph := false

	if useWebcam {
		videoFilter = fmt.Sprintf(
			"[0:v:0][%d:v:0]overlay=%s",
			webcamInputIdx,
			webcamOverlayPosition(camPos),
		)

		if hw.Kind == hwVAAPI {
			videoFilter += ",format=nv12,hwupload"
		}

		videoFilter += "[vout]"
		videoMap = "[vout]"
		vfInGraph = true
	}

	if videoFilter != "" {
		videoArgs = append(videoArgs, "-filter_complex", videoFilter)
	}

	videoArgs = append(videoArgs, "-map", videoMap)
	videoArgs = append(
		videoArgs,
		getFormatSettings(
			formatExt,
			fps,
			hw,
			false,
			vfInGraph,
			quality,
		)...,
	)
	videoArgs = append(videoArgs, videoPartFile)

	debugf("video ffmpeg %s", strings.Join(videoArgs, " "))

	videoCmd := exec.Command("ffmpeg", videoArgs...)
	videoStderr := newTailBuffer(16 * 1024)

	if debugEnabled {
		videoCmd.Stderr = io.MultiWriter(videoStderr, os.Stderr)
	} else {
		videoCmd.Stderr = videoStderr
	}

	videoStdin, err := videoCmd.StdinPipe()
	if err != nil {
		m.stopScreenkeyLocked()
		m.stopWebcamPreviewLocked()
		m.resetFailedStartLocked()
		m.setStatus("Error: Cannot create video FFmpeg input")
		m.restoreWindow()
		return
	}

	if err := videoCmd.Start(); err != nil {
		_ = videoStdin.Close()
		m.stopScreenkeyLocked()
		m.stopWebcamPreviewLocked()
		m.resetFailedStartLocked()
		m.setStatus("Error: Cannot start video FFmpeg: " + err.Error())
		m.restoreWindow()
		return
	}

	// ----------------------------
	// Audio FFmpeg process
	// ----------------------------
	var audioCmd *exec.Cmd
	var audioStdin io.WriteCloser
	var audioStderr *tailBuffer

	if len(audioInputs) > 0 {
		audioArgs := []string{
			"-y",
			"-hide_banner",
			"-loglevel", ffmpegLogLevel(),
		}

		if debugEnabled {
			audioArgs = append(audioArgs, "-stats")
		}

		for _, source := range audioInputs {
			audioArgs = append(audioArgs, inputQueueArgs("1024")...)
			audioArgs = append(
				audioArgs,
				"-use_wallclock_as_timestamps", "1",
				"-f", "pulse",
				"-i", source,
			)
		}

		audioFilter, audioMaps := buildAudioFilter(mixOptions)

		if audioFilter != "" {
			audioArgs = append(audioArgs, "-filter_complex", audioFilter)
		}

		for _, label := range audioMaps {
			audioArgs = append(audioArgs, "-map", label)
		}

		if len(audioMaps) == 2 {
			audioArgs = append(
				audioArgs,
				"-metadata:s:a:0", "title=Microphone",
				"-metadata:s:a:1", "title=System Audio",
			)
		}

		if formatExt == "webm" {
			audioArgs = append(
				audioArgs,
				"-c:a", "libopus",
				"-b:a", "192k",
				"-f", "webm",
			)
		} else {
			audioArgs = append(
				audioArgs,
				"-c:a", "aac",
				"-b:a", "192k",
				"-f", "matroska",
			)
		}

		audioArgs = append(audioArgs, audioPartFile)

		debugf("audio ffmpeg %s", strings.Join(audioArgs, " "))

		audioCmd = exec.Command("ffmpeg", audioArgs...)
		audioStderr = newTailBuffer(16 * 1024)

		if debugEnabled {
			audioCmd.Stderr = io.MultiWriter(audioStderr, os.Stderr)
		} else {
			audioCmd.Stderr = audioStderr
		}

		audioStdin, err = audioCmd.StdinPipe()
		if err != nil {
			_ = videoStdin.Close()
			_ = videoCmd.Process.Kill()
			_ = videoCmd.Wait()
			m.stopScreenkeyLocked()
			m.stopWebcamPreviewLocked()
			m.resetFailedStartLocked()
			m.setStatus("Error: Cannot create audio FFmpeg input")
			m.restoreWindow()
			return
		}

		if err := audioCmd.Start(); err != nil {
			_ = audioStdin.Close()
			_ = videoStdin.Close()
			_ = videoCmd.Process.Kill()
			_ = videoCmd.Wait()
			m.stopScreenkeyLocked()
			m.stopWebcamPreviewLocked()
			m.resetFailedStartLocked()
			m.setStatus("Error: Cannot start audio FFmpeg: " + err.Error())
			m.restoreWindow()
			return
		}
	}

	m.cmd = videoCmd
	m.stdinPipe = videoStdin
	m.stderr = videoStderr
	m.audioCmd = audioCmd
	m.audioStdinPipe = audioStdin
	m.audioStderr = audioStderr

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
	if audioCmd != nil {
		m.audioProcessDone = make(chan struct{})
	} else {
		m.audioProcessDone = nil
	}

	ticker := m.stopTicker
	mode := hw.Label()

	if useWebcam {
		mode += " + Webcam PiP"
	}
	if audioCmd != nil {
		mode += " + Separate Audio"
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
	go m.watchProcess(videoCmd, videoStderr)
	if audioCmd != nil {
		go m.watchAudioProcess(audioCmd, audioStderr)
	}
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

	// A fixed bitrate overrides the quality level, so that combo is disabled
	// while a bitrate is chosen.
	m.ui.ComboBitrate.SetSensitive(enabled)
	m.ui.ComboQuality.SetSensitive(
		enabled &&
			m.ui.ComboBitrate.GetActiveID() == "0",
	)
	m.ui.ComboAutoStop.SetSensitive(enabled)
	m.ui.ComboDiskGuard.SetSensitive(enabled)

	m.ui.ComboMicVol.SetSensitive(enabled)
	m.ui.ComboSysVol.SetSensitive(enabled)
	m.ui.ChkSeparate.SetSensitive(enabled)
	m.ui.ChkDenoise.SetSensitive(enabled && ffmpegHasFilter("afftdn"))
	m.ui.ChkCursor.SetSensitive(enabled)

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
	stderr *tailBuffer,
) {
	waitErr := cmd.Wait()
	stderrText := lastLine(stderr.String())

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
		if info, err := os.Stat(currentPart); err == nil && info.Size() > 0 {
			m.mu.Lock()

			alreadyAdded := false
			for _, p := range m.tempPartFiles {
				if p == currentPart {
					alreadyAdded = true
					break
				}
			}

			if !alreadyAdded {
				m.tempPartFiles = append(m.tempPartFiles, currentPart)
			}

			if m.currentPartFile == currentPart {
				m.currentPartFile = ""
			}

			m.mu.Unlock()
		}
	}

	if stopping {
		if done != nil {
			close(done)
		}
		return
	}

	if isPaused {
		m.mu.Lock()
		audioStillRunning := m.audioCmd != nil
		m.mu.Unlock()

		if !audioStillRunning {
			glib.IdleAdd(func() {
				m.ui.BtnPause.SetSensitive(true)
				m.ui.BtnPause.SetLabel("Resume (Ctrl+E)")
				m.ui.LblStatus.SetText("Status: Paused")
			})

			m.updateWindowState("paused")
		}

		if done != nil {
			close(done)
		}

		return
	}

	if waitErr != nil {
		// If video dies unexpectedly, stop the independent audio process too.
		m.mu.Lock()
		audioCmd := m.audioCmd
		audioStdin := m.audioStdinPipe
		m.isRecording = false
		m.isPaused = false
		m.stopping = false
		m.mu.Unlock()

		if audioStdin != nil {
			_, _ = audioStdin.Write([]byte("q\n"))
			_ = audioStdin.Close()
		} else if audioCmd != nil && audioCmd.Process != nil {
			_ = audioCmd.Process.Kill()
		}

		m.stopScreenkeyLocked()
		m.stopWebcamPreviewLocked()
		hideFloatingOSD()
		m.updateWindowState("ready")
		m.restoreWindow()

		glib.IdleAdd(func() {
			m.ui.BtnStart.SetSensitive(true)
			m.ui.BtnPause.SetSensitive(false)
			m.ui.BtnStop.SetSensitive(false)
			m.setControlsSensitive(true)

			if stderrText == "" {
				stderrText = "FFmpeg video process exited unexpectedly"
			}

			m.ui.LblStatus.SetText(
				"Error: Recording interrupted: " + stderrText,
			)
		})
	}

	if done != nil {
		close(done)
	}
}

func (m *RecorderManager) watchAudioProcess(
	cmd *exec.Cmd,
	stderr *tailBuffer,
) {
	waitErr := cmd.Wait()
	stderrText := lastLine(stderr.String())

	m.mu.Lock()

	stopping := m.stopping
	isPaused := m.isPaused
	currentPart := m.currentAudioPartFile
	done := m.audioProcessDone

	if m.audioCmd == cmd {
		m.audioCmd = nil
		m.audioStdinPipe = nil
	}

	m.mu.Unlock()

	if currentPart != "" {
		if info, err := os.Stat(currentPart); err == nil && info.Size() > 0 {
			m.mu.Lock()

			alreadyAdded := false
			for _, p := range m.audioPartFiles {
				if p == currentPart {
					alreadyAdded = true
					break
				}
			}

			if !alreadyAdded {
				m.audioPartFiles = append(m.audioPartFiles, currentPart)
			}

			if m.currentAudioPartFile == currentPart {
				m.currentAudioPartFile = ""
			}

			m.mu.Unlock()
		}
	}

	if stopping || isPaused {
		if isPaused && !stopping {
			m.mu.Lock()
			videoStillRunning := m.cmd != nil
			m.mu.Unlock()

			if !videoStillRunning {
				glib.IdleAdd(func() {
					m.ui.BtnPause.SetSensitive(true)
					m.ui.BtnPause.SetLabel("Resume (Ctrl+E)")
					m.ui.LblStatus.SetText("Status: Paused")
				})
				m.updateWindowState("paused")
			}
		}

		if done != nil {
			close(done)
		}
		return
	}

	if waitErr != nil {
		// Audio is independent: a failure here must not stall the screen.
		glib.IdleAdd(func() {
			if stderrText == "" {
				stderrText = "FFmpeg audio process exited unexpectedly"
			}
			m.ui.LblStatus.SetText(
				"Warning: Audio recording stopped: " + stderrText,
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

	videoParts := append([]string(nil), m.tempPartFiles...)
	audioParts := append([]string(nil), m.audioPartFiles...)
	outputFile := m.outputFile
	suppressNotification := m.suppressNotifications
	autoStopReason := m.autoStopReason
	m.autoStopReason = ""

	m.stopScreenkeyLocked()
	m.stopWebcamPreviewLocked()
	hideFloatingOSD()

	m.isRecording = false
	m.isPaused = false
	m.stopping = false

	m.outputFile = ""
	m.tempPartFiles = nil
	m.currentPartFile = ""
	m.audioPartFiles = nil
	m.currentAudioPartFile = ""

	m.activeRes = ""
	m.activeOffset = ""

	m.startTime = time.Time{}
	m.pausedDuration = 0
	m.pauseStart = time.Time{}

	m.cmd = nil
	m.stdinPipe = nil
	m.audioCmd = nil
	m.audioStdinPipe = nil
	m.stderr = nil
	m.audioStderr = nil

	m.mu.Unlock()

	m.updateWindowState("ready")
	m.restoreWindow()

	if len(videoParts) == 0 {
		glib.IdleAdd(func() {
			m.ui.BtnStart.SetSensitive(true)
			m.ui.BtnPause.SetSensitive(false)
			m.ui.BtnStop.SetSensitive(false)
			m.setControlsSensitive(true)
			m.ui.LblStatus.SetText("Error: No recorded video parts found")
		})
		return
	}

	m.setStatus("Status: Finalizing video and audio...")

	var err error

	if strings.EqualFold(filepath.Ext(outputFile), ".gif") {
		if len(videoParts) == 1 {
			err = finalizeSinglePart(videoParts[0], outputFile)
		} else {
			err = mergeRecordingParts(videoParts, outputFile)
		}
	} else {
		videoSource := videoParts[0]
		var tempVideo string

		if len(videoParts) > 1 {
			tempFile, createErr := os.CreateTemp(filepath.Dir(outputFile), ".hel_video_merge_*.mkv")
			if createErr != nil {
				err = createErr
			} else {
				tempVideo = tempFile.Name()
				_ = tempFile.Close()
				err = mergeVideoPartsToFile(videoParts, tempVideo)
				if err == nil {
					videoSource = tempVideo
				}
			}
		}

		if err == nil && len(audioParts) > 0 {
			audioSource := audioParts[0]
			var tempAudio string

			if len(audioParts) > 1 {
				tempFile, createErr := os.CreateTemp(filepath.Dir(outputFile), ".hel_audio_merge_*.mka")
				if createErr != nil {
					err = createErr
				} else {
					tempAudio = tempFile.Name()
					_ = tempFile.Close()
					err = mergeAudioPartsToFile(audioParts, tempAudio)
					if err == nil {
						audioSource = tempAudio
					}
				}
			}

			if err == nil {
				err = muxVideoAudio(videoSource, audioSource, outputFile)
			}

			if tempAudio != "" {
				_ = os.Remove(tempAudio)
			}
		} else if err == nil {
			err = finalizeSinglePart(videoSource, outputFile)
		}

		if tempVideo != "" {
			_ = os.Remove(tempVideo)
		}
	}

	if err == nil {
		err = verifyOutput(outputFile)
	}

	success := err == nil

	if success {
		for _, p := range videoParts {
			_ = os.Remove(p)
		}
		for _, p := range audioParts {
			_ = os.Remove(p)
		}
	} else {
		_ = os.Remove(outputFile)

		kept := append([]string(nil), videoParts...)
		kept = append(kept, audioParts...)

		if len(kept) > 0 {
			err = fmt.Errorf(
				"%w\nThe raw recording parts were kept in:\n%s",
				err,
				filepath.Dir(kept[0]),
			)
		}
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
					autoStopReason,
				)
			}
			return
		}

		message := "Video could not be saved"
		if err != nil {
			message = err.Error()
		}

		m.ui.LblStatus.SetText("Error: " + message)

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

	m.isPaused = true
	m.pauseStart = time.Now()

	_, _ = m.stdinPipe.Write([]byte("q\n"))
	_ = m.stdinPipe.Close()

	if m.audioStdinPipe != nil {
		_, _ = m.audioStdinPipe.Write([]byte("q\n"))
		_ = m.audioStdinPipe.Close()
	}

	m.updateWindowState("paused")

	glib.IdleAdd(func() {
		m.ui.BtnPause.SetSensitive(false)
		m.ui.BtnPause.SetLabel("Resume (Ctrl+E)")
		m.ui.LblStatus.SetText("Status: Pausing...")
	})
}

// resumeRecordingLocked starts a new recording part. It is always called from
// the GTK main thread (button or hotkey handler), which is required because
// startRecordingLocked reads GTK widgets; GTK is not thread-safe.
func (m *RecorderManager) resumeRecordingLocked() {
	if m.cmd != nil || m.audioCmd != nil {
		return
	}

	m.isPaused = false

	if !m.pauseStart.IsZero() {
		m.pausedDuration += time.Since(
			m.pauseStart,
		)

		m.pauseStart = time.Time{}
	}

	glib.IdleAdd(func() {
		m.ui.BtnPause.SetSensitive(false)
		m.ui.BtnPause.SetLabel("Pause (Ctrl+E)")
		m.ui.LblStatus.SetText(
			"Status: Recording...",
		)
	})

	m.startRecordingLocked()

	if m.cmd == nil {
		// The new part could not be started: go back to the paused state so
		// the user can retry or stop and save what was already recorded.
		m.isPaused = true
		m.pauseStart = time.Now()

		glib.IdleAdd(func() {
			m.ui.BtnPause.SetSensitive(true)
			m.ui.BtnPause.SetLabel("Resume (Ctrl+E)")
		})
	}
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

	videoCmd := m.cmd
	videoStdin := m.stdinPipe
	videoDone := m.processDone

	audioCmd := m.audioCmd
	audioStdin := m.audioStdinPipe
	audioDone := m.audioProcessDone

	done := m.stopDone
	m.mu.Unlock()

	m.setStatus("Status: Stopping and saving...")

	if videoStdin != nil {
		_, _ = videoStdin.Write([]byte("q\n"))
		_ = videoStdin.Close()
	} else if videoCmd != nil && videoCmd.Process != nil {
		_ = videoCmd.Process.Signal(syscall.SIGINT)
	}

	if audioStdin != nil {
		_, _ = audioStdin.Write([]byte("q\n"))
		_ = audioStdin.Close()
	} else if audioCmd != nil && audioCmd.Process != nil {
		_ = audioCmd.Process.Signal(syscall.SIGINT)
	}

	go func() {
		if videoDone != nil {
			select {
			case <-videoDone:
			case <-time.After(stopTimeout):
				if videoCmd != nil && videoCmd.Process != nil {
					_ = videoCmd.Process.Kill()
				}
				<-videoDone
			}
		}

		if audioDone != nil {
			select {
			case <-audioDone:
			case <-time.After(stopTimeout):
				if audioCmd != nil && audioCmd.Process != nil {
					_ = audioCmd.Process.Kill()
				}
				<-audioDone
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

func mergeAudioPartsToFile(parts []string, outputFile string) error {
	listName, err := writeConcatList(parts)
	if err != nil {
		return err
	}
	defer os.Remove(listName)

	return runFFmpeg([]string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-f", "concat",
		"-safe", "0",
		"-i", listName,
		"-map", "0",
		"-c", "copy",
		"-f", "matroska",
		outputFile,
	})
}

// audioSyncOffset returns the audio shift in seconds.
// Negative = audio earlier, positive = audio later.
// Set with the HSR_AUDIO_OFFSET_MS environment variable.
func audioSyncOffset() float64 {
	ms, err := strconv.Atoi(os.Getenv("HSR_AUDIO_OFFSET_MS"))
	if err != nil {
		return 0
	}
	return float64(ms) / 1000
}

func muxVideoAudio(videoFile, audioFile, outputFile string) error {
	args := []string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", videoFile,
	}

	off := audioSyncOffset()
	switch {
	case off < 0:
		args = append(args, "-ss", fmt.Sprintf("%.3f", -off))
	case off > 0:
		args = append(args, "-itsoffset", fmt.Sprintf("%.3f", off))
	}

	args = append(args,
		"-i", audioFile,
		"-map", "0:v:0",
		"-map", "1:a",
		"-c", "copy",
	)

	switch strings.ToLower(filepath.Ext(outputFile)) {
	case ".mp4":
		args = append(args, "-movflags", "+faststart", "-f", "mp4")
	case ".webm":
		args = append(args, "-f", "webm")
	default:
		args = append(args, "-f", "matroska")
	}

	return runFFmpeg(append(args, outputFile))
}

func (m *RecorderManager) runTimer(
	stop <-chan struct{},
) {
	ticker := time.NewTicker(
		time.Second,
	)

	defer ticker.Stop()

	var (
		tick      int
		triggered bool
	)

	for {
		select {
		case <-stop:
			return

		case <-ticker.C:
			tick++

			m.mu.Lock()

			if !m.isRecording {
				m.mu.Unlock()
				return
			}

			isPaused := m.isPaused

			elapsed :=
				time.Since(m.startTime) -
					m.pausedDuration

			limit := m.autoStopAfter
			minFree := m.minFreeBytes
			dir := m.recordDir

			// The disk guard is checked every third second; the list of files
			// is copied here and measured after the lock is released.
			var recordedFiles []string
			if minFree > 0 && !isPaused && tick%3 == 0 {
				recordedFiles = append(recordedFiles, m.tempPartFiles...)
				recordedFiles = append(recordedFiles, m.audioPartFiles...)
				recordedFiles = append(
					recordedFiles,
					m.currentPartFile,
					m.currentAudioPartFile,
				)
			}

			m.mu.Unlock()

			if elapsed < 0 {
				elapsed = 0
			}

			if !triggered && !isPaused {
				reason := ""

				if limit > 0 && elapsed >= limit {
					reason = "Recording stopped automatically: the time limit was reached."
				} else if recordedFiles != nil {
					reason = diskLowReason(dir, minFree, totalFileSize(recordedFiles))
				}

				if reason != "" {
					triggered = true

					m.mu.Lock()
					m.autoStopReason = reason
					m.mu.Unlock()

					// Same path as the Stop button / Ctrl+Q.
					glib.IdleAdd(func() {
						m.StopRecording()
					})
				}
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

			updateTrayTooltip(
				timeStr,
				isPaused,
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
