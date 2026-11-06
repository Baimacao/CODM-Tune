package win32

import (
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"unsafe"

	"codmtune/internal/ui"
)

// gpuProcs are the Direct2D/D3D11 entry points compiled into the same shim
// DLL. They are resolved lazily and may be absent on an old build, so every
// caller checks gpuOK first.
type gpuProcs struct {
	init        *syscall.LazyProc
	shutdown    *syscall.LazyProc
	surfaceNew  *syscall.LazyProc
	surfaceFree *syscall.LazyProc
	resize      *syscall.LazyProc
	present     *syscall.LazyProc
	canvasNew   *syscall.LazyProc
	canvasFree  *syscall.LazyProc
	fillRect    *syscall.LazyProc
	fillRound   *syscall.LazyProc
	strokeRound *syscall.LazyProc
	line        *syscall.LazyProc
	circle      *syscall.LazyProc
	strokeCirc  *syscall.LazyProc
	text        *syscall.LazyProc
	measure     *syscall.LazyProc
	pushClip    *syscall.LazyProc
	popClip     *syscall.LazyProc
	glyph       *syscall.LazyProc
	setFamily   *syscall.LazyProc
	imageSize   *syscall.LazyProc
	drawImage   *syscall.LazyProc
	scale       *syscall.LazyProc
	view        *syscall.LazyProc
}

var (
	gpu      *gpuProcs
	gpuOK    atomic.Bool
	gpuErr   string
	gpuInUse atomic.Bool
)

// gpuDisabled lets a user bypass the GPU path (CODMTUNE_NO_GPU=1) when a
// driver misbehaves, without needing a rebuild.
func gpuDisabled() bool { return os.Getenv("CODMTUNE_NO_GPU") == "1" }

// LoadGPU tries to bring up the Direct2D backend. It returns an error instead
// of failing the process: a machine without a usable D3D11 device keeps the
// GDI+ path and the app still runs.
func LoadGPU() error {
	if gpuOK.Load() {
		return nil
	}
	if gpuErr != "" {
		return fmt.Errorf("%s", gpuErr)
	}
	if shim == nil {
		return fmt.Errorf("shim not loaded")
	}
	dll := shim.dll
	p := &gpuProcs{
		init:        dll.NewProc("gtg_init"),
		shutdown:    dll.NewProc("gtg_shutdown"),
		surfaceNew:  dll.NewProc("gtg_surface_new"),
		surfaceFree: dll.NewProc("gtg_surface_free"),
		resize:      dll.NewProc("gtg_resize"),
		present:     dll.NewProc("gtg_present"),
		canvasNew:   dll.NewProc("gtg_canvas_new"),
		canvasFree:  dll.NewProc("gtg_canvas_free"),
		fillRect:    dll.NewProc("gtg_fill_rect"),
		fillRound:   dll.NewProc("gtg_fill_round"),
		strokeRound: dll.NewProc("gtg_stroke_round"),
		line:        dll.NewProc("gtg_line"),
		circle:      dll.NewProc("gtg_circle"),
		strokeCirc:  dll.NewProc("gtg_stroke_circle"),
		text:        dll.NewProc("gtg_text"),
		measure:     dll.NewProc("gtg_measure"),
		pushClip:    dll.NewProc("gtg_push_clip"),
		popClip:     dll.NewProc("gtg_pop_clip"),
		glyph:       dll.NewProc("gtg_glyph"),
		setFamily:   dll.NewProc("gtg_set_font_family"),
		imageSize:   dll.NewProc("gtg_image_size"),
		drawImage:   dll.NewProc("gtg_draw_image"),
		scale:       dll.NewProc("gtg_set_scale"),
		view:        dll.NewProc("gtg_set_view"),
	}
	// Resolve before calling: a missing export would panic inside Call.
	for _, pr := range []*syscall.LazyProc{
		p.init, p.shutdown, p.surfaceNew, p.surfaceFree, p.resize, p.present,
		p.canvasNew, p.canvasFree, p.fillRect, p.fillRound, p.strokeRound,
		p.line, p.circle, p.strokeCirc, p.text, p.measure, p.pushClip,
		p.popClip, p.glyph, p.setFamily, p.imageSize, p.drawImage, p.scale, p.view,
	} {
		if err := pr.Find(); err != nil {
			gpuErr = "gpu exports missing: " + err.Error()
			return fmt.Errorf("%s", gpuErr)
		}
	}
	if r, _, e := p.init.Call(); r != 0 {
		gpuErr = fmt.Sprintf("gtg_init failed: code=%d %v", r, e)
		return fmt.Errorf("%s", gpuErr)
	}
	gpu = p
	gpuOK.Store(true)
	return nil
}

