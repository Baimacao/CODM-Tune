package appui

import (
	"math"

	"codmtune/internal/ui"
)

// iconGlyphs maps the app's icon names to Segoe MDL2 Assets codepoints. The
// font ships with Windows 10/11, so the whole icon set is native and sharp at
// every DPI without embedding glyph assets.
var iconGlyphs = map[string]string{
	"gauge":    "\uE9D2",
	"gamepad":  "\uE7FC",
	"bolt":     "\uE945",
	"monitor":  "\uE7F4",
	"window":   "\uE737",
	"disc":     "\uE714",
	"wifi":     "\uE968",
	"book":     "\uE8F1",
	"info":     "\uE946",
	"gear":     "\uE713",
	"mouse":    "\uE962",
	"shield":   "\uE83D",
	"chip":     "\uE950",
	"keyboard": "\uE765",
	"rocket":   "\uE7B8",
	"turbine":  "\uE9D9",
}

// drawIcon paints one icon-font glyph inside r.
func drawIcon(c ui.Painter, r ui.Rect, kind string, col ui.Color) {
	switch kind {
	case "weapon":
		path := logoInk()
		if ui.CurrentTheme.Dark {
			path = logoFile()
		}
		c.DrawImage(path, ui.R(r.X, r.Y+r.H*0.30, r.W, r.H*0.40), 255)
		return
	case "scope":
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		c.StrokeCircle(cx, cy, r.W*0.34, 1.6, col)
		c.StrokeCircle(cx, cy, r.W*0.13, 1.5, col)
		c.Line(cx-r.W*0.48, cy, cx-r.W*0.30, cy, 1.5, col)
		c.Line(cx+r.W*0.30, cy, cx+r.W*0.48, cy, 1.5, col)
		c.Line(cx, cy-r.H*0.48, cx, cy-r.H*0.28, 1.5, col)
		c.Line(cx, cy+r.H*0.28, cx, cy+r.H*0.48, 1.5, col)
		return
	case "ammo":
		c.FillRoundRect(ui.R(r.X+r.W*0.38, r.Y+r.H*0.30, r.W*0.24, r.H*0.48), 3, col)
		c.FillRect(ui.R(r.X+r.W*0.34, r.Y+r.H*0.70, r.W*0.32, r.H*0.10), col)
		c.Circle(r.X+r.W*0.50, r.Y+r.H*0.26, r.W*0.13, col)
		return
	}
	if g, ok := iconGlyphs[kind]; ok {
		c.Glyph(g, r, r.H*0.92, col)
		return
	}
	drawIconLegacy(c, r, kind, col)
}

