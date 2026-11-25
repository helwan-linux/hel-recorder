// formats.go
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

func isSupportedVideoFormat(formatExt string) bool {
	switch strings.ToLower(strings.TrimSpace(formatExt)) {
	case "mp4", "mkv", "webm", "gif":
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Hardware encoder detection
// ---------------------------------------------------------------------------

const (
	hwNone  = ""
	hwVAAPI = "vaapi"
	hwNVENC = "nvenc"
)

// hwEncoder describes the video encoder pipeline that was verified to work on
// this machine. It is detected once (in the background at start-up) and cached,
// so pausing/resuming never pays the detection cost again.
type hwEncoder struct {
	Kind   string // hwNone, hwVAAPI or hwNVENC
	Device string // VAAPI render node, e.g. /dev/dri/renderD128

	// GPUConvert is true when the BGRx -> NV12 conversion can run on the GPU
	// (hwupload,scale_vaapi). When false the conversion happens on the CPU
	// (format=nv12,hwupload) which is slower but works on older drivers.
	GPUConvert bool
}

func (h hwEncoder) Label() string {
	switch h.Kind {
	case hwVAAPI:
		return "VAAPI"
	case hwNVENC:
		return "NVENC"
	default:
		return "CPU"
	}
}

// InitArgs returns the global/input options required by the encoder.
func (h hwEncoder) InitArgs() []string {
	if h.Kind == hwVAAPI && h.Device != "" {
		return []string{"-vaapi_device", h.Device}
	}
	return nil
}

func (h hwEncoder) vaapiUploadFilter() string {
	if h.GPUConvert {
		return "hwupload,scale_vaapi=format=nv12"
	}
	return "format=nv12,hwupload"
}

var (
	hwOnce   sync.Once
	hwCached hwEncoder
)

// getHWEncoder returns the cached result of detectHWEncoder.
func getHWEncoder() hwEncoder {
	hwOnce.Do(func() {
		hwCached = detectHWEncoder()
		debugf("encoder detected: %s %+v", hwCached.Label(), hwCached)
	})
	return hwCached
}

func testEncode(ffmpeg string, args ...string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	base := []string{"-hide_banner", "-loglevel", "error"}
	base = append(base, args...)

	return exec.CommandContext(ctx, ffmpeg, base...).Run() == nil
}

// detectHWEncoder probes VAAPI first (Intel/AMD), then NVENC (NVIDIA). Every
// candidate is tested with the exact pixel format x11grab produces (bgr0), so a
// positive result means the real recording pipeline will work.
func detectHWEncoder() hwEncoder {
	if os.Getenv("HSR_FORCE_CPU") != "" {
		return hwEncoder{}
	}

	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return hwEncoder{}
	}

	const source = "color=c=black:s=256x256:r=30:d=0.2,format=bgr0"

	devices, _ := filepath.Glob("/dev/dri/renderD*")
	for _, device := range devices {
		if info, err := os.Stat(device); err != nil || info.IsDir() {
			continue
		}

		for _, gpuConvert := range []bool{true, false} {
			candidate := hwEncoder{Kind: hwVAAPI, Device: device, GPUConvert: gpuConvert}

			ok := testEncode(
				ffmpeg,
				"-vaapi_device", device,
				"-f", "lavfi", "-i", source,
				"-vf", candidate.vaapiUploadFilter(),
				"-frames:v", "3",
				"-r", "30",
				"-c:v", "h264_vaapi", "-qp", "22",
				"-f", "null", "-",
			)
			if ok {
				return candidate
			}
		}
	}

	if _, err := os.Stat("/dev/nvidiactl"); err == nil {
		ok := testEncode(
			ffmpeg,
			"-f", "lavfi", "-i", source,
			"-frames:v", "3",
			"-c:v", "h264_nvenc", "-preset", "p1", "-tune", "ll",
			"-rc", "constqp", "-qp", "23",
			"-f", "null", "-",
		)
		if ok {
			return hwEncoder{Kind: hwNVENC}
		}
	}

	return hwEncoder{}
}

// ---------------------------------------------------------------------------
// Input queue support
// ---------------------------------------------------------------------------

var (
	queueOnce      sync.Once
	queueSupported bool
)

// inputQueueArgs returns "-thread_queue_size N" only if this FFmpeg build
// accepts it as an input option. Some FFmpeg versions reject it with
// "cannot be applied to input", which would make every recording fail.
func inputQueueArgs(size string) []string {
	queueOnce.Do(func() {
		ffmpeg, err := exec.LookPath("ffmpeg")
		if err != nil {
			return
		}

		queueSupported = testEncode(
			ffmpeg,
			"-thread_queue_size", "8",
			"-f", "lavfi", "-i", "color=c=black:s=16x16:d=0.1",
			"-frames:v", "1",
			"-f", "null", "-",
		)

		debugf("input -thread_queue_size supported: %v", queueSupported)
	})

	if !queueSupported {
		return nil
	}

	return []string{"-thread_queue_size", size}
}

// ---------------------------------------------------------------------------
// Encoder settings
// ---------------------------------------------------------------------------

func gopSize(fps string) int {
	n, err := strconv.Atoi(fps)
	if err != nil || n <= 0 {
		n = 30
	}
	return n * 2
}

// qualitySettings is the rate-control choice made in the Advanced panel.
type qualitySettings struct {
	Level   string // "high", "standard" or "small"; ignored when Bitrate > 0
	Bitrate int    // target bitrate in kbps; 0 means constant quality
}

// qualityOffset shifts the default QP/CRF value of every encoder. A lower
// number means better quality and a larger file.
func (q qualitySettings) qualityOffset() int {
	switch q.Level {
	case "high":
		return -4
	case "small":
		return 6
	default:
		return 0
	}
}

func (q qualitySettings) bitrateArg() string {
	return strconv.Itoa(q.Bitrate) + "k"
}

func (q qualitySettings) bufsizeArg() string {
	return strconv.Itoa(q.Bitrate*2) + "k"
}

func h264VideoArgs(hw hwEncoder, gop string, vfInGraph bool, q qualitySettings) []string {
	switch hw.Kind {
	case hwVAAPI:
		args := []string{}
		if !vfInGraph {
			args = append(args, "-vf", hw.vaapiUploadFilter())
		}

		args = append(args, "-c:v", "h264_vaapi")

		if q.Bitrate > 0 {
			args = append(args, "-b:v", q.bitrateArg(), "-maxrate", q.bitrateArg())
		} else {
			args = append(args, "-qp", strconv.Itoa(22+q.qualityOffset()))
		}

		return append(args, "-bf", "0", "-g", gop)

	case hwNVENC:
		args := []string{
			"-c:v", "h264_nvenc",
			"-preset", "p1",
			"-tune", "ll",
		}

		if q.Bitrate > 0 {
			args = append(args, "-rc", "cbr", "-b:v", q.bitrateArg())
		} else {
			args = append(args, "-rc", "constqp", "-qp", strconv.Itoa(23+q.qualityOffset()))
		}

		return append(args, "-bf", "0", "-g", gop)

	default:
		args := []string{
			"-pix_fmt", "yuv420p",
			"-sws_flags", "fast_bilinear",
			"-c:v", "libx264",
			"-preset", "ultrafast",
			"-tune", "zerolatency",
		}

		if q.Bitrate > 0 {
			args = append(args,
				"-b:v", q.bitrateArg(),
				"-maxrate", q.bitrateArg(),
				"-bufsize", q.bufsizeArg(),
			)
		} else {
			args = append(args, "-crf", strconv.Itoa(22+q.qualityOffset()))
		}

		return append(args, "-g", gop)
	}
}

// internalPartExt returns the container used while recording.
//
// MP4 and GIF sessions are recorded into Matroska: it survives crashes and
// power loss, and the final MP4 (with +faststart) or GIF is produced by a fast
// stream-copy remux / palette pass after recording stops.
func internalPartExt(formatExt string) string {
	if strings.ToLower(strings.TrimSpace(formatExt)) == "webm" {
		return "webm"
	}
	return "mkv"
}

// getFormatSettings returns the encoder options for the live recording.
//
// vfInGraph tells the function that the video filter chain (including the
// VAAPI upload) is already part of -filter_complex, so no -vf must be added.
// q selects the quality level or a fixed bitrate.
func getFormatSettings(
	formatExt string,
	fps string,
	hw hwEncoder,
	hasAudio bool,
	vfInGraph bool,
	q qualitySettings,
) []string {
	formatExt = strings.ToLower(strings.TrimSpace(formatExt))
	gop := strconv.Itoa(gopSize(fps))

	// A GIF is produced from this recording by a palette pass, so the
	// intermediate file always uses the standard quality.
	if formatExt == "gif" {
		q = qualitySettings{Level: "standard"}
	}

	if formatExt == "webm" {
		args := []string{
			"-r", fps,
			"-pix_fmt", "yuv420p",
			"-c:v", "libvpx-vp9",
			"-deadline", "realtime",
			"-cpu-used", "6",
			"-row-mt", "1",
			"-tile-columns", "2",
			"-lag-in-frames", "0",
		}

		if q.Bitrate > 0 {
			args = append(args, "-b:v", q.bitrateArg())
		} else {
			args = append(args,
				"-crf", strconv.Itoa(32+q.qualityOffset()),
				"-b:v", "0",
			)
		}

		args = append(args, "-g", gop)

		if hasAudio {
			args = append(args, "-c:a", "libopus", "-b:a", "192k")
		}

		return append(args, "-f", "webm")
	}

	// mp4, mkv and gif are all recorded as H.264 in Matroska.
	args := []string{"-r", fps}
	args = append(args, h264VideoArgs(hw, gop, vfInGraph, q)...)

	if hasAudio && formatExt != "gif" {
		args = append(args, "-c:a", "aac", "-b:a", "192k")
	}

	return append(args, "-f", "matroska")
}

// ---------------------------------------------------------------------------
// Files
// ---------------------------------------------------------------------------

func verifyOutput(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("output file is missing: %w", err)
	}

	if info.IsDir() {
		return fmt.Errorf("output path is a directory")
	}

	if info.Size() == 0 {
		return fmt.Errorf("output file is empty")
	}

	return nil
}

