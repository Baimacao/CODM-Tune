package appui

import (
	"math"
	"strconv"
	"strings"

	"codmtune/internal/sound"
	"codmtune/internal/tune"
	"codmtune/internal/ui"
	"codmtune/internal/win32"
)

func applyCustomAccent(base *ui.Theme, hex string) {
	col, ok := parseHexColor(hex)
	if !ok {
		return
	}
	r, g, b := col>>16&0xff, col>>8&0xff, col&0xff
	lum := 0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)
	if lum < 110 {
		f := 110 / (lum + 1)
		r = uint32(math.Min(255, float64(r)*f+30))
		g = uint32(math.Min(255, float64(g)*f+30))
		b = uint32(math.Min(255, float64(b)*f+30))
	}
	base.Primary = ui.RGB(uint8(r), uint8(g), uint8(b))
	if lum > 170 {
		base.OnPrimary = ui.RGB(0x10, 0x12, 0x10)
	} else {
		base.OnPrimary = ui.RGB(0xFF, 0xFF, 0xFF)
	}
	base.PrimaryDim = ui.RGB(uint8(r*68/100), uint8(g*68/100), uint8(b*68/100))
	if base.Dark {
		base.PrimaryDim = mixColor(base.Primary, base.Bg, 0.38)
		base.PrimaryBg = mixColor(base.Surface, base.Primary, 0.22)
	} else if base.Style == "ark" {
		base.OnPrimary = ui.RGB(0xFF, 0xFF, 0xFF)
		base.PrimaryDim = mixColor(base.Primary, base.Bg, 0.38)
		base.PrimaryBg = mixColor(base.Surface, base.Primary, 0.16)
	} else {
		base.PrimaryBg = ui.RGB(uint8(r*16/100), uint8(g*16/100), uint8(b*16/100))
	}
}

