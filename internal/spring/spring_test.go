package spring

import (
	"math"
	"testing"
)

// run advances v at 60 Hz until settled or the budget runs out and returns the
// peak overshoot past the target.
func run(t *testing.T, v *Value, target, seconds float32) float32 {
	t.Helper()
	sign := float32(1)
	if v.Get() > target {
		sign = -1
	}
	peak := float32(0)
	for i := 0; i < int(seconds*60); i++ {
		Step(1.0 / 60)
		if d := (v.Get() - target) * sign; d > peak {
			peak = d
		}
	}
	return peak
}

func TestCriticallyDampedSettlesWithoutOvershoot(t *testing.T) {
	v := New(0, &Spring{Response: 0.3, Damping: 1})
	v.Set(100)
	over := run(t, v, 100, 2)
	if over > 0.6 {
		t.Fatalf("critical damping overshot by %.3f", over)
	}
	if v.Get() != 100 || !v.Settled() {
		t.Fatalf("value did not settle at 100: %.4f active=%v", v.Get(), !v.Settled())
	}
	if Awake() != 0 {
		t.Fatalf("scheduler kept %d values awake", Awake())
	}
}

func TestBouncyOvershoots(t *testing.T) {
	v := New(0, Bouncy)
	v.Set(100)
	over := run(t, v, 100, 2)
	if over < 3 {
		t.Fatalf("expected a visible overshoot, got %.3f", over)
	}
}

func TestShiftGlidesBack(t *testing.T) {
	v := New(50, Default)
	v.Shift(-20)
	if v.Get() != 30 {
		t.Fatalf("shift did not move the value: %.2f", v.Get())
	}
	run(t, v, 50, 2)
	if math.Abs(float64(v.Get()-50)) > 0.1 {
		t.Fatalf("value did not return to target: %.3f", v.Get())
	}
}

func TestJumpStopsMotion(t *testing.T) {
	v := New(0, Bouncy)
	v.Set(100)
	Step(1.0 / 60)
	v.Jump(12)
	if v.Velocity() != 0 || v.Get() != 12 || !v.Settled() {
		t.Fatalf("jump left motion behind: v=%.3f vel=%.3f", v.Get(), v.Velocity())
	}
}

func TestRubberBandIsMonotonicAndBounded(t *testing.T) {
	prev := float32(0)
	for x := float32(1); x < 400; x += 7 {
		y := RubberBand(x, 100)
		if y <= prev {
			t.Fatalf("rubber band not monotonic at %.1f: %.4f <= %.4f", x, y, prev)
		}
		if y >= 100 {
			t.Fatalf("rubber band reached the dimension: %.4f", y)
		}
		prev = y
	}
	if RubberBand(-30, 100) != -RubberBand(30, 100) {
		t.Fatal("rubber band is not symmetric")
	}
	if RubberBand(10, 0) != 0 {
		t.Fatal("zero dimension should map to zero")
	}
}

func TestEaseClosesTheGap(t *testing.T) {
	v := New(0, CursorLead)
	v.Set(100)
	Step(1.0 / 60)
	if v.Get() <= 0 || v.Get() >= 100 {
		t.Fatalf("eased value did not move within its first frame: %.3f", v.Get())
	}
	if v.Settled() {
		t.Fatal("value settled after one frame")
	}
}
