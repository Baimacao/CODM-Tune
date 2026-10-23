package appui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

// ---- background processes ---------------------------------------------

func (s *screens) processes() *ui.Node {
	root := s.newRoot("后台进程抑制", "候选来自有可见窗口的进程；系统关键进程、ACE 与本工具永不可选")
	list := ui.NewList(ui.R(PadX, 112, Wide, 432), 0, 6)
	lastSig := ""
	focusName := ""

	var rebuild func()
	rebuild = func() {
		data := s.c.BG()
		focusName = ""
		if f := s.app.Focus; f != nil && f.Data != nil {
			focusName, _ = f.Data.(string)
		}
		list.Clear()
		for _, p := range data.Procs {
			p := p
			row := ui.NewNode(ui.R(PadX, 0, Wide, 58))
			row.Focusable = true
			row.SetCursorRadius(14)
			row.Data = p.Name
			row.Draw = func(n *ui.Node, c ui.Painter, rr ui.Rect) {
				card(c, rr, n.Press.Get(), ui.CurrentTheme.Surface)
				box := ui.R(rr.X+20, rr.Y+19, 20, 20)
				if p.Selected {
					c.FillRoundRect(box, 6, ui.CurrentTheme.Primary)
					c.Line(box.X+5, box.Y+10, box.X+9, box.Y+14, 2, ui.CurrentTheme.OnPrimary)
					c.Line(box.X+9, box.Y+14, box.X+16, box.Y+6, 2, ui.CurrentTheme.OnPrimary)
				} else {
					c.StrokeRoundRect(box, 6, 1.5, ui.CurrentTheme.Outline2)
				}
				c.Text(ui.R(rr.X+54, rr.Y+10, 460, 22), p.Name, 14, ui.CurrentTheme.Text, ui.AlignLeft, true)
				c.Text(ui.R(rr.X+54, rr.Y+32, 460, 18), fmt.Sprintf("%d 个进程", p.PIDs), 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
				right := fmt.Sprintf("CPU %.1f%%   %.0f MB", p.CPU, p.MemMB)
				c.Text(ui.R(rr.Right()-330, rr.Y+19, 300, 22), right, 13, ui.CurrentTheme.Text, ui.AlignRight, false)
				if p.Applied {
					chip(c, ui.R(rr.Right()-160, rr.Y+16, 128, 26), "已抑制", ui.CurrentTheme.Success)
				} else if p.Selected {
					chip(c, ui.R(rr.Right()-160, rr.Y+16, 128, 26), "已选择", ui.CurrentTheme.Primary)
				}
			}
			row.Activate = func(n *ui.Node) {
				s.c.ToggleProcess(p.Name, !p.Selected)
			}
			list.Add(row)
		}
		if focusName != "" {
			for _, r := range list.Rows() {
				if r.Data == focusName {
					s.app.SetFocus(r)
				}
			}
		}
	}

	refreshBtn := s.button(ui.R(PadX, 560, 150, 38), "刷新读数", btnGhost, func() {
		s.c.RefreshProcesses()
		rebuild()
	})
	restoreBtn := s.button(ui.R(PadX+166, 560, 150, 38), "恢复全部", btnGhost, func() {
		s.c.RestoreBackgroundNow()
	})
	hint := ui.NewNode(ui.R(PadX+336, 560, Wide-336, 38))
	hint.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		b := s.c.BG()
		c.Text(ui.R(r.X, r.Y, r.W, r.H), b.Text+" · "+b.Detail, 11, ui.CurrentTheme.Muted, ui.AlignRight, false)
	}
	root.Add(list.View, refreshBtn, restoreBtn, hint)

	root.Update = func(n *ui.Node, dt float32) {
		sig := procSig(s.c.BG())
		if sig != lastSig {
			lastSig = sig
			rebuild()
		}
	}
	s.c.RefreshProcesses()
	lastSig = procSig(s.c.BG())
	rebuild()
	return root
}

func procSig(b tune.BgState) string {
	var sb strings.Builder
	for _, p := range b.Procs {
		fmt.Fprintf(&sb, "%s|%v|%v|%d,", p.Name, p.Selected, p.Applied, p.PIDs)
	}
	return sb.String()
}

