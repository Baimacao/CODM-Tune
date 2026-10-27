package appui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"codmtune/internal/hud"
	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

func openPath(p string) error { return exec.Command("explorer", p).Start() }

// overlayAlphaMin is the darkest allowed HUD window alpha (about 70%).
// Keeping a floor preserves text contrast while still letting the panel fade.
const overlayAlphaMin = 178

func (s *screens) section(l *ui.List, text string) {
	n := ui.NewNode(ui.R(PadX, 0, RowW, 26))
	n.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		th := ui.CurrentTheme
		if th.Style == "endfield" {
			// Record-row header: a solid acid tick, then // and a wide-tracked
			// label. The rule underneath is ink, not acid — acid is a fill.
			// It sits one pixel above the node edge so it never doubles up
			// with the top border of the card below.
			c.FillRect(ui.R(r.X, r.Y+9, 4, 12), th.Mark)
			c.Text(ui.R(r.X+12, r.Y+8, r.W-40, 18), "// "+strings.ToUpper(text), 10, th.Text, ui.AlignLeft, true)
			c.Line(r.X+12, r.Y+23, r.X+r.W, r.Y+23, 1, th.Outline)
			return
		}
		if th.Industrial() {
			c.FillRect(ui.R(r.X, r.Y+18, 3, 8), th.Primary)
			c.Text(ui.R(r.X+12, r.Y+8, r.W-120, 18), "// "+strings.ToUpper(text), 10, th.Text, ui.AlignLeft, true)
			for i := 0; i < 3; i++ {
				x := r.X + r.W - 26 + float32(i)*8
				c.FillRect(ui.R(x, r.Y+11, 4, 4), th.Primary)
				c.FillRect(ui.R(x+4, r.Y+11, 4, 4), th.Bg)
			}
			c.Line(r.X+12, r.Y+26, r.X+r.W, r.Y+26, 1, th.Outline)
			return
		}
		c.Text(ui.R(r.X+2, r.Y+8, r.W, 18), strings.ToUpper(text), 10, th.Muted, ui.AlignLeft, true)
	}
	l.Add(n)
}

// ---- game and launch ---------------------------------------------------

func (s *screens) gameCat() *ui.Node {
	root := s.cat("game", "游戏与启动", "安装探测、直接启动、启动器回退与战绩入口")
	l := s.listIn(root)
	exe, _, launcher, running, pid := s.c.Game()

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "rocket", title: "启动 CODM",
		desc: func() string {
			if running {
				return fmt.Sprintf("已在运行 (pid %d)；启动按钮会再拉起一个实例", pid)
			}
			return "直接启动主程序，跳过启动器；启动后自动进入监控。"
		}(),
		value: func() string { return exe },
		open:  func() { s.c.Launch() },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "window", title: "安装与探测",
		desc:  "扫描 D 盘固定目录（文件名 + CODM_Data 双重校验）；多个候选必须手动选。",
		value: func() string { return filepath.Dir(exe) },
		open:  func() { s.c.Detect() },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "book", title: "启动器",
		desc: "从卸载表解析；仅当主程序缺失时回退使用。",
		value: func() string {
			if launcher == "" {
				return "未发现"
			}
			return launcher
		},
		open: func() { s.c.OpenFolder() },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "window", title: "独占全屏快捷方式",
		desc:  "在桌面生成 .bat，用 -window-mode exclusive 5 启动；不改游戏文件，失败可删除。",
		value: func() string { return "桌面 / CODM-Exclusive-Fullscreen.bat" },
		open: func() {
			if p, err := s.c.ExclusiveShortcut(); err == nil {
				_ = openPath(filepath.Dir(p))
			}
		},
	}))

	s.section(l, "账号与回放")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "book", title: "战绩查询（官方站）",
		desc:  "打开官方页面自行登录查询；工具不代登录、不抓取账号数据。",
		value: func() string { return "codm.qq.com" },
		open:  func() { _ = tune.OpenURL("https://codm.qq.com/main.shtml") },
	}))
	return root
}

// ---- performance -------------------------------------------------------

