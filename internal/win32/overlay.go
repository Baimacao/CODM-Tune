package win32

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"codmtune/internal/ui"
)

const (
	overlayClass = "CodmTuneOverlay"
	overlayTimer = 3
)

type overlayState struct {
	hwnd    uintptr
	surface *Surface
	W, H    int
	alpha   uint8
	corner  string
	visible bool
	render  func(*Canvas)
	last    time.Time
}

var gOverlay *overlayState

// OverlaySpec configures the floating HUD before Run enters the message loop.
type OverlaySpec struct {
	W, H   int
	Render func(*Canvas)
	Alpha  uint8
	Corner string
}

var pendingOverlay *OverlaySpec

// SetOverlay registers the HUD renderer; Run creates the window on its own
// locked UI thread.
func SetOverlay(spec *OverlaySpec) { pendingOverlay = spec }

// OverlayVisible shows or hides the floating HUD.
func OverlayVisible(v bool) {
	if gOverlay == nil || gOverlay.visible == v {
		return
	}
	gOverlay.visible = v
	if v {
		pShowWindow.Call(gOverlay.hwnd, 4) // SW_SHOWNOACTIVATE
	} else {
		pShowWindow.Call(gOverlay.hwnd, SW_HIDE)
	}
}

// OverlayStyle updates opacity, corner and size while running.
func OverlayStyle(alpha uint8, corner string) {
	if gOverlay == nil {
		return
	}
	if gOverlay.alpha == alpha && gOverlay.corner == corner {
		return
	}
	gOverlay.alpha = alpha
	gOverlay.corner = corner
	// applyOverlayAlpha()
	moveOverlay()
}

// OverlaySize resizes the HUD surface (compact vs full).
func OverlaySize(w, h int) {
	if gOverlay == nil || (gOverlay.W == w && gOverlay.H == h) {
		return
	}
	if s, err := NewSurface(w, h); err == nil {
		gOverlay.surface.Close()
		gOverlay.surface = s
		gOverlay.W, gOverlay.H = w, h
		moveOverlay()
	}
}

func createOverlay(spec *OverlaySpec) error {
	if spec == nil {
		return nil
	}
	hinst, _, _ := pGetModuleHandleW.Call(0)
	cls, _ := syscall.UTF16FromString(overlayClass)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		LpfnWndProc:   syscall.NewCallback(overlayProc),
		HInstance:     hinst,
		HCursor:       loadCursor(0, IDC_ARROW),
		LpszClassName: &cls[0],
	}
	if r, _, _ := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 && gOverlay == nil {
		// Class may already exist from a previous preview run.
	}
	surf, err := NewSurface(spec.W, spec.H)
	if err != nil {
		return err
	}
	ex := uintptr(0x00000008 | 0x00000020 | 0x00000080 | 0x08000000) // topmost|click-through|toolwindow|noactivate
	hwnd, _, e := pCreateWindowExW.Call(ex, uintptr(unsafe.Pointer(&cls[0])), 0,
		uintptr(0x80000000), 0, 0, uintptr(spec.W), uintptr(spec.H), 0, 0, hinst, 0)
	if hwnd == 0 {
		surf.Close()
		return fmt.Errorf("overlay window: %v", e)
	}
	alpha := spec.Alpha
	if alpha == 0 {
		alpha = 215
	}
	gOverlay = &overlayState{hwnd: hwnd, surface: surf, W: spec.W, H: spec.H, alpha: alpha, corner: spec.Corner, render: spec.Render, last: time.Now()}
	// No WS_EX_LAYERED: a second layered window makes the main window's GDI
	// blits fail in this process, so the HUD stays rectangular.
	// applyOverlayAlpha()
	moveOverlay()
	pSetTimer.Call(hwnd, overlayTimer, 200, 0)
	return nil
}

func overlayProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	o := gOverlay
	switch msg {
	case WM_TIMER:
		if wparam == overlayTimer && o != nil && o.visible {
			renderOverlay(o)
		}
		return 0
	case WM_PAINT:
		if o != nil && o.visible {
			renderOverlay(o)
		}
		return 0
	case WM_DESTROY:
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func renderOverlay(o *overlayState) {
	if o.render == nil {
		return
	}
	o.surface.Canvas.FillRect(ui.R(0, 0, float32(o.W), float32(o.H)), ui.RGB(0, 0, 0))
	o.render(o.surface.Canvas)
	dc, _, _ := pGetDC.Call(o.hwnd)
	if dc != 0 {
		o.surface.Blit(dc)
		pReleaseDC.Call(o.hwnd, dc)
	}
}

func applyOverlayAlpha() {
	if gOverlay == nil {
		return
	}
	pSetLayeredWindowAttributes.Call(gOverlay.hwnd, 0, uintptr(gOverlay.alpha), 0x00000002)
}

func moveOverlay() {
	if gOverlay == nil {
		return
	}
	sw, _, _ := pGetSystemMetrics.Call(0)
	sh, _, _ := pGetSystemMetrics.Call(1)
	const margin = 24
	x, y := int32(sw)-int32(gOverlay.W)-margin, int32(margin)
	switch gOverlay.corner {
	case "tl":
		x, y = margin, margin
	case "br":
		x, y = int32(sw)-int32(gOverlay.W)-margin, int32(sh)-int32(gOverlay.H)-margin-40
	case "bl":
		x, y = margin, int32(sh)-int32(gOverlay.H)-margin-40
	}
	pSetWindowPos.Call(gOverlay.hwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(gOverlay.W), uintptr(gOverlay.H), 0x0010|0x0004)
}
