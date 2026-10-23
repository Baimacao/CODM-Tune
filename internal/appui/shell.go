package appui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

type navItem struct {
	id    string
	label string
	icon  string
}

var navItems = []navItem{
	{"overview", "概览", "gauge"},
	{"game", "游戏与启动", "gamepad"},
	{"performance", "性能与调度", "bolt"},
	{"system", "系统调优", "shield"},
	{"visual", "画面与 FOV", "monitor"},
	{"input", "输入与鼠标", "mouse"},
	{"overlay", "悬浮窗", "window"},
	{"crosshair", "辅助准心", "scope"},
	{"replay", "回放管理", "disc"},
	{"diagnostics", "诊断", "wifi"},
	{"knowledge", "知识库", "book"},
	{"settings", "设置", "gear"},
	{"about", "关于", "info"},
}

// rail paints the category rail and adds its focusable items.
func (s *screens) rail(root *ui.Node) {
	th := ui.CurrentTheme
	root.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		if th.Style == "ark" {
			c.FillRect(ui.R(0, 0, RailW, WinH), th.Bg)
			c.Line(RailW, 0, RailW, WinH, 1, th.Outline2)
			if th.Dark {
				c.FillRect(ui.R(10, 10, 42, 42), th.PrimaryDim)
				c.Line(10, 10, 52, 10, 2, th.Text)
			} else {
				c.FillRect(ui.R(10, 10, 42, 42), th.Primary)
				c.Line(10, 10, 52, 10, 2, th.Text)
			}
		} else {
			c.FillRect(ui.R(0, 0, RailW, WinH), th.Surface)
			c.Line(RailW, 0, RailW, WinH, 1, th.Outline)
			if th.Dark {
				c.FillRoundRect(ui.R(10, 10, 42, 42), 12, th.PrimaryBg)
			} else {
				c.FillRoundRect(ui.R(10, 10, 42, 42), 12, th.Primary)
			}
		}
		if p := logoMark(); p != "" {
			c.DrawImage(p, ui.R(13, 13, 36, 36), 255)
		} else {
			drawLogo(c, ui.R(16, 18, 30, 26), th.Primary)
		}
		name := "CODM TUNE"
		sub := "MD3E · v10"
		if th.Style == "ark" {
			name = "CODM//TUNE"
			sub = fmt.Sprintf("ARMORY SYSTEM / %02d", len(navItems))
		}
		c.Text(ui.R(60, 18, 132, 22), name, 15, th.Text, ui.AlignLeft, true)
		c.Text(ui.R(60, 40, 132, 18), sub, 10, th.Muted, ui.AlignLeft, false)
		s.headerDraw(c, r)
		tool := s.c.Tool()
		c.Text(ui.R(16, WinH-42, RailW-32, 18), "内存 "+fmtMB(tool.MemMB)+" · CPU "+fmtPct(tool.CPU), 10, th.Muted, ui.AlignLeft, false)
		if s.c.Cfg().Advanced {
			c.Text(ui.R(16, WinH-24, RailW-32, 16), "高级模式 · 危险项可见", 10, th.Danger, ui.AlignLeft, true)
		}
	}
	for i, it := range navItems {
		it := it
		n := ui.NewNode(ui.R(12, 88+float32(i)*42, RailW-24, 38))
		n.Focusable = true
		n.SetCursorRadius(12)
		n.Data = "nav:" + it.id
		n.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
			active := s.active == it.id
			p := n.Press.Get()
			rr := r.Inset(p * 2)
			col := th.Muted
			if active {
				col = th.Primary
			}
			if th.Style == "ark" {
				if active {
					c.FillRect(rr, th.Surface2)
					c.FillRect(ui.R(rr.X, rr.Y, 3, rr.H), th.Primary)
				}
				c.Text(ui.R(rr.X+10, rr.Y+12, 24, 16), fmt.Sprintf("%02d", i+1), 9, th.PrimaryDim, ui.AlignLeft, false)
				drawIcon(c, ui.R(rr.X+36, rr.Y+12, 16, 16), it.icon, col)
				c.Text(ui.R(rr.X+60, rr.Y, rr.W-66, rr.H), it.label, 12, col, ui.AlignLeft, active)
				c.Line(rr.X+60, rr.Bottom()-1, rr.X+rr.W, rr.Bottom()-1, 1, th.Outline)
				return
			}
			if active {
				c.FillRoundRect(rr, 12, th.PrimaryBg)
			}
			drawIcon(c, ui.R(rr.X+12, rr.Y+11, 18, 18), it.icon, col)
			c.Text(ui.R(rr.X+40, rr.Y, rr.W-48, rr.H), it.label, 13, col, ui.AlignLeft, active)
		}
		n.Activate = func(*ui.Node) { s.switchTo(it.id) }
		root.Add(n)
	}
}

