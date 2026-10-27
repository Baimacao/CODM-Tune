package appui

import (
	"fmt"
	"os/exec"
	"strings"

	"codmtune/internal/crosshair"
	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

func clipboardText() string {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		"[Console]::OutputEncoding=[Text.Encoding]::UTF8; Get-Clipboard -Raw")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (s *screens) settingsFromConfig(c tune.Config) crosshair.Settings {
	return crosshair.Settings{
		Style: c.CrosshairStyle, Length: c.CrosshairLength, Thickness: c.CrosshairThickness,
		Gap: c.CrosshairGap, Color: c.CrosshairColor, Alpha: c.CrosshairAlpha,
		Dot: c.CrosshairDot, Outline: c.CrosshairOutline,
		OffsetX: c.CrosshairOffsetX, OffsetY: c.CrosshairOffsetY,
	}
}

func (s *screens) syncCrosshair() {
	c := s.c.Cfg()
	_ = crosshair.Write(s.settingsFromConfig(c), c.CrosshairEnabled)
	crosshair.EnsureChild(c.CrosshairEnabled)
}

func (s *screens) importCrosshair() {
	text := clipboardText()
	if strings.TrimSpace(text) == "" {
		s.c.LogLine("准心导入：剪贴板为空")
		return
	}
	p, err := crosshair.Parse(text)
	if err != nil {
		s.c.LogLine("准心导入失败：" + err.Error())
		return
	}
	s.c.Update(func(c *tune.Config) {
		c.CrosshairStyle = p.Style
		c.CrosshairLength = p.Length
		c.CrosshairThickness = p.Thickness
		c.CrosshairGap = p.Gap
		c.CrosshairColor = p.Color
		c.CrosshairAlpha = p.Alpha
		c.CrosshairDot = p.Dot
		c.CrosshairOutline = p.Outline
		c.CrosshairOffsetX = p.OffsetX
		c.CrosshairOffsetY = p.OffsetY
		c.CrosshairCode = p.Source
		c.CrosshairEnabled = true
	})
	s.c.LogLine("准心导入成功：" + p.Source)
	s.syncCrosshair()
	s.switchTo("crosshair")
}

func (s *screens) crosshairChoices(parent, title, sub, current string, opts []choiceOption, key string) {
	s.choose(parent, title, sub, current, opts, func(v string) {
		s.c.Update(func(c *tune.Config) {
			switch key {
			case "style":
				c.CrosshairStyle = v
			case "color":
				c.CrosshairColor = v
			}
		})
		s.syncCrosshair()
	})
}

func (s *screens) crosshairCat() *ui.Node {
	root := s.cat("crosshair", "辅助准心", "独立透明准心；支持 CS:GO / CS2 / Valorant 码与逐项微调")
	l := s.listIn(root)
	cfg := s.c.Cfg()

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "scope", title: "启用准心",
		desc: "置顶、鼠标穿透的独立窗口；不改游戏文件、不读游戏内存。取消后立即消失。",
		value: func() string {
			if s.c.Cfg().CrosshairEnabled {
				return "显示中 · 居中"
			}
			return "已关闭"
		},
		toggle: func() bool { return s.c.Cfg().CrosshairEnabled },
		onToggle: func(v bool) {
			s.c.Update(func(c *tune.Config) { c.CrosshairEnabled = v })
			s.syncCrosshair()
		},
	}))

	s.section(l, "导入与形状")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "window", title: "从剪贴板导入",
		desc: "先复制 CSGO-… / CS2 分享码、Valorant 准心码、cl_crosshair 参数或 JSON，再按 Enter。",
		value: func() string {
			v := s.c.Cfg().CrosshairCode
			if v == "" {
				return "等待剪贴板"
			}
			return "已导入 · " + v
		},
		open: s.importCrosshair,
		hint: "Enter 读取剪贴板",
	}))
	style := cfg.CrosshairStyle
	if style == "" {
		style = "cross"
	}
	styleLabel := map[string]string{"cross": "十字", "dot": "点", "circle": "圆环", "t": "T 形"}[style]
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "scope", title: "准心形状",
		desc: "十字 / 点 / 圆环 / T 形；导入码会覆盖形状。",
		value: func() string {
			v := s.c.Cfg().CrosshairStyle
			return map[string]string{"cross": "十字", "dot": "点", "circle": "圆环", "t": "T 形"}[v]
		},
		open: func() {
			s.crosshairChoices("crosshair", "准心形状", "选择准心的基础几何", style, []choiceOption{
				{"cross", "十字", "四向线段，适合步枪"},
				{"dot", "点", "单点，视野最干净"},
				{"circle", "圆环", "环形轮廓，适合喷子/近战"},
				{"t", "T 形", "上方与左右，留出下半区"},
			}, "style")
		},
		hint: "Enter 展开",
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "monitor", title: "颜色",
		desc:  "预设 + 导入码 RGB；描边保证亮背景也看得清。",
		value: func() string { return s.c.Cfg().CrosshairColor + " · " + styleLabel },
		open: func() {
			s.crosshairChoices("crosshair", "准心颜色", "选择高对比预设；自定义可在 JSON 中写 #RRGGBB", s.c.Cfg().CrosshairColor, []choiceOption{
				{"#39FF14", "荧光绿", "深色场景最醒目"},
				{"#00E5FF", "青色", "与 UI 青色一致"},
				{"#FF3B3B", "红色", "与强调色一致"},
				{"#FFFFFF", "白色", "中性、干净"},
				{"#FFD400", "琥珀", "暖色高对比"},
			}, "color")
		},
		hint: "Enter 展开",
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 86), feature{
		icon: "monitor", title: "线段长度",
		desc:  "← → 调节，范围 1–64 px；导入值会映射到此范围。",
		value: func() string { return fmt.Sprintf("%d px", s.c.Cfg().CrosshairLength) },
		adjust: func(d int) {
			s.c.Update(func(c *tune.Config) { c.CrosshairLength = clampInt(c.CrosshairLength+d*2, 1, 64) })
			s.syncCrosshair()
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 86), feature{
		icon: "monitor", title: "线宽 / 中心间隙",
		desc: "← → 同时调整线宽与间隙；细线适合远距离，粗线适合近战。",
		value: func() string {
			c := s.c.Cfg()
			return fmt.Sprintf("%d px / %d px", c.CrosshairThickness, c.CrosshairGap)
		},
		adjust: func(d int) {
			s.c.Update(func(c *tune.Config) {
				c.CrosshairThickness = clampInt(c.CrosshairThickness+(d+1)/2, 1, 12)
				c.CrosshairGap = clampInt(c.CrosshairGap+d, 0, 40)
			})
			s.syncCrosshair()
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 86), feature{
		icon: "monitor", title: "不透明度",
		desc:  "← → 调节 40–255；在明亮场景可降到 120 左右。",
		value: func() string { return fmt.Sprintf("%d / 255", s.c.Cfg().CrosshairAlpha) },
		adjust: func(d int) {
			s.c.Update(func(c *tune.Config) { c.CrosshairAlpha = clampInt(c.CrosshairAlpha+d*10, 40, 255) })
			s.syncCrosshair()
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 86), feature{
		icon: "monitor", title: "中心点 / 黑色描边",
		desc: "中心点便于腰射；描边在浅色墙面和天空上保持可读。",
		value: func() string {
			c := s.c.Cfg()
			return fmt.Sprintf("中心点 %s · 描边 %s", onOff(c.CrosshairDot), onOff(c.CrosshairOutline))
		},
		open: func() {
			s.c.Update(func(c *tune.Config) {
				c.CrosshairDot = !c.CrosshairDot
				c.CrosshairOutline = !c.CrosshairOutline
			})
			s.syncCrosshair()
		},
		hint: "Enter 同时切换",
	}))

	s.section(l, "位置与恢复")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 86), feature{
		icon: "mouse", title: "水平 / 垂直偏移",
		desc: "← → 微调屏幕中心；0/0 为绝对中心，适配准星不居中的游戏。",
		value: func() string {
			c := s.c.Cfg()
			return fmt.Sprintf("X %+d · Y %+d", c.CrosshairOffsetX, c.CrosshairOffsetY)
		},
		adjust: func(d int) {
			s.c.Update(func(c *tune.Config) { c.CrosshairOffsetY = clampInt(c.CrosshairOffsetY+d*2, -200, 200) })
			s.syncCrosshair()
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 86), feature{
		icon: "shield", title: "恢复默认",
		desc:  "恢复荧光绿十字、中心点、黑色描边；不会改动游戏设置。",
		value: func() string { return "准心专用" },
		open: func() {
			d := crosshair.Default()
			s.c.Update(func(c *tune.Config) {
				c.CrosshairStyle = d.Style
				c.CrosshairLength = d.Length
				c.CrosshairThickness = d.Thickness
				c.CrosshairGap = d.Gap
				c.CrosshairColor = d.Color
				c.CrosshairAlpha = d.Alpha
				c.CrosshairDot = d.Dot
				c.CrosshairOutline = d.Outline
				c.CrosshairOffsetX = 0
				c.CrosshairOffsetY = 0
				c.CrosshairCode = "默认"
			})
			s.syncCrosshair()
		},
		hint: "Enter 恢复",
	}))
	return root
}

func onOff(v bool) string {
	if v {
		return "开"
	}
	return "关"
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
