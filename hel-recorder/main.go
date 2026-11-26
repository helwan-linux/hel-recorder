// main.go
package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

func main() {
	gtk.Init(nil)

	ui, win := InitUI()
	manager := NewRecorderManager(ui)

	hotkeys, err := StartGlobalHotkeys(manager)

	if err != nil {
		ui.LblStatus.SetText(
			"Status: Global shortcuts unavailable: " +
				err.Error(),
		)
	}

	closing := false

	// quit stops any recording, waits until it is saved, then exits.
	// It is shared by the window close button and the tray "Quit" item.
	quit := func() {
		if closing {
			return
		}

		closing = true

		manager.StopRecording()
		manager.StopMagnifier()
		manager.StopMouseEffects()

		go func() {
			manager.WaitForStop()

			if hotkeys != nil {
				hotkeys.Stop()
			}

			glib.IdleAdd(func() {
				ShutdownTray()
				win.Destroy()
				gtk.MainQuit()
			})
		}()
	}

	win.Connect(
		"delete-event",
		func(
			_ *gtk.Window,
			_ *gdk.Event,
		) bool {
			quit()
			return true
		},
	)

	SetupTray(manager, ui, quit)

	manager.connectMouseEffectsCheckbox()
	ui.BtnScreenshot.Connect("clicked", manager.TakeScreenshot)

	// A terminal Ctrl+C, "kill" or a desktop shutdown must stop the recording
	// and save it instead of leaving unfinished part files behind.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		<-signals
		glib.IdleAdd(quit)
	}()

	win.ShowAll()

	// Run once the main loop is up: warn about Wayland and offer to recover
	// recordings left behind by a crash or a power loss.
	glib.IdleAdd(func() {
		warnIfWayland(win)
		manager.offerRecovery()
	})

	gtk.Main()
}
