package appui

import (
	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

// aboutCat is the project identity / boundaries page. It credits the UI
// references and the AI collaborator without turning the page into a link
// board for unrelated projects.
func (s *screens) aboutCat() *ui.Node {
	root := s.cat("about", "关于", "CODM Tune · 原生 Windows 性能与体验工具")
	l := s.listIn(root)

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 112), feature{
		icon: "book", title: "CODM Tune", info: true,
		desc:  "Go + Win32/GDI+ 原生界面，独立进程悬浮窗；不注入游戏、不读写游戏内存、不结束或挂起 ACE。",
		value: func() string { return "v12.2 · FIELD TERMINAL" },
		hint:  "安全边界",
	}))

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 112), feature{
		icon: "shield", title: "设计目标", info: true,
		desc:  "把可逆的系统调优、只读诊断和可验证的体验设置放在一个轻量工具里；所有写操作先备份，再提供恢复路径。",
		value: func() string { return "可逆 · 可验证 · 低占用" },
	}))

	s.section(l, "作者与协作")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 110), feature{
		icon: "book", title: "作者 · Baimacao",
		desc:  "独立开发者。CODM Tune 的界面、系统调优和诊断均在本仓库维护；问题反馈请附诊断 txt。",
		value: func() string { return "GitHub · Baimacao" },
		hint:  "Enter 打开",
		open:  func() { _ = tune.OpenURL("https://github.com/Baimacao") },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 126), feature{
		icon: "info", title: "界面协作 · DeepSeek Harness",
		desc:  "舟味工业终端、焦点/切页动画、纯文字 HUD 与图标生成由本会话的 coding agent 协作完成；审美取舍与最终验收由作者拍板。",
		value: func() string { return "AI 协作 · DeepSeek V4" },
		hint:  "工具与代码同源",
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 126), feature{
		icon: "monitor", title: "界面参考",
		desc:  "动效与排版学习 lilt 的弹簧语义、Astra OLED UI 的深色层次、eDEX-UI 的启动仪式感；只借鉴交互语言，不复制代码。",
		value: func() string { return "开源社区 · 交互参考" },
		hint:  "非官方关联",
	}))

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 104), feature{
		icon: "book", title: "知识库与诊断", info: true,
		desc:  "工具内的 markdown 知识库、诊断导出和日志都放在配置目录；反馈问题时附上诊断 txt 即可。",
		value: func() string { return "只读导出 · 不含凭证" },
		hint:  "设置 → 文件",
	}))

	s.section(l, "版本信息")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon: "monitor", title: "构建", info: true,
		desc:  "UI：舟味工业终端 / 标准 MD3E；测量：DXGI + PDH；悬浮窗：独立 HUD 子进程。",
		value: func() string { return "v12.2 · 2026-10" },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 88), feature{
		icon: "book", title: "免责声明", info: true,
		desc:  "非腾讯/动视官方产品。系统调优有风险，请先阅读每一项说明；修改前工具会尽量备份，但不保证覆盖所有环境。",
		value: func() string { return "使用即表示理解风险" },
	}))

	return root
}
