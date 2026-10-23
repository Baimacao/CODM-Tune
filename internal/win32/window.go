package win32

import (
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"codmtune/internal/spring"
	"codmtune/internal/ui"
)

const (
	WM_APP_TICK = WM_APP + 1
	WM_APP_TRAY = WM_APP + 2
	WM_APP_SHOW = WM_APP + 3
	timerTick   = 1
)

type window struct {
	hwnd       uintptr
	surface    *Surface
	app        *ui.App
	last       time.Time
	hidden     bool
	interval   int
	tray       bool
	scale      float32
	baseScale  float32
	viewScale  float32
	offX, offY int
	fs         bool
	savedStyle uintptr
	savedRect  RECT
}

var gWin *window

// Tray hooks are installed by appui so the Win32 layer does not need to know
// about Control or Config.
var (
	trayState  func() (overlay, crosshair bool)
	trayToggle func(kind string)
	traySound  func()
)

// SetTrayHooks wires the tray menu and the tray sound to the app layer.
func SetTrayHooks(state func() (bool, bool), toggle func(string), sound func()) {
	trayState, trayToggle, traySound = state, toggle, sound
}

// Run creates the window and enters the message loop. It must run on the
// main goroutine; the caller has already called ui.NewApp.
func Run(title string, app *ui.App) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// PER_MONITOR_AWARE_V2: no bitmap scaling by Windows. The surface is then
	// rendered at the monitor's physical pixel size and the canvas is scaled.
	pSetProcessDpiAwarenessContext.Call(^uintptr(3)) // -4
	pSetProcessDPIAware.Call()
	pTimeBeginPeriod.Call(1)
	defer pTimeEndPeriod.Call(1)

	if err := LoadUI(); err != nil {
		return err
	}
	defer StopUI()

	scale := float32(1)
	if dpi, _, _ := pGetDpiForSystem.Call(); dpi > 96 {
		scale = float32(dpi) / 96
	}
	surf, err := NewSurface(int(float32(app.W)*scale), int(float32(app.H)*scale))
	if err != nil {
		return err
	}
	defer surf.Close()
	surf.Canvas.SetScale(scale)
	hinst, _, _ := pGetModuleHandleW.Call(0)
	clsName, _ := syscall.UTF16FromString("CodmTuneWnd")
	titlep, _ := syscall.UTF16FromString(title)

	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         CS_HREDRAW | CS_VREDRAW,
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     hinst,
		HCursor:       loadCursor(0, IDC_ARROW),
		HIcon:         loadAppIcon(),
		HIconSm:       loadAppIcon(),
		LpszClassName: &clsName[0],
	}
	if r, _, e := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassExW: %v", e)
	}

	style := uint32(WS_POPUP | WS_MINIMIZEBOX | WS_SYSMENU)
	rc := RECT{0, 0, int32(float32(app.W) * scale), int32(float32(app.H) * scale)}
	pAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&rc)), uintptr(style), 0, 0)
	winW := rc.Right - rc.Left
	winH := rc.Bottom - rc.Top

	hwnd, _, e := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(&clsName[0])), uintptr(unsafe.Pointer(&titlep[0])),
		uintptr(style|WS_VISIBLE), CW_USEDEFAULT, CW_USEDEFAULT, uintptr(winW), uintptr(winH), 0, 0, hinst, 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowExW: %v", e)
	}

	// Center on the work area (not the raw screen) so the first frame does not
	// land under the taskbar or in the top-left corner on a new profile.
	work := RECT{}
	if r, _, _ := pSystemParametersInfoW.Call(0x0030 /*SPI_GETWORKAREA*/, 0, uintptr(unsafe.Pointer(&work)), 0); r != 0 {
		x := work.Left + (work.Right-work.Left-winW)/2
		y := work.Top + (work.Bottom-work.Top-winH)/2
		pSetWindowPos.Call(hwnd, 0, uintptr(x), uintptr(y), 0, 0, 0x0001|0x0004|0x0010)
	}

	gWin = &window{hwnd: hwnd, surface: surf, app: app, last: time.Now(), scale: scale, baseScale: scale, viewScale: scale}
	setDarkTitle(hwnd)
	if icon := loadAppIcon(); icon != 0 {
		pSendMessageW.Call(hwnd, 0x0080 /*WM_SETICON*/, 1 /*ICON_BIG*/, icon)
		pSendMessageW.Call(hwnd, 0x0080, 0 /*ICON_SMALL*/, icon)
	}

	pShowWindow.Call(hwnd, SW_SHOW)
	pUpdateWindow.Call(hwnd)
	setDarkTitle(hwnd)
	gWin.setTimer(16)

	var msg MSG
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	return nil
}

func setDarkTitle(hwnd uintptr) {
	dark := int32(1)
	// 20 on Windows 10 20H1+, 19 on 1809–1903; unknown attributes fail quietly.
	pDwmSetWindowAttribute.Call(hwnd, 20, uintptr(unsafe.Pointer(&dark)), 4)
	pDwmSetWindowAttribute.Call(hwnd, 19, uintptr(unsafe.Pointer(&dark)), 4)
}

