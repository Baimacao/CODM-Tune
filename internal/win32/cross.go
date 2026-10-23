package win32

import (
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"codmtune/internal/ui"
)

// The crosshair is a second layered child process, deliberately separate from
// both the main window and the stats HUD. It is click-through and topmost.
const (
	crossClass = "CodmTuneCross"
	crossTimer = 7
)

type crossWindow struct {
	hwnd    uintptr
	surface *Surface
	render  func(*Canvas)
	alpha   uint8
	visible bool
}

var gCross *crossWindow

// RunCrosshair creates the layered crosshair window and blocks in its loop.
func RunCrosshair(w, h int, render func(*Canvas)) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := LoadUI(); err != nil {
		return err
	}
	scale := float32(1)
	if dpi, _, _ := pGetDpiForSystem.Call(); dpi > 96 {
		scale = float32(dpi) / 96
	}
	surf, err := NewSurface(int(float32(w)*scale), int(float32(h)*scale))
	if err != nil {
		return err
	}
	surf.Canvas.SetScale(scale)
	hinst, _, _ := pGetModuleHandleW.Call(0)
	cls, _ := syscall.UTF16FromString(crossClass)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		LpfnWndProc:   syscall.NewCallback(crossProc),
		HInstance:     hinst,
		HCursor:       loadCursor(0, IDC_ARROW),
		LpszClassName: &cls[0],
	}
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	// topmost | transparent | toolwindow | layered | noactivate
	ex := uintptr(0x8 | 0x20 | 0x80 | 0x80000 | 0x08000000)
	hwnd, _, e := pCreateWindowExW.Call(ex, uintptr(unsafe.Pointer(&cls[0])), 0,
		0x80000000, 0, 0, uintptr(int(float32(w)*scale)), uintptr(int(float32(h)*scale)), 0, 0, hinst, 0)
	if hwnd == 0 {
		surf.Close()
		return fmt.Errorf("crosshair window: %v", e)
	}
	gCross = &crossWindow{hwnd: hwnd, surface: surf, render: render, alpha: 220}
	CrossAlpha(220)
	CrossCenter()
	pSetTimer.Call(hwnd, crossTimer, 120, 0)
	var msg MSG
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	if gCross != nil {
		gCross.surface.Close()
		gCross = nil
	}
	return nil
}

func crossProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	o := gCross
	switch msg {
	case WM_TIMER:
		if wparam == crossTimer && o != nil && o.visible {
			crossPaint(o)
		}
		return 0
	case WM_DESTROY:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func crossPaint(o *crossWindow) {
	o.surface.Canvas.FillRect(ui.R(0, 0, float32(o.surface.W), float32(o.surface.H)), ui.RGB(0, 0, 0))
	if o.render != nil {
		o.render(o.surface.Canvas)
	}
	dc, _, _ := pGetDC.Call(o.hwnd)
	if dc != 0 {
		o.surface.Blit(dc)
		pReleaseDC.Call(o.hwnd, dc)
	}
}

// CrossAlpha sets crosshair opacity; black stays colour-keyed.
func CrossAlpha(a uint8) {
	if gCross == nil {
		return
	}
	gCross.alpha = a
	pSetLayeredWindowAttributes.Call(gCross.hwnd, 0x000000, uintptr(a), 0x00000003)
}

// CrossShow toggles visibility without stealing focus.
func CrossShow(v bool) {
	if gCross == nil || gCross.visible == v {
		return
	}
	gCross.visible = v
	if v {
		pShowWindow.Call(gCross.hwnd, 4)
		crossPaint(gCross)
	} else {
		pShowWindow.Call(gCross.hwnd, SW_HIDE)
	}
}

// CrossCenter puts the crosshair window in the middle of the screen.
func CrossCenter() {
	if gCross == nil {
		return
	}
	sw, _, _ := pGetSystemMetrics.Call(0)
	sh, _, _ := pGetSystemMetrics.Call(1)
	w, h := int32(gCross.surface.W), int32(gCross.surface.H)
	x, y := (int32(sw)-w)/2, (int32(sh)-h)/2
	pSetWindowPos.Call(gCross.hwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x0010)
}

var _ = time.Now
