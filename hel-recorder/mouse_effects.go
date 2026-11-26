// mouse_effects.go
package main

import (
	"math"
	"time"

	"github.com/gotk3/gotk3/cairo"
	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

const (
	mouseEffectsTickMs    = 16
	mouseHaloRadius       = 22.0
	mouseHaloLineWidth    = 4.0
	mouseClickDuration    = 450 * time.Millisecond
	mouseClickStartRadius = 8.0
	mouseClickEndRadius   = 38.0
)

type mouseClickEffect struct {
	x, y    float64
	started time.Time
}

type MouseEffects struct {
	window  *gtk.Window
	area    *gtk.DrawingArea
	root    *gdk.Window
	pointer *gdk.Device

	active      bool
	lastButtons gdk.ModifierType
	pointerX    float64
	pointerY    float64

	clicks []mouseClickEffect
	timer  glib.SourceHandle
}

func newMouseEffects() *MouseEffects {
	return &MouseEffects{}
}

func (e *MouseEffects) ensureWindow() error {
	if e.window != nil {
		return nil
	}

	screen, err := gdk.ScreenGetDefault()
	if err != nil {
		return err
	}

	root, err := screen.GetRootWindow()
	if err != nil {
		return err
	}

	display, err := screen.GetDisplay()
	if err != nil {
		return err
	}

	seat, err := display.GetDefaultSeat()
	if err != nil || seat == nil {
		return errMouseEffectsUnavailable(
			"X11 pointer device is unavailable",
		)
	}

	pointer, err := seat.GetPointer()
	if err != nil {
		return err
	}

	if !screen.IsComposited() {
		return errMouseEffectsUnavailable(
			"desktop compositing is required for mouse effects",
		)
	}

	visual, err := screen.GetRGBAVisual()
	if err != nil || visual == nil {
		return errMouseEffectsUnavailable(
			"transparent X11 windows are unavailable",
		)
	}

	width := root.WindowGetWidth()
	height := root.WindowGetHeight()

	win, err := gtk.WindowNew(gtk.WINDOW_POPUP)
	if err != nil {
		return err
	}

	win.SetDecorated(false)
	win.SetResizable(false)
	win.SetAppPaintable(true)
	win.SetVisual(visual)
	win.SetKeepAbove(true)
	win.SetSkipTaskbarHint(true)
	win.SetSkipPagerHint(true)
	win.SetAcceptFocus(false)
	win.SetDefaultSize(width, height)
	win.Move(0, 0)

	area, err := gtk.DrawingAreaNew()
	if err != nil {
		win.Destroy()
		return err
	}

	area.SetSizeRequest(width, height)

	area.Connect("draw", func(
		_ *gtk.DrawingArea,
		cr *cairo.Context,
	) bool {
		e.draw(cr)
		return true
	})

	win.Add(area)

	e.window = win
	e.area = area
	e.root = root
	e.pointer = pointer

	return nil
}

func (e *MouseEffects) Toggle() error {
	if e.active {
		e.Disable()
		return nil
	}

	if err := e.Enable(); err != nil {
		debugf("mouse effects: %v", err)
		return err
	}

	return nil
}

func (e *MouseEffects) Enable() error {
	if e.active {
		return nil
	}

	if err := e.ensureWindow(); err != nil {
		return err
	}

	e.active = true
	e.lastButtons = 0
	e.clicks = nil

	e.window.ShowAll()

	// The overlay must never intercept clicks or pointer movement.
	if gdkWindow, err := e.window.GetWindow(); err == nil {
		gdkWindow.SetPassThrough(true)
	}

	e.timer = glib.TimeoutAdd(mouseEffectsTickMs, func() bool {
		if !e.active {
			return false
		}

		e.updatePointer()
		e.area.QueueDraw()

		return true
	})

	e.updatePointer()
	e.area.QueueDraw()

	return nil
}

func (e *MouseEffects) Disable() {
	if !e.active {
		return
	}

	e.active = false

	if e.timer != 0 {
		glib.SourceRemove(e.timer)
		e.timer = 0
	}

	e.clicks = nil

	if e.window != nil {
		e.window.Hide()
	}
}

func (e *MouseEffects) Stop() {
	e.Disable()

	if e.window != nil {
		e.window.Destroy()
		e.window = nil
		e.area = nil
		e.root = nil
		e.pointer = nil
	}
}

func (e *MouseEffects) updatePointer() {
	if e.root == nil || e.pointer == nil {
		return
	}

	_, x, y, buttons := e.root.GetDevicePosition(e.pointer)

	e.pointerX = float64(x)
	e.pointerY = float64(y)

	buttonMask := buttons & (
		gdk.BUTTON1_MASK |
			gdk.BUTTON2_MASK |
			gdk.BUTTON3_MASK |
			gdk.BUTTON4_MASK |
			gdk.BUTTON5_MASK)

	previous := e.lastButtons
	pressed := buttonMask &^ previous

	if pressed != 0 {
		e.clicks = append(e.clicks, mouseClickEffect{
			x:       e.pointerX,
			y:       e.pointerY,
			started: time.Now(),
		})
	}

	e.lastButtons = buttonMask

	now := time.Now()
	kept := e.clicks[:0]

	for _, click := range e.clicks {
		if now.Sub(click.started) < mouseClickDuration {
			kept = append(kept, click)
		}
	}

	e.clicks = kept
}

func (e *MouseEffects) draw(cr *cairo.Context) {
	// Clear the transparent overlay completely.
	cr.Save()
	cr.SetOperator(cairo.OPERATOR_SOURCE)
	cr.SetSourceRGBA(0, 0, 0, 0)
	cr.Paint()
	cr.Restore()

	if !e.active {
		return
	}

	// Cursor Halo.
	cr.Save()
	cr.SetOperator(cairo.OPERATOR_OVER)
	cr.SetSourceRGBA(0.10, 0.78, 0.88, 0.90)
	cr.SetLineWidth(mouseHaloLineWidth)

	cr.Arc(
		e.pointerX,
		e.pointerY,
		mouseHaloRadius,
		0,
		2*math.Pi,
	)

	cr.Stroke()
	cr.Restore()

	// Click Highlight.
	now := time.Now()

	for _, click := range e.clicks {
		progress := float64(now.Sub(click.started)) /
			float64(mouseClickDuration)

		if progress < 0 {
			progress = 0
		}

		if progress > 1 {
			progress = 1
		}

		radius := mouseClickStartRadius +
			(mouseClickEndRadius-mouseClickStartRadius)*progress

		alpha := 0.85 * (1.0 - progress)

		cr.Save()
		cr.SetOperator(cairo.OPERATOR_OVER)
		cr.SetSourceRGBA(1.0, 0.78, 0.10, alpha)
		cr.SetLineWidth(5.0)

		cr.Arc(
			click.x,
			click.y,
			radius,
			0,
			2*math.Pi,
		)

		cr.Stroke()
		cr.Restore()
	}
}

type mouseEffectsError string

func (e mouseEffectsError) Error() string {
	return string(e)
}

func errMouseEffectsUnavailable(message string) error {
	return mouseEffectsError(message)
}

var mouseEffectsController = newMouseEffects()

func (m *RecorderManager) ToggleMouseEffects() {
	// Keep the "Mouse click effects" checkbox in step with the real state,
	// also when the overlay could not be started.
	defer m.syncMouseEffectsCheckbox()

	if err := mouseEffectsController.Toggle(); err != nil {
		m.setStatus(
			"Error: Mouse effects unavailable: " + err.Error(),
		)
		return
	}

	if mouseEffectsController.active {
		m.setStatus("Status: Mouse effects enabled")
	} else {
		m.setStatus("Status: Mouse effects disabled")
	}
}

func (m *RecorderManager) StopMouseEffects() {
	mouseEffectsController.Stop()
}