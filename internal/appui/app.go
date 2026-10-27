// Package appui builds the native screens on top of the ui toolkit and the
// tune controller. Every visual state is read live from the controller; the
// screens themselves never own tuner state.
package appui

import (
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"codmtune/internal/crosshair"
	"codmtune/internal/hud"
	"codmtune/internal/spring"
	"codmtune/internal/sys"
	"codmtune/internal/tune"
	"codmtune/internal/ui"
	"codmtune/internal/win32"
)

const (
	WinW  = 1100
	WinH  = 700
	RailW = 196
	PadX  = 224
	Wide  = WinW - PadX - 24
	RowW  = Wide - 12
	FootY = 656
)

type screens struct {
	app       *ui.App
	c         *tune.Control
	active    string
	title     string
	sub       string
	fsReturn  string
	termStart time.Time
	scanKind  string
	scanMsg   string
	scanStart time.Time
	oobeOn    bool
	oobeStep  int
	oobeAt    time.Time
	uiReady   atomic.Bool
}

// Run wires the controller, the toolkit and the native window together.
func Run() error {
	if !win32.AcquireSingleInstance("CODMTune") {
		return nil
	}
	// The GDI+ shim must exist before applyTheme tries to switch the font
	// family; win32.Run loads it again later, which is a no-op.
	if err := win32.LoadUI(); err != nil {
		return err
	}
	c := tune.NewControl()
	if err := sys.EnableDebugPrivilege(); err != nil {
		c.LogLine("SeDebugPrivilege 未启用：" + err.Error())
	}
	a := ui.NewApp(WinW, WinH)
	c.SetNotify(func() { a.Post(func() {}) })
	s := &screens{app: a, c: c}
	s.applyTheme()
	a.Root(s.boot())
	time.AfterFunc(180*time.Millisecond, func() {
		a.Post(func() {
			if a.OnSound != nil {
				a.OnSound("startup")
			}
		})
	})
	time.AfterFunc(1500*time.Millisecond, func() {
		a.Post(func() {
			if !c.Cfg().OOBEDone {
				s.enterOOBE()
				return
			}
			s.switchTo("overview")
			s.uiReady.Store(true)
		})
	})
	go s.overlayDriver()
	go s.crosshairDriver()
	c.Start()
	win32.SetFullscreenHook(s.toggleFullscreen)
	win32.SetTrayHooks(func() (bool, bool) {
		cfg := c.Cfg()
		return cfg.OverlayEnabled, cfg.CrosshairEnabled
	}, func(kind string) {
		switch kind {
		case "overlay":
			c.Update(func(cfg *tune.Config) { cfg.OverlayEnabled = !cfg.OverlayEnabled })
		case "crosshair":
			c.Update(func(cfg *tune.Config) { cfg.CrosshairEnabled = !cfg.CrosshairEnabled })
			s.syncCrosshair()
		case "config":
			_ = openPath(tune.ConfigPath())
		case "about":
			s.switchTo("about")
		}
	}, func() {
		if a.OnSound != nil {
			a.OnSound("tray")
		}
	})
	go func() {
		t := time.NewTicker(3 * time.Minute)
		defer t.Stop()
		for range t.C {
			win32.TrimWorkingSet()
		}
	}()
	err := win32.Run("CODM Tune", a)
	hud.StopChild()
	crosshair.StopChild()
	return err
}

func (s *screens) crosshairDriver() {
	tick := time.NewTicker(600 * time.Millisecond)
	for range tick.C {
		cfg := s.c.Cfg()
		on := cfg.CrosshairEnabled && s.uiReady.Load()
		_ = crosshair.Write(s.settingsFromConfig(cfg), on)
		crosshair.EnsureChild(on)
	}
}

// NewHeadless builds the same app without a window, for cmd/preview.
func NewHeadless() (*ui.App, *tune.Control) {
	c := tune.NewControl()
	a := ui.NewApp(WinW, WinH)
	c.SetNotify(func() { a.Post(func() {}) })
	s := &screens{app: a, c: c}
	s.applyTheme()
	a.Root(s.overview())
	return a, c
}