// headerDraw is the content-area header, shared by every category.
func (s *screens) headerDraw(c ui.Painter, r ui.Rect) {
	th := ui.CurrentTheme
	title, sub := s.title, s.sub
	if th.Style == "ark" {
		title = "// " + title
		sub = "> " + sub
		c.FillRect(ui.R(PadX, 20, 4, 34), th.Primary)
	}
	c.Text(ui.R(PadX+8, 22, 700, 34), title, 26, th.Text, ui.AlignLeft, true)
	c.Text(ui.R(PadX+8, 62, 760, 20), sub, 13, th.Muted, ui.AlignLeft, false)
	if th.Advanced {
		// Hazard band: the advanced state has to be visible without opening a
		// row, so the divider itself becomes a warning stripe.
		for x := float32(PadX); x < PadX+Wide; x += 12 {
			c.FillRect(ui.R(x, 96, 6, 4), th.Primary)
		}
	}
	c.Line(PadX, 100, PadX+Wide, 100, 1, th.Outline)
	c.Line(PadX, FootY, PadX+Wide, FootY, 1, th.Outline)
	if th.Style == "ark" {
		// Large section numeral: the reference UI leans on one oversized
		// circular index instead of decorative chips.
		cx, cy := float32(PadX+Wide-60), float32(58)
		for ix := -2; ix <= 2; ix++ {
			for iy := -2; iy <= 2; iy++ {
				c.Circle(cx+float32(ix)*7, cy+float32(iy)*7, 1.2, th.Tertiary.WithAlpha(95))
			}
		}
		if th.Advanced {
			c.Circle(cx, cy, 34, th.Primary)
			c.StrokeCircle(cx, cy, 34, 3, th.Text.WithAlpha(210))
			c.Text(ui.R(cx-34, cy-19, 68, 34), fmt.Sprintf("%02d", navNumber(s.active)), 28, th.OnPrimary, ui.AlignCenter, true)
			c.Text(ui.R(cx-40, cy+38, 80, 10), "ADVANCED", 6, th.Primary, ui.AlignCenter, true)
		} else {
			c.Circle(cx, cy, 34, th.Bg)
			c.StrokeCircle(cx, cy, 34, 2, th.Primary)
			c.Text(ui.R(cx-34, cy-19, 68, 34), fmt.Sprintf("%02d", navNumber(s.active)), 28, th.Text, ui.AlignCenter, true)
			c.Text(ui.R(cx-40, cy+17, 80, 12), "SECTION", 7, th.PrimaryDim, ui.AlignCenter, true)
		}
		c.Line(PadX+Wide-100, 100, PadX+Wide-100, 112, 2, th.Tertiary)
		c.Line(PadX, FootY-1, PadX+Wide, FootY-1, 1, th.Outline2)
	}
	c.Text(ui.R(PadX, FootY+8, Wide-360, 20), "↑↓ 移动 · Enter 确认 / 按住 · ←→ 调节 · Esc 返回 · 滚轮滚动", 11, th.Muted, ui.AlignLeft, false)
	tool := s.c.Tool()
	right := "自身 " + fmtPct(tool.CPU) + " CPU · " + fmtMB(tool.MemMB) + " · ACE " + itoa(tool.ACECnt)
	c.Text(ui.R(PadX+Wide-350, FootY+8, 350, 20), right, 11, th.Muted, ui.AlignRight, false)
}

