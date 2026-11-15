package main

import (
	"os"
	"path/filepath"

	"github.com/gotk3/gotk3/gdk"
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
	BtnHelp     *gtk.Button
	BtnAbout    *gtk.Button
}

func InitUI() (*AppUI, *gtk.Window) {
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		panic(err)
	}

	win.SetTitle("Helwan Screen Recorder")
	win.SetDefaultSize(460, 400)
	win.SetPosition(gtk.WIN_POS_CENTER)
	win.SetResizable(false)

	iconPath := "assets/icon.png"
	if _, err := os.Stat(iconPath); err == nil {
		win.SetIconFromFile(iconPath)
	} else {
		win.SetIconName("video-record")
	}

	vbox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 12)
	vbox.SetMarginTop(14)
	vbox.SetMarginBottom(14)
	vbox.SetMarginStart(14)
	vbox.SetMarginEnd(14)
	win.Add(vbox)

	topBar, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 6)
	btnHelp, _ := gtk.ButtonNewWithLabel("Help")
	btnAbout, _ := gtk.ButtonNewWithLabel("About")
	topBar.PackEnd(btnAbout, false, false, 0)
	topBar.PackEnd(btnHelp, false, false, 0)
	vbox.PackStart(topBar, false, false, 0)

	lblTimer, _ := gtk.LabelNew("00:00:00")
	lblTimer.SetMarkup("<span font='26' weight='bold' color='#3584e4'>00:00:00</span>")
	vbox.PackStart(lblTimer, false, false, 0)

	lblStatus, _ := gtk.LabelNew("Status: Ready (Ctrl+R: Start/Pause, Ctrl+S: Stop)")
	vbox.PackStart(lblStatus, false, false, 0)

	sep1, _ := gtk.SeparatorNew(gtk.ORIENTATION_HORIZONTAL)
	vbox.PackStart(sep1, false, false, 4)

	grid, _ := gtk.GridNew()
	grid.SetRowSpacing(8)
	grid.SetColumnSpacing(10)
	vbox.PackStart(grid, false, false, 0)

	lblFPS, _ := gtk.LabelNew("Frame Rate:")
	lblFPS.SetHAlign(gtk.ALIGN_START)
	comboFPS, _ := gtk.ComboBoxTextNew()
	comboFPS.Append("30", "30 FPS")
	comboFPS.Append("60", "60 FPS")
	comboFPS.SetActiveID("30")
	grid.Attach(lblFPS, 0, 0, 1, 1)
	grid.Attach(comboFPS, 1, 0, 1, 1)

	lblRes, _ := gtk.LabelNew("Resolution:")
	lblRes.SetHAlign(gtk.ALIGN_START)
	comboRes, _ := gtk.ComboBoxTextNew()
	comboRes.Append("auto", "Native (Auto)")
	comboRes.Append("1920x1080", "1080p")
	comboRes.Append("1280x720", "720p")
	comboRes.SetActiveID("auto")
	grid.Attach(lblRes, 0, 1, 1, 1)
	grid.Attach(comboRes, 1, 1, 1, 1)

	chkAudio, _ := gtk.CheckButtonNewWithLabel("Record Audio (Mic/System)")
	chkAudio.SetActive(true)
	grid.Attach(chkAudio, 0, 2, 2, 1)

	hboxFolder, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	lblFolder, _ := gtk.LabelNew("Save Dir:")
	fileChooser, _ := gtk.FileChooserButtonNew("Select Directory", gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER)

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

	btnStart, _ := gtk.ButtonNewWithLabel("Start (Ctrl+R)")
	btnStop, _ := gtk.ButtonNewWithLabel("Stop (Ctrl+S)")
	btnStop.SetSensitive(false)

	hboxButtons.PackStart(btnStart, true, true, 0)
	hboxButtons.PackStart(btnStop, true, true, 0)

	btnHelp.Connect("clicked", func() {
		showHelpDialog(win)
	})
	btnAbout.Connect("clicked", func() {
		showAboutDialog(win)
	})

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
		BtnHelp:     btnHelp,
		BtnAbout:    btnAbout,
	}, win
}

func showHelpDialog(parent *gtk.Window) {
	dialog := gtk.MessageDialogNew(parent, gtk.DIALOG_MODAL, gtk.MESSAGE_INFO, gtk.BUTTONS_OK, 
		"Keyboard Shortcuts:\n\n• Ctrl + R: Start / Pause / Resume recording\n• Ctrl + S: Stop and save recording\n\nFiles are saved automatically as MP4 in the selected directory.")
	dialog.SetTitle("Help - Helwan Recorder")
	dialog.Run()
	dialog.Destroy()
}

func showAboutDialog(parent *gtk.Window) {
	dialog := gtk.AboutDialogNew()
	dialog.SetTransientFor(parent)
	dialog.SetProgramName("Helwan Screen Recorder")
	dialog.SetVersion("1.0.0")
	dialog.SetComments("Professional high-performance screen recorder for Arch Linux.")
	dialog.SetWebsite("https://github.com/helwan-linux")
	dialog.Run()
	dialog.Destroy()
}

func SetupKeybindings(win *gtk.Window, manager *RecorderManager) {
	win.Connect("key-press-event", func(_ *gtk.Window, event *gdk.Event) bool {
		keyEvent := gdk.EventKeyNewFrom(event)
		keyval := keyEvent.KeyVal()
		state := keyEvent.State()

		if (state&uint(gdk.CONTROL_MASK)) != 0 && (keyval == gdk.KEY_r || keyval == gdk.KEY_R) {
			manager.ToggleRecordPause()
			return true
		}
		if (state&uint(gdk.CONTROL_MASK)) != 0 && (keyval == gdk.KEY_s || keyval == gdk.KEY_S) {
			manager.StopRecording()
			return true
		}
		return false
	})
}