func (s *screens) perfCat() *ui.Node {
	root := s.cat("performance", "性能与调度", "电源、ACE、后台、调度与计时器；系统项在“系统调优”")
	l := s.listIn(root)

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 100), feature{
		icon: "bolt", title: "电源计划 · 卓越性能",
		desc: "powercfg 解锁/复用卓越性能并激活；记录注入前 GUID。会增加功耗与发热。",
		value: func() string {
			p := s.c.Power()
			if p.Name == "" {
				return "读取中"
			}
			return p.Name + " · " + shortGUID(p.GUID)
		},
		tone: func() tune.Tone {
			if s.c.ModuleOn("power") {
				return tune.ToneGood
			}
			return tune.ToneNeutral
		},
		toggle:   func() bool { return s.c.ModuleOn("power") },
		onToggle: func(on bool) { s.c.SetModule("power", on) },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 112), feature{
		icon: "shield", title: "ACE 资源限制",
		desc:     "Idle 优先级 + 单核亲和性，每 60 秒重新枚举；不结束/不挂起/不注入。启用需按住。",
		value:    func() string { return s.c.ACE().Text },
		tone:     func() tune.Tone { return s.c.ACE().Tone },
		toggle:   func() bool { return s.c.ModuleOn("ace") },
		onToggle: func(on bool) { s.c.SetModule("ace", on) },
		hold:     1.2,
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 100), feature{
		icon: "window", title: "后台进程抑制",
		desc:     "游戏运行时对选中进程 Idle + 低亲和性；退出自动恢复。进入进程列表查看读数。",
		value:    func() string { b := s.c.BG(); return b.Text },
		tone:     func() tune.Tone { return s.c.BG().Tone },
		toggle:   func() bool { return s.c.ModuleOn("bg") },
		onToggle: func(on bool) { s.c.SetModule("bg", on) },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 100), feature{
		icon: "chip", title: "系统调度 · CODM High",
		desc:     "CODM → High，亲和性排除 CPU 0；退出回滚。ACE 初始化后若拒绝访问会如实显示。",
		value:    func() string { return s.c.Sched().Text },
		tone:     func() tune.Tone { return s.c.Sched().Tone },
		toggle:   func() bool { return s.c.ModuleOn("schedule") },
		onToggle: func(on bool) { s.c.SetModule("schedule", on) },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 100), feature{
		icon: "gauge", title: "计时器精度 · 0.5 ms",
		desc: "游戏运行时请求 0.5 ms 定时器，退出释放；可降低帧时间抖动。",
		value: func() string {
			if s.c.TimerBoostOn() {
				return "已启用 · 游戏时生效"
			}
			return "未启用"
		},
		toggle:   func() bool { return s.c.TimerBoostOn() },
		onToggle: func(on bool) { s.c.SetTimerBoost(on) },
	}))

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "disc", title: "后台进程与读数",
		desc: "查看每个候选进程的 CPU/内存，再决定是否抑制。",
		open: func() { s.app.Push(s.processes()) },
	}))
	return root
}

// ---- system tweaks -----------------------------------------------------