// ---- registry tweaks ---------------------------------------------------

var fovOptions = []int{90, 96, 103, 110, 120}

func (s *screens) registry() *ui.Node {
	root := s.newRoot("注册表外部调优", "只写 HKCU\\SOFTWARE\\Tencent\\Call-of-Duty；游戏运行中只读拒绝写入")

	statusCard := ui.NewNode(ui.R(PadX, 112, Wide, 112))
	statusCard.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		card(c, r, 0, ui.CurrentTheme.Surface)
		reg := s.c.Reg()
		c.Text(ui.R(r.X+22, r.Y+16, 420, 22), "目标键状态", 15, ui.CurrentTheme.Text, ui.AlignLeft, true)
		if !reg.KeyExists {
			statusText(c, ui.R(r.X+22, r.Y+44, 760, 20), "未检测到 HKCU\\SOFTWARE\\Tencent\\Call-of-Duty", tune.ToneBad, true)
			c.TextBlock(ui.R(r.X+22, r.Y+70, r.W-44, 34), "请先正常进入一次游戏，并在游戏内拖动灵敏度、打开帧数微调，让键存在后再回来。", 12, ui.CurrentTheme.Muted, ui.AlignLeft, false)
			return
		}
		fps := "帧率微调键：未发现"
		tone := tune.ToneWarn
		if reg.FPSKey != "" {
			fps = "帧率微调键：已发现"
			tone = tune.ToneGood
		}
		statusText(c, ui.R(r.X+22, r.Y+44, 420, 20), fps, tone, true)
		c.Text(ui.R(r.X+470, r.Y+42, 520, 20), fmt.Sprintf("FOV 键：%d 个 · 当前帧率值 %d", len(reg.FOVKeys), reg.FPSValue), 12, ui.CurrentTheme.Muted, ui.AlignRight, false)
		c.TextBlock(ui.R(r.X+22, r.Y+70, r.W-44, 34), reg.Detail, 12, ui.CurrentTheme.Muted, ui.AlignLeft, false)
	}
	root.Add(statusCard)

	fovCard := ui.NewNode(ui.R(PadX, 240, 600, 150))
	fovCard.Focusable = true
	fovCard.SetCursorRadius(20)
	fovCard.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		card(c, r, n.Press.Get(), ui.CurrentTheme.Surface)
		c.Text(ui.R(r.X+22, r.Y+16, 300, 22), "FOV 视场角", 15, ui.CurrentTheme.Text, ui.AlignLeft, true)
		c.Text(ui.R(r.X+22, r.Y+48, 180, 50), fmt.Sprintf("%d°", s.c.FOV()), 40, ui.CurrentTheme.Primary, ui.AlignLeft, true)
		c.Text(ui.R(r.X+200, r.Y+56, 380, 20), "← → 调节（90–120）", 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
		c.TextBlock(ui.R(r.X+22, r.Y+104, r.W-44, 40), "写入游戏 MainCameraFovInGame 键；重启游戏生效。超过 90 属实验性，恢复请回游戏内拖动滑块。", 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
	}
	fovCard.OnKey = func(n *ui.Node, k ui.Key, ph ui.Phase) bool {
		if k == ui.KeyRight || k == ui.KeyLeft {
			if ph != ui.Release {
				cur := s.c.FOV()
				idx := 0
				for i, v := range fovOptions {
					if v == cur {
						idx = i
					}
				}
				if k == ui.KeyRight {
					idx = minInt(idx+1, len(fovOptions)-1)
				} else {
					idx = maxInt(idx-1, 0)
				}
				s.c.SetFOV(fovOptions[idx])
			}
			return true
		}
		return false
	}
	root.Add(fovCard)

	fpsCard := ui.NewNode(ui.R(PadX+620, 240, 400, 150))
	fpsCard.Focusable = true
	fpsCard.SetCursorRadius(20)
	fpsToggle, _ := s.toggle(ui.R(PadX+620+330, 240+18, 52, 30), func() bool { return s.c.FPSUnlimited() })
	fpsCard.Add(fpsToggle)
	fpsCard.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		card(c, r, n.Press.Get(), ui.CurrentTheme.Surface)
		c.Text(ui.R(r.X+22, r.Y+16, 300, 22), "帧率无上限", 15, ui.CurrentTheme.Text, ui.AlignLeft, true)
		reg := s.c.Reg()
		text := "写入 0 = 不限制帧率"
		tone := tune.ToneGood
		if reg.FPSKey == "" {
			text = "缺少帧率键：先在游戏内开启帧数微调"
			tone = tune.ToneWarn
		}
		statusText(c, ui.R(r.X+22, r.Y+48, r.W-60, 20), text, tone, false)
		c.TextBlock(ui.R(r.X+22, r.Y+80, r.W-44, 50), "仅当键存在时写入；应用后需重启游戏。", 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
	}
	fpsCard.Activate = func(*ui.Node) { s.c.SetFPSUnlimited(!s.c.FPSUnlimited()) }
	root.Add(fpsCard)

	running := func() bool {
		_, _, _, r, _ := s.c.Game()
		return r
	}
	available := func() bool {
		reg := s.c.Reg()
		return !running() && reg.KeyExists && len(reg.FOVKeys) > 0
	}
	apply := s.holdButton(ui.R(PadX, 410, 320, 54), "按住写入注册表", 1.2, func() bool { return !available() }, func() {
		s.c.ApplyRegistry()
	})
	restore := s.holdButton(ui.R(PadX+340, 410, 300, 54), "按住从备份还原", 1.2, func() bool {
		reg := s.c.Reg()
		return running() || reg.Backup == ""
	}, func() {
		s.c.RestoreRegistry()
	})
	refresh := s.button(ui.R(PadX+660, 410, 200, 54), "刷新键状态", btnGhost, func() {
		s.c.RefreshRegistry()
	})
	root.Add(apply, restore, refresh)

	warn := ui.NewNode(ui.R(PadX, 484, Wide, 86))
	warn.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		if running() {
			rr := r
			c.FillRoundRect(rr, 18, ui.CurrentTheme.DangerBg)
			c.StrokeRoundRect(rr, 18, 1, ui.CurrentTheme.Danger)
			c.Text(ui.R(rr.X+20, rr.Y+12, rr.W-40, 22), "检测到 CODM 运行中：注册表模块只读，拒绝写入", 14, ui.CurrentTheme.Danger, ui.AlignLeft, true)
			c.TextBlock(ui.R(rr.X+20, rr.Y+40, rr.W-40, 38), "关闭游戏后重新进入本面板即可写入。FOV 恢复请回游戏内拖动滑块。", 12, ui.CurrentTheme.Danger, ui.AlignLeft, false)
			return
		}
		card(c, r, 0, ui.CurrentTheme.Surface2)
		c.Text(ui.R(r.X+20, r.Y+12, r.W-40, 22), "写入前自动 reg export 备份；FOV 的官方恢复路径是游戏内滑块", 13, ui.CurrentTheme.Text, ui.AlignLeft, true)
		c.TextBlock(ui.R(r.X+20, r.Y+40, r.W-40, 38), "帧率无上限依赖 FramerateCustomizeValue 键：请先在游戏内打开帧数微调并拖动一次。", 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
	}
	root.Add(warn)

	backup := ui.NewNode(ui.R(PadX, 580, Wide, 60))
	backup.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		reg := s.c.Reg()
		c.Text(ui.R(r.X, r.Y, r.W, 20), "备份："+reg.Backup, 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
		c.Text(ui.R(r.X, r.Y+22, r.W, 20), "键：+"+strings.Join(reg.FOVKeys, "  +"), 10, ui.CurrentTheme.Muted, ui.AlignLeft, false)
	}
	root.Add(backup)

	go s.c.RefreshRegistry()
	return root
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ---- GPU and driver panel ---------------------------------------------

func (s *screens) gpu() *ui.Node {
	root := s.newRoot("显卡与运行时", "DXGI 适配器枚举 · 每进程 GPU 引擎计数标注实际 GPU · 4101 事件翻译")
	list := ui.NewList(ui.R(PadX, 112, Wide, FootY-112-8), 0, 12)

	refresh := s.button(ui.R(PadX, 0, 200, 42), "刷新驱动信息", btnGhost, func() {
		s.c.RefreshGPU()
	})
	list.Add(refresh)

	note := ui.NewNode(ui.R(PadX, 0, Wide, 74))
	note.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		card(c, r, 0, ui.CurrentTheme.Surface)
		g := s.c.GPU()
		text := g.Note
		if text == "" {
			text = "正在枚举显卡…"
		}
		c.Text(ui.R(r.X+20, r.Y+18, r.W-40, 20), "实际使用", 12, ui.CurrentTheme.Muted, ui.AlignLeft, false)
		c.Text(ui.R(r.X+20, r.Y+40, r.W-40, 24), text, 14, ui.CurrentTheme.Text, ui.AlignLeft, true)
	}
	list.Add(note)

	for i := 0; i < 3; i++ {
		i := i
		row := ui.NewNode(ui.R(PadX, 0, Wide, 82))
		row.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
			g := s.c.GPU()
			if i >= len(g.Adapters) {
				return
			}
			a := g.Adapters[i]
			card(c, r, 0, ui.CurrentTheme.Surface)
			c.Text(ui.R(r.X+20, r.Y+14, 560, 24), a.Name, 15, ui.CurrentTheme.Text, ui.AlignLeft, true)
			c.Text(ui.R(r.X+20, r.Y+44, 560, 20), fmt.Sprintf("%s · 驱动 %s · 显存 %.1f GB", a.Vendor, a.Driver, a.VRAMGB), 12, ui.CurrentTheme.Muted, ui.AlignLeft, false)
			if a.Used {
				chip(c, ui.R(r.Right()-200, r.Y+24, 180, 28), "CODM 实际使用", ui.CurrentTheme.Success)
			}
		}
		list.Add(row)
	}

	events := ui.NewNode(ui.R(PadX, 0, Wide, 150))
	events.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		card(c, r, 0, ui.CurrentTheme.Surface)
		c.Text(ui.R(r.X+20, r.Y+14, 400, 22), "Event ID 4101 · 显示驱动恢复", 15, ui.CurrentTheme.Text, ui.AlignLeft, true)
		g := s.c.GPU()
		if len(g.Events) == 0 {
			c.Text(ui.R(r.X+20, r.Y+48, r.W-40, 20), "最近没有 4101 事件", 12, ui.CurrentTheme.Muted, ui.AlignLeft, false)
			return
		}
		for i, e := range g.Events {
			if i >= 3 || r.Y+48+float32(i)*32 > r.Bottom()-20 {
				break
			}
			c.Text(ui.R(r.X+20, r.Y+46+float32(i)*30, 220, 20), e.When, 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
			c.Text(ui.R(r.X+220, r.Y+46+float32(i)*30, 780, 20), e.Module+" · "+e.Advice, 11, ui.CurrentTheme.Text, ui.AlignLeft, false)
		}
	}
	list.Add(events)

	tdr := s.moduleRow(ui.R(PadX, 0, Wide, 150), "TdrDelay = 8 秒",
		func() bool { return s.c.GPU().TdrOn },
		func() (string, tune.Tone) {
			if s.c.GPU().TdrOn {
				return "已写入 · 重启后生效", tune.ToneWarn
			}
			return "未写入 · 默认关闭", tune.ToneNeutral
		},
		func() string {
			return "写入 HKLM\\SYSTEM\\CurrentControlSet\\Control\\GraphicsDrivers\\TdrDelay，备份原值。这是缓冲，不是修复：驱动真崩时只会让黑屏更久。需要管理员。"
		},
		1.2,
		func(v bool) { s.c.SetTdr(v) },
		nil,
	)
	list.Add(tdr)

	deps := ui.NewNode(ui.R(PadX, 0, Wide, 116))
	deps.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		card(c, r, 0, ui.CurrentTheme.Surface)
		c.Text(ui.R(r.X+20, r.Y+14, 500, 22), "运行时依赖（仅检测，不自动安装）", 15, ui.CurrentTheme.Text, ui.AlignLeft, true)
		g := s.c.GPU()
		x := r.X + 20
		for _, d := range g.Deps {
			w := float32(len(d.Name))*7 + 44
			col := ui.CurrentTheme.Success
			txt := d.Name
			if !d.OK {
				col = ui.CurrentTheme.Danger
				txt = d.Name + " 缺失"
			}
			c.FillRoundRect(ui.R(x, r.Y+52, w, 28), 14, col.WithAlpha(28))
			c.Text(ui.R(x, r.Y+52, w, 28), txt, 11, col, ui.AlignCenter, false)
			x += w + 8
		}
		c.Text(ui.R(r.X+20, r.Y+88, r.W-40, 18), "缺失时请到微软官方下载 VC++ 运行库 / DirectX 终端运行时。", 10, ui.CurrentTheme.Muted, ui.AlignLeft, false)
	}
	list.Add(deps)

	root.Add(list.View)
	s.c.RefreshGPU()
	return root
}

