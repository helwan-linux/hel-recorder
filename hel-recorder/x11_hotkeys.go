// x11_hotkeys.go
package main

/*
#cgo LDFLAGS: -lX11

#include <X11/Xlib.h>
#include <X11/keysym.h>
#include <stdlib.h>
#include <sys/select.h>

// Global shortcuts are Ctrl+<key> (R, E, Q, G, T).
#define HSR_KEY_COUNT 5
#define HSR_LOCK_VARIANTS 4
#define HSR_BASE_MODS (ControlMask)

// Order matters: the index + 1 is the value returned by hsr_wait_hotkey.
//   1 = start, 2 = pause/resume, 3 = stop, 4 = magnifier, 5 = mouse effects
static const KeySym hsr_keysyms[HSR_KEY_COUNT] = {
    XK_r, XK_e, XK_q, XK_g, XK_t
};

// CapsLock and NumLock must not prevent a shortcut from working.
static const unsigned int hsr_lock_variants[HSR_LOCK_VARIANTS] = {
    0, LockMask, Mod2Mask, LockMask | Mod2Mask
};

static KeyCode hsr_keycodes[HSR_KEY_COUNT];
static int hsr_grab_error = 0;

static int hsr_x_error_handler(Display *display, XErrorEvent *event) {
    (void)display;

    if (event->error_code == BadAccess) {
        hsr_grab_error = 1;
    }

    return 0;
}

static void hsr_set_grabs(Display *display, int grab) {
    Window root = DefaultRootWindow(display);

    for (int i = 0; i < HSR_KEY_COUNT; i++) {
        for (int j = 0; j < HSR_LOCK_VARIANTS; j++) {
            unsigned int mods =
                HSR_BASE_MODS | hsr_lock_variants[j];

            if (grab) {
                XGrabKey(
                    display,
                    hsr_keycodes[i],
                    mods,
                    root,
                    False,
                    GrabModeAsync,
                    GrabModeAsync
                );
            } else {
                XUngrabKey(
                    display,
                    hsr_keycodes[i],
                    mods,
                    root
                );
            }
        }
    }
}

static Display* hsr_open_and_grab(void) {
    Display *display = XOpenDisplay(NULL);

    if (display == NULL) {
        return NULL;
    }

    for (int i = 0; i < HSR_KEY_COUNT; i++) {
        hsr_keycodes[i] =
            XKeysymToKeycode(display, hsr_keysyms[i]);

        if (hsr_keycodes[i] == 0) {
            XCloseDisplay(display);
            return NULL;
        }
    }

    hsr_grab_error = 0;

    int (*old_handler)(Display *, XErrorEvent *);
    old_handler = XSetErrorHandler(
        hsr_x_error_handler
    );

    hsr_set_grabs(display, 1);

    XSync(display, False);
    XSetErrorHandler(old_handler);

    if (hsr_grab_error) {
        hsr_set_grabs(display, 0);
        XSync(display, False);
        XCloseDisplay(display);
        return NULL;
    }

    return display;
}

// Blocks for up to timeout_ms waiting for X events (no busy polling), and
// returns 1..5 for a recognised shortcut or 0 otherwise. Matching is done on
// keycodes, so it works regardless of the active keyboard layout (for example
// Arabic).
static int hsr_wait_hotkey(
    Display *display,
    int timeout_ms
) {
    if (display == NULL) {
        return 0;
    }

    if (XPending(display) == 0) {
        int fd = ConnectionNumber(display);
        fd_set fds;
        struct timeval tv;

        FD_ZERO(&fds);
        FD_SET(fd, &fds);

        tv.tv_sec = timeout_ms / 1000;
        tv.tv_usec =
            (timeout_ms % 1000) * 1000;

        if (select(
                fd + 1,
                &fds,
                NULL,
                NULL,
                &tv
            ) <= 0) {
            return 0;
        }
    }

    while (XPending(display) > 0) {
        XEvent event;
        XNextEvent(display, &event);

        if (event.type != KeyPress) {
            continue;
        }

        unsigned int state =
            event.xkey.state & HSR_BASE_MODS;

        if (state != HSR_BASE_MODS) {
            continue;
        }

        for (int i = 0; i < HSR_KEY_COUNT; i++) {
            if (event.xkey.keycode ==
                hsr_keycodes[i]) {
                return i + 1;
            }
        }
    }

    return 0;
}

static void hsr_close_hotkeys(Display *display) {
    if (display == NULL) {
        return;
    }

    hsr_set_grabs(display, 0);
    XSync(display, False);
    XCloseDisplay(display);
}
*/
import "C"

import (
	"fmt"
	"sync"
	"time"

	"github.com/gotk3/gotk3/glib"
)

const (
	hotkeyWaitMs = 100

	// Holding a shortcut makes X11 auto-repeat it; without this guard a held
	// Ctrl+E would toggle pause/resume dozens of times per second.
	hotkeyRepeatGuard = 800 * time.Millisecond
)

type GlobalHotkeys struct {
	display *C.Display
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

func StartGlobalHotkeys(
	manager *RecorderManager,
) (*GlobalHotkeys, error) {
	display := C.hsr_open_and_grab()

	if display == nil {
		return nil, fmt.Errorf(
			"cannot register global X11 shortcuts (Ctrl+R/E/Q/G/T); they may already be in use",
		)
	}

	hotkeys := &GlobalHotkeys{
		display: display,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}

	go hotkeys.run(manager)

	return hotkeys, nil
}

func (h *GlobalHotkeys) run(
	manager *RecorderManager,
) {
	defer close(h.done)
	defer C.hsr_close_hotkeys(h.display)

	var (
		lastKey int
		lastAt  time.Time
	)

	for {
		select {
		case <-h.stop:
			return

		default:
		}

		key := int(
			C.hsr_wait_hotkey(
				h.display,
				C.int(hotkeyWaitMs),
			),
		)

		if key == 0 {
			continue
		}

		now := time.Now()

		repeated :=
			key == lastKey &&
				now.Sub(lastAt) <
					hotkeyRepeatGuard

		lastKey = key
		lastAt = now

		if repeated {
			continue
		}

		glib.IdleAdd(func() {
			switch key {
			case 1:
				manager.StartRecording()

			case 2:
				manager.PauseResumeRecording()

			case 3:
				manager.StopRecording()

			case 4:
				manager.ToggleMagnifier()

			case 5:
				manager.ToggleMouseEffects()
			}
		})
	}
}

func (h *GlobalHotkeys) Stop() {
	if h == nil {
		return
	}

	h.once.Do(func() {
		close(h.stop)
		<-h.done
	})
}