func navNumber(id string) int {
	for i, it := range navItems {
		if it.id == id {
			return i + 1
		}
	}
	return 0
}

func (s *screens) switchTo(id string) {
	s.active = id
	n := s.build(id)
	s.app.Switch(n)
	for _, f := range ui.Focusables(n) {
		if d, ok := f.Data.(string); ok && strings.HasPrefix(d, "nav:") {
			continue
		}
		s.app.SetFocus(f)
		break
	}
}

// switchToSolo is the F11 path: the window just changed physical size, so the
// previous screen must not be scaled across the new surface.
func (s *screens) switchToSolo(id string) {
	s.active = id
	n := s.build(id)
	s.app.SwitchSolo(n)
	for _, f := range ui.Focusables(n) {
		if d, ok := f.Data.(string); ok && strings.HasPrefix(d, "nav:") {
			continue
		}
		s.app.SetFocus(f)
		break
	}
}

// cat prepares the shared category header state and returns the root.
func (s *screens) cat(id, title, sub string) *ui.Node {
	s.active = id
	s.title = title
	s.sub = sub
	root := ui.NewNode(ui.R(0, 0, WinW, WinH))
	s.rail(root)
	return root
}

// listIn makes the scrolling content column used by most categories.
func (s *screens) listIn(root *ui.Node) *ui.List {
	l := ui.NewList(ui.R(PadX, 112, Wide, FootY-112-8), 0, 10)
	root.Add(l.View)
	return l
}

func (s *screens) build(id string) *ui.Node {
	switch id {
	case "game":
		return s.gameCat()
	case "performance":
		return s.perfCat()
	case "system":
		return s.systemCat()
	case "visual":
		return s.visualCat()
	case "input":
		return s.inputCat()
	case "overlay":
		return s.overlayCat()
	case "crosshair":
		return s.crosshairCat()
	case "replay":
		return s.replayCat()
	case "diagnostics":
		return s.diagCat()
	case "knowledge":
		return s.knowledgeCat()
	case "settings":
		return s.settingsCat()
	case "about":
		return s.aboutCat()
	}
	return s.overview()
}

// feature is one row: icon, bold name, thin description, trailing control.
type feature struct {
	icon     string
	title    string
	desc     string
	value    func() string
	tone     func() tune.Tone
	toggle   func() bool
	onToggle func(bool)
	open     func()
	hold     float32
	hint     string
	disabled func() bool
	adjust   func(int)
	slider   func(float64)
	info     bool
	progress func() float64
}

