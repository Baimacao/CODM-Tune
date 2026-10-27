package hud

import (
	"fmt"
	"strings"

	"codmtune/internal/sys"
	"codmtune/internal/ui"
	"codmtune/internal/win32"
)

// Logical HUD size; the child window may be DPI-scaled beyond this.
const (
	hudW = 320
	hudH = 216
)

// draw renders one HUD frame. The window is cleared to pure black before this
// call, and LWA_COLORKEY makes those corners transparent.
func draw(c *win32.Canvas) {
	st := snapshot()
	scale := float32(st.HudScale) / 100
	if scale < 0.7 || scale > 1.6 {
		scale = 1
	}
	c.SetScale(scale)
	if st.Theme == "oled" {
		ui.SetTheme(&ui.OLED)
	} else {
		ui.SetTheme(&ui.Dark)
	}
	th := ui.CurrentTheme
	w, h := float32(hudW), float32(hudH)
	if !st.ShowText {
		c.FillRoundRect(ui.R(1, 1, w-2, h-2), 16, th.Surface)
		c.StrokeRoundRect(ui.R(1, 1, w-2, h-2), 16, 1, th.Outline)
	}

	hudText(c, st, ui.R(16, 10, 200, 18), "CODM HUD", 9, th.Muted, ui.AlignLeft, true)
	col := th.Success
	label := "LIVE"
	if !st.Game {
		col, label = th.Danger, "OFFLINE"
	}
	c.Circle(w-82, 19, 4, col)
	hudText(c, st, ui.R(w-74, 10, 58, 18), label, 8, col, ui.AlignRight, true)
	if !st.ShowText {
		c.Line(14, 30, w-14, 30, 1, th.Outline)
	}

	if st.ShowFPS {
		fps := "—"
		size := float32(26)
		fpsCol := th.Muted
		if st.Ready {
			fps = fmt.Sprintf("%.0f", st.FPS)
			size = 42
			fpsCol = th.Text
		}
		hudText(c, st, ui.R(16, 38, 150, 48), fps, size, fpsCol, ui.AlignLeft, true)
		hudText(c, st, ui.R(16, 84, 150, 14), "FPS", 8, th.Text, ui.AlignLeft, false)

		low, stag := "—", "—"
		if st.Ready {
			low = fmt.Sprintf("%.0f", st.Low)
			stag = fmt.Sprintf("%.1f%%", st.Stutter)
		}
		x := float32(150)
		if st.ShowLow {
			hudText(c, st, ui.R(x, 40, 70, 16), "1% LOW", 8, th.Text, ui.AlignLeft, false)
			hudText(c, st, ui.R(x, 56, 70, 22), low, 15, th.Text, ui.AlignLeft, true)
			x += 76
		}
		if st.ShowStut {
			hudText(c, st, ui.R(x, 40, 90, 16), "STUTTER", 8, th.Text, ui.AlignLeft, false)
			hudText(c, st, ui.R(x, 56, 90, 22), stag, 15, th.Text, ui.AlignLeft, true)
		}
	}

	if st.ShowNet {
		rtt := "—"
		if st.RTT > 0 {
			rtt = fmt.Sprintf("%d ms", st.RTT)
		}
		hudText(c, st, ui.R(16, 92, 130, 16), "网络延迟", 8, th.Text, ui.AlignLeft, false)
		hudText(c, st, ui.R(16, 108, 130, 24), rtt, 17, th.Text, ui.AlignLeft, true)
		if st.Jitter > 0 {
			hudText(c, st, ui.R(150, 98, 160, 16), fmt.Sprintf("jitter %d ms", st.Jitter), 8, th.Muted, ui.AlignLeft, false)
		}
	}

	rowY := float32(132)
	if st.ShowLoss {
		los := "—"
		if st.LossKnown {
			los = fmt.Sprintf("%.0f%%", st.Loss)
		}
		hudText(c, st, ui.R(16, rowY, 190, 14), "吞子弹风险（估算）", 8, th.Text, ui.AlignLeft, false)
		hudText(c, st, ui.R(w-56, rowY, 40, 14), los, 9, th.Text, ui.AlignRight, true)
		rowY += 16
	}
	if st.ShowProv {
		prov := st.Province
		if prov == "" {
			prov = st.ServerIP
		}
		if prov != "" {
			hudText(c, st, ui.R(16, rowY, w-32, 14), "服务器 "+prov, 8, th.Text, ui.AlignLeft, false)
			rowY += 16
		}
	}

	if st.Style == "full" && st.ShowKeys {
		y := rowY + 2
		keys := []struct {
			vk int
			l  string
		}{{0x57, "W"}, {0x41, "A"}, {0x53, "S"}, {0x44, "D"}, {0x10, "SHIFT"}, {0x11, "CTRL"}, {0x20, "SPACE"}, {0x52, "R"}}
		x := float32(16)
		for _, k := range keys {
			wid := float32(22)
			if len(k.l) > 1 {
				wid = 40
			}
			if x+wid > w-16 {
				break
			}
			on := sys.KeyDown(k.vk)
			bg, fg := th.Surface2, th.Muted
			if on {
				bg, fg = th.Primary, th.OnPrimary
			}
			if st.ShowText {
				hudText(c, st, ui.R(x, y, wid, 20), k.l, 9, fg, ui.AlignCenter, true)
			} else {
				c.FillRoundRect(ui.R(x, y, wid, 20), 5, bg)
				hudText(c, st, ui.R(x, y, wid, 20), k.l, 8, fg, ui.AlignCenter, true)
			}
			x += wid + 4
		}
	}

	hudText(c, st, ui.R(16, h-18, w-32, 12), "PresentMon · 估算值，不是命中判定", 7, th.Muted, ui.AlignLeft, false)
}