func colorOf(t tune.Tone) ui.Color {
	switch t {
	case tune.ToneGood:
		return ui.CurrentTheme.Success
	case tune.ToneWarn:
		return ui.CurrentTheme.Tertiary
	case tune.ToneBad:
		return ui.CurrentTheme.Danger
	}
	return ui.CurrentTheme.Muted
}

// header paints the title, subtitle, divider and the live footer footprint.
// Every x is relative to r.X so the whole header slides with its screen.
func (s *screens) header(root *ui.Node, title, sub string) {
	root.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		x := r.X
		c.Text(ui.R(x+PadX, 22, 700, 34), title, 26, ui.CurrentTheme.Text, ui.AlignLeft, true)
		c.Text(ui.R(x+PadX, 62, 760, 20), sub, 13, ui.CurrentTheme.Muted, ui.AlignLeft, false)
		c.Line(x+PadX, 100, x+PadX+Wide, 100, 1, ui.CurrentTheme.Outline)
		c.Line(x+PadX, FootY, x+PadX+Wide, FootY, 1, ui.CurrentTheme.Outline)
		c.Text(ui.R(x+PadX, FootY+8, Wide-360, 20), "↑↓ 移动 · Enter 确认 / 按住确认 · ←→ 调节 · Esc 返回 · 滚轮滚动", 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
		tool := s.c.Tool()
		right := fmt.Sprintf("自身 %.1f%% CPU · %.0f MB · ACE %d", tool.CPU, tool.MemMB, tool.ACECnt)
		c.Text(ui.R(x+PadX+Wide-350, FootY+8, 350, 20), right, 11, ui.CurrentTheme.Muted, ui.AlignRight, false)
	}
}

func (s *screens) newRoot(title, sub string) *ui.Node {
	s.title = title
	s.sub = sub
	root := ui.NewNode(ui.R(0, 0, WinW, WinH))
	s.rail(root)
	return root
}

// card draws a surface with the configured MD3E radius, morphing on press.
func card(c ui.Painter, r ui.Rect, press float32, fill ui.Color) {
	th := ui.CurrentTheme
	rr := r.Inset(press * 2)
	if th.Style == "endfield" {
		// Stock plate: flat paper/board, one ink hairline, one cut corner.
		// No rounding and no shadow — both read as a different product.
		c.FillRect(rr, fill)
		c.Line(rr.X, rr.Y, rr.X+rr.W, rr.Y, 1, th.Outline)
		c.Line(rr.X, rr.Bottom(), rr.X+rr.W, rr.Bottom(), 1, th.Outline)
		c.Line(rr.X, rr.Y, rr.X, rr.Bottom(), 1, th.Outline)
		c.Line(rr.X+rr.W, rr.Y, rr.X+rr.W, rr.Bottom(), 1, th.Outline)
		const cut = 10
		for i := 0; i < cut; i++ {
			c.FillRect(ui.R(rr.X+rr.W-1-float32(i), rr.Y+float32(i), 1, 1), th.Bg)
			c.FillRect(ui.R(rr.X+rr.W-1-float32(i), rr.Y+float32(i)+1, 1, 1), th.Bg)
		}
		c.Line(rr.X+rr.W-cut, rr.Y, rr.X+rr.W-cut, rr.Y+cut, 1, th.Outline)
		c.Line(rr.X+rr.W-cut, rr.Y+cut, rr.X+rr.W, rr.Y+cut, 1, th.Outline)
		if th.Advanced {
			c.FillRect(ui.R(rr.X, rr.Y, rr.W-cut, 3), th.Danger)
		}
		return
	}
	if th.Industrial() && th.CardRadius <= 6 {
		c.FillRect(rr, fill)
		c.Line(rr.X, rr.Y, rr.X+rr.W, rr.Y, 1, th.Outline2)
		c.Line(rr.X, rr.Bottom(), rr.X+rr.W, rr.Bottom(), 1, th.Outline)
		c.Line(rr.X, rr.Y, rr.X, rr.Bottom(), 1, th.Outline2)
		c.FillRect(ui.R(rr.X, rr.Y, 3, rr.H), th.PrimaryDim)
		c.Line(rr.X+rr.W-10, rr.Y, rr.X+rr.W, rr.Y, 2, th.Primary)
		c.Line(rr.X+rr.W-10, rr.Y, rr.X+rr.W, rr.Y+10, 2, th.Primary)
		if th.Advanced {
			c.FillRect(ui.R(rr.X+rr.W-24, rr.Y+8, 14, 3), th.Primary)
			c.FillRect(ui.R(rr.X+rr.W-24, rr.Y+14, 8, 3), th.PrimaryDim)
		}
		// Stepped diagonal cuts at two corners (the reference uses clipped
		// industrial plates rather than rounded cards).
		for i := 0; i < 10; i++ {
			c.FillRect(ui.R(rr.X+float32(i), rr.Y, 1, 10-float32(i)), th.Bg)
			c.FillRect(ui.R(rr.X+rr.W-1-float32(i), rr.Bottom()-(10-float32(i)), 1, 10-float32(i)), th.Bg)
		}
		return
	}
	rad := th.CardRadius - press*6
	if rad < 6 {
		rad = 6
	}
	c.FillRoundRect(rr, rad, fill)
	c.StrokeRoundRect(rr, rad, 1, th.Outline)
}

