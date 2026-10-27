package appui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

// ensureArkGrid bakes the 48px grid and 3px scanlines into one layer. The frame
// loop used to issue ~430 GDI lines per redraw, which made a single repaint
// cost tens of milliseconds.
//
// When there is no wallpaper the layer is flattened onto the theme background
// and saved as an opaque image: a full-window translucent blit costs ~17 ms per
// frame in GDI+, an opaque one costs almost nothing. With a wallpaper the grid
// has to stay transparent so the image shows through.
func ensureArkGrid(th *ui.Theme, backdrop bool) string {
	if !th.Industrial() {
		return ""
	}
	w, h := WinW, WinH
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := th.Bg
	// The flattened layer is the window base, so these alphas are what the user
	// actually sees (they used to be overlaid on top of the pages). Kept low
	// enough to read as paper texture instead of graph paper.
	scan := color.RGBA{0, 0, 0, 0x05}
	g := th.Tertiary
	grid := color.RGBA{g.R(), g.G(), g.B(), 0x09}

	// blend composites src over the base the way GDI+ would.
	blend := func(base color.RGBA, src color.RGBA) color.RGBA {
		a := float32(src.A) / 255
		mix := func(x, y uint8) uint8 {
			return uint8(float32(x)*(1-a) + float32(y)*a + 0.5)
		}
		return color.RGBA{mix(base.R, src.R), mix(base.G, src.G), mix(base.B, src.B), 255}
	}
	base := color.RGBA{}
	if !backdrop {
		base = color.RGBA{bg.R(), bg.G(), bg.B(), 255}
	}
	put := func(x, y int, src color.RGBA) {
		if backdrop {
			img.SetRGBA(x, y, src)
			return
		}
		img.SetRGBA(x, y, blend(base, src))
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			put(x, y, base)
		}
	}
	for y := 0; y < h; y += 3 {
		for x := 0; x < w; x++ {
			put(x, y, scan)
		}
	}
	for y := 0; y < h; y += 48 {
		for x := 0; x < w; x++ {
			put(x, y, grid)
		}
	}
	for x := 0; x < w; x += 48 {
		for y := 0; y < h; y++ {
			put(x, y, grid)
		}
	}

	name := "ark-grid-light.png"
	if th.Dark {
		name = "ark-grid-dark.png"
	}
	if !backdrop {
		// The flattened variant depends on the exact background colour, so it
		// gets its own file instead of overwriting the transparent one.
		name = fmt.Sprintf("ark-grid-%06x-v2.png", uint32(bg)&0xFFFFFF)
	}
	path := filepath.Join(tune.ConfigPath(), name)
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		return path
	}
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
