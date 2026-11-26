// extras.go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

const (
	respOpenFolder gtk.ResponseType = 100
	respPlay       gtk.ResponseType = 103
)

// ---------------------------------------------------------------------------
// Opening files and folders
// ---------------------------------------------------------------------------

// openWithDefaultApp opens a file or folder with the desktop's default
// application (xdg-open). Failures are ignored: it is only a convenience.
func openWithDefaultApp(path string) {
	if path == "" || !hasCommand("xdg-open") {
		return
	}

	cmd := exec.Command("xdg-open", path)

	if err := cmd.Start(); err != nil {
		debugf("xdg-open failed: %v", err)
		return
	}

	go func() { _ = cmd.Wait() }()
}

// ---------------------------------------------------------------------------
// Wayland detection
// ---------------------------------------------------------------------------

func isWaylandSession() bool {
	return strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland")
}

// warnIfWayland tells the user, once, that screen capture is X11-only.
// Under Wayland, x11grab can only see XWayland windows (usually a black
// screen), and global shortcuts only work while an X11 window has focus.
func warnIfWayland(parent *gtk.Window) {
	if !isWaylandSession() {
		return
	}

	marker := ""
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		marker = filepath.Join(dir, "helwan-screen-recorder", "wayland_warned")

		if _, err := os.Stat(marker); err == nil {
			return
		}
	}

	dialog := gtk.MessageDialogNew(
		parent,
		gtk.DIALOG_MODAL,
		gtk.MESSAGE_WARNING,
		gtk.BUTTONS_OK,
		"%s",
		"A Wayland session was detected.\n\n"+
			"Helwan Screen Recorder captures the screen through X11, so under "+
			"Wayland the recording may be black or only show X11 applications, "+
			"and the global shortcuts may not work.\n\n"+
			"For reliable results, log in with an X11 session.",
	)

	dialog.SetTitle("Wayland Detected")
	dialog.Run()
	dialog.Destroy()

	if marker != "" {
		_ = os.MkdirAll(filepath.Dir(marker), 0o755)
		_ = os.WriteFile(marker, []byte("1\n"), 0o644)
	}
}

// ---------------------------------------------------------------------------
// Mouse effects: keep the checkbox and the Ctrl+T shortcut in sync
// ---------------------------------------------------------------------------

var syncingMouseFxCheckbox bool

// syncMouseEffectsCheckbox makes the checkbox reflect the real state of the
// effect overlay (it can also be changed with Ctrl+T, or fail to start).
func (m *RecorderManager) syncMouseEffectsCheckbox() {
	glib.IdleAdd(func() {
		if m.ui.ChkMouseFx == nil {
			return
		}

		syncingMouseFxCheckbox = true
		m.ui.ChkMouseFx.SetActive(mouseEffectsController.active)
		syncingMouseFxCheckbox = false
	})
}

// connectMouseEffectsCheckbox wires the checkbox to the overlay. The
// Ctrl+T shortcut is unchanged and keeps working.
func (m *RecorderManager) connectMouseEffectsCheckbox() {
	if m.ui.ChkMouseFx == nil {
		return
	}

	m.ui.ChkMouseFx.Connect("toggled", func() {
		if syncingMouseFxCheckbox {
			return
		}

		if m.ui.ChkMouseFx.GetActive() != mouseEffectsController.active {
			m.ToggleMouseEffects()
		}
	})
}

// ---------------------------------------------------------------------------
// Screenshot
// ---------------------------------------------------------------------------

// TakeScreenshot saves one PNG of the selected screen / area / window / monitor
// into the save directory. It must be called on the GTK main thread (it reads
// widgets) and does the slow work in the background.
func (m *RecorderManager) TakeScreenshot() {
	m.mu.Lock()
	busy := m.stopping || m.finalizing || m.countdownActive
	recording := m.isRecording
	m.mu.Unlock()

	if busy {
		return
	}

	saveDir := strings.TrimSpace(m.ui.FileChooser.GetFilename())
	if saveDir == "" {
		m.setStatus("Error: Select a save directory")
		return
	}

	if info, err := os.Stat(saveDir); err != nil || !info.IsDir() {
		m.setStatus("Error: Invalid save directory")
		return
	}

	if !hasCommand("ffmpeg") {
		m.setStatus("Error: FFmpeg is not installed")
		return
	}

	display := strings.TrimSpace(os.Getenv("DISPLAY"))
	if display == "" {
		m.setStatus("Error: X11 DISPLAY is not available")
		return
	}

	mode := m.ui.ComboRes.GetActiveID()
	cursor := m.ui.ChkCursor == nil || m.ui.ChkCursor.GetActive()
	minimize := m.ui.ChkMinimize != nil && m.ui.ChkMinimize.GetActive()

	go m.captureScreenshot(display, saveDir, mode, cursor, minimize && !recording, recording)
}

func (m *RecorderManager) captureScreenshot(
	display, saveDir, mode string,
	cursor, hideWindow, recording bool,
) {
	var (
		res    string
		offset string
		err    error
	)

	switch {
	case mode == "area":
		m.setStatus("Status: Drag mouse to select the screenshot area...")
		res, offset, err = selectAreaWithSlop()

	case mode == "window":
		m.setStatus("Status: Click the window you want to capture...")
		res, offset, err = selectWindowWithSlop()

	case strings.HasPrefix(mode, "monitor:"):
		mon, found := findMonitor(strings.TrimPrefix(mode, "monitor:"))
		if !found {
			err = fmt.Errorf("the selected monitor is not connected")
			break
		}

		res = fmt.Sprintf("%dx%d", mon.W&^1, mon.H&^1)
		offset = fmt.Sprintf("+%d,%d", mon.X, mon.Y)

	default:
		res, err = getNativeResolution()
	}

	if err != nil {
		m.setStatus("Error: " + err.Error())
		return
	}

	res = makeEvenResolution(res)

	if hideWindow {
		glib.IdleAdd(func() { m.ui.Window.Iconify() })
		time.Sleep(600 * time.Millisecond)
	}

	out := filepath.Join(
		saveDir,
		fmt.Sprintf("Screenshot_%s.png", time.Now().Format("2006-01-02_15-04-05")),
	)

	drawMouse := "0"
	if cursor {
		drawMouse = "1"
	}

	runErr := runFFmpeg([]string{
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-f", "x11grab",
		"-draw_mouse", drawMouse,
		"-video_size", res,
		"-i", display + offset,
		"-frames:v", "1",
		out,
	})

	if hideWindow {
		m.restoreWindow()
	}

	if runErr != nil {
		_ = os.Remove(out)
		m.setStatus("Error: Screenshot failed: " + lastLine(runErr.Error()))
		return
	}

	m.setStatus("Status: Screenshot saved: " + out)
}
