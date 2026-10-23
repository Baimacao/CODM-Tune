package win32

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"codmtune/internal/ui"
)

//go:embed codmtune_ui.dll
var shimBytes []byte

type shimProcs struct {
	dll        *syscall.LazyDLL
	init       *syscall.LazyProc
	shutdown   *syscall.LazyProc
	canvasNew  *syscall.LazyProc
	canvasFree *syscall.LazyProc
	fillRect   *syscall.LazyProc
	fillRound  *syscall.LazyProc
	strokeRound *syscall.LazyProc
	line       *syscall.LazyProc
	circle     *syscall.LazyProc
	strokeCircle *syscall.LazyProc
	text       *syscall.LazyProc
	measure    *syscall.LazyProc
	pushClip   *syscall.LazyProc
	popClip    *syscall.LazyProc
	fontFamily *syscall.LazyProc
	imageSize  *syscall.LazyProc
	drawImage  *syscall.LazyProc
	glyph      *syscall.LazyProc
	scale      *syscall.LazyProc
	view       *syscall.LazyProc
}

var shim *shimProcs

// LoadUI extracts the embedded GDI+ shim (once per build hash) and loads it.
func LoadUI() error {
	if shim != nil {
		return nil
	}
	path, err := extractShim()
	if err != nil {
		return err
	}
	dll := syscall.NewLazyDLL(path)
	s := &shimProcs{
		dll:         dll,
		init:        dll.NewProc("gt_init"),
		shutdown:    dll.NewProc("gt_shutdown"),
		canvasNew:   dll.NewProc("gt_canvas_new"),
		canvasFree:  dll.NewProc("gt_canvas_free"),
		fillRect:    dll.NewProc("gt_fill_rect"),
		fillRound:   dll.NewProc("gt_fill_round"),
		strokeRound: dll.NewProc("gt_stroke_round"),
		line:        dll.NewProc("gt_line"),
		circle:      dll.NewProc("gt_circle"),
		strokeCircle: dll.NewProc("gt_stroke_circle"),
		text:        dll.NewProc("gt_text"),
		measure:     dll.NewProc("gt_measure"),
		pushClip:    dll.NewProc("gt_push_clip"),
		popClip:     dll.NewProc("gt_pop_clip"),
		fontFamily:  dll.NewProc("gt_set_font_family"),
		imageSize:   dll.NewProc("gt_image_size"),
		drawImage:   dll.NewProc("gt_draw_image"),
		glyph:       dll.NewProc("gt_glyph"),
		scale:       dll.NewProc("gt_set_scale"),
		view:        dll.NewProc("gt_set_view"),
	}
	if r, _, e := s.init.Call(); r != 0 {
		return fmt.Errorf("gt_init failed: %v", e)
	}
	shim = s
	return nil
}