// featureRow builds a focusable card and its trailing toggle.
func (s *screens) featureRow(r ui.Rect, f feature) *ui.Node {
	n := ui.NewNode(r)
	n.Focusable = !f.info
	n.SetCursorRadius(18)
	th := ui.CurrentTheme
	if f.hold > 0 {
		n.SetHold(f.hold)
		if f.onToggle != nil {
			on := f.toggle
			n.NeedsHold = func(*ui.Node) bool { return on != nil && !on() }
		}
	}
	if f.toggle != nil {
		tv, _ := s.toggle(ui.R(r.Right()-72, r.Y+20, 52, 30), f.toggle)
		n.Add(tv)
	}
	n.Draw = func(n *ui.Node, c ui.Painter, rr ui.Rect) {
		card(c, rr, n.Press.Get(), th.Surface)
		if f.disabled != nil && f.disabled() {
			c.FillRoundRect(rr.Inset(1), th.CardRadius-1, th.Bg.WithAlpha(120))
		}
		iconCol := th.Primary
		if f.tone != nil {
			iconCol = colorOf(f.tone())
		}
		c.FillRoundRect(ui.R(rr.X+16, rr.Y+16, 38, 38), 12, iconCol.WithAlpha(26))
		drawIcon(c, ui.R(rr.X+23, rr.Y+23, 24, 24), f.icon, iconCol)
		title := f.title
		if th.Style == "ark" {
			title = "▌ " + title
		}
		c.Text(ui.R(rr.X+66, rr.Y+12, rr.W-150, 24), title, 15, th.Text, ui.AlignLeft, true)
		if f.value != nil {
			vc := th.Muted
			if f.tone != nil {
				vc = colorOf(f.tone())
			}
			c.Text(ui.R(rr.X+66, rr.Y+36, rr.W-280, 18), f.value(), 12, vc, ui.AlignLeft, false)
		}
		if f.desc != "" {
			c.TextBlock(ui.R(rr.X+66, rr.Y+56, rr.W-150, rr.H-62), f.desc, 12, th.Muted, ui.AlignLeft, false)
		}
		hint := f.hint
		if hint == "" {
			switch {
			case f.hold > 0:
				hint = "按住 Enter"
			case f.toggle != nil:
				hint = "Enter 切换"
			case f.open != nil:
				hint = "Enter 打开 →"
			case f.adjust != nil:
				hint = "←→ 调节"
			}
		}
		if hint != "" {
			c.Text(ui.R(rr.Right()-190, rr.Y+56, 170, 18), hint, 10, th.PrimaryDim, ui.AlignRight, false)
		}
		if f.hold > 0 && (n.Holding || n.HoldFired) {
			w := (rr.W - 40) * n.Progress.Get()
			c.FillRoundRect(ui.R(rr.X+20, rr.Bottom()-7, w, 3), 1.5, th.Primary)
		}
		if f.adjust != nil && f.progress != nil {
			frac := f.progress()
			if frac < 0 {
				frac = 0
			}
			if frac > 1 {
				frac = 1
			}
			tx := rr.X + 20
			tw := rr.W - 40
			ty := rr.Bottom() - 10
			c.FillRoundRect(ui.R(tx, ty, tw, 4), 2, th.Outline)
			c.FillRoundRect(ui.R(tx, ty, tw*float32(frac), 4), 2, th.Primary)
			c.Circle(tx+tw*float32(frac), ty+2, 6, th.Primary)
		}
	}
	switch {
	case f.adjust != nil:
		n.OnKey = func(n *ui.Node, k ui.Key, ph ui.Phase) bool {
			if ph == ui.Release {
				return k == ui.KeyLeft || k == ui.KeyRight
			}
			if ph != ui.Press && ph != ui.Repeat {
				return false
			}
			switch k {
			case ui.KeyLeft:
				f.adjust(-1)
				return true
			case ui.KeyRight:
				f.adjust(1)
				return true
			}
			return false
		}
	case f.open != nil:
		n.Activate = func(*ui.Node) {
			if f.disabled == nil || !f.disabled() {
				f.open()
			}
		}
	case f.onToggle != nil:
		n.Activate = func(*ui.Node) {
			if f.disabled == nil || !f.disabled() {
				f.onToggle(!f.toggle())
			}
		}
	}
	if f.slider != nil {
		n.OnMouse = func(nn *ui.Node, x, y float32, action ui.MouseAction) {
			if action != ui.MouseDown && action != ui.MouseMove {
				return
			}
			x0, x1 := nn.R.X+20, nn.R.Right()-20
			if x1 <= x0 {
				return
			}
			frac := (x - x0) / (x1 - x0)
			if frac < 0 {
				frac = 0
			}
			if frac > 1 {
				frac = 1
			}
			f.slider(float64(frac))
		}
	}
	return n
}

func fmtPct(v float64) string { return ftoa1(v) + "%" }
func fmtMB(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		v = 0
	}
	return itoa(int(v+0.5)) + " MB"
}

func itoa(v int) string { return strconv.Itoa(v) }

func ftoa1(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "0.0"
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}
