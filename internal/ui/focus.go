package ui

// Finder implements Android's FocusFinder scoring: candidates inside the
// beam win, then the lowest 13*major^2 + minor^2 distance. It is the same
// algorithm lilt uses, so arrow travel feels identical.
type finder struct {
	dir      Key
	src      Rect
	best     *Node
	bestRect Rect
	accept   func(*Node) bool
}

func findFocus(scope *Node, from *Node, dir Key) *Node {
	return findFocusWhere(scope, from, dir, nil)
}

// findFocusWhere is findFocus restricted to nodes accepted by accept. The
// caller uses it to keep vertical travel inside the rail or the content area
// instead of letting Up jump from the first row back into the sidebar.
func findFocusWhere(scope *Node, from *Node, dir Key, accept func(*Node) bool) *Node {
	if from == nil {
		if accept == nil {
			return firstFocusable(scope)
		}
		return firstFocusableWhere(scope, accept)
	}
	f := finder{dir: dir, src: RestRect(from, 0), accept: accept}
	f.walk(scope)
	return f.best
}

func firstFocusableWhere(n *Node, accept func(*Node) bool) *Node {
	var found *Node
	walk(n, func(m *Node) {
		if found == nil && m.Focusable && !m.Disabled && accept(m) {
			found = m
		}
	})
	return found
}

// IsRailNode reports whether a node belongs to the persistent navigation rail.
// Shell nodes carry the "nav:" data marker.
func IsRailNode(n *Node) bool {
	if n == nil {
		return false
	}
	s, ok := n.Data.(string)
	return ok && len(s) >= 4 && s[:4] == "nav:"
}

func firstFocusable(n *Node) *Node {
	var found *Node
	walk(n, func(m *Node) {
		if found == nil && m.Focusable && !m.Disabled {
			found = m
		}
	})
	return found
}

func focusables(root *Node) []*Node {
	var out []*Node
	walk(root, func(n *Node) {
		if n.Focusable && !n.Disabled {
			out = append(out, n)
		}
	})
	return out
}

// Focusables returns the focusable nodes in tree order.
func Focusables(root *Node) []*Node { return focusables(root) }

func (f *finder) walk(scope *Node) { f.visit(scope) }

func (f *finder) visit(d *Node) {
	if d.Focusable && !d.Disabled && (f.accept == nil || f.accept(d)) {
		r := RestRect(d, 0)
		if f.candidate(r) && (f.best == nil || f.better(r, f.bestRect)) {
			f.best, f.bestRect = d, r
		}
	}
	for _, c := range d.Kids {
		if c.Visible {
			f.visit(c)
		}
	}
}

func (f *finder) candidate(r Rect) bool {
	s := f.src
	switch f.dir {
	case KeyRight:
		return (s.X < r.X || s.Right() <= r.X) && s.Right() < r.Right()
	case KeyLeft:
		return (s.Right() > r.Right() || s.X >= r.Right()) && s.X > r.X
	case KeyDown:
		return (s.Y < r.Y || s.Bottom() <= r.Y) && s.Bottom() < r.Bottom()
	case KeyUp:
		return (s.Bottom() > r.Bottom() || s.Y >= r.Bottom()) && s.Y > r.Y
	}
	return true
}

func (f *finder) better(a, b Rect) bool {
	if !f.candidate(a) {
		return false
	}
	if f.beamBeats(a, b) {
		return true
	}
	if f.beamBeats(b, a) {
		return false
	}
	return f.weight(a) < f.weight(b)
}

func (f *finder) inBeam(r Rect) bool {
	s := f.src
	if f.dir == KeyLeft || f.dir == KeyRight {
		return r.Bottom() > s.Y && r.Y < s.Bottom()
	}
	return r.Right() > s.X && r.X < s.Right()
}

func (f *finder) beamBeats(a, b Rect) bool {
	if !f.inBeam(a) || f.inBeam(b) {
		return false
	}
	if !f.beyond(b) || f.dir == KeyLeft || f.dir == KeyRight {
		return true
	}
	return f.major(a) < f.majorFar(b)
}

func (f *finder) beyond(r Rect) bool {
	s := f.src
	switch f.dir {
	case KeyRight:
		return s.Right() <= r.X
	case KeyLeft:
		return s.X >= r.Right()
	case KeyDown:
		return s.Bottom() <= r.Y
	}
	return s.Y >= r.Bottom()
}

func (f *finder) major(r Rect) float32 {
	s := f.src
	var d float32
	switch f.dir {
	case KeyRight:
		d = r.X - s.Right()
	case KeyLeft:
		d = s.X - r.Right()
	case KeyDown:
		d = r.Y - s.Bottom()
	case KeyUp:
		d = s.Y - r.Bottom()
	}
	return maxf(d, 0)
}

func (f *finder) majorFar(r Rect) float32 {
	s := f.src
	var d float32
	switch f.dir {
	case KeyRight:
		d = r.Right() - s.Right()
	case KeyLeft:
		d = s.X - r.X
	case KeyDown:
		d = r.Bottom() - s.Bottom()
	case KeyUp:
		d = s.Y - r.Y
	}
	return maxf(d, 1)
}

func (f *finder) weight(r Rect) float32 {
	major := f.major(r)
	sc, rc := f.src.Center(), r.Center()
	minor := absf(sc.X - rc.X)
	if f.dir == KeyLeft || f.dir == KeyRight {
		minor = absf(sc.Y - rc.Y)
	}
	return 13*major*major + minor*minor
}

func absf(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
