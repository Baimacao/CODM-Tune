package appui

import (
	"fmt"
	"strings"

	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

// overview is the dashboard: install, launch and live module tiles.
func (s *screens) overview() *ui.Node {
	root := s.cat("overview", "概览", "直接启动、进程调度、电源注入、驱动修复与只读网络诊断")

	hero := ui.NewNode(ui.R(PadX, 112, Wide, 168))
	hero.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		th := ui.CurrentTheme
		card(c, r, 0, th.Surface)
		exe, _, _, running, pid := s.c.Game()
		c.Text(ui.R(r.X+22, r.Y+16, 420, 24), "CODM 安装", 16, th.Text, ui.AlignLeft, true)
		if exe == "" {
			c.Text(ui.R(r.X+22, r.Y+48, r.W-60, 20), "尚未探测到安装目录", 13, th.Muted, ui.AlignLeft, false)
			chip(c, ui.R(r.Right()-250, r.Y+14, 230, 28), "等待探测", th.Danger)
		} else {
			c.Text(ui.R(r.X+22, r.Y+48, r.W-300, 20), exe, 13, th.Muted, ui.AlignLeft, false)
			if running {
				chip(c, ui.R(r.Right()-250, r.Y+14, 230, 28), fmt.Sprintf("游戏运行中 · pid %d", pid), th.Success)
			} else {
				chip(c, ui.R(r.Right()-250, r.Y+14, 230, 28), "已就绪 · 可直接启动", th.Primary)
			}
		}
	}
	launch := s.button(ui.R(PadX+22, 206, 176, 46), "启动 CODM", btnPrimary, func() {
		s.c.Launch()
		if s.c.LaunchError() == "" && s.app.OnSound != nil {
			s.app.OnSound("launch")
		}
	})
	redetect := s.button(ui.R(PadX+214, 212, 116, 34), "重新探测", btnGhost, func() { s.c.Detect() })
	openDir := s.button(ui.R(PadX+342, 212, 116, 34), "打开目录", btnGhost, func() { s.c.OpenFolder() })
	cands, _, _ := s.c.Candidates()
	candBtn := s.button(ui.R(PadX+470, 212, 170, 34), fmt.Sprintf("选择安装 (%d)", len(cands)), btnGhost, func() {
		s.app.Push(s.candidates())
	})
	candBtn.Update = func(*ui.Node, float32) {
		list, _, _ := s.c.Candidates()
		candBtn.Visible = len(list) > 1
	}
	settings := s.button(ui.R(PadX+656, 212, 150, 34), "外观与设置", btnGhost, func() { s.switchTo("settings") })
	hero.Add(launch, redetect, openDir, candBtn, settings)
	root.Add(hero)

	if count := s.enabledCount(); count >= 5 {
		warn := ui.NewNode(ui.R(PadX, 288, Wide, 20))
		warn.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
			th := ui.CurrentTheme
			if th.Style == "endfield" {
				// A caution line is a mark block plus ink type; red is
				// reserved for destructive rows, and the accent may not be
				// used as a line or as type in this style.
				c.FillRect(ui.R(r.X, r.Y+5, 3, 12), th.Mark)
				c.Text(ui.R(r.X+9, r.Y, r.W-9, r.H), fmt.Sprintf("已启用 %d 项调整，可能影响帧稳定性；工具自身仍保持在 128 MB 以内", count), 11, th.Text, ui.AlignLeft, true)
				return
			}
			c.Text(ui.R(r.X, r.Y, r.W, r.H), fmt.Sprintf("已启用 %d 项调整，可能影响帧稳定性；工具自身仍保持在 128 MB 以内", count), 11, th.Danger, ui.AlignLeft, true)
		}
		root.Add(warn)
	}

	gx := []float32{PadX, PadX + 291, PadX + 582}
	gy := []float32{312, 482}
	tiles := []*ui.Node{
		s.tile(ui.R(gx[0], gy[0], 270, 150), "电源计划", "bolt", func() (string, string, tune.Tone) {
			p := s.c.Power()
			if p.Name == "" {
				return "读取中", "卓越性能注入", tune.ToneNeutral
			}
			d := "卓越性能"
			if p.Injected {
				d = "已注入 · 原 " + shortGUID(p.PriorGUID)
			}
			return p.Name, d, tune.ToneGood
		}, func() { s.switchTo("performance") }),
		s.tile(ui.R(gx[1], gy[0], 270, 150), "ACE 资源限制", "shield", func() (string, string, tune.Tone) {
			a := s.c.ACE()
			return a.Text, "Idle · 单核 · 60s", a.Tone
		}, func() { s.switchTo("performance") }),
		s.tile(ui.R(gx[2], gy[0], 270, 150), "系统调度", "chip", func() (string, string, tune.Tone) {
			a := s.c.Sched()
			return a.Text, "High · 排除 CPU 0", a.Tone
		}, func() { s.switchTo("performance") }),
		s.tile(ui.R(gx[0], gy[1], 270, 150), "后台抑制", "window", func() (string, string, tune.Tone) {
			b := s.c.BG()
			return b.Text, "退出自动恢复", b.Tone
		}, func() { s.switchTo("performance") }),
		s.tile(ui.R(gx[1], gy[1], 270, 150), "显卡与驱动", "monitor", func() (string, string, tune.Tone) {
			g := s.c.GPU()
			if !g.Checked {
				return "未扫描", "DXGI · 4101 · TdrDelay", tune.ToneNeutral
			}
			name := g.Actual
			if name == "" && len(g.Adapters) > 0 {
				name = g.Adapters[0].Name
			}
			return name, fmt.Sprintf("事件 %d · 运行库 %d/6", len(g.Events), okDeps(g.Deps)), tune.ToneNeutral
		}, func() { s.switchTo("diagnostics") }),
		s.tile(ui.R(gx[2], gy[1], 270, 150), "悬浮窗", "keyboard", func() (string, string, tune.Tone) {
			cfg := s.c.Cfg()
			if cfg.OverlayEnabled {
				return "已启用", "FPS · 1% low · RTT · 按键", tune.ToneGood
			}
			return "未启用", "游戏内实时 HUD", tune.ToneNeutral
		}, func() { s.switchTo("overlay") }),
	}
	for _, t := range tiles {
		root.Add(t)
	}
	return root
}