// GPUAvailable reports whether frames are currently going through D3D11.
func GPUAvailable() bool { return gpuOK.Load() }

// GPUError returns why the GPU backend is not in use, or "".
func GPUError() string { return gpuErr }

// StopGPU releases the device. Safe when the backend never started.
func StopGPU() {
	if gpu != nil && gpuOK.Load() {
		gpu.shutdown.Call()
	}
	gpuOK.Store(false)
}

// GpuSurface is a DXGI swap chain bound to a window plus the D2D context that
// draws into it. It replaces the DIB Section in Surface when the GPU backend is
// live.
//
// A frame is one BeginCanvas/Close pair: D2D needs BeginDraw before the first
// primitive and EndDraw before Present, so the canvas cannot be long-lived the
// way the GDI+ one is.
type GpuSurface struct {
	P      uintptr
	W, H   int
	Canvas *GpuCanvas
	// Scale and offset mirror the GDI+ canvas view transform. They are
	// re-applied at the start of every frame because each frame gets a fresh
	// D2D device context state.
	Scale float32
	OffX  int
	OffY  int
}

func NewGpuSurface(hwnd uintptr, w, h int) (*GpuSurface, error) {
	if !gpuOK.Load() || gpu == nil {
		return nil, fmt.Errorf("gpu backend not loaded")
	}
	p, _, e := gpu.surfaceNew.Call(hwnd, uintptr(w), uintptr(h))
	if p == 0 {
		return nil, fmt.Errorf("gtg_surface_new: %v", e)
	}
	return &GpuSurface{P: p, W: w, H: h}, nil
}

func (s *GpuSurface) Resize(w, h int) error {
	if s == nil || s.P == 0 {
		return fmt.Errorf("nil surface")
	}
	if r, _, _ := gpu.resize.Call(s.P, uintptr(w), uintptr(h)); r != 0 {
		return fmt.Errorf("gtg_resize: %d", r)
	}
	s.W, s.H = w, h
	return nil
}

// Present flips the swap chain. It returns false when the device was lost, so
// the caller can rebuild the surface rather than present a black frame forever.
func (s *GpuSurface) Present() bool {
	if s == nil || s.P == 0 {
		return false
	}
	r, _, _ := gpu.present.Call(s.P)
	return r == 0
}

func (s *GpuSurface) Close() {
	if s == nil || s.P == 0 {
		return
	}
	if s.Canvas != nil {
		s.Canvas.Close()
		s.Canvas = nil
	}
	gpu.surfaceFree.Call(s.P)
	s.P = 0
}

// BeginCanvas starts a frame and clears to bg.
func (s *GpuSurface) BeginCanvas(bg ui.Color) *GpuCanvas {
	if s == nil || s.P == 0 {
		return nil
	}
	p, _, _ := gpu.canvasNew.Call(s.P, uintptr(uint32(bg)))
	if p == 0 {
		return nil
	}
	c := &GpuCanvas{G: p, W: s.W, H: s.H}
	if s.Scale > 0 && (s.Scale != 1 || s.OffX != 0 || s.OffY != 0) {
		if s.OffX != 0 || s.OffY != 0 {
			c.SetView(s.Scale, s.OffX, s.OffY)
		} else {
			c.SetScale(s.Scale)
		}
	}
	s.Canvas = c
	return c
}

// SetView records the logical-to-device transform for the next frame.
func (s *GpuSurface) SetView(scale float32, ox, oy int) {
	if s == nil {
		return
	}
	s.Scale, s.OffX, s.OffY = scale, ox, oy
	if s.Canvas != nil {
		s.Canvas.SetView(scale, ox, oy)
	}
}

