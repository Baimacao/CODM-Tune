package ui

import (
	"strings"
	"sync"
	"sync/atomic"

	"codmtune/internal/spring"
)

var current *App

// App owns the screen stack, focus, cursor and the frame loop. There is one
// per process.
type App struct {
	W, H    int
	Th      *Theme
	Screens Screens
	Focus   *Node
	Cursor  Cursor
	// OnSound plays a short interaction cue: move, press, confirm, back, error.
	OnSound func(kind string)
	// BgImage is an optional full-window backdrop; BgDim is the veil over it.
	BgImage string
	// GridImage is a pre-rendered Ark grid/scanline layer. Drawing it as one
	// blit keeps the idle frame cheap instead of issuing ~430 GDI lines.
	GridImage string
	BgDim     uint8

	dirty atomic.Bool
	posts []func()
	mu    sync.Mutex

	down     *Node
	mouseX   float32
	mouseY   float32
	pressed  bool
	lastList *List
	keyNode  *Node
	quiet    bool
	wipeOn   bool
	wipeT    float32
	wipeDur  float32

	hazardOn  bool
	hazardT   float32
	hazardDur float32
}

// NewApp makes a app and sets it current.
func NewApp(w, h int) *App {
	a := &App{W: w, H: h, Th: CurrentTheme}
	a.Screens.W = float32(w)
	a.Cursor.init()
	current = a
	return a
}

// UseTheme switches the palette used for backgrounds and the cursor.
func (a *App) UseTheme(t *Theme) { a.Th = t; a.dirty.Store(true) }

// HazardFlash announces a dangerous-mode change with a red sweep. It is a
// short, explicit frame animation rather than an instant palette swap.
func (a *App) HazardFlash() {
	a.hazardOn = true
	a.hazardT = 0
	a.hazardDur = 0.95
	a.dirty.Store(true)
}

// Switch replaces the whole screen stack with one screen, used by the
// category rail. It keeps a single host so navigation history does not grow.
func (a *App) Switch(n *Node) {
	// Category switches drop the old screen instead of sliding both: the old
	// slide made the new background expose a seam and read as a stutter. Ark
	// uses a short clipped wipe; MD3E uses a quiet 14 px settle.
	if old := a.Screens.Top(); old != nil {
		old.exiting = true
	}
	h := &Screen{Content: n}
	h.OX.SetSpring(spring.Smooth)
	if a.Th.Style == "ark" {
		h.OX.Jump(0)
		a.wipeOn = true
		a.wipeT = 0
		a.wipeDur = 0.78
	} else {
		h.OX.Jump(14)
		a.wipeOn = false
	}
	a.Screens.Hosts = append(a.Screens.Hosts, h)
	h.OX.Set(0)
	a.quiet = true
	a.Focus = firstFocusable(n)
	a.Cursor.Settle()
	a.dirty.Store(true)
}

// SwitchSolo replaces the stack without keeping the previous screen. It is
// used when the window itself changes shape (F11 fullscreen): scaling the old
// 1100x700 page across a 1920x1080 surface during the wipe reads as a violent
// zoom rather than a transition.
func (a *App) SwitchSolo(n *Node) {
	a.Screens.Hosts = nil
	h := &Screen{Content: n}
	h.OX.SetSpring(spring.Smooth)
	h.OX.Jump(0)
	a.Screens.Hosts = append(a.Screens.Hosts, h)
	// The terminal itself animates its scanline on entry. Starting a second
	// wipe here leaves the right half showing whatever was in the resized
	// surface, which reads as a ghost after F11 exit.
	a.wipeOn = false
	a.wipeT = 0
	a.wipeDur = 0
	a.quiet = false
	a.Focus = firstFocusable(n)
	a.Cursor.Settle()
	a.dirty.Store(true)
}

func (a *App) sound(kind string) {
	if a.OnSound != nil {
		a.OnSound(kind)
	}
}

// Current returns the app created last.
func Current() *App { return current }

// Invalidate marks the frame dirty. It is safe on the UI goroutine only.
func Invalidate() {
	if current != nil {
		current.dirty.Store(true)
	}
}

