package win32

import (
	"unsafe"

	"codmtune/internal/ui"
)

const (
	wsPopup     = 0x80000000
	wsVisible   = 0x10000000
	swpNoZOrder = 0x0004
	swpNoActivate = 0x0010
	swpFrameChanged = 0x0020
)

// ToggleFullscreen switches between the normal window and a borderless
// landscape presentation. The 1100x700 UI is scaled to fit the monitor and
// centered; the unused side bars stay black.
func ToggleFullscreen() bool {
	w := gWin
	if w == nil {
		return false
	}
	if !w.fs {
		w.savedStyle, _, _ = pGetWindowLongPtrW.Call(w.hwnd, ^uintptr(15)) // GWL_STYLE -16
		pGetWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&w.savedRect)))
		sw, _, _ := pGetSystemMetrics.Call(0)
		sh, _, _ := pGetSystemMetrics.Call(1)
		nw, nh := int(sw), int(sh)
		surf, err := NewSurface(nw, nh)
		if err != nil {
			return false
		}
		scale := float32(nw) / float32(w.app.W)
		if s := float32(nh) / float32(w.app.H); s < scale {
			scale = s
		}
		ox := int((float32(nw) - float32(w.app.W)*scale) / 2)
		oy := int((float32(nh) - float32(w.app.H)*scale) / 2)
		surf.Canvas.FillRect(ui.R(0, 0, float32(nw), float32(nh)), ui.CurrentTheme.Bg)
		surf.Canvas.SetView(scale, ox, oy)
		old := w.surface
		w.surface = surf
		old.Close()
		w.viewScale, w.offX, w.offY = scale, ox, oy
		w.fs = true
		pSetWindowLongPtrW.Call(w.hwnd, ^uintptr(15), w.savedStyle&^uintptr(0x00CF0000)|wsPopup|wsVisible)
		pSetWindowPos.Call(w.hwnd, ^uintptr(0), 0, 0, uintptr(nw), uintptr(nh), swpFrameChanged|swpNoActivate)
		w.present()
		ui.Invalidate()
		return true
	}
	// Restore the decorated window and the original DPI view.
	ns, err := NewSurface(int(float32(w.app.W)*w.baseScale), int(float32(w.app.H)*w.baseScale))
	if err == nil {
		ns.Canvas.FillRect(ui.R(0, 0, float32(ns.W), float32(ns.H)), ui.CurrentTheme.Bg)
		ns.Canvas.SetView(w.baseScale, 0, 0)
		old := w.surface
		w.surface = ns
		old.Close()
	}
	w.viewScale, w.offX, w.offY = w.baseScale, 0, 0
	pSetWindowLongPtrW.Call(w.hwnd, ^uintptr(15), w.savedStyle)
	pSetWindowPos.Call(w.hwnd, 0, uintptr(w.savedRect.Left), uintptr(w.savedRect.Top),
		uintptr(w.savedRect.Right-w.savedRect.Left), uintptr(w.savedRect.Bottom-w.savedRect.Top),
		swpFrameChanged|swpNoZOrder|swpNoActivate)
	w.fs = false
	w.present()
	ui.Invalidate()
	return false
}

// SetFullscreenFill moves the landscape viewport from centered letterbox to
// edge-to-edge. The terminal draws a wider logical canvas, so this lets its
// grid cover the physical width instead of leaving black side bars.
func SetFullscreenFill(fill bool) {
	w := gWin
	if w == nil || !w.fs || w.surface == nil {
		return
	}
	sw, _, _ := pGetSystemMetrics.Call(0)
	scale := w.viewScale
	if fill {
		w.offX = 0
	} else {
		w.offX = int((float32(sw) - float32(w.app.W)*scale) / 2)
	}
	w.surface.Canvas.SetView(scale, w.offX, 0)
	ui.Invalidate()
}

// Fullscreen reports the current presentation mode.
func Fullscreen() bool { return gWin != nil && gWin.fs }

var fullscreenHook func() bool

// SetFullscreenHook lets the UI swap to its terminal screen on the same
// transition. The hook must call ToggleFullscreen itself.
func SetFullscreenHook(fn func() bool) { fullscreenHook = fn }

func requestFullscreen() bool {
	if fullscreenHook != nil {
		return fullscreenHook()
	}
	return ToggleFullscreen()
}