func (s *screens) systemCat() *ui.Node {
	root := s.cat("system", "系统调优", "游戏模式、录制、外观与 HKLM 项；全部保存原值，可还原")
	l := s.listIn(root)
	cfg := s.c.Cfg()

	for _, t := range []struct{ id, icon, name, desc string }{
		{"gamemode", "gamepad", "Windows 游戏模式", "把前台游戏标记为游戏，系统优先调度。"},
		{"gamedvr", "monitor", "关闭后台录制", "关闭 GameDVR 捕获，减少后台编码与磁盘写入。"},
		{"gpupref", "chip", "GPU 高性能偏好", "把 CODM.exe 标记为高性能 GPU；双显卡机器直接受益。"},
	} {
		t := t
		l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
			icon: t.icon, title: t.name, desc: t.desc,
			value: func() string { return s.c.TweakNote(t.id) },
			tone: func() tune.Tone {
				if s.c.TweakOn(t.id) {
					return tune.ToneGood
				}
				return tune.ToneNeutral
			},
			toggle:   func() bool { return s.c.TweakOn(t.id) },
			onToggle: func(on bool) { s.c.SetTweak(t.id, on) },
		}))
	}

	s.section(l, "内存回收（Mem Reduct 同源调用）")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "bolt", title: "立即回收",
		desc: "清空工作集、系统文件缓存、已修改页、待机列表与注册表缓存；只调用系统内存列表接口，不碰任何进程。",
		value: func() string {
			st := s.c.MemState()
			if st.Runs == 0 {
				return fmt.Sprintf("当前可用 %d MB · 从未执行", st.Stat.Avail/1024/1024)
			}
			return fmt.Sprintf("当前可用 %d MB · 上次释放 %d MB · 累计 %d 次",
				st.Stat.Avail/1024/1024, st.LastFreed/1024/1024, st.Runs)
		},
		hint: "Enter 立即回收",
		open: func() {
			s.scanMsg = "正在回收内存…"
			go func() {
				st := s.c.RunMemory(0)
				msg := fmt.Sprintf("回收完成 · 释放 %d MB · 可用 %d / %d MB",
					st.LastFreed/1024/1024, st.Stat.Avail/1024/1024, st.Stat.Total/1024/1024)
				if len(st.Failed) > 0 {
					msg += " · 跳过 " + strings.Join(st.Failed, "/")
				}
				s.scanMsg = msg
				s.app.Post(func() {})
			}()
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "gauge", title: "自动定期回收",
		desc: "空闲按间隔执行；游戏运行时改用更短间隔。默认开启，间隔可调。",
		value: func() string {
			st := s.c.MemState()
			if !st.Auto {
				return "已关闭 · 仅手动"
			}
			next := "—"
			if st.NextUnix > 0 {
				next = time.Unix(st.NextUnix, 0).Format("15:04")
			}
			return fmt.Sprintf("%d 分钟 · 游戏时 %d 分钟 · 下次 %s", st.Interval, s.c.Cfg().MemGameGap, next)
		},
		toggle:   func() bool { return s.c.Cfg().MemAuto },
		onToggle: func(on bool) { s.c.Update(func(c *tune.Config) { c.MemAuto = on }) },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "gauge", title: "空闲间隔",
		desc:  "无游戏时的回收间隔。",
		value: func() string { return fmt.Sprintf("%d 分钟", s.c.Cfg().MemInterval) },
		adjust: func(d int) {
			s.c.Update(func(c *tune.Config) {
				c.MemInterval += d * 5
				if c.MemInterval < 5 {
					c.MemInterval = 5
				}
				if c.MemInterval > 240 {
					c.MemInterval = 240
				}
			})
		},
		progress: func() float64 { return float64(s.c.Cfg().MemInterval-5) / 235 },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "gamepad", title: "游戏时间隔",
		desc:  "CODM 运行时的回收间隔；内存紧张时更短的间隔更有意义。",
		value: func() string { return fmt.Sprintf("%d 分钟", s.c.Cfg().MemGameGap) },
		adjust: func(d int) {
			s.c.Update(func(c *tune.Config) {
				c.MemGameGap += d * 2
				if c.MemGameGap < 2 {
					c.MemGameGap = 2
				}
				if c.MemGameGap > 60 {
					c.MemGameGap = 60
				}
			})
		},
		progress: func() float64 { return float64(s.c.Cfg().MemGameGap-2) / 58 },
	}))

	if cfg.Advanced {
		s.section(l, "高级项（修改系统 / HKLM）")
		for _, t := range []struct{ id, icon, name, desc string }{
			{"visualfx", "monitor", "极简外观（Atlas 风格）", "关闭透明与动画效果；高级项，随时可还原。"},
			{"priosep", "chip", "前台调度偏好 38", "Win32PrioritySeparation=38；需重启，高级项。"},
			{"hags", "bolt", "硬件加速 GPU 调度", "HwSchMode=2；需重启，部分驱动会掉帧。"},
			{"fso", "window", "禁用全屏优化（CODM）", "写入 AppCompatFlags 层，绕过全屏优化合成。"},
		} {
			t := t
			l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
				icon: t.icon, title: t.name, desc: t.desc,
				value: func() string { return s.c.TweakNote(t.id) },
				tone: func() tune.Tone {
					if s.c.TweakOn(t.id) {
						return tune.ToneWarn
					}
					return tune.ToneNeutral
				},
				toggle:   func() bool { return s.c.TweakOn(t.id) },
				onToggle: func(on bool) { s.c.SetTweak(t.id, on) },
			}))
		}
	} else {
		hint := ui.NewNode(ui.R(PadX, 0, RowW, 40))
		hint.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
			c.Text(ui.R(r.X+2, r.Y+10, r.W, 20), "高级项已隐藏 · 在设置中开启高级模式后显示", 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
		}
		l.Add(hint)
	}
	return root
}

// ---- visual ------------------------------------------------------------