// chip draws a small status pill with a leading dot.
func chip(c ui.Painter, r ui.Rect, text string, col ui.Color) {
	th := ui.CurrentTheme
	if th.Style == "endfield" {
		// A status tag is a stamped block, never a pill: pill radii belong to
		// the MD3E family and this family has none. Warning is a mark block
		// with ink type (red is reserved for destructive rows); a settled
		// state is a hairline box. Nothing here may be a coloured line.
		ink := ui.RGB(0x16, 0x16, 0x16)
		if col == th.Danger {
			c.FillRect(r, th.Mark)
			c.Text(ui.R(r.X+10, r.Y, r.W-12, r.H), text, 11, ink, ui.AlignLeft, true)
			return
		}
		c.Line(r.X, r.Y, r.Right(), r.Y, 1, th.Outline2)
		c.Line(r.X, r.Bottom(), r.Right(), r.Bottom(), 1, th.Outline2)
		c.Line(r.X, r.Y, r.X, r.Bottom(), 1, th.Outline2)
		c.Line(r.Right(), r.Y, r.Right(), r.Bottom(), 1, th.Outline2)
		c.Text(ui.R(r.X+10, r.Y, r.W-12, r.H), text, 11, th.Text, ui.AlignLeft, true)
		return
	}
	c.FillRoundRect(r, r.H/2, col.WithAlpha(30))
	c.Circle(r.X+r.H/2, r.Y+r.H/2, 3, col)
	c.Text(ui.R(r.X+r.H/2+10, r.Y, r.W-r.H/2-14, r.H), text, 12, col, ui.AlignLeft, true)
}

type btnStyle int

const (
	btnPrimary btnStyle = iota
	btnGhost
	btnDanger
)

