// Package crosshair parses portable crosshair descriptions into the small
// renderer model used by the overlay. It never touches game files or memory.
package crosshair

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// Settings is the renderer's normalized crosshair model.
type Settings struct {
	Style       string `json:"style"`
	Length      int    `json:"length"`
	Thickness   int    `json:"thickness"`
	Gap         int    `json:"gap"`
	Color       string `json:"color"`
	Alpha       int    `json:"alpha"`
	Dot         bool   `json:"dot"`
	Outline     bool   `json:"outline"`
	OffsetX     int    `json:"offset_x"`
	OffsetY     int    `json:"offset_y"`
	Source      string `json:"source"`
	Raw         string `json:"raw"`
}

// Default returns a visible, non-invasive starting point.
func Default() Settings {
	return Settings{Style: "cross", Length: 10, Thickness: 2, Gap: 5,
		Color: "#39FF14", Alpha: 230, Dot: true, Outline: true}
}

// Parse accepts JSON, CS:GO / CS2 share codes, Valorant profile strings, or
// a block of cl_crosshair console variables.
func Parse(text string) (Settings, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Default(), fmt.Errorf("剪贴板为空")
	}
	s := Default()
	s.Raw = text
	switch {
	case strings.HasPrefix(text, "{"):
		if err := json.Unmarshal([]byte(text), &s); err != nil {
			return s, fmt.Errorf("JSON 格式错误：%v", err)
		}
		s.Source = "JSON"
	case strings.HasPrefix(text, "CSGO-"):
		if err := decodeCS(text, &s); err != nil {
			return s, err
		}
		s.Source = "CS:GO / CS2"
	case strings.HasPrefix(text, "CS") && len(text) == 46:
		if err := decodeCS2New(text, &s); err != nil {
			return s, err
		}
		s.Source = "CS2"
	case strings.Contains(text, "cl_crosshair"):
		if err := parseConsole(text, &s); err != nil {
			return s, err
		}
		s.Source = "CS 控制台"
	case strings.HasPrefix(text, "0;") || strings.Contains(text, ";P;"):
		if err := parseValorant(text, &s); err != nil {
			return s, err
		}
		s.Source = "Valorant"
	default:
		return s, fmt.Errorf("无法识别：请粘贴 JSON、CSGO-/CS 分享码、Valorant 准心码或 cl_crosshair 参数")
	}
	s.normalize()
	return s, nil
}

func (s *Settings) normalize() {
	if s.Style != "cross" && s.Style != "dot" && s.Style != "circle" && s.Style != "t" {
		s.Style = "cross"
	}
	if s.Length < 1 {
		s.Length = 1
	}
	if s.Length > 64 {
		s.Length = 64
	}
	if s.Thickness < 1 {
		s.Thickness = 1
	}
	if s.Thickness > 12 {
		s.Thickness = 12
	}
	if s.Gap < 0 {
		s.Gap = 0
	}
	if s.Gap > 40 {
		s.Gap = 40
	}
	if s.Alpha < 40 {
		s.Alpha = 40
	}
	if s.Alpha > 255 {
		s.Alpha = 255
	}
	if len(s.Color) != 7 || s.Color[0] != '#' {
		s.Color = "#39FF14"
	}
}

const alphabet = "ABCDEFGHJKLMNOPQRSTUVWXYZabcdefhijkmnopqrstuvwxyz23456789"

var codePattern = regexp.MustCompile(`^CSGO-(.{5})-(.{5})-(.{5})-(.{5})-(.{5})$`)

func decode57(code string) (*big.Int, error) {
	v := new(big.Int)
	rs := []rune(code)
	for i := len(rs) - 1; i >= 0; i-- {
		d := strings.IndexRune(alphabet, rs[i])
		if d < 0 {
			return nil, fmt.Errorf("分享码含非法字符 %q", rs[i])
		}
		v.Mul(v, big.NewInt(57))
		v.Add(v, big.NewInt(int64(d)))
	}
	return v, nil
}

func decodeCS(code string, s *Settings) error {
	m := codePattern.FindStringSubmatch(code)
	if m == nil {
		return fmt.Errorf("CS:GO 分享码格式不正确")
	}
	v, err := decode57(strings.Join(m[1:], ""))
	if err != nil {
		return err
	}
	if v.BitLen() > 144 {
		return fmt.Errorf("分享码超出范围")
	}
	b := make([]byte, 18)
	be := v.Bytes()
	copy(b[len(b)-len(be):], be)
	var sum byte
	for _, x := range b[1:] {
		sum += x
	}
	if b[0] != sum {
		return fmt.Errorf("分享码校验失败")
	}
	if b[1] != 3 && b[1] != 4 {
		return fmt.Errorf("暂不支持该 CS 版本（v%d）", b[1])
	}
	packed := uint32(b[10]) | uint32(b[11])<<8 | uint32(b[12])<<16 | uint32(b[13])<<24
	s.Style = "cross"
	if b[2]&0x40 != 0 {
		s.Dot = true
	}
	if b[2]&0x80 != 0 {
		s.Style = "t"
	}
	s.Color = fmt.Sprintf("#%02X%02X%02X", b[3], b[4], b[5])
	s.Alpha = int(b[6])
	s.Gap = int(b[7])
	s.Length = int(b[8])
	s.Thickness = int((packed >> 23) & 31)
	s.Outline = b[1] == 4 && (packed>>28)&3 != 0 || b[2]&0x20 != 0
	return nil
}