// Post runs fn on the UI goroutine at the start of the next frame.
func (a *App) Post(fn func()) {
	a.mu.Lock()
	a.posts = append(a.posts, fn)
	a.mu.Unlock()
	a.dirty.Store(true)
}

func (a *App) drain() {
	a.mu.Lock()
	posts := a.posts
	a.posts = nil
	a.mu.Unlock()
	for _, fn := range posts {
		fn()
	}
}

// Root installs the first screen; it cannot be popped.
func (a *App) Root(n *Node) {
	a.Screens.Hosts = nil
	h := &Screen{Content: n}
	h.OX.SetSpring(spring.Smooth)
	a.Screens.Hosts = append(a.Screens.Hosts, h)
	a.Focus = firstFocusable(n)
	a.dirty.Store(true)
}

// Push slides a new screen in and moves focus to its first focusable node.
func (a *App) Push(n *Node) {
	if top := a.Screens.Top(); top != nil {
		top.Restore = a.Focus
	}
	a.Focus = nil
	a.Cursor.Leap()
	a.Screens.Push(n)
	for _, f := range focusables(n) {
		if d, ok := f.Data.(string); ok && strings.HasPrefix(d, "nav:") {
			continue
		}
		a.Focus = f
		a.reveal(f)
		break
	}
	a.dirty.Store(true)
}

// Pop dismisses the top screen and restores its focus.
func (a *App) Pop() bool {
	top := a.Screens.Top()
	if top == nil || len(a.Screens.Hosts) <= 1 {
		return false
	}
	a.Focus = nil
	a.Cursor.Leap()
	ok := a.Screens.Pop()
	if prev := a.Screens.Top(); prev != nil {
		if prev.Restore != nil {
			a.Focus = prev.Restore
		} else {
			a.Focus = firstFocusable(prev.Content)
		}
	}
	a.dirty.Store(true)
	return ok
}

// SetFocus moves focus, ignoring hidden or unfocusable nodes.
func (a *App) SetFocus(n *Node) {
	if n != nil && (!n.Focusable || n.Disabled || !shown(n)) {
		return
	}
	if a.Focus == n {
		return
	}
	a.Focus = n
	a.Cursor.Settle()
	if n != nil {
		a.reveal(n)
	}
	a.sound("move")
	a.dirty.Store(true)
}

func shown(n *Node) bool {
	for ; n != nil; n = n.Parent {
		if !n.Visible {
			return false
		}
	}
	return true
}

// reveal scrolls every list ancestor so n is inside the viewport.
func (a *App) reveal(n *Node) {
	for m := n; m != nil; m = m.Parent {
		if m.List != nil {
			m.List.Reveal(n)
		}
	}
}

// Key routes one key event: the focused node sees it first, then navigation.
func (a *App) Key(k Key, ph Phase) {
	a.dirty.Store(true)
	top := a.Screens.Top()
	if top == nil {
		return
	}
	f := a.Focus
	if f != nil && f.OnKey != nil && f.OnKey(f, k, ph) {
		return
	}
	switch k {
	case KeyUp, KeyDown, KeyLeft, KeyRight:
		dx, dy := dirOf(k)
		if ph == Press {
			a.Cursor.Lean(dx*2, dy*2)
			return
		}
		a.Cursor.Lean(0, 0)
		if f == nil {
			if ph == Release || ph == Repeat {
				a.SetFocus(firstFocusable(top.Content))
			}
			return
		}
		if ph == Release || ph == Repeat {
			var next *Node
			if k == KeyUp || k == KeyDown {
				// Keep vertical travel inside the rail or the content area. Without
				// this, Up on the first content row jumps back into the sidebar.
				rail := IsRailNode(f)
				next = findFocusWhere(top.Content, f, k, func(n *Node) bool {
					return IsRailNode(n) == rail
				})
			} else {
				next = findFocus(top.Content, f, k)
			}
			if next != nil {
				a.SetFocus(next)
			}
		}
	case KeyEnter:
		if f == nil {
			a.SetFocus(firstFocusable(top.Content))
			return
		}
		switch ph {
		case Press:
			a.keyNode = f
			a.pressNode(f)
		case Release:
			n := a.keyNode
			a.keyNode = nil
			if n == nil {
				return
			}
			// The screen may have switched while Enter was down; releasing
			// must never activate the newly focused row by accident.
			a.releaseNode(n, a.Focus != n)
		}
	case KeyBack:
		if ph == Press {
			top.OX.Set(8)
			return
		}
		top.OX.Set(0)
		if !a.Pop() {
			top.OX.Bump(6)
			a.Cursor.Nudge(6)
		}
	case KeyPageDown, KeyPageUp:
		if l := a.focusedList(f); l != nil {
			d := float32(l.viewH) * 0.8
			if k == KeyPageUp {
				d = -d
			}
			l.ScrollBy(d)
		}
	case KeyHome, KeyEnd:
		fs := focusables(top.Content)
		if len(fs) == 0 {
			return
		}
		if k == KeyHome {
			a.SetFocus(fs[0])
		} else {
			a.SetFocus(fs[len(fs)-1])
		}
	case KeyTab:
		fs := focusables(top.Content)
		if len(fs) == 0 {
			return
		}
		idx := 0
		for i, n := range fs {
			if n == f {
				idx = i
				break
			}
		}
		a.SetFocus(fs[(idx+1)%len(fs)])
	}
}