// generatePartFileName creates the temporary part file next to the final
// recording. Using the save directory instead of /tmp avoids filling RAM when
// /tmp is a tmpfs, and makes the final rename instant (same filesystem).
func generatePartFileName(saveDir, formatExt string) string {
	dir := saveDir
	if strings.TrimSpace(dir) == "" {
		dir = os.TempDir()
	}

	return filepath.Join(
		dir,
		fmt.Sprintf(
			".hel_rec_part_%d.%s",
			time.Now().UnixNano(),
			strings.ToLower(strings.TrimSpace(formatExt)),
		),
	)
}

// writeConcatList writes an FFmpeg concat-demuxer list and returns its path.
func writeConcatList(parts []string) (string, error) {
	listFile, err := os.CreateTemp("", "hel_concat_*.txt")
	if err != nil {
		return "", fmt.Errorf("cannot create temporary merge list: %w", err)
	}

	name := listFile.Name()

	for _, part := range parts {
		escaped := strings.ReplaceAll(part, "'", "'\\''")

		if _, err := fmt.Fprintf(listFile, "file '%s'\n", escaped); err != nil {
			_ = listFile.Close()
			_ = os.Remove(name)
			return "", fmt.Errorf("cannot write merge list: %w", err)
		}
	}

	if err := listFile.Close(); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("cannot close merge list: %w", err)
	}

	return name, nil
}

