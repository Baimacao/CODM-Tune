// Command preview replays the real screens headlessly, renders them through
// the same GDI+ canvas the window uses, and writes a contact sheet plus a GIF.
// It is the evidence tool for hold-to-confirm, spring transitions and the
// always-visible risk copy.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"

	"codmtune/internal/appui"
	"codmtune/internal/tune"
	"codmtune/internal/ui"
	"codmtune/internal/win32"
)

type event struct {
	at   float32
	fn   func()
	shot string
}

func main() {
	out := flag.String("o", "preview", "output directory")
	style := flag.String("style", "", "ui style override: standard | ark | endfield")
	themeName := flag.String("theme", "", "theme override: dark | light | oled")
	accent := flag.String("accent", "", "accent override")
	allShots := flag.Bool("all", false, "write every shot as an individual png")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	// Keep preview runs out of the real configuration.
	previewHome := filepath.Join(os.TempDir(), "codmtune-preview")
	_ = os.RemoveAll(previewHome)
	_ = os.Setenv("CODMTUNE_HOME", previewHome)
	if err := win32.LoadUI(); err != nil {
		fail(err)
	}
	defer win32.StopUI()

	surf, err := win32.NewSurface(appui.WinW, appui.WinH)
	if err != nil {
		fail(err)
	}
	defer surf.Close()

	app, ctrl := appui.NewHeadless()
	dt := float32(1) / 60
	if *style != "" || *themeName != "" || *accent != "" {
		ctrl.Update(func(c *tune.Config) {
			if *style != "" {
				c.Style = *style
			}
			if *themeName != "" {
				c.Theme = *themeName
			}
			if *accent != "" {
				c.Accent = *accent
			}
		})
		appui.Retheme(app, ctrl)
		// Retheme rebuilds the screen through the normal switch path, which
		// starts a wipe. Let it finish so the first capture is not clipped.
		for i := 0; i < 90; i++ {
			app.Frame(surf.Canvas, dt)
		}
	}
	ctrl.Detect()

	// Let detection finish so the capture shows the real install path.
	for i := 0; i < 180; i++ {
		app.Frame(surf.Canvas, dt)
		exe, _, _, _, _ := ctrl.Game()
		if exe != "" {
			break
		}
	}

	key := func(k ui.Key, ph ui.Phase) func() {
		return func() { app.Key(k, ph) }
	}
	root := func() *ui.Node {
		hosts := app.Screens.Hosts
		if len(hosts) == 0 {
			return nil
		}
		return hosts[len(hosts)-1].Content
	}
	focusNav := func(id string) func() {
		return func() {
			r := root()
			if r == nil {
				return
			}
			for _, n := range ui.Focusables(r) {
				if d, ok := n.Data.(string); ok && d == "nav:"+id {
					app.SetFocus(n)
					return
				}
			}
		}
	}
	mk := func(at float32, fn func()) event { return event{at: at, fn: fn} }
	openNav := func(at float32, id string) []event {
		return []event{
			mk(at, focusNav(id)),
			mk(at+0.03, key(ui.KeyEnter, ui.Press)),
			mk(at+0.06, key(ui.KeyEnter, ui.Release)),
		}
	}
	sh := func(at float32, name string) event { return event{at: at, shot: name} }

	var events []event
	events = append(events, sh(0.30, "home"))
	events = append(events, openNav(0.55, "settings")...)
	events = append(events, sh(1.55, "settings"))
	// Walk down to the 主题 row (界面风格 / 强调色 / 从壁纸取色 / 主题) and open
	// its chooser, so the capture shows a real second-level menu rather than
	// the first focusable row doing something unrelated.
	events = append(events,
		mk(1.65, key(ui.KeyDown, ui.Press)),
		mk(1.69, key(ui.KeyDown, ui.Release)),
		mk(1.75, key(ui.KeyDown, ui.Press)),
		mk(1.79, key(ui.KeyDown, ui.Release)),
		mk(1.85, key(ui.KeyDown, ui.Press)),
		mk(1.89, key(ui.KeyDown, ui.Release)),
		mk(1.95, key(ui.KeyEnter, ui.Press)),
		mk(2.01, key(ui.KeyEnter, ui.Release)),
		sh(2.75, "theme-choose"),
		mk(2.85, key(ui.KeyBack, ui.Press)),
		mk(2.90, key(ui.KeyBack, ui.Release)),
	)
	events = append(events, openNav(3.10, "visual")...)
	events = append(events, sh(4.05, "visual"))
	events = append(events, openNav(4.25, "system")...)
	events = append(events, sh(5.20, "system"))
	events = append(events, openNav(5.40, "performance")...)
	events = append(events,
		sh(6.30, "performance"),
		mk(6.45, key(ui.KeyDown, ui.Press)),
		mk(6.53, key(ui.KeyDown, ui.Release)),
		mk(6.65, key(ui.KeyEnter, ui.Press)),
		sh(7.10, "hold-62"),
		sh(7.65, "hold-95"),
		mk(7.90, key(ui.KeyEnter, ui.Release)),
		sh(8.10, "ace-fired"),
	)
	events = append(events, openNav(8.30, "overlay")...)
	events = append(events, sh(9.25, "overlay"))
	events = append(events, openNav(9.45, "overview")...)
	events = append(events, sh(10.45, "home-back"))

	var shots []image.Image
	var names []string
	gifFrames := []*image.Paletted{}
	gifDelay := []int{}
	const gifStart, gifEnd = 0.30, 10.05

	frame := 0
	next := 0
	for t := float32(0); t < 11.0; t += dt {
		for next < len(events) && events[next].at <= t {
			e := events[next]
			if e.shot != "" {
				img := toRGBA(surf.Pixels(), appui.WinW, appui.WinH)
				if *allShots || e.shot == "hold-62" || e.shot == "ace-fired" {
					writePNG(filepath.Join(*out, e.shot+".png"), img)
				}
				shots = append(shots, img)
				names = append(names, e.shot)
			} else if e.fn != nil {
				e.fn()
			}
			next++
		}
		app.Frame(surf.Canvas, dt)
		if t >= gifStart && t <= gifEnd && frame%2 == 0 {
			full := toRGBA(surf.Pixels(), appui.WinW, appui.WinH)
			crop := full.SubImage(image.Rect(0, 100, 1100, 656)).(*image.RGBA)
			half := boxScale(crop, 2)
			pal := image.NewPaletted(half.Bounds(), palette.Plan9)
			draw.Draw(pal, pal.Bounds(), half, half.Bounds().Min, draw.Src)
			gifFrames = append(gifFrames, pal)
			gifDelay = append(gifDelay, 3)
		}
		frame++
	}

	writePNG(filepath.Join(*out, "shots.png"), contactSheet(shots))
	writeGIF(filepath.Join(*out, "flow.gif"), gifFrames, gifDelay)
	for i, n := range names {
		fmt.Printf("%2d %s\n", i+1, n)
	}
	fmt.Printf("%d gif frames, %d shots\n", len(gifFrames), len(shots))
}

