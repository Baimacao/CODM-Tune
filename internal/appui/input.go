package appui

import (
	"fmt"

	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

// inputCat groups mouse, launch-parameter and sensitivity work so the game
// category stays about launching and detecting the install.
func (s *screens) inputCat() *ui.Node {
	root := s.cat("input", "输入与鼠标", "鼠标加速、视角启动参数、Unity 渲染与灵敏度快照")
	l := s.listIn(root)

	s.section(l, "鼠标")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 100), feature{
		icon: "mouse", title: "禁用 Windows 鼠标加速",
		desc: "写入 Control Panel\\Mouse 的三个阈值并立即 SPI_SETMOUSE；关闭增强指针精度，恢复原值。",
		value: func() string {
			switch {
			case s.c.MouseAccelOn():
				return "已由本工具关闭"
			case s.c.MouseAccelSystemOff():
				return "系统已关闭（非本工具）"
			}
			return "系统默认（可能有加速）"
		},
		tone: func() tune.Tone {
			if s.c.MouseAccelOn() || s.c.MouseAccelSystemOff() {
				return tune.ToneGood
			}
			return tune.ToneNeutral
		},
		toggle:   func() bool { return s.c.MouseAccelOn() },
		onToggle: func(on bool) { s.c.SetMouseAccel(on) },
	}))

	s.section(l, "启动参数")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "gamepad", title: "视角手感启动参数",
		desc: "-noforcemaccel -noforcemspd -useforcemparms；来源视频，引擎是否读取未在本机对局验证。",
		value: func() string {
			if s.c.HasLaunchToken("-noforcemaccel") {
				return "已附加"
			}
			return "未附加"
		},
		toggle: func() bool { return s.c.HasLaunchToken("-noforcemaccel") },
		onToggle: func(on bool) {
			s.c.SetLaunchToken("-noforcemaccel", on)
			s.c.SetLaunchToken("-noforcemspd", on)
			s.c.SetLaunchToken("-useforcemparms", on)
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "bolt", title: "Unity 渲染参数",
		desc: "-force-d3d11 可绕开部分 D3D12 驱动问题；把当前 LaunchArgs 显示在这里。",
		value: func() string {
			a := s.c.LaunchArgs()
			if a == "" {
				return "无额外参数"
			}
			return a
		},
		toggle:   func() bool { return s.c.HasLaunchToken("-force-d3d11") },
		onToggle: func(on bool) { s.c.SetLaunchToken("-force-d3d11", on) },
	}))

	s.section(l, "灵敏度")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "gauge", title: "保存灵敏度快照",
		desc:  "把所有含 sensitivity / ADS / fov 的值导出到 sensitivity-snapshot.json，调参前先存一份。",
		value: func() string { return "写入 " + tune.ConfigPath() + "\\sensitivity-snapshot.json" },
		open: func() {
			if n, err := s.c.SensitivitySnapshot(); err == nil {
				s.c.LogLine(fmt.Sprintf("灵敏度快照：%d 个值", n))
			}
		},
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon: "shield", title: "还原灵敏度快照",
		desc:  "把快照里的灵敏度/FOV 值写回；游戏运行中请先退出。",
		value: func() string { return "需已有快照" },
		open:  func() { _, _ = s.c.RestoreSensitivity() },
	}))
	return root
}
