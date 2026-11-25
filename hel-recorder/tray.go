// tray.go
package main

/*
#cgo pkg-config: gtk+-3.0
#cgo CFLAGS: -Wno-deprecated-declarations

#include <stdlib.h>
#include <gtk/gtk.h>

// System tray support: prefers libayatana-appindicator3 / libappindicator3 via
// dlopen so StatusNotifier-based desktop panels (like Cinnamon / modern X11/Wayland)
// show the icon reliably, with GtkStatusIcon as a fallback.

#define HSR_TRAY_NONE  0
#define HSR_TRAY_SHOW  1
#define HSR_TRAY_START 2
#define HSR_TRAY_PAUSE 3
#define HSR_TRAY_STOP  4
#define HSR_TRAY_QUIT  5

#if defined(__linux__) || defined(__unix__)
#include <dlfcn.h>

typedef enum {
    HSR_IND_CATEGORY_APPLICATION_STATUS = 0
} HsrIndCategory;

typedef enum {
    HSR_IND_STATUS_PASSIVE = 0,
    HSR_IND_STATUS_ACTIVE,
    HSR_IND_STATUS_ATTENTION
} HsrIndStatus;

typedef gpointer (*hsr_ind_new_fn)(const gchar *id, const gchar *icon_name, HsrIndCategory category);
typedef void (*hsr_ind_set_status_fn)(gpointer indicator, HsrIndStatus status);
typedef void (*hsr_ind_set_menu_fn)(gpointer indicator, GtkMenu *menu);
typedef void (*hsr_ind_set_icon_full_fn)(gpointer indicator, const gchar *icon_name, const gchar *icon_desc);
typedef void (*hsr_ind_set_attention_icon_full_fn)(gpointer indicator, const gchar *icon_name, const gchar *icon_desc);
typedef void (*hsr_ind_set_title_fn)(gpointer indicator, const gchar *title);
typedef void (*hsr_ind_set_label_fn)(gpointer indicator, const gchar *label, const gchar *guide);

static void *hsr_ind_lib = NULL;
static gpointer hsr_indicator = NULL;
static hsr_ind_set_status_fn fn_ind_set_status = NULL;
static hsr_ind_set_icon_full_fn fn_ind_set_icon_full = NULL;
static hsr_ind_set_attention_icon_full_fn fn_ind_set_attention_icon_full = NULL;
static hsr_ind_set_title_fn fn_ind_set_title = NULL;
static hsr_ind_set_label_fn fn_ind_set_label = NULL;
#endif

static GtkStatusIcon *hsr_tray_icon = NULL;
static GtkWidget *hsr_tray_menu = NULL;
static GtkWidget *hsr_tray_item_start = NULL;
static GtkWidget *hsr_tray_item_pause = NULL;
static GtkWidget *hsr_tray_item_stop = NULL;
static char *hsr_tray_icon_path = NULL;
static char *hsr_tray_fallback_icon = "applications-multimedia";
static int hsr_tray_pending = HSR_TRAY_NONE;
static int hsr_using_appindicator = 0;

static void hsr_tray_menu_cb(GtkMenuItem *item, gpointer data) {
    (void)item;
    hsr_tray_pending = GPOINTER_TO_INT(data);
}

static void hsr_tray_activate_cb(GtkStatusIcon *icon, gpointer data) {
    (void)icon;
    (void)data;
    hsr_tray_pending = HSR_TRAY_SHOW;
}

static void hsr_tray_popup_cb(GtkStatusIcon *icon, guint button,
                              guint activate_time, gpointer data) {
    (void)data;
    gtk_menu_popup(GTK_MENU(hsr_tray_menu), NULL, NULL,
                   gtk_status_icon_position_menu, icon, button, activate_time);
}

static GtkWidget *hsr_tray_add_item(const char *label, int action) {
    GtkWidget *item = gtk_menu_item_new_with_label(label);
    g_signal_connect(item, "activate", G_CALLBACK(hsr_tray_menu_cb),
                     GINT_TO_POINTER(action));
    gtk_menu_shell_append(GTK_MENU_SHELL(hsr_tray_menu), item);
    return item;
}

static void hsr_tray_add_separator(void) {
    gtk_menu_shell_append(GTK_MENU_SHELL(hsr_tray_menu),
                          gtk_separator_menu_item_new());
}

static const char *hsr_resolve_icon_name(const char *preferred) {
    GtkIconTheme *theme = gtk_icon_theme_get_default();
    if (theme != NULL && preferred != NULL && preferred[0] != '\0') {
        if (gtk_icon_theme_has_icon(theme, preferred)) {
            return preferred;
        }
    }

    const char *candidates[] = {
        "applications-multimedia",
        "camera-video",
        "video-display",
        "media-record",
        "application-x-executable",
        NULL
    };

    if (theme != NULL) {
        for (int i = 0; candidates[i] != NULL; i++) {
            if (gtk_icon_theme_has_icon(theme, candidates[i])) {
                return candidates[i];
            }
        }
    }

    return "application-x-executable";
}

#if defined(__linux__) || defined(__unix__)
static int hsr_try_init_appindicator(const char *icon_path, const char *tooltip, int debug) {
    const char *libs[] = {
        "libayatana-appindicator3.so.1",
        "libayatana-appindicator3.so",
        "libappindicator3.so.1",
        "libappindicator3.so",
        NULL
    };

    void *handle = NULL;
    for (int i = 0; libs[i] != NULL; i++) {
        handle = dlopen(libs[i], RTLD_LAZY);
        if (handle != NULL) {
            if (debug) {
                fprintf(stderr, "[hsr] loaded tray indicator library: %s\n", libs[i]);
            }
            break;
        }
    }

    if (handle == NULL) {
        if (debug) {
            fprintf(stderr, "[hsr] no appindicator3 library found via dlopen\n");
        }
        return 0;
    }

    hsr_ind_new_fn fn_new = (hsr_ind_new_fn)dlsym(handle, "app_indicator_new");
    hsr_ind_set_status_fn fn_status = (hsr_ind_set_status_fn)dlsym(handle, "app_indicator_set_status");
    hsr_ind_set_menu_fn fn_menu = (hsr_ind_set_menu_fn)dlsym(handle, "app_indicator_set_menu");
    fn_ind_set_icon_full = (hsr_ind_set_icon_full_fn)dlsym(handle, "app_indicator_set_icon_full");
    fn_ind_set_attention_icon_full = (hsr_ind_set_attention_icon_full_fn)dlsym(handle, "app_indicator_set_attention_icon_full");
    fn_ind_set_title = (hsr_ind_set_title_fn)dlsym(handle, "app_indicator_set_title");
    fn_ind_set_label = (hsr_ind_set_label_fn)dlsym(handle, "app_indicator_set_label");

    if (fn_new == NULL || fn_status == NULL || fn_menu == NULL) {
        dlclose(handle);
        return 0;
    }

    const char *init_icon = hsr_resolve_icon_name(hsr_tray_fallback_icon);
    if (icon_path != NULL && icon_path[0] != '\0') {
        init_icon = icon_path;
    }

    gpointer ind = fn_new("helwan-screen-recorder", init_icon, HSR_IND_CATEGORY_APPLICATION_STATUS);
    if (ind == NULL) {
        dlclose(handle);
        return 0;
    }

    fn_status(ind, HSR_IND_STATUS_ACTIVE);
    fn_menu(ind, GTK_MENU(hsr_tray_menu));

    if (fn_ind_set_title != NULL) {
        fn_ind_set_title(ind, tooltip != NULL ? tooltip : "Helwan Screen Recorder");
    }

    hsr_ind_lib = handle;
    hsr_indicator = ind;
    fn_ind_set_status = fn_status;
    hsr_using_appindicator = 1;
    return 1;
}
#endif

// Returns 1 on success, 0 if the tray could not be created.
static int hsr_tray_init(const char *icon_path, const char *tooltip, int debug) {
    if (hsr_tray_icon != NULL || hsr_indicator != NULL) {
        return 1;
    }

    hsr_tray_fallback_icon = (char *)hsr_resolve_icon_name("applications-multimedia");
    if (icon_path != NULL && icon_path[0] != '\0') {
        g_free(hsr_tray_icon_path);
        hsr_tray_icon_path = g_strdup(icon_path);
    }

    hsr_tray_menu = gtk_menu_new();
    hsr_tray_add_item("Show Window", HSR_TRAY_SHOW);
    hsr_tray_add_separator();
    hsr_tray_item_start = hsr_tray_add_item("Start Recording", HSR_TRAY_START);
    hsr_tray_item_pause = hsr_tray_add_item("Pause", HSR_TRAY_PAUSE);
    hsr_tray_item_stop = hsr_tray_add_item("Stop and Save", HSR_TRAY_STOP);
    hsr_tray_add_separator();
    hsr_tray_add_item("Quit", HSR_TRAY_QUIT);
    gtk_widget_show_all(hsr_tray_menu);
    gtk_widget_set_sensitive(hsr_tray_item_pause, FALSE);

#if defined(__linux__) || defined(__unix__)
    if (hsr_try_init_appindicator(icon_path, tooltip, debug)) {
        return 1;
    }
#endif

    if (hsr_tray_icon_path != NULL && hsr_tray_icon_path[0] != '\0') {
        hsr_tray_icon = gtk_status_icon_new_from_file(hsr_tray_icon_path);
    } else {
        hsr_tray_icon = gtk_status_icon_new_from_icon_name(hsr_tray_fallback_icon);
    }

    if (hsr_tray_icon == NULL) {
        if (debug) {
            fprintf(stderr, "[hsr] gtk_status_icon creation returned NULL\n");
        }
        return 0;
    }

    gtk_status_icon_set_title(hsr_tray_icon, "Helwan Screen Recorder");
    gtk_status_icon_set_tooltip_text(hsr_tray_icon, tooltip);
    gtk_status_icon_set_visible(hsr_tray_icon, TRUE);

    g_signal_connect(hsr_tray_icon, "activate",
                     G_CALLBACK(hsr_tray_activate_cb), NULL);
    g_signal_connect(hsr_tray_icon, "popup-menu",
                     G_CALLBACK(hsr_tray_popup_cb), NULL);

    if (debug) {
        fprintf(stderr, "[hsr] using GtkStatusIcon fallback (embedded=%d)\n",
                gtk_status_icon_is_embedded(hsr_tray_icon) ? 1 : 0);
    }

    return 1;
}

static int hsr_tray_take_action(void) {
    int action = hsr_tray_pending;
    hsr_tray_pending = HSR_TRAY_NONE;
    return action;
}

// state: 0 = ready, 1 = recording, 2 = paused
static void hsr_tray_set_state(int state, const char *tooltip) {
    if (hsr_using_appindicator && hsr_indicator != NULL) {
        if (fn_ind_set_status != NULL) {
            fn_ind_set_status(hsr_indicator, state == 1 ? HSR_IND_STATUS_ATTENTION : HSR_IND_STATUS_ACTIVE);
        }

        if (state == 1) {
            const char *rec_icon = hsr_resolve_icon_name("media-record");
            if (fn_ind_set_attention_icon_full != NULL) {
                fn_ind_set_attention_icon_full(hsr_indicator, rec_icon, "Recording");
            } else if (fn_ind_set_icon_full != NULL) {
                fn_ind_set_icon_full(hsr_indicator, rec_icon, "Recording");
            }
        } else if (state == 2) {
            const char *pause_icon = hsr_resolve_icon_name("media-playback-pause");
            if (fn_ind_set_icon_full != NULL) {
                fn_ind_set_icon_full(hsr_indicator, pause_icon, "Paused");
            }
        } else {
            const char *ready_icon = (hsr_tray_icon_path != NULL && hsr_tray_icon_path[0] != '\0')
                ? hsr_tray_icon_path
                : hsr_resolve_icon_name(hsr_tray_fallback_icon);
            if (fn_ind_set_icon_full != NULL) {
                fn_ind_set_icon_full(hsr_indicator, ready_icon, "Ready");
            }
        }

        if (fn_ind_set_title != NULL && tooltip != NULL) {
            fn_ind_set_title(hsr_indicator, tooltip);
        }
    } else if (hsr_tray_icon != NULL) {
        if (state == 1) {
            gtk_status_icon_set_from_icon_name(hsr_tray_icon, hsr_resolve_icon_name("media-record"));
        } else if (state == 2) {
            gtk_status_icon_set_from_icon_name(hsr_tray_icon, hsr_resolve_icon_name("media-playback-pause"));
        } else if (hsr_tray_icon_path != NULL && hsr_tray_icon_path[0] != '\0') {
            gtk_status_icon_set_from_file(hsr_tray_icon, hsr_tray_icon_path);
        } else {
            gtk_status_icon_set_from_icon_name(hsr_tray_icon, hsr_resolve_icon_name(hsr_tray_fallback_icon));
        }

        gtk_status_icon_set_tooltip_text(hsr_tray_icon, tooltip);
    }

    if (hsr_tray_item_start != NULL) {
        gtk_widget_set_sensitive(hsr_tray_item_start, state == 0);
    }
    if (hsr_tray_item_pause != NULL) {
        gtk_widget_set_sensitive(hsr_tray_item_pause, state != 0);
        gtk_menu_item_set_label(GTK_MENU_ITEM(hsr_tray_item_pause),
                                state == 2 ? "Resume" : "Pause");
    }
    if (hsr_tray_item_stop != NULL) {
        gtk_widget_set_sensitive(hsr_tray_item_stop, TRUE);
    }
}

static void hsr_tray_set_tooltip(const char *tooltip) {
    if (hsr_using_appindicator && hsr_indicator != NULL) {
        if (fn_ind_set_title != NULL && tooltip != NULL) {
            fn_ind_set_title(hsr_indicator, tooltip);
        }
        return;
    }

    if (hsr_tray_icon != NULL) {
        gtk_status_icon_set_tooltip_text(hsr_tray_icon, tooltip);
    }
}

static void hsr_tray_destroy(void) {
    if (hsr_tray_icon != NULL) {
        gtk_status_icon_set_visible(hsr_tray_icon, FALSE);
        g_object_unref(hsr_tray_icon);
        hsr_tray_icon = NULL;
    }

#if defined(__linux__) || defined(__unix__)
    if (hsr_indicator != NULL) {
        if (fn_ind_set_status != NULL) {
            fn_ind_set_status(hsr_indicator, HSR_IND_STATUS_PASSIVE);
        }
        g_object_unref(hsr_indicator);
        hsr_indicator = NULL;
    }
    if (hsr_ind_lib != NULL) {
        dlclose(hsr_ind_lib);
        hsr_ind_lib = NULL;
    }
#endif

    if (hsr_tray_icon_path != NULL) {
        g_free(hsr_tray_icon_path);
        hsr_tray_icon_path = NULL;
    }
}
*/
import "C"

