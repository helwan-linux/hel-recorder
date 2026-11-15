package main

import (
	"os"
	"path/filepath"

	"github.com/gotk3/gotk3/gtk"
)

type AppUI struct {
	Window      *gtk.Window
	BtnStart    *gtk.Button
	BtnStop     *gtk.Button
	LblTimer    *gtk.Label
	LblStatus   *gtk.Label
	ChkAudio    *gtk.CheckButton
	ComboFPS    *gtk.ComboBoxText
	ComboRes    *gtk.ComboBoxText
	FileChooser *gtk.FileChooserButton
}

func InitUI() (*AppUI, *gtk.Window) {
	gtk.Init(nil)

	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		panic(err)
	}

	win.SetTitle("Helwan Screen Recorder")
	win.SetDefaultSize(440, 340)
	win.SetPosition(gtk.WIN_POS_CENTER)
	win.SetResizable(false)

	vbox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 14)
	vbox.SetMarginTop(18)
	vbox.SetMarginBottom(18)
	vbox.SetMarginStart(18)
	vbox.SetMarginEnd(18)
	win.Add(vbox)

	lblTimer, _ := gtk.LabelNew("00:00:00")
	lblTimer.SetMarkup("<span font='28' weight='bold' color='#3584e4'>00:00:00</span>")
	vbox.PackStart(lblTimer, false, false, 0)

	lblStatus, _ := gtk.LabelNew("Status: Ready")
	vbox.PackStart(lblStatus, false, false, 0)

	sep1, _ := gtk.SeparatorNew(gtk.ORIENTATION_HORIZONTAL)
	vbox.PackStart(sep1, false, false, 5)

	grid, _ := gtk.GridNew()
	grid.SetRowSpacing(10)
	grid.SetColumnSpacing(12)
	vbox.PackStart(grid, false, false, 0)

	lblFPS, _ := gtk.LabelNew("Frame Rate:")
	lblFPS.SetHAlign(gtk.ALIGN_START)
	comboFPS, _ := gtk.ComboBoxTextNew()
	comboFPS.Append("30", "30 FPS (Balanced)")
	comboFPS.Append("60", "60 FPS (Smooth)")
	comboFPS.SetActiveID("30")
	grid.Attach(lblFPS, 0, 0, 1, 1)
	grid.Attach(comboFPS, 1, 0, 1, 1)

	lblRes, _ := gtk.LabelNew("Resolution:")
	lblRes.SetHAlign(gtk.ALIGN_START)
	comboRes, _ := gtk.ComboBoxTextNew()
	comboRes.Append("auto", "Native (Auto)")
	comboRes.Append("1920x1080", "1080p (Full HD)")
	comboRes.Append("1280x720", "720p (HD)")
	comboRes.SetActiveID("auto")
	grid.Attach(lblRes, 0, 1, 1, 1)
	grid.Attach(comboRes, 1, 1, 1, 1)

	chkAudio, _ := gtk.CheckButtonNewWithLabel("Record Audio (Microphone / System)")
	chkAudio.SetActive(true)
	grid.Attach(chkAudio, 0, 2, 2, 1)

	hboxFolder, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	lblFolder, _ := gtk.LabelNew("Save Directory:")
	fileChooser, _ := gtk.FileChooserButtonNew("Select Output Directory", gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER)

	homeDir, _ := os.UserHomeDir()
	videosDir := filepath.Join(homeDir, "Videos")
	if _, err := os.Stat(videosDir); err == nil {
		fileChooser.SetCurrentFolder(videosDir)
	} else {
		fileChooser.SetCurrentFolder(homeDir)
	}

	hboxFolder.PackStart(lblFolder, false, false, 0)
	hboxFolder.PackStart(fileChooser, true, true, 0)
	vbox.PackStart(hboxFolder, false, false, 0)

	hboxButtons, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 12)
	hboxButtons.SetHomogeneous(true)
	vbox.PackEnd(hboxButtons, false, false, 0)

	btnStart, _ := gtk.ButtonNewWithLabel("Start Recording")
	btnStop, _ := gtk.ButtonNewWithLabel("Stop & Save")
	btnStop.SetSensitive(false)

	hboxButtons.PackStart(btnStart, true, true, 0)
	hboxButtons.PackStart(btnStop, true, true, 0)

	return &AppUI{
		Window:      win,
		BtnStart:    btnStart,
		BtnStop:     btnStop,
		LblTimer:    lblTimer,
		LblStatus:   lblStatus,
		ChkAudio:    chkAudio,
		ComboFPS:    comboFPS,
		ComboRes:    comboRes,
		FileChooser: fileChooser,
	}, win
}
