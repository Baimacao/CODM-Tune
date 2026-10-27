package appui

import (
	"fmt"

	"codmtune/internal/ui"
)

type choiceOption struct {
	Value string
	Label string
	Desc  string
}

// choicePage is the generic secondary menu: one radio row per option. It is
// used for every multi-choice block instead of "press Enter to cycle".
func (s *screens) choicePage(parent, title, sub, current string, opts []choiceOption, pick func(string)) *ui.Node {
	root := s.cat(parent, title, sub)
	l := s.listIn(root)
	for i, opt := range opts {
		opt := opt
		idx := i
		n := ui.NewNode(ui.R(PadX, 0, RowW, 66))
		n.Focusable = true
		n.Data = "choice:" + opt.Value
		n.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
			th := ui.CurrentTheme
			selected := opt.Value == current
			if th.Style == "endfield" {
				// A choice is a numbered record row on hairlines; the current
				// value is marked with the accent block on its left edge and
				// bold ink. No radio discs, no check glyph, no rounded plate.
				c.Line(r.X, r.Bottom(), r.Right(), r.Bottom(), 1, th.Outline)
				if selected {
					c.FillRect(ui.R(r.X, r.Y+8, 4, r.H-16), th.Mark)
				}
				c.Text(ui.R(r.X+16, r.Y+12, 32, 20), fmt.Sprintf("%02d", idx+1), 10, th.Muted, ui.AlignLeft, true)
				c.Text(ui.R(r.X+52, r.Y+12, r.W-100, 22), opt.Label, 15, th.Text, ui.AlignLeft, selected)
				c.TextBlock(ui.R(r.X+52, r.Y+36, r.W-110, 24), opt.Desc, 11, th.Muted, ui.AlignLeft, false)
				if selected {
					c.Text(ui.R(r.X+r.W-64, r.Y+12, 52, 20), "CURRENT", 9, th.Muted, ui.AlignRight, true)
				}
				return
			}
			c.FillRoundRect(r, th.CardRadius, th.Surface)
			cc := th.Muted
			if selected {
				cc = th.Primary
			}
			c.StrokeCircle(r.X+24, r.Y+26, 9, 1.5, cc)
			if selected {
				c.Circle(r.X+24, r.Y+26, 5, th.Primary)
			}
			c.Text(ui.R(r.X+46, r.Y+12, r.W-100, 22), opt.Label, 15, th.Text, ui.AlignLeft, selected)
			c.TextBlock(ui.R(r.X+46, r.Y+36, r.W-110, 24), opt.Desc, 11, th.Muted, ui.AlignLeft, false)
			if selected {
				c.Text(ui.R(r.X+r.W-44, r.Y+12, 26, 22), "✓", 16, th.Primary, ui.AlignRight, true)
			}
		}
		n.Activate = func(*ui.Node) {
			if pick != nil {
				pick(opt.Value)
			}
			s.switchTo(parent)
		}
		n.SetCursorRadius(ui.CurrentTheme.CardRadius - 2)
		l.Add(n)
	}
	return root
}

func (s *screens) choose(parent, title, sub, current string, opts []choiceOption, pick func(string)) {
	page := s.choicePage(parent, title, sub, current, opts, pick)
	s.app.Push(page)
	// Match the cursor ring to the current value instead of the first row;
	// otherwise the ring sits on "standard" while the check marks another row.
	for _, n := range ui.Focusables(page) {
		if n.Data == "choice:"+current {
			s.app.SetFocus(n)
			break
		}
	}
}