func (w *window) setTimer(ms int) {
	if w.interval == ms {
		return
	}
	w.interval = ms
	pSetTimer.Call(w.hwnd, timerTick, uintptr(ms), 0)
}

// Wake schedules one frame soon. Background scans use it so their progress
// animation does not wait for the idle 140 ms timer.
func Wake() {
	if gWin == nil || gWin.hwnd == 0 {
		return
	}
	pPostMessageW.Call(gWin.hwnd, WM_APP_TICK, 0, 0)
}

func (w *window) px(v int32) float32 {
	if w.viewScale <= 0 {
		return float32(v)
	}
	return (float32(v) - float32(w.offX)) / w.viewScale
}

func (w *window) tick() {
	if w.hidden {
		return
	}
	now := time.Now()
	dt := float32(now.Sub(w.last).Seconds())
	w.last = now
	if dt > 0.1 {
		dt = 0.1
	}
	drew := w.app.Frame(w.surface.Canvas, dt)
	if drew {
		w.drawChrome()
		w.present()
	}
	if spring.Awake() == 0 && !drew {
		w.setTimer(140)
	} else {
		w.setTimer(16)
	}
}

func (w *window) present() {
	dc, _, _ := pGetDC.Call(w.hwnd)
	if dc != 0 {
		w.surface.Blit(dc)
		pReleaseDC.Call(w.hwnd, dc)
	}
}

func (w *window) hideToTray() {
	if w.hidden {
		return
	}
	w.hidden = true
	w.setTimer(0)
	pKillTimer.Call(w.hwnd, timerTick)
	pShowWindow.Call(w.hwnd, SW_HIDE)
	w.addTray()
	if traySound != nil {
		traySound()
	}
}

func (w *window) restore() {
	w.hidden = false
	w.removeTray()
	w.last = time.Now()
	pShowWindow.Call(w.hwnd, SW_SHOW)
	pSetForegroundWindow.Call(w.hwnd)
	w.setTimer(16)
}

func (w *window) addTray() {
	if w.tray {
		return
	}
	nid := NOTIFYICONDATAW{
		CbSize:           uint32(unsafe.Sizeof(NOTIFYICONDATAW{})),
		HWnd:             w.hwnd,
		UID:              1,
		UFlags:           NIF_MESSAGE | NIF_ICON | NIF_TIP,
		UCallbackMessage: WM_APP_TRAY,
		HIcon:            loadAppIcon(),
	}
	copyTip(nid.SzTip[:], "CODM Tune - 最小化到托盘")
	pShellNotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&nid)))
	w.tray = true
}

func (w *window) removeTray() {
	if !w.tray {
		return
	}
	nid := NOTIFYICONDATAW{
		CbSize: uint32(unsafe.Sizeof(NOTIFYICONDATAW{})),
		HWnd:   w.hwnd,
		UID:    1,
	}
	pShellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&nid)))
	w.tray = false
}

func (w *window) trayMenu() {
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	overlay, cross := false, false
	if trayState != nil {
		overlay, cross = trayState()
	}
	show, _ := syscall.UTF16FromString("显示主界面")
	overlayTxt, _ := syscall.UTF16FromString("统计悬浮窗")
	crossTxt, _ := syscall.UTF16FromString("辅助准心")
	folderTxt, _ := syscall.UTF16FromString("打开配置目录")
	aboutTxt, _ := syscall.UTF16FromString("关于 CODM Tune")
	quit, _ := syscall.UTF16FromString("退出")
	title, _ := syscall.UTF16FromString("CODM Tune · 已最小化到托盘")
	pAppendMenuW.Call(menu, MF_STRING|MF_DISABLED|MF_GRAYED, 0, uintptr(unsafe.Pointer(&title[0])))
	pAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)
	pAppendMenuW.Call(menu, MF_STRING|MF_DEFAULT, 1, uintptr(unsafe.Pointer(&show[0])))
	pAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)
	flags := uintptr(MF_STRING)
	if overlay {
		flags |= MF_CHECKED
	}
	pAppendMenuW.Call(menu, flags, 2, uintptr(unsafe.Pointer(&overlayTxt[0])))
	flags = uintptr(MF_STRING)
	if cross {
		flags |= MF_CHECKED
	}
	pAppendMenuW.Call(menu, flags, 3, uintptr(unsafe.Pointer(&crossTxt[0])))
	pAppendMenuW.Call(menu, MF_STRING, 4, uintptr(unsafe.Pointer(&folderTxt[0])))
	pAppendMenuW.Call(menu, MF_STRING, 6, uintptr(unsafe.Pointer(&aboutTxt[0])))
	pAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0)
	pAppendMenuW.Call(menu, MF_STRING, 5, uintptr(unsafe.Pointer(&quit[0])))
	var pt POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(w.hwnd)
	cmd, _, _ := pTrackPopupMenu.Call(menu, TPM_RETURNCMD|TPM_RIGHTBUTTON, uintptr(pt.X), uintptr(pt.Y), 0, w.hwnd, 0)
	pDestroyMenu.Call(menu)
	switch cmd {
	case 1:
		w.restore()
	case 2:
		if trayToggle != nil {
			trayToggle("overlay")
		}
	case 3:
		if trayToggle != nil {
			trayToggle("crosshair")
		}
	case 4:
		if trayToggle != nil {
			trayToggle("config")
		}
	case 6:
		w.restore()
		if trayToggle != nil {
			trayToggle("about")
		}
	case 5:
		pDestroyWindow.Call(w.hwnd)
	}
}

