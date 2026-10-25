package appui

import (
	"fmt"
	"math"

	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

// fpsRecordCat is the manual capture flow: start, play, stop, read the
// waveform and smoothness score. It records one sample per second so the page
// stays independent from PresentMon's file format.
func (s *screens) fpsRecordCat() *ui.Node {
	root := s.cat("record", "帧率记录", "手动开始，结束生成帧数波形与流畅性评价")
	l := s.listIn(root)
	active, secs := s.c.FPSRecordState()
	last := s.c.LastFPSRecord()

	l.Add(s.featureRow(ui.R(PadX, 0, RowW, 100), feature{
		icon: "bolt",
		title: func() string {
			if active {
				return "停止记录并生成报告"
			}
			return "开始记录帧数"
		}(),
		desc: func() string {
			if active {
				return fmt.Sprintf("正在记录 · 已 %s；回到游戏，结束后回来点停止。", durationText(secs))
			}
			return "启动后进入训练场/对局，工具每秒采样一次 FPS；停止后生成波形和评分。"
		}(),
		value: func() string {
			if active {
				return "REC"
			}
			if len(last.Samples) > 0 {
				return fmt.Sprintf("上次 %d 个样本 · %s", len(last.Samples), last.Verdict)
			}
			return "未开始"
		},
		tone: func() tune.Tone {
			if active {
				return tune.ToneWarn
			}
			return tune.ToneGood
		},
		hint: func() string {
			if active {
				return "Enter 停止"
			}
			return "Enter 开始"
		}(),
		open: func() {
			if active {
				s.c.StopFPSRecord()
			} else {
				s.c.StartFPSRecord()
			}
			s.switchTo("record")
		},
	}))

	l.Add(s.fpsGraphNode(last))
	s.section(l, "评价")
	l.Add(s.fpsStatRow("平均帧率", func() string {
		if len(last.Samples) == 0 {
			return "—"
		}
		return fmt.Sprintf("%.1f FPS", last.Average)
	}))
	l.Add(s.fpsStatRow("1% low", func() string {
		if len(last.Samples) == 0 {
			return "—"
		}
		return fmt.Sprintf("%.1f FPS", last.Low)
	}))
	l.Add(s.fpsStatRow("帧数波动", func() string {
		if len(last.Samples) == 0 {
			return "—"
		}
		return fmt.Sprintf("%.1f%% · 低于均值 80%% 的秒占 %.0f%%", last.Jitter, last.Drops)
	}))
	l.Add(s.fpsStatRow("流畅度", func() string {
		if len(last.Samples) == 0 {
			return "—"
		}
		return fmt.Sprintf("%.0f / 100 · %s", last.Score, last.Verdict)
	}))
	if last.Note != "" {
		n := ui.NewNode(ui.R(PadX, 0, RowW, 62))
		n.Focusable = false
		n.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
			th := ui.CurrentTheme
			c.Text(ui.R(r.X+4, r.Y+10, r.W-8, 44), last.Note, 11, th.Muted, ui.AlignLeft, false)
		}
		l.Add(n)
	}
	return root
}

func durationText(seconds float64) string {
	if seconds < 60 {
		return fmt.Sprintf("%.0f 秒", seconds)
	}
	return fmt.Sprintf("%d 分 %02d 秒", int(seconds)/60, int(seconds)%60)
}

func (s *screens) fpsStatRow(label string, value func() string) *ui.Node {
	n := ui.NewNode(ui.R(PadX, 0, RowW, 48))
	n.Focusable = false
	n.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		th := ui.CurrentTheme
		c.Line(r.X+2, r.Y+r.H-1, r.X+r.W-2, r.Y+r.H-1, 1, th.Outline2)
		c.Text(ui.R(r.X+6, r.Y+14, 260, 20), label, 12, th.Muted, ui.AlignLeft, false)
		c.Text(ui.R(r.X+280, r.Y+12, r.W-290, 22), value(), 14, th.Text, ui.AlignRight, true)
	}
	return n
}

func (s *screens) fpsGraphNode(rec tune.FPSRecord) *ui.Node {
	n := ui.NewNode(ui.R(PadX, 0, RowW, 250))
	n.Focusable = false
	n.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
		th := ui.CurrentTheme
		card(c, r, 0, th.Surface)
		c.Text(ui.R(r.X+16, r.Y+12, 300, 18), "FPS 波形", 12, th.Text, ui.AlignLeft, true)
		if len(rec.Samples) == 0 {
			c.Text(ui.R(r.X+16, r.Y+54, r.W-32, 80), "还没有样本。开始记录后进入对局，停止时会在这里画出每秒帧数。", 12, th.Muted, ui.AlignLeft, false)
			return
		}
		plot := ui.R(r.X+48, r.Y+46, r.W-72, r.H-86)
		c.Line(plot.X, plot.Y+plot.H, plot.Right(), plot.Y+plot.H, 1, th.Outline)
		for i := 0; i <= 3; i++ {
			y := plot.Y + float32(i)*plot.H/3
			c.Line(plot.X, y, plot.Right(), y, 1, th.Outline2)
		}
		vals := rec.Samples
		maxV := 30.0
		for _, v := range vals {
			if v > maxV {
				maxV = v
			}
		}
		maxV = math.Ceil(maxV/30) * 30
		if maxV < 60 {
			maxV = 60
		}
		yOf := func(v float64) float32 {
			return plot.Y + plot.H - float32(v/maxV)*plot.H
		}
		// The average is a reading aid, not a fake "target" line.
		avgY := yOf(rec.Average)
		c.Line(plot.X, avgY, plot.Right(), avgY, 1, th.Primary.WithAlpha(150))
		var prevX, prevY float32
		for i, v := range vals {
			x := plot.X + float32(i)/float32(max(1, len(vals)-1))*plot.W
			y := yOf(v)
			if i > 0 {
				c.Line(prevX, prevY, x, y, 2, th.Primary)
			}
			prevX, prevY = x, y
		}
		c.Text(ui.R(r.X+8, r.Y+46, 38, 16), fmt.Sprintf("%.0f", maxV), 9, th.Muted, ui.AlignRight, false)
		c.Text(ui.R(r.X+8, r.Y+plot.H+29, 38, 16), "0", 9, th.Muted, ui.AlignRight, false)
		c.Text(ui.R(plot.Right()-130, r.Y+12, 126, 16), fmt.Sprintf("平均 %.1f", rec.Average), 10, th.Muted, ui.AlignRight, false)
		if len(vals) > 1 {
			c.Text(ui.R(r.X+16, r.Y+r.H-30, r.W-32, 16), fmt.Sprintf("样本 %d · %s · 低点 %.1f", len(vals), durationText(rec.Duration), rec.Low), 9, th.Muted, ui.AlignLeft, false)
		}
	}
	return n
}