// ---- read-only network ------------------------------------------------

func (s *screens) net() *ui.Node {
	root := s.newRoot("只读网络诊断", "DNS 解析延迟 + CODM 进程 TCP 端点握手 RTT；不修改 hosts/DNS、不装虚拟网卡")
	list := ui.NewList(ui.R(PadX, 112, Wide, FootY-112-8), 0, 12)

	dns := ui.NewNode(ui.R(PadX, 0, Wide, 120))
	dns.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		card(c, r, 0, ui.CurrentTheme.Surface)
		net := s.c.Net()
		c.Text(ui.R(r.X+20, r.Y+14, 400, 22), "DNS 解析 · "+net.Host, 15, ui.CurrentTheme.Text, ui.AlignLeft, true)
		text := net.DNSTime
		tone := tune.ToneGood
		if net.DNSErr != "" {
			text = "失败：" + net.DNSErr
			tone = tune.ToneBad
		}
		statusText(c, ui.R(r.X+20, r.Y+46, 500, 22), text, tone, true)
		c.Text(ui.R(r.X+20, r.Y+76, r.W-40, 20), net.DNSAddrs, 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
	}
	list.Add(dns)

	eps := ui.NewNode(ui.R(PadX, 0, Wide, 250))
	eps.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		card(c, r, 0, ui.CurrentTheme.Surface)
		net := s.c.Net()
		c.Text(ui.R(r.X+20, r.Y+14, 500, 22), "CODM TCP 端点与握手 RTT", 15, ui.CurrentTheme.Text, ui.AlignLeft, true)
		if len(net.Endpoints) == 0 {
			c.Text(ui.R(r.X+20, r.Y+48, r.W-40, 20), "游戏未运行时无法读取端点；只统计 CODM 进程的 TCP 连接。", 12, ui.CurrentTheme.Muted, ui.AlignLeft, false)
			return
		}
		for i, e := range net.Endpoints {
			if i >= 5 {
				break
			}
			y := r.Y + 46 + float32(i)*34
			c.Text(ui.R(r.X+20, y, 460, 20), e.Remote, 12, ui.CurrentTheme.Text, ui.AlignLeft, false)
			val := e.RTT
			col := ui.CurrentTheme.Success
			if val == "" {
				val = e.Err
				col = ui.CurrentTheme.Muted
			}
			c.Text(ui.R(r.X+500, y, 460, 20), val, 12, col, ui.AlignLeft, false)
		}
	}
	list.Add(eps)

	note := ui.NewNode(ui.R(PadX, 0, Wide, 100))
	note.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		card(c, r, 0, ui.CurrentTheme.Surface2)
		c.Text(ui.R(r.X+20, r.Y+14, r.W-40, 22), "边界", 14, ui.CurrentTheme.Text, ui.AlignLeft, true)
		c.TextBlock(ui.R(r.X+20, r.Y+42, r.W-40, 46), "只读探测：不修改 hosts / DNS，不安装虚拟网卡，不集成加速器，不对游戏收发任何数据包。TCP 握手仅用于测量，连接后立即关闭。", 12, ui.CurrentTheme.Muted, ui.AlignLeft, false)
	}
	list.Add(note)

	refresh := s.button(ui.R(PadX, 0, 200, 42), "重新探测", btnGhost, func() {
		s.c.RefreshNet()
	})
	list.Add(refresh)

	root.Add(list.View)
	s.c.RefreshNet()
	return root
}

