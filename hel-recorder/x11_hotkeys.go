
package main

/*
#cgo LDFLAGS: -lX11

#include <X11/Xlib.h>
#include <X11/keysym.h>
#include <stdlib.h>

static int hsr_grab_error = 0;

static int hsr_x_error_handler(Display *display, XErrorEvent *event) {
	if (event->error_code == BadAccess) {
		hsr_grab_error = 1;
	}
	return 0;
}

static Display* hsr_open_and_grab(void) {
	Display *display = XOpenDisplay(NULL);
	if (display == NULL) {
		return NULL;
	}

	Window root = DefaultRootWindow(display);

	KeyCode keys[3];
	keys[0] = XKeysymToKeycode(display, XK_r);
	keys[1] = XKeysymToKeycode(display, XK_s);
	keys[2] = XKeysymToKeycode(display, XK_g);

	for (int i = 0; i < 3; i++) {
		if (keys[i] == 0) {
			XCloseDisplay(display);
			return NULL;
		}
	}

	unsigned int modifiers[] = {
		ControlMask,
		ControlMask | LockMask,
		ControlMask | Mod2Mask,
		ControlMask | LockMask | Mod2Mask
	};

	hsr_grab_error = 0;

	int (*old_handler)(Display *, XErrorEvent *);
	old_handler = XSetErrorHandler(hsr_x_error_handler);

	for (int i = 0; i < 3; i++) {
		for (int j = 0; j < 4; j++) {
			XGrabKey(
				display,
				keys[i],
				modifiers[j],
				root,
				False,
				GrabModeAsync,
				GrabModeAsync
			);
		}
	}

	XSync(display, False);
	XSetErrorHandler(old_handler);

	if (hsr_grab_error) {
		for (int i = 0; i < 3; i++) {
			for (int j = 0; j < 4; j++) {
				XUngrabKey(display, keys[i], modifiers[j], root);
			}
		}
		XSync(display, False);
		XCloseDisplay(display);
		return NULL;
	}

	return display;
}

static int hsr_poll_hotkey(Display *display) {
	if (display == NULL || XPending(display) == 0) {
		return 0;
	}

	XEvent event;
	XNextEvent(display, &event);

	if (event.type != KeyPress) {
		return 0;
	}

	if (!(event.xkey.state & ControlMask)) {
		return 0;
	}

	KeySym key = XLookupKeysym(&event.xkey, 0);

	switch (key) {
		case XK_r:
		case XK_R:
			return 1;
		case XK_s:
		case XK_S:
			return 2;
		case XK_g:
		case XK_G:
			return 3;
	}

	return 0;
}

static void hsr_close_hotkeys(Display *display) {
	if (display == NULL) {
		return;
	}

	Window root = DefaultRootWindow(display);

	KeyCode keys[3];
	keys[0] = XKeysymToKeycode(display, XK_r);
	keys[1] = XKeysymToKeycode(display, XK_s);
	keys[2] = XKeysymToKeycode(display, XK_g);

	unsigned int modifiers[] = {
		ControlMask,
		ControlMask | LockMask,
		ControlMask | Mod2Mask,
		ControlMask | LockMask | Mod2Mask
	};

	for (int i = 0; i < 3; i++) {
		for (int j = 0; j < 4; j++) {
			XUngrabKey(display, keys[i], modifiers[j], root);
		}
	}

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

type GlobalHotkeys struct {
	display *C.Display
	stop    chan struct{}
	done    chan struct{}
	once    sync.Once
}

func StartGlobalHotkeys(manager *RecorderManager) (*GlobalHotkeys, error) {
	display := C.hsr_open_and_grab()
	if display == nil {
		return nil, fmt.Errorf("cannot register global X11 shortcuts; they may already be in use")
	}

	hotkeys := &GlobalHotkeys{
		display: display,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}

	go hotkeys.run(manager)

	return hotkeys, nil
}

func (h *GlobalHotkeys) run(manager *RecorderManager) {
	defer close(h.done)
	defer C.hsr_close_hotkeys(h.display)

	for {
		select {
		case <-h.stop:
			return
		default:
		}

		key := int(C.hsr_poll_hotkey(h.display))

		if key == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		glib.IdleAdd(func() {
			switch key {
			case 1:
				manager.ToggleRecordPause()
			case 2:
				manager.StopRecording()
			case 3:
				manager.ToggleMagnifier()
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

