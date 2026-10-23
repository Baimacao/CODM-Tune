package tune

import "testing"

func TestFrameStatsUsesRecentContinuousRun(t *testing.T) {
	var rows []frameRow
	now := 0.0
	// Old, slow menu run.
	for i := 0; i < 120; i++ {
		rows = append(rows, frameRow{t: now, ms: 66})
		now += 0.066
	}
	// A long pause starts a new run.
	now += 1.5
	start := now
	// Recent gameplay run: ~143 FPS, with one 40 ms hitch.
	for i := 0; i < 120; i++ {
		ms := 7.0
		if i == 60 {
			ms = 40
		}
		rows = append(rows, frameRow{t: now, ms: ms})
		now += ms / 1000
	}
	fps, low, stutter := frameStats(rows)
	if fps < 100 || fps > 190 {
		t.Fatalf("fps=%.1f, want recent run near 140", fps)
	}
	if low <= 0 || low > fps {
		t.Fatalf("low=%.1f fps=%.1f; 1%% low must be in (0, fps]", low, fps)
	}
	if stutter < 0 || stutter > 5 {
		t.Fatalf("stutter=%.2f%% out of range", stutter)
	}
	if now-start <= 0 {
		t.Fatal("bad test clock")
	}
}
