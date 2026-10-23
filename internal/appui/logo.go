package appui

import (
	_ "embed"
	"os"
	"path/filepath"

	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

//go:embed logo-wide.png
var logoPNG []byte

//go:embed logo-mark.png
var logoMarkPNG []byte

//go:embed logo-ink.png
var logoInkPNG []byte

var logoInkCache string

// logoInk is the black game-material silhouette used for feature icons on
// light surfaces; the white mark is used on dark surfaces.
func logoInk() string {
	if logoInkCache != "" {
		return logoInkCache
	}
	path := filepath.Join(tune.ConfigPath(), "logo-ink.png")
	if st, err := os.Stat(path); err != nil || st.Size() != int64(len(logoInkPNG)) {
		_ = os.WriteFile(path, logoInkPNG, 0o644)
	}
	logoInkCache = path
	return path
}

// logoFile extracts the game-material silhouette for GDI+ and returns its path.
func logoFile() string {
	path := filepath.Join(tune.ConfigPath(), "logo-wide.png")
	if st, err := os.Stat(path); err != nil || st.Size() != int64(len(logoPNG)) {
		_ = os.WriteFile(path, logoPNG, 0o644)
	}
	return path
}

// logoMark is the square, diagonal variant used in the sidebar.
func logoMark() string {
	path := filepath.Join(tune.ConfigPath(), "logo-mark.png")
	if st, err := os.Stat(path); err != nil || st.Size() != int64(len(logoMarkPNG)) {
		_ = os.WriteFile(path, logoMarkPNG, 0o644)
	}
	return path
}

// drawLogo is the in-app turbine-rifle mark. At 30 px the silhouette has to
// drop detail and keep only stock, receiver, barrel, magazine and the turbine.
func drawLogo(c ui.Painter, r ui.Rect, col ui.Color) {
	x, y, w, h := r.X, r.Y, r.W, r.H
	line := func(x1, y1, x2, y2, wid float32) { c.Line(x1, y1, x2, y2, wid, col) }
	rx, ry := x+w*0.24, y+h*0.40
	c.FillRect(ui.R(rx, ry, w*0.34, h*0.22), col)
	line(x+w*0.04, y+h*0.58, rx, y+h*0.48, h*0.12)
	line(x+w*0.06, y+h*0.56, x+w*0.16, y+h*0.76, h*0.08)
	line(x+w*0.58, y+h*0.51, x+w*0.90, y+h*0.51, h*0.07)
	c.FillRect(ui.R(x+w*0.87, y+h*0.45, w*0.09, h*0.12), col)
	line(x+w*0.38, y+h*0.62, x+w*0.33, y+h*0.86, h*0.09)
	c.Circle(x+w*0.74, y+h*0.64, w*0.20, col)
	c.Circle(x+w*0.74, y+h*0.64, w*0.15, ui.CurrentTheme.PrimaryBg)
	c.Circle(x+w*0.74, y+h*0.64, w*0.04, col)
	for i := 0; i < 4; i++ {
		ang := float64(i)*1.5708 + 0.4
		line(x+w*0.74+float32(cosf(ang))*w*0.05, y+h*0.64+float32(sinf(ang))*h*0.05,
			x+w*0.74+float32(cosf(ang))*w*0.14, y+h*0.64+float32(sinf(ang))*h*0.14, h*0.045)
	}
}
