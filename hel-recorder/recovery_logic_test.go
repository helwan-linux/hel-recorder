package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseOrphanName(t *testing.T) {
	stamp, ext, ok := parseOrphanName(".hel_rec_part_1763000000000000000.mkv")
	if !ok || stamp != 1763000000000000000 || ext != "mkv" {
		t.Fatalf("got %d %q %v", stamp, ext, ok)
	}

	for _, bad := range []string{
		"Recording_2026.mp4",
		".hel_rec_part_abc.mkv",
		".hel_rec_part_123",
		".hel_rec_part_0.mkv",
	} {
		if _, _, ok := parseOrphanName(bad); ok {
			t.Errorf("%q must not be accepted", bad)
		}
	}
}

func writeAged(t *testing.T, dir, name string, size int, age time.Duration) {
	t.Helper()

	path := filepath.Join(dir, name)

	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}

	when := time.Now().Add(-age)

	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func TestScanOrphanParts(t *testing.T) {
	dir := t.TempDir()

	writeAged(t, dir, ".hel_rec_part_2000.mkv", 10, time.Hour)
	writeAged(t, dir, ".hel_rec_part_1000.mkv", 10, time.Hour)
	writeAged(t, dir, ".hel_rec_part_1001.mka", 10, time.Hour)
	writeAged(t, dir, ".hel_rec_part_3000.mkv", 10, time.Second) // too fresh
	writeAged(t, dir, ".hel_rec_part_4000.mkv", 0, time.Hour)    // empty
	writeAged(t, dir, ".hel_rec_part_5000.txt", 10, time.Hour)   // wrong type
	writeAged(t, dir, "Recording_1.mp4", 10, time.Hour)          // real recording

	videos, audios := scanOrphanParts(dir, time.Now(), 30*time.Second)

	if len(videos) != 2 || videos[0].Stamp != 1000 || videos[1].Stamp != 2000 {
		t.Errorf("unexpected videos: %+v", videos)
	}

	if len(audios) != 1 || audios[0].Stamp != 1001 {
		t.Errorf("unexpected audios: %+v", audios)
	}
}

func TestPairRecoveryParts(t *testing.T) {
	sec := int64(time.Second)

	videos := []orphanPart{
		{Path: "v1", Stamp: 100 * sec},
		{Path: "v2", Stamp: 500 * sec},
	}

	audios := []orphanPart{
		{Path: "a1", Stamp: 100*sec + 1000, Audio: true},
		{Path: "a3", Stamp: 900 * sec, Audio: true}, // no video
	}

	jobs := pairRecoveryParts(videos, audios)

	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d: %+v", len(jobs), jobs)
	}

	if jobs[0].Audio == nil || jobs[0].Audio.Path != "a1" {
		t.Errorf("v1 should be paired with a1: %+v", jobs[0])
	}

	if jobs[1].Audio != nil {
		t.Errorf("v2 has no audio: %+v", jobs[1])
	}

	if jobs[2].Video.Path != "" || jobs[2].Audio == nil || jobs[2].Audio.Path != "a3" {
		t.Errorf("a3 should be an audio-only job: %+v", jobs[2])
	}
}

func TestRecoveredFileNameAvoidsOverwrite(t *testing.T) {
	dir := t.TempDir()
	job := recoveryJob{Video: orphanPart{Path: "x.mkv", Stamp: time.Now().UnixNano()}}

	first := recoveredFileName(dir, job)

	if err := os.WriteFile(first, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	second := recoveredFileName(dir, job)
	if second == first {
		t.Fatalf("existing file would be overwritten: %s", second)
	}
}
