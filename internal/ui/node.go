package ui

import (
	"codmtune/internal/spring"
)

// MouseAction is delivered to Node.OnMouse for drag/click sliders.
type MouseAction int

const (
	MouseDown MouseAction = iota
	MouseMove
	MouseUp
)

// Node is one element in a retained screen tree. R is its laid-out rect in
// screen pixels; OX, OY and Rad spring so widgets can morph and stagger
// without any fixed-duration animation. Holding a node with Hold > 0 shrinks
// the cursor from the left until it disappears, then Activate fires once.
type Node struct {
	R     Rect
	OX    spring.Value
	OY    spring.Value
	Rad   spring.Value
	Press spring.Value

	Visible   bool
	Disabled  bool
	Focusable bool
	Clip      bool
	Hover     bool

	Parent *Node
	Kids   []*Node

	ID     string
	Data   any
	List   *List
	Radius float32 // resting corner radius; Rad follows it

	Draw     func(n *Node, c Painter, r Rect)
	DrawOver func(n *Node, c Painter, r Rect)
	Update   func(n *Node, dt float32)
	Activate func(n *Node)
	OnKey    func(n *Node, k Key, ph Phase) bool
	OnMouse  func(n *Node, x, y float32, action MouseAction)
	// NeedsHold decides per press whether this direction requires a hold;
	// nil means always. Risky toggles return true only when turning on.
	NeedsHold func(n *Node) bool

	Hold      float32
	Holding   bool
	HoldFired bool
	Progress  spring.Value

	cursorRad float32
}

// NewNode makes a visible node occupying r. Add it to a parent to draw it.
func NewNode(r Rect) *Node {
	n := &Node{R: r, Visible: true, Radius: 12}
	n.Rad.Jump(12)
	n.Progress.SetSpring(spring.Snappy)
	n.Progress.SetPrecision(0.002)
	n.Press.SetPrecision(0.01)
	return n
}

// Add attaches children and returns the parent for chaining.
func (n *Node) Add(kids ...*Node) *Node {
	for _, k := range kids {
		k.Parent = n
		n.Kids = append(n.Kids, k)
	}
	return n
}

// SetFocusable marks n as reachable by arrows and the pointer.
func (n *Node) SetFocusable(f bool) *Node { n.Focusable = f; return n }

// SetHold arms hold-to-confirm for seconds. Activate runs when the hold
// completes; release before that cancels it.
func (n *Node) SetHold(seconds float32) *Node {
	n.Hold = seconds
	return n
}

// SetCursorRadius overrides the focus ring radius; pills pass their height/2.
func (n *Node) SetCursorRadius(r float32) *Node { n.cursorRad = r; return n }

// CursorRadius is the radius the focus ring wraps n with.
func (n *Node) CursorRadius() float32 {
	if n.cursorRad > 0 {
		return n.cursorRad
	}
	return 12
}

// IsAncestor reports whether a is n or contains n.
func IsAncestor(a, n *Node) bool {
	for ; n != nil; n = n.Parent {
		if n == a {
			return true
		}
	}
	return false
}

// VisualRect is n's current on-screen rect after every spring offset and the
// screen-host slide.
func VisualRect(n *Node, hostOX float32) Rect {
	x, y := hostOX, float32(0)
	for m := n; m != nil; m = m.Parent {
		x += m.OX.Get()
		y += m.OY.Get()
	}
	return n.R.Offset(x, y)
}

// RestRect is n's rect where its springs will settle, used for focus finding
// so arrow keys aim at destinations rather than mid-flight positions.
func RestRect(n *Node, hostOX float32) Rect {
	x, y := hostOX, float32(0)
	for m := n; m != nil; m = m.Parent {
		x += m.OX.Target()
		y += m.OY.Target()
	}
	return n.R.Offset(x, y)
}

// Pressed runs the press half of the lilt interaction: press only gives
// feedback; the action waits for release.
func (n *Node) Pressed() {
	n.Press.SetSpring(spring.Interactive)
	n.Press.Set(1)
}

// Released springs the press feedback back with the wobble the reference
// cursor uses.
func (n *Node) Released() {
	n.Press.SetSpring(spring.CursorRelease)
	n.Press.Set(0)
}

func (n *Node) holdUpdate(dt float32) {
	if !n.Holding || n.Hold <= 0 {
		return
	}
	p := n.Progress.Current() + dt/n.Hold
	if p >= 1 {
		p = 1
		n.Holding = false
		n.HoldFired = true
		if n.Activate != nil {
			n.Activate(n)
		}
	}
	n.Progress.Jump(p)
}

func (n *Node) update(dt float32) {
	if !n.Visible {
		return
	}
	n.holdUpdate(dt)
	if n.Update != nil {
		n.Update(n, dt)
	}
	for _, k := range n.Kids {
		k.update(dt)
	}
}

// walk visits n and its visible descendants in tree order.
func walk(n *Node, fn func(*Node)) {
	if !n.Visible {
		return
	}
	fn(n)
	for _, k := range n.Kids {
		walk(k, fn)
	}
}
