package sys

import (
	"runtime"
	"syscall"
	"unsafe"
)

// System-wide memory reclamation, modelled on Mem Reduct. Everything here is a
// documented NT memory-list operation: no process is touched, nothing is
// injected, and CODM's own working set is only affected the same way Windows
// would trim it under pressure. That is why the game-time default is off.

var (
	ntdllDLL = syscall.NewLazyDLL("ntdll.dll")

	pNtSetSystemInformation = ntdllDLL.NewProc("NtSetSystemInformation")
	pGlobalMemoryStatusEx   = kernel32DLL.NewProc("GlobalMemoryStatusEx")
)

const (
	systemMemoryListInformation             = 0x50 // 80
	systemFileCacheInformationEx            = 0x51 // 81
	systemRegistryReconciliationInfo        = 0x9B // 155
	memoryEmptyWorkingSets           uint32 = 2
	memoryFlushModifiedList          uint32 = 3
	memoryPurgeStandbyList           uint32 = 4
	memoryPurgeLowPriorityStandby    uint32 = 5
)

// MemMask selects which reclamation steps run, mirroring Mem Reduct's mask.
type MemMask uint32

const (
	MemWorkingSets MemMask = 1 << iota
	MemFileCache
	MemModifiedList
	MemStandbyList
	MemLowPriorityStandby
	MemRegistryCache
)

// MemAll is the conservative default set: every step is reversible cache
// reclamation, none of them suspend, kill or write into another process.
const MemAll = MemWorkingSets | MemFileCache | MemModifiedList | MemStandbyList | MemLowPriorityStandby | MemRegistryCache

// MemStat is a physical-memory snapshot in bytes.
type MemStat struct {
	Total   uint64
	Avail   uint64
	Used    uint64
	Percent int
}

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// MemoryStat reads physical memory usage.
func MemoryStat() MemStat {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	r, _, _ := pGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&m)))
	if r == 0 {
		return MemStat{}
	}
	return MemStat{
		Total:   m.TotalPhys,
		Avail:   m.AvailPhys,
		Used:    m.TotalPhys - m.AvailPhys,
		Percent: int(m.MemoryLoad),
	}
}

type fileCacheInfo struct {
	CurrentSize                    uintptr
	PeakSize                       uintptr
	PageFaultCount                 uint32
	MinimumWorkingSet              uintptr
	MaximumWorkingSet              uintptr
	CurrentSizeIncludingTransition uintptr
	PeakSizeIncludingTransition    uintptr
	TransitionRePurposeCount       uint32
	Flags                          uint32
}

// EnableMemPrivileges turns on the two privileges Mem Reduct needs. It is a
// token change for this process only; failure is reported so the UI can say
// "run as administrator" instead of silently doing nothing.
func EnableMemPrivileges() error {
	var firstErr error
	for _, name := range []string{"SeProfileSingleProcessPrivilege", "SeIncreaseQuotaPrivilege"} {
		if err := enablePrivilege(name); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// FlushMemory runs the selected reclamation steps and reports what Windows
// says the physical usage was before and after. A partial failure returns the
// steps that failed by name, because some builds reject the registry call.
func FlushMemory(mask MemMask) (before, after MemStat, failed []string) {
	runtime.GC() // release Go's own heap first, then ask Windows for the rest
	before = MemoryStat()

	run := func(step MemMask, name string, fn func() error) {
		if mask&step == 0 {
			return
		}
		if err := fn(); err != nil {
			failed = append(failed, name)
		}
	}

	cmd := memoryEmptyWorkingSets
	run(MemWorkingSets, "工作集", func() error {
		return ntSet(systemMemoryListInformation, unsafe.Pointer(&cmd), unsafe.Sizeof(cmd))
	})

	run(MemFileCache, "系统文件缓存", func() error {
		fc := fileCacheInfo{MinimumWorkingSet: ^uintptr(0), MaximumWorkingSet: ^uintptr(0)}
		return ntSet(systemFileCacheInformationEx, unsafe.Pointer(&fc), unsafe.Sizeof(fc))
	})

	cmd2 := memoryFlushModifiedList
	run(MemModifiedList, "已修改页列表", func() error {
		return ntSet(systemMemoryListInformation, unsafe.Pointer(&cmd2), unsafe.Sizeof(cmd2))
	})

	cmd3 := memoryPurgeStandbyList
	run(MemStandbyList, "待机列表", func() error {
		return ntSet(systemMemoryListInformation, unsafe.Pointer(&cmd3), unsafe.Sizeof(cmd3))
	})

	cmd4 := memoryPurgeLowPriorityStandby
	run(MemLowPriorityStandby, "低优先级待机列表", func() error {
		return ntSet(systemMemoryListInformation, unsafe.Pointer(&cmd4), unsafe.Sizeof(cmd4))
	})

	run(MemRegistryCache, "注册表缓存", func() error {
		return ntSet(systemRegistryReconciliationInfo, nil, 0)
	})

	after = MemoryStat()
	return before, after, failed
}

func ntSet(class int, buf unsafe.Pointer, size uintptr) error {
	r, _, _ := pNtSetSystemInformation.Call(uintptr(class), uintptr(buf), size)
	if r != 0 {
		// NTSTATUS: 0 is success, everything else is a failure code.
		return syscall.Errno(r)
	}
	return nil
}
