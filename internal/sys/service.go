package sys

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	advapi32DLLSC         = syscall.NewLazyDLL("advapi32.dll")
	pOpenSCManagerW       = advapi32DLLSC.NewProc("OpenSCManagerW")
	pOpenServiceW         = advapi32DLLSC.NewProc("OpenServiceW")
	pQueryServiceStatusEx = advapi32DLLSC.NewProc("QueryServiceStatusEx")
	pCloseServiceHandle   = advapi32DLLSC.NewProc("CloseServiceHandle")
)

const (
	scManagerConnect    = 0x0001
	serviceQueryStatus  = 0x0004
	scStatusProcessInfo = 0
)

type serviceStatusProcess struct {
	ServiceType      uint32
	CurrentState     uint32
	ControlsAccepted uint32
	Win32ExitCode    uint32
	ServiceExitCode  uint32
	CheckPoint       uint32
	WaitHint         uint32
	ProcessID        uint32
	ServiceFlags     uint32
}

// QueryService reports one service's state in words. It is read-only: the
// service control manager is opened for query access only.
func QueryService(name string) (string, error) {
	mgr, _, e := pOpenSCManagerW.Call(0, 0, scManagerConnect)
	if mgr == 0 {
		return "", fmt.Errorf("OpenSCManagerW: %v", e)
	}
	defer pCloseServiceHandle.Call(mgr)
	nm, _ := syscall.UTF16PtrFromString(name)
	h, _, e := pOpenServiceW.Call(mgr, uintptr(unsafe.Pointer(nm)), serviceQueryStatus)
	if h == 0 {
		return "", fmt.Errorf("OpenServiceW %s: %v", name, e)
	}
	defer pCloseServiceHandle.Call(h)
	var st serviceStatusProcess
	var need uint32
	r, _, e := pQueryServiceStatusEx.Call(h, scStatusProcessInfo,
		uintptr(unsafe.Pointer(&st)), unsafe.Sizeof(st), uintptr(unsafe.Pointer(&need)))
	if r == 0 {
		return "", fmt.Errorf("QueryServiceStatusEx: %v", e)
	}
	switch st.CurrentState {
	case 1:
		return "已停止", nil
	case 2:
		return "启动中", nil
	case 3:
		return "停止中", nil
	case 4:
		return "运行中", nil
	case 5:
		return "继续中", nil
	case 6:
		return "暂停中", nil
	case 7:
		return "已暂停", nil
	}
	return "未知", nil
}