// drawIconLegacy is the old vector fallback, used when the icon font is
// missing or a name is not mapped.
func drawIconLegacy(c ui.Painter, r ui.Rect, kind string, col ui.Color) {
	x, y, w, h := r.X, r.Y, r.W, r.H
	px := func(fx, fy float32) (float32, float32) { return x + fx*w, y + fy*h }
	line := func(x1, y1, x2, y2 float32) { c.Line(x1, y1, x2, y2, 1.6, col) }
	lineP := func(fx1, fy1, fx2, fy2 float32) {
		a, b := px(fx1, fy1)
		cc, d := px(fx2, fy2)
		line(a, b, cc, d)
	}
	switch kind {
	case "gauge":
		cx, cy := x+w/2, y+h*0.55
		c.StrokeCircle(cx, cy, w*0.38, 1.6, col)
		line(cx, cy, cx+w*0.22, cy-h*0.22)
		c.Circle(cx, cy, 1.7, col)
		lineP(0.18, 0.78, 0.32, 0.78)
		lineP(0.68, 0.78, 0.82, 0.78)
	case "gamepad":
		c.FillRoundRect(ui.R(x, y+h*0.2, w, h*0.62), h*0.31, col.WithAlpha(30))
		c.StrokeRoundRect(ui.R(x, y+h*0.2, w, h*0.62), h*0.31, 1.6, col)
		c.Circle(x+w*0.3, y+h*0.51, w*0.06, col)
		lineP(0.22, 0.51, 0.38, 0.51)
		lineP(0.3, 0.43, 0.3, 0.59)
		c.Circle(x+w*0.68, y+h*0.44, w*0.045, col)
		c.Circle(x+w*0.78, y+h*0.58, w*0.045, col)
	case "bolt":
		lineP(0.56, 0.08, 0.30, 0.52)
		lineP(0.30, 0.52, 0.50, 0.52)
		lineP(0.50, 0.52, 0.42, 0.92)
		lineP(0.42, 0.92, 0.72, 0.44)
		lineP(0.72, 0.44, 0.51, 0.44)
		lineP(0.51, 0.44, 0.56, 0.08)
	case "monitor":
		c.StrokeRoundRect(ui.R(x+w*0.04, y+h*0.10, w*0.92, h*0.66), 3, 1.6, col)
		lineP(0.5, 0.76, 0.5, 0.88)
		lineP(0.32, 0.9, 0.68, 0.9)
		lineP(0.2, 0.28, 0.42, 0.28)
	case "window":
		c.StrokeRoundRect(ui.R(x+w*0.16, y+h*0.06, w*0.78, h*0.66), 3, 1.5, col)
		c.StrokeRoundRect(ui.R(x+w*0.06, y+h*0.24, w*0.78, h*0.66), 3, 1.5, col)
		lineP(0.12, 0.36, 0.44, 0.36)
	case "disc":
		c.StrokeCircle(x+w/2, y+h/2, w*0.43, 1.6, col)
		c.StrokeCircle(x+w/2, y+h/2, w*0.16, 1.6, col)
		lineP(0.5, 0.07, 0.5, 0.18)
	case "wifi":
		lineP(0.10, 0.64, 0.90, 0.64)
		lineP(0.20, 0.44, 0.80, 0.44)
		lineP(0.32, 0.26, 0.68, 0.26)
		c.Circle(x+w*0.5, y+h*0.82, 2.2, col)
	case "book":
		c.StrokeRoundRect(ui.R(x+w*0.06, y+h*0.10, w*0.88, h*0.80), 2, 1.6, col)
		lineP(0.5, 0.10, 0.5, 0.90)
		lineP(0.18, 0.30, 0.40, 0.30)
		lineP(0.18, 0.46, 0.40, 0.46)
		lineP(0.60, 0.30, 0.82, 0.30)
	case "gear":
		cx, cy := x+w/2, y+h/2
		c.StrokeCircle(cx, cy, w*0.24, 1.6, col)
		c.Circle(cx, cy, w*0.06, col)
		for i := 0; i < 8; i++ {
			ang := float64(i) * 3.14159 / 4
			cos, sin := float32(cosf(ang)), float32(sinf(ang))
			line(cx+cos*w*0.28, cy+sin*w*0.28, cx+cos*w*0.40, cy+sin*w*0.40)
		}
	case "mouse":
		c.StrokeRoundRect(ui.R(x+w*0.26, y+h*0.08, w*0.48, h*0.84), w*0.24, 1.6, col)
		lineP(0.5, 0.16, 0.5, 0.42)
	case "shield":
		lineP(0.5, 0.06, 0.88, 0.22)
		lineP(0.88, 0.22, 0.82, 0.66)
		lineP(0.5, 0.94, 0.18, 0.66)
		lineP(0.18, 0.66, 0.12, 0.22)
		lineP(0.12, 0.22, 0.5, 0.06)
		lineP(0.34, 0.44, 0.48, 0.58)
		lineP(0.48, 0.58, 0.68, 0.34)
	case "chip":
		c.StrokeRoundRect(ui.R(x+w*0.2, y+h*0.2, w*0.6, h*0.6), 2, 1.6, col)
		for i := 0; i < 3; i++ {
			f := float32(i+1) / 4
			lineP(0.2, f, 0.06, f)
			lineP(0.8, f, 0.94, f)
			lineP(f, 0.2, f, 0.06)
			lineP(f, 0.8, f, 0.94)
		}
	case "turbine":
		cx, cy := x+w/2, y+h/2
		c.StrokeCircle(cx, cy, w*0.40, 1.8, col)
		c.Circle(cx, cy, w*0.07, col)
		for i := 0; i < 3; i++ {
			ang := float64(i)*2.0944 - 0.4
			cos, sin := float32(cosf(ang)), float32(sinf(ang))
			line(cx+cos*w*0.10, cy+sin*w*0.10, cx+cos*w*0.32, cy+sin*w*0.32)
		}
	case "keyboard":
		c.StrokeRoundRect(ui.R(x+w*0.04, y+h*0.26, w*0.92, h*0.5), 3, 1.5, col)
		for row := 0; row < 2; row++ {
			for i := 0; i < 4; i++ {
				cx := x + w*(0.16+float32(i)*0.22)
				cy := y + h*(0.42+float32(row)*0.16)
				c.Circle(cx, cy, 1.4, col)
			}
		}
	case "rocket":
		lineP(0.5, 0.06, 0.68, 0.44)
		lineP(0.68, 0.44, 0.62, 0.72)
		lineP(0.62, 0.72, 0.38, 0.72)
		lineP(0.38, 0.72, 0.32, 0.44)
		lineP(0.32, 0.44, 0.5, 0.06)
		c.Circle(x+w*0.5, y+h*0.42, w*0.07, col)
		lineP(0.42, 0.78, 0.5, 0.94)
		lineP(0.58, 0.78, 0.5, 0.94)
	default:
		c.StrokeCircle(x+w/2, y+h/2, w*0.4, 1.6, col)
	}
}

func cosf(v float64) float64 { return math.Cos(v) }
func sinf(v float64) float64 { return math.Sin(v) }
