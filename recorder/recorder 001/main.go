package main

import (
	"github.com/gotk3/gotk3/gtk"
)

func main() {
	ui, win := InitUI()
	_ = NewRecorderManager(ui)

	win.Connect("destroy", func() {
		gtk.MainQuit()
	})

	win.ShowAll()
	gtk.Main()
}