func (s *screens) visualCat() *ui.Node {
	root := s.cat("visual", "画面与 FOV", "视野/持枪 FOV、帧率、注册表备份与画面精简")
	go s.c.RefreshRegistry()
	l := s.listIn(root)
	viewKeys, weaponKeys := s.c.FOVKeys()

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 100), feature{
		icon: "monitor", title: "视野 FOV（游戏内滑块）",
		desc:  fmt.Sprintf("本工具不再写入视野键（%d 个）；v3 的 double 写法会破坏 payload，已在 v6 移除。请用游戏内滑块。", len(viewKeys)),
		value: func() string { return fmt.Sprintf("游戏内设置 · 记录 %d°", s.c.FOVView()) },
		info:  true,
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 104), feature{
		icon: "gamepad", title: "持枪 FOV 解锁（第 7 字节）",
		desc: fmt.Sprintf("社区验证：只改 8 字节 payload 的 byte 6（默认 0xF2，0xFF 最大），保留其余字节；%d 个键。", len(weaponKeys)),
		value: func() string {
			cur := "无键"
			if b, ok := s.c.CurrentWeaponFOVByte(); ok {
				cur = fmt.Sprintf("当前 0x%02X", b)
			}
			return fmt.Sprintf("写入 0x%02X · %s", s.c.FOVByte(), cur)
		},
		hint: "Enter 展开",
		open: func() {
			vals := []int{0xF2, 0xF6, 0xFA, 0xFF}
			opts := make([]choiceOption, 0, len(vals))
			for _, v := range vals {
				opts = append(opts, choiceOption{fmt.Sprintf("%d", v), fmt.Sprintf("0x%02X", v), "只改第 7 字节"})
			}
			s.choose("visual", "持枪 FOV 解锁", "字节越大 FOV 越大；先备份，游戏内滑块可恢复", fmt.Sprintf("%d", s.c.FOVByte()), opts, func(v string) {
				var n int
				fmt.Sscanf(v, "%d", &n)
				s.c.SetFOVByte(n)
			})
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "bolt", title: "帧率无上限",
		desc: "写 EnableFramerateCustomize=1 + FramerateCustomizeValue=0；两个键都缺失时拒绝。",
		value: func() string {
			r := s.c.Reg()
			if r.FPSKey == "" {
				return "帧率键缺失"
			}
			return "键已发现"
		},
		tone: func() tune.Tone {
			if s.c.Reg().FPSKey == "" {
				return tune.ToneWarn
			}
			return tune.ToneGood
		},
		toggle:   func() bool { return s.c.FPSUnlimited() },
		onToggle: func(on bool) { s.c.SetFPSUnlimited(on) },
	}))

	apply := s.featureRow(ui.R(PadX, 0, RowW, 104), feature{
		icon: "rocket", title: "按住写入注册表",
		desc:  "写入前自动 reg export；只改持枪 FOV 第 7 字节，游戏运行中拒绝；恢复用游戏内滑块。",
		value: func() string { r := s.c.Reg(); return r.Text },
		tone: func() tune.Tone {
			if s.c.Reg().Applied {
				return tune.ToneGood
			}
			return tune.ToneNeutral
		},
		hold: 1.2,
		open: func() { s.c.ApplyFOV() },
		disabled: func() bool {
			_, _, _, running, _ := s.c.Game()
			r := s.c.Reg()
			return running || !r.KeyExists
		},
	})
	l.Add(apply)

	restore := s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "disc", title: "按住从备份还原",
		desc:  "导入写入前的整键备份；成功后清空备份指针，下次写入重新备份。",
		value: func() string { return s.c.Reg().Backup },
		hold:  1.2,
		open:  func() { s.c.RestoreRegistry() },
		disabled: func() bool {
			_, _, _, running, _ := s.c.Game()
			return running || s.c.Reg().Backup == ""
		},
	})
	l.Add(restore)
	return root
}

// ---- overlay -----------------------------------------------------------

