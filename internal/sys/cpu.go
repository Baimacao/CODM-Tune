package sys

import (
	"unsafe"
)

// groupAffinity mirrors GROUP_AFFINITY.
type groupAffinity struct {
	Mask     uintptr
	Group    uint16
	Reserved [3]uint16
}

// processorRelationship mirrors PROCESSOR_RELATIONSHIP on x64.
type processorRelationship struct {
	Flags           byte
	EfficiencyClass byte
	Reserved        [20]byte
	GroupCount      uint16
	GroupMask       [1]groupAffinity
}

// efficiencyMask returns the affinity mask of the highest-efficiency cores
// (Intel P/E hybrids expose EfficiencyClass 1 for E-cores), or 0 when the CPU
// reports no hybrid topology.
func efficiencyMask() uint64 {
	var needed uint32
	r, _, _ := pGetLogicalProcessorInformationEx.Call(0 /*RelationProcessorCore*/, 0, uintptr(unsafe.Pointer(&needed)))
	if r != 0 || needed == 0 {
		return 0
	}
	buf := make([]byte, needed)
	r, _, _ = pGetLogicalProcessorInformationEx.Call(0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&needed)))
	if r == 0 {
		return 0
	}
	best := byte(0)
	var bestMask uint64
	off := 0
	for off+8 <= len(buf) {
		rel := *(*uint32)(unsafe.Pointer(&buf[off]))
		size := *(*uint32)(unsafe.Pointer(&buf[off+4]))
		if size == 0 || off+int(size) > len(buf) {
			break
		}
		if rel == 0 && size >= uint32(8+unsafe.Sizeof(processorRelationship{})) {
			pr := (*processorRelationship)(unsafe.Pointer(&buf[off+8]))
			if pr.EfficiencyClass > 0 && pr.EfficiencyClass >= best && pr.GroupCount > 0 {
				best = pr.EfficiencyClass
				bestMask = uint64(pr.GroupMask[0].Mask)
			}
		}
		off += int(size)
	}
	return bestMask
}

// EfficiencyCore is one E-core on P/E CPUs; on hybrid-less CPUs it is 0.
func EfficiencyCore() uint64 { return efficiencyMask() }

// SingleCore is the affinity mask the ACE limiter uses: one E-core when the
// CPU has them, otherwise the last logical processor.
func SingleCore() uint64 {
	if m := efficiencyMask(); m != 0 {
		return m & (^m + 1) // lowest set bit
	}
	n := LogicalCPUs()
	if n <= 0 {
		return 1
	}
	return uint64(1) << uint(n-1)
}

// GameMask excludes logical CPU 0 so the game and the system never fight for
// the bootstrap processor.
func GameMask() uint64 {
	n := LogicalCPUs()
	if n <= 1 {
		return 1
	}
	full := (uint64(1) << uint(n)) - 1
	return full &^ 1
}

// MaskLabel renders a mask as a compact hex string for logs.
func MaskLabel(m uint64) string {
	const hex = "0123456789ABCDEF"
	if m == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for m > 0 {
		i--
		b[i] = hex[m&0xf]
		m >>= 4
	}
	return "0x" + string(b[i:])
}
