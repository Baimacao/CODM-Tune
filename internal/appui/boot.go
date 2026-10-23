package appui

import (
	"fmt"
	"time"

	"codmtune/internal/ui"
)

// boot is the eDEX-UI inspired cold start: a terminal handshake, a thin
// scanline grid and a spring progress bar. It is deliberately short (1.5 s)
// and has no focusable nodes, so a stray key cannot start a module.
func (s *screens) boot() *ui.Node {
	start := time.Now()
	root := ui.NewNode(ui.R(0, 0, WinW, WinH))
	lines := []string{
		"codmtune v2  ·  native win32/gdi+",
		"gdi+ shim .............. OK",
		"ace guard ............... passive",
		"power plan .............. ultimate",
		"hud bridge .............. ready",
		"registry snapshot ....... armed",
	}
	root.Update = func(n *ui.Node, dt float32) {
		ui.Invalidate()
	}
	root.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		elapsed := time.Since(start).Seconds()
		c.FillRect(r, ui.RGB(0, 0, 0))
		// Scanlines.
		for y := float32(0); y < r.H; y += 4 {
			c.FillRect(ui.R(0, y, r.W, 1), ui.RGB(255, 255, 255).WithAlpha(7))
		}
		// A single mint horizon line.
		c.Line(48, 92, r.W-48, 92, 1, ui.RGB(139, 255, 217))
		c.Text(ui.R(48, 30, 500, 22), "CODMTUNE", 14, ui.RGB(139, 255, 217), ui.AlignLeft, true)
		c.Text(ui.R(48, 54, 700, 22), "cold start / system handshake", 11, ui.RGB(90, 112, 104), ui.AlignLeft, false)
		if p := logoFile(); p != "" {
			c.DrawImage(p, ui.R(r.W-300, 30, 240, 52), 230)
		}
		y := float32(130)
		for i, line := range lines {
			if elapsed < 0.10*float64(i) {
				break
			}
			col := ui.RGB(139, 255, 217)
			if i > 3 {
				col = ui.RGB(160, 180, 172)
			}
			c.Text(ui.R(64, y, r.W-128, 20), "> "+line, 11, col, ui.AlignLeft, false)
			y += 26
		}
		// Progress.
		prog := float32(elapsed / 1.5)
		if prog > 1 {
			prog = 1
		}
		bar := ui.R(64, 420, r.W-128, 2)
		c.FillRect(bar, ui.RGB(20, 30, 26))
		c.FillRect(ui.R(bar.X, bar.Y, bar.W*prog, bar.H), ui.RGB(139, 255, 217))
		c.Text(ui.R(64, 434, 400, 16), fmt.Sprintf("initializing  %3.0f%%", prog*100), 9, ui.RGB(90, 112, 104), ui.AlignLeft, false)
		// Blinking cursor.
		if int(elapsed*2)%2 == 0 {
			c.FillRect(ui.R(64, y+8, 7, 12), ui.RGB(139, 255, 217))
		}
		c.Text(ui.R(64, r.H-70, 600, 16), "loading categories · 1.5 s", 9, ui.RGB(70, 86, 80), ui.AlignLeft, false)
	}
	return root
}