func (s *screens) overlayCat() *ui.Node {
	root := s.cat("overlay", "悬浮窗", "游戏内 HUD：FPS、1% low、卡顿、RTT、按键与服务器归属")
	l := s.listIn(root)

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 100), feature{
		icon: "window", title: "启用悬浮窗",
		desc: "置顶 + 鼠标穿透；启用后即使游戏未运行也会显示 OFFLINE，便于确认位置与透明度。",
		value: func() string {
			if s.c.Cfg().OverlayEnabled {
				return "已启用"
			}
			return "未启用"
		},
		toggle: func() bool { return s.c.Cfg().OverlayEnabled },
		onToggle: func(on bool) {
			s.c.Update(func(c *tune.Config) { c.OverlayEnabled = on })
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 88), feature{
		icon: "monitor", title: "未进游戏也显示",
		desc: "开启后 HUD 以 OFFLINE 状态常驻，便于调透明度与位置；关闭后只在游戏运行时显示。",
		value: func() string {
			if s.c.Cfg().OverlayIdle {
				return "常驻（OFFLINE）"
			}
			return "仅游戏时"
		},
		toggle:   func() bool { return s.c.Cfg().OverlayIdle },
		onToggle: func(on bool) { s.c.Update(func(c *tune.Config) { c.OverlayIdle = on }) },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 88), feature{
		icon: "gauge", title: "样式",
		desc: "紧凑 / 完整；完整模式多显示 jitter、帧数与按键。",
		value: func() string {
			if s.c.Cfg().OverlayStyle == "full" {
				return "完整"
			}
			return "紧凑"
		},
		hint: "Enter 展开",
		open: func() {
			cur := s.c.Cfg().OverlayStyle
			s.choose("overlay", "悬浮窗样式", "选择 HUD 显示密度", cur, []choiceOption{
				{"compact", "紧凑", "FPS / 1% low / RTT"},
				{"full", "完整", "额外显示 jitter、帧数与按键"},
			}, func(v string) {
				s.c.Update(func(c *tune.Config) { c.OverlayStyle = v })
			})
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 88), feature{
		icon: "monitor", title: "HUD 大小",
		desc: "80% / 100% / 120% / 150%；切换后会重建悬浮窗，位置保持不变。",
		value: func() string {
			v := s.c.Cfg().OverlayScale
			if v <= 0 {
				v = 100
			}
			return fmt.Sprintf("%d%%", v)
		},
		hint: "Enter 展开",
		open: func() {
			cur := fmt.Sprintf("%d", s.c.Cfg().OverlayScale)
			s.choose("overlay", "HUD 大小", "按比例缩放整个 HUD", cur, []choiceOption{
				{"80", "80%", "更小，适合小屏"},
				{"100", "100%", "默认"},
				{"120", "120%", "更醒目"},
				{"150", "150%", "最大，适合远距离观看"},
			}, func(v string) {
				n := 100
				_, _ = fmt.Sscanf(v, "%d", &n)
				s.c.Update(func(c *tune.Config) { c.OverlayScale = n })
				// The child owns its surface size; rebuild it with the new scale.
				hud.StopChild()
			})
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 88), feature{
		icon: "window", title: "位置",
		desc: "右上 / 左上 / 右下 / 左下；选「自定义」后用下面 X / Y 微调。",
		value: func() string {
			switch s.c.Cfg().OverlayCorner {
			case "tl":
				return "左上"
			case "br":
				return "右下"
			case "bl":
				return "左下"
			case "custom":
				c := s.c.Cfg()
				return fmt.Sprintf("自定义 %d,%d", c.OverlayX, c.OverlayY)
			}
			return "右上"
		},
		hint: "Enter 展开",
		open: func() {
			s.choose("overlay", "悬浮窗位置", "选择贴合屏幕的角落", s.c.Cfg().OverlayCorner, []choiceOption{
				{"tr", "右上", "默认，避开小地图"},
				{"tl", "左上", "避开击杀信息"},
				{"br", "右下", "靠近拇指区"},
				{"bl", "左下", "避开技能区"},
				{"custom", "自定义", "用 X / Y 行以 10px 步进移动"},
			}, func(v string) {
				s.c.Update(func(c *tune.Config) {
					c.OverlayCorner = v
					if v == "custom" {
						if c.OverlayX <= 0 {
							c.OverlayX = 800
						}
						if c.OverlayY <= 0 {
							c.OverlayY = 240
						}
					}
				})
			})
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 80), feature{
		icon: "window", title: "自定义 X",
		desc:  "HUD 左边缘距屏幕左侧像素；← → 每次 10px，自动切换到自定义。",
		value: func() string { return fmt.Sprintf("%d px", s.c.Cfg().OverlayX) },
		adjust: func(d int) {
			s.c.Update(func(c *tune.Config) {
				c.OverlayCorner = "custom"
				c.OverlayX += d * 10
				if c.OverlayX < 0 {
					c.OverlayX = 0
				}
				if c.OverlayX > 4000 {
					c.OverlayX = 4000
				}
			})
		},
		hint: "← → 移动",
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 80), feature{
		icon: "window", title: "自定义 Y",
		desc:  "HUD 上边缘距屏幕顶部像素；← → 每次 10px，自动切换到自定义。",
		value: func() string { return fmt.Sprintf("%d px", s.c.Cfg().OverlayY) },
		adjust: func(d int) {
			s.c.Update(func(c *tune.Config) {
				c.OverlayCorner = "custom"
				c.OverlayY += d * 10
				if c.OverlayY < 0 {
					c.OverlayY = 0
				}
				if c.OverlayY > 4000 {
					c.OverlayY = 4000
				}
			})
		},
		hint: "← → 移动",
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 100), feature{
		icon: "monitor", title: "HUD 不透明度",
		desc: "点轨道或拖动滑块直接设置；← → 每次约 4%；70% 更透，100% 不透明。",
		value: func() string {
			a := s.c.Cfg().OverlayOpacity
			return fmt.Sprintf("%d%%", (a*100+127)/255)
		},
		adjust: func(d int) {
			s.c.Update(func(c *tune.Config) {
				c.OverlayOpacity += d * 10
				if c.OverlayOpacity < overlayAlphaMin {
					c.OverlayOpacity = overlayAlphaMin
				}
				if c.OverlayOpacity > 255 {
					c.OverlayOpacity = 255
				}
			})
		},
		slider: func(frac float64) {
			s.c.Update(func(c *tune.Config) {
				c.OverlayOpacity = overlayAlphaMin + int(frac*float64(255-overlayAlphaMin)+0.5)
			})
		},
		progress: func() float64 {
			a := s.c.Cfg().OverlayOpacity
			if a < overlayAlphaMin {
				a = overlayAlphaMin
			}
			if a > 255 {
				a = 255
			}
			return float64(a-overlayAlphaMin) / float64(255-overlayAlphaMin)
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 88), feature{
		icon: "gauge", title: "FPS / 1% low / 卡顿",
		desc:     "PresentMon 帧时间；卡顿定义为 >2×中位数或 >33.3 ms 的帧占比。",
		toggle:   func() bool { return s.c.Cfg().OverlayFPS },
		onToggle: func(on bool) { s.c.Update(func(c *tune.Config) { c.OverlayFPS = on }) },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 80), feature{
		icon: "gauge", title: "显示 1% low",
		desc:     "只关闭低帧分位数，保留 FPS；更少文字，适合小屏。",
		toggle:   func() bool { return s.c.Cfg().OverlayLow },
		onToggle: func(on bool) { s.c.Update(func(c *tune.Config) { c.OverlayLow = on }) },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 80), feature{
		icon: "gauge", title: "显示卡顿占比",
		desc:     "只关闭 stutter 百分比，保留 FPS 与 1% low。",
		toggle:   func() bool { return s.c.Cfg().OverlayStutter },
		onToggle: func(on bool) { s.c.Update(func(c *tune.Config) { c.OverlayStutter = on }) },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "bolt", title: "显示吞子弹估算",
		desc:     "按 RTT/jitter 与帧卡顿估算风险；无样本显示 —，不是命中判定。",
		toggle:   func() bool { return s.c.Cfg().OverlayLoss },
		onToggle: func(on bool) { s.c.Update(func(c *tune.Config) { c.OverlayLoss = on }) },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 100), feature{
		icon: "book", title: "纯文字模式",
		desc: "去掉卡片底与边框，只保留文字与数值；适合不想遮挡画面的场景。",
		value: func() string {
			if s.c.Cfg().OverlayText {
				return "已开启"
			}
			return "面板模式"
		},
		toggle:   func() bool { return s.c.Cfg().OverlayText },
		onToggle: func(on bool) { s.c.Update(func(c *tune.Config) { c.OverlayText = on }) },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 88), feature{
		icon: "keyboard", title: "键盘按键显示",
		desc:     "轮询 GetAsyncKeyState（WASD/Shift/Ctrl/Space/R/E/Q/1-4），不装钩子。",
		toggle:   func() bool { return s.c.Cfg().OverlayKeys },
		onToggle: func(on bool) { s.c.Update(func(c *tune.Config) { c.OverlayKeys = on }) },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 88), feature{
		icon: "wifi", title: "网络延迟与服务器 IP",
		desc:     "从 CODM 进程 TCP 表取远端 IP，ICMP 测 RTT；不修改网络。",
		toggle:   func() bool { return s.c.Cfg().OverlayNet },
		onToggle: func(on bool) { s.c.Update(func(c *tune.Config) { c.OverlayNet = on }) },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "wifi", title: "服务器省份查询（联网）",
		desc: "用 ip-api.com 反查归属地；开启即把服务器 IP 发给第三方。",
		value: func() string {
			if s.c.Cfg().OverlayServer {
				return "已开启 · 会联网"
			}
			return "关闭 · 不联网"
		},
		toggle:   func() bool { return s.c.Cfg().OverlayServer },
		onToggle: func(on bool) { s.c.Update(func(c *tune.Config) { c.OverlayServer = on }) },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "gauge", title: "FPS 采集器（诊断）",
		desc: "内置 PresentMon 2.6；部分 Windows 10 的 ETW 不提供 Present 事件，此时 FPS 显示 N/A，其余指标可用。",
		value: func() string {
			d := s.c.OverlayData()
			if !d.Game {
				return "等待游戏"
			}
			if d.Ready {
				return fmt.Sprintf("正常 · %d 帧", d.Frames)
			}
			return "无数据（本机 ETW）"
		},
		tone: func() tune.Tone {
			d := s.c.OverlayData()
			if d.Ready {
				return tune.ToneGood
			}
			return tune.ToneWarn
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "shield", title: "网络延迟与卡顿",
		desc: "RTT 来自 ICMP 或 TCP 回退；卡顿来自帧时间样本。吞子弹估算已停用，避免把推测误当判定。",
		value: func() string {
			d := s.c.OverlayData()
			if !d.Game {
				return "等待游戏"
			}
			return fmt.Sprintf("RTT %d ms · 卡顿 %.1f%%", d.RTT, d.Stutter)
		},
	}))
	return root
}

// ---- replay ------------------------------------------------------------

func (s *screens) replayCat() *ui.Node {
	root := s.cat("replay", "回放管理", "扫描整局回放、备份到工具目录、打开原文件")
	go s.c.ScanReplays()
	l := s.listIn(root)
	cfg := s.c.Cfg()
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "disc", title: "扫描回放",
		desc:  "游戏目录 + 文档/视频 + 配置目录；扩展名 .replay/.rec/.demo/.codm 或回放命名目录。",
		value: func() string { return fmt.Sprintf("已发现 %d 个", len(s.c.Replays())) },
		open:  func() { s.c.ScanReplays() },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "shield", title: "备份全部回放",
		desc:  "复制到 config\\replays\\时间戳；不移动、不删除原文件。",
		value: func() string { return tune.ConfigPath() + "\\replays" },
		open: func() {
			if n, err := s.c.BackupReplays(nil); err == nil {
				s.c.LogLine(fmt.Sprintf("回放备份：%d 个", n))
			}
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "book", title: "打开回放目录",
		desc: "打开配置目录下的回放备份；若已手动指定目录，可先打开该目录。",
		value: func() string {
			if cfg.ReplayDir != "" {
				return cfg.ReplayDir
			}
			return tune.ConfigPath() + "\\replays"
		},
		open: func() {
			if cfg.ReplayDir != "" {
				_ = openPath(cfg.ReplayDir)
				return
			}
			_ = openPath(tune.ConfigPath() + "\\replays")
		},
	}))
	for _, r := range s.c.Replays() {
		r := r
		row := ui.NewNode(ui.R(PadX, 0, RowW, 64))
		row.Focusable = true
		row.SetCursorRadius(14)
		row.Draw = func(n *ui.Node, c ui.Painter, rr ui.Rect) {
			card(c, rr, n.Press.Get(), ui.CurrentTheme.Surface)
			c.Text(ui.R(rr.X+18, rr.Y+8, rr.W-220, 22), r.Name, 13, ui.CurrentTheme.Text, ui.AlignLeft, true)
			c.Text(ui.R(rr.X+18, rr.Y+34, rr.W-220, 18), fmt.Sprintf("%s · %.1f MB", r.When.Format("2006-01-02 15:04"), float64(r.Size)/1048576), 10, ui.CurrentTheme.Muted, ui.AlignLeft, false)
			c.Text(ui.R(rr.Right()-190, rr.Y, 170, rr.H), "Enter 定位 →", 10, ui.CurrentTheme.PrimaryDim, ui.AlignRight, false)
		}
		row.Activate = func(*ui.Node) { _ = exec.Command("explorer", "/select,", r.Path).Start() }
		l.Add(row)
	}
	if len(s.c.Replays()) == 0 {
		hint := ui.NewNode(ui.R(PadX, 0, RowW, 50))
		hint.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
			c.Text(ui.R(r.X+2, r.Y+14, r.W, 20), "尚未发现回放文件。先在游戏内保存一局整局回放，再点“扫描回放”。", 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
		}
		l.Add(hint)
	}
	return root
}

// ---- diagnostics -------------------------------------------------------

func (s *screens) diagCat() *ui.Node {
	root := s.cat("diagnostics", "诊断", "DXGI 适配器、实际 GPU、Event 4101、TdrDelay 与只读网络")
	l := s.listIn(root)
	g := s.c.GPU()
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 100), feature{
		icon: "monitor", title: "显卡与驱动",
		desc: "DXGI 枚举 + PDH 每进程 GPU 引擎计数标注实际 GPU；驱动版本来自显示类注册表。",
		value: func() string {
			gg := s.c.GPU()
			actual := gg.Actual
			if actual == "" && len(gg.Adapters) > 0 {
				actual = gg.Adapters[0].Name
			}
			if actual == "" {
				actual = "未扫描"
			}
			return s.scanLabel("gpu", actual)
		},
		hint: "Enter 扫描",
		open: func() { s.startScan("gpu") },
	}))
	for _, a := range g.Adapters {
		a := a
		row := ui.NewNode(ui.R(PadX, 0, RowW, 74))
		row.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
			card(c, r, 0, ui.CurrentTheme.Surface)
			c.Text(ui.R(r.X+18, r.Y+12, 420, 22), a.Name, 13, ui.CurrentTheme.Text, ui.AlignLeft, true)
			c.Text(ui.R(r.X+18, r.Y+40, r.W-220, 18), fmt.Sprintf("%s · 驱动 %s · %.1f GB", a.Vendor, a.Driver, a.VRAMGB), 10, ui.CurrentTheme.Muted, ui.AlignLeft, false)
			if a.Used {
				chip(c, ui.R(r.Right()-180, r.Y+22, 162, 26), "CODM 实际使用", ui.CurrentTheme.Success)
			}
		}
		l.Add(row)
	}
	if s.c.Cfg().Advanced {
		l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
			icon: "bolt", title: "4101 / TdrDelay",
			desc: "显示驱动恢复事件；TdrDelay=8 是缓冲不是修复，写 HKLM 前备份，默认关闭。",
			value: func() string {
				gg := s.c.GPU()
				if gg.TdrOn {
					return fmt.Sprintf("TdrDelay=8 · 事件 %d", len(gg.Events))
				}
				return fmt.Sprintf("TdrDelay 未写入 · 事件 %d", len(gg.Events))
			},
			tone: func() tune.Tone {
				if s.c.GPU().TdrOn {
					return tune.ToneWarn
				}
				return tune.ToneNeutral
			},
			toggle:   func() bool { return s.c.GPU().TdrOn },
			onToggle: func(on bool) { s.c.SetTdr(on) },
			hold:     1.2,
		}))
	} else {
		hint := ui.NewNode(ui.R(PadX, 0, RowW, 34))
		hint.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
			c.Text(ui.R(r.X+2, r.Y+8, r.W, 18), "TdrDelay 写入需高级模式 · 当前只读显示 4101 事件", 10, ui.CurrentTheme.Muted, ui.AlignLeft, false)
		}
		l.Add(hint)
	}
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "wifi", title: "网络诊断",
		desc: "DNS 解析延迟 + CODM 进程 TCP 端点；不修改 hosts/DNS，不装加速器。",
		value: func() string {
			n := s.c.Net()
			normal := "DNS " + n.DNSTime
			if n.DNSErr != "" {
				normal = "DNS 失败"
			}
			return s.scanLabel("net", normal)
		},
		hint: "Enter 扫描",
		open: func() { s.startScan("net") },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "book", title: "导出诊断报告",
		desc:  "生成 txt：系统/管理员、配置、GPU、网络、FOV 原始字节、HUD 状态与最近日志；不含凭据。",
		value: func() string { return tune.ConfigPath() },
		open:  func() { s.exportDiagnostics() },
	}))
	for _, d := range g.Deps {
		d := d
		row := ui.NewNode(ui.R(PadX, 0, RowW, 34))
		row.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
			col := ui.CurrentTheme.Success
			txt := d.Name
			if !d.OK {
				col = ui.CurrentTheme.Danger
				txt += " 缺失"
			}
			c.Text(ui.R(r.X+2, r.Y, 300, 20), txt, 11, col, ui.AlignLeft, false)
		}
		l.Add(row)
	}
	return root
}