func parseHexColor(s string) (uint32, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

var fontChoices = []string{"MiSans", "Microsoft YaHei UI", "DengXian", "SimHei", "Segoe UI", "Arial"}

// applyTheme rebuilds the palette from settings: scheme, corner radius,
// advanced accent, font family, wallpaper and sound.
func mixColor(a, b ui.Color, t float64) ui.Color {
	ar, ag, ab := float64(a>>16&0xff), float64(a>>8&0xff), float64(a&0xff)
	br, bg, bb := float64(b>>16&0xff), float64(b>>8&0xff), float64(b&0xff)
	return ui.RGB(uint8(ar+(br-ar)*t), uint8(ag+(bg-ag)*t), uint8(ab+(bb-ab)*t))
}

// applyAccentPreset gives every style the same named accents. Ark used to
// ignore the presets entirely, which made the colour setting look broken.
func applyAccentPreset(base *ui.Theme, name string) {
	var c ui.Color
	switch name {
	case "blue":
		if base.Dark {
			c = ui.RGB(0xA9, 0xC7, 0xFF)
		} else {
			c = ui.RGB(0x3D, 0x6F, 0xD8)
		}
	case "mint":
		if base.Dark {
			c = ui.RGB(0x8B, 0xFF, 0xD9)
		} else {
			c = ui.RGB(0x1F, 0x8A, 0x70)
		}
	case "amber":
		if base.Dark {
			c = ui.RGB(0xFF, 0xD1, 0x66)
		} else {
			c = ui.RGB(0xB4, 0x76, 0x12)
		}
	case "violet":
		if base.Dark {
			c = ui.RGB(0xC9, 0xA7, 0xFF)
		} else {
			c = ui.RGB(0x6E, 0x45, 0xC7)
		}
	default: // red
		if base.Dark {
			c = ui.RGB(0xFF, 0x8A, 0x80)
		} else {
			c = ui.RGB(0xC9, 0x38, 0x38)
		}
	}
	base.Primary = c
	if base.Dark {
		base.OnPrimary = base.Bg
		base.PrimaryDim = mixColor(c, base.Bg, 0.38)
		base.PrimaryBg = mixColor(base.Surface, c, 0.22)
	} else {
		lum := 0.2126*float64(c>>16&0xff) + 0.7152*float64(c>>8&0xff) + 0.0722*float64(c&0xff)
		if lum > 170 {
			base.OnPrimary = ui.RGB(0x17, 0x16, 0x14)
		} else {
			base.OnPrimary = ui.RGB(0xFF, 0xFF, 0xFF)
		}
		base.PrimaryDim = mixColor(c, base.Bg, 0.38)
		base.PrimaryBg = mixColor(base.Surface, c, 0.16)
	}
}

// endfieldMark returns the single fill colour this style is allowed. Every
// value is a high-chroma block colour: on paper it must survive at 4.5:1
// behind ink type, and on board-black it must not turn into a pastel.
func endfieldMark(name, hex string) ui.Color {
	switch name {
	case "cyan":
		return ui.RGB(0x00, 0xE5, 0xFF)
	case "lime":
		return ui.RGB(0xC6, 0xFF, 0x00)
	case "orange":
		return ui.RGB(0xFF, 0x6B, 0x00)
	case "mint":
		return ui.RGB(0x00, 0xE0, 0x8A)
	case "custom":
		if v, ok := parseHexColor(hex); ok {
			return ui.RGB(uint8(v>>16&0xff), uint8(v>>8&0xff), uint8(v&0xff))
		}
	}
	return ui.RGB(0xFF, 0xFA, 0x00)
}

// applyEndfieldAccent keeps endfield honest: the accent is a fill only. Type,
// rules and the current-position trace stay ink on paper (and ink-white on
// board), because acid yellow on white is 1.1:1 and unusable as a line.
func applyEndfieldAccent(base *ui.Theme, name, hex string) {
	base.Mark = endfieldMark(name, hex)
	if base.Dark {
		base.Primary = ui.RGB(0xED, 0xED, 0xED)
		base.OnPrimary = ui.RGB(0x16, 0x16, 0x16)
		base.PrimaryDim = base.Mark // trace: the only yellow line allowed
		base.PrimaryBg = mixColor(base.Surface, base.Mark, 0.22)
		base.Warn = base.Mark
		base.WarnBg = mixColor(base.Surface, base.Mark, 0.30)
	} else {
		base.Primary = ui.RGB(0x16, 0x16, 0x16)
		base.OnPrimary = ui.RGB(0xFF, 0xFF, 0xFF)
		base.PrimaryDim = ui.RGB(0x16, 0x16, 0x16)
		base.PrimaryBg = mixColor(base.Surface, base.Mark, 0.20)
		base.Warn = ui.RGB(0x16, 0x16, 0x16)
		base.WarnBg = base.Mark
	}
}

func (s *screens) applyTheme() {
	cfg := s.c.Cfg()
	var base ui.Theme
	if cfg.Style == "endfield" {
		// Endfield has exactly two stocks: paper and board-black. The OLED
		// scheme would erase the ink rules that carry its geometry, so it is
		// folded into the dark stock instead of rendered as pure black.
		if cfg.Theme == "dark" || cfg.Theme == "oled" {
			base = ui.EndfieldDark
		} else {
			base = ui.Endfield
		}
	} else if cfg.Style == "ark" {
		if cfg.Theme == "dark" || cfg.Theme == "oled" {
			base = ui.ArkDark
		} else {
			base = ui.Ark
		}
	} else {
		switch cfg.Theme {
		case "light":
			base = ui.Light
		case "oled":
			base = ui.OLED
		default:
			base = ui.Dark
		}
	}
	base.Style = cfg.Style
	base.Dark = cfg.Theme == "dark" || cfg.Theme == "oled"
	base.Advanced = cfg.Advanced
	base.CardRadius = float32(cfg.Radius)
	if base.Industrial() {
		// Square plates are part of the industrial geometry, not a colour
		// preference: a rounded card would read as a different product.
		base.CardRadius = 0
	}
	if base.Style == "ark" {
		base.CardRadius = 2
	}
	if base.Style == "endfield" {
		// Endfield has one accent and it is a fill colour, never a line or a
		// glyph. The preset therefore paints Mark; ink type stays ink.
		applyEndfieldAccent(&base, cfg.Accent, cfg.AccentHex)
	} else if cfg.Accent == "custom" {
		applyCustomAccent(&base, cfg.AccentHex)
	} else {
		applyAccentPreset(&base, cfg.Accent)
	}
	if cfg.Advanced {
		// Advanced mode is a deliberate warning surface: red signal colours,
		// hazard bands and doubled rules, not merely a hidden checkbox.
		base.Advanced = true
		if base.Style == "endfield" {
			// Endfield keeps its yellow for here/act/look and spends red only
			// on the destructive rows; two competing signal colours would
			// make the plate read as decoration instead of warning.
			base.Outline2 = base.Danger
			base.DangerBg = mixColor(base.Surface, base.Danger, 0.22)
			base.Surface2 = mixColor(base.Surface, base.Danger, 0.06)
		} else if base.Dark {
			base.Primary = ui.RGB(0xFF, 0x8B, 0x7A)
			base.OnPrimary = ui.RGB(0x1A, 0x0C, 0x0B)
			base.PrimaryDim = ui.RGB(0xF2, 0x9A, 0x8C)
			base.PrimaryBg = ui.RGB(0x4A, 0x22, 0x1D)
			base.Surface = ui.RGB(0x24, 0x21, 0x1E)
			base.Surface2 = ui.RGB(0x2E, 0x29, 0x24)
			base.Outline = ui.RGB(0x5A, 0x43, 0x3D)
			base.Outline2 = ui.RGB(0x7A, 0x5A, 0x52)
			base.Muted = ui.RGB(0xC0, 0xB4, 0xA5)
		} else {
			base.Primary = ui.RGB(0xC5, 0x3A, 0x3A)
			base.OnPrimary = ui.RGB(0xFF, 0xFF, 0xFF)
			base.PrimaryDim = ui.RGB(0x8C, 0x28, 0x28)
			base.PrimaryBg = ui.RGB(0xF7, 0xDA, 0xDA)
		}
	}
	ui.SetTheme(&base)
	s.app.UseTheme(ui.CurrentTheme)
	s.app.BgImage = cfg.Background
	s.app.GridImage = ensureArkGrid(&base, cfg.Background != "")
	s.app.BgDim = 214
	if cfg.Font != "" {
		if err := win32.SetFontFamily(cfg.Font); err != nil {
			s.c.LogLine("字体不可用，回退微软雅黑：" + cfg.Font)
			_ = win32.SetFontFamily("Microsoft YaHei UI")
		}
	}
	sound.Init(cfg.SoundVol)
	sound.SetEnabled(cfg.Sound)
	s.app.OnSound = func(kind string) {
		cfg := s.c.Cfg()
		if !cfg.Sound {
			return
		}
		idx := -1
		switch kind {
		case "startup":
			idx = 0
		case "move", "press":
			idx = 1
		case "confirm", "tray":
			idx = 2
		case "launch":
			idx = 3
		}
		if idx >= 0 && !tune.SoundKindOn(cfg.SoundKinds, idx) {
			return
		}
		sound.Play(kind)
	}
}

// retheme applies the palette and rebuilds the current screen so every row
// picks up the new colours, radius and font.
func (s *screens) retheme() {
	s.applyTheme()
	s.switchTo(s.active)
}

// Retheme re-applies the palette after the caller changed style/theme/accent
// in the config. It exists for headless tools (preview renders) that drive the
// real screens without going through the settings UI.
func Retheme(a *ui.App, c *tune.Control) {
	s := &screens{app: a, c: c, active: "overview"}
	s.retheme()
}

// enabledCount drives the "too many features" hint.
func (s *screens) enabledCount() int {
	cfg := s.c.Cfg()
	n := 0
	for _, on := range []bool{cfg.PowerEnabled, cfg.ScheduleEnabled, cfg.ACEEnabled, cfg.BGEnabled,
		cfg.OverlayEnabled, cfg.MouseAccelOff, cfg.OverlayEnabled && cfg.OverlayServer} {
		if on {
			n++
		}
	}
	for _, on := range cfg.Tweaks {
		if on {
			n++
		}
	}
	return n
}
