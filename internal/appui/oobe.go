package appui

import (
	"fmt"
	"time"

	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

// First-run wizard. It is deliberately a full-screen page instead of a modal:
// the user can still read the safety boundary before any system write happens.
func (s *screens) enterOOBE() {
	s.uiReady.Store(false)
	s.oobeOn = true
	s.oobeStep = 0
	s.oobeAt = time.Now()
	s.app.Root(s.oobe())
	go func() {
		for s.oobeOn {
			time.Sleep(33 * time.Millisecond)
			ui.Invalidate()
		}
	}()
}

func (s *screens) finishOOBE() {
	if !s.oobeOn {
		return
	}
	s.oobeOn = false
	s.c.Update(func(c *tune.Config) { c.OOBEDone = true })
	s.c.LogLine("首次引导完成")
	if s.app.OnSound != nil {
		s.app.OnSound("confirm")
	}
	s.active = "overview"
	s.app.Root(s.overview())
	s.uiReady.Store(true)
}

func (s *screens) oobe() *ui.Node {
	root := ui.NewNode(ui.R(0, 0, WinW, WinH))
	root.Focusable = false
	root.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		th := ui.CurrentTheme
		c.FillRect(r, th.Bg)
		// A restrained paper grid, drawn only while the wizard is open.
		for x := float32(0); x < WinW; x += 48 {
			c.Line(x, 0, x, WinH, 1, th.Outline)
		}
		for y := float32(0); y < WinH; y += 48 {
			c.Line(0, y, WinW, y, 1, th.Outline)
		}
		c.Line(0, 84, WinW, 84, 1, th.Outline2)
		c.FillRect(ui.R(0, 84, 280, 2), th.Primary)
		if p := logoMark(); p != "" {
			box := th.Primary
			if th.Dark {
				box = th.PrimaryDim
			}
			c.FillRect(ui.R(48, 28, 42, 42), box)
			c.DrawImage(p, ui.R(53, 33, 32, 32), 255)
		}
		c.Text(ui.R(104, 30, 300, 24), "CODM//TUNE", 17, th.Text, ui.AlignLeft, true)
		c.Text(ui.R(104, 52, 300, 18), "FIRST BOOT / FIELD MANUAL", 10, th.Muted, ui.AlignLeft, false)
		c.Text(ui.R(920, 30, 130, 18), fmt.Sprintf("0%d / 03", s.oobeStep+1), 12, th.Primary, ui.AlignRight, true)

		pages := [3][4]string{
			{"先确认边界", "不注入、不读写游戏内存、不挂起 ACE。", "只做可逆的注册表、电源与调度调优。\n每一次写入前自动导出备份，并提供恢复入口。", "第一次启动不会自动改任何系统项；\n所有危险开关只在高级模式中开放。"},
			{"把体验放在轻量工具里", "原生 Win32 / GDI+，无 CGO、无 Electron。", "空闲时主进程约 0.3% CPU、32 MB 内存；\n悬浮窗与准心是独立子进程，关掉即退出。", "电源、FOV、调度、HUD、准心都可在导航中\n单独控制，互不绑定。"},
			{"先看，再决定", "悬浮窗显示采集状态；准心可独立开关。", "第一局建议只开诊断与 HUD，确认帧率/延迟\n显示符合预期后再启用准心。", "你可以随时按 Esc 退出向导；\n之后在“设置 → 关于”重新查看说明。"},
		}
		p := pages[s.oobeStep]
		t := float32(time.Since(s.oobeAt).Seconds())
		ease := 1 - float32(1-t/0.55)
		if ease < 0 {
			ease = 0
		}
		if ease > 1 {
			ease = 1
		}
		off := (1 - ease) * 22
		c.Text(ui.R(48+off, 130, 150, 44), fmt.Sprintf("%02d", s.oobeStep+1), 38, th.Primary, ui.AlignLeft, true)
		c.Text(ui.R(48+off, 186, 900, 44), p[0], 34, th.Text, ui.AlignLeft, true)
		c.TextBlock(ui.R(48+off, 242, 880, 26), p[1], 14, th.Muted, ui.AlignLeft, false)

		c.FillRect(ui.R(48, 300, 1004, 1), th.Outline)
		for i := 0; i < 3; i++ {
			x := float32(48 + i*344)
			cy := float32(344 + (1-ease)*18)
			c.FillRect(ui.R(x, cy, 320, 168), th.Surface)
			c.Line(x, cy, x+320, cy, 2, th.Primary)
			c.Text(ui.R(x+18, cy+16, 280, 18), fmt.Sprintf("0%d / NOTE", i+1), 10, th.Primary, ui.AlignLeft, false)
			c.TextBlock(ui.R(x+18, cy+44, 284, 108), p[i+1], 13, th.Text, ui.AlignLeft, false)
		}

		// Scan line runs only while the wizard is visible; it is cheap and gives
		// the first-run page a sense of motion without animating the whole UI.
		sy := float32(120) + float32(int(time.Since(s.oobeAt).Seconds()*70)%520)
		c.Line(0, sy, WinW, sy, 1, th.Outline2)
		c.FillRect(ui.R(0, 596, WinW, 1), th.Outline)
		c.Text(ui.R(48, 612, 700, 18), "ENTER 继续  ·  ESC 跳过  ·  之后可在“设置”重新查看", 11, th.Muted, ui.AlignLeft, false)
	}

	next := ui.NewNode(ui.R(846, 602, 206, 42))
	next.Focusable = true
	next.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		th := ui.CurrentTheme
		c.FillRect(r, th.Primary)
		label := "继续  →"
		if s.oobeStep >= 2 {
			label = "开始使用  →"
		}
		c.Text(r, label, 14, th.OnPrimary, ui.AlignCenter, true)
	}
	next.Activate = func(*ui.Node) {
		if s.oobeStep >= 2 {
			s.finishOOBE()
			return
		}
		s.oobeStep++
		s.oobeAt = time.Now()
		ui.Invalidate()
	}
	next.OnKey = func(n *ui.Node, k ui.Key, ph ui.Phase) bool {
		if k == ui.KeyBack && ph == ui.Press {
			s.finishOOBE()
			return true
		}
		return false
	}
	root.Add(next)

	skip := ui.NewNode(ui.R(732, 602, 100, 42))
	skip.Focusable = true
	skip.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		th := ui.CurrentTheme
		c.Text(r, "跳过", 13, th.Muted, ui.AlignCenter, false)
	}
	skip.Activate = func(*ui.Node) { s.finishOOBE() }
	skip.OnKey = func(n *ui.Node, k ui.Key, ph ui.Phase) bool {
		if k == ui.KeyBack && ph == ui.Press {
			s.finishOOBE()
			return true
		}
		return false
	}
	root.Add(skip)
	return root
}
