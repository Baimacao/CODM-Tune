// Package spring is a native port of the lilt/osu-mc motion model
// (github.com/chhongzh/lilt/anim). Every moving value in the UI is a Value
// driven by a Spring described the SwiftUI way: Response is the period of the
// undamped oscillation in seconds and Damping is the fraction of critical
// damping. There are no durations or easing curves in the interface.
package spring

// Spring describes motion. Ease drops the spring for exponential easing: full
// speed from the first frame, no momentum and no overshoot.
type Spring struct {
	Response float32
	Damping  float32
	Ease     bool
}

const twoPi = 6.2831855

// Presets are pointers so retuning one at runtime changes every value that
// uses it. The numbers are the ones lilt ships with.
var (
	// Default moves list items and layout, with a light overshoot.
	Default = &Spring{Response: 0.29, Damping: 0.75}
	// Smooth carries whole screens in and out.
	Smooth = &Spring{Response: 0.39, Damping: 0.85}
	// Snappy is for thumbs and progress that should track closely.
	Snappy = &Spring{Response: 0.31, Damping: 0.8}
	// Bouncy pops marks in so they land with a snap.
	Bouncy = &Spring{Response: 0.25, Damping: 0.45}
	// Interactive follows a finger or a key going down: quick, no bounce.
	Interactive = &Spring{Response: 0.14, Damping: 1}
	// CursorLead trails behind the edge on the side of travel.
	CursorLead = &Spring{Response: 0.22, Ease: true}
	// CursorTrail is the slower opposite edge, so the cursor stretches toward
	// where focus is going and contracts as it arrives.
	CursorTrail = &Spring{Response: 0.52, Ease: true}
	// CursorRelease brings the cursor back from a press with a wobble.
	CursorRelease = &Spring{Response: 0.57, Damping: 0.4}
	// CursorLeap lifts the cursor across screens and lands it with a squash.
	CursorLeap = &Spring{Response: 0.63, Damping: 0.45}
	// CursorLean moves the cursor toward a held key and back.
	CursorLean = &Spring{Response: 0.35, Damping: 0.7}
)

// TimeScale slows or speeds up all animation; useful when inspecting motion.
var TimeScale float32 = 1

// RubberBandCoefficient matches UIScrollView's overscroll resistance.
var RubberBandCoefficient float32 = 0.55

const substep = 1.0 / 240

// Value is a float that springs toward its target. The zero value is ready to
// use, rests at 0 and follows Default. Values must not be copied once
// animating because the scheduler holds a pointer to them.
type Value struct {
	current   float32
	target    float32
	velocity  float32
	precision float32
	spring    *Spring
	parent    *Value
	active    bool
}

// New returns a value resting at v.
func New(v float32, s *Spring) *Value {
	return &Value{current: v, target: v, spring: s}
}

var active []*Value

// Get returns the displayed value, including the parent's.
func (v *Value) Get() float32 {
	if v.parent != nil {
		return v.current + v.parent.Get()
	}
	return v.current
}

// Current returns the value without the parent's contribution.
func (v *Value) Current() float32 { return v.current }

// Target returns where the value is heading, without the parent's.
func (v *Value) Target() float32 { return v.target }

// Velocity is in units per second.
func (v *Value) Velocity() float32 { return v.velocity }

// Settled reports whether the value is at rest.
func (v *Value) Settled() bool { return !v.active }

// Set animates toward x.
func (v *Value) Set(x float32) {
	if x == v.target {
		return
	}
	v.target = x
	v.wake()
}

// Jump moves to x immediately and stops any motion.
func (v *Value) Jump(x float32) {
	v.current, v.target, v.velocity = x, x, 0
	v.active = false
}

// Shift moves the current value by d while keeping the target, so the value
// glides back. Lists use it to animate position changes.
func (v *Value) Shift(d float32) {
	if d == 0 {
		return
	}
	v.current += d
	v.wake()
}

// Kick adds velocity, for feedback such as a nudge at a list edge.
func (v *Value) Kick(velocity float32) {
	v.velocity += velocity
	v.wake()
}

// Bump kicks the value hard enough that, critically damped and at rest, it
// swings about px away before coming back. Bouncier springs swing somewhat
// further. Eased values ignore it.
func (v *Value) Bump(px float32) {
	v.Kick(px * v.curve().rate() * e)
}

// SetSpring changes the physics; nil means Default.
func (v *Value) SetSpring(s *Spring) { v.spring = s }

// SetParent makes Get add p's value, so a child follows its parent's motion.
func (v *Value) SetParent(p *Value) { v.parent = p }

// SetPrecision sets the distance under which the value counts as settled. The
// default of 0.05 suits pixel values; use smaller ones for 0..1 progress.
func (v *Value) SetPrecision(p float32) { v.precision = p }

func (v *Value) wake() {
	if !v.active {
		v.active = true
		active = append(active, v)
	}
}

func (v *Value) curve() *Spring {
	if v.spring == nil {
		return Default
	}
	return v.spring
}

func (v *Value) step(dt float32) {
	s := v.curve()
	x, vel := v.current-v.target, v.velocity
	if s.Ease {
		x, vel = x*expNeg(s.rate()*dt), 0
	} else {
		k, c := s.coefficients()
		n := int(dt/substep) + 1
		h := dt / float32(n)
		for i := 0; i < n; i++ {
			vel += (-k*x - c*vel) * h
			x += vel * h
		}
	}
	p := v.precision
	if p == 0 {
		p = 0.05
	}
	if abs(x) < p && abs(vel) < p*10 {
		v.current, v.velocity, v.active = v.target, 0, false
		return
	}
	v.current, v.velocity = v.target+x, vel
}

// Step advances every active value by dt seconds and reports whether any
// motion remains. It compacts the active list in place, so no allocation
// happens per frame.
func Step(dt float32) bool {
	if TimeScale <= 0 {
		return false
	}
	dt *= TimeScale
	if dt > 0.1 {
		dt = 0.1
	}
	if dt <= 0 {
		return len(active) > 0
	}
	n := 0
	for _, v := range active {
		v.step(dt)
		if v.active {
			active[n] = v
			n++
		}
	}
	clear(active[n:])
	active = active[:n]
	return n > 0
}

// Awake is the number of values still moving. The UI uses it to stop the
// frame loop and put the spring system to sleep.
func Awake() int { return len(active) }

// RubberBand maps a raw overscroll distance to the displayed one. The result
// approaches but never reaches dim, so pushing harder yields less movement.
func RubberBand(x, dim float32) float32 {
	if dim <= 0 {
		return 0
	}
	neg := x < 0
	if neg {
		x = -x
	}
	y := (1 - 1/(x*RubberBandCoefficient/dim+1)) * dim
	if neg {
		return -y
	}
	return y
}

func (s *Spring) coefficients() (k, c float32) {
	w := s.rate()
	return w * w, 2 * s.Damping * w
}

// rate is the angular frequency, or the easing rate, in 1/s.
func (s *Spring) rate() float32 { return twoPi / s.Response }

const e = 2.7182817

// expNeg approximates e^(-x) for x >= 0 with a squared (2,2) Pade approximant.
func expNeg(x float32) float32 {
	if x <= 0 {
		return 1
	}
	if x > 16 {
		return 0
	}
	n := 0
	for x > 0.5 {
		x *= 0.5
		n++
	}
	x2 := x * x / 12
	h := x / 2
	r := (1 - h + x2) / (1 + h + x2)
	for ; n > 0; n-- {
		r *= r
	}
	return r
}

func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
