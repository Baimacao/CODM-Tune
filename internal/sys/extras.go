package sys

import (
	"syscall"
	"unsafe"
)

var ntdll = syscall.NewLazyDLL("ntdll.dll")

var (
	pNtSetTimerResolution   = ntdll.NewProc("NtSetTimerResolution")
	pNtQueryTimerResolution = ntdll.NewProc("NtQueryTimerResolution")
	pSystemParametersInfoW  = user32DLL.NewProc("SystemParametersInfoW")
	pIcmpCreateFile         = syscall.NewLazyDLL("icmp.dll").NewProc("IcmpCreateFile")
	pIcmpSendEcho           = syscall.NewLazyDLL("icmp.dll").NewProc("IcmpSendEcho")
	pIcmpCloseHandle        = syscall.NewLazyDLL("icmp.dll").NewProc("IcmpCloseHandle")
	pGetAsyncKeyState       = user32DLL.NewProc("GetAsyncKeyState")
)

// KeyDown polls a virtual-key state without installing a hook.
func KeyDown(vk int) bool {
	r, _, _ := pGetAsyncKeyState.Call(uintptr(vk))
	return r&0x8000 != 0
}

// TimerResolution asks the kernel for a finer timer. Resolution is in 100 ns
// units (5000 = 0.5 ms). Passing on=false releases the request.
func TimerResolution(hundredNs uint32, on bool) (uint32, error) {
	var cur uint32
	set := uintptr(0)
	if on {
		set = 1
	}
	r, _, e := pNtSetTimerResolution.Call(uintptr(hundredNs), set, uintptr(unsafe.Pointer(&cur)))
	if r != 0 {
		return 0, e
	}
	return cur, nil
}

// CurrentTimerResolution reports the effective timer period in 100 ns units.
func CurrentTimerResolution() uint32 {
	var min, max, cur uint32
	pNtQueryTimerResolution.Call(uintptr(unsafe.Pointer(&min)), uintptr(unsafe.Pointer(&max)), uintptr(unsafe.Pointer(&cur)))
	return cur
}

// MouseAcceleration enables or disables the Windows pointer acceleration
// curves immediately (SPI_SETMOUSE), for the current session.
func MouseAcceleration(on bool) {
	vals := [3]int32{0, 0, 0}
	if on {
		vals = [3]int32{1, 6, 10}
	}
	pSystemParametersInfoW.Call(0x0004 /*SPI_SETMOUSE*/, 0, uintptr(unsafe.Pointer(&vals[0])), 0)
}

// IcmpRTT measures an ICMP echo round trip to an IPv4 address, in
// milliseconds. It is read-only: one echo request, no payload semantics.
func IcmpRTT(dst [4]byte, timeoutMs uint32) (uint32, bool) {
	h, _, _ := pIcmpCreateFile.Call()
	if h == 0 || h == uintptr(syscall.InvalidHandle) {
		return 0, false
	}
	defer pIcmpCloseHandle.Call(h)
	addr := uint32(dst[0]) | uint32(dst[1])<<8 | uint32(dst[2])<<16 | uint32(dst[3])<<24
	payload := []byte("codmtune")
	reply := make([]byte, 128)
	r, _, _ := pIcmpSendEcho.Call(h, uintptr(addr),
		uintptr(unsafe.Pointer(&payload[0])), uintptr(len(payload)), 0,
		uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), uintptr(timeoutMs))
	if r == 0 {
		return 0, false
	}
	status := *(*uint32)(unsafe.Pointer(&reply[4]))
	if status != 0 {
		return 0, false
	}
	rtt := *(*uint32)(unsafe.Pointer(&reply[8]))
	return rtt, true
}

// HiddenAttr hides helper consoles for exec.Cmd.
func HiddenAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// OpenPath opens a file, folder or URL with its default handler.
func OpenPath(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	verb, _ := syscall.UTF16PtrFromString("open")
	r, _, e := shell32DLL.NewProc("ShellExecuteW").Call(0,
		uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(p)), 0, 0, 1)
	if r <= 32 {
		return e
	}
	return nil
}
