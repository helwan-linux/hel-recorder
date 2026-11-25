// features.go
package main

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

// Floating OSD Status Pill (Mini Control Bar)
var (
	osdWindow *gtk.Window
	osdLabel  *gtk.Label

	// Last position chosen by the user; reused the next time the OSD opens.
	osdPosSaved bool
	osdLastX    int
	osdLastY    int
)

// makeWindowDraggable lets the user move a borderless popup window by
// dragging it with the left mouse button. Popup windows are not managed by the
// window manager, so the move is done manually. onMoved is called when the
// drag ends.
func makeWindowDraggable(win *gtk.Window, onMoved func(x, y int)) {
	var (
		dragging   bool
		startRootX float64
		startRootY float64
		startWinX  int
		startWinY  int
	)

	win.AddEvents(int(
		gdk.BUTTON_PRESS_MASK |
			gdk.BUTTON_RELEASE_MASK |
			gdk.POINTER_MOTION_MASK,
	))

	win.Connect("button-press-event", func(w *gtk.Window, ev *gdk.Event) bool {
		btn := gdk.EventButtonNewFromEvent(ev)
		if btn.Button() != 1 {
			return false
		}

		dragging = true
		startRootX, startRootY = btn.XRoot(), btn.YRoot()
		startWinX, startWinY = w.GetPosition()

		return true
	})

	win.Connect("motion-notify-event", func(w *gtk.Window, ev *gdk.Event) bool {
		if !dragging {
			return false
		}

		motion := gdk.EventMotionNewFromEvent(ev)
		rootX, rootY := motion.MotionValRoot()

		w.Move(
			startWinX+int(rootX-startRootX),
			startWinY+int(rootY-startRootY),
		)

		return true
	})

	win.Connect("button-release-event", func(w *gtk.Window, ev *gdk.Event) bool {
		if !dragging {
			return false
		}

		dragging = false

		if onMoved != nil {
			x, y := w.GetPosition()
			onMoved(x, y)
		}

		return true
	})
}

func (m *RecorderManager) showFloatingOSD() {
	glib.IdleAdd(func() {
		if osdWindow != nil {
			return
		}

		win, err := gtk.WindowNew(gtk.WINDOW_POPUP)
		if err != nil {
			return
		}

		// Keep the compact status bar at the upper-right edge so it
		// does not cover the center of the user's workspace.
		win.SetDecorated(false)
		win.SetResizable(false)
		win.SetDefaultSize(250, 42)
		win.SetKeepAbove(true)
		win.SetSkipTaskbarHint(true)
		win.SetSkipPagerHint(true)

		screen := win.GetScreen()
		if screen != nil {
			visual, _ := screen.GetRGBAVisual()
			if visual != nil {
				win.SetVisual(visual)
			}
		}

		box, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
		box.SetMarginTop(6)
		box.SetMarginBottom(6)
		box.SetMarginStart(12)
		box.SetMarginEnd(12)

		lbl, _ := gtk.LabelNew("🔴 REC 00:00:00")
		lbl.SetMarkup("<span font='12' weight='bold' color='#ff3333'>🔴 REC 00:00:00</span>")
		box.PackStart(lbl, false, false, 0)
		osdLabel = lbl

		btnPause, _ := gtk.ButtonNewWithLabel("⏸")
		btnPause.Connect("clicked", func() {
			m.PauseResumeRecording()
		})
		box.PackStart(btnPause, false, false, 0)

		btnStop, _ := gtk.ButtonNewWithLabel("⏹")
		btnStop.Connect("clicked", func() {
			m.StopRecording()
		})
		box.PackStart(btnStop, false, false, 0)

		win.Add(box)
		win.ShowAll()

		if osdPosSaved {
			win.Move(osdLastX, osdLastY)
		} else {
			// Move after showing so GTK has applied the requested window size.
			if screen != nil {
				if root, err := screen.GetRootWindow(); err == nil && root != nil {
					width := root.WindowGetWidth()
					x := width - 270
					if x < 12 {
						x = 12
					}
					win.Move(x, 18)
				} else {
					win.SetPosition(gtk.WIN_POS_CENTER)
				}
			} else {
				win.SetPosition(gtk.WIN_POS_CENTER)
			}
		}

		win.SetTooltipText("Drag to move")

		makeWindowDraggable(win, func(x, y int) {
			osdPosSaved = true
			osdLastX, osdLastY = x, y
		})

		osdWindow = win
	})
}