func (s *screens) button(r ui.Rect, label string, style btnStyle, onClick func()) *ui.Node {
	n := ui.NewNode(r)
	n.Focusable = true
	n.SetCursorRadius(r.H / 2)
	n.Draw = func(n *ui.Node, c ui.Painter, rr ui.Rect) {
		p := n.Press.Get()
		rr = rr.Inset(p * 2)
		th := ui.CurrentTheme
		if th.Style == "endfield" {
			// Cut-corner action plate: the shape itself says "pressable".
			const cut = 8
			c.FillRect(rr, th.Surface2)
			switch style {
			case btnPrimary:
				c.FillRect(ui.R(rr.X, rr.Y, rr.W-cut, rr.H), th.Mark)
				c.Text(ui.R(rr.X, rr.Y, rr.W-cut-6, rr.H), label, 14, th.OnPrimary, ui.AlignCenter, true)
			case btnDanger:
				c.FillRect(rr, th.DangerBg)
				c.Text(ui.R(rr.X, rr.Y, rr.W-cut, rr.H), label, 13, th.Danger, ui.AlignCenter, true)
			default:
				c.Text(ui.R(rr.X, rr.Y, rr.W-cut, rr.H), label, 13, th.Text, ui.AlignCenter, false)
			}
			for i := 0; i < cut; i++ {
				c.FillRect(ui.R(rr.X+rr.W-1-float32(i), rr.Y+float32(i), 1, 1), th.Bg)
				c.FillRect(ui.R(rr.X+rr.W-1-float32(i), rr.Bottom()-1-float32(i), 1, 1), th.Bg)
			}
			c.Line(rr.X, rr.Y, rr.X+rr.W-cut, rr.Y, 1, th.Outline)
			c.Line(rr.X, rr.Bottom(), rr.X+rr.W-cut, rr.Bottom(), 1, th.Outline)
			c.Line(rr.X, rr.Y, rr.X, rr.Bottom(), 1, th.Outline)
			return
		}
		if th.Industrial() {
			switch style {
			case btnPrimary:
				c.FillRect(rr, th.Primary)
				c.Text(rr, label, 14, th.OnPrimary, ui.AlignCenter, true)
			case btnDanger:
				c.FillRect(rr, th.DangerBg)
				c.Line(rr.X, rr.Y, rr.X+rr.W, rr.Y, 1, th.Danger)
				c.Line(rr.X, rr.Bottom(), rr.X+rr.W, rr.Bottom(), 1, th.Danger)
				c.Text(rr, label, 13, th.Danger, ui.AlignCenter, true)
			default:
				c.FillRect(rr, th.Surface2)
				c.FillRect(ui.R(rr.X, rr.Y, 3, rr.H), th.PrimaryDim)
				c.Text(rr, label, 13, th.Text, ui.AlignCenter, false)
			}
			return
		}
		rad := rr.H / 2
		switch style {
		case btnPrimary:
			c.FillRoundRect(rr, rad, th.Primary)
			c.Text(rr, label, 15, th.OnPrimary, ui.AlignCenter, true)
		case btnDanger:
			c.FillRoundRect(rr, rad, th.DangerBg)
			c.StrokeRoundRect(rr, rad, 1, th.Danger)
			c.Text(rr, label, 14, th.Danger, ui.AlignCenter, true)
		default:
			c.FillRoundRect(rr, rad, th.Surface2)
			c.StrokeRoundRect(rr, rad, 1, th.Outline)
			c.Text(rr, label, 14, th.Text, ui.AlignCenter, false)
		}
	}
	if onClick != nil {
		n.Activate = func(*ui.Node) { onClick() }
	}
	return n
}

// holdButton fires once when the hold fills; releasing early cancels it.
func (s *screens) holdButton(r ui.Rect, label string, hold float32, disabled func() bool, onFire func()) *ui.Node {
	n := ui.NewNode(r)
	n.Focusable = true
	n.SetHold(hold)
	n.SetCursorRadius(r.H / 2)
	n.Update = func(n *ui.Node, dt float32) {
		if disabled != nil {
			n.Disabled = disabled()
		}
	}
	n.Draw = func(n *ui.Node, c ui.Painter, rr ui.Rect) {
		off := n.Disabled
		p := n.Press.Get()
		rr = rr.Inset(p * 2)
		rad := rr.H / 2
		fill := ui.CurrentTheme.Primary
		fg := ui.CurrentTheme.OnPrimary
		if off {
			fill = ui.CurrentTheme.Surface2
			fg = ui.CurrentTheme.Muted
		} else if btnDangerFill(label) {
			fill = ui.CurrentTheme.DangerBg
			fg = ui.CurrentTheme.Danger
		}
		c.FillRoundRect(rr, rad, fill)
		if !off && n.Progress.Get() > 0 {
			w := rr.W * n.Progress.Get()
			c.FillRoundRect(ui.R(rr.X, rr.Y, w, rr.H), rad, fg.WithAlpha(34))
		}
		if off {
			c.StrokeRoundRect(rr, rad, 1, ui.CurrentTheme.Outline)
		}
		c.Text(rr, label, 14, fg, ui.AlignCenter, true)
	}
	n.Activate = func(*ui.Node) {
		if disabled == nil || !disabled() {
			onFire()
		}
	}
	return n
}

