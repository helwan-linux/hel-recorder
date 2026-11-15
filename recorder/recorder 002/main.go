package main

import (
	"github.com/gotk3/gotk3/gtk"
)

func main() {
	gtk.Init(nil)

	ui, win := InitUI()
	manager := NewRecorderManager(ui)

	SetupKeybindings(win, manager)

	win.Connect("destroy", func() {
		if manager.isRecording {
			manager.StopRecording()
		}
		gtk.MainQuit()
	})

	win.ShowAll()
	gtk.Main()
}
