package main

// Measures the two backends on the same workload: how long a full frame takes
// when rasterised by GDI+ on the CPU versus drawn by D2D1 on the GPU.

import (
	"fmt"
	"time"

	"codmtune/internal/ui"
	"codmtune/internal/win32"
)

func main() {
	if err := win32.LoadUI(); err != nil {
		fmt.Println("LoadUI:", err)
		return
	}
	// Shutdown order matters: the GPU surface owns D2D resources that must be
	// released before StopGPU tears the device down, and both must happen
	// before StopUI unloads the shim.
	defer func() {
		win32.StopGPU()
		win32.StopUI()
	}()

	if err := win32.LoadGPU(); err != nil {
		fmt.Println("GPU unavailable:", err)
	} else {
		fmt.Println("GPU backend initialised")
	}

	const w, h = 1100, 700
	// Software reference: the DIB surface the app used before.
	soft, err := win32.NewSurface(w, h)
	if err != nil {
		fmt.Println("NewSurface:", err)
		return
	}
	defer soft.Close()
	soft.Canvas.SetScale(1)

	work := func(c ui.Painter) {
		th := ui.CurrentTheme
		c.FillRect(ui.R(0, 0, w, h), th.Bg)
		for i := 0; i < 12; i++ {
			y := float32(40 + i*52)
			c.FillRoundRect(ui.R(24, y, w-48, 44), 12, th.Surface)
			c.StrokeRoundRect(ui.R(24, y, w-48, 44), 12, 1, th.Outline)
			c.Circle(52, y+22, 10, th.Primary)
			c.Text(ui.R(76, y+10, 300, 22), "CODM TUNE 帧时间测量", 13, th.Text, ui.AlignLeft, true)
			c.Text(ui.R(400, y+12, 400, 18), "the quick brown fox jumps over 14 lazy dogs", 12, th.Muted, ui.AlignLeft, false)
			c.Line(24, y+46, w-24, y+46, 1, th.Outline2)
		}
	}

	measure := func(label string, run func()) {
		run() // warm caches
		const n = 60
		start := time.Now()
		for i := 0; i < n; i++ {
			run()
		}
		fmt.Printf("%-10s %6.2f ms/frame\n", label, float64(time.Since(start).Microseconds())/1000/float64(n))
	}

	measure("software", func() { work(soft.Canvas) })

	if !win32.GPUAvailable() {
		fmt.Println("skipping GPU measurement")
		return
	}
	if true {
		// A hidden window is enough for a swap chain; nothing is presented to
		// the user during the measurement.
		hwnd, err := win32.CreateOffscreenWindow("gputest", w, h)
		if err != nil {
			fmt.Println("CreateOffscreenWindow:", err)
			return
		}
		gs, err := win32.NewGpuSurface(hwnd, w, h)
		if err != nil {
			fmt.Println("NewGpuSurface:", err)
			return
		}
		gs.SetScale(1)
		measure("gpu", func() {
			c := gs.BeginCanvas(ui.CurrentTheme.Bg)
			if c == nil {
				return
			}
			work(c)
			c.Close()
			gs.Canvas = nil
		})
		gs.Close()
	}
}
