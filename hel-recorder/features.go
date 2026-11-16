//features.go
package main

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

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

	// إنشاء نافذة عائمة ثابتة ومستقلة للعداد في منتصف الشاشة
	var osdWindow *gtk.Window
	var osdLabel *gtk.Label

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
				label.SetMarkup(fmt.Sprintf("<span font='80' weight='bold' color='#ff3333'>%02d</span>", seconds))
				win.Add(label)
				osdLabel = label
			}

			win.SetSizeRequest(300, 200)
			win.ShowAll()
			osdWindow = win
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
					if osdWindow != nil {
						osdWindow.Destroy()
					}
					return
				}

				// تحديث النص داخل نفس النافذة العائمة بسلاسة
				if osdLabel != nil {
					osdLabel.SetMarkup(fmt.Sprintf("<span font='80' weight='bold' color='#ff3333'>%02d</span>", value))
				}

				m.ui.LblTimer.SetMarkup(
					fmt.Sprintf(
						"<span font='28' weight='bold' color='#e01b24'>%02d</span>",
						value,
					),
				)

				m.ui.LblStatus.SetText(
					fmt.Sprintf("Status: Recording starts in %d seconds", value),
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
					if osdWindow != nil {
						osdWindow.Destroy()
					}
				})
				return

			case <-timer.C:
			}
		}

		glib.IdleAdd(func() {
			if osdWindow != nil {
				osdWindow.Destroy()
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