func copyTip(dst []uint16, s string) {
	src := syscall.StringToUTF16(s)
	if len(src) > len(dst) {
		src = src[:len(dst)]
	}
	copy(dst, src)
}

func loadCursor(h uintptr, id uintptr) uintptr {
	r, _, _ := pLoadCursorW.Call(h, id)
	return r
}

func loadIcon(h uintptr, id uintptr) uintptr {
	r, _, _ := pLoadIconW.Call(h, id)
	return r
}

func wndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	w := gWin
	if w == nil {
		r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
		return r
	}
	switch msg {
	case WM_ERASEBKGND:
		return 1
	case WM_PAINT:
		var ps PAINTSTRUCT
		pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		w.present()
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0
	case WM_TIMER:
		if wparam == timerTick {
			w.tick()
		}
		return 0
	case WM_APP_TICK:
		w.last = time.Now()
		w.setTimer(16)
		w.tick()
		return 0
	case WM_SIZE:
		if wparam == SIZE_MINIMIZED {
			w.hideToTray()
		}
		return 0
	case WM_KEYDOWN, WM_SYSKEYDOWN:
		if wparam == 0x7A { // F11 landscape presentation
			requestFullscreen()
			return 0
		}
		if k, ok := mapKey(wparam); ok {
			rep := lparam&(1<<30) != 0
			if rep {
				gWin.app.Key(k, ui.Repeat)
			} else {
				gWin.app.Key(k, ui.Press)
			}
			return 0
		}
	case WM_KEYUP, WM_SYSKEYUP:
		if k, ok := mapKey(wparam); ok {
			gWin.app.Key(k, ui.Release)
			return 0
		}
	case WM_MOUSEMOVE:
		gWin.app.MouseMove(gWin.px(loword(lparam)), gWin.px(hiword(lparam)))
		return 0
	case WM_LBUTTONDOWN:
		x, y := gWin.px(loword(lparam)), gWin.px(hiword(lparam))
		switch gWin.chromeHit(x, y) {
		case 1:
			pShowWindow.Call(hwnd, SW_MINIMIZE)
			return 0
		case 2:
			pDestroyWindow.Call(hwnd)
			return 0
		case 3:
			pReleaseCapture.Call()
			pSendMessageW.Call(hwnd, 0x00A1, 2, 0) // WM_NCLBUTTONDOWN / HTCAPTION
			return 0
		}
		gWin.app.MouseDown(x, y)
		return 0
	case WM_LBUTTONUP:
		gWin.app.MouseUp(gWin.px(loword(lparam)), gWin.px(hiword(lparam)))
		return 0
	case WM_MOUSEWHEEL:
		var pt POINT
		pt.X = loword(lparam)
		pt.Y = hiword(lparam)
		pScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
		delta := hiWord32(uint32(wparam))
		gWin.app.Wheel(gWin.px(pt.X), gWin.px(pt.Y), float32(delta)/120)
		return 0
	case WM_APP_TRAY:
		switch lparam & 0xffff {
		case WM_LBUTTONUP_:
			gWin.restore()
		case WM_RBUTTONUP_:
			gWin.trayMenu()
		}
		return 0
	case WM_APP_SHOW:
		// A second launch asks the running instance to leave the tray.
		gWin.restore()
		return 0
	case WM_CLOSE:
		pDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		if gOverlay != nil {
			pDestroyWindow.Call(gOverlay.hwnd)
			gOverlay.surface.Close()
			gOverlay = nil
		}
		w.removeTray()
		pPostQuitMessage.Call(0)
		gWin = nil
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func mapKey(vk uintptr) (ui.Key, bool) {
	switch vk {
	case 0x26:
		return ui.KeyUp, true
	case 0x28:
		return ui.KeyDown, true
	case 0x25:
		return ui.KeyLeft, true
	case 0x27:
		return ui.KeyRight, true
	case 0x0D, 0x20:
		return ui.KeyEnter, true
	case 0x1B:
		return ui.KeyBack, true
	case 0x09:
		return ui.KeyTab, true
	case 0x21:
		return ui.KeyPageUp, true
	case 0x22:
		return ui.KeyPageDown, true
	case 0x24:
		return ui.KeyHome, true
	case 0x23:
		return ui.KeyEnd, true
	}
	return ui.KeyNone, false
}
