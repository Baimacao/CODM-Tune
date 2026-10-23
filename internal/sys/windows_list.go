package sys

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	user32DLL            = syscall.NewLazyDLL("user32.dll")
	pEnumWindows         = user32DLL.NewProc("EnumWindows")
	pIsWindowVisible     = user32DLL.NewProc("IsWindowVisible")
	pGetWindowTextLength = user32DLL.NewProc("GetWindowTextLengthW")
	pGetWindowThreadPID  = user32DLL.NewProc("GetWindowThreadProcessId")
)

var (
	enumMu  sync.Mutex
	enumSet map[uint32]bool
)

var enumCallback = syscall.NewCallback(func(hwnd, lparam uintptr) uintptr {
	if r, _, _ := pIsWindowVisible.Call(hwnd); r == 0 {
		return 1
	}
	if r, _, _ := pGetWindowTextLength.Call(hwnd); r == 0 {
		return 1
	}
	var pid uint32
	pGetWindowThreadPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != 0 {
		enumSet[pid] = true
	}
	return 1
})

// VisibleWindowPIDs lists processes owning a visible, titled top-level window.
// It bounds the set of processes the UI samples for CPU and memory.
func VisibleWindowPIDs() []uint32 {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumSet = map[uint32]bool{}
	pEnumWindows.Call(enumCallback, 0)
	out := make([]uint32, 0, len(enumSet))
	for pid := range enumSet {
		out = append(out, pid)
	}
	return out
}
