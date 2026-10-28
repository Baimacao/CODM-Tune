package sys

import (
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	pGetVolumeInformationW = kernel32DLL.NewProc("GetVolumeInformationW")
	pGetDiskFreeSpaceExW   = kernel32DLL.NewProc("GetDiskFreeSpaceExW")
	pCreateFileW           = kernel32DLL.NewProc("CreateFileW")
	pDeviceIoControl       = kernel32DLL.NewProc("DeviceIoControl")
	pCloseHandle           = kernel32DLL.NewProc("CloseHandle")
)

const (
	driveFixed     = 3
	driveRemovable = 2

	ioctlStorageQueryProperty = 0x002D1400
	storageDeviceSeekPenalty  = 7
	// storageDeviceProperty reports the bus type, which is what separates a
	// USB stick from an internal SATA drive when the seek-penalty query is
	// refused (some cheap controllers do refuse it).
	storageDeviceProperty = 0
)

type storagePropertyQuery struct {
	PropertyID uint32
	QueryType  uint32
	_          [1]byte
}

// deviceSeekPenalty mirrors STORAGE_DEVICE_SEEK_PENALTY_DESCRIPTOR. The Size
// field is part of the struct — reading the boolean straight after Version
// yields the size field and every disk looks like an HDD.
type deviceSeekPenalty struct {
	Version           uint32
	Size              uint32
	IncursSeekPenalty byte
	_                 [3]byte
}

type storageDeviceDescriptor struct {
	Version               uint32
	Size                  uint32
	DeviceType            byte
	DeviceTypeModifier    byte
	RemovableMedia        byte
	CommandQueueing       byte
	VendorIDOffset        uint32
	ProductIDOffset       uint32
	ProductRevisionOffset uint32
	SerialNumberOffset    uint32
	BusType               uint32
}

// Volume is one mounted drive letter as the disk page shows it.
type Volume struct {
	Letter string // "C:"
	Label  string
	FS     string
	Media  string // SSD / HDD / 可移动 / 未知
	Total  uint64
	Free   uint64
	Busy   float64 // 0..100, filled from the PDH disk sample when available
}

// Volumes lists every mounted letter that reports a size, ordered C first and
// then by letter. Media detection is best effort: a refused ioctl leaves the
// string empty rather than guessing "SSD".
func Volumes() []Volume {
	bits, _, _ := pGetLogicalDrives.Call()
	order := []string{"C", "D", "E", "F", "G", "H", "I", "J"}
	seen := map[string]bool{}
	var out []Volume
	add := func(letter string) {
		if seen[letter] {
			return
		}
		seen[letter] = true
		v, ok := volumeInfo(letter)
		if ok {
			out = append(out, v)
		}
	}
	for _, l := range order {
		if bits&(1<<uint(int(l[0]-'A'))) != 0 {
			add(l)
		}
	}
	for i := 0; i < 26; i++ {
		if bits&(1<<uint(i)) != 0 {
			add(string(rune('A' + i)))
		}
	}
	return out
}