func toRGBA(b []byte, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		img.Pix[i*4+0] = b[i*4+2]
		img.Pix[i*4+1] = b[i*4+1]
		img.Pix[i*4+2] = b[i*4+0]
		img.Pix[i*4+3] = 255
	}
	return img
}

func boxScale(src *image.RGBA, f int) *image.RGBA {
	w, h := src.Bounds().Dx()/f, src.Bounds().Dy()/f
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b, a int
			for dy := 0; dy < f; dy++ {
				for dx := 0; dx < f; dx++ {
					i := src.PixOffset(src.Bounds().Min.X+x*f+dx, src.Bounds().Min.Y+y*f+dy)
					r += int(src.Pix[i])
					g += int(src.Pix[i+1])
					b += int(src.Pix[i+2])
					a += int(src.Pix[i+3])
				}
			}
			n := f * f
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), uint8(a / n)})
		}
	}
	return dst
}

func contactSheet(shots []image.Image) *image.RGBA {
	const cols, gap, scale = 4, 10, 2
	if len(shots) == 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	tw, th := appui.WinW/scale, appui.WinH/scale
	rows := (len(shots) + cols - 1) / cols
	sheet := image.NewRGBA(image.Rect(0, 0, cols*(tw+gap)+gap, rows*(th+gap)+gap))
	for i := range sheet.Pix {
		sheet.Pix[i] = 24
	}
	for i, s := range shots {
		src := s.(*image.RGBA)
		small := boxScale(src, scale)
		x, y := gap+(i%cols)*(tw+gap), gap+(i/cols)*(th+gap)
		draw.Draw(sheet, image.Rect(x, y, x+tw, y+th), small, image.Point{}, draw.Src)
	}
	return sheet
}

func writePNG(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fail(err)
	}
}

func writeGIF(path string, frames []*image.Paletted, delay []int) {
	f, err := os.Create(path)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	if err := gif.EncodeAll(f, &gif.GIF{Image: frames, Delay: delay}); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "preview:", err)
	os.Exit(1)
}
