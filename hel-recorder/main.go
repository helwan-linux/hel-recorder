// main.go
package main

import (
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
		ui.LblStatus.SetText("Status: Global shortcuts unavailable: " + err.Error())
	}

	closing := false

	// quit stops any recording, waits until it is saved, then exits. It is
	// shared by the window close button and the tray "Quit" item.
	quit := func() {
		if closing {
			return
		}

		closing = true

		manager.StopRecording()
		manager.StopMagnifier()

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

	win.Connect("delete-event", func(
		_ *gtk.Window,
		_ *gdk.Event,
	) bool {
		quit()
		return true
	})

	SetupTray(manager, ui, quit)

	win.ShowAll()
	gtk.Main()
}