// extractShim finds a usable shim DLL: next to the executable when running
// from a checkout, otherwise the copy extracted from the embedded bytes.
func extractShim() (string, error) {
	if exe, err := os.Executable(); err == nil {
		local := filepath.Join(filepath.Dir(exe), "codmtune_ui.dll")
		if _, err := os.Stat(local); err == nil {
			return local, nil
		}
	}
	sum := sha256.Sum256(shimBytes)
	tag := hex.EncodeToString(sum[:6])
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "CODMTune")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "codmtune_ui_"+tag+".dll")
	if st, err := os.Stat(path); err == nil && st.Size() == int64(len(shimBytes)) {
		return path, nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, shimBytes, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

// StopUI releases GDI+ and its cached objects.
func StopUI() {
	if shim != nil {
		shim.shutdown.Call()
		shim = nil
	}
}

// Canvas implements ui.Painter through the shim.
type Canvas struct {
	G uintptr
	W int
	H int
}

func NewCanvas(hdc uintptr, w, h int) *Canvas {
	c := &Canvas{W: w, H: h}
	if shim == nil {
		return c
	}
	ptr, _, _ := shim.canvasNew.Call(hdc)
	c.G = ptr
	return c
}

func (c *Canvas) Close() {
	if c.G != 0 && shim != nil {
		shim.canvasFree.Call(c.G)
		c.G = 0
	}
}

func i32(v float32) int32 { return int32(roundf(v)) }

func (c *Canvas) FillRect(r ui.Rect, col ui.Color) {
	if c.G == 0 || r.Empty() {
		return
	}
	shim.fillRect.Call(c.G, uintptr(i32(r.X)), uintptr(i32(r.Y)), uintptr(i32(r.W)), uintptr(i32(r.H)), uintptr(uint32(col)))
}

func (c *Canvas) FillRoundRect(r ui.Rect, rad float32, col ui.Color) {
	if c.G == 0 || r.Empty() {
		return
	}
	shim.fillRound.Call(c.G, uintptr(i32(r.X)), uintptr(i32(r.Y)), uintptr(i32(r.W)), uintptr(i32(r.H)), uintptr(i32(rad)), uintptr(uint32(col)))
}

func (c *Canvas) StrokeRoundRect(r ui.Rect, rad, width float32, col ui.Color) {
	if c.G == 0 || r.Empty() {
		return
	}
	shim.strokeRound.Call(c.G, uintptr(i32(r.X)), uintptr(i32(r.Y)), uintptr(i32(r.W)), uintptr(i32(r.H)), uintptr(i32(rad)), uintptr(i32(width*10)), uintptr(uint32(col)))
}

func (c *Canvas) Line(x1, y1, x2, y2, width float32, col ui.Color) {
	if c.G == 0 {
		return
	}
	shim.line.Call(c.G, uintptr(i32(x1)), uintptr(i32(y1)), uintptr(i32(x2)), uintptr(i32(y2)), uintptr(i32(width*10)), uintptr(uint32(col)))
}

func (c *Canvas) Circle(cx, cy, radius float32, col ui.Color) {
	if c.G == 0 {
		return
	}
	shim.circle.Call(c.G, uintptr(i32(cx)), uintptr(i32(cy)), uintptr(i32(radius)), uintptr(uint32(col)))
}

func (c *Canvas) StrokeCircle(cx, cy, radius, width float32, col ui.Color) {
	if c.G == 0 {
		return
	}
	shim.strokeCircle.Call(c.G, uintptr(i32(cx)), uintptr(i32(cy)), uintptr(i32(radius)), uintptr(i32(width*10)), uintptr(uint32(col)))
}

func (c *Canvas) Text(r ui.Rect, s string, size float32, col ui.Color, a ui.Align, bold bool) {
	c.text(r, s, size, col, a, bold, false)
}

func (c *Canvas) TextBlock(r ui.Rect, s string, size float32, col ui.Color, a ui.Align, bold bool) {
	c.text(r, s, size, col, a, bold, true)
}

func (c *Canvas) text(r ui.Rect, s string, size float32, col ui.Color, a ui.Align, bold, wrap bool) {
	if c.G == 0 || s == "" || r.Empty() {
		return
	}
	txt, err := syscall.UTF16FromString(s)
	if err != nil {
		return
	}
	align := int32(0)
	switch a {
	case ui.AlignCenter:
		align = 1
	case ui.AlignRight:
		align = 2
	}
	b := int32(0)
	if bold {
		b = 1
	}
	w := int32(0)
	if wrap {
		w = 1
	}
	shim.text.Call(c.G, uintptr(i32(r.X)), uintptr(i32(r.Y)), uintptr(i32(r.W)), uintptr(i32(r.H)),
		uintptr(unsafe.Pointer(&txt[0])), uintptr(int32(roundf(size))), uintptr(uint32(col)), uintptr(align), uintptr(b), uintptr(w))
}

func (c *Canvas) Measure(s string, size float32, bold bool) ui.Vec2 {
	if c.G == 0 || s == "" {
		return ui.Vec2{}
	}
	txt, err := syscall.UTF16FromString(s)
	if err != nil {
		return ui.Vec2{}
	}
	var w, h int32
	b := int32(0)
	if bold {
		b = 1
	}
	shim.measure.Call(c.G, uintptr(unsafe.Pointer(&txt[0])), uintptr(int32(roundf(size))), uintptr(b),
		uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&h)))
	return ui.Vec2{X: float32(w), Y: float32(h)}
}

func (c *Canvas) PushClip(r ui.Rect) {
	if c.G == 0 {
		return
	}
	shim.pushClip.Call(c.G, uintptr(i32(r.X)), uintptr(i32(r.Y)), uintptr(i32(r.W)), uintptr(i32(r.H)))
}

func (c *Canvas) PopClip() {
	if c.G == 0 {
		return
	}
	shim.popClip.Call(c.G)
}

func roundf(v float32) float32 {
	if v >= 0 {
		return float32(int32(v + 0.5))
	}
	return float32(int32(v - 0.5))
}

// SetFontFamily switches the process-wide UI font family. It returns an error
// when the family is not installed.
func SetFontFamily(name string) error {
	if shim == nil {
		return fmt.Errorf("ui shim not loaded")
	}
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	r, _, _ := shim.fontFamily.Call(uintptr(unsafe.Pointer(p)))
	if r != 0 {
		return fmt.Errorf("font family %q unavailable", name)
	}
	return nil
}