// SetScale records a plain scale for the next frame.
func (s *GpuSurface) SetScale(scale float32) {
	if s == nil {
		return
	}
	s.Scale, s.OffX, s.OffY = scale, 0, 0
	if s.Canvas != nil {
		s.Canvas.SetScale(scale)
	}
}

// GpuCanvas implements ui.Painter on the D2D context.
type GpuCanvas struct {
	G uintptr
	W int
	H int
}

func (c *GpuCanvas) Close() {
	if c.G != 0 && gpu != nil {
		gpu.canvasFree.Call(c.G)
		c.G = 0
	}
}

func (c *GpuCanvas) ok() bool { return c != nil && c.G != 0 }

func (c *GpuCanvas) FillRect(r ui.Rect, col ui.Color) {
	if !c.ok() || r.Empty() {
		return
	}
	gpu.fillRect.Call(c.G, uintptr(i32(r.X)), uintptr(i32(r.Y)), uintptr(i32(r.W)), uintptr(i32(r.H)), uintptr(uint32(col)))
}

func (c *GpuCanvas) FillRoundRect(r ui.Rect, rad float32, col ui.Color) {
	if !c.ok() || r.Empty() {
		return
	}
	gpu.fillRound.Call(c.G, uintptr(i32(r.X)), uintptr(i32(r.Y)), uintptr(i32(r.W)), uintptr(i32(r.H)), uintptr(i32(rad)), uintptr(uint32(col)))
}

func (c *GpuCanvas) StrokeRoundRect(r ui.Rect, rad, width float32, col ui.Color) {
	if !c.ok() || r.Empty() {
		return
	}
	gpu.strokeRound.Call(c.G, uintptr(i32(r.X)), uintptr(i32(r.Y)), uintptr(i32(r.W)), uintptr(i32(r.H)), uintptr(i32(rad)), uintptr(i32(width*10)), uintptr(uint32(col)))
}

func (c *GpuCanvas) Line(x1, y1, x2, y2, width float32, col ui.Color) {
	if !c.ok() {
		return
	}
	gpu.line.Call(c.G, uintptr(i32(x1)), uintptr(i32(y1)), uintptr(i32(x2)), uintptr(i32(y2)), uintptr(i32(width*10)), uintptr(uint32(col)))
}

func (c *GpuCanvas) Circle(cx, cy, radius float32, col ui.Color) {
	if !c.ok() || radius <= 0 {
		return
	}
	gpu.circle.Call(c.G, uintptr(i32(cx)), uintptr(i32(cy)), uintptr(i32(radius)), uintptr(uint32(col)))
}

func (c *GpuCanvas) StrokeCircle(cx, cy, radius, width float32, col ui.Color) {
	if !c.ok() || radius <= 0 {
		return
	}
	gpu.strokeCirc.Call(c.G, uintptr(i32(cx)), uintptr(i32(cy)), uintptr(i32(radius)), uintptr(i32(width*10)), uintptr(uint32(col)))
}

func (c *GpuCanvas) Text(r ui.Rect, s string, size float32, col ui.Color, a ui.Align, bold bool) {
	c.text(r, s, size, col, a, bold, false)
}

func (c *GpuCanvas) TextBlock(r ui.Rect, s string, size float32, col ui.Color, a ui.Align, bold bool) {
	c.text(r, s, size, col, a, bold, true)
}

func (c *GpuCanvas) text(r ui.Rect, s string, size float32, col ui.Color, a ui.Align, bold, wrap bool) {
	if !c.ok() || s == "" || r.Empty() {
		return
	}
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return
	}
	b := uintptr(0)
	if bold {
		b = 1
	}
	w := uintptr(0)
	if wrap {
		w = 1
	}
	gpu.text.Call(c.G, uintptr(i32(r.X)), uintptr(i32(r.Y)), uintptr(i32(r.W)), uintptr(i32(r.H)),
		uintptr(unsafe.Pointer(p)), uintptr(i32(size)), uintptr(uint32(col)), uintptr(a), b, w)
}

