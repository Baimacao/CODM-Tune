package appui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

// ensureArkGrid bakes the 48px grid and 3px scanlines into one transparent
// layer. The frame loop used to issue ~430 GDI lines per redraw, which made a
// single repaint cost tens of milliseconds.
func ensureArkGrid(th *ui.Theme) string {
	if !th.Industrial() {
		return ""
	}
	w, h := WinW, WinH
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	scan := color.RGBA{0, 0, 0, 0x08}
	for y := 0; y < h; y += 3 {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, scan)
		}
	}
	g := th.Tertiary
	grid := color.RGBA{g.R(), g.G(), g.B(), 0x10}
	for y := 0; y < h; y += 48 {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, grid)
		}
	}
	for x := 0; x < w; x += 48 {
		for y := 0; y < h; y++ {
			img.SetRGBA(x, y, grid)
		}
	}
	name := "ark-grid-light.png"
	if th.Dark {
		name = "ark-grid-dark.png"
	}
	path := filepath.Join(tune.ConfigPath(), name)
	f, err := os.Create(path)
	if err != nil {
		return ""
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return ""
	}
	_ = f.Close()
	return path
}
