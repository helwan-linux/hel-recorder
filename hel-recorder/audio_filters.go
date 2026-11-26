// audio_filters.go
package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// ---------------------------------------------------------------------------
// Audio filter graph (pure logic: no GTK, easy to unit-test)
// ---------------------------------------------------------------------------

// audioMixOptions describes how the microphone and the system audio are
// processed by the audio FFmpeg process.
type audioMixOptions struct {
	// Index of the FFmpeg input that carries the microphone / system audio,
	// or -1 when that source is not recorded.
	MicIndex int
	SysIndex int

	// Linear gain (1.0 = unchanged).
	MicVolume float64
	SysVolume float64

	// Denoise applies a high-pass filter and FFT noise reduction to the
	// microphone only.
	Denoise bool

	// Separate keeps the microphone and the system audio as two audio tracks
	// instead of mixing them into one.
	Separate bool
}

// parseVolumePercent converts a combo-box id such as "150" into a linear gain
// (1.5). Invalid or out-of-range values fall back to 1.0.
func parseVolumePercent(id string) float64 {
	percent, err := strconv.Atoi(strings.TrimSpace(id))
	if err != nil || percent < 10 || percent > 400 {
		return 1.0
	}

	return float64(percent) / 100.0
}

func formatGain(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// channelChain builds the per-source filter chain that turns FFmpeg input
// `index` into the label `out`.
func channelChain(index int, volume float64, denoise bool, out string) string {
	filters := make([]string, 0, 3)

	if denoise {
		filters = append(filters, "highpass=f=80", "afftdn=nf=-25")
	}

	if volume > 0 && volume != 1.0 {
		filters = append(filters, "volume="+formatGain(volume))
	}

	if len(filters) == 0 {
		filters = append(filters, "anull")
	}

	return fmt.Sprintf("[%d:a:0]%s[%s]", index, strings.Join(filters, ","), out)
}

// buildAudioFilter returns the -filter_complex graph and the labels that must
// be passed to -map (one label for a mixed result, two for separate tracks).
// It returns an empty graph when no audio source is selected.
func buildAudioFilter(o audioMixOptions) (string, []string) {
	hasMic := o.MicIndex >= 0
	hasSys := o.SysIndex >= 0

	const tail = "alimiter=limit=0.95,aresample=async=1000"

	switch {
	case hasMic && hasSys && o.Separate:
		graph := channelChain(o.MicIndex, o.MicVolume, o.Denoise, "m") + ";" +
			channelChain(o.SysIndex, o.SysVolume, false, "s") + ";" +
			"[m]" + tail + "[a0];" +
			"[s]" + tail + "[a1]"

		return graph, []string{"[a0]", "[a1]"}

	case hasMic && hasSys:
		graph := channelChain(o.MicIndex, o.MicVolume, o.Denoise, "m") + ";" +
			channelChain(o.SysIndex, o.SysVolume, false, "s") + ";" +
			"[m][s]amix=inputs=2:duration=longest:dropout_transition=2," +
			"volume=2," + tail + "[aout]"

		return graph, []string{"[aout]"}

	case hasMic:
		graph := channelChain(o.MicIndex, o.MicVolume, o.Denoise, "m") + ";" +
			"[m]aresample=async=1000[aout]"

		return graph, []string{"[aout]"}

	case hasSys:
		graph := channelChain(o.SysIndex, o.SysVolume, false, "s") + ";" +
			"[s]aresample=async=1000[aout]"

		return graph, []string{"[aout]"}
	}

	return "", nil
}

var (
	ffmpegFilterOnce sync.Once
	ffmpegFilterList string
)

// ffmpegHasFilter reports whether the installed FFmpeg provides the named
// filter (for example "afftdn", which older builds do not include).
func ffmpegHasFilter(name string) bool {
	ffmpegFilterOnce.Do(func() {
		out, err := exec.Command(
			"ffmpeg", "-hide_banner", "-filters",
		).Output()
		if err == nil {
			ffmpegFilterList = string(out)
		}
	})

	for _, line := range strings.Split(ffmpegFilterList, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == name {
			return true
		}
	}

	return false
}

// hasCommand reports whether an executable is available in PATH.
func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
