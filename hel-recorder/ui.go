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
	ComboFormat      *gtk.ComboBoxText
	ChkMinimize      *gtk.CheckButton
	ChkScreenkey     *gtk.CheckButton
	ChkWebcam        *gtk.CheckButton
	ComboCamPos      *gtk.ComboBoxText
	ComboCamSize     *gtk.ComboBoxText
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

	paths = append(paths,
		"/usr/share/icons/hicolor/128x128/apps/hel-recorder.png",
	)

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
	matches, err := filepath.Glob("/dev/video*")
	if err != nil {
		return false
	}

	for _, path := range matches {
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return true
		}
	}

	return false
}

func loadRecorderCSS() {
	paths := []string{}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		paths = append(paths,
			filepath.Join(exeDir, "style.css"),
			filepath.Join(exeDir, "assets", "style.css"),
		)
	}

	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths,
			filepath.Join(cwd, "style.css"),
			filepath.Join(cwd, "assets", "style.css"),
		)
	}

	paths = append(paths,
		"/usr/share/helwan-screen-recorder/style.css",
		"/usr/share/helwan-screen-recorder/assets/style.css",
	)

	var data []byte
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}

		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if len(data) == 0 {
		return
	}

	provider, err := gtk.CssProviderNew()
	if err != nil {
		return
	}

	if err := provider.LoadFromData(string(data)); err != nil {
		return
	}

	screen, err := gdk.ScreenGetDefault()
	if err != nil || screen == nil {
		return
	}

	gtk.AddProviderForScreen(
		screen,
		provider,
		gtk.STYLE_PROVIDER_PRIORITY_APPLICATION,
	)
}

func makeIconLabel(iconName string, text string) *gtk.Box {
	box, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)

	img, err := gtk.ImageNewFromIconName(
		iconName,
		gtk.ICON_SIZE_LARGE_TOOLBAR,
	)

	if err == nil {
		img.SetPixelSize(24)
		box.PackStart(img, false, false, 0)
	}

	label, _ := gtk.LabelNew(text)
	label.SetName("hsr-label")
	box.PackStart(label, false, false, 0)

	return box
}

func makeActionButton(iconName string, text string, name string) *gtk.Button {
	button, _ := gtk.ButtonNew()

	box, _ := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 8)

	img, err := gtk.ImageNewFromIconName(
		iconName,
		gtk.ICON_SIZE_BUTTON,
	)

	if err == nil {
		img.SetPixelSize(20)
		box.PackStart(img, false, false, 0)
	}

	label, _ := gtk.LabelNew(text)
	box.PackStart(label, false, false, 0)

	button.Add(box)
	button.SetName(name)

	return button
}

