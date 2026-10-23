package win32

import (
	"codmtune/internal/ui"
)

// Custom chrome: the app is borderless now, so a compact minimize/close pair is
// drawn over the header's empty right corner. The top 30 px is a drag region.
func (w *window) drawChrome() {
	if w.fs || w.surface == nil {
		return
	}
	c := w.surface.Canvas
	th := ui.CurrentTheme
	W := float32(w.app.W)
	bg := th.Bg
	c.FillRect(ui.R(W-112, 0, 112, 32), bg)
	c.Line(W-112, 0, W-112, 32, 1, th.Outline)
	cy := float32(16)
	c.Line(W-85, cy, W-73, cy, 1, th.Muted)
	c.Line(W-54, cy-6, W-42, cy+6, 1, th.Muted)
	c.Line(W-54, cy+6, W-42, cy-6, 1, th.Muted)
}

// chromeHit returns 1 for minimize, 2 for close, 3 for drag, 0 otherwise.
func (w *window) chromeHit(x, y float32) int {
	if w.fs || y > 30 {
		return 0
	}
	W := float32(w.app.W)
	switch {
	case x > W-70 && x <= W-36:
		return 1
	case x > W-36:
		return 2
	case x < W-112:
		return 3
	}
	return 0
}