func updateFloatingOSDText(timeStr string, isPaused bool) {
	glib.IdleAdd(func() {
		updateTrayTooltip(timeStr, isPaused)

		if osdLabel == nil {
			return
		}

		if isPaused {
			osdLabel.SetMarkup(
				fmt.Sprintf(
					"<span font='12' weight='bold' color='#f6d32d'>⏸ PAUSED %s</span>",
					timeStr,
				),
			)
		} else {
			osdLabel.SetMarkup(
				fmt.Sprintf(
					"<span font='12' weight='bold' color='#ff3333'>🔴 REC %s</span>",
					timeStr,
				),
			)
		}
	})
}

func hideFloatingOSD() {
	glib.IdleAdd(func() {
		trayApplyState("ready")

		if osdWindow != nil {
			osdWindow.Destroy()
			osdWindow = nil
			osdLabel = nil
		}
	})
}

// Countdown

func (m *RecorderManager) selectedCountdown() int {
	if m.ui.ComboCountdown == nil {
		return 0
	}

	switch m.ui.ComboCountdown.GetActiveID() {
	case "3":
		return 3
	case "5":
		return 5
	case "10":
		return 10
	default:
		return 0
	}
}

func (m *RecorderManager) startCountdownLocked(seconds int) {
	cancel := make(chan struct{})

	m.countdownCancel = cancel
	m.countdownActive = true

	m.ui.BtnStart.SetLabel("Cancel Countdown (Ctrl+R)")
	m.ui.BtnStop.SetSensitive(true)
	m.ui.ComboCountdown.SetSensitive(false)

	var countdownWin *gtk.Window
	var countdownLbl *gtk.Label

	glib.IdleAdd(func() {
		win, err := gtk.WindowNew(gtk.WINDOW_POPUP)
		if err == nil {
			win.SetPosition(gtk.WIN_POS_CENTER)
			win.SetKeepAbove(true)
			win.SetAppPaintable(true)

			screen := win.GetScreen()
			if screen != nil {
				visual, _ := screen.GetRGBAVisual()
				if visual != nil {
					win.SetVisual(visual)
				}
			}

			label, err := gtk.LabelNew("")
			if err == nil {
				label.SetMarkup(
					fmt.Sprintf(
						"<span font='80' weight='bold' color='#ff3333'>%02d</span>",
						seconds,
					),
				)
				win.Add(label)
				countdownLbl = label
			}

			win.SetSizeRequest(300, 200)
			win.ShowAll()
			countdownWin = win
		}
	})

	go func() {
		for remaining := seconds; remaining > 0; remaining-- {
			value := remaining

			glib.IdleAdd(func() {
				m.mu.Lock()
				active := m.countdownActive && m.countdownCancel == cancel
				m.mu.Unlock()

				if !active {
					if countdownWin != nil {
						countdownWin.Destroy()
					}
					return
				}

				if countdownLbl != nil {
					countdownLbl.SetMarkup(
						fmt.Sprintf(
							"<span font='80' weight='bold' color='#ff3333'>%02d</span>",
							value,
						),
					)
				}

				m.ui.LblTimer.SetMarkup(
					fmt.Sprintf(
						"<span font='28' weight='bold' color='#e01b24'>%02d</span>",
						value,
					),
				)

				m.ui.LblStatus.SetText(
					fmt.Sprintf(
						"Status: Recording starts in %d seconds",
						value,
					),
				)
			})

			timer := time.NewTimer(time.Second)

			select {
			case <-cancel:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}

				glib.IdleAdd(func() {
					if countdownWin != nil {
						countdownWin.Destroy()
					}
				})
				return

			case <-timer.C:
			}
		}

		glib.IdleAdd(func() {
			if countdownWin != nil {
				countdownWin.Destroy()
			}

			m.mu.Lock()
			defer m.mu.Unlock()

			if !m.countdownActive || m.countdownCancel != cancel {
				return
			}

			m.countdownActive = false
			m.countdownCancel = nil

			m.ui.BtnStart.SetLabel("Start (Ctrl+R)")
			m.ui.BtnStop.SetSensitive(false)
			m.ui.ComboCountdown.SetSensitive(true)

			m.ui.LblTimer.SetMarkup(
				"<span font='28' weight='bold' color='#3584e4'>00:00:00</span>",
			)

			m.startRecordingLocked()
		})
	}()
}

