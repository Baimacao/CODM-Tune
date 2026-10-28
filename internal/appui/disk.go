package appui

import (
	"fmt"
	"time"

	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

// gbText renders a byte count as GB with one decimal.
func gbText(v uint64) string { return fmt.Sprintf("%.1f GB", float64(v)/(1<<30)) }

// diskCat is the storage page. It reports what the volumes actually are and
// then offers the three things that help a game install: defrag/TRIM through
// the built-in tool, and a higher I/O priority for the game process. Nothing
// here writes an invented registry value.
func (s *screens) diskCat() *ui.Node {
	root := s.cat("disk", "硬盘与文件系统", "卷容量与实时 I/O、碎片整理 / TRIM、游戏 I/O 优先级")
	l := s.listIn(root)
	st := s.c.Disk()

	// ---- volumes ---------------------------------------------------------
	s.section(l, "卷 · 当前目标 "+st.Target)
	if len(st.Volumes) == 0 {
		l.Add(s.noteRow("没有读到可用卷。"))
	}
	for _, v := range st.Volumes {
		v := v
		title := v.Letter
		if v.Label != "" {
			title += "  " + v.Label
		}
		l.Add(s.featureRow(ui.R(PadX, 0, RowW, 88), feature{
			icon:  "disc",
			title: title,
			descFn: func() string {
				if v.Media == "" {
					return fmt.Sprintf("%s · 介质未知 · 可用 %s / 共 %s", v.FS, gbText(v.Free), gbText(v.Total))
				}
				return fmt.Sprintf("%s · %s · 可用 %s / 共 %s", v.FS, v.Media, gbText(v.Free), gbText(v.Total))
			},
			value: func() string {
				if v.Total == 0 {
					return "容量未知"
				}
				return fmt.Sprintf("已用 %.0f%%", float64(v.Total-v.Free)/float64(v.Total)*100)
			},
			tone: func() tune.Tone {
				// st.Target is the resolved one: when the remembered volume is
				// gone the page falls back to the game drive, and the highlight
				// has to follow that fallback rather than the raw config value.
				if v.Letter == s.c.Disk().Target {
					return tune.ToneGood
				}
				return tune.ToneNeutral
			},
			hintFn: func() string {
				if v.Letter == s.c.Disk().Target {
					return "当前目标"
				}
				return "Enter 设为目标"
			},
			open: func() {
				s.c.SelectDiskTarget(v.Letter)
				// The section label and the per-row hints are baked into the
				// list when it is built, so the page is rebuilt rather than
				// patched in place.
				s.switchTo("disk")
			},
		}))
	}

	// ---- live I/O --------------------------------------------------------
	s.section(l, "实时 I/O · 每 2 秒采样")
	if len(st.Disks) == 0 {
		// The first PDH sample lands about 250 ms after the page opens, but the
		// row list is already built by then. Without this the per-disk rows
		// would stay missing until the user left the page and came back.
		s.diskCatchUp()
	}
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 84), feature{
		icon:  "chip",
		title: "合计吞吐",
		descFn: func() string {
			d := s.c.Disk()
			if d.IOErr != "" {
				return "采样失败：" + d.IOErr
			}
			if len(d.Disks) == 0 {
				return "PDH 需要两个采集窗口，约 250 ms 后出第一个数。"
			}
			for _, x := range d.Disks {
				if x.Index == "_Total" {
					return fmt.Sprintf("读 %.1f MB/s · 写 %.1f MB/s · %.0f IOPS · 平均延迟 %.2f ms",
						x.ReadMBs, x.WriteMBs, x.IOPS, x.Latency)
				}
			}
			// _Total is always in the sample once the first window lands, so
			// reaching here means the instance list is malformed, not that the
			// machine has one disk.
			return "采样里没有合计实例。"
		},
		value: func() string {
			for _, x := range s.c.Disk().Disks {
				if x.Index == "_Total" {
					return fmt.Sprintf("队列 %.1f", x.Queue)
				}
			}
			return "—"
		},
		tone: func() tune.Tone {
			for _, x := range s.c.Disk().Disks {
				if x.Index == "_Total" && x.Queue >= 2 {
					return tune.ToneWarn
				}
			}
			return tune.ToneNeutral
		},
		info: true,
	}))
	// The per-disk rows are appended into this list as samples arrive, so the
	// page records where they start and which instances it already shows.
	s.diskList = l
	s.diskNext = len(l.Rows())
	s.diskShown = map[string]bool{}
	for _, d := range st.Disks {
		if d.Index == "_Total" {
			continue
		}
		s.addDiskIORow(d.Index)
	}

	// ---- maintenance -----------------------------------------------------
	s.section(l, "维护 · 调用系统自带 defrag")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon:  "gauge",
		title: "分析碎片",
		descFn: func() string {
			if b := s.c.Disk().Busy; b != "" {
				return "正在执行 " + b + "；碎片整理按容量可能要几分钟，关闭本页不会中断，结果会写进日志。"
			}
			return "只读：报告目标卷的碎片比例与是否需要整理，不改动任何文件。"
		},
		value: func() string { return s.lastDiskLine("analyze") },
		tone: func() tune.Tone {
			if s.c.Disk().Busy != "" {
				return tune.ToneWarn
			}
			return tune.ToneNeutral
		},
		hintFn: func() string {
			if s.c.Disk().Busy != "" {
				return ""
			}
			return "Enter 分析"
		},
		open: func() { s.c.RunVolumeAction("analyze") },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 88), feature{
		icon:  "disc",
		title: "优化目标卷",
		desc:  "HDD 做碎片整理，SSD 由系统自动转为 TRIM 剪裁；按住确认，耗时较长。",
		value: func() string { return s.lastDiskLine("optimize") },
		hold:  1.2,
		open:  func() { s.c.RunVolumeAction("optimize") },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 88), feature{
		icon:  "bolt",
		title: "重新剪裁（TRIM）",
		desc:  "对 SSD 重发 TRIM，让主控重新认识已删除块；HDD 上无意义。",
		value: func() string {
			if d := s.c.Disk(); d.TrimRead && !d.TrimOn {
				return "该卷未启用 TRIM"
			}
			return s.lastDiskLine("retrim")
		},
		hold: 1.2,
		open: func() { s.c.RunVolumeAction("retrim") },
	}))
	if st.HasLast && st.Last.Detail != "" {
		l.Add(s.noteRow(st.Last.Detail))
	}

	// ---- game I/O priority ----------------------------------------------
	s.section(l, "游戏 I/O 优先级")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 96), feature{
		icon:  "rocket",
		title: "提高 CODM 的 I/O 优先级",
		desc:  "游戏运行时把它的磁盘读写排到后台任务前面；退出即恢复普通。提高不需要管理员权限。",
		value: func() string { return s.c.Disk().IONote },
		tone: func() tune.Tone {
			if s.c.Cfg().DiskIOBoost {
				return tune.ToneGood
			}
			return tune.ToneNeutral
		},
		toggle:   func() bool { return s.c.Cfg().DiskIOBoost },
		onToggle: func(on bool) { s.c.SetDiskIOBoost(on) },
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 92), feature{
		icon:  "gauge",
		title: "优先级档位",
		desc:  "高档足够；关键档被系统拒绝对普通进程生效，因此不提供。",
		value: func() string { return tune.DiskIOName(s.c.Cfg().DiskIOLevel) },
		adjust: func(d int) {
			levels := tune.DiskIOLevels
			idx := 0
			for i, v := range levels {
				if v == s.c.Cfg().DiskIOLevel {
					idx = i
				}
			}
			idx += d
			if idx < 0 {
				idx = 0
			}
			if idx >= len(levels) {
				idx = len(levels) - 1
			}
			s.c.SetDiskIOLevel(levels[idx])
		},
		progress: func() float64 {
			levels := tune.DiskIOLevels
			lo, hi := levels[0], levels[len(levels)-1]
			if hi <= lo {
				return 0
			}
			return float64(s.c.Cfg().DiskIOLevel-lo) / float64(hi-lo)
		},
	}))

	// ---- read-only facts -------------------------------------------------
	s.section(l, "系统事实（只读）")
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 84), feature{
		icon:  "shield",
		title: "SysMain（Superfetch）",
		desc:  "机械盘上它明显有用，SSD 上收益很小但不该关：关掉只会让冷启动变慢。",
		value: func() string { return s.c.Disk().SysMain },
		info:  true,
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 84), feature{
		icon:  "book",
		title: "预读与超级预取",
		desc:  "只显示当前值。默认值在 SSD 上已经合适，改错只换来更长的启动时间。",
		value: func() string { return s.c.Disk().Prefetch },
		info:  true,
	}))
	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 84), feature{
		icon:  "monitor",
		title: "TRIM / 剪裁状态",
		desc:  "由系统查询 DisableDeleteNotify 得到；关闭它会让 SSD 越用越慢。",
		value: func() string {
			d := s.c.Disk()
			if !d.TrimRead {
				return "读取失败"
			}
			if d.TrimOn {
				return "已启用"
			}
			return "已禁用"
		},
		tone: func() tune.Tone {
			if d := s.c.Disk(); d.TrimRead && !d.TrimOn {
				return tune.ToneWarn
			}
			return tune.ToneNeutral
		},
		info: true,
	}))
	return root
}

