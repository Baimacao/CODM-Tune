package sys

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pPdhOpenQueryW                = pdhDLL.NewProc("PdhOpenQueryW")
	pPdhAddEnglishCounterW        = pdhDLL.NewProc("PdhAddEnglishCounterW")
	pPdhCollectQueryData          = pdhDLL.NewProc("PdhCollectQueryData")
	pPdhGetFormattedCounterArrayW = pdhDLL.NewProc("PdhGetFormattedCounterArrayW")
	pPdhCloseQuery                = pdhDLL.NewProc("PdhCloseQuery")
)

const (
	pdhFmtDouble = 0x00000200
	pdhMoreData  = 0x800007D2
)

type pdhFmtValue struct {
	Status uint32
	_      uint32
	Double float64
}

type pdhFmtItem struct {
	Name  *uint16
	Value pdhFmtValue
}

// GPUEngine is one GPU engine counter instance attributed to a process.
type GPUEngine struct {
	PID      uint32
	LUIDHigh uint32
	LUIDLow  uint32
	Engine   string
	Usage    float64
}

// GPUUsage samples the per-process GPU engine counters. The caller should run
// it off the UI goroutine: it needs two collections 250ms apart.
func GPUUsage() ([]GPUEngine, error) {
	var query uintptr
	if r, _, _ := pPdhOpenQueryW.Call(0, 0, uintptr(unsafe.Pointer(&query))); r != 0 {
		return nil, fmt.Errorf("PdhOpenQuery: 0x%08x", uint32(r))
	}
	defer pPdhCloseQuery.Call(query)

	path, _ := syscall.UTF16PtrFromString(`\GPU Engine(*)\Utilization Percentage`)
	var counter uintptr
	if r, _, e := pPdhAddEnglishCounterW.Call(query, uintptr(unsafe.Pointer(path)), 0, uintptr(unsafe.Pointer(&counter))); r != 0 {
		return nil, fmt.Errorf("PdhAddEnglishCounter: %v", e)
	}
	if r, _, _ := pPdhCollectQueryData.Call(query); r != 0 {
		return nil, errors.New("PdhCollectQueryData failed")
	}
	time.Sleep(250 * time.Millisecond)
	if r, _, _ := pPdhCollectQueryData.Call(query); r != 0 {
		return nil, errors.New("PdhCollectQueryData failed")
	}

	var size, count uint32
	r, _, _ := pPdhGetFormattedCounterArrayW.Call(counter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if uint32(r) != pdhMoreData {
		return nil, errors.New("GPU engine counters unavailable")
	}
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	r, _, _ = pPdhGetFormattedCounterArrayW.Call(counter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 {
		return nil, errors.New("PdhGetFormattedCounterArray failed")
	}
	items := unsafe.Slice((*pdhFmtItem)(unsafe.Pointer(&buf[0])), int(count))
	out := make([]GPUEngine, 0, len(items))
	for _, it := range items {
		name := windows.UTF16PtrToString(it.Name)
		e, ok := parseGPUInstance(name)
		if !ok {
			continue
		}
		e.Usage = it.Value.Double
		out = append(out, e)
	}
	return out, nil
}

// parseGPUInstance parses
// pid_1234_luid_0x00000000_0x0000ABCD_phys_0_eng_0_engtype_3D.
func parseGPUInstance(s string) (GPUEngine, bool) {
	var e GPUEngine
	parts := strings.Split(s, "_")
	okPid, okLuid := false, false
	for i := 0; i+1 < len(parts); i++ {
		switch parts[i] {
		case "pid":
			if v, err := strconv.ParseUint(parts[i+1], 10, 32); err == nil {
				e.PID = uint32(v)
				okPid = true
			}
		case "luid":
			if i+2 < len(parts) {
				hi, err1 := strconv.ParseUint(strings.TrimPrefix(parts[i+1], "0x"), 16, 32)
				lo, err2 := strconv.ParseUint(strings.TrimPrefix(parts[i+2], "0x"), 16, 32)
				if err1 == nil && err2 == nil {
					e.LUIDHigh = uint32(hi)
					e.LUIDLow = uint32(lo)
					okLuid = true
				}
			}
		case "engtype":
			if i+1 < len(parts) {
				e.Engine = parts[i+1]
			}
		}
	}
	return e, okPid && okLuid
}