func InitUI() (*AppUI, *gtk.Window) {
	loadRecorderCSS()

	win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)
	if err != nil {
		panic(err)
	}

	win.SetTitle("Helwan Screen Recorder")
	win.SetDefaultSize(900, 900)
	win.SetPosition(gtk.WIN_POS_CENTER)
	win.SetResizable(false)
	win.SetName("hsr-window")

	iconPixbuf := loadAppIconPixbuf()

	if iconPixbuf != nil {
		win.SetIcon(iconPixbuf)
	}

	mainBox, _ := gtk.BoxNew(
		gtk.ORIENTATION_VERTICAL,
		12,
	)

	mainBox.SetMarginTop(18)
	mainBox.SetMarginBottom(18)
	mainBox.SetMarginStart(22)
	mainBox.SetMarginEnd(22)
	mainBox.SetName("hsr-main")

	win.Add(mainBox)

	/* =========================
	   HEADER
	========================= */

	header, _ := gtk.BoxNew(
		gtk.ORIENTATION_HORIZONTAL,
		10,
	)

	titleArea, _ := gtk.BoxNew(
		gtk.ORIENTATION_HORIZONTAL,
		10,
	)

	if iconPixbuf != nil {
		logo, err := gtk.ImageNewFromPixbuf(iconPixbuf)

		if err == nil {
			logo.SetPixelSize(46)
			titleArea.PackStart(logo, false, false, 0)
		}
	}

	titleText, _ := gtk.BoxNew(
		gtk.ORIENTATION_VERTICAL,
		2,
	)

	title, _ := gtk.LabelNew("Helwan Screen Recorder")
	title.SetName("hsr-title")
	title.SetHAlign(gtk.ALIGN_START)

	subtitle, _ := gtk.LabelNew(
		"Simple, lightweight and practical screen recording",
	)
	subtitle.SetName("hsr-subtitle")
	subtitle.SetHAlign(gtk.ALIGN_START)

	titleText.PackStart(title, false, false, 0)
	titleText.PackStart(subtitle, false, false, 0)

	titleArea.PackStart(titleText, false, false, 0)

	btnHelp := makeActionButton(
		"help-browser",
		"Help",
		"hsr-help",
	)

	btnAbout := makeActionButton(
		"help-about",
		"About",
		"hsr-about",
	)

	header.PackStart(titleArea, true, true, 0)
	header.PackEnd(btnAbout, false, false, 0)
	header.PackEnd(btnHelp, false, false, 0)

	mainBox.PackStart(header, false, false, 0)

	/* =========================
	   TIMER
	========================= */

	timerBox, _ := gtk.BoxNew(
		gtk.ORIENTATION_HORIZONTAL,
		10,
	)

	timerBox.SetMarginTop(6)

	timerIcon, err := gtk.ImageNewFromIconName(
		"media-playback-start",
		gtk.ICON_SIZE_DIALOG,
	)

	if err == nil {
		timerIcon.SetPixelSize(42)
		timerBox.PackStart(timerIcon, false, false, 0)
	}

	lblTimer, _ := gtk.LabelNew("00:00:00")
	lblTimer.SetName("hsr-timer")

	timerBox.PackStart(lblTimer, false, false, 0)

	mainBox.PackStart(timerBox, false, false, 0)

	statusBox, _ := gtk.BoxNew(
		gtk.ORIENTATION_HORIZONTAL,
		8,
	)

	statusDot, _ := gtk.LabelNew("●")
	statusDot.SetMarkup(
		"<span font='16' color='#20a65a'>●</span>",
	)

	lblStatus, _ := gtk.LabelNew("Status: Ready")
	lblStatus.SetName("hsr-status")

	statusBox.PackStart(statusDot, false, false, 0)
	statusBox.PackStart(lblStatus, false, false, 0)

	mainBox.PackStart(statusBox, false, false, 0)

	separator, _ := gtk.SeparatorNew(
		gtk.ORIENTATION_HORIZONTAL,
	)

	mainBox.PackStart(separator, false, false, 4)

	/* =========================
	   VIDEO SETTINGS
	========================= */

	videoPanel, _ := gtk.BoxNew(
		gtk.ORIENTATION_VERTICAL,
		10,
	)

	videoPanel.SetName("hsr-panel")

	videoTitle, _ := gtk.LabelNew("Video Settings")
	videoTitle.SetName("hsr-section")
	videoTitle.SetHAlign(gtk.ALIGN_START)

	videoPanel.PackStart(videoTitle, false, false, 0)

	videoGrid, _ := gtk.GridNew()

	videoGrid.SetRowSpacing(10)
	videoGrid.SetColumnSpacing(14)

	/* FPS */

	lblFPS := makeIconLabel(
		"video-display",
		"Frame Rate:",
	)

	comboFPS, _ := gtk.ComboBoxTextNew()
	comboFPS.Append("30", "30 FPS")
	comboFPS.Append("60", "60 FPS")
	comboFPS.SetActiveID("30")
	comboFPS.SetName("hsr-combo")
	comboFPS.SetSizeRequest(250, 36)

	videoGrid.Attach(lblFPS, 0, 0, 1, 1)
	videoGrid.Attach(comboFPS, 1, 0, 1, 1)

	/* SCREEN */

	lblRes := makeIconLabel(
		"view-fullscreen",
		"Screen:",
	)

	comboRes, _ := gtk.ComboBoxTextNew()
	comboRes.Append("auto", "Full Screen (Native)")
	comboRes.Append("area", "Select Area (slop)")
	comboRes.SetActiveID("auto")
	comboRes.SetName("hsr-combo")
	comboRes.SetSizeRequest(250, 36)

	videoGrid.Attach(lblRes, 2, 0, 1, 1)
	videoGrid.Attach(comboRes, 3, 0, 1, 1)

	/* FORMAT */

	lblFormat := makeIconLabel(
		"video-x-generic",
		"Format / Codec:",
	)

	comboFormat, _ := gtk.ComboBoxTextNew()

	comboFormat.Append(
		"mp4",
		"MP4 (H.264 / Standard)",
	)

	comboFormat.Append(
		"mkv",
		"MKV (Safe against crashes)",
	)

	comboFormat.Append(
		"webm",
		"WebM (Web optimized)",
	)

	comboFormat.Append(
		"gif",
		"Animated GIF (Short clips)",
	)

	comboFormat.SetActiveID("mp4")
	comboFormat.SetName("hsr-combo")
	comboFormat.SetSizeRequest(250, 36)

	videoGrid.Attach(lblFormat, 0, 1, 1, 1)
	videoGrid.Attach(comboFormat, 1, 1, 1, 1)

	/* COUNTDOWN */

	lblCountdown := makeIconLabel(
		"alarm",
		"Start Delay:",
	)

	comboCountdown, _ := gtk.ComboBoxTextNew()

	comboCountdown.Append(
		"0",
		"No Countdown",
	)

	comboCountdown.Append(
		"3",
		"3 Seconds",
	)

	comboCountdown.Append(
		"5",
		"5 Seconds",
	)

	comboCountdown.Append(
		"10",
		"10 Seconds",
	)

	comboCountdown.SetActiveID("0")
	comboCountdown.SetName("hsr-combo")
	comboCountdown.SetSizeRequest(250, 36)

	videoGrid.Attach(lblCountdown, 2, 1, 1, 1)
	videoGrid.Attach(comboCountdown, 3, 1, 1, 1)

	videoPanel.PackStart(
		videoGrid,
		false,
		false,
		0,
	)

	mainBox.PackStart(
		videoPanel,
		false,
		false,
		0,
	)

	/* =========================
	   AUDIO
	========================= */

	audioPanel, _ := gtk.BoxNew(
		gtk.ORIENTATION_VERTICAL,
		8,
	)

	audioPanel.SetName("hsr-panel")

	audioTitle, _ := gtk.LabelNew("Audio")
	audioTitle.SetName("hsr-section")
	audioTitle.SetHAlign(gtk.ALIGN_START)

	audioPanel.PackStart(
		audioTitle,
		false,
		false,
		0,
	)

	chkAudio, _ := gtk.CheckButtonNewWithLabel(
		"Record Microphone",
	)

	chkAudio.SetActive(true)
	chkAudio.SetName("hsr-check")

	micRow, _ := gtk.BoxNew(
		gtk.ORIENTATION_HORIZONTAL,
		10,
	)

	micIcon, err := gtk.ImageNewFromIconName(
		"audio-input-microphone",
		gtk.ICON_SIZE_LARGE_TOOLBAR,
	)

	if err == nil {
		micIcon.SetPixelSize(25)
		micRow.PackStart(micIcon, false, false, 0)
	}

	lblMic, _ := gtk.LabelNew("Microphone:")
	lblMic.SetName("hsr-label")
	lblMic.SetHAlign(gtk.ALIGN_START)

	comboMic, _ := gtk.ComboBoxTextNew()

	comboMic.Append(
		"default",
		"Default Microphone",
	)

	microphones, micErr := GetMicrophoneSources()

	for _, source := range microphones {
		if source == "" || source == "default" {
			continue
		}

		comboMic.Append(source, source)
	}

	comboMic.SetActiveID("default")
	comboMic.SetName("hsr-combo")
	comboMic.SetSizeRequest(420, 36)

	micRow.PackStart(
		lblMic,
		false,
		false,
		0,
	)

	micRow.PackStart(
		comboMic,
		false,
		false,
		0,
	)

	audioPanel.PackStart(
		chkAudio,
		false,
		false,
		0,
	)

	audioPanel.PackStart(
		micRow,
		false,
		false,
		0,
	)

	chkAudio.Connect("toggled", func() {
		comboMic.SetSensitive(
			chkAudio.GetActive(),
		)
	})

	/* SYSTEM AUDIO */

	chkSystemAudio, _ := gtk.CheckButtonNewWithLabel(
		"Record System Audio",
	)

	chkSystemAudio.SetName("hsr-check")

	systemRow, _ := gtk.BoxNew(
		gtk.ORIENTATION_HORIZONTAL,
		10,
	)

	systemIcon, err := gtk.ImageNewFromIconName(
		"audio-volume-high",
		gtk.ICON_SIZE_LARGE_TOOLBAR,
	)

	if err == nil {
		systemIcon.SetPixelSize(25)
		systemRow.PackStart(systemIcon, false, false, 0)
	}

	lblSystem, _ := gtk.LabelNew("Audio Output:")
	lblSystem.SetName("hsr-label")
	lblSystem.SetHAlign(gtk.ALIGN_START)

	comboSystemAudio, _ := gtk.ComboBoxTextNew()

	systemSources, systemErr := GetSystemAudioSources()

	for _, source := range systemSources {
		if source == "" {
			continue
		}

		comboSystemAudio.Append(
			source,
			source,
		)
	}

	if len(systemSources) > 0 {
		comboSystemAudio.SetActive(0)
	} else {
		chkSystemAudio.SetSensitive(false)
		comboSystemAudio.SetSensitive(false)
	}

	comboSystemAudio.SetName("hsr-combo")
	comboSystemAudio.SetSizeRequest(420, 36)

	systemRow.PackStart(
		lblSystem,
		false,
		false,
		0,
	)

	systemRow.PackStart(
		comboSystemAudio,
		false,
		false,
		0,
	)

	audioPanel.PackStart(
		chkSystemAudio,
		false,
		false,
		0,
	)

	audioPanel.PackStart(
		systemRow,
		false,
		false,
		0,
	)

	chkSystemAudio.Connect("toggled", func() {
		comboSystemAudio.SetSensitive(
			chkSystemAudio.GetActive() &&
				len(systemSources) > 0,
		)
	})

	comboSystemAudio.SetSensitive(
		chkSystemAudio.GetActive() &&
			len(systemSources) > 0,
	)

	if micErr != nil && systemErr != nil {
		lblStatus.SetText(
			"Status: Ready - Audio sources unavailable",
		)
	}

	mainBox.PackStart(
		audioPanel,
		false,
		false,
		0,
	)

	/* =========================
	   OPTIONS + WEBCAM
	========================= */

	optionsPanel, _ := gtk.BoxNew(
		gtk.ORIENTATION_HORIZONTAL,
		20,
	)

	optionsPanel.SetName("hsr-panel")

	leftBox, _ := gtk.BoxNew(
		gtk.ORIENTATION_VERTICAL,
		7,
	)

	rightBox, _ := gtk.BoxNew(
		gtk.ORIENTATION_VERTICAL,
		7,
	)

	chkMinimize, _ := gtk.CheckButtonNewWithLabel(
		"Minimize window automatically on start",
	)

	chkMinimize.SetActive(true)
	chkMinimize.SetName("hsr-check")

	chkScreenkey, _ := gtk.CheckButtonNewWithLabel(
		"Show Keystrokes on screen (screenkey)",
	)

	chkScreenkey.SetName("hsr-check")

	leftBox.PackStart(
		chkMinimize,
		false,
		false,
		0,
	)

	leftBox.PackStart(
		chkScreenkey,
		false,
		false,
		0,
	)

	chkWebcam, _ := gtk.CheckButtonNewWithLabel(
		"Webcam Overlay & Live Preview (PiP)",
	)

	chkWebcam.SetName("hsr-check")

	rightBox.PackStart(
		chkWebcam,
		false,
		false,
		0,
	)

	cameraRow, _ := gtk.BoxNew(
		gtk.ORIENTATION_HORIZONTAL,
		8,
	)

	lblCamPos, _ := gtk.LabelNew(
		"Position:",
	)

	lblCamPos.SetName("hsr-label")

	comboCamPos, _ := gtk.ComboBoxTextNew()

	comboCamPos.Append(
		"bottom-right",
		"Bottom-Right",
	)

	comboCamPos.Append(
		"bottom-left",
		"Bottom-Left",
	)

	comboCamPos.Append(
		"top-right",
		"Top-Right",
	)

	comboCamPos.Append(
		"top-left",
		"Top-Left",
	)

	comboCamPos.SetActiveID("bottom-right")
	comboCamPos.SetName("hsr-combo")

	lblCamSize, _ := gtk.LabelNew(
		"Size:",
	)

	lblCamSize.SetName("hsr-label")

	comboCamSize, _ := gtk.ComboBoxTextNew()

	comboCamSize.Append(
		"240x180",
		"Small (240x180)",
	)

	comboCamSize.Append(
		"320x240",
		"Medium (320x240)",
	)

	comboCamSize.Append(
		"480x360",
		"Large (480x360)",
	)

	comboCamSize.SetActiveID("320x240")
	comboCamSize.SetName("hsr-combo")

	cameraRow.PackStart(
		lblCamPos,
		false,
		false,
		0,
	)

	cameraRow.PackStart(
		comboCamPos,
		false,
		false,
		0,
	)

	cameraRow.PackStart(
		lblCamSize,
		false,
		false,
		8,
	)

	cameraRow.PackStart(
		comboCamSize,
		false,
		false,
		0,
	)

	rightBox.PackStart(
		cameraRow,
		false,
		false,
		0,
	)

	optionsPanel.PackStart(
		leftBox,
		true,
		true,
		0,
	)

	optionsPanel.PackStart(
		rightBox,
		true,
		true,
		0,
	)

	comboCamPos.SetSensitive(false)
	comboCamSize.SetSensitive(false)

	if !hasWebcamDevice() {
		chkWebcam.SetSensitive(false)
		chkWebcam.SetTooltipText(
			"No webcam detected at /dev/video0",
		)
	} else {
		chkWebcam.Connect("toggled", func() {
			active := chkWebcam.GetActive()

			comboCamPos.SetSensitive(active)
			comboCamSize.SetSensitive(active)
		})
	}

	mainBox.PackStart(
		optionsPanel,
		false,
		false,
		0,
	)

	/* =========================
	   SAVE DIRECTORY
	========================= */

	savePanel, _ := gtk.BoxNew(
		gtk.ORIENTATION_HORIZONTAL,
		10,
	)

	savePanel.SetName("hsr-panel")

	folderIcon, err := gtk.ImageNewFromIconName(
		"folder",
		gtk.ICON_SIZE_LARGE_TOOLBAR,
	)

	if err == nil {
		folderIcon.SetPixelSize(28)
		savePanel.PackStart(
			folderIcon,
			false,
			false,
			0,
		)
	}

	lblFolder, _ := gtk.LabelNew(
		"Save Directory:",
	)

	lblFolder.SetName("hsr-label")

	fileChooser, _ := gtk.FileChooserButtonNew(
		"Select Directory",
		gtk.FILE_CHOOSER_ACTION_SELECT_FOLDER,
	)

	fileChooser.SetName("hsr-folder")

	homeDir, _ := os.UserHomeDir()

	videosDir := filepath.Join(
		homeDir,
		"Videos",
	)

	if info, err := os.Stat(videosDir); err == nil &&
		info.IsDir() {

		_ = fileChooser.SetCurrentFolder(
			videosDir,
		)

	} else {

		_ = fileChooser.SetCurrentFolder(
			homeDir,
		)
	}

	savePanel.PackStart(
		lblFolder,
		false,
		false,
		0,
	)

	savePanel.PackStart(
		fileChooser,
		true,
		true,
		0,
	)

	mainBox.PackStart(
		savePanel,
		false,
		false,
		0,
	)

	/* =========================
	   ACTION BUTTONS
	========================= */

	buttonBox, _ := gtk.BoxNew(
		gtk.ORIENTATION_HORIZONTAL,
		14,
	)

	buttonBox.SetHomogeneous(true)

	btnStart := makeActionButton(
		"media-playback-start",
		"Start (Ctrl+R)",
		"hsr-start",
	)

	btnPause := makeActionButton(
		"media-playback-pause",
		"Pause (Ctrl+E)",
		"hsr-pause",
	)

	btnStop := makeActionButton(
		"media-playback-stop",
		"Stop (Ctrl+Q)",
		"hsr-stop",
	)

	btnPause.SetSensitive(false)
	btnStop.SetSensitive(false)

	buttonBox.PackStart(
		btnStart,
		true,
		true,
		0,
	)

	buttonBox.PackStart(
		btnPause,
		true,
		true,
		0,
	)

	buttonBox.PackStart(
		btnStop,
		true,
		true,
		0,
	)

	mainBox.PackEnd(
		buttonBox,
		false,
		false,
		0,
	)

	/* =========================
	   CALLBACKS
	========================= */

	btnHelp.Connect("clicked", func() {
		showHelpDialog(win)
	})

	btnAbout.Connect("clicked", func() {
		showAboutDialog(
			win,
			iconPixbuf,
		)
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
		ComboFormat:      comboFormat,
		ChkMinimize:      chkMinimize,
		ChkScreenkey:     chkScreenkey,
		ChkWebcam:        chkWebcam,
		ComboCamPos:      comboCamPos,
		ComboCamSize:     comboCamSize,
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

	dialog.AddButton(
		"Unmute & Record",
		gtk.RESPONSE_YES,
	)

	dialog.AddButton(
		"Record Muted",
		gtk.RESPONSE_NO,
	)

	dialog.AddButton(
		"Cancel",
		gtk.RESPONSE_CANCEL,
	)

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

	message := "Video saved successfully.\n\nLocation:\n" +
		outputFile

	switch result {

	case "warning":
		messageType = gtk.MESSAGE_WARNING
		title = "Recording Warning"

		message =
			"A video file was created, but it may be incomplete.\n\nFile:\n" +
				outputFile

		if details != "" {
			message += "\n\nDetails:\n" + details
		}

	case "error":
		messageType = gtk.MESSAGE_ERROR
		title = "Recording Failed"

		message =
			"The video could not be saved.\n\nExpected file:\n" +
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
			"- Multiple video formats (MP4, MKV, WebM, GIF).\n"+
			"- Floating OSD status pill during recording.\n"+
			"- Webcam PiP preview & Screenkey lag fixes.",
	)

	dialog.SetTitle("Help - Helwan Recorder")
	dialog.Run()
	dialog.Destroy()
}

func showAboutDialog(
	parent *gtk.Window,
	logo *gdk.Pixbuf,
) {
	dialog, err := gtk.AboutDialogNew()

	if err != nil {
		return
	}

	dialog.SetTransientFor(parent)

	if logo != nil {
		dialog.SetLogo(logo)
	}

	dialog.SetProgramName(
		"Helwan Screen Recorder",
	)

	dialog.SetVersion("1.1.0")

	dialog.SetComments(
		"Screen and audio recorder for Helwan Linux.",
	)

	dialog.SetWebsite(
		"https://github.com/helwan-linux",
	)

	dialog.Run()
	dialog.Destroy()
}