func mergeRecordingParts(parts []string, outputFile string) error {
	if len(parts) < 2 {
		return fmt.Errorf("at least two recording parts are required")
	}

	ext := strings.ToLower(filepath.Ext(outputFile))

	if ext == ".gif" {
		// GIF parts are intermediate MKV files. Merge them first, then run a
		// single palette conversion over the complete recording.
		merged, err := os.CreateTemp(filepath.Dir(outputFile), ".hel_gif_merge_*.mkv")
		if err != nil {
			return fmt.Errorf("cannot create temporary GIF merge file: %w", err)
		}

		mergedPath := merged.Name()

		if err := merged.Close(); err != nil {
			_ = os.Remove(mergedPath)
			return fmt.Errorf("cannot close temporary GIF merge file: %w", err)
		}

		defer os.Remove(mergedPath)

		if err := mergeVideoPartsToFile(parts, mergedPath); err != nil {
			return err
		}

		return convertVideoToGIF(mergedPath, outputFile)
	}

	listName, err := writeConcatList(parts)
	if err != nil {
		return err
	}
	defer os.Remove(listName)

	args := []string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-f", "concat",
		"-safe", "0",
		"-i", listName,
		"-c", "copy",
	}

	switch ext {
	case ".mp4":
		args = append(args, "-movflags", "+faststart", "-f", "mp4")
	case ".webm":
		args = append(args, "-f", "webm")
	default:
		args = append(args, "-f", "matroska")
	}

	return runFFmpeg(append(args, outputFile))
}

