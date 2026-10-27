package appui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"codmtune/internal/tune"
	"codmtune/internal/ui"
	"codmtune/internal/win32"
)

func (s *screens) settingsCat() *ui.Node {
	root := s.cat("settings", "设置", "外观、字体、圆角、背景、音效与高级模式")
	l := s.listIn(root)
	cfg := s.c.Cfg()

	s.section(l, "外观")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "shield", title: "界面风格",
		desc: "标准 MD3E / 舟味工业 / 终末地终端：三种几何语言，不只是换色。",
		value: func() string {
			switch s.c.Cfg().Style {
			case "ark":
				return "舟味 · 工业终端"
			case "endfield":
				return "终末地 · 操作台"
			}
			return "标准 · MD3E"
		},
		hint: "Enter 展开",
		open: func() {
			s.choose("settings", "界面风格", "风格会改变几何与装饰，不只是换色", s.c.Cfg().Style, []choiceOption{
				{"standard", "标准 MD3E", "圆角、弹簧、lilt 动效"},
				{"ark", "舟味 · 工业终端", "方角、编号、警示条、网格扫描线"},
				{"endfield", "终末地 · 操作台", "纸白/板黑、切角、单一强调色只作填充"},
			}, func(v string) {
				s.c.Update(func(c *tune.Config) { c.Style = v })
				s.retheme()
			})
		},
	}))

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "gauge", title: "强调色",
		desc:  "展开二级菜单；终末地风格里强调色只作填充块，不作线条或文字。",
		value: func() string { return accentName(s.c.Cfg()) },
		hint:  "Enter 展开",
		open: func() {
			cur := s.c.Cfg().Accent
			s.choose("settings", "强调色", "选择主色；选择壁纸色会覆盖预设", cur, []choiceOption{
				{"blue", "蓝", "默认 MD3E 蓝"},
				{"mint", "薄荷", "OLED Astra 的薄荷绿"},
				{"amber", "琥珀", "工业黄，适合深色"},
				{"violet", "紫", "低饱和紫"},
				{"red", "红", "危险/高级模式"},
				{"cyan", "青", "终末地的技术青"},
				{"lime", "酸柠", "高亮填充，用作当前位置"},
				{"orange", "橙", "警示块"},
				{"custom", "壁纸色", "从当前桌面壁纸取色"},
			}, func(v string) {
				hex := ""
				if v == "custom" {
					hex, _ = accentFromWallpaper()
				}
				s.c.Update(func(c *tune.Config) {
					c.Accent = v
					if v != "custom" {
						c.AccentHex = ""
					} else if hex != "" {
						c.AccentHex = hex
					}
				})
				s.retheme()
			})
		},
	}))

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "monitor", title: "从壁纸取色",
		desc: "读取当前壁纸的中间调彩色像素，生成可读的强调色；只读图片，不改壁纸。",
		value: func() string {
			h := s.c.Cfg().AccentHex
			if h == "" {
				return "未取色"
			}
			return h
		},
		open: func() {
			if hex, ok := accentFromWallpaper(); ok {
				s.c.Update(func(c *tune.Config) { c.Accent = "custom"; c.AccentHex = hex })
				s.retheme()
				s.c.LogLine("取色：壁纸 → " + hex)
			} else {
				s.c.LogLine("取色失败：未找到可读壁纸")
			}
			s.switchTo("settings")
		},
	}))

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "monitor", title: "主题",
		desc: "舟味支持纸灰与墨黑；终末地只有纸白与板黑两套；标准模式使用 MD3E / Astra 色板。",
		value: func() string {
			cfg := s.c.Cfg()
			if cfg.Style == "endfield" {
				if cfg.Theme == "light" {
					return "纸白 · STOCK"
				}
				return "板黑 · BOARD"
			}
			if cfg.Style == "ark" {
				if cfg.Theme == "light" {
					return "纸灰 · ARK LIGHT"
				}
				return "墨黑 · ARK DARK"
			}
			switch cfg.Theme {
			case "light":
				return "浅色"
			case "oled":
				return "OLED Astra"
			}
			return "深色"
		},
		hint: "Enter 展开",
		open: func() {
			cfg := s.c.Cfg()
			// The stock names differ per style because the palettes are not the
			// same thing: ark has a paper-grey and an ink night, endfield has
			// only stock-white and board-black, MD3E has three. Offering the
			// shared list here would label a palette with another family's name.
			opts := []choiceOption{
				{"dark", "墨黑 / 深色", "高对比墨黑；舟味为工业夜场"},
				{"light", "纸灰 / 浅色", "纸张灰白 + 深色文字"},
				{"oled", "OLED Astra", "纯黑 + 薄荷，参考 Astra"},
			}
			if cfg.Style == "endfield" {
				opts = []choiceOption{
					{"dark", "板黑 · BOARD", "板黑底 + 墨白字；强调色只做填充"},
					{"light", "纸白 · STOCK", "纸白底 + 黑字；表格线用墨"},
				}
			} else if cfg.Style == "ark" {
				opts = []choiceOption{
					{"dark", "墨黑 · ARK DARK", "工业夜场；深底 + 危险红"},
					{"light", "纸灰 · ARK LIGHT", "纸张灰白；等宽标签与斜切板"},
				}
			}
			s.choose("settings", "主题", "选择基础色板", cfg.Theme, opts, func(v string) {
				s.c.Update(func(c *tune.Config) { c.Theme = v })
				s.retheme()
			})
		},
	}))

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "book", title: "界面字体",
		desc:  "展开二级菜单选择；缺字时回退微软雅黑。",
		value: func() string { return s.c.Cfg().Font },
		hint:  "Enter 展开",
		open: func() {
			opts := make([]choiceOption, 0, len(fontChoices))
			for _, f := range fontChoices {
				opts = append(opts, choiceOption{f, f, "系统字体"})
			}
			s.choose("settings", "界面字体", "选择后立即重建字型", s.c.Cfg().Font, opts, func(v string) {
				s.c.Update(func(c *tune.Config) { c.Font = v })
				s.retheme()
			})
		},
	}))

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "window", title: "横屏全屏（F11）",
		desc: "进入 FIELD TERMINAL 全屏终端；F11 返回原页面。",
		value: func() string {
			if win32.Fullscreen() {
				return "已开启"
			}
			return "关闭"
		},
		hint: "Enter 切换",
		open: func() { s.toggleFullscreen() },
	}))

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "window", title: "圆角曲率",
		desc: "标准模式的卡片半径；舟味使用固定的 2px 方角作为几何语言。",
		value: func() string {
			if s.c.Cfg().Style == "ark" {
				return "2 px · 方角（舟味固定）"
			}
			return fmt.Sprintf("%.0f px", s.c.Cfg().Radius)
		},
		disabled: func() bool { return s.c.Cfg().Style == "ark" },
		hint:     "Enter 展开",
		open: func() {
			s.choose("settings", "圆角曲率", "选择卡片半径", fmt.Sprintf("%.0f", s.c.Cfg().Radius), []choiceOption{
				{"14", "14 px", "紧凑"},
				{"18", "18 px", "偏硬"},
				{"22", "22 px", "默认"},
				{"26", "26 px", "柔和"},
				{"30", "30 px", "最圆"},
			}, func(v string) {
				var f float64
				fmt.Sscanf(v, "%f", &f)
				s.c.Update(func(c *tune.Config) { c.Radius = f })
				s.retheme()
			})
		},
	}))

	bg := filepath.Base(cfg.Background)
	if bg == "." || cfg.Background == "" {
		bg = "无"
	}
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "monitor", title: "背景图片",
		desc:  "从“图片”文件夹或配置目录的 backgrounds 中选择；会加深色遮罩保证可读性。",
		value: func() string { return bg },
		open:  func() { s.app.Push(s.backgroundPicker()) },
	}))

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "book", title: "打开背景文件夹",
		desc:  "把图片放进该目录后回到上一页重新选择；支持 png/jpg/jpeg/bmp/webp。",
		value: func() string { return filepath.Join(tune.ConfigPath(), "backgrounds") },
		open: func() {
			dir := filepath.Join(tune.ConfigPath(), "backgrounds")
			_ = os.MkdirAll(dir, 0o755)
			_ = openPath(dir)
		},
	}))

	s.section(l, "高级")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "shield", title: "高级模式",
		desc: "显示会修改系统或 HKLM 的危险项，并切换为红色强调色与更专业的文案。",
		value: func() string {
			if s.c.Cfg().Advanced {
				return "已开启 · 危险项可见"
			}
			return "关闭 · 只显示安全项"
		},
		tone: func() tune.Tone {
			if s.c.Cfg().Advanced {
				return tune.ToneBad
			}
			return tune.ToneNeutral
		},
		toggle: func() bool { return s.c.Cfg().Advanced },
		onToggle: func(on bool) {
			s.c.Update(func(c *tune.Config) { c.Advanced = on })
			s.retheme()
			s.app.HazardFlash()
		},
	}))

	s.section(l, "性能")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "gauge", title: "静止重绘频率",
		desc: "没有动画时的刷新间隔。调低到 10 Hz 可显著减少空转 CPU；界面不会因此变慢，因为任何输入都会立刻恢复全速。",
		value: func() string {
			return fmt.Sprintf("%d Hz · 约 %d ms/帧", s.c.Cfg().IdleFPS, 1000/s.c.Cfg().IdleFPS)
		},
		adjust: func(d int) {
			s.c.Update(func(c *tune.Config) {
				c.IdleFPS += d * 5
				if c.IdleFPS < 10 {
					c.IdleFPS = 10
				}
				if c.IdleFPS > 60 {
					c.IdleFPS = 60
				}
			})
		},
		progress: func() float64 { return float64(s.c.Cfg().IdleFPS-10) / 50 },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "gamepad", title: "游戏时重绘频率",
		desc: "CODM 运行时的静止频率。CPU 很敏感时设 5–10 Hz：本工具只在内容变化时重绘，静止时不该占用核心。",
		value: func() string {
			return fmt.Sprintf("%d Hz · 约 %d ms/帧", s.c.Cfg().GameIdleFPS, 1000/s.c.Cfg().GameIdleFPS)
		},
		adjust: func(d int) {
			s.c.Update(func(c *tune.Config) {
				c.GameIdleFPS += d * 5
				if c.GameIdleFPS < 5 {
					c.GameIdleFPS = 5
				}
				if c.GameIdleFPS > 30 {
					c.GameIdleFPS = 30
				}
			})
		},
		progress: func() float64 { return float64(s.c.Cfg().GameIdleFPS-5) / 25 },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "bolt", title: "进程优先级",
		desc: "本工具始终以 BelowNormal 运行，并让出 CPU 0；这里只显示当前状态。",
		value: func() string {
			return "BelowNormal · 让出 CPU 0"
		},
		info: true,
	}))

	s.section(l, "音效")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "gamepad", title: "界面音效",
		desc:   "合成短音总开关；下面四类可单独关闭。不加载外部音频文件。",
		value:  func() string { return fmt.Sprintf("音量 %d%%", s.c.Cfg().SoundVol) },
		toggle: func() bool { return s.c.Cfg().Sound },
		onToggle: func(on bool) {
			s.c.Update(func(c *tune.Config) { c.Sound = on })
			s.retheme()
		},
	}))

	soundRows := []struct {
		icon, title, desc string
		idx               int
	}{
		{"power", "启动音", "冷启动握手完成后播放一次；关闭后启动静音。", 0},
		{"cursor", "选择音", "焦点在卡片或侧栏之间移动时的轻提示。", 1},
		{"check", "确认音", "Enter 执行按钮、切换开关时的确认。", 2},
		{"play", "游戏启动音", "点击启动 CODM 后播放的低频提示。", 3},
	}
	for i := range soundRows {
		row := soundRows[i]
		l.Add(s.featureRow(ui.R(PadX, 0, RowW, 76), feature{
			icon: row.icon, title: row.title, desc: row.desc,
			value: func() string {
				if tune.SoundKindOn(s.c.Cfg().SoundKinds, row.idx) {
					return "开"
				}
				return "关"
			},
			toggle: func() bool { return tune.SoundKindOn(s.c.Cfg().SoundKinds, row.idx) },
			onToggle: func(bool) {
				s.c.ToggleSoundKind(row.idx)
				s.retheme()
			},
		}))
	}

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "gauge", title: "音效音量",
		desc:  "展开二级菜单选择音量档。",
		value: func() string { return fmt.Sprintf("%d%%", s.c.Cfg().SoundVol) },
		hint:  "Enter 展开",
		open: func() {
			s.choose("settings", "音效音量", "选择提示音音量", fmt.Sprintf("%d", s.c.Cfg().SoundVol), []choiceOption{
				{"0", "0%", "静音"},
				{"20", "20%", "很轻"},
				{"40", "40%", "轻"},
				{"55", "55%", "默认"},
				{"70", "70%", "响"},
				{"85", "85%", "很响"},
				{"100", "100%", "最大"},
			}, func(v string) {
				var n int
				fmt.Sscanf(v, "%d", &n)
				s.c.Update(func(c *tune.Config) { c.SoundVol = n })
				s.retheme()
			})
		},
	}))

	s.section(l, "文件")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "book", title: "打开配置目录",
		desc:  "config.json、日志、注册表备份、回放备份与 knowledge/ 都在这里。",
		value: func() string { return tune.ConfigPath() },
		open:  func() { _ = openPath(tune.ConfigPath()) },
	}))
	return root
}

