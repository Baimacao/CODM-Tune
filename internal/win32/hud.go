package win32

import (
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"codmtune/internal/ui"
)

// The floating HUD lives in its own process. A second WS_EX_LAYERED window in
// the main process breaks GDI blits for the main window, so opacity is only
// safe here, where the HUD is the only window.

const (
	hudClass = "CodmTuneHUD"
	hudTimer = 5
)

type hudWindow struct {
	hwnd    uintptr
	surface *Surface
	render  func(*Canvas)
	digest  func() string
	alpha   uint8
	corner  string
	visible bool
	// sig is a cheap fingerprint of what the last painted frame showed. The
	// timer fires at a fixed 2 Hz, but a repaint only happens when the text
	// actually changed, so a static HUD costs one GDI+ pass every few
	// seconds instead of five per second.
	sig string
}

var gHUD *hudWindow

// RunHUD creates the layered HUD and blocks in its message loop. digest
// returns a short fingerprint of the currently rendered content: the 2 Hz timer
// only repaints when the fingerprint changed, so an idle HUD is free.
func RunHUD(w, h int, render func(*Canvas), digest func() string) error {
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
	cls, _ := syscall.UTF16FromString(hudClass)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		LpfnWndProc:   syscall.NewCallback(hudProc),
		HInstance:     hinst,
		HCursor:       loadCursor(0, IDC_ARROW),
		LpszClassName: &cls[0],
	}
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	ex := uintptr(0x00000008 | 0x00000020 | 0x00000080 | 0x08000000 | 0x00080000)
	hwnd, _, e := pCreateWindowExW.Call(ex, uintptr(unsafe.Pointer(&cls[0])), 0,
		0x80000000, 0, 0, uintptr(int(float32(w)*scale)), uintptr(int(float32(h)*scale)), 0, 0, hinst, 0)
	if hwnd == 0 {
		surf.Close()
		return fmt.Errorf("hud window: %v", e)
	}
	gHUD = &hudWindow{hwnd: hwnd, surface: surf, render: render, digest: digest, alpha: 220, corner: "tr"}
	HUDAlpha(220)
	HUDPosition("tr")
	pSetTimer.Call(hwnd, hudTimer, 200, 0)
	return hudMessageLoop(hwnd)
}

func hudMessageLoop(hwnd uintptr) error {
	var msg MSG
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	if gHUD != nil {
		gHUD.surface.Close()
		gHUD = nil
	}
	return nil
}

func hudProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	o := gHUD
	switch msg {
	case WM_TIMER:
		if wparam == hudTimer && o != nil && o.visible {
			// Repaint only when the numbers on screen actually changed.
			// A HUD that is showing the same FPS/RTT as last tick produces
			// no GDI+ work at all; this is what keeps it off the CPU while
			// the game is running.
			sig := ""
			if o.digest != nil {
				sig = o.digest()
			}
			if sig != o.sig || o.sig == "" {
				o.sig = sig
				hudPaint(o)
			}
		}
		return 0
	case WM_DESTROY:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func hudPaint(o *hudWindow) {
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

// HUDAlpha sets uniform opacity and keeps black corners transparent.
func HUDAlpha(a uint8) {
	if gHUD == nil {
		return
	}
	gHUD.alpha = a
	pSetLayeredWindowAttributes.Call(gHUD.hwnd, 0x000000, uintptr(a), 0x00000003)
}

// HUDShow shows or hides the HUD without stealing focus.
func HUDShow(v bool) {
	if gHUD == nil || gHUD.visible == v {
		return
	}
	gHUD.visible = v
	if v {
		pShowWindow.Call(gHUD.hwnd, 4)
		gHUD.sig = ""
		hudPaint(gHUD)
	} else {
		pShowWindow.Call(gHUD.hwnd, SW_HIDE)
	}
}

// HUDPosition moves the HUD to a screen corner.
func HUDPosition(corner string) {
	if gHUD == nil {
		return
	}
	gHUD.corner = corner
	sw, _, _ := pGetSystemMetrics.Call(0)
	sh, _, _ := pGetSystemMetrics.Call(1)
	const margin = int32(22)
	w, h := int32(gHUD.surface.W), int32(gHUD.surface.H)
	// CODM's minimap occupies roughly the top-right 150 px. Keeping the HUD
	// below it avoids covering the map while still leaving it out of the
	// crosshair area. Users can move it to another corner in settings.
	x, y := int32(sw)-w-margin, margin+170
	switch corner {
	case "tl":
		x, y = margin, margin+40
	case "br":
		x, y = int32(sw)-w-margin, int32(sh)-h-margin-40
	case "bl":
		x, y = margin, int32(sh)-h-margin-40
	}
	pSetWindowPos.Call(gHUD.hwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x0010)
}

func HUDPositionCustom(x, y int) {
	if gHUD == nil {
		return
	}
	gHUD.corner = "custom"
	sw, _, _ := pGetSystemMetrics.Call(0)
	sh, _, _ := pGetSystemMetrics.Call(1)
	w, h := int(gHUD.surface.W), int(gHUD.surface.H)
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x+w > int(sw) {
		x = int(sw) - w
	}
	if y+h > int(sh) {
		y = int(sh) - h
	}
	pSetWindowPos.Call(gHUD.hwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x0010)
}

var _ = time.Now
