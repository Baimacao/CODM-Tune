package sys

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	pShellExecuteW = shell32DLL.NewProc("ShellExecuteW")
	pMessageBoxW   = user32DLL.NewProc("MessageBoxW")
)

// RelaunchElevated starts this executable through the runas verb. The caller
// should exit when it returns nil.
func RelaunchElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	filep, _ := syscall.UTF16PtrFromString(exe)
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))
	r, _, e := pShellExecuteW.Call(0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(filep)),
		0,
		uintptr(unsafe.Pointer(dir)),
		1) // SW_SHOWNORMAL
	if r <= 32 {
		return fmt.Errorf("ShellExecuteW runas: %v (%d)", e, r)
	}
	return nil
}

// MessageBox shows a native modal dialog; used only for fatal startup errors.
func MessageBox(title, text string) {
	t, _ := syscall.UTF16PtrFromString(title)
	b, _ := syscall.UTF16PtrFromString(text)
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(t)), 0x10)
}
