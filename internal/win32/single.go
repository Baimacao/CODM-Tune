package win32

import (
	"syscall"
	"time"
	"unsafe"
)

var singleMutex uintptr

// ActivateExistingInstance brings a running CodmTune window forward, including
// a window hidden to the tray. It returns false when no instance is present.
func ActivateExistingInstance() bool {
	cls, _ := syscall.UTF16FromString("CodmTuneWnd")
	var w uintptr
	for i := 0; i < 30; i++ {
		w, _, _ = pFindWindowW.Call(uintptr(unsafe.Pointer(&cls[0])), 0)
		if w != 0 {
			break
		}
		// The first process may still be creating its window right after it
		// acquired the mutex. Retry briefly instead of dropping the wake-up.
		time.Sleep(50 * time.Millisecond)
	}
	if w == 0 {
		return false
	}
	vis, _, _ := pIsWindowVisible.Call(w)
	pShowWindow.Call(w, SW_SHOW)
	pShowWindow.Call(w, SW_RESTORE)
	pPostMessageW.Call(w, WM_APP_SHOW, 0, 0)
	pSetForegroundWindow.Call(w)
	// A tray-hidden window is restored silently (that is the expected action).
	// If the window was already visible, a second launch otherwise looks like
	// nothing happened; show a short, topmost notice owned by this short-lived
	// process instead of adding a toast system to the main loop.
	if vis != 0 {
		title, _ := syscall.UTF16FromString("CODM Tune")
		text, _ := syscall.UTF16FromString("CODM Tune 已经在运行。\n已把现有窗口切到前台。")
		pMessageBoxW.Call(0, uintptr(unsafe.Pointer(&text[0])), uintptr(unsafe.Pointer(&title[0])), 0x40|0x40000)
	}
	return true
}

// AcquireSingleInstance creates a named mutex. When another instance already
// owns it, the existing window is brought forward and false is returned.
func AcquireSingleInstance(name string) bool {
	n, err := syscall.UTF16FromString(name)
	if err != nil {
		return true
	}
	h, _, e := pCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(&n[0])))
	if h == 0 {
		// An elevated first instance may deny a non-elevated second one the
		// right to open the mutex; that is still a duplicate, not a failure.
		if errno, ok := e.(syscall.Errno); ok && errno == syscall.Errno(5) {
			ActivateExistingInstance()
			return false
		}
		return true
	}
	if errno, ok := e.(syscall.Errno); ok && errno == ERROR_ALREADY_EXISTS {
		ActivateExistingInstance()
		return false
	}
	singleMutex = h
	return true
}
