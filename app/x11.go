// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build linux,!android

package app

/*
Simple on-screen app debugging for X11. Not an officially supported
development target for apps, as screens with mice are very different
than screens with touch panels.
*/

/*
#cgo LDFLAGS: -lEGL -lGLESv2 -lX11

void createWindow(void);
void processEvents(void);
void swapBuffers(void);
*/
import "C"
import (
	"runtime"
	"time"

	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
	"golang.org/x/mobile/geom"
)

func init() {
	theApp.registerGLViewportFilter()
}

func main(f func(App)) {
	runtime.LockOSThread()

	workAvailable := theApp.worker.WorkAvailable()

	C.createWindow()

	// TODO: send lifecycle events when e.g. the X11 window is iconified or moved off-screen.
	theApp.sendLifecycle(lifecycle.StageFocused)

	// TODO: translate X11 expose events to shiny paint events, instead of
	// sending this synthetic paint event as a hack.
	theApp.eventsIn <- paint.Event{}

	donec := make(chan struct{})
	go func() {
		f(theApp)
		close(donec)
	}()

	// TODO: can we get the actual vsync signal?
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()
	var tc <-chan time.Time

	for {
		select {
		case <-donec:
			return
		case <-workAvailable:
			theApp.worker.DoWork()
		case <-theApp.publish:
			C.swapBuffers()
			tc = ticker.C
		case <-tc:
			tc = nil
			theApp.publishResult <- PublishResult{}
		}
		C.processEvents()
	}
}

//export onResize
func onResize(w, h int) {
	// TODO(nigeltao): don't assume 72 DPI. DisplayWidth and DisplayWidthMM
	// is probably the best place to start looking.
	pixelsPerPt := float32(1)
	theApp.eventsIn <- size.Event{
		WidthPx:     w,
		HeightPx:    h,
		WidthPt:     geom.Pt(w),
		HeightPt:    geom.Pt(h),
		PixelsPerPt: pixelsPerPt,
	}
}

func sendMouse(d mouse.Direction, x, y float32, button uint, state uint) {
	var b mouse.Button
	switch button {
	case 1:
		b = mouse.ButtonLeft
	case 2:
		b = mouse.ButtonMiddle
	case 3:
		b = mouse.ButtonRight
	case 4:
		b = mouse.ButtonWheelUp
	case 5:
		b = mouse.ButtonWheelDown
	case 6:
		b = mouse.ButtonWheelLeft
	case 7:
		b = mouse.ButtonWheelRight
	default:
		// Unknown/unsupported button
		b = mouse.ButtonNone
	}

	var m key.Modifiers
	if state&(1<<0) != 0 {
		m += key.ModShift
	}
	if state&(1<<2) != 0 {
		m += key.ModControl
	}
	if state&(1<<3) != 0 {
		m += key.ModAlt
	}
	if state&(1<<6) != 0 {
		m += key.ModMeta
	}

	theApp.eventsIn <- mouse.Event{
		X:         x,
		Y:         y,
		Button:    b,
		Direction: d,
		Modifiers: m,
	}
}

//export onMousePress
func onMousePress(x, y float32, button uint, state uint) {
	sendMouse(mouse.DirPress, x, y, button, state)
}

//export onMouseMove
func onMouseMove(x, y float32, state uint) { sendMouse(mouse.DirNone, x, y, 0, state) }

//export onMouseRelease
func onMouseRelease(x, y float32, button uint, state uint) {
	sendMouse(mouse.DirRelease, x, y, button, state)
}

var stopped bool

//export onStop
func onStop() {
	if stopped {
		return
	}
	stopped = true
	theApp.sendLifecycle(lifecycle.StageDead)
	theApp.eventsIn <- stopPumping{}
}
