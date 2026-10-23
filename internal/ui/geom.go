package ui

import "math"

// Color is 0xAARRGGBB, the byte order GDI+ uses.
type Color uint32

func RGB(r, g, b uint8) Color { return Color(0xFF000000) | Color(r)<<16 | Color(g)<<8 | Color(b) }
func RGBA(r, g, b, a uint8) Color {
	return Color(a)<<24 | Color(r)<<16 | Color(g)<<8 | Color(b)
}

func (c Color) A() uint8 { return uint8(c >> 24) }
func (c Color) R() uint8 { return uint8(c >> 16) }
func (c Color) G() uint8 { return uint8(c >> 8) }
func (c Color) B() uint8 { return uint8(c) }

func (c Color) WithAlpha(a uint8) Color { return c&0x00FFFFFF | Color(a)<<24 }

// Mix blends a toward b by t in [0,1].
func Mix(a, b Color, t float32) Color {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	f := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return RGBA(f(a.R(), b.R()), f(a.G(), b.G()), f(a.B(), b.B()), f(a.A(), b.A()))
}

// Vec2 is a point or a size.
type Vec2 struct{ X, Y float32 }

// Rect is an axis-aligned rectangle in pixels.
type Rect struct{ X, Y, W, H float32 }

func R(x, y, w, h float32) Rect { return Rect{x, y, w, h} }

func (r Rect) Right() float32  { return r.X + r.W }
func (r Rect) Bottom() float32 { return r.Y + r.H }
func (r Rect) Center() Vec2    { return Vec2{r.X + r.W/2, r.Y + r.H/2} }
func (r Rect) Empty() bool     { return r.W <= 0 || r.H <= 0 }

func (r Rect) Offset(dx, dy float32) Rect { return Rect{r.X + dx, r.Y + dy, r.W, r.H} }
func (r Rect) Inset(d float32) Rect       { return Rect{r.X + d, r.Y + d, r.W - 2*d, r.H - 2*d} }
func (r Rect) InsetXY(x, y float32) Rect  { return Rect{r.X + x, r.Y + y, r.W - 2*x, r.H - 2*y} }
func (r Rect) Contains(p Vec2) bool {
	return p.X >= r.X && p.Y >= r.Y && p.X < r.Right() && p.Y < r.Bottom()
}
func (r Rect) Overlaps(o Rect) bool {
	return r.X < o.Right() && o.X < r.Right() && r.Y < o.Bottom() && o.Y < r.Bottom()
}
func (r Rect) Intersect(o Rect) Rect {
	x := maxf(r.X, o.X)
	y := maxf(r.Y, o.Y)
	right := minf(r.Right(), o.Right())
	bottom := minf(r.Bottom(), o.Bottom())
	return Rect{x, y, maxf(right-x, 0), maxf(bottom-y, 0)}
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
func clampf(x, lo, hi float32) float32 { return maxf(lo, minf(x, hi)) }

// Round rounds to a whole pixel, the way the reference renderer does.
func Round(x float32) float32 { return float32(math.Round(float64(x))) }

// Align is horizontal text alignment.
type Align uint8

const (
	AlignLeft Align = iota
	AlignCenter
	AlignRight
)

// Painter is the renderer the UI draws through. win32.Canvas is the only
// implementation in the app; cmd/preview uses the same one on a memory DC.
type Painter interface {
	FillRect(r Rect, c Color)
	FillRoundRect(r Rect, rad float32, c Color)
	StrokeRoundRect(r Rect, rad, width float32, c Color)
	Line(x1, y1, x2, y2, width float32, c Color)
	Circle(cx, cy, radius float32, c Color)
	StrokeCircle(cx, cy, radius, width float32, c Color)
	Text(r Rect, s string, size float32, c Color, a Align, bold bool)
	// TextBlock wraps inside r instead of drawing one ellipsised line.
	TextBlock(r Rect, s string, size float32, c Color, a Align, bold bool)
	Measure(s string, size float32, bold bool) Vec2
	// Glyph draws one icon-font codepoint, centered in r.
	Glyph(s string, r Rect, size float32, c Color)
	ImageSize(path string) (int, int)
	DrawImage(path string, r Rect, alpha uint8) bool
	PushClip(r Rect)
	PopClip()
}