func dirOf(k Key) (dx, dy float32) {
	switch k {
	case KeyLeft:
		return -1, 0
	case KeyRight:
		return 1, 0
	case KeyUp:
		return 0, -1
	case KeyDown:
		return 0, 1
	}
	return 0, 0
}

func (a *App) focusedList(n *Node) *List {
	for m := n; m != nil; m = m.Parent {
		if m.List != nil {
			return m.List
		}
	}
	return nil
}

// pressNode starts the press half of an interaction. Hold-to-confirm only
// arms when the node itself says this direction needs a hold.
func (a *App) pressNode(n *Node) {
	if n.Disabled {
		a.Cursor.Nudge(5)
		return
	}
	n.HoldFired = false
	if n.Hold > 0 && needsHold(n) {
		n.Holding = true
		n.Progress.Jump(0)
	}
	n.Pressed()
	a.Cursor.pressed(Press)
	a.pressed = true
	a.sound("press")
}

// releaseNode commits on release unless the hold was cancelled or the pointer
// left the node.
func (a *App) releaseNode(n *Node, cancel bool) {
	n.Released()
	a.Cursor.pressed(Release)
	a.pressed = false
	if n.Holding {
		n.Holding = false
		if !n.HoldFired {
			n.Progress.Jump(0)
		}
	}
	if n.HoldFired {
		n.HoldFired = false
		return
	}
	if n.Hold > 0 && needsHold(n) {
		return
	}
	if !cancel && n.Activate != nil {
		n.Activate(n)
		a.sound("confirm")
	}
}

func needsHold(n *Node) bool {
	if n.NeedsHold == nil {
		return true
	}
	return n.NeedsHold(n)
}

// MouseMove updates hover and the cursor lean. Moving over another focusable
// node transfers focus so the ring springs across.
func (a *App) MouseMove(x, y float32) {
	a.mouseX, a.mouseY = x, y
	a.Cursor.pointerAt(x, y)
	if a.down != nil && a.down.OnMouse != nil {
		a.down.OnMouse(a.down, x, y, MouseMove)
	}
	if a.pressed {
		return
	}
	if n := a.hit(x, y); n != nil && n != a.Focus && n.Focusable {
		a.SetFocus(n)
	}
	a.dirty.Store(true)
}

func (a *App) MouseDown(x, y float32) {
	if n := a.hit(x, y); n != nil {
		a.SetFocus(n)
		a.down = n
		a.pressNode(n)
		if n.OnMouse != nil {
			n.OnMouse(n, x, y, MouseDown)
		}
	}
}

func (a *App) MouseUp(x, y float32) {
	if a.down == nil {
		a.pressed = false
		return
	}
	n := a.down
	a.down = nil
	cancel := true
	if hit := a.hit(x, y); hit != nil && hit == n {
		cancel = false
	}
	if n.OnMouse != nil {
		n.OnMouse(n, x, y, MouseUp)
	}
	a.releaseNode(n, cancel)
}

