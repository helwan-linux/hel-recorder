
package main

import (
	"os"
	"path/filepath"

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
	ComboCountdown   *gtk.ComboBoxText
	FileChooser      *gtk.FileChooserButton
	BtnHelp          *gtk.Button
	BtnAbout         *gtk.Button
}

func setRecorderIcon(win *gtk.Window) {
	paths := []string{}

	exePath, err := os.Executable()
	if err == nil {
		paths = append(paths,
			filepath.Join(filepath.Dir(exePath), "assets", "icon.png"),
		)
	}

	cwd, err := os.Getwd()
	if err == nil {
		paths = append(paths,
			filepath.Join(cwd, "assets", "icon.png"),
		)
	}

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}

		if win.SetIconFromFile(path) == nil {
			return
		}
	}
}

func InitUI() (*AppUI, *gtk.Window) {
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		panic(err)
	}

	win.SetTitle("Helwan Screen Recorder")
	win.SetDefaultSize(500, 570)
	win.SetPosition(gtk.WIN_POS_CENTER)
	win.SetResizable(false)
	setRecorderIcon(win)

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

	lblFPS, _ := gtk.LabelNew("Frame Rate:")
	lblFPS.SetHAlign(gtk.ALIGN_START)
	comboFPS, _ := gtk.ComboBoxTextNew()
	comboFPS.Append("30", "30 FPS")
	comboFPS.Append("60", "60 FPS")
	comboFPS.SetActiveID("30")
	grid.Attach(lblFPS, 0, 0, 1, 1)
	grid.Attach(comboFPS, 1, 0, 1, 1)

	lblRes, _ := gtk.LabelNew("Screen:")
	lblRes.SetHAlign(gtk.ALIGN_START)
	comboRes, _ := gtk.ComboBoxTextNew()
	comboRes.Append("auto", "Full Screen (Native)")
	comboRes.SetActiveID("auto")
	grid.Attach(lblRes, 0, 1, 1, 1)
	grid.Attach(comboRes, 1, 1, 1, 1)

	lblCountdown, _ := gtk.LabelNew("Start Delay:")
	lblCountdown.SetHAlign(gtk.ALIGN_START)
	comboCountdown, _ := gtk.ComboBoxTextNew()
	comboCountdown.Append("0", "No Countdown")
	comboCountdown.Append("3", "3 Seconds")
	comboCountdown.Append("5", "5 Seconds")
	comboCountdown.Append("10", "10 Seconds")
	comboCountdown.SetActiveID("0")
	grid.Attach(lblCountdown, 0, 2, 1, 1)
	grid.Attach(comboCountdown, 1, 2, 1, 1)

	chkAudio, _ := gtk.CheckButtonNewWithLabel("Record Microphone")
	chkAudio.SetActive(true)
	grid.Attach(chkAudio, 0, 3, 2, 1)

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
	grid.Attach(lblMic, 0, 4, 1, 1)
	grid.Attach(comboMic, 1, 4, 1, 1)

	chkAudio.Connect("toggled", func() {
		comboMic.SetSensitive(chkAudio.GetActive())
	})
	comboMic.SetSensitive(true)

	chkSystemAudio, _ := gtk.CheckButtonNewWithLabel("Record System Audio")
	grid.Attach(chkSystemAudio, 0, 5, 2, 1)

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

	grid.Attach(lblSystem, 0, 6, 1, 1)
	grid.Attach(comboSystemAudio, 1, 6, 1, 1)

	chkSystemAudio.Connect("toggled", func() {
		comboSystemAudio.SetSensitive(
			chkSystemAudio.GetActive() && len(systemSources) > 0,
		)
	})
	comboSystemAudio.SetSensitive(false)

	if micErr != nil && systemErr != nil {
		lblStatus.SetText("Status: Ready - Audio sources unavailable")
	}

	hboxFolder, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	lblFolder, _ := gtk.LabelNew("Save Directory:")

	fileChooser, _ := gtk.FileChooserButtonNew(
		"Select Directory",
		gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER,
	)

	homeDir, _ := os.UserHomeDir()
	videosDir := filepath.Join(homeDir, "Videos")

	if info, err := os.Stat(videosDir); err == nil && info.IsDir() {
		_ = fileChooser.SetCurrentFolder(videosDir)
	} else {
		_ = fileChooser.SetCurrentFolder(homeDir)
	}

	hboxFolder.PackStart(lblFolder, false, false, 0)
	hboxFolder.PackStart(fileChooser, true, true, 0)
	vbox.PackStart(hboxFolder, false, false, 0)

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
		ComboCountdown:   comboCountdown,
		FileChooser:      fileChooser,
		BtnHelp:          btnHelp,
		BtnAbout:         btnAbout,
	}

	return ui, win
}

func showRecordingResult(
	parent *gtk.Window,
	result string,
	outputFile string,
	details string,
) {
	messageType := gtk.MESSAGE_INFO
	title := "Recording Saved"
	message := "Video saved successfully.\n\nLocation:\n" + outputFile

	switch result {
	case "warning":
		messageType = gtk.MESSAGE_WARNING
		title = "Recording Warning"
		message = "A video file was created, but it may be incomplete.\n\nFile:\n" +
			outputFile
		if details != "" {
			message += "\n\nDetails:\n" + details
		}

	case "error":
		messageType = gtk.MESSAGE_ERROR
		title = "Recording Failed"
		message = "The video could not be saved.\n\nExpected file:\n" +
			outputFile
		if details != "" {
			message += "\n\nDetails:\n" + details
		}
	}

	dialog := gtk.MessageDialogNew(
		parent,
		gtk.DIALOG_MODAL,
		messageType,
		gtk.BUTTONS_OK,
		"%s",
		message,
	)
	dialog.SetTitle(title)
	dialog.Run()
	dialog.Destroy()
}

func showHelpDialog(parent *gtk.Window) {
	dialog := gtk.MessageDialogNew(
		parent,
		gtk.DIALOG_MODAL,
		gtk.MESSAGE_INFO,
		gtk.BUTTONS_OK,
		"%s",
		"Keyboard Shortcuts:\n\n"+
			"Ctrl + R: Start / Pause / Resume / Cancel Countdown\n"+
			"Ctrl + S: Stop and save\n"+
			"Ctrl + G: Toggle Magnifier\n\n"+
			"Select a countdown delay before recording.\n"+
			"Microphone and system audio can be recorded independently.\n"+
			"Recordings are saved as MP4.\n\n"+
			"Magnifier requires X11.",
	)
	dialog.SetTitle("Help - Helwan Recorder")
	dialog.Run()
	dialog.Destroy()
}

func showAboutDialog(parent *gtk.Window) {
	dialog, err := gtk.AboutDialogNew()
	if err != nil {
		return
	}

	dialog.SetTransientFor(parent)
	dialog.SetProgramName("Helwan Screen Recorder")
	dialog.SetVersion("1.1.0")
	dialog.SetComments("Screen and audio recorder for Helwan Linux.")
	dialog.SetWebsite("https://github.com/helwan-linux")

	dialog.Run()
	dialog.Destroy()
}
