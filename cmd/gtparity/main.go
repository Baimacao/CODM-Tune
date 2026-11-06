package main

// gtparity renders one identical frame through both backends and reports the
// pixels that differ. It is the only honest way to answer "did the GPU path
// draw the same thing": a live-window capture carries scroll position, focus
// and live readouts, so diffing it against a headless preview compares state
// rather than rendering.
//
// Usage: gtparity [-style endfield] [-shot home] [-out diff.png]

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"codmtune/internal/appui"
	"codmtune/internal/tune"
	"codmtune/internal/ui"
	"codmtune/internal/win32"
)

func main() {
	style := flag.String("style", "endfield", "ui style")
	themeName := flag.String("theme", "dark", "theme")
	shot := flag.String("shot", "home", "screen to capture: home | settings")
	switchTo := flag.String("switch", "", "draw -style first, then switch to this style and compare")
	out := flag.String("out", "", "write a diff png here")
	flag.Parse()

	previewHome := filepath.Join(os.TempDir(), "codmtune-parity")
	_ = os.RemoveAll(previewHome)
	_ = os.Setenv("CODMTUNE_HOME", previewHome)

	if err := win32.LoadUI(); err != nil {
		fail(err)
	}
	defer win32.StopUI()

	soft, err := win32.NewSurface(appui.WinW, appui.WinH)
	if err != nil {
		fail(err)
	}
	defer soft.Close()

	hwnd, err := win32.CreateOffscreenWindow("parity", appui.WinW, appui.WinH)
	if err != nil {
		fail(err)
	}
	if err := win32.LoadGPU(); err != nil {
		fmt.Println("GPU unavailable, nothing to compare:", err)
		return
	}
	defer win32.StopGPU()
	gs, err := win32.NewGpuSurface(hwnd, appui.WinW, appui.WinH)
	if err != nil {
		fail(err)
	}
	defer gs.Close()

	app, ctrl := appui.NewHeadless()
	ctrl.Update(func(c *tune.Config) {
		c.Style = *style
		c.Theme = *themeName
	})
	appui.Retheme(app, ctrl)

	// Style-switch mode: the bug the user hit only appears after a switch, so
	// draw one style on the GPU surface first and let the transform and every
	// cached resource settle before the frame that gets compared.
	if *switchTo != "" {
		ui.Invalidate()
		c0 := gs.BeginCanvas(ui.CurrentTheme.Bg)
		if c0 == nil {
			fail(fmt.Errorf("gpu canvas (warmup)"))
		}
		app.Frame(c0, 0)
		c0.Close()
		gs.Canvas = nil
		ctrl.Update(func(c *tune.Config) { c.Style = *switchTo })
		appui.Retheme(app, ctrl)
	}

	const dt = float32(1) / 60
	for i := 0; i < 120; i++ {
		app.Frame(soft.Canvas, dt)
	}
	ctrl.Detect()
	for i := 0; i < 180; i++ {
		app.Frame(soft.Canvas, dt)
	}

	if *shot == "settings" {
		hosts := app.Screens.Hosts
		root := hosts[len(hosts)-1].Content
		for _, n := range ui.Focusables(root) {
			if d, ok := n.Data.(string); ok && d == "nav:settings" {
				app.SetFocus(n)
				break
			}
		}
		app.Key(ui.KeyEnter, ui.Press)
		app.Key(ui.KeyEnter, ui.Release)
		for i := 0; i < 90; i++ {
			app.Frame(soft.Canvas, dt)
		}
	}

	// Both frames must come from the SAME app state or the diff measures the
	// state, not the renderer. An earlier version let detection finish between
	// the two draws and the diff lit up every row whose status text changed.
	// dt=0 draws without advancing any animation, and Invalidate is what asks
	// Frame to draw at all when nothing is moving.
	ui.Invalidate()
	if !app.Frame(soft.Canvas, 0) {
		fail(fmt.Errorf("software frame did not draw"))
	}
	softPix := append([]byte(nil), soft.Pixels()...)

	c := gs.BeginCanvas(ui.CurrentTheme.Bg)
	if c == nil {
		fail(fmt.Errorf("gpu canvas"))
	}
	ui.Invalidate()
	if !app.Frame(c, 0) {
		fail(fmt.Errorf("gpu frame did not draw"))
	}
	c.Close()
	gs.Canvas = nil

	gpuPix := make([]byte, appui.WinW*appui.WinH*4)
	if err := gs.ReadPixels(gpuPix); err != nil {
		fail(err)
	}

	w, h := appui.WinW, appui.WinH
	var diff, big int
	var worstX, worstY, worstD int
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rows := map[int]int{}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			// BGRA from both.
			dr := int(softPix[i+2]) - int(gpuPix[i+2])
			dg := int(softPix[i+1]) - int(gpuPix[i+1])
			db := int(softPix[i]) - int(gpuPix[i])
			if dr < 0 {
				dr = -dr
			}
			if dg < 0 {
				dg = -dg
			}
			if db < 0 {
				db = -db
			}
			d := dr + dg + db
			if d > 12 {
				diff++
				if d > 60 {
					big++
				}
				if d > worstD {
					worstD, worstX, worstY = d, x, y
				}
				rows[y]++
				img.SetRGBA(x, y, color.RGBA{255, 0, 0, 255})
			} else {
				v := uint8(0)
				if d > 0 {
					v = 40
				}
				img.SetRGBA(x, y, color.RGBA{v, v, v, 255})
			}
		}
	}
	fmt.Printf("%s/%s %dx%d: %d px differ (>12), %d badly (>60)\n",
		*style, *themeName, w, h, diff, big)
	fmt.Printf("worst delta %d at (%d,%d)\n", worstD, worstX, worstY)
	type rc struct{ y, n int }
	var top []rc
	for y, n := range rows {
		top = append(top, rc{y, n})
	}
	for i := 0; i < len(top); i++ {
		for j := i + 1; j < len(top); j++ {
			if top[j].n > top[i].n {
				top[i], top[j] = top[j], top[i]
			}
		}
	}
	if len(top) > 10 {
		top = top[:10]
	}
	for _, r := range top {
		fmt.Printf("  y=%-4d %d px\n", r.y, r.n)
	}
	if *out != "" {
		writeBGRA(*out+".soft.png", softPix, w, h)
		writeBGRA(*out+".gpu.png", gpuPix, w, h)
		f, err := os.Create(*out)
		if err != nil {
			fail(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			fail(err)
		}
		fmt.Println("wrote", *out)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func writeBGRA(path string, pix []byte, w, h int) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := (y*w + x) * 4
			o := img.PixOffset(x, y)
			img.Pix[o+0] = pix[i+2]
			img.Pix[o+1] = pix[i+1]
			img.Pix[o+2] = pix[i+0]
			img.Pix[o+3] = 255
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	_ = png.Encode(f, img)
}