// Wheel scrolls the list under the pointer, or the focused row's list.
func (a *App) Wheel(x, y, delta float32) {
	l := a.listAt(x, y)
	if l == nil {
		l = a.focusedList(a.Focus)
	}
	if l != nil {
		// ScrollBy takes pixels, not rows. A notch is roughly half a viewport;
		// the spring smooths the jump, and the old tiny values felt like no
		// movement at all.
		l.ScrollBy(-delta * 12.0)
	}
}

func (a *App) listAt(x, y float32) *List {
	n := a.hit(x, y)
	for m := n; m != nil; m = m.Parent {
		if m.List != nil {
			return m.List
		}
	}
	return nil
}

// hit finds the deepest focusable node under a point in the top screen.
func (a *App) hit(x, y float32) *Node {
	top := a.Screens.Top()
	if top == nil {
		return nil
	}
	n := hitNode(top.Content, x, y, top.OX.Get(), 0)
	for m := n; m != nil; m = m.Parent {
		if m.Focusable && !m.Disabled {
			return m
		}
	}
	return nil
}

func hitNode(n *Node, x, y, dx, dy float32) *Node {
	if !n.Visible {
		return nil
	}
	ox := dx + n.OX.Get()
	oy := dy + n.OY.Get()
	r := n.R.Offset(ox, oy)
	if n.Clip && !r.Contains(Vec2{x, y}) {
		return nil
	}
	for i := len(n.Kids) - 1; i >= 0; i-- {
		if h := hitNode(n.Kids[i], x, y, ox, oy); h != nil {
			return h
		}
	}
	if r.Contains(Vec2{x, y}) {
		return n
	}
	return nil
}

// Frame advances the tree and draws when anything moved or was invalidated.
// It reports whether a frame was drawn; the window uses that to stop the
// animation timer when everything has settled.
func (a *App) Frame(c Painter, dt float32) bool {
	a.drain()
	top := a.Screens.Top()
	if top != nil {
		top.Content.update(dt)
	}
	for _, h := range a.Screens.Hosts {
		if h.exiting {
			h.Content.update(dt)
		}
	}
	a.Cursor.update(a.Focus, a.Screens.offset())
	if a.wipeOn {
		a.wipeT += dt
		if a.wipeT >= a.wipeDur {
			a.wipeT = a.wipeDur
			a.wipeOn = false
		}
	}
	if a.hazardOn {
		a.hazardT += dt
		if a.hazardT >= a.hazardDur {
			a.hazardOn = false
		}
	}
	moving := spring.Step(dt)
	if !a.quiet {
		a.Screens.update()
	} else if !a.wipeOn {
		if top := a.Screens.Top(); top != nil && top.OX.Settled() {
			a.quiet = false
			a.Screens.dropExiting()
			a.dirty.Store(true)
		}
	}
	if a.quiet {
		moving = true
	}
	if a.hazardOn {
		moving = true
	}
	if dt == 0 && !a.dirty.Load() {
		return false
	}
	if !a.dirty.Load() && !moving {
		return false
	}
	a.draw(c)
	a.dirty.Store(false)
	return true
}