// ---- knowledge ---------------------------------------------------------

func (s *screens) knowledgeCat() *ui.Node {
	root := s.cat("knowledge", "知识库", "markdown 笔记：放在配置目录的 knowledge 文件夹")
	l := s.listIn(root)
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "book", title: "知识库目录",
		desc:  "把 .md 文件放进该目录；点击文件进入阅读，支持标题/列表/代码块。",
		value: func() string { return s.c.KnowledgeDir() },
		open:  func() { _ = openPath(s.c.KnowledgeDir()) },
	}))
	for _, f := range s.c.KnowledgeFiles() {
		f := f
		row := ui.NewNode(ui.R(PadX, 0, RowW, 64))
		row.Focusable = true
		row.SetCursorRadius(14)
		row.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
			card(c, r, n.Press.Get(), ui.CurrentTheme.Surface)
			c.Text(ui.R(r.X+18, r.Y+8, r.W-160, 22), f.Name, 13, ui.CurrentTheme.Text, ui.AlignLeft, true)
			c.Text(ui.R(r.X+18, r.Y+34, r.W-36, 18), f.Path, 10, ui.CurrentTheme.Muted, ui.AlignLeft, false)
		}
		row.Activate = func(*ui.Node) { s.app.Push(s.markdown(f.Path, f.Name)) }
		l.Add(row)
	}
	if len(s.c.KnowledgeFiles()) == 0 {
		hint := ui.NewNode(ui.R(PadX, 0, RowW, 50))
		hint.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
			c.Text(ui.R(r.X+2, r.Y+14, r.W, 20), "还没有 .md 文件。把笔记放进上面的目录，重新进入本页即可。", 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
		}
		l.Add(hint)
	}
	return root
}

var _ = tune.OpenURL
