package appui

import (
	"fmt"
	"strings"

	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

// home is the dashboard: install, launch and six live module cards.
func (s *screens) home() *ui.Node {
	root := s.newRoot("CODM TUNE", "原生 PC 客户端性能伴侣 · 直接启动、进程调度、电源注入、驱动修复与只读网络诊断")

	hero := ui.NewNode(ui.R(PadX, 112, Wide, 176))
	hero.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		card(c, r, 0, ui.CurrentTheme.Surface)
		exe, _, _, running, pid := s.c.Game()
		c.Text(ui.R(r.X+24, r.Y+18, 420, 24), "CODM 安装", 16, ui.CurrentTheme.Text, ui.AlignLeft, true)
		if exe == "" {
			c.Text(ui.R(r.X+24, r.Y+52, r.W-60, 20), "尚未探测到安装目录", 13, ui.CurrentTheme.Muted, ui.AlignLeft, false)
			chip(c, ui.R(r.X+r.W-260, r.Y+18, 236, 28), "等待探测", ui.CurrentTheme.Danger)
		} else {
			c.Text(ui.R(r.X+24, r.Y+52, r.W-300, 20), exe, 13, ui.CurrentTheme.Muted, ui.AlignLeft, false)
			if running {
				chip(c, ui.R(r.X+r.W-260, r.Y+18, 236, 28), fmt.Sprintf("游戏运行中 · pid %d", pid), ui.CurrentTheme.Success)
			} else {
				chip(c, ui.R(r.X+r.W-260, r.Y+18, 236, 28), "已就绪 · 可直接启动", ui.CurrentTheme.Primary)
			}
		}
	}

	launch := s.button(ui.R(PadX+24, 208, 176, 46), "启动 CODM", btnPrimary, func() {
		s.c.Launch()
	})
	redetect := s.button(ui.R(PadX+216, 214, 116, 34), "重新探测", btnGhost, func() {
		s.c.Detect()
	})
	openDir := s.button(ui.R(PadX+344, 214, 116, 34), "打开目录", btnGhost, func() {
		s.c.OpenFolder()
	})
	candBtn := s.button(ui.R(PadX+472, 214, 160, 34), "选择安装 (0)", btnGhost, func() {
		s.app.Push(s.candidates())
	})
	probe := ui.NewNode(ui.R(PadX+660, 214, 320, 34))
	probe.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		_, detecting, scan := s.c.Candidates()
		cands, _, _ := s.c.Candidates()
		text := ""
		switch {
		case detecting:
			text = "正在扫描 " + scan
			c.Text(ui.R(r.X, r.Y, r.W, r.H), text, 11, ui.CurrentTheme.Muted, ui.AlignRight, false)
		case len(cands) > 1:
			text = fmt.Sprintf("发现 %d 个安装，请选择", len(cands))
			c.Text(ui.R(r.X, r.Y, r.W, r.H), text, 11, ui.CurrentTheme.Tertiary, ui.AlignRight, false)
		case len(cands) == 0:
			c.Text(ui.R(r.X, r.Y, r.W, r.H), "未找到安装：请重新探测或选择目录", 11, ui.CurrentTheme.Muted, ui.AlignRight, false)
		}
		_ = text
	}
	probe.Update = func(*ui.Node, float32) {
		list, _, _ := s.c.Candidates()
		if len(list) > 1 {
			candBtn.Visible = true
			candBtn.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
				p := n.Press.Get()
				rr := r.Inset(p * 2)
				c.FillRoundRect(rr, rr.H/2, ui.CurrentTheme.Surface2)
				c.StrokeRoundRect(rr, rr.H/2, 1, ui.CurrentTheme.Tertiary)
				c.Text(rr, fmt.Sprintf("选择安装 (%d)", len(list)), 13, ui.CurrentTheme.Tertiary, ui.AlignCenter, true)
			}
		} else {
			candBtn.Visible = false
		}
	}
	hero.Add(launch, redetect, openDir, candBtn, probe)
	root.Add(hero)

	gridY := []float32{304, 474}
	gridX := []float32{PadX, PadX + 347, PadX + 694}
	cards := []*ui.Node{
		s.summaryCard(ui.R(gridX[0], gridY[0], 326, 150), "电源计划", func() (string, string, tune.Tone) {
			p := s.c.Power()
			if p.Name == "" {
				return "未应用", "读取电源方案中", tune.ToneNeutral
			}
			detail := "卓越性能 · " + shortGUID(p.GUID)
			if p.Injected {
				detail = "已注入 · 原方案 " + shortGUID(p.PriorGUID)
			}
			return p.Name, detail, tune.ToneGood
		}, func() { s.app.Push(s.modules()) }),
		s.summaryCard(ui.R(gridX[1], gridY[0], 326, 150), "ACE 资源限制", func() (string, string, tune.Tone) {
			a := s.c.ACE()
			detail := "默认关闭 · 启用需按住确认"
			if s.c.ModuleOn("ace") {
				detail = "已启用 · 每 60 秒重新枚举"
			}
			return a.Text, detail, a.Tone
		}, func() { s.app.Push(s.modules()) }),
		s.summaryCard(ui.R(gridX[2], gridY[0], 326, 150), "系统调度", func() (string, string, tune.Tone) {
			a := s.c.Sched()
			return a.Text, "High 优先级 · 排除 CPU 0", a.Tone
		}, func() { s.app.Push(s.modules()) }),
		s.summaryCard(ui.R(gridX[0], gridY[1], 326, 150), "后台进程抑制", func() (string, string, tune.Tone) {
			b := s.c.BG()
			return b.Text, "游戏运行时应用 · 退出恢复", b.Tone
		}, func() { s.app.Push(s.processes()) }),
		s.summaryCard(ui.R(gridX[1], gridY[1], 326, 150), "显卡与运行时", func() (string, string, tune.Tone) {
			g := s.c.GPU()
			if !g.Checked {
				return "未扫描", "DXGI 适配器 · 4101 事件 · VC++", tune.ToneNeutral
			}
			name := g.Actual
			if name == "" && len(g.Adapters) > 0 {
				name = g.Adapters[0].Name
			}
			return name, fmt.Sprintf("事件 %d · 运行库 %d/6", len(g.Events), okDeps(g.Deps)), tune.ToneNeutral
		}, func() { s.app.Push(s.gpu()) }),
		s.summaryCard(ui.R(gridX[2], gridY[1], 326, 150), "只读网络诊断", func() (string, string, tune.Tone) {
			n := s.c.Net()
			if n.Host == "" {
				return "未探测", "DNS 延迟 · TCP 握手 RTT", tune.ToneNeutral
			}
			line := "DNS " + n.DNSTime
			if n.DNSErr != "" {
				line = "DNS 失败"
			}
			return line, "不修改 hosts/DNS · 不装加速器", tune.ToneNeutral
		}, func() { s.app.Push(s.net()) }),
	}
	for _, c := range cards {
		root.Add(c)
	}
	return root
}