// SetView applies a scale and a viewport offset for fullscreen presentation.
func (c *Canvas) SetView(scale float32, ox, oy int) {
	if c.G == 0 || scale <= 0 {
		return
	}
	shim.view.Call(c.G, uintptr(uint32(scale*1000+0.5)), uintptr(ox), uintptr(oy))
}

// SetScale scales all logical coordinates into physical pixels (DPI scaling).
func (c *Canvas) SetScale(scale float32) {
	if c.G == 0 || scale <= 0 {
		return
	}
	shim.scale.Call(c.G, uintptr(uint32(scale*1000+0.5)))
}

// Glyph draws one Segoe MDL2 / Fluent icon-font codepoint.
func (c *Canvas) Glyph(s string, r ui.Rect, size float32, col ui.Color) {
	if c.G == 0 || s == "" || r.Empty() {
		return
	}
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		return
	}
	shim.glyph.Call(c.G, uintptr(unsafe.Pointer(p)),
		uintptr(i32(r.X)), uintptr(i32(r.Y)), uintptr(i32(r.W)), uintptr(i32(r.H)),
		uintptr(i32(size)), uintptr(col))
}

// ImageSize reports an image's pixel dimensions via GDI+.
func (c *Canvas) ImageSize(path string) (int, int) {
	return ImageSize(path)
}

// ImageSize reports an image's pixel dimensions via GDI+.
func ImageSize(path string) (int, int) {
	if shim == nil {
		return 0, 0
	}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0
	}
	var w, h int32
	shim.imageSize.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&h)))
	return int(w), int(h)
}

// DrawImage paints a still image into the canvas with global opacity.
func (c *Canvas) DrawImage(path string, r ui.Rect, alpha uint8) bool {
	if c.G == 0 || !fileExists(path) || r.Empty() {
		return false
	}
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	shim.drawImage.Call(c.G, uintptr(unsafe.Pointer(p)),
		uintptr(i32(r.X)), uintptr(i32(r.Y)), uintptr(i32(r.W)), uintptr(i32(r.H)), uintptr(alpha))
	return true
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// Surface is an offscreen 32-bit DIB with a memory DC and a Canvas.
type Surface struct {
	W, H  int
	HDC   uintptr
	Bmp   uintptr
	Old   uintptr
	Bits  unsafe.Pointer
	Canvas *Canvas
}

func NewSurface(w, h int) (*Surface, error) {
	s := &Surface{W: w, H: h}
	screen, _, _ := pGetDC.Call(0)
	s.HDC, _, _ = pCreateCompatibleDC.Call(screen)
	pReleaseDC.Call(0, screen)
	if s.HDC == 0 {
		return nil, syscall.EINVAL
	}
	bmi := BITMAPINFO{BmiHeader: BITMAPINFOHEADER{
		BiSize: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})), BiWidth: int32(w), BiHeight: int32(-h),
		BiPlanes: 1, BiBitCount: 32, BiCompression: BI_RGB,
	}}
	var bits unsafe.Pointer
	s.Bmp, _, _ = pCreateDIBSection.Call(s.HDC, uintptr(unsafe.Pointer(&bmi)), DIB_RGB_COLORS, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if s.Bmp == 0 {
		pDeleteDC.Call(s.HDC)
		return nil, syscall.EINVAL
	}
	s.Bits = bits
	s.Old, _, _ = pSelectObject.Call(s.HDC, s.Bmp)
	s.Canvas = NewCanvas(s.HDC, w, h)
	return s, nil
}

func (s *Surface) Blit(dst uintptr) {
	if dst == 0 || s.HDC == 0 {
		return
	}
	pBitBlt.Call(dst, 0, 0, uintptr(s.W), uintptr(s.H), s.HDC, 0, 0, SRCCOPY)
}

// Pixels are BGRA bytes, top row first.
func (s *Surface) Pixels() []byte {
	if s.Bits == nil {
		return nil
	}
	return unsafe.Slice((*byte)(s.Bits), s.W*s.H*4)
}

func (s *Surface) Close() {
	if s.Canvas != nil {
		s.Canvas.Close()
	}
	if s.Old != 0 && s.HDC != 0 {
		pSelectObject.Call(s.HDC, s.Old)
	}
	if s.Bmp != 0 {
		pDeleteObject.Call(s.Bmp)
	}
	if s.HDC != 0 {
		pDeleteDC.Call(s.HDC)
	}
}
