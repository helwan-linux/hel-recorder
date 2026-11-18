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

	win.Connect("delete-event", func(
		_ *gtk.Window,
		_ *gdk.Event,
	) bool {
		if closing {
			return true
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
				win.Destroy()
				gtk.MainQuit()
			})
		}()

		return true
	})

	win.ShowAll()
	gtk.Main()
}