func shortGUID(g string) string {
	if len(g) > 8 {
		return g[:8]
	}
	return g
}

func okDeps(deps []tune.Dep) int {
	n := 0
	for _, d := range deps {
		if d.OK {
			n++
		}
	}
	return n
}

// summaryCard is a live dashboard tile that opens a detail screen.
func (s *screens) summaryCard(r ui.Rect, title string, get func() (string, string, tune.Tone), open func()) *ui.Node {
	n := ui.NewNode(r)
	n.Focusable = true
	n.SetCursorRadius(22)
	n.Draw = func(n *ui.Node, c ui.Painter, rr ui.Rect) {
		card(c, rr, n.Press.Get(), ui.CurrentTheme.Surface)
		c.Text(ui.R(rr.X+22, rr.Y+18, rr.W-44, 22), title, 15, ui.CurrentTheme.Text, ui.AlignLeft, true)
		status, detail, tone := get()
		runes := []rune(status)
		if len(runes) > 24 {
			status = string(runes[:23]) + "…"
		}
		statusText(c, ui.R(rr.X+22, rr.Y+50, rr.W-44, 20), status, tone, true)
		c.TextBlock(ui.R(rr.X+22, rr.Y+78, rr.W-44, 44), detail, 12, ui.CurrentTheme.Muted, ui.AlignLeft, false)
		c.Text(ui.R(rr.X+22, rr.Bottom()-30, rr.W-44, 18), "Enter 打开 →", 11, ui.CurrentTheme.PrimaryDim, ui.AlignRight, false)
	}
	n.Activate = func(*ui.Node) { open() }
	return n
}

