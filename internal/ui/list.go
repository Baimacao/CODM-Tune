package ui

import "codmtune/internal/spring"

// List is a vertically scrolling stack of focusable rows. Scrolling moves a
// spring value; every row's OX/OY is shifted so it stays where it was on
// screen and then glides back on its own spring, the further from the focus
// the slower it follows (lilt's wave). At either end the list rubber-bands
// and springs back.
type List struct {
	View *Node

	rows     []*Node
	gap      float32
	pad      float32
	scroll   spring.Value
	pull     spring.Value
	pullRaw  float32
	pullWait float32

	contentH float32
	limit    float32
	viewH    float32

	last      float32
	focusIdx  int
	stagger   float32
	scrollBar bool
}

// NewList makes a clipped view at r. pad is horizontal padding for rows.
func NewList(r Rect, pad, gap float32) *List {
	l := &List{gap: gap, pad: pad, stagger: 0.18, scrollBar: true, focusIdx: -1}
	l.View = NewNode(r)
	l.View.Clip = true
	l.viewH = r.H
	l.scroll.SetSpring(spring.Smooth)
	l.pull.SetSpring(spring.Bouncy)
	l.scroll.SetPrecision(0.1)
	l.pull.SetPrecision(0.1)
	l.View.Update = func(n *Node, dt float32) { l.update(dt) }
	l.View.DrawOver = func(n *Node, c Painter, vr Rect) { l.drawBar(c, vr) }
	return l
}

// Add appends one row and returns it.
func (l *List) Add(row *Node) *Node {
	row.List = l
	row.R.X = l.View.R.X + l.pad
	l.rows = append(l.rows, row)
	l.View.Add(row)
	l.layout()
	return row
}

// Rows returns the rows in order.
func (l *List) Rows() []*Node { return l.rows }

// Clear removes every row, for a refresh that rebuilds from live data.
func (l *List) Clear() {
	l.View.Kids = nil
	l.rows = nil
	l.focusIdx = -1
}

// Insert puts one row at idx, used for the pinned action row in the folder
// browser. idx is clamped to the current length.
func (l *List) Insert(row *Node, idx int) {
	row.List = l
	row.R.X = l.View.R.X + l.pad
	if idx < 0 || idx > len(l.rows) {
		idx = len(l.rows)
	}
	l.rows = append(l.rows, nil)
	copy(l.rows[idx+1:], l.rows[idx:])
	l.rows[idx] = row
	l.View.Kids = append(l.View.Kids, nil)
	copy(l.View.Kids[idx+1:], l.View.Kids[idx:])
	l.View.Kids[idx] = row
	row.Parent = l.View
	l.layout()
}

// SetStagger tunes how much slower distant rows follow.
func (l *List) SetStagger(f float32) { l.stagger = f }

// ScrollBar toggles the thin thumb.
func (l *List) SetScrollBar(v bool) { l.scrollBar = v }

func (l *List) layout() {
	l.contentH = 0
	for _, r := range l.rows {
		l.contentH += r.R.H + l.gap
	}
	if l.contentH > 0 {
		l.contentH -= l.gap
	}
	l.limit = maxf(l.contentH-l.viewH, 0)
	if l.scroll.Target() > l.limit {
		l.scroll.Set(l.limit)
	}
}

// ScrollBy moves the content. Past an end it stretches and will spring back.
func (l *List) ScrollBy(d float32) {
	if d == 0 {
		return
	}
	next := clampf(l.scroll.Target()+d, 0, l.limit)
	over := l.scroll.Target() + d - next
	if over != 0 {
		l.pullRaw += over * 0.6
		l.pull.Set(spring.RubberBand(l.pullRaw, 90))
		l.pullWait = 0.25
	} else {
		l.pullRaw = 0
	}
	l.scroll.Set(next)
	Invalidate()
}

// Reveal scrolls the focused row into view.
func (l *List) Reveal(n *Node) {
	i := l.rowOf(n)
	if i < 0 {
		return
	}
	l.focusIdx = i
	top := l.contentY(i)
	bottom := top + l.rows[i].R.H
	cur := l.scroll.Target()
	const margin = 10
	if top < cur {
		l.scroll.Set(maxf(top-margin, 0))
	} else if bottom > cur+l.viewH {
		l.scroll.Set(clampf(bottom-l.viewH+margin, 0, l.limit))
	}
	Invalidate()
}

func (l *List) rowOf(n *Node) int {
	for i, r := range l.rows {
		if IsAncestor(r, n) {
			return i
		}
	}
	return -1
}

func (l *List) contentY(i int) float32 {
	y := float32(0)
	for k := 0; k < i; k++ {
		y += l.rows[k].R.H + l.gap
	}
	return y
}

func (l *List) update(dt float32) {
	if l.pullWait > 0 {
		l.pullWait -= dt
		if l.pullWait <= 0 {
			l.pullRaw = 0
			l.pull.Set(0)
		}
	}
	cur := l.scroll.Get()
	if d := cur - l.last; d != 0 {
		l.last = cur
		l.wave(d)
	}
	y := l.View.R.Y - cur + l.pull.Get()
	for _, r := range l.rows {
		if dy := y - r.R.Y; dy != 0 {
			shiftChildren(r, 0, dy)
			r.R.Y = y
		}
		y += r.R.H + l.gap
	}
}

// shiftChildren moves every descendant rect with the row, because children
// carry absolute layout rects while a list row's Y is reassigned each frame.
func shiftChildren(n *Node, dx, dy float32) {
	for _, k := range n.Kids {
		k.R.X += dx
		k.R.Y += dy
		shiftChildren(k, dx, dy)
	}
}

// wave keeps rows where they were on screen after the content jumped d, then
// lets each spring home; the spring is slower further from the focus row.
func (l *List) wave(d float32) {
	base := spring.Default
	for i, r := range l.rows {
		k := 0
		if l.focusIdx >= 0 {
			k = i - l.focusIdx
			if k < 0 {
				k = -k
			}
		} else {
			k = i
		}
		if k > 7 {
			k = 7
		}
		r.OY.SetSpring(&spring.Spring{
			Response: base.Response * (1 + l.stagger*float32(k)),
			Damping:  base.Damping,
		})
		r.OY.Shift(d)
		r.OY.Set(0)
	}
	Invalidate()
}

func (l *List) drawBar(c Painter, vr Rect) {
	if !l.scrollBar || l.limit <= 0 {
		return
	}
	track := Rect{vr.Right() - 4, vr.Y + 8, 2, vr.H - 16}
	c.FillRoundRect(track, 1, CurrentTheme.Outline)
	size := maxf(vr.H*l.viewH/(l.viewH+l.limit), 24)
	thumb := Rect{track.X, vr.Y + 8 + (vr.H-16-size)*clampf(l.scroll.Get()/maxf(l.limit, 1), 0, 1), 2, size}
	c.FillRoundRect(thumb, 1, CurrentTheme.PrimaryDim)
}
