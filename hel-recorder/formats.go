// formats.go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func getFormatSettings(
	formatExt string,
	fps string,
	res string,
	vaapiDevice string,
	hasAudio bool,
) []string {
	formatExt = strings.ToLower(strings.TrimSpace(formatExt))

	switch formatExt {
	case "gif":
		// GIF is produced after recording from an intermediate MKV part.
		// Keeping the live recording in a normal video container avoids
		// fragile palette generation during the live capture process.
		return []string{
			"-pix_fmt", "yuv420p",
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-crf", "23",
			"-r", fps,
			"-f", "matroska",
		}

	case "webm":
		args := []string{
			"-pix_fmt", "yuv420p",
			"-c:v", "libvpx-vp9",
			"-deadline", "realtime",
			"-cpu-used", "5",
			"-crf", "32",
			"-b:v", "0",
			"-r", fps,
		}

		if hasAudio {
			args = append(args,
				"-c:a", "libopus",
				"-b:a", "192k",
			)
		}

		return args

	case "mkv":
		args := []string{
			"-pix_fmt", "yuv420p",
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-crf", "23",
			"-r", fps,
			"-f", "matroska",
		}

		if hasAudio {
			args = append(args,
				"-c:a", "aac",
				"-b:a", "192k",
			)
		}

		return args

	case "mp4":
		fallthrough

	default:
		args := []string{"-r", fps}

		if vaapiDevice != "" {
			args = append(args,
				"-vf", "format=nv12,hwupload",
				"-c:v", "h264_vaapi",
				"-qp", "22",
			)
		} else {
			args = append(args,
				"-pix_fmt", "yuv420p",
				"-c:v", "libx264",
				"-preset", "ultrafast",
				"-tune", "zerolatency",
				"-crf", "22",
			)
		}

		if hasAudio {
			args = append(args,
				"-c:a", "aac",
				"-b:a", "192k",
			)
		}

		args = append(args,
			"-movflags", "+faststart",
			"-f", "mp4",
		)

		return args
	}
}

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

func generatePartFileName(saveDir, formatExt string) string {
	_ = saveDir

	return filepath.Join(
		os.TempDir(),
		fmt.Sprintf(
			"hel_rec_part_%d.%s",
			time.Now().UnixNano(),
			strings.ToLower(strings.TrimSpace(formatExt)),
		),
	)
}

func mergeRecordingParts(parts []string, outputFile string) error {
	if len(parts) < 2 {
		return fmt.Errorf("at least two recording parts are required")
	}

	ext := strings.ToLower(filepath.Ext(outputFile))

	if ext == ".gif" {
		// GIF parts are actually intermediate MKV files. Merge them first,
		// then perform one palette conversion over the complete recording.
		merged, err := os.CreateTemp(os.TempDir(), "hel_gif_merge_*.mkv")
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

	listFile, err := os.CreateTemp("", "hel_concat_*.txt")
	if err != nil {
		return fmt.Errorf("cannot create temporary merge list: %w", err)
	}

	listName := listFile.Name()
	defer os.Remove(listName)

	for _, part := range parts {
		escaped := strings.ReplaceAll(part, "'", "'\\''")

		if _, err := fmt.Fprintf(listFile, "file '%s'\n", escaped); err != nil {
			_ = listFile.Close()
			return fmt.Errorf("cannot write merge list: %w", err)
		}
	}

	if err := listFile.Close(); err != nil {
		return fmt.Errorf("cannot close merge list: %w", err)
	}

	return runFFmpeg([]string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-f", "concat",
		"-safe", "0",
		"-i", listName,
		"-c", "copy",
		outputFile,
	})
}

func mergeVideoPartsToFile(parts []string, outputFile string) error {
	listFile, err := os.CreateTemp(os.TempDir(), "hel_gif_concat_*.txt")
	if err != nil {
		return fmt.Errorf("cannot create GIF merge list: %w", err)
	}
	listName := listFile.Name()
	defer os.Remove(listName)

	for _, part := range parts {
		escaped := strings.ReplaceAll(part, "'", "'\\''")
		if _, err := fmt.Fprintf(listFile, "file '%s'\n", escaped); err != nil {
			_ = listFile.Close()
			return fmt.Errorf("cannot write GIF merge list: %w", err)
		}
	}

	if err := listFile.Close(); err != nil {
		return fmt.Errorf("cannot close GIF merge list: %w", err)
	}

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
		"[0:v:0]fps=15,scale=720:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=256[p];[s1][p]paletteuse=dither=sierra2_4a[vout]",
		"-map", "[vout]",
		"-loop", "0",
		"-gifflags", "+transdiff",
		"-f", "gif",
		outputFile,
	})
}

func runFFmpeg(args []string) error {
	output, err := exec.Command("ffmpeg", args...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}

		return fmt.Errorf("FFmpeg merge failed: %s", message)
	}

	return nil
}
