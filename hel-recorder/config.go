// config.go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/gotk3/gotk3/gtk"
)

// AppSettings holds the options that are remembered between runs.
// It is stored as JSON in ~/.config/helwan-screen-recorder/config.json.
type AppSettings struct {
	FPS         string `json:"fps"`
	Screen      string `json:"screen"` // auto, area, window
	Format      string `json:"format"`
	Countdown   string `json:"countdown"`
	Quality     string `json:"quality"`
	Bitrate     string `json:"bitrate_kbps"`
	AutoStop    string `json:"auto_stop_minutes"`
	DiskGuard   string `json:"min_free_mb"`
	RecordMic   bool   `json:"record_mic"`
	Microphone  string `json:"microphone"`
	RecordSys   bool   `json:"record_system_audio"`
	SystemAudio string `json:"system_audio"`
	Minimize    bool   `json:"minimize_on_start"`
	Screenkey   bool   `json:"show_keystrokes"`
	Webcam      bool   `json:"webcam_overlay"`
	CamPos      string `json:"webcam_position"`
	CamSize     string `json:"webcam_size"`
	SaveDir     string `json:"save_dir"`
}

func settingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return ""
	}

	return filepath.Join(dir, "helwan-screen-recorder", "config.json")
}

// loadSettings returns nil when there is no (valid) settings file yet.
func loadSettings() *AppSettings {
	path := settingsPath()
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var s AppSettings
	if err := json.Unmarshal(data, &s); err != nil {
		debugf("invalid settings file ignored: %v", err)
		return nil
	}

	return &s
}

// writeSettings saves the settings atomically (temp file + rename) so a crash
// can never leave a half-written config behind.
func writeSettings(s AppSettings) {
	path := settingsPath()
	if path == "" {
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		debugf("cannot create config dir: %v", err)
		return
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		debugf("cannot write settings: %v", err)
		return
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		debugf("cannot save settings: %v", err)
	}
}

// collectSettings reads the current widget state. GTK main thread only.
func collectSettings(ui *AppUI) AppSettings {
	return AppSettings{
		FPS:         ui.ComboFPS.GetActiveID(),
		Screen:      ui.ComboRes.GetActiveID(),
		Format:      ui.ComboFormat.GetActiveID(),
		Countdown:   ui.ComboCountdown.GetActiveID(),
		Quality:     ui.ComboQuality.GetActiveID(),
		Bitrate:     ui.ComboBitrate.GetActiveID(),
		AutoStop:    ui.ComboAutoStop.GetActiveID(),
		DiskGuard:   ui.ComboDiskGuard.GetActiveID(),
		RecordMic:   ui.ChkAudio.GetActive(),
		Microphone:  ui.ComboMic.GetActiveID(),
		RecordSys:   ui.ChkSystemAudio.GetActive(),
		SystemAudio: ui.ComboSystemAudio.GetActiveID(),
		Minimize:    ui.ChkMinimize.GetActive(),
		Screenkey:   ui.ChkScreenkey.GetActive(),
		Webcam:      ui.ChkWebcam.GetActive(),
		CamPos:      ui.ComboCamPos.GetActiveID(),
		CamSize:     ui.ComboCamSize.GetActiveID(),
		SaveDir:     ui.FileChooser.GetFilename(),
	}
}

func saveSettings(ui *AppUI) {
	writeSettings(collectSettings(ui))
}

func setComboID(combo *gtk.ComboBoxText, id string) {
	if combo == nil || id == "" {
		return
	}

	// Unknown IDs (for example a microphone that is no longer connected)
	// leave the current default selection untouched.
	combo.SetActiveID(id)
}

// applySettings restores the saved options into the widgets. It must run
// before the "save on change" handlers are connected.
func applySettings(ui *AppUI) {
	s := loadSettings()
	if s == nil {
		return
	}

	setComboID(ui.ComboFPS, s.FPS)
	setComboID(ui.ComboRes, s.Screen)
	setComboID(ui.ComboFormat, s.Format)
	setComboID(ui.ComboCountdown, s.Countdown)
	setComboID(ui.ComboQuality, s.Quality)
	setComboID(ui.ComboBitrate, s.Bitrate)
	setComboID(ui.ComboAutoStop, s.AutoStop)
	setComboID(ui.ComboDiskGuard, s.DiskGuard)

	ui.ChkAudio.SetActive(s.RecordMic)
	setComboID(ui.ComboMic, s.Microphone)

	if ui.ChkSystemAudio.GetSensitive() {
		ui.ChkSystemAudio.SetActive(s.RecordSys)
		setComboID(ui.ComboSystemAudio, s.SystemAudio)
	}

	ui.ChkMinimize.SetActive(s.Minimize)
	ui.ChkScreenkey.SetActive(s.Screenkey)

	if hasWebcamDevice() {
		ui.ChkWebcam.SetActive(s.Webcam)
	}

	setComboID(ui.ComboCamPos, s.CamPos)
	setComboID(ui.ComboCamSize, s.CamSize)

	if s.SaveDir != "" {
		if info, err := os.Stat(s.SaveDir); err == nil && info.IsDir() {
			_ = ui.FileChooser.SetCurrentFolder(s.SaveDir)
		}
	}
}

// connectSettingsSaving saves the settings whenever the user changes one.
func connectSettingsSaving(ui *AppUI) {
	save := func() {
		saveSettings(ui)
	}

	for _, combo := range []*gtk.ComboBoxText{
		ui.ComboFPS,
		ui.ComboRes,
		ui.ComboFormat,
		ui.ComboCountdown,
		ui.ComboQuality,
		ui.ComboBitrate,
		ui.ComboAutoStop,
		ui.ComboDiskGuard,
		ui.ComboMic,
		ui.ComboSystemAudio,
		ui.ComboCamPos,
		ui.ComboCamSize,
	} {
		combo.Connect("changed", save)
	}

	for _, check := range []*gtk.CheckButton{
		ui.ChkAudio,
		ui.ChkSystemAudio,
		ui.ChkMinimize,
		ui.ChkScreenkey,
		ui.ChkWebcam,
	} {
		check.Connect("toggled", save)
	}

	ui.FileChooser.Connect("file-set", save)
}