// Digest fingerprints exactly the values the HUD draws. The window timer runs
// at 2 Hz but only repaints when this string changes, so a static overlay
// performs no drawing at all.
func Digest() string {
	st := snapshot()
	var b strings.Builder
	fmt.Fprintf(&b, "%t|%t|%t|%.0f|%.0f|%.1f|%d|%d|%.0f|%t|%s|%s|%s|%d|%d|%d|%d|",
		st.Show, st.Game, st.Ready, st.FPS, st.Low, st.Stutter, st.RTT, st.Jitter,
		st.Loss, st.LossKnown, st.ServerIP, st.Province, st.Style, st.Opacity,
		st.HudX, st.HudY, st.HudScale)
	for _, v := range []bool{st.ShowFPS, st.ShowLow, st.ShowStut, st.ShowLoss, st.ShowText, st.ShowKeys, st.ShowNet, st.ShowProv} {
		if v {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	b.WriteString("|" + st.Theme)
	if st.Style == "full" && st.ShowKeys {
		// Held keys change the little key row; they are part of the picture.
		for _, vk := range []int{0x57, 0x41, 0x53, 0x44, 0x10, 0x11, 0x20, 0x52} {
			if sys.KeyDown(vk) {
				b.WriteByte('1')
			} else {
				b.WriteByte('0')
			}
		}
	}
	return b.String()
}

// hudText keeps the HUD legible over bright game scenes in pure-text mode.
// A 2px dark outline is drawn in twelve directions before the glyphs.
func hudText(c *win32.Canvas, st State, r ui.Rect, s string, size float32, col ui.Color, align ui.Align, bold bool) {
	if st.ShowText {
		out := ui.RGB(0, 0, 0)
		for _, d := range [][2]float32{{-2, 0}, {2, 0}, {0, -2}, {0, 2}, {-2, -2}, {2, -2}, {-2, 2}, {2, 2}, {-2, -1}, {2, 1}, {-1, -2}, {1, 2}} {
			c.Text(ui.R(r.X+d[0], r.Y+d[1], r.W, r.H), s, size, out, align, bold)
		}
	}
	c.Text(r, s, size, col, align, bold)
}
