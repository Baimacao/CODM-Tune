package sys

import (
	"math/bits"
	"testing"
)

func TestSchedulingMasks(t *testing.T) {
	n := LogicalCPUs()
	single := SingleCore()
	if bits.OnesCount64(single) != 1 {
		t.Fatalf("single-core mask must have one bit: %s", MaskLabel(single))
	}
	if n > 1 && single&1 != 0 && EfficiencyCore() == 0 {
		t.Fatalf("without E-cores the last logical CPU should be chosen: %s", MaskLabel(single))
	}
	game := GameMask()
	if n > 1 && game&1 != 0 {
		t.Fatalf("game mask must exclude CPU 0: %s", MaskLabel(game))
	}
	if bits.OnesCount64(game) < n-1 {
		t.Fatalf("game mask lost cores: %s for %d CPUs", MaskLabel(game), n)
	}
}
