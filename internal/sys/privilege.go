package sys

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pOpenProcessToken      = kernel32DLL.NewProc("OpenProcessToken")
	pLookupPrivilegeValueW = advapi32DLL.NewProc("LookupPrivilegeValueW")
	pAdjustTokenPrivileges = advapi32DLL.NewProc("AdjustTokenPrivileges")
)

var advapi32DLL = syscall.NewLazyDLL("advapi32.dll")

type tokenLuid struct {
	LowPart  uint32
	HighPart int32
}

type tokenPrivileges struct {
	PrivilegeCount uint32
	Luid           tokenLuid
	Attributes     uint32
}

const tokenAdjustPrivileges = 0x0020
const tokenQuery = 0x0008
const sePrivilegeEnabled = 0x00000002

// EnableDebugPrivilege turns on SeDebugPrivilege when the token holds it. It is
// only requested so ACE's own SGuard processes accept priority changes; no
// memory access is performed with it.
func EnableDebugPrivilege() error {
	return enablePrivilege("SeDebugPrivilege")
}

// enablePrivilege turns on one named privilege for this process's own token.
// A failure means the token does not hold it (usually: not elevated), and the
// caller decides whether that is fatal for the feature.
func enablePrivilege(priv string) error {
	var token windows.Handle
	r, _, e := pOpenProcessToken.Call(uintptr(windows.CurrentProcess()), tokenAdjustPrivileges|tokenQuery, uintptr(unsafe.Pointer(&token)))
	if r == 0 {
		return e
	}
	defer windows.CloseHandle(token)
	name, _ := syscall.UTF16PtrFromString(priv)
	var l tokenLuid
	r, _, e = pLookupPrivilegeValueW.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&l)))
	if r == 0 {
		return e
	}
	tp := tokenPrivileges{PrivilegeCount: 1, Luid: l, Attributes: sePrivilegeEnabled}
	r, _, e = pAdjustTokenPrivileges.Call(uintptr(token), 0, uintptr(unsafe.Pointer(&tp)), 0, 0, 0)
	if r == 0 {
		return e
	}
	// ERROR_NOT_ALL_ASSIGNED (1300) means the token does not hold the privilege.
	if errno, ok := e.(syscall.Errno); ok && errno == 1300 {
		return errno
	}
	return nil
}