// addDiskIORow appends the row for one physical disk instance. It is called
// when the page is built and again from diskCatchUp as samples arrive.
func (s *screens) addDiskIORow(index string) {
	if s.diskList == nil || s.diskShown[index] {
		return
	}
	s.diskShown[index] = true
	idx := index
	row := s.featureRow(ui.R(PadX, 0, RowW, 78), feature{
		icon:  "chip",
		title: "磁盘 " + idx,
		descFn: func() string {
			for _, x := range s.c.Disk().Disks {
				if x.Index == idx {
					return fmt.Sprintf("读 %.1f MB/s · 写 %.1f MB/s · %.0f IOPS · 延迟 %.2f ms",
						x.ReadMBs, x.WriteMBs, x.IOPS, x.Latency)
				}
			}
			return "该磁盘已不在采样里。"
		},
		value: func() string {
			for _, x := range s.c.Disk().Disks {
				if x.Index == idx {
					return fmt.Sprintf("队列 %.1f", x.Queue)
				}
			}
			return "—"
		},
		tone: func() tune.Tone {
			// A deep queue is the one reading that actually predicts stutter,
			// so it is the only one coloured.
			for _, x := range s.c.Disk().Disks {
				if x.Index == idx && x.Queue >= 2 {
					return tune.ToneWarn
				}
			}
			return tune.ToneNeutral
		},
		info: true,
	})
	s.diskList.Insert(row, s.diskNext)
	s.diskNext++
}

