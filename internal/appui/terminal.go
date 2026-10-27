package appui

import (
	"fmt"
	"time"

	"codmtune/internal/ui"
	"codmtune/internal/win32"
)

// terminal is the landscape/F11 presentation: a dedicated field terminal
// instead of the normal sidebar UI scaled up. It deliberately has no
// focusable controls; F11 is the only input, so the fullscreen coordinate
// mapping cannot select the wrong row.
func (s *screens) toggleFullscreen() bool {
	on := win32.ToggleFullscreen()
	if on {
		win32.SetFullscreenFill(true)
		s.termStart = time.Now()
		if s.active != "" && s.active != "terminal" {
			s.fsReturn = s.active
		}
		s.app.SwitchSolo(s.terminal())
	} else {
		win32.SetFullscreenFill(false)
		s.termStart = time.Time{}
		target := s.fsReturn
		if target == "" {
			target = "overview"
		}
		s.switchToSolo(target)
	}
	return on
}

func hudState(s *screens) string {
	if s.c.Cfg().OverlayEnabled {
		return "ACTIVE"
	}
	return "OFF"
}

const termW = 1246

func (s *screens) terminal() *ui.Node {
	root := ui.NewNode(ui.R(0, 0, termW, WinH))
	root.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) { s.drawTerminal(c) }
	root.Update = func(n *ui.Node, dt float32) {
		if !s.termStart.IsZero() && time.Since(s.termStart) < 900*time.Millisecond {
			ui.Invalidate()
		}
	}
	return root
}

