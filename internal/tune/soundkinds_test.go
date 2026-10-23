package tune

import "testing"

func TestSoundKindGates(t *testing.T) {
	mask := "1011"
	if !SoundKindOn(mask, 0) || SoundKindOn(mask, 1) || !SoundKindOn(mask, 2) || !SoundKindOn(mask, 3) {
		t.Fatalf("unexpected gates for %q", mask)
	}
	for _, i := range []int{-1, 4} {
		if SoundKindOn(mask, i) {
			t.Fatalf("out-of-range index %d should be off", i)
		}
	}
	if !SoundKindOn("", 0) {
		t.Fatal("malformed mask falls back to all cues on")
	}
}