func (a *App) draw(c Painter) {
	full := Rect{0, 0, float32(a.W), float32(a.H)}
	c.FillRect(full, a.Th.Bg)
	if a.BgImage != "" {
		if iw, ih := c.ImageSize(a.BgImage); iw > 0 && ih > 0 {
			s := maxf(float32(a.W)/float32(iw), float32(a.H)/float32(ih))
			dw, dh := float32(iw)*s, float32(ih)*s
			dst := Rect{(float32(a.W) - dw) / 2, (float32(a.H) - dh) / 2, dw, dh}
			if c.DrawImage(a.BgImage, dst, 255) && a.BgDim > 0 {
				c.FillRect(full, a.Th.Bg.WithAlpha(a.BgDim))
			}
		}
	}
	top := a.Screens.Top()
	wipeP := float32(0)
	if a.wipeOn && a.wipeDur > 0 {
		wipeP = clampf(a.wipeT/a.wipeDur, 0, 1)
		// Ease-out cubic: quick to leave the old frame, long soft landing. The
		// wipe is only painted on the incoming screen, so this curve is the
		// whole transition — no second full-screen repaint per frame.
		u := 1 - wipeP
		wipeP = 1 - u*u*u
	}
	wipeX := wipeP * float32(a.W)
	for _, h := range a.Screens.Hosts {
		if h.Content == nil {
			continue
		}
		// Never repaint the outgoing page during a switch. Painting both hosts
		// was the main source of frame drops, and the old page under the wipe
		// made text from two screens collide at the seam. The incoming page is
		// clipped instead; the empty part is the theme background.
		if a.quiet && h != top {
			continue
		}
		// Without a wallpaper each screen is opaque: its background slides with
		// it so an incoming screen covers the old one instead of letting it
		// show through gaps. With a wallpaper the image stays visible.
		if a.quiet && a.wipeOn && h == top {
			c.PushClip(Rect{0, 0, wipeX, float32(a.H)})
			if a.BgImage == "" {
				c.FillRect(Rect{h.OX.Get(), 0, float32(a.W), float32(a.H)}, a.Th.Bg)
			}
			paint(h.Content, c, h.OX.Get(), 0)
			c.PopClip()
			continue
		}
		if a.BgImage == "" {
			c.FillRect(Rect{h.OX.Get(), 0, float32(a.W), float32(a.H)}, a.Th.Bg)
		}
		paint(h.Content, c, h.OX.Get(), 0)
	}
	if a.Th.Style == "ark" {
		if a.GridImage != "" {
			if iw, ih := c.ImageSize(a.GridImage); iw > 0 && ih > 0 {
				c.DrawImage(a.GridImage, Rect{0, 0, float32(a.W), float32(a.H)}, 255)
			} else {
				a.drawArkGrid(c)
			}
		} else {
			a.drawArkGrid(c)
		}
	}
	if a.wipeOn {
		x := wipeX - 120
		for i := 0; i < 2; i++ {
			c.Line(x+float32(i*18), 0, x+120+float32(i*18), float32(a.H), 10, a.Th.Tertiary.WithAlpha(26))
		}
		c.Line(x+120, 0, x+240, float32(a.H), 2, a.Th.Primary)
	}
	if a.hazardOn && a.hazardDur > 0 {
		p := clampf(a.hazardT/a.hazardDur, 0, 1)
		band := (1 - p) * 120
		y := p * float32(a.H)
		for i := 0; i < 6; i++ {
			yy := y - band + float32(i)*20
			c.Line(0, yy, float32(a.W), yy, 18, a.Th.Primary.WithAlpha(uint8(30+i*8)))
		}
		c.Line(0, y, float32(a.W), y, 2, a.Th.Primary)
		msg := "ADVANCED MODE  //  HAZARD VISIBLE"
		c.FillRect(Rect{0, y - 30, float32(a.W), 30}, a.Th.Primary.WithAlpha(230))
		c.Text(Rect{20, y - 27, float32(a.W) - 40, 24}, msg, 11, a.Th.OnPrimary, AlignLeft, true)
	}
	var cut float32
	if a.Focus != nil && a.Focus.Hold > 0 {
		if a.Focus.Holding || a.Focus.HoldFired {
			cut = a.Focus.Progress.Get()
		}
	}
	a.Cursor.draw(c, a.Th, a.Focus, cut)
}

func (a *App) drawArkGrid(c Painter) {
	grid := a.Th.Tertiary.WithAlpha(0x10)
	for y := 0; y < a.H; y += 48 {
		c.Line(0, float32(y), float32(a.W), float32(y), 1, grid)
	}
	for x := 0; x < a.W; x += 48 {
		c.Line(float32(x), 0, float32(x), float32(a.H), 1, grid)
	}
	for y := 0; y < a.H; y += 3 {
		c.Line(0, float32(y), float32(a.W), float32(y), 1, RGBA(0, 0, 0, 0x08))
	}
}

func paint(n *Node, c Painter, dx, dy float32) {
	if !n.Visible {
		return
	}
	ox := dx + n.OX.Get()
	oy := dy + n.OY.Get()
	r := n.R.Offset(ox, oy)
	if n.Clip {
		c.PushClip(r)
	}
	if n.Draw != nil {
		n.Draw(n, c, r)
	}
	for _, k := range n.Kids {
		paint(k, c, ox, oy)
	}
	if n.Clip {
		c.PopClip()
	}
	if n.DrawOver != nil {
		n.DrawOver(n, c, r)
	}
}