func (c *GpuCanvas) Measure(s string, size float32, bold bool) ui.Vec2 {
	if !c.ok() || s == "" {
		return ui.Vec2{}
	}
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return ui.Vec2{}
	}
	b := uintptr(0)
	if bold {
		b = 1
	}
	var w, h int32
	gpu.measure.Call(c.G, uintptr(unsafe.Pointer(p)), uintptr(i32(size)), b,
		uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&h)))
	return ui.Vec2{X: float32(w), Y: float32(h)}
}

func (c *GpuCanvas) Glyph(s string, r ui.Rect, size float32, col ui.Color) {
	if !c.ok() || s == "" || r.Empty() {
		return
	}
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return
	}
	gpu.glyph.Call(c.G, uintptr(unsafe.Pointer(p)), uintptr(i32(r.X)), uintptr(i32(r.Y)),
		uintptr(i32(r.W)), uintptr(i32(r.H)), uintptr(i32(size)), uintptr(uint32(col)))
}

func (c *GpuCanvas) ImageSize(path string) (int, int) {
	if gpu == nil || path == "" {
		return 0, 0
	}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0
	}
	var w, h int32
	gpu.imageSize.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&h)))
	return int(w), int(h)
}

func (c *GpuCanvas) DrawImage(path string, r ui.Rect, alpha uint8) bool {
	return c.drawImage(path, r, alpha)
}

func (c *GpuCanvas) DrawImageSolid(path string, r ui.Rect, alpha uint8) bool {
	return c.drawImage(path, r, alpha)
}

func (c *GpuCanvas) drawImage(path string, r ui.Rect, alpha uint8) bool {
	if !c.ok() || path == "" || r.Empty() {
		return false
	}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	ok, _, _ := gpu.drawImage.Call(c.G, uintptr(unsafe.Pointer(p)), uintptr(i32(r.X)),
		uintptr(i32(r.Y)), uintptr(i32(r.W)), uintptr(i32(r.H)), uintptr(alpha))
	return ok != 0
}

func (c *GpuCanvas) PushClip(r ui.Rect) {
	if !c.ok() {
		return
	}
	gpu.pushClip.Call(c.G, uintptr(i32(r.X)), uintptr(i32(r.Y)), uintptr(i32(r.W)), uintptr(i32(r.H)))
}

func (c *GpuCanvas) PopClip() {
	if !c.ok() {
		return
	}
	gpu.popClip.Call(c.G)
}

func (c *GpuCanvas) SetView(scale float32, ox, oy int) {
	if !c.ok() {
		return
	}
	gpu.view.Call(c.G, uintptr(i32(scale*1000)), uintptr(ox), uintptr(oy))
}

func (c *GpuCanvas) SetScale(scale float32) {
	if !c.ok() {
		return
	}
	gpu.scale.Call(c.G, uintptr(i32(scale*1000)))
}

// SetFontFamily switches the UI face for the GPU backend.
func (c *GpuCanvas) SetFontFamily(name string) int {
	if gpu == nil || name == "" {
		return 1
	}
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 1
	}
	r, _, _ := gpu.setFamily.Call(uintptr(unsafe.Pointer(p)))
	return int(r)
}

var _ ui.Painter = (*GpuCanvas)(nil)

// CreateOffscreenWindow makes a hidden window with a real HWND, which is what a
// DXGI swap chain needs. It exists for benchmarks and smoke tests.
func CreateOffscreenWindow(title string, w, h int) (uintptr, error) {
	hinst, _, _ := pGetModuleHandleW.Call(0)
	name, _ := syscall.UTF16FromString("CodmTuneOffscreen")
	titlep, _ := syscall.UTF16FromString(title)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         CS_HREDRAW | CS_VREDRAW,
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     hinst,
		HCursor:       loadCursor(0, IDC_ARROW),
		LpszClassName: &name[0],
	}
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, e := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(&name[0])),
		uintptr(unsafe.Pointer(&titlep[0])), uintptr(WS_POPUP), 0, 0,
		uintptr(w), uintptr(h), 0, 0, hinst, 0)
	if hwnd == 0 {
		return 0, fmt.Errorf("CreateWindowExW: %v", e)
	}
	return hwnd, nil
}