func decodeCS2New(code string, s *Settings) error {
	v, err := decode57(code[2:])
	if err != nil {
		return err
	}
	if v.BitLen() > 256 {
		return fmt.Errorf("分享码超出范围")
	}
	b := make([]byte, 32)
	be := v.Bytes()
	copy(b[len(b)-len(be):], be)
	var sum byte
	for _, x := range b[1:] {
		sum += x
	}
	if b[0] != sum {
		return fmt.Errorf("分享码校验失败")
	}
	if b[1] != 1 {
		return fmt.Errorf("暂不支持该 CS2 版本（v%d）", b[1])
	}
	if b[15] != 0 {
		return fmt.Errorf("该 CS2 码含尚未支持的动态参数")
	}
	s.Style = "cross"
	if b[4]&0x40 != 0 {
		s.Dot = true
	}
	if b[4]&0x80 != 0 {
		s.Style = "t"
	}
	s.Color = fmt.Sprintf("#%02X%02X%02X", b[5], b[6], b[7])
	s.Alpha = int(b[8])
	s.Thickness = int(b[13] & 63)
	s.Gap = int(b[14])
	s.Length = int(b[16])
	s.Outline = b[13]>>6 != 0
	return nil
}

var consoleRe = regexp.MustCompile(`(?i)cl_crosshair([a-z0-9_]+)\s+([-0-9.]+)`)

func parseConsole(text string, s *Settings) error {
	seen := false
	for _, m := range consoleRe.FindAllStringSubmatch(text, -1) {
		key := strings.ToLower(m[1])
		val := strings.ToLower(m[2])
		n, _ := strconv.Atoi(m[2])
		seen = true
		switch key {
		case "size", "length":
			s.Length = n
		case "thickness":
			s.Thickness = n
		case "gap":
			s.Gap = n
		case "dot":
			s.Dot = val == "1" || val == "true"
		case "drawoutline":
			s.Outline = val != "0" && val != "false"
		case "color":
			// Old CS color index; preserve a readable default rather than
			// pretending the index is an RGB value.
			switch val {
			case "0":
				s.Color = "#FF0000"
			case "1":
				s.Color = "#00FF00"
			case "2":
				s.Color = "#FFFF00"
			case "3":
				s.Color = "#0000FF"
			case "4":
				s.Color = "#00FFFF"
			}
		case "color_r":
			s.Color = setRGB(s.Color, 0, n)
		case "color_g":
			s.Color = setRGB(s.Color, 1, n)
		case "color_b":
			s.Color = setRGB(s.Color, 2, n)
		case "color_a":
			s.Alpha = n
		}
	}
	if !seen {
		return fmt.Errorf("没有找到 cl_crosshair 参数")
	}
	return nil
}

func parseValorant(text string, s *Settings) error {
	parts := strings.Split(text, ";")
	if len(parts) < 4 {
		return fmt.Errorf("Valorant 准心码格式不正确")
	}
	kv := map[string]string{}
	for i := 0; i+1 < len(parts); i += 2 {
		kv[strings.ToLower(parts[i])] = parts[i+1]
	}
	seen := false
	for k, v := range kv {
		switch k {
		case "u":
			if len(v) >= 6 {
				s.Color, seen = "#"+strings.ToUpper(v[:6]), true
			}
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			continue
		}
		switch k {
		case "0t":
			s.Thickness, seen = n, true
		case "0l":
			s.Length, seen = n*2, true
		case "0o":
			s.Gap, seen = n, true
		case "0a":
			s.Alpha, seen = n*255/10, true
		case "d":
			s.Dot, seen = n != 0, true
		case "o":
			s.Outline, seen = n != 0, true
		}
	}
	if !seen {
		return fmt.Errorf("未识别到 Valorant 准心字段")
	}
	s.Style = "cross"
	if s.Dot && s.Length <= 1 {
		s.Style = "dot"
	}
	return nil
}

func setRGB(hex string, idx, v int) string {
	if len(hex) != 7 {
		hex = "#FFFFFF"
	}
	b := []byte(hex[1:])
	pos := idx * 2
	if pos+2 > len(b) {
		return hex
	}
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	b[pos] = "0123456789ABCDEF"[v>>4]
	b[pos+1] = "0123456789ABCDEF"[v&15]
	return "#" + strings.ToUpper(string(b))
}
