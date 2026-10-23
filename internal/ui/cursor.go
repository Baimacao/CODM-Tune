package ui

import "codmtune/internal/spring"

// Key is a logical navigation key. The window maps VK codes onto these.
type Key uint8

const (
	KeyNone Key = iota
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyEnter
	KeyBack
	KeyTab
	KeyPageUp
	KeyPageDown
	KeyHome
	KeyEnd
)

// Phase is the stage of a key press.
type Phase uint8

const (
	Press Phase = iota
	Repeat
	Release
)

const (
	// pressDepth is the cursor squeeze per side while pressed.
	pressDepth = 3
	// leapHeight and leapGrow lift the cursor across screens.
	leapHeight = 3
	leapGrow   = 2
	// leanFactor and leanMax let the ring follow a hovering pointer.
	leanFactor = 0.12
	leanMax    = 3
)

// Cursor is the single highlight that marks focus. Each edge moves on its own
// spring so the highlight stretches toward where focus went and then catches
// up; pressing squeezes it and release springs back with a wobble. It is the
// native port of the osu-mc cursor, scaled to a desktop window.
type Cursor struct {
	l, t, r, b spring.Value
	press      spring.Value
	lift       spring.Value
	shiftX     spring.Value
	shiftY     spring.Value

	leanX, leanY float32
	target       Rect
	has          bool
	px, py       float32
	hasPointer   bool
}

func (c *Cursor) init() {
	c.press.SetPrecision(0.01)
	c.lift.SetSpring(spring.CursorLeap)
	c.shiftX.SetSpring(spring.CursorLean)
	c.shiftY.SetSpring(spring.CursorLean)
}

func (c *Cursor) pressed(p Phase) {
	switch p {
	case Press:
		c.press.SetSpring(spring.Interactive)
		c.press.Set(1)
	case Release:
		c.press.SetSpring(spring.CursorRelease)
		c.press.Set(0)
	}
}

// Lean moves the cursor by dx, dy and holds it there: anticipation while a
// key is down, before release commits the move.
func (c *Cursor) Lean(dx, dy float32) { c.leanX, c.leanY = dx, dy }

// Settle removes keyboard anticipation so the ring lands exactly on the
// focused bounds. Without this the 2px lean from the last press stayed in the
// spring and made the box look one or two pixels off after a few moves.
func (c *Cursor) Settle() {
	c.leanX, c.leanY = 0, 0
	c.shiftX.Set(0)
	c.shiftY.Set(0)
	c.lift.Set(0)
	c.hasPointer = false
}

// Leap makes the cursor jump across screens and land with a squash.
func (c *Cursor) Leap() { c.lift.Bump(leapHeight) }

// Nudge throws the cursor sideways and lets it spring back.
func (c *Cursor) Nudge(px float32) { c.shiftX.Bump(px) }

func (c *Cursor) pointerAt(x, y float32) { c.hasPointer, c.px, c.py = true, x, y }

func (c *Cursor) update(f *Node, hostOX float32) {
	if f == nil || !f.Visible {
		if c.has {
			c.has = false
			m := c.target.Center()
			c.moveTo(Rect{m.X, m.Y, 0, 0})
		}
		c.shiftX.Set(0)
		c.shiftY.Set(0)
		return
	}
	r := VisualRect(f, hostOX)
	if !c.has {
		m := r.Center()
		c.l.Jump(m.X)
		c.r.Jump(m.X)
		c.t.Jump(m.Y)
		c.b.Jump(m.Y)
		c.has = true
	}
	if r != c.target {
		c.moveTo(r)
	}
	lx, ly := c.leanX, c.leanY
	c.shiftX.Set(lx)
	c.shiftY.Set(ly)
}

func (c *Cursor) moveTo(r Rect) {
	old, m := c.target.Center(), r.Center()
	edgeSpring(&c.l, &c.r, m.X-old.X)
	edgeSpring(&c.t, &c.b, m.Y-old.Y)
	c.target = r
	c.l.Set(r.X)
	c.t.Set(r.Y)
	c.r.Set(r.Right())
	c.b.Set(r.Bottom())
}

// edgeSpring gives the edge facing the direction of travel the faster spring.
func edgeSpring(lo, hi *spring.Value, d float32) {
	switch {
	case d > 0:
		lo.SetSpring(spring.CursorTrail)
		hi.SetSpring(spring.CursorLead)
	case d < 0:
		lo.SetSpring(spring.CursorLead)
		hi.SetSpring(spring.CursorTrail)
	default:
		lo.SetSpring(spring.CursorLead)
		hi.SetSpring(spring.CursorLead)
	}
}

// draw paints the ring. cut is the hold-to-confirm progress: the ring is eaten
// from the left so it runs out at the right edge when the hold completes.
func (c *Cursor) draw(cv Painter, th *Theme, f *Node, cut float32) {
	lift := c.lift.Get()
	in := c.press.Get()*pressDepth - lift*leapGrow/leapHeight
	dx, dy := c.shiftX.Get(), c.shiftY.Get()-lift
	r := Rect{c.l.Get() + in + dx, c.t.Get() + in/2 + dy, 0, 0}
	r.W = c.r.Get() - in + dx - r.X
	r.H = c.b.Get() - in/2 + dy - r.Y
	if r.W < 2 || r.H < 2 {
		return
	}
	if cut > 0 {
		cutPx := r.W * clampf(cut, 0, 1)
		r.X += cutPx
		r.W -= cutPx
		if r.W < 2 {
			return
		}
	}
	rad := float32(12)
	if f != nil {
		rad = f.CursorRadius()
	}
	if th.Style == "ark" {
		rad = 2
	}
	// StrokeRoundRect centres its 2px stroke on the rect; inset by 1.5px so the
	// painted ring sits just inside the card border instead of one pixel out.
	r = r.Inset(1.5)
	cv.StrokeRoundRect(r, rad, 2, th.Primary)
}