// ---- install picker ---------------------------------------------------

func (s *screens) candidates() *ui.Node {
	root := s.newRoot("选择 CODM 安装", "探测到多个候选目录时必须由你选择；不会替你随便挑一个")
	list := ui.NewList(ui.R(PadX, 112, Wide, 420), 0, 10)
	last := -1

	var rebuild func()
	rebuild = func() {
		cands, detecting, _ := s.c.Candidates()
		ver := len(cands)*1000 + s.c.Version()
		if ver == last {
			return
		}
		last = ver
		list.Clear()
		for _, cd := range cands {
			cd := cd
			row := ui.NewNode(ui.R(PadX, 0, Wide, 84))
			row.Focusable = true
			row.SetCursorRadius(16)
			row.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
				card(c, r, n.Press.Get(), ui.CurrentTheme.Surface)
				c.Text(ui.R(r.X+20, r.Y+14, r.W-40, 22), cd.GameExe, 14, ui.CurrentTheme.Text, ui.AlignLeft, true)
				c.Text(ui.R(r.X+20, r.Y+44, r.W-40, 20), cd.InstallDir, 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
				if cd.Launcher != "" {
					c.Text(ui.R(r.X+20, r.Y+64, r.W-40, 16), "启动器："+cd.Launcher, 10, ui.CurrentTheme.Muted, ui.AlignLeft, false)
				}
			}
			row.Activate = func(*ui.Node) {
				s.c.SelectCandidate(tune.Candidate{GameExe: cd.GameExe, InstallDir: cd.InstallDir, Launcher: cd.Launcher})
				s.app.Pop()
			}
			list.Add(row)
		}
		if len(cands) == 0 {
			row := ui.NewNode(ui.R(PadX, 0, Wide, 84))
			row.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
				card(c, r, 0, ui.CurrentTheme.Surface)
				c.Text(ui.R(r.X+20, r.Y+30, r.W-40, 24), "没有候选，使用下方手动选择目录", 13, ui.CurrentTheme.Muted, ui.AlignLeft, false)
			}
			list.Add(row)
		}
		_ = detecting
	}

	browse := s.button(ui.R(PadX, 548, 240, 42), "手动选择目录", btnGhost, func() {
		s.app.Push(s.folder(startDir()))
	})
	root.Add(list.View, browse)
	root.Update = func(*ui.Node, float32) { rebuild() }
	rebuild()
	return root
}