// diskCatchUp fills in the per-disk rows once the first PDH sample lands. It
// patches the live list instead of rebuilding the page: a rebuild here would
// run a screen wipe about a second after the user arrived, which reads as a
// flicker and moves the focus ring.
func (s *screens) diskCatchUp() {
	if s.diskWaiting {
		return
	}
	s.diskWaiting = true
	go func() {
		for i := 0; i < 40; i++ {
			time.Sleep(60 * time.Millisecond)
			if len(s.c.Disk().Disks) > 0 {
				break
			}
		}
		s.app.Post(func() {
			s.diskWaiting = false
			if s.active != "disk" {
				return
			}
			for _, d := range s.c.Disk().Disks {
				if d.Index != "_Total" {
					s.addDiskIORow(d.Index)
				}
			}
		})
	}()
}

// lastDiskLine is the subtitle for one maintenance row: when that action last
// ran on the target volume, plus the fragment figure when the report had one.
func (s *screens) lastDiskLine(action string) string {
	cfg := s.c.Cfg()
	var at int64
	switch action {
	case "optimize":
		at = cfg.DiskLastOpt
	case "retrim":
		at = cfg.DiskLastTrim
	}
	st := s.c.Disk()
	if st.HasLast && st.Last.Action == action {
		line := "上次：" + st.Last.Volume
		if st.Last.FragPct >= 0 {
			line += fmt.Sprintf(" · 碎片 %d%%", st.Last.FragPct)
		}
		if st.Last.Err != "" {
			return line + " · 失败"
		}
		return line + " · 完成"
	}
	if at > 0 {
		return "上次 " + time.Unix(at, 0).Format("01-02 15:04")
	}
	if action == "analyze" {
		return "尚未分析"
	}
	return "从未执行"
}

// noteRow is a plain one-line remark inside the list.
func (s *screens) noteRow(text string) *ui.Node {
	n := ui.NewNode(ui.R(PadX, 0, RowW, 46))
	n.Draw = func(nn *ui.Node, c ui.Painter, r ui.Rect) {
		c.TextBlock(ui.R(r.X+2, r.Y+8, r.W-4, r.H-8), text, 11, ui.CurrentTheme.Muted, ui.AlignLeft, false)
	}
	return n
}