// modules is the toggle panel for the four scheduling/power modules plus the
// registry entry.
func (s *screens) modules() *ui.Node {
	root := s.newRoot("优化模块", "电源与调度默认开启；ACE 与后台抑制默认关闭；启用危险项需要按住确认")
	list := ui.NewList(ui.R(PadX, 112, Wide, FootY-112-8), 0, 12)

	powerRow := s.moduleRow(ui.R(PadX, 0, Wide, 136), "电源计划 · 卓越性能",
		func() bool { return s.c.ModuleOn("power") },
		func() (string, tune.Tone) {
			p := s.c.Power()
			if p.Injected {
				return "已注入 · " + shortGUID(p.GUID), tune.ToneGood
			}
			if strings.Contains(p.Note, "卓越性能") {
				return "已在用 · " + shortGUID(p.GUID), tune.ToneGood
			}
			if p.Note != "" {
				return p.Note, tune.ToneWarn
			}
			return "未应用", tune.ToneNeutral
		},
		func() string { return tune.PowerWarning + "　" + s.c.Power().Note },
		0,
		func(v bool) { s.c.SetModule("power", v) },
		nil,
	)
	list.Add(powerRow)

	powerRestore := s.entryRow(ui.R(PadX, 0, Wide, 96), "恢复原电源计划",
		func() (string, tune.Tone) {
			p := s.c.Power()
			if p.PriorGUID == "" {
				return "尚未记录注入前方案", tune.ToneWarn
			}
			if strings.EqualFold(p.PriorGUID, p.GUID) {
				return "当前即原方案，无需还原", tune.ToneNeutral
			}
			return "记录的原方案 " + shortGUID(p.PriorGUID), tune.ToneNeutral
		},
		func() string {
			p := s.c.Power()
			if strings.EqualFold(p.PriorGUID, p.GUID) {
				return "系统本来就在「" + p.Name + "」；工具没有改动电源方案，按 Enter 也不会制造变化。"
			}
			return "当前：" + p.Name + "（" + shortGUID(p.GUID) + "）。按 Enter 切回注入前记录的 GUID；不修改 powercfg 之外的注册表。"
		},
		"Enter 还原", func() { s.c.RestorePower() },
	)
	list.Add(powerRestore)

	aceRow := s.moduleRow(ui.R(PadX, 0, Wide, 176), "ACE 资源限制",
		func() bool { return s.c.ModuleOn("ace") },
		func() (string, tune.Tone) {
			a := s.c.ACE()
			return a.Text, a.Tone
		},
		func() string {
			a := s.c.ACE()
			if a.Detail != "" {
				return tune.ACEWarning + "　" + a.Detail
			}
			return tune.ACEWarning
		},
		1.2,
		func(v bool) { s.c.SetModule("ace", v) },
		nil,
	)
	list.Add(aceRow)

	bgRow := s.moduleRow(ui.R(PadX, 0, Wide, 136), "后台进程抑制",
		func() bool { return s.c.ModuleOn("bg") },
		func() (string, tune.Tone) {
			b := s.c.BG()
			return b.Text, b.Tone
		},
		func() string { return s.c.BG().Detail },
		0,
		func(v bool) { s.c.SetModule("bg", v) },
		nil,
	)
	list.Add(bgRow)

	procRow := s.entryRow(ui.R(PadX, 0, Wide, 110), "后台进程与读数",
		func() (string, tune.Tone) {
			b := s.c.BG()
			return b.Text, b.Tone
		},
		func() string {
			return "查看每个候选进程的 CPU / 内存占用，再决定是否抑制；系统关键进程、ACE 与本工具永不可选。"
		},
		"打开进程列表", func() { s.app.Push(s.processes()) },
	)
	list.Add(procRow)

	schedRow := s.moduleRow(ui.R(PadX, 0, Wide, 136), "系统调度",
		func() bool { return s.c.ModuleOn("schedule") },
		func() (string, tune.Tone) {
			a := s.c.Sched()
			return a.Text, a.Tone
		},
		func() string { return s.c.Sched().Detail },
		0,
		func(v bool) { s.c.SetModule("schedule", v) },
		nil,
	)
	list.Add(schedRow)

	regRow := s.entryRow(ui.R(PadX, 0, Wide, 150), "注册表外部调优",
		func() (string, tune.Tone) {
			r := s.c.Reg()
			if !r.KeyExists {
				return "未检测到设置键", tune.ToneWarn
			}
			if r.Applied {
				return "已写入 · " + r.Text, tune.ToneGood
			}
			return r.Text, tune.ToneNeutral
		},
		func() string {
			r := s.c.Reg()
			return "FOV · 帧率无上限 · 写入前自动备份；游戏运行中只读拒绝写入。" + r.Detail
		},
		"打开调优面板", func() { s.app.Push(s.registry()) },
	)
	list.Add(regRow)

	logRow := s.entryRow(ui.R(PadX, 0, Wide, 110), "运行日志与证据",
		func() (string, tune.Tone) {
			return "每次写入、回滚与注入均有时间戳", tune.ToneNeutral
		},
		func() string {
			return "记录电源 GUID 变化、注册表备份、进程恢复与 ACE 限制结果；文件位于 %APPDATA%\\CODMTune\\codmtune.log"
		},
		"打开日志", func() { s.app.Push(s.logs()) },
	)
	list.Add(logRow)

	root.Add(list.View)
	return root
}

// entryRow is a focusable card without a switch, for opening a sub-screen.
func (s *screens) entryRow(r ui.Rect, title string, status func() (string, tune.Tone), detail func() string, action string, open func()) *ui.Node {
	n := ui.NewNode(r)
	n.Focusable = true
	n.SetCursorRadius(20)
	n.Draw = func(n *ui.Node, c ui.Painter, rr ui.Rect) {
		card(c, rr, n.Press.Get(), ui.CurrentTheme.Surface)
		c.Text(ui.R(rr.X+22, rr.Y+16, rr.W-160, 26), title, 16, ui.CurrentTheme.Text, ui.AlignLeft, true)
		txt, tone := status()
		statusText(c, ui.R(rr.X+22, rr.Y+44, rr.W-330, 20), txt, tone, false)
		c.TextBlock(ui.R(rr.X+22, rr.Y+66, rr.W-230, rr.H-70), detail(), 12, ui.CurrentTheme.Muted, ui.AlignLeft, false)
		c.Text(ui.R(rr.Right()-220, rr.Bottom()-34, 200, 20), action+" →", 11, ui.CurrentTheme.PrimaryDim, ui.AlignRight, false)
	}
	n.Activate = func(*ui.Node) { open() }
	return n
}
