package sys

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	pCoInitializeEx   = ole32DLL.NewProc("CoInitializeEx")
	pCoUninitialize   = ole32DLL.NewProc("CoUninitialize")
	pCreateDXGIFactory1 = syscall.NewLazyDLL("dxgi.dll").NewProc("CreateDXGIFactory1")
)

var dxgiFactory1 = windows.GUID{
	Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba,
	Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87},
}

const dxgiErrorNotFound = 0x887A0002

// Adapter is one DXGI adapter as the game sees it.
type Adapter struct {
	Name           string
	VendorID       uint32
	DeviceID       uint32
	DedicatedVRAM  uint64
	SharedVRAM     uint64
	LUIDHigh       int32
	LUIDLow        uint32
	Software       bool
}

type luid struct {
	LowPart  uint32
	HighPart int32
}

type dxgiAdapterDesc1 struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	AdapterLuid           luid
	Flags                 uint32
}

func toPointer(p uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&p))
}

// DebugCom prints the factory vtable, for diagnosing COM binding problems.
var DebugCom bool

func comCall(obj uintptr, index int, args ...uintptr) uintptr {
	if obj == 0 {
		return 0x80004003 // E_POINTER
	}
	objp := toPointer(obj)
	vtbl := *(*unsafe.Pointer)(objp)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(index)*unsafe.Sizeof(uintptr(0))))
	if DebugCom {
		fmt.Printf("com obj=%#x vtbl=%p index=%d fn=%#x\n", obj, vtbl, index, fn)
	}
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, obj)
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func comRelease(obj uintptr) {
	if obj != 0 {
		comCall(obj, 2)
	}
}

// Adapters enumerates DXGI adapters through IDXGIFactory1.
func Adapters() ([]Adapter, error) {
	r, _, _ := pCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED
	if int32(r) >= 0 {
		defer pCoUninitialize.Call()
	}
	var factory uintptr
	hr, _, _ := pCreateDXGIFactory1.Call(
		uintptr(unsafe.Pointer(&dxgiFactory1)), uintptr(unsafe.Pointer(&factory)))
	if int32(hr) < 0 || factory == 0 {
		return nil, fmt.Errorf("DXGI factory: 0x%08x", uint32(hr))
	}
	defer comRelease(factory)

	var out []Adapter
	for i := 0; i < 8; i++ {
		var ad uintptr
		h := comCall(factory, 12, uintptr(i), uintptr(unsafe.Pointer(&ad))) // EnumAdapters1
		if uint32(h) == dxgiErrorNotFound || ad == 0 {
			break
		}
		if int32(h) < 0 {
			break
		}
		var d dxgiAdapterDesc1
		if comCall(ad, 10, uintptr(unsafe.Pointer(&d))) == 0 { // GetDesc1
			out = append(out, Adapter{
				Name:          windows.UTF16ToString(d.Description[:]),
				VendorID:      d.VendorID,
				DeviceID:      d.DeviceID,
				DedicatedVRAM: uint64(d.DedicatedVideoMemory),
				SharedVRAM:    uint64(d.SharedSystemMemory),
				LUIDHigh:      d.AdapterLuid.HighPart,
				LUIDLow:       d.AdapterLuid.LowPart,
				Software:      d.Flags&2 != 0,
			})
		}
		comRelease(ad)
	}
	return out, nil
}

// VendorName maps PCI vendor ids to the three names users recognise.
func VendorName(id uint32) string {
	switch id {
	case 0x1002:
		return "AMD"
	case 0x10DE:
		return "NVIDIA"
	case 0x8086:
		return "Intel"
	case 0x1414:
		return "Microsoft"
	}
	return "其他"
}
