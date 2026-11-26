// recovery.go
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

// A leftover part must be at least this old, so a file that is still being
// written by another running instance is never touched.
const recoveryMinAge = 30 * time.Second

const (
	recoverResponseRecover gtk.ResponseType = 101
	recoverResponseDelete  gtk.ResponseType = 102
)

// offerRecovery looks for unfinished recordings in the save directory and
// asks the user what to do with them. It must run on the GTK main thread.
func (m *RecorderManager) offerRecovery() {
	dir := strings.TrimSpace(m.ui.FileChooser.GetFilename())
	if dir == "" {
		return
	}

	videos, audios := scanOrphanParts(dir, time.Now(), recoveryMinAge)
	if len(videos) == 0 && len(audios) == 0 {
		return
	}

	message := fmt.Sprintf(
		"Unfinished recording data was found in:\n%s\n\n"+
			"%d video part(s), %d audio part(s), %s in total.\n\n"+
			"This usually happens after a crash or a power loss.\n"+
			"Recover them into playable files?",
		dir,
		len(videos),
		len(audios),
		formatBytes(uint64(orphanTotalSize(videos, audios))),
	)

	dialog := gtk.MessageDialogNew(
		m.ui.Window,
		gtk.DIALOG_MODAL,
		gtk.MESSAGE_QUESTION,
		gtk.BUTTONS_NONE,
		"%s",
		message,
	)

	dialog.SetTitle("Recover Unfinished Recordings")
	dialog.AddButton("Recover", recoverResponseRecover)
	dialog.AddButton("Delete", recoverResponseDelete)
	dialog.AddButton("Later", gtk.RESPONSE_CANCEL)

	response := dialog.Run()
	dialog.Destroy()

	switch response {
	case recoverResponseRecover:
		m.setStatus("Status: Recovering unfinished recordings...")

		go m.runRecovery(dir, videos, audios)

	case recoverResponseDelete:
		removeOrphanParts(videos, audios)
		m.setStatus("Status: Unfinished recording data deleted")
	}
}

func (m *RecorderManager) runRecovery(
	dir string,
	videos []orphanPart,
	audios []orphanPart,
) {
	jobs := pairRecoveryParts(videos, audios)

	var (
		recovered []string
		problems  []string
	)

	for _, job := range jobs {
		path, err := recoverJob(dir, job)

		if path != "" {
			recovered = append(recovered, path)
		}

		if err != nil {
			problems = append(problems, err.Error())
		}
	}

	glib.IdleAdd(func() {
		message := fmt.Sprintf(
			"Recovered %d file(s) into:\n%s",
			len(recovered),
			dir,
		)

		messageType := gtk.MESSAGE_INFO

		if len(problems) > 0 {
			messageType = gtk.MESSAGE_WARNING
			message += "\n\nWarnings:\n" + strings.Join(problems, "\n")
		}

		m.ui.LblStatus.SetText(
			fmt.Sprintf("Status: Recovered %d file(s)", len(recovered)),
		)

		dialog := gtk.MessageDialogNew(
			m.ui.Window,
			gtk.DIALOG_MODAL,
			messageType,
			gtk.BUTTONS_NONE,
			"%s",
			message,
		)

		dialog.SetTitle("Recovery Finished")
		dialog.AddButton("Open Folder", respOpenFolder)
		dialog.AddButton("OK", gtk.RESPONSE_OK)

		if dialog.Run() == respOpenFolder {
			openWithDefaultApp(dir)
		}

		dialog.Destroy()
	})
}