func btnDangerFill(label string) bool { return false }

// toggle draws an MD3 switch whose thumb is a spring, never a tween.
type toggleVis struct {
	knob spring.Value
	get  func() bool
}

func (s *screens) toggle(r ui.Rect, get func() bool) (*ui.Node, *toggleVis) {
	v := &toggleVis{get: get}
	if get() {
		v.knob.Jump(1)
	}
	n := ui.NewNode(r)
	n.Draw = func(_ *ui.Node, c ui.Painter, rr ui.Rect) {
		on := v.knob.Get()
		th := ui.CurrentTheme
		if th.Style == "endfield" {
			// A switch here is a stamped checkbox, not a slider: filled acid
			// block for on, hairline box for off. The state has to read from
			// across the room, and a moving knob does not.
			box := ui.R(rr.X+2, rr.Y+(rr.H-22)/2, 22, 22)
			if on > 0.5 {
				c.FillRect(box, th.Mark)
				c.Line(box.X+5, box.Y+11, box.X+9, box.Y+16, 2, ui.RGB(0x16, 0x16, 0x16))
				c.Line(box.X+9, box.Y+16, box.X+17, box.Y+5, 2, ui.RGB(0x16, 0x16, 0x16))
				c.Text(ui.R(box.Right()+8, rr.Y, 40, rr.H), "ON", 10, th.Text, ui.AlignLeft, true)
			} else {
				c.Line(box.X, box.Y, box.Right(), box.Y, 1, th.Outline2)
				c.Line(box.X, box.Bottom(), box.Right(), box.Bottom(), 1, th.Outline2)
				c.Line(box.X, box.Y, box.X, box.Bottom(), 1, th.Outline2)
				c.Line(box.Right(), box.Y, box.Right(), box.Bottom(), 1, th.Outline2)
				c.Text(ui.R(box.Right()+8, rr.Y, 40, rr.H), "OFF", 10, th.Muted, ui.AlignLeft, false)
			}
			return
		}
		if th.Industrial() {
			track := ui.R(rr.X, rr.Y+(rr.H-22)/2, 52, 22)
			c.FillRect(track, th.Surface3)
			c.Line(track.X, track.Y, track.X+track.W, track.Y, 1, th.Outline2)
			c.Line(track.X, track.Bottom(), track.X+track.W, track.Bottom(), 1, th.Outline2)
			x := track.X + 2 + on*(track.W-22)
			c.FillRect(ui.R(x, track.Y+3, 18, 16), ui.Mix(th.Muted, th.Primary, on))
			return
		}
		track := ui.R(rr.X, rr.Y+(rr.H-30)/2, 52, 30)
		c.FillRoundRect(track, 15, ui.Mix(th.Surface3, th.Primary, on))
		x := track.X + 15 + on*22
		c.Circle(x, track.Y+15, 11, ui.Mix(th.Muted, th.OnPrimary, on))
	}
	n.Update = func(*ui.Node, float32) {
		t := float32(0)
		if v.get() {
			t = 1
		}
		v.knob.Set(t)
	}
	return n, v
}

// statusText renders a tone-coloured dot + label.
func statusText(c ui.Painter, r ui.Rect, text string, t tune.Tone, bold bool) {
	col := colorOf(t)
	c.Circle(r.X+4, r.Y+r.H/2, 3.5, col)
	c.Text(ui.R(r.X+14, r.Y, r.W-14, r.H), text, 13, col, ui.AlignLeft, bold)
}

