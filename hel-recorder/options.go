// options.go
package main

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
// Options read from the "Advanced" panel when a recording part starts
// ---------------------------------------------------------------------------

// selectedQuality returns the quality / bitrate choice. A bitrate other than
// "Auto" overrides the quality level.
func (m *RecorderManager) selectedQuality() qualitySettings {
	q := qualitySettings{Level: "standard"}

	if m.ui.ComboQuality != nil {
		switch level := m.ui.ComboQuality.GetActiveID(); level {
		case "high", "standard", "small":
			q.Level = level
		}
	}

	if m.ui.ComboBitrate != nil {
		kbps, err := strconv.Atoi(m.ui.ComboBitrate.GetActiveID())
		if err == nil && kbps >= 500 && kbps <= 100000 {
			q.Bitrate = kbps
		}
	}

	return q
}

// selectedAutoStop returns the maximum recording length (0 = no limit).
func (m *RecorderManager) selectedAutoStop() time.Duration {
	if m.ui.ComboAutoStop == nil {
		return 0
	}

	minutes, err := strconv.Atoi(m.ui.ComboAutoStop.GetActiveID())
	if err != nil || minutes <= 0 {
		return 0
	}

	return time.Duration(minutes) * time.Minute
}

// selectedMinFreeBytes returns the free disk space that must remain available
// (0 = guard disabled).
func (m *RecorderManager) selectedMinFreeBytes() uint64 {
	if m.ui.ComboDiskGuard == nil {
		return 0
	}

	megabytes, err := strconv.Atoi(m.ui.ComboDiskGuard.GetActiveID())
	if err != nil || megabytes <= 0 {
		return 0
	}

	return uint64(megabytes) * 1024 * 1024
}

// ---------------------------------------------------------------------------
// Disk space helpers
// ---------------------------------------------------------------------------

// freeDiskBytes returns the space available to a normal user on the
// filesystem that holds dir.
func freeDiskBytes(dir string) (uint64, error) {
	var st syscall.Statfs_t

	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}

	return uint64(st.Bavail) * uint64(st.Bsize), nil
}

// totalFileSize adds up the size of the given files; missing files and empty
// names are ignored.
func totalFileSize(paths []string) uint64 {
	var total uint64

	for _, path := range paths {
		if path == "" {
			continue
		}

		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() <= 0 {
			continue
		}

		total += uint64(info.Size())
	}

	return total
}

// diskLowReason returns a message when the free space in dir is too low, or
// an empty string when everything is fine.
//
// Finishing a recording (remuxing to MP4, muxing audio) writes a second file
// about as large as the recording itself while the raw parts still exist. So
// the space that must stay free is the user's margin PLUS the size already
// recorded; stopping at that point guarantees there is room to save.
func diskLowReason(dir string, margin uint64, recorded uint64) string {
	free, err := freeDiskBytes(dir)
	if err != nil {
		return ""
	}

	if free < margin+recorded {
		return fmt.Sprintf(
			"Recording stopped automatically: free disk space is low (%s left).",
			formatBytes(free),
		)
	}

	return ""
}

func formatBytes(n uint64) string {
	const unit = 1024

	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	div, exp := uint64(unit), 0
	for rest := n / unit; rest >= unit; rest /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