func (s *screens) drawTerminal(c ui.Painter) {
	// The terminal borrows the active palette. On the paper and board skins a
	// neon-green console would read as a different product, so those two keep
	// their own ink and use the accent only as a fill, exactly like the rest
	// of the style.
	th := ui.CurrentTheme
	bg := ui.RGB(0x05, 0x08, 0x07)
	grid := ui.RGBA(0x3D, 0x8B, 0x72, 0x22)
	dim := ui.RGB(0x4C, 0x6B, 0x5E)
	text := ui.RGB(0xD9, 0xE8, 0xDF)
	green := ui.RGB(0x8B, 0xFF, 0xD9)
	amber := ui.RGB(0xF2, 0xC9, 0x4C)
	if th.Style == "endfield" {
		bg = th.Bg
		grid = th.Outline.WithAlpha(0x30)
		dim = th.Muted
		text = th.Text
		green = th.Mark
		amber = th.Text
	} else if th.Style == "ark" {
		bg = th.Bg
		grid = th.Outline.WithAlpha(0x40)
		dim = th.Muted
		text = th.Text
		green = th.Primary
		amber = th.Warn
	}

	c.FillRect(ui.R(0, 0, termW, WinH), bg)
	reveal := float32(1)
	if !s.termStart.IsZero() {
		reveal = float32(time.Since(s.termStart).Seconds() / 0.72)
		if reveal < 0 {
			reveal = 0
		}
		if reveal > 1 {
			reveal = 1
		}
		reveal = reveal * reveal * (3 - 2*reveal)
	}
	if reveal < 1 {
		c.PushClip(ui.R(0, 0, termW*reveal, WinH))
	}
	for x := 0; x < termW; x += 44 {
		c.Line(float32(x), 0, float32(x), WinH, 1, grid)
	}
	for y := 0; y < WinH; y += 44 {
		c.Line(0, float32(y), termW, float32(y), 1, grid)
	}
	// Framing ticks.
	corner := func(x, y, sx, sy float32) {
		c.Line(x, y, x+sx, y, 2, green)
		c.Line(x, y, x, y+sy, 2, green)
	}
	corner(24, 24, 46, 46)
	corner(termW-24, 24, -46, 46)
	corner(24, WinH-24, 46, -46)
	corner(termW-24, WinH-24, -46, -46)

	c.Text(ui.R(48, 34, 620, 18), "CODMTUNE // FIELD TERMINAL", 11, dim, ui.AlignLeft, true)
	c.Text(ui.R(48, 56, 540, 62), "CODMTUNE", 44, text, ui.AlignLeft, true)
	c.Text(ui.R(50, 116, 540, 18), "C O D M   F I E L D   T E R M I N A L", 10, green, ui.AlignLeft, false)

	now := time.Now()
	c.Text(ui.R(termW-360, 42, 312, 26), now.Format("2006-01-02  15:04:05"), 16, amber, ui.AlignRight, false)
	c.Text(ui.R(termW-360, 70, 312, 18), "BUILD v12.2 · LANDSCAPE", 10, dim, ui.AlignRight, false)

	game, _, _, running, _ := s.c.Game()
	state := "STANDBY"
	stateCol := dim
	if running {
		state = "OPERATIONAL"
		stateCol = green
	}
	s.termPanel(c, 48, 170, 300, 250, "01 / SESSION", green)
	c.Text(ui.R(68, 200, 260, 26), state, 20, stateCol, ui.AlignLeft, true)
	c.Text(ui.R(68, 232, 260, 18), "CODM PROCESS", 10, dim, ui.AlignLeft, false)
	if game == "" {
		game = "NOT DETECTED"
	}
	c.TextBlock(ui.R(68, 254, 258, 38), game, 10, text, ui.AlignLeft, false)

	cfg := s.c.Cfg()
	c.Text(ui.R(68, 302, 258, 16), "VIEW FOV", 9, dim, ui.AlignLeft, false)
	c.Text(ui.R(210, 302, 110, 16), fmt.Sprintf("%03d°", cfg.FOV), 10, amber, ui.AlignRight, true)
	c.Text(ui.R(68, 322, 258, 16), "WEAPON FOV", 9, dim, ui.AlignLeft, false)
	c.Text(ui.R(210, 322, 110, 16), fmt.Sprintf("0x%02X", cfg.FOVByte), 10, amber, ui.AlignRight, true)
	c.Text(ui.R(68, 346, 258, 16), "POWER", 9, dim, ui.AlignLeft, false)
	c.Text(ui.R(150, 346, 170, 16), s.c.Power().Name, 10, text, ui.AlignRight, false)
	c.Text(ui.R(68, 370, 258, 16), "ACE", 9, dim, ui.AlignLeft, false)
	c.Text(ui.R(150, 370, 170, 16), s.c.ACE().Text, 10, text, ui.AlignRight, false)

	st := s.c.Tool()
	s.termPanel(c, 372, 170, 300, 250, "02 / HOST LOAD", green)
	s.termBar(c, 392, 206, 260, "CPU", st.CPU/100, fmt.Sprintf("%.1f%%", st.CPU), green)
	s.termBar(c, 392, 256, 260, "MEM", st.MemMB/512, fmt.Sprintf("%.0f MB", st.MemMB), green)
	s.termBar(c, 392, 306, 260, "ACE", float64(st.ACECnt)/8, fmt.Sprintf("%d PROC", st.ACECnt), amber)
	c.Text(ui.R(392, 356, 260, 18), "SCHED  "+s.c.Sched().Text, 10, dim, ui.AlignLeft, false)
	c.Text(ui.R(392, 378, 260, 18), "GPU     "+s.c.GPU().Actual, 10, dim, ui.AlignLeft, false)

	s.termPanel(c, 696, 170, 502, 250, "03 / LINK", green)
	c.Text(ui.R(716, 204, 316, 16), "TELEMETRY", 9, dim, ui.AlignLeft, false)
	c.Text(ui.R(716, 226, 316, 30), "RTT / SERVER / LOSS", 16, text, ui.AlignLeft, false)
	c.Text(ui.R(716, 264, 316, 96), "FPS 采集器：N/A\nPresentMon ETW: BLOCKED\nRTT: 由 HUD 子进程采集\nSERVER: 由 HUD 子进程显示", 11, dim, ui.AlignLeft, false)
	c.Text(ui.R(716, 372, 316, 18), "OVERLAY CHILD  "+hudState(s), 10, green, ui.AlignLeft, false)

	logs := s.c.Logs()
	if len(logs) > 5 {
		logs = logs[len(logs)-5:]
	}
	s.termPanel(c, 48, 446, 640, 172, "04 / EVENT LOG", green)
	y := 476
	for i := len(logs) - 1; i >= 0; i-- {
		line := logs[i]
		if len(line) > 100 {
			line = line[:100]
		}
		c.Text(ui.R(68, float32(y), 600, 18), "> "+line, 10, text, ui.AlignLeft, false)
		y += 24
	}
	if len(logs) == 0 {
		c.Text(ui.R(68, 476, 600, 18), "> no events", 10, dim, ui.AlignLeft, false)
	}

	s.termPanel(c, 712, 446, 486, 172, "05 / CONTROLS", green)
	c.Text(ui.R(732, 478, 300, 18), "[F11]  RETURN TO CONSOLE", 12, amber, ui.AlignLeft, true)
	c.Text(ui.R(732, 506, 300, 18), "[ESC]  no-op in terminal", 10, dim, ui.AlignLeft, false)
	c.Text(ui.R(732, 528, 300, 18), "[CLICK] disabled", 10, dim, ui.AlignLeft, false)
	c.Text(ui.R(732, 550, 300, 18), "input is F11 only", 10, dim, ui.AlignLeft, false)

	c.Text(ui.R(48, 646, 1000, 18), "NETWORK / POWER / FOV ARE READ-ONLY IN TERMINAL · F11 RETURNS TO THE SPRING CONSOLE", 9, dim, ui.AlignLeft, false)
	c.Text(ui.R(48, 666, 1000, 18), "© CODMTUNE  ·  NO PROCESS MEMORY ACCESS  ·  NO ACE SUSPEND", 9, dim, ui.AlignLeft, false)
	if reveal < 1 {
		c.PopClip()
		c.Line(termW*reveal, 0, termW*reveal, WinH, 2, green)
		c.Line(termW*reveal-8, 0, termW*reveal-8, WinH, 1, ui.CurrentTheme.Outline)
	}
}

func (s *screens) termPanel(c ui.Painter, x, y, w, h float32, title string, col ui.Color) {
	r := ui.R(x, y, w, h)
	c.FillRect(r, ui.CurrentTheme.Surface.WithAlpha(0xDD))
	c.Line(x, y, x+w, y, 1, ui.CurrentTheme.Outline)
	c.Text(ui.R(x+16, y+10, w-32, 16), "["+title+"]", 9, col, ui.AlignLeft, true)
}

func (s *screens) termBar(c ui.Painter, x, y, w float32, label string, frac float64, value string, col ui.Color) {
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	c.Text(ui.R(x, y, 48, 16), label, 9, ui.CurrentTheme.Muted, ui.AlignLeft, false)
	c.Text(ui.R(x+180, y, 80, 16), value, 10, col, ui.AlignRight, true)
	segs := 22
	for i := 0; i < segs; i++ {
		sx := x + float32(i)*10
		cc := ui.CurrentTheme.Outline.WithAlpha(0x55)
		if float64(i+1)/float64(segs) <= frac {
			cc = col
		}
		c.FillRect(ui.R(sx, y+20, 7, 10), cc)
	}
}