func (s *screens) tile(r ui.Rect, title, icon string, get func() (string, string, tune.Tone), open func()) *ui.Node {
	n := ui.NewNode(r)
	n.Focusable = true
	n.SetCursorRadius(ui.CurrentTheme.CardRadius)
	n.Draw = func(n *ui.Node, c ui.Painter, rr ui.Rect) {
		th := ui.CurrentTheme
		card(c, rr, n.Press.Get(), th.Surface)
		status, detail, tone := get()
		col := colorOf(tone)
		if th.Style == "endfield" {
			// Icon plate is a hairline square, not a tinted disc: rounded
			// plates would read as the MD3E family. The tone shows up as a
			// mark bar beside the status line, which is the one place a block
			// of accent is allowed to mean "needs attention".
			r := ui.R(rr.X+16, rr.Y+14, 34, 34)
			c.Line(r.X, r.Y, r.Right(), r.Y, 1, th.Outline2)
			c.Line(r.X, r.Bottom(), r.Right(), r.Bottom(), 1, th.Outline2)
			c.Line(r.X, r.Y, r.X, r.Bottom(), 1, th.Outline2)
			c.Line(r.Right(), r.Y, r.Right(), r.Bottom(), 1, th.Outline2)
			drawIcon(c, ui.R(rr.X+24, rr.Y+22, 18, 18), icon, th.Text)
			if tone != tune.ToneGood {
				c.FillRect(ui.R(rr.X+16, rr.Y+63, 3, 16), th.Mark)
			}
			c.Text(ui.R(rr.X+58, rr.Y+14, rr.W-72, 22), title, 13, th.Text, ui.AlignLeft, true)
			// 230 px of room at 12 px type fits about 18 CJK glyphs; cutting
			// at 14 dropped the distinguishing half of strings like
			// "限制中 · 2 个 ACE 进程独立".
			if runes := []rune(status); len(runes) > 18 {
				status = string(runes[:17]) + "…"
			}
			c.Text(ui.R(rr.X+24, rr.Y+62, rr.W-40, 20), status, 12, th.Text, ui.AlignLeft, true)
			c.TextBlock(ui.R(rr.X+16, rr.Y+90, rr.W-32, 40), detail, 11, th.Muted, ui.AlignLeft, false)
			c.Text(ui.R(rr.X+16, rr.Bottom()-24, rr.W-32, 16), "打开 →", 10, th.Muted, ui.AlignRight, false)
			return
		}
		c.FillRoundRect(ui.R(rr.X+16, rr.Y+14, 34, 34), 11, col.WithAlpha(26))
		drawIcon(c, ui.R(rr.X+22, rr.Y+20, 22, 22), icon, col)
		c.Text(ui.R(rr.X+58, rr.Y+14, rr.W-72, 22), title, 14, th.Text, ui.AlignLeft, true)
		if runes := []rune(status); len(runes) > 18 {
			status = string(runes[:17]) + "…"
		}
		c.Text(ui.R(rr.X+16, rr.Y+62, rr.W-32, 20), status, 13, col, ui.AlignLeft, true)
		c.TextBlock(ui.R(rr.X+16, rr.Y+88, rr.W-32, 40), detail, 11, th.Muted, ui.AlignLeft, false)
		c.Text(ui.R(rr.X+16, rr.Bottom()-24, rr.W-32, 16), "Enter 打开 →", 10, th.PrimaryDim, ui.AlignRight, false)
	}
	n.Activate = func(*ui.Node) { open() }
	return n
}

var _ = strings.TrimSpace
