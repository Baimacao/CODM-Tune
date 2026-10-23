// Command icon renders the turbine-rifle silhouette and writes a multi-size
// Windows .ico. The artwork is original vector work, not a game asset.
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func main() {
	big := draw(256)
	sizes := []int{256, 128, 64, 48, 32, 24, 16}
	var entries []icoEntry
	for _, s := range sizes {
		img := resize(big, s)
		entries = append(entries, icoEntry{size: s, data: dib(img)})
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, uint16(0))
	binary.Write(&out, binary.LittleEndian, uint16(1))
	binary.Write(&out, binary.LittleEndian, uint16(len(entries)))
	offset := 6 + 16*len(entries)
	for _, e := range entries {
		w := byte(e.size)
		if e.size == 256 {
			w = 0
		}
		out.WriteByte(w)
		out.WriteByte(w)
		out.WriteByte(0)
		out.WriteByte(0)
		binary.Write(&out, binary.LittleEndian, uint16(1))
		binary.Write(&out, binary.LittleEndian, uint16(32))
		binary.Write(&out, binary.LittleEndian, uint32(len(e.data)))
		binary.Write(&out, binary.LittleEndian, uint32(offset))
		offset += len(e.data)
	}
	for _, e := range entries {
		out.Write(e.data)
	}
	if err := os.WriteFile("internal/win32/app.ico", out.Bytes(), 0o644); err != nil {
		panic(err)
	}
	// A PNG copy is handy for docs and the README.
	_ = png.Encode(must(os.Create("docs/assets/icon.png")), big)
	_ = png.Encode(must(os.Create("preview/icon.png")), big)
}

type icoEntry struct {
	size int
	data []byte
}

func must(f *os.File, err error) *os.File {
	if err != nil {
		panic(err)
	}
	return f
}

func draw(n int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	white := color.RGBA{235, 242, 255, 255}
	s := float64(n) / 256.0
	line := func(x1, y1, x2, y2, w float64) {
		thickLine(img, x1*s, y1*s, x2*s, y2*s, w*s, white)
	}
	rect := func(x, y, w, h float64) { fillRect(img, x*s, y*s, w*s, h*s, white) }
	circle := func(cx, cy, r float64, fill bool) {
		if fill {
			fillCircle(img, cx*s, cy*s, r*s, white)
		} else {
			strokeCircle(img, cx*s, cy*s, r*s, 6*s, white)
		}
	}

	// Stock, receiver, barrel, muzzle.
	line(24, 150, 70, 132, 24)
	line(28, 152, 44, 174, 16)
	rect(62, 112, 90, 42)
	line(150, 128, 232, 128, 11)
	rect(230, 118, 18, 20)
	rect(150, 118, 56, 22)
	// Magazine and grip.
	line(98, 152, 84, 198, 17)
	line(74, 152, 64, 182, 15)
	// Trigger guard.
	circle(82, 164, 13, false)
	// Scope.
	rect(86, 90, 38, 13)
	line(92, 103, 92, 112, 7)
	line(118, 103, 118, 112, 7)
	// Turbine shroud: the "巨擎涡轮" motif.
	cx, cy, r := 190.0, 178.0, 46.0
	circle(cx, cy, r, false)
	circle(cx, cy, 7, true)
	for i := 0; i < 6; i++ {
		ang := float64(i) * math.Pi / 3
		x2 := cx + math.Cos(ang)*(r-10)
		y2 := cy + math.Sin(ang)*(r-10)
		line(cx+math.Cos(ang)*10, cy+math.Sin(ang)*10, x2, y2, 9)
	}
	// A cut through the receiver keeps the silhouette from reading as a blob.
	line(104, 134, 136, 134, 6)
	return img
}

func fillRect(img *image.RGBA, x, y, w, h float64, c color.RGBA) {
	for yy := int(y); yy < int(y+h); yy++ {
		for xx := int(x); xx < int(x+w); xx++ {
			img.SetRGBA(xx, yy, c)
		}
	}
}

func fillCircle(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	for y := int(cy - r); y <= int(cy+r); y++ {
		for x := int(cx - r); x <= int(cx+r); x++ {
			dx, dy := float64(x)-cx, float64(y)-cy
			if dx*dx+dy*dy <= r*r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func strokeCircle(img *image.RGBA, cx, cy, r, w float64, c color.RGBA) {
	steps := int(2 * math.Pi * r)
	if steps < 32 {
		steps = 32
	}
	for i := 0; i < steps; i++ {
		a := 2 * math.Pi * float64(i) / float64(steps)
		fillCircle(img, cx+math.Cos(a)*r, cy+math.Sin(a)*r, w/2, c)
	}
}

func thickLine(img *image.RGBA, x1, y1, x2, y2, w float64, c color.RGBA) {
	dx, dy := x2-x1, y2-y1
	steps := int(math.Hypot(dx, dy))
	if steps < 1 {
		steps = 1
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		fillCircle(img, x1+dx*t, y1+dy*t, w/2, c)
	}
}

func resize(src *image.RGBA, n int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, n, n))
	f := src.Bounds().Dx() / n
	if f < 1 {
		f = 1
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			var r, g, b, a int
			for dy := 0; dy < f; dy++ {
				for dx := 0; dx < f; dx++ {
					c := src.RGBAAt(x*f+dx, y*f+dy)
					r += int(c.R)
					g += int(c.G)
					b += int(c.B)
					a += int(c.A)
				}
			}
			cnt := f * f
			dst.SetRGBA(x, y, color.RGBA{uint8(r / cnt), uint8(g / cnt), uint8(b / cnt), uint8(a / cnt)})
		}
	}
	return dst
}

func dib(img *image.RGBA) []byte {
	n := img.Bounds().Dx()
	var buf bytes.Buffer
	// BITMAPINFOHEADER: height doubled, XOR then AND mask.
	binary.Write(&buf, binary.LittleEndian, uint32(40))
	binary.Write(&buf, binary.LittleEndian, int32(n))
	binary.Write(&buf, binary.LittleEndian, int32(n*2))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(32))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(n*n*4))
	binary.Write(&buf, binary.LittleEndian, int32(0))
	binary.Write(&buf, binary.LittleEndian, int32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	for y := n - 1; y >= 0; y-- {
		for x := 0; x < n; x++ {
			c := img.RGBAAt(x, y)
			buf.Write([]byte{c.B, c.G, c.R, c.A})
		}
	}
	maskRow := ((n + 31) / 32) * 4
	buf.Write(make([]byte, maskRow*n))
	return buf.Bytes()
}