// moduleRow is a focusable card with a title, live status, detail text and a
// switch. When hold > 0 the switch direction that enables the module requires
// a hold; disabling is always immediate.
func (s *screens) moduleRow(r ui.Rect, title string, getOn func() bool, status func() (string, tune.Tone), detail func() string, hold float32, onToggle func(bool), extra *ui.Node) *ui.Node {
	n := ui.NewNode(r)
	n.Focusable = true
	n.SetCursorRadius(20)
	if hold > 0 {
		n.SetHold(hold)
		n.NeedsHold = func(*ui.Node) bool { return !getOn() }
	}
	row, _ := s.toggle(ui.R(r.Right()-82, r.Y+18, 52, 30), getOn)
	n.Add(row)
	if extra != nil {
		n.Add(extra)
	}
	n.Draw = func(n *ui.Node, c ui.Painter, rr ui.Rect) {
		card(c, rr, n.Press.Get(), ui.CurrentTheme.Surface)
		c.Text(ui.R(rr.X+22, rr.Y+16, rr.W-160, 26), title, 16, ui.CurrentTheme.Text, ui.AlignLeft, true)
		txt, tone := status()
		statusText(c, ui.R(rr.X+22, rr.Y+44, rr.W-150, 20), txt, tone, false)
		if d := detail(); d != "" {
			c.TextBlock(ui.R(rr.X+22, rr.Y+68, rr.W-236, rr.H-78), d, 12, ui.CurrentTheme.Muted, ui.AlignLeft, false)
		}
		if hold > 0 && (n.Holding || n.HoldFired) {
			w := (rr.W - 44) * n.Progress.Get()
			c.FillRoundRect(ui.R(rr.X+22, rr.Bottom()-6, w, 3), 1.5, ui.CurrentTheme.Primary)
		}
	}
	n.Activate = func(*ui.Node) { onToggle(!getOn()) }
	return n
}

// miniButton is a compact pill used inside module rows.
func (s *screens) miniButton(r ui.Rect, label string, onClick func()) *ui.Node {
	return s.button(r, label, btnGhost, onClick)
}

// startScan runs a diagnostic scan and keeps the page animated while the
// worker is busy. Results are rebuilt in place so adapters/DNS rows appear
// without asking the user to re-enter the page.
func (s *screens) startScan(kind string) {
	if s.scanKind != "" {
		return
	}
	start := time.Now()
	s.scanKind = kind
	s.scanStart = start
	go func() {
		if kind == "gpu" {
			s.c.RefreshGPU()
		} else {
			s.c.RefreshNet()
		}
	}()
	go func() {
		for i := 0; i < 300; i++ {
			win32.Wake()
			time.Sleep(30 * time.Millisecond)
		}
	}()
	go func() {
		for time.Since(start) < 900*time.Millisecond || s.scanBusy(kind) {
			if time.Since(start) > 8*time.Second {
				break
			}
			time.Sleep(45 * time.Millisecond)
		}
		s.app.Post(func() {
			s.scanKind = ""
			if s.active == "diagnostics" {
				s.switchTo("diagnostics")
			}
		})
	}()
}

func (s *screens) scanBusy(kind string) bool {
	if kind == "gpu" {
		return s.c.GPU().Busy
	}
	return s.c.Net().Busy
}

func (s *screens) scanLabel(kind, normal string) string {
	if s.scanKind != kind {
		return normal
	}
	frames := []string{"▘", "▝", "▗", "▖"}
	f := frames[int(time.Since(s.scanStart).Milliseconds()/110)%len(frames)]
	p := int(time.Since(s.scanStart).Seconds() / 1.2 * 100)
	if p > 96 {
		p = 96
	}
	return f + " 扫描中 " + strconv.Itoa(p) + "%"
}
