package sys

import (
	"fmt"
	"os"
	"unsafe"
)

var (
	pNtSetInformationProcess   = ntdllDLL.NewProc("NtSetInformationProcess")
	pNtQueryInformationProcess = ntdllDLL.NewProc("NtQueryInformationProcess")
	pGetCurrentProcess         = kernel32DLL.NewProc("GetCurrentProcess")
)

// ProcessIoPriority is the PROCESSINFOCLASS value for the process-wide I/O
// priority. It is not exposed by any documented Win32 wrapper, which is why it
// goes through ntdll; the value itself is stable across Windows 7..11.
const processIoPriority = 33

// I/O priority levels accepted by NtSetInformationProcess. Level 4 (critical)
// is rejected with STATUS_INVALID_PARAMETER for an ordinary process.
const (
	IOVeryLow  = 0
	IOLow      = 1
	IONormal   = 2
	IOHigh     = 3
	IOCritical = 4
)

// IOPriorityName is the UI label for one level.
func IOPriorityName(v int) string {
	switch v {
	case IOVeryLow:
		return "极低"
	case IOLow:
		return "低"
	case IONormal:
		return "普通"
	case IOHigh:
		return "高"
	case IOCritical:
		return "关键"
	}
	return "—"
}

// GetIOPriority reads a process's I/O priority. pid 0 means this process.
func GetIOPriority(pid uint32) (int, error) {
	h, err := ioPriorityHandle(pid)
	if err != nil {
		return 0, err
	}
	defer closeIfReal(h, pid)
	var out uint32
	var ret uint32
	r, _, e := pNtQueryInformationProcess.Call(h, processIoPriority,
		uintptr(unsafe.Pointer(&out)), unsafe.Sizeof(out), uintptr(unsafe.Pointer(&ret)))
	if r != 0 {
		return 0, fmt.Errorf("NtQueryInformationProcess: 0x%08x (%v)", uint32(r), e)
	}
	return int(out), nil
}

// SetIOPriority writes a process's I/O priority. pid 0 means this process.
// Raising another process needs SeIncreaseBasePriorityPrivilege; the caller
// enables it (EnableIOPriorityPrivilege) and decides whether failure is fatal.
func SetIOPriority(pid uint32, level int) error {
	h, err := ioPriorityHandle(pid)
	if err != nil {
		return err
	}
	defer closeIfReal(h, pid)
	v := uint32(level)
	r, _, e := pNtSetInformationProcess.Call(h, processIoPriority,
		uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
	if r != 0 {
		return fmt.Errorf("NtSetInformationProcess: 0x%08x (%v)", uint32(r), e)
	}
	return nil
}

// EnableIOPriorityPrivilege turns on SeIncreaseBasePriorityPrivilege, which is
// what allows raising another process's I/O priority above Normal.
func EnableIOPriorityPrivilege() error {
	return enablePrivilege("SeIncreaseBasePriorityPrivilege")
}

// ioPriorityHandle opens a handle for the priority calls. The pseudo handle is
// used for this process so nothing has to be closed.
func ioPriorityHandle(pid uint32) (uintptr, error) {
	if pid == 0 || pid == uint32(os.Getpid()) {
		h, _, _ := pGetCurrentProcess.Call()
		return h, nil
	}
	h, err := openTune(pid)
	if err != nil {
		return 0, err
	}
	return uintptr(h), nil
}

func closeIfReal(h uintptr, pid uint32) {
	if pid == 0 || pid == uint32(os.Getpid()) {
		return
	}
	pCloseHandle.Call(h)
}
