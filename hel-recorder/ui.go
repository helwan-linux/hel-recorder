//ui.go
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
	BtnPause         *gtk.Button
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
	ChkMinimize      *gtk.CheckButton
	ChkScreenkey     *gtk.CheckButton
	ChkWebcam        *gtk.CheckButton
	FileChooser      *gtk.FileChooserButton
	BtnHelp          *gtk.Button
	BtnAbout         *gtk.Button
	AppIconPixbuf    *gdk.Pixbuf
}

func loadAppIconPixbuf() *gdk.Pixbuf {
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

	paths = append(paths, "/usr/share/icons/hicolor/128x128/apps/hel-recorder.png")

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}

		if pb, err := gdk.PixbufNewFromFile(path); err == nil {
			return pb
		}
	}
	return nil
}

func hasWebcamDevice() bool {
	info, err := os.Stat("/dev/video0")
	return err == nil && !info.IsDir()
}

func InitUI() (*AppUI, *gtk.Window) {
	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		panic(err)
	}

	win.SetTitle("Helwan Screen Recorder")
	win.SetDefaultSize(500, 660)
	win.SetPosition(gtk.WIN_POS_CENTER)
	win.SetResizable(false)

	iconPixbuf := loadAppIconPixbuf()
	if iconPixbuf != nil {
		win.SetIcon(iconPixbuf)
	}

	vbox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 10)
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
	vbox.PackStart(sep, false, false, 2)

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

	lblRes, _ := gtk.LabelNew("Screen:")
	lblRes.SetHAlign(gtk.ALIGN_START)
	comboRes, _ := gtk.ComboBoxTextNew()
	comboRes.Append("auto", "Full Screen (Native)")
	comboRes.Append("area", "Select Area (slop)")
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

	sep2, _ := gtk.SeparatorNew(gtk.ORIENTATION_HORIZONTAL)
	vbox.PackStart(sep2, false, false, 2)

	chkMinimize, _ := gtk.CheckButtonNewWithLabel("Minimize window automatically on start")
	chkMinimize.SetActive(true)
	vbox.PackStart(chkMinimize, false, false, 0)

	chkScreenkey, _ := gtk.CheckButtonNewWithLabel("Show Keystrokes on screen (screenkey)")
	vbox.PackStart(chkScreenkey, false, false, 0)

	chkWebcam, _ := gtk.CheckButtonNewWithLabel("Webcam Overlay (Bottom-Right PiP)")
	if !hasWebcamDevice() {
		chkWebcam.SetSensitive(false)
		chkWebcam.SetTooltipText("No webcam detected at /dev/video0")
	}
	vbox.PackStart(chkWebcam, false, false, 0)

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

	hboxButtons, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)
	hboxButtons.SetHomogeneous(true)

	btnStart, _ := gtk.ButtonNewWithLabel("Start (Ctrl+R)")
	btnPause, _ := gtk.ButtonNewWithLabel("Pause (Ctrl+E)")
	btnStop, _ := gtk.ButtonNewWithLabel("Stop (Ctrl+Q)")

	btnPause.SetSensitive(false)
	btnStop.SetSensitive(false)

	hboxButtons.PackStart(btnStart, true, true, 0)
	hboxButtons.PackStart(btnPause, true, true, 0)
	hboxButtons.PackStart(btnStop, true, true, 0)
	vbox.PackEnd(hboxButtons, false, false, 0)

	btnHelp.Connect("clicked", func() {
		showHelpDialog(win)
	})

	btnAbout.Connect("clicked", func() {
		showAboutDialog(win, iconPixbuf)
	})

	ui := &AppUI{
		Window:           win,
		BtnStart:         btnStart,
		BtnPause:         btnPause,
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
		ChkMinimize:      chkMinimize,
		ChkScreenkey:     chkScreenkey,
		ChkWebcam:        chkWebcam,
		FileChooser:      fileChooser,
		BtnHelp:          btnHelp,
		BtnAbout:         btnAbout,
		AppIconPixbuf:    iconPixbuf,
	}

	return ui, win
}

func showMicMutedDialog(parent *gtk.Window) string {
	dialog := gtk.MessageDialogNew(
		parent,
		gtk.DIALOG_MODAL,
		gtk.MESSAGE_WARNING,
		gtk.BUTTONS_NONE,
		"%s",
		"Your selected microphone is currently MUTED.\n\nWould you like to unmute it and start recording?",
	)
	dialog.SetTitle("Microphone Muted")
	dialog.AddButton("Unmute & Record", gtk.RESPONSE_YES)
	dialog.AddButton("Record Muted", gtk.RESPONSE_NO)
	dialog.AddButton("Cancel", gtk.RESPONSE_CANCEL)

	resp := dialog.Run()
	dialog.Destroy()

	switch resp {
	case gtk.RESPONSE_YES:
		return "unmute"
	case gtk.RESPONSE_NO:
		return "keep"
	default:
		return "cancel"
	}
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
			"Ctrl + R: Start recording\n"+
			"Ctrl + E: Pause / Resume recording\n"+
			"Ctrl + Q: Stop and save\n"+
			"Ctrl + G: Toggle Magnifier\n\n"+
			"Features:\n"+
			"- Select Area requires 'slop' package.\n"+
			"- Keystrokes overlay requires 'screenkey' package.\n"+
			"- Webcam PiP uses /dev/video0.",
	)
	dialog.SetTitle("Help - Helwan Recorder")
	dialog.Run()
	dialog.Destroy()
}

func showAboutDialog(parent *gtk.Window, logo *gdk.Pixbuf) {
	dialog, err := gtk.AboutDialogNew()
	if err != nil {
		return
	}

	dialog.SetTransientFor(parent)
	if logo != nil {
		dialog.SetLogo(logo)
	}
	dialog.SetProgramName("Helwan Screen Recorder")
	dialog.SetVersion("1.0.0")
	dialog.SetComments("Screen and audio recorder for Helwan Linux.")
	dialog.SetWebsite("https://github.com/helwan-linux")

	dialog.Run()
	dialog.Destroy()
}
