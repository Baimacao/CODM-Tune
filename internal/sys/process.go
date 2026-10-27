// Package sys wraps the Win32 calls the tuner needs: process snapshots,
// scheduling, CPU topology, DXGI, PDH and TCP tables. No CGO and no writes
// into other processes' memory.
package sys

import (
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	PROCESS_SET_INFORMATION           = 0x0200
	PROCESS_QUERY_INFORMATION         = 0x0400
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
	IDLE_PRIORITY_CLASS               = 0x00000040
	BELOW_NORMAL_PRIORITY_CLASS       = 0x00004000
	NORMAL_PRIORITY_CLASS             = 0x00000020
	ABOVE_NORMAL_PRIORITY_CLASS       = 0x00008000
	HIGH_PRIORITY_CLASS               = 0x00000080
	REALTIME_PRIORITY_CLASS           = 0x00000100
)

// Proc is one row of a process snapshot.
type Proc struct {
	PID     uint32
	Parent  uint32
	Name    string
	Threads uint32
}

// Snapshot enumerates every process in one Toolhelp call. Loops must call
// this once per cycle instead of probing processes one by one.
func Snapshot() ([]Proc, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	if err := windows.Process32First(snap, &e); err != nil {
		return nil, err
	}
	var out []Proc
	for {
		out = append(out, Proc{
			PID:     e.ProcessID,
			Parent:  e.ParentProcessID,
			Name:    windows.UTF16ToString(e.ExeFile[:]),
			Threads: e.Threads,
		})
		if err := windows.Process32Next(snap, &e); err != nil {
			break
		}
	}
	return out, nil
}

func openTune(pid uint32) (windows.Handle, error) {
	// Some protected processes accept QUERY_INFORMATION while rejecting the
	// limited variant, so try both masks before giving up.
	h, err := windows.OpenProcess(PROCESS_SET_INFORMATION|PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err == nil {
		return h, nil
	}
	h2, err2 := windows.OpenProcess(PROCESS_SET_INFORMATION|PROCESS_QUERY_INFORMATION, false, pid)
	if err2 == nil {
		return h2, nil
	}
	return 0, err
}

// ImagePath returns the full executable path.
func ImagePath(pid uint32) (string, error) {
	h, err := windows.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:size]), nil
}

// PriorityClass reads the current scheduling class.
func PriorityClass(pid uint32) (uint32, error) {
	h, err := openTune(pid)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(h)
	return windows.GetPriorityClass(h)
}

// SetPriorityClass applies a scheduling class.
func SetPriorityClass(pid uint32, class uint32) error {
	h, err := openTune(pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.SetPriorityClass(h, class)
}

// Affinity returns the process mask and the system mask.
func Affinity(pid uint32) (uint64, uint64, error) {
	h, err := openTune(pid)
	if err != nil {
		return 0, 0, err
	}
	defer windows.CloseHandle(h)
	var procMask, sysMask uintptr
	r, _, e := pGetProcessAffinityMask.Call(uintptr(h), uintptr(unsafe.Pointer(&procMask)), uintptr(unsafe.Pointer(&sysMask)))
	if r == 0 {
		return 0, 0, e
	}
	return uint64(procMask), uint64(sysMask), nil
}

// SetAffinity sets the process affinity mask.
func SetAffinity(pid uint32, mask uint64) error {
	h, err := openTune(pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	r, _, e := pSetProcessAffinityMask.Call(uintptr(h), uintptr(mask))
	if r == 0 {
		return e
	}
	return nil
}

// CreationTime is the 100ns FILETIME at which the process started; it guards
// against a recycled PID when restoring settings later.
func CreationTime(pid uint32) (uint64, error) {
	h, err := windows.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(h)
	var c, x, k, u windows.Filetime
	if err := windows.GetProcessTimes(h, &c, &x, &k, &u); err != nil {
		return 0, err
	}
	return uint64(c.HighDateTime)<<32 | uint64(c.LowDateTime), nil
}

// CPUTime is kernel+user time in 100ns units.
func CPUTime(pid uint32) (uint64, error) {
	h, err := windows.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(h)
	var c, x, k, u windows.Filetime
	if err := windows.GetProcessTimes(h, &c, &x, &k, &u); err != nil {
		return 0, err
	}
	kv := uint64(k.HighDateTime)<<32 | uint64(k.LowDateTime)
	uv := uint64(u.HighDateTime)<<32 | uint64(u.LowDateTime)
	return kv + uv, nil
}

type processMemoryCountersEx struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

// WorkingSet is the process working set in bytes.
func WorkingSet(pid uint32) (uint64, error) {
	h, err := windows.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(h)
	var pmc processMemoryCountersEx
	pmc.Cb = uint32(unsafe.Sizeof(pmc))
	r, _, e := pGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.Cb))
	if r == 0 {
		return 0, e
	}
	return uint64(pmc.WorkingSetSize), nil
}

// IsElevated reports whether the process token is a member of the local
// Administrators group and running with that group enabled.
func IsElevated() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(&windows.SECURITY_NT_AUTHORITY, 2,
		windows.SECURITY_BUILTIN_DOMAIN_RID, windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0, &sid)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)
	tok := windows.Token(0)
	member, err := tok.IsMember(sid)
	if err != nil {
		return false
	}
	return member
}

// SetSelfBelowNormal makes the tuner itself yield to the game.
func SetSelfBelowNormal() error {
	h := windows.CurrentProcess()
	return windows.SetPriorityClass(h, BELOW_NORMAL_PRIORITY_CLASS)
}

// LogicalCPUs is the number of logical processors Go sees.
func LogicalCPUs() int { return runtime.NumCPU() }

// FindByPath returns processes whose image path is under dir.
func FindByPath(dir string) ([]Proc, error) {
	procs, err := Snapshot()
	if err != nil {
		return nil, err
	}
	dir = strings.ToLower(strings.TrimRight(dir, `\`))
	var out []Proc
	for _, p := range procs {
		path, err := ImagePath(p.PID)
		if err != nil {
			continue
		}
		if strings.HasPrefix(strings.ToLower(path), dir+`\`) {
			p.Name = path
			out = append(out, p)
		}
	}
	return out, nil
}

// FormatError keeps Win32 errors readable in the UI.
func FormatError(what string, err error) string {
	if err == nil {
		return what
	}
	return fmt.Sprintf("%s: %v", what, err)
}