func mergeVideoPartsToFile(parts []string, outputFile string) error {
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
		"-c", "copy",
		"-f", "matroska",
		outputFile,
	})
}

func convertVideoToGIF(inputFile, outputFile string) error {
	return runFFmpeg([]string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputFile,
		"-filter_complex",
		"[0:v:0]fps=15,scale='min(720,iw)':-1:flags=lanczos,split[s0][s1];" +
			"[s0]palettegen=max_colors=256:stats_mode=diff[p];" +
			"[s1][p]paletteuse=dither=sierra2_4a:diff_mode=rectangle[vout]",
		"-map", "[vout]",
		"-loop", "0",
		"-f", "gif",
		outputFile,
	})
}

// remuxToMP4 converts a Matroska recording to a standard MP4 without
// re-encoding (stream copy), moving the index to the front for web playback.
func remuxToMP4(inputFile, outputFile string) error {
	return runFFmpeg([]string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputFile,
		"-c", "copy",
		"-movflags", "+faststart",
		"-f", "mp4",
		outputFile,
	})
}

// finalizeSinglePart turns the only recorded part into the final output file.
// If the conversion fails, the raw recording is kept as an MKV next to the
// intended output so that no footage is ever lost.
func finalizeSinglePart(part, outputFile string) error {
	var err error

	switch strings.ToLower(filepath.Ext(outputFile)) {
	case ".mp4":
		err = remuxToMP4(part, outputFile)
	case ".gif":
		err = convertVideoToGIF(part, outputFile)
	default:
		return moveFile(part, outputFile)
	}

	if err == nil {
		_ = os.Remove(part)
		return nil
	}

	_ = os.Remove(outputFile)

	fallback := strings.TrimSuffix(outputFile, filepath.Ext(outputFile)) + ".mkv"
	if moveErr := moveFile(part, fallback); moveErr == nil {
		return fmt.Errorf(
			"%w\nThe raw recording was saved as:\n%s",
			err,
			fallback,
		)
	}

	return fmt.Errorf("%w\nThe raw recording was kept at:\n%s", err, part)
}

func runFFmpeg(args []string) error {
	output, err := exec.Command("ffmpeg", args...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}

		if len(message) > 600 {
			message = "..." + message[len(message)-600:]
		}

		return fmt.Errorf("FFmpeg failed: %s", message)
	}

	return nil
}