func startDir() string {
	if st, err := os.Stat(`D:\`); err == nil && st.IsDir() {
		return `D:\`
	}
	return `C:\`
}

type folderState struct {
	path string
}

func (s *screens) folder(path string) *ui.Node {
	st := &folderState{path: path}
	root := s.newRoot("手动选择目录", "导航到包含 CODM.exe（旁边有 CODM_Data）的目录，再按“使用此目录”")
	pathLabel := ui.NewNode(ui.R(PadX, 106, Wide, 30))
	pathLabel.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		c.Text(ui.R(r.X, r.Y+6, r.W, 20), st.path, 12, ui.CurrentTheme.Primary, ui.AlignLeft, false)
	}
	list := ui.NewList(ui.R(PadX, 142, Wide, 440), 0, 6)

	var rebuild func()
	rebuild = func() {
		list.Clear()
		entries, err := os.ReadDir(st.path)
		dirs := make([]string, 0, 64)
		hasGame := false
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				name := e.Name()
				if strings.HasPrefix(name, "$") || strings.EqualFold(name, "System Volume Information") ||
					strings.EqualFold(name, "RECYCLER") || strings.HasPrefix(name, ".") {
					continue
				}
				dirs = append(dirs, name)
			}
		}
		sort.Slice(dirs, func(i, j int) bool { return strings.ToLower(dirs[i]) < strings.ToLower(dirs[j]) })
		if len(dirs) > 60 {
			dirs = dirs[:60]
		}
		for _, name := range dirs {
			name := name
			if strings.EqualFold(name, "CODM_Data") {
				continue
			}
			row := ui.NewNode(ui.R(PadX, 0, Wide, 44))
			row.Focusable = true
			row.SetCursorRadius(12)
			row.Data = name
			row.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
				card(c, r, n.Press.Get(), ui.CurrentTheme.Surface)
				c.Text(ui.R(r.X+18, r.Y, r.W-36, r.H), "▸ "+name, 13, ui.CurrentTheme.Text, ui.AlignLeft, false)
			}
			row.Activate = func(*ui.Node) {
				st.path = filepath.Join(st.path, name)
				rebuild()
			}
			list.Add(row)
		}
		// CODM.exe in this directory?
		exe := filepath.Join(st.path, "CODM.exe")
		if fileExistsUI(exe) && dirExists(filepath.Join(st.path, "CODM_Data")) {
			hasGame = true
			row := ui.NewNode(ui.R(PadX, 0, Wide, 52))
			row.Focusable = true
			row.SetCursorRadius(14)
			row.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
				c.FillRoundRect(r, 18, ui.CurrentTheme.SuccessBg)
				c.StrokeRoundRect(r, 18, 1, ui.CurrentTheme.Success)
				c.Text(ui.R(r.X+18, r.Y, r.W-36, r.H), "✓ 使用此目录（找到 CODM.exe）", 14, ui.CurrentTheme.Success, ui.AlignLeft, true)
			}
			row.Activate = func(*ui.Node) {
				s.c.SelectDir(st.path)
				s.app.Pop()
				s.app.Pop()
			}
			list.Insert(row, 0)
		}
		if !hasGame {
			hint := ui.NewNode(ui.R(PadX, 0, Wide, 40))
			hint.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
				c.Text(ui.R(r.X, r.Y, r.W, r.H), "此目录没有 CODM.exe / CODM_Data，请继续进入子目录", 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
			}
			list.Add(hint)
		}
	}
	up := s.button(ui.R(PadX, 594, 140, 40), "上一级", btnGhost, func() {
		parent := filepath.Dir(strings.TrimRight(st.path, `\`))
		if parent != st.path && parent != "" {
			st.path = parent
			rebuild()
		}
	})
	back := s.button(ui.R(PadX+156, 594, 140, 40), "返回", btnGhost, func() { s.app.Pop() })
	root.Add(pathLabel, list.View, up, back)
	rebuild()
	return root
}

func fileExistsUI(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// ---- logs -------------------------------------------------------------

func (s *screens) logs() *ui.Node {
	root := s.newRoot("运行日志与证据", "每次写入、回滚、注入都会留下时间戳；日志同时写入 %APPDATA%\\CODMTune\\codmtune.log")
	cardNode := ui.NewNode(ui.R(PadX, 112, Wide, FootY-112-8))
	lastVer := -1
	lines := []string{}
	cardNode.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		card(c, r, 0, ui.CurrentTheme.Surface)
		y := r.Y + 16
		for i := len(lines) - 1; i >= 0 && y < r.Bottom()-18; i-- {
			c.Text(ui.R(r.X+20, y, r.W-40, 18), lines[i], 10, ui.CurrentTheme.Muted, ui.AlignLeft, false)
			y += 18
		}
		if len(lines) == 0 {
			c.Text(ui.R(r.X+20, r.Y+20, r.W-40, 20), "暂无日志", 12, ui.CurrentTheme.Muted, ui.AlignLeft, false)
		}
	}
	cardNode.Update = func(*ui.Node, float32) {
		if v := s.c.Version(); v != lastVer {
			lastVer = v
			lines = s.c.Logs()
			if len(lines) > 60 {
				lines = lines[len(lines)-60:]
			}
		}
	}
	root.Add(cardNode)
	lines = s.c.Logs()
	return root
}
