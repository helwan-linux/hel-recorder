// recovery_logic.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Recovery of unfinished recordings (pure logic: no GTK)
//
// While recording, the application writes hidden part files such as
// ".hel_rec_part_1763000000000000000.mkv" (video) and ".mka" (audio) next to
// the final recording. They are removed after a successful save. If the
// application or the computer dies mid-recording, they stay behind; the
// functions below find them so the footage can be recovered.
// ---------------------------------------------------------------------------

const orphanPartPrefix = ".hel_rec_part_"

// orphanPairGap is the largest difference between the creation stamps of a
// video part and its audio part. Both are created back to back when a
// recording part starts, so the real difference is a few milliseconds.
const orphanPairGap = int64(5 * time.Second)

type orphanPart struct {
	Path  string
	Stamp int64 // nanosecond timestamp taken from the file name
	Size  int64
	Audio bool
}

type recoveryJob struct {
	Video orphanPart // empty Path when only an audio part survived
	Audio *orphanPart
}

// parseOrphanName extracts the stamp and extension from a part file name.
func parseOrphanName(name string) (stamp int64, ext string, ok bool) {
	if !strings.HasPrefix(name, orphanPartPrefix) {
		return 0, "", false
	}

	rest := strings.TrimPrefix(name, orphanPartPrefix)
	dot := strings.LastIndex(rest, ".")
	if dot <= 0 {
		return 0, "", false
	}

	n, err := strconv.ParseInt(rest[:dot], 10, 64)
	if err != nil || n <= 0 {
		return 0, "", false
	}

	return n, strings.ToLower(rest[dot+1:]), true
}

// scanOrphanParts lists leftover part files in dir. Files modified less than
// minAge ago, and empty files, are ignored.
func scanOrphanParts(
	dir string,
	now time.Time,
	minAge time.Duration,
) (videos []orphanPart, audios []orphanPart) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		stamp, ext, ok := parseOrphanName(entry.Name())
		if !ok {
			continue
		}

		isAudio := false

		switch ext {
		case "mkv", "webm":
		case "mka":
			isAudio = true
		default:
			continue
		}

		info, err := entry.Info()
		if err != nil || info.Size() <= 0 {
			continue
		}

		if now.Sub(info.ModTime()) < minAge {
			continue
		}

		part := orphanPart{
			Path:  filepath.Join(dir, entry.Name()),
			Stamp: stamp,
			Size:  info.Size(),
			Audio: isAudio,
		}

		if isAudio {
			audios = append(audios, part)
		} else {
			videos = append(videos, part)
		}
	}

	sort.Slice(videos, func(i, j int) bool { return videos[i].Stamp < videos[j].Stamp })
	sort.Slice(audios, func(i, j int) bool { return audios[i].Stamp < audios[j].Stamp })

	return videos, audios
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}

	return v
}

// pairRecoveryParts matches every video part with the audio part created at
// the same moment. Audio parts without a video part become audio-only jobs.
func pairRecoveryParts(videos, audios []orphanPart) []recoveryJob {
	used := make([]bool, len(audios))
	jobs := make([]recoveryJob, 0, len(videos))

	for _, video := range videos {
		best := -1
		bestGap := orphanPairGap + 1

		for i, audio := range audios {
			if used[i] {
				continue
			}

			if gap := absInt64(audio.Stamp - video.Stamp); gap < bestGap {
				best = i
				bestGap = gap
			}
		}

		job := recoveryJob{Video: video}

		if best >= 0 {
			used[best] = true
			matched := audios[best]
			job.Audio = &matched
		}

		jobs = append(jobs, job)
	}

	for i, audio := range audios {
		if used[i] {
			continue
		}

		leftover := audio
		jobs = append(jobs, recoveryJob{Audio: &leftover})
	}

	return jobs
}

// recoveredFileName returns a non-existing output path for a recovered job.
func recoveredFileName(dir string, job recoveryJob) string {
	stamp := job.Video.Stamp
	ext := "mkv"

	switch {
	case job.Video.Path == "" && job.Audio != nil:
		stamp = job.Audio.Stamp
		ext = "mka"

	case strings.EqualFold(filepath.Ext(job.Video.Path), ".webm"):
		ext = "webm"
	}

	base := fmt.Sprintf(
		"Recovered_%s",
		time.Unix(0, stamp).Format("2006-01-02_15-04-05"),
	)

	candidate := filepath.Join(dir, base+"."+ext)

	for n := 2; ; n++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}

		candidate = filepath.Join(dir, fmt.Sprintf("%s_%d.%s", base, n, ext))
	}
}

// recoverJob turns one job into a playable file and removes the parts that
// were used. It returns the path of the recovered file.
func recoverJob(dir string, job recoveryJob) (string, error) {
	out := recoveredFileName(dir, job)

	switch {
	case job.Video.Path == "" && job.Audio != nil:
		if err := moveFile(job.Audio.Path, out); err != nil {
			return "", err
		}

		return out, nil

	case job.Audio == nil:
		if err := moveFile(job.Video.Path, out); err != nil {
			return "", err
		}

		return out, nil
	}

	if err := muxVideoAudio(job.Video.Path, job.Audio.Path, out); err != nil {
		// Never lose the video: keep it on its own and leave the audio part.
		_ = os.Remove(out)

		if moveErr := moveFile(job.Video.Path, out); moveErr != nil {
			return "", err
		}

		return out, fmt.Errorf(
			"video recovered without audio (audio part kept): %w",
			err,
		)
	}

	_ = os.Remove(job.Video.Path)
	_ = os.Remove(job.Audio.Path)

	return out, nil
}

// removeOrphanParts deletes the given leftover parts.
func removeOrphanParts(videos, audios []orphanPart) {
	for _, p := range videos {
		_ = os.Remove(p.Path)
	}

	for _, p := range audios {
		_ = os.Remove(p.Path)
	}
}

func orphanTotalSize(videos, audios []orphanPart) int64 {
	var total int64

	for _, p := range videos {
		total += p.Size
	}

	for _, p := range audios {
		total += p.Size
	}

	return total
}