func (m *RecorderManager) cancelCountdownLocked() {
	if !m.countdownActive {
		return
	}

	if m.countdownCancel != nil {
		close(m.countdownCancel)
	}

	m.countdownCancel = nil
	m.countdownActive = false

	glib.IdleAdd(func() {
		m.ui.BtnStart.SetLabel("Start (Ctrl+R)")
		m.ui.BtnStop.SetSensitive(false)
		m.ui.ComboCountdown.SetSensitive(true)

		m.ui.LblTimer.SetMarkup(
			"<span font='28' weight='bold' color='#3584e4'>00:00:00</span>",
		)

		m.ui.LblStatus.SetText("Status: Countdown cancelled")
	})
}

// Magnifier - KMag

func (m *RecorderManager) ToggleMagnifier() {
	m.mu.Lock()

	if m.magnifierCmd != nil {
		cmd := m.magnifierCmd
		m.magnifierCmd = nil

		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}

		m.setStatus("Status: Magnifier disabled")
		m.mu.Unlock()
		return
	}

	path, err := exec.LookPath("kmag")
	if err != nil {
		m.setStatus("Error: KMag is not installed")
		m.mu.Unlock()
		return
	}

	cmd := exec.Command(path)

	if err := cmd.Start(); err != nil {
		m.setStatus("Error: Cannot start KMag: " + err.Error())
		m.mu.Unlock()
		return
	}

	m.magnifierCmd = cmd
	m.setStatus("Status: Magnifier enabled")
	m.mu.Unlock()

	go func() {
		_ = cmd.Wait()

		m.mu.Lock()
		if m.magnifierCmd == cmd {
			m.magnifierCmd = nil
			m.setStatus("Status: Magnifier disabled")
		}
		m.mu.Unlock()
	}()
}

func (m *RecorderManager) StopMagnifier() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.magnifierCmd != nil {
		cmd := m.magnifierCmd
		m.magnifierCmd = nil

		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
}

// Keystrokes Overlay - Screenkey (Optimized)

func (m *RecorderManager) startScreenkeyLocked() {
	if m.screenkeyCmd != nil {
		return
	}

	path, err := exec.LookPath("screenkey")
	if err != nil {
		m.setStatus("Warning: screenkey is not installed")
		return
	}

	cmd := exec.Command(path, "--no-systray", "-t", "1.2")
	if err := cmd.Start(); err != nil {
		return
	}

	m.screenkeyCmd = cmd

	go func(c *exec.Cmd) {
		_ = c.Wait()

		m.mu.Lock()
		if m.screenkeyCmd == c {
			m.screenkeyCmd = nil
		}
		m.mu.Unlock()
	}(cmd)
}

func (m *RecorderManager) stopScreenkeyLocked() {
	if m.screenkeyCmd != nil {
		cmd := m.screenkeyCmd
		m.screenkeyCmd = nil

		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
}