func accentName(c tune.Config) string {
	if c.Style == "endfield" {
		// In endfield the accent is one fill colour, and only five presets
		// map to a real one; every other name falls through to the default
		// acid yellow. Showing "蓝" while the UI paints yellow is a lie, so
		// the label comes from the same table the renderer uses.
		switch c.Accent {
		case "cyan":
			return "技术青"
		case "lime":
			return "酸柠"
		case "orange":
			return "橙"
		case "mint":
			return "薄荷"
		case "custom":
			if c.AccentHex != "" {
				return "壁纸色 " + c.AccentHex
			}
			return "壁纸色"
		}
		return "酸柠黄（默认）"
	}
	switch c.Accent {
	case "mint":
		return "薄荷"
	case "amber":
		return "琥珀"
	case "violet":
		return "紫"
	case "red":
		return "红"
	case "cyan":
		return "青"
	case "lime":
		return "酸柠"
	case "orange":
		return "橙"
	case "custom":
		if c.AccentHex != "" {
			return "壁纸色 " + c.AccentHex
		}
		return "壁纸色"
	}
	return "蓝"
}

// backgroundPicker lists usable images plus a "none" row.
func (s *screens) backgroundPicker() *ui.Node {
	root := s.cat("settings", "选择背景", "支持 png / jpg / jpeg / bmp；选择后立即生效")
	l := s.listIn(root)
	cfg := s.c.Cfg()

	none := ui.NewNode(ui.R(PadX, 0, RowW, 64))
	none.Focusable = true
	none.SetCursorRadius(14)
	none.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		th := ui.CurrentTheme
		card(c, r, n.Press.Get(), th.Surface)
		c.Text(ui.R(r.X+18, r.Y, r.W-36, r.H), "无背景（使用纯色）", 14, th.Text, ui.AlignLeft, false)
	}
	none.Activate = func(*ui.Node) {
		s.c.Update(func(c *tune.Config) { c.Background = "" })
		s.retheme()
	}
	l.Add(none)

	for _, p := range backgroundFiles() {
		p := p
		row := ui.NewNode(ui.R(PadX, 0, RowW, 64))
		row.Focusable = true
		row.SetCursorRadius(14)
		row.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
			th := ui.CurrentTheme
			card(c, r, n.Press.Get(), th.Surface)
			c.Text(ui.R(r.X+18, r.Y+8, r.W-200, 22), filepath.Base(p), 14, th.Text, ui.AlignLeft, true)
			c.Text(ui.R(r.X+18, r.Y+34, r.W-36, 18), p, 10, th.Muted, ui.AlignLeft, false)
			if strings.EqualFold(p, cfg.Background) {
				chip(c, ui.R(r.Right()-130, r.Y+18, 112, 26), "当前", th.Primary)
			}
		}
		row.Activate = func(*ui.Node) {
			s.c.Update(func(c *tune.Config) { c.Background = p })
			s.retheme()
		}
		l.Add(row)
	}
	return root
}

func backgroundFiles() []string {
	var roots []string
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, "Pictures"), filepath.Join(home, "Desktop"))
	}
	roots = append(roots, filepath.Join(tune.ConfigPath(), "backgrounds"))
	var out []string
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				rel, _ := filepath.Rel(root, path)
				if strings.Count(rel, string(os.PathSeparator)) >= 2 {
					return filepath.SkipDir
				}
				return nil
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".png", ".jpg", ".jpeg", ".bmp", ".webp":
				out = append(out, path)
			}
			return nil
		})
	}
	sort.Strings(out)
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}
