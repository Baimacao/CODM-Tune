package appui

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

// wallpaperPath returns the current desktop wallpaper. TranscodedWallpaper is
// the reliable fallback when the source path is a slideshow or a UWP asset.
func wallpaperPath() string {
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Control Panel\Desktop`, registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetStringValue("Wallpaper"); err == nil && v != "" {
			k.Close()
			if st, err := os.Stat(v); err == nil && !st.IsDir() {
				return v
			}
		} else {
			k.Close()
		}
	}
	if dir, err := os.UserConfigDir(); err == nil {
		p := filepath.Join(dir, "Microsoft", "Windows", "Themes", "TranscodedWallpaper")
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// accentFromWallpaper samples the wallpaper's saturated mid-tones and returns
// a representative #RRGGBB color. Near-black and near-white pixels are ignored
// so a dark desktop still yields a usable accent.
func accentFromWallpaper() (string, bool) {
	path := wallpaperPath()
	if path == "" {
		return "", false
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return "", false
	}
	b := img.Bounds()
	stepX := b.Dx() / 96
	stepY := b.Dy() / 54
	if stepX < 1 {
		stepX = 1
	}
	if stepY < 1 {
		stepY = 1
	}
	var sr, sg, sb, n float64
	var ar, ag, ab, an float64
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			r, g, bl, a := img.At(x, y).RGBA()
			if a < 0x8000 {
				continue
			}
			r8, g8, b8 := float64(r>>8), float64(g>>8), float64(bl>>8)
			lum := 0.2126*r8 + 0.7152*g8 + 0.0722*b8
			if lum < 18 || lum > 242 {
				continue
			}
			mx, mn := r8, r8
			if g8 > mx {
				mx = g8
			}
			if b8 > mx {
				mx = b8
			}
			if g8 < mn {
				mn = g8
			}
			if b8 < mn {
				mn = b8
			}
			ar, ag, ab, an = ar+r8, ag+g8, ab+b8, an+1
			if mx-mn >= 18 {
				sr, sg, sb, n = sr+r8, sg+g8, sb+b8, n+1
			}
		}
	}
	if n < 12 {
		sr, sg, sb, n = ar, ag, ab, an
	}
	if n < 1 {
		return "", false
	}
	return fmt.Sprintf("#%02X%02X%02X", int(sr/n), int(sg/n), int(sb/n)), true
}