func volumeInfo(letter string) (Volume, bool) {
	root, err := syscall.UTF16PtrFromString(letter + `:\`)
	if err != nil {
		return Volume{}, false
	}
	typ, _, _ := pGetDriveTypeW.Call(uintptr(unsafe.Pointer(root)))
	var freeBytes, totalBytes, totalFree uint64
	r, _, _ := pGetDiskFreeSpaceExW.Call(uintptr(unsafe.Pointer(root)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFree)))
	if r == 0 || totalBytes == 0 {
		return Volume{}, false
	}
	v := Volume{Letter: letter + ":", Total: totalBytes, Free: freeBytes}

	labelBuf := make([]uint16, 64)
	fsBuf := make([]uint16, 64)
	var serial, maxLen, flags uint32
	pGetVolumeInformationW.Call(uintptr(unsafe.Pointer(root)),
		uintptr(unsafe.Pointer(&labelBuf[0])), uintptr(len(labelBuf)),
		uintptr(unsafe.Pointer(&serial)),
		uintptr(unsafe.Pointer(&maxLen)),
		uintptr(unsafe.Pointer(&flags)),
		uintptr(unsafe.Pointer(&fsBuf[0])), uintptr(len(fsBuf)))
	v.Label = windows.UTF16ToString(labelBuf)
	v.FS = windows.UTF16ToString(fsBuf)

	switch typ {
	case driveRemovable:
		v.Media = "可移动"
	case driveFixed:
		v.Media = diskMediaKind(letter)
	default:
		v.Media = "网络/其他"
	}
	return v, true
}

// diskMediaKind answers the only question that matters for a "should I
// optimize this volume" decision: does the device incur seek penalties.
func diskMediaKind(letter string) string {
	path, _ := syscall.UTF16PtrFromString(`\\.\` + letter + ":")
	h, _, _ := pCreateFileW.Call(uintptr(unsafe.Pointer(path)),
		0, // no access: the query does not need read rights
		uintptr(syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE),
		0, uintptr(syscall.OPEN_EXISTING), 0, 0)
	if h == 0 || h == uintptr(syscall.InvalidHandle) {
		return ""
	}
	defer pCloseHandle.Call(h)

	q := storagePropertyQuery{PropertyID: storageDeviceSeekPenalty}
	var desc deviceSeekPenalty
	var ret uint32
	r, _, _ := pDeviceIoControl.Call(h, ioctlStorageQueryProperty,
		uintptr(unsafe.Pointer(&q)), unsafe.Sizeof(q),
		uintptr(unsafe.Pointer(&desc)), unsafe.Sizeof(desc),
		uintptr(unsafe.Pointer(&ret)), 0)
	if r != 0 {
		if desc.IncursSeekPenalty != 0 {
			return "HDD"
		}
		return "SSD"
	}

	// Seek penalty unavailable: fall back to the bus type so a USB stick is
	// not silently labelled as an SSD.
	buf := make([]byte, 1024)
	q = storagePropertyQuery{PropertyID: storageDeviceProperty}
	r, _, _ = pDeviceIoControl.Call(h, ioctlStorageQueryProperty,
		uintptr(unsafe.Pointer(&q)), unsafe.Sizeof(q),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
		uintptr(unsafe.Pointer(&ret)), 0)
	if r == 0 || len(buf) < int(unsafe.Sizeof(storageDeviceDescriptor{})) {
		return ""
	}
	sd := (*storageDeviceDescriptor)(unsafe.Pointer(&buf[0]))
	if sd.RemovableMedia != 0 {
		return "可移动"
	}
	switch sd.BusType {
	case 4, 7, 12, 13: // 1394 / USB / SD / MMC
		return "外接"
	}
	return ""
}

// DiskIO is one physical disk's sampled throughput.
type DiskIO struct {
	Index    string // "0", "1", "_Total"
	ReadMBs  float64
	WriteMBs float64
	IOPS     float64
	Latency  float64 // milliseconds per transfer
	Queue    float64
}

// DiskUsage samples the PhysicalDisk counter set. Like GPUUsage it needs two
// collections and must run off the UI goroutine.
func DiskUsage() ([]DiskIO, error) {
	var query uintptr
	if r, _, _ := pPdhOpenQueryW.Call(0, 0, uintptr(unsafe.Pointer(&query))); r != 0 {
		return nil, fmt.Errorf("PdhOpenQuery: 0x%08x", uint32(r))
	}
	defer pPdhCloseQuery.Call(query)

	counters := []struct {
		path string
		idx  int
	}{
		{`\PhysicalDisk(*)\Disk Read Bytes/sec`, 0},
		{`\PhysicalDisk(*)\Disk Write Bytes/sec`, 1},
		{`\PhysicalDisk(*)\Disk Transfers/sec`, 2},
		{`\PhysicalDisk(*)\Avg. Disk sec/Transfer`, 3},
		{`\PhysicalDisk(*)\Current Disk Queue Length`, 4},
	}
	handles := make([]uintptr, len(counters))
	for i, c := range counters {
		p, _ := syscall.UTF16PtrFromString(c.path)
		if r, _, _ := pPdhAddEnglishCounterW.Call(query, uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&handles[i]))); r != 0 {
			return nil, fmt.Errorf("PdhAddEnglishCounter %s", c.path)
		}
	}
	if r, _, _ := pPdhCollectQueryData.Call(query); r != 0 {
		return nil, errors.New("PdhCollectQueryData failed")
	}
	time.Sleep(250 * time.Millisecond)
	if r, _, _ := pPdhCollectQueryData.Call(query); r != 0 {
		return nil, errors.New("PdhCollectQueryData failed")
	}

	agg := map[string]*DiskIO{}
	for i, h := range handles {
		items, err := pdhArray(h)
		if err != nil {
			continue
		}
		for _, it := range items {
			d := agg[it.name]
			if d == nil {
				d = &DiskIO{Index: it.name}
				agg[it.name] = d
			}
			switch counters[i].idx {
			case 0:
				d.ReadMBs = it.value / (1024 * 1024)
			case 1:
				d.WriteMBs = it.value / (1024 * 1024)
			case 2:
				d.IOPS = it.value
			case 3:
				// Avg. Disk sec/Transfer is seconds per operation.
				d.Latency = it.value * 1000
			case 4:
				d.Queue = it.value
			}
		}
	}
	// _Total is kept: it is the only instance that answers "how busy is this
	// machine's storage as a whole", and the page shows it as its own row.
	out := make([]DiskIO, 0, len(agg))
	for _, d := range agg {
		out = append(out, *d)
	}
	sortDiskIO(out)
	return out, nil
}

func sortDiskIO(v []DiskIO) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0; j-- {
			a, b := v[j-1].Index, v[j].Index
			an, aerr := strconv.Atoi(a)
			bn, berr := strconv.Atoi(b)
			swap := false
			if aerr == nil && berr == nil {
				swap = an > bn
			} else {
				swap = strings.Compare(a, b) > 0
			}
			if !swap {
				break
			}
			v[j-1], v[j] = v[j], v[j-1]
		}
	}
}

// pdhCounterItem is one (instance, value) pair from a wildcard counter.
type pdhCounterItem struct {
	name  string
	value float64
}

// pdhArray reads one wildcard counter as a list of (instance, value) pairs.
func pdhArray(counter uintptr) ([]pdhCounterItem, error) {
	var size, count uint32
	r, _, _ := pPdhGetFormattedCounterArrayW.Call(counter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if uint32(r) != pdhMoreData || size == 0 {
		return nil, errors.New("no counter instances")
	}
	buf := make([]byte, size)
	r, _, _ = pPdhGetFormattedCounterArrayW.Call(counter, pdhFmtDouble,
		uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 {
		return nil, errors.New("PdhGetFormattedCounterArray failed")
	}
	items := unsafe.Slice((*pdhFmtItem)(unsafe.Pointer(&buf[0])), int(count))
	out := make([]pdhCounterItem, 0, len(items))
	for _, it := range items {
		out = append(out, pdhCounterItem{
			name:  windows.UTF16PtrToString(it.Name),
			value: it.Value.Double,
		})
	}
	return out, nil
}

// ---- volume maintenance ------------------------------------------------

// VolumeReport is the parsed result of one defrag/Optimize-Volume operation.
// Everything the UI shows comes from the tool's own report: a volume is never
// described as fragmented or optimised on a guess.
type VolumeReport struct {
	Volume   string // "D:"
	Action   string // analyze | optimize | retrim
	OK       bool   // the tool reported success
	FragPct  int    // total fragmented space, -1 when the report omitted it
	NeedWork bool   // the report recommended defragmenting
	TotalGB  float64
	FreeGB   float64
	Detail   string // trimmed report text, for the log and the row subtitle
	Err      string
}

var defragNumRE = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*GB`)
var defragPctRE = regexp.MustCompile(`总碎片空间\s*=\s*([0-9]+)\s*%`)

// RunDefrag runs one volume operation and parses the report. analyze, optimize
// and retrim map to /A, /O and /L. The call blocks for as long as the volume
// takes, so callers must run it off the UI goroutine.
func RunDefrag(letter, action string) VolumeReport {
	rep := VolumeReport{Volume: letter, Action: action, FragPct: -1}
	flag := map[string]string{"analyze": "/A", "optimize": "/O", "retrim": "/L"}[action]
	if flag == "" {
		rep.Err = "未知操作"
		return rep
	}
	cmd := exec.Command("defrag", letter, flag)
	hideWindow(cmd)
	raw, err := cmd.CombinedOutput()
	out := decodeConsole(raw)
	rep.Detail = condenseDefrag(out)
	if err != nil {
		// defrag exits non-zero for a refused operation (a volume whose
		// hardware has no TRIM support, for example); the console text is the
		// only useful part of that.
		rep.Err = firstDefragError(out)
		if rep.Err == "" {
			rep.Err = err.Error()
		}
		return rep
	}
	rep.OK = true
	if m := defragPctRE.FindStringSubmatch(out); m != nil {
		rep.FragPct, _ = strconv.Atoi(m[1])
	}
	nums := defragNumRE.FindAllStringSubmatch(out, -1)
	if len(nums) >= 2 {
		rep.TotalGB, _ = strconv.ParseFloat(nums[0][1], 64)
		rep.FreeGB, _ = strconv.ParseFloat(nums[1][1], 64)
	}
	rep.NeedWork = strings.Contains(out, "建议对该卷进行碎片整理")
	if action == "retrim" {
		rep.NeedWork = false
	}
	return rep
}

// condenseDefrag keeps the lines a person can act on and drops the report's
// boilerplate, so a row subtitle never becomes a wall of text.
func condenseDefrag(out string) string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case strings.Contains(line, "碎片空间"),
			strings.Contains(line, "不需要对该卷"),
			strings.Contains(line, "建议对该卷"),
			strings.Contains(line, "重新剪裁"),
			strings.Contains(line, "操作成功完成"),
			strings.Contains(line, "卷大小"),
			strings.Contains(line, "可用空间"):
			keep = append(keep, strings.Join(strings.Fields(line), " "))
		}
	}
	if len(keep) > 4 {
		keep = keep[len(keep)-4:]
	}
	return strings.Join(keep, " · ")
}

// firstDefragError pulls the tool's own one-line failure text.
func firstDefragError(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "(0x") {
			return strings.Join(strings.Fields(line), " ")
		}
	}
	return ""
}

// TrimEnabled reports whether NTFS delete-notify (TRIM) is on. The second
// result is false when the value cannot be read; the UI then shows "—" rather
// than claiming TRIM is enabled.
func TrimEnabled() (bool, bool) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\FileSystem`, registry.QUERY_VALUE)
	if err != nil {
		return false, false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("DisableDeleteNotify")
	if err != nil {
		// Absent means the default, which is enabled.
		return true, true
	}
	return v == 0, true
}
