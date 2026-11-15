
package main

import (
	"os"
	"path/filepath"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type AppUI struct {
	Window           *gtk.Window
	BtnStart         *gtk.Button
	BtnStop          *gtk.Button
	LblTimer         *gtk.Label
	LblStatus        *gtk.Label
	ChkAudio         *gtk.CheckButton
	ComboMic         *gtk.ComboBoxText
	ChkSystemAudio   *gtk.CheckButton
	ComboSystemAudio *gtk.ComboBoxText
	ComboFPS         *gtk.ComboBoxText
	ComboRes         *gtk.ComboBoxText
	FileChooser      *gtk.FileChooserButton
	BtnHelp          *gtk.Button
	BtnAbout         *gtk.Button
}

func InitUI() (*AppUI, *gtk.Window) {
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		panic(err)
	}

	win.SetTitle("Helwan Screen Recorder")
	win.SetDefaultSize(500, 520)
	win.SetPosition(gtk.WIN_POS_CENTER)
	win.SetResizable(false)

	iconPath := "/usr/share/hel-recorder/icon.png"
	if _, err := os.Stat(iconPath); err == nil {
		_ = win.SetIconFromFile(iconPath)
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
	lblTimer.SetMarkup(
		"<span font='28' weight='bold' color='#3584e4'>00:00:00</span>",
	)
	vbox.PackStart(lblTimer, false, false, 0)

	lblStatus, _ := gtk.LabelNew("Status: Ready")
	lblStatus.SetLineWrap(true)
	vbox.PackStart(lblStatus, false, false, 0)

	sep, _ := gtk.SeparatorNew(gtk.ORIENTATION_HORIZONTAL)
	vbox.PackStart(sep, false, false, 4)

	grid, _ := gtk.GridNew()
	grid.SetRowSpacing(10)
	grid.SetColumnSpacing(10)
	vbox.PackStart(grid, false, false, 0)

	// Frame rate
	lblFPS, _ := gtk.LabelNew("Frame Rate:")
	lblFPS.SetHAlign(gtk.ALIGN_START)

	comboFPS, _ := gtk.ComboBoxTextNew()
	comboFPS.Append("30", "30 FPS")
	comboFPS.Append("60", "60 FPS")
	comboFPS.SetActiveID("30")

	grid.Attach(lblFPS, 0, 0, 1, 1)
	grid.Attach(comboFPS, 1, 0, 1, 1)

	// Screen resolution
	lblRes, _ := gtk.LabelNew("Screen:")
	lblRes.SetHAlign(gtk.ALIGN_START)

	comboRes, _ := gtk.ComboBoxTextNew()
	comboRes.Append("auto", "Full Screen (Native)")
	comboRes.SetActiveID("auto")

	grid.Attach(lblRes, 0, 1, 1, 1)
	grid.Attach(comboRes, 1, 1, 1, 1)

	// Microphone
	chkAudio, _ := gtk.CheckButtonNewWithLabel("Record Microphone")
	chkAudio.SetActive(true)
	grid.Attach(chkAudio, 0, 2, 2, 1)

	lblMic, _ := gtk.LabelNew("Microphone:")
	lblMic.SetHAlign(gtk.ALIGN_START)

	comboMic, _ := gtk.ComboBoxTextNew()
	comboMic.Append("default", "Default Microphone")

	microphones, micErr := GetMicrophoneSources()
	for _, source := range microphones {
		if source == "" || source == "default" {
			continue
		}
		comboMic.Append(source, source)
	}
	comboMic.SetActiveID("default")

	grid.Attach(lblMic, 0, 3, 1, 1)
	grid.Attach(comboMic, 1, 3, 1, 1)

	chkAudio.Connect("toggled", func() {
		comboMic.SetSensitive(chkAudio.GetActive())
	})
	comboMic.SetSensitive(true)

	// System audio
	chkSystemAudio, _ := gtk.CheckButtonNewWithLabel("Record System Audio")
	grid.Attach(chkSystemAudio, 0, 4, 2, 1)

	lblSystem, _ := gtk.LabelNew("Audio Output:")
	lblSystem.SetHAlign(gtk.ALIGN_START)

	comboSystemAudio, _ := gtk.ComboBoxTextNew()

	systemSources, systemErr := GetSystemAudioSources()
	for _, source := range systemSources {
		if source == "" {
			continue
		}
		comboSystemAudio.Append(source, source)
	}

	if len(systemSources) > 0 {
		comboSystemAudio.SetActive(0)
	} else {
		chkSystemAudio.SetSensitive(false)
		comboSystemAudio.SetSensitive(false)
	}

	grid.Attach(lblSystem, 0, 5, 1, 1)
	grid.Attach(comboSystemAudio, 1, 5, 1, 1)

	chkSystemAudio.Connect("toggled", func() {
		comboSystemAudio.SetSensitive(
			chkSystemAudio.GetActive() && len(systemSources) > 0,
		)
	})

	comboSystemAudio.SetSensitive(false)

	if micErr != nil && systemErr != nil {
		lblStatus.SetText("Status: Ready - Audio sources unavailable")
	}

	// Save directory
	hboxFolder, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	lblFolder, _ := gtk.LabelNew("Save Directory:")

	fileChooser, _ := gtk.FileChooserButtonNew(
		"Select Directory",
		gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER,
	)

	homeDir, _ := os.UserHomeDir()
	videosDir := filepath.Join(homeDir, "Videos")

	if info, err := os.Stat(videosDir); err == nil && info.IsDir() {
		_, _ = fileChooser.SetCurrentFolder(videosDir)
	} else {
		_, _ = fileChooser.SetCurrentFolder(homeDir)
	}

	hboxFolder.PackStart(lblFolder, false, false, 0)
	hboxFolder.PackStart(fileChooser, true, true, 0)
	vbox.PackStart(hboxFolder, false, false, 0)

	// Recording buttons
	hboxButtons, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 12)
	hboxButtons.SetHomogeneous(true)

	btnStart, _ := gtk.ButtonNewWithLabel("Start (Ctrl+R)")
	btnStop, _ := gtk.ButtonNewWithLabel("Stop (Ctrl+S)")
	btnStop.SetSensitive(false)

	hboxButtons.PackStart(btnStart, true, true, 0)
	hboxButtons.PackStart(btnStop, true, true, 0)
	vbox.PackEnd(hboxButtons, false, false, 0)

	btnHelp.Connect("clicked", func() {
		showHelpDialog(win)
	})

	btnAbout.Connect("clicked", func() {
		showAboutDialog(win)
	})

	ui := &AppUI{
		Window:           win,
		BtnStart:         btnStart,
		BtnStop:          btnStop,
		LblTimer:         lblTimer,
		LblStatus:        lblStatus,
		ChkAudio:         chkAudio,
		ComboMic:         comboMic,
		ChkSystemAudio:   chkSystemAudio,
		ComboSystemAudio: comboSystemAudio,
		ComboFPS:         comboFPS,
		ComboRes:         comboRes,
		FileChooser:      fileChooser,
		BtnHelp:          btnHelp,
		BtnAbout:         btnAbout,
	}

	return ui, win
}