import (
	"os"
	"path/filepath"
	"unsafe"

	"github.com/gotk3/gotk3/glib"
)

const (
	trayActionShow  = 1
	trayActionStart = 2
	trayActionPause = 3
	trayActionStop  = 4
	trayActionQuit  = 5

	trayPollMs = 150
)

// findTrayIconPath looks for the application icon in the same places as the
// window icon. An empty result makes the tray use a theme icon.
func findTrayIconPath() string {
	var paths []string

	if exePath, err := os.Executable(); err == nil {
		paths = append(paths,
			filepath.Join(filepath.Dir(exePath), "assets", "icon.png"),
		)
	}

	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths,
			filepath.Join(cwd, "assets", "icon.png"),
		)
	}

	paths = append(paths,
		"/usr/share/icons/hicolor/128x128/apps/hel-recorder.png",
	)

	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}

	return ""
}

// SetupTray creates the tray icon.
func SetupTray(manager *RecorderManager, ui *AppUI, onQuit func()) {
	iconPath := C.CString(findTrayIconPath())
	tooltip := C.CString("Helwan Screen Recorder")

	defer C.free(unsafe.Pointer(iconPath))
	defer C.free(unsafe.Pointer(tooltip))

	debugFlag := C.int(0)
	if debugEnabled {
		debugFlag = 1
	}

	if C.hsr_tray_init(iconPath, tooltip, debugFlag) == 0 {
		debugf("tray initialization failed")
		return
	}

	glib.TimeoutAdd(trayPollMs, func() bool {
		switch int(C.hsr_tray_take_action()) {
		case trayActionShow:
			ui.Window.Deiconify()
			ui.Window.Present()

		case trayActionStart:
			manager.StartRecording()

		case trayActionPause:
			manager.PauseResumeRecording()

		case trayActionStop:
			manager.StopRecording()

		case trayActionQuit:
			if onQuit != nil {
				onQuit()
			}
		}

		return true
	})
}

// trayApplyState updates the icon and menu. state is "recording", "paused" or
// anything else for ready. GTK main thread only.
func trayApplyState(state string) {
	code := 0
	tip := "Helwan Screen Recorder"

	switch state {
	case "recording":
		code = 1
		tip += " - Recording"

	case "paused":
		code = 2
		tip += " - Paused"
	}

	cTip := C.CString(tip)
	defer C.free(unsafe.Pointer(cTip))

	C.hsr_tray_set_state(C.int(code), cTip)
}

// updateTrayTooltip shows the elapsed time in the tray tooltip. Safe to call
// from any goroutine.
func updateTrayTooltip(timeStr string, paused bool) {
	text := "Helwan Screen Recorder - Recording " + timeStr
	if paused {
		text = "Helwan Screen Recorder - Paused " + timeStr
	}

	glib.IdleAdd(func() {
		cText := C.CString(text)
		defer C.free(unsafe.Pointer(cText))

		C.hsr_tray_set_tooltip(cText)
	})
}

// ShutdownTray removes the icon. GTK main thread only.
func ShutdownTray() {
	C.hsr_tray_destroy()
}

