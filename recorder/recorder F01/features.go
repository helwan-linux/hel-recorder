
package main

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/gotk3/gotk3/glib"
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

	go func() {
		for remaining := seconds; remaining > 0; remaining-- {
			value := remaining

			glib.IdleAdd(func() {
				m.mu.Lock()
				active := m.countdownActive && m.countdownCancel == cancel
				m.mu.Unlock()

				if !active {
					return
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
					<-timer.C
				}
				return

			case <-timer.C:
			}
		}

		// All GTK operations and recording startup happen on the GTK main loop.
		glib.IdleAdd(func() {
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

// Magnifier

func (m *RecorderManager) ToggleMagnifier() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.magnifierCmd != nil {
		if m.magnifierCmd.Process != nil {
			_ = m.magnifierCmd.Process.Kill()
		}

		m.magnifierCmd = nil
		m.setStatus("Status: Magnifier disabled")
		return
	}

	path, err := exec.LookPath("xzoom")
	if err != nil {
		m.setStatus("Error: xzoom is not installed")
		return
	}

	cmd := exec.Command(path, "-mag", "2")
	if err := cmd.Start(); err != nil {
		m.setStatus("Error: Cannot start magnifier: " + err.Error())
		return
	}

	m.magnifierCmd = cmd
	m.setStatus("Status: Magnifier enabled (2x)")

	go func() {
		_ = cmd.Wait()

		m.mu.Lock()
		if m.magnifierCmd == cmd {
			m.magnifierCmd = nil
		}
		m.mu.Unlock()
	}()
}

func (m *RecorderManager) StopMagnifier() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.magnifierCmd != nil {
		if m.magnifierCmd.Process != nil {
			_ = m.magnifierCmd.Process.Kill()
		}
		m.magnifierCmd = nil
	}
}