func showHelpDialog(parent *gtk.Window) {
	dialog := gtk.MessageDialogNew(
		parent,
		gtk.DIALOG_MODAL,
		gtk.MESSAGE_INFO,
		gtk.BUTTONS_OK,
		"Keyboard Shortcuts:\n\n"+
			"Ctrl + R: Start / Pause / Resume\n"+
			"Ctrl + S: Stop and save\n\n"+
			"Microphone and system audio can be recorded independently.\n"+
			"Recordings are saved as MP4.",
	)

	dialog.SetTitle("Help - Helwan Recorder")
	dialog.Run()
	dialog.Destroy()
}

func showAboutDialog(parent *gtk.Window) {
	dialog := gtk.AboutDialogNew()
	dialog.SetTransientFor(parent)
	dialog.SetProgramName("Helwan Screen Recorder")
	dialog.SetVersion("1.0.0")
	dialog.SetComments("Screen and audio recorder for Helwan Linux.")
	dialog.SetWebsite("https://github.com/helwan-linux")
	dialog.Run()
	dialog.Destroy()
}

func SetupKeybindings(win *gtk.Window, manager *RecorderManager) {
	win.Connect("key-press-event", func(
		_ *gtk.Window,
		event *gdk.Event,
	) bool {
		keyEvent := gdk.EventKeyNewFrom(event)
		keyval := keyEvent.KeyVal()
		state := keyEvent.State()

		if state&uint(gdk.CONTROL_MASK) != 0 {
			switch keyval {
			case gdk.KEY_r, gdk.KEY_R:
				manager.ToggleRecordPause()
				return true
			case gdk.KEY_s, gdk.KEY_S:
				manager.StopRecording()
				return true
			}
		}

		return false
	})
}
