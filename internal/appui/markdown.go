package appui

import (
	"strings"
	"unicode"

	"codmtune/internal/tune"
	"codmtune/internal/ui"
)

type mdBlock struct {
	kind string // h1, h2, bullet, code, text
	text string
}

// markdown renders a small, safe subset of Markdown: headings, bullets,
// fenced code and paragraphs. It never interprets HTML or links as commands.
func (s *screens) markdown(path, name string) *ui.Node {
	raw, err := tune.ReadKnowledge(path)
	if err != nil {
		raw = "读取失败：" + err.Error()
	}
	blocks := parseMarkdown(raw)
	root := s.cat("knowledge", name, path)
	l := s.listIn(root)
	for _, b := range blocks {
		b := b
		lines := estimateLines(b, Wide-40)
		h := float32(lines)*lineHeight(b) + 10
		if b.kind == "h1" || b.kind == "h2" {
			h += 8
		}
		if b.kind == "bullet" {
			h += 4
		}
		n := ui.NewNode(ui.R(PadX, 0, Wide, h))
		n.Draw = func(n *ui.Node, c ui.Painter, r ui.Rect) {
			th := ui.CurrentTheme
			switch b.kind {
			case "h1":
				c.TextBlock(ui.R(r.X+2, r.Y+8, r.W-4, r.H), b.text, 20, th.Text, ui.AlignLeft, true)
			case "h2":
				c.TextBlock(ui.R(r.X+2, r.Y+6, r.W-4, r.H), b.text, 16, th.Text, ui.AlignLeft, true)
			case "code":
				c.FillRoundRect(ui.R(r.X, r.Y, r.W, r.H-4), 10, th.Surface2)
				c.TextBlock(ui.R(r.X+14, r.Y+8, r.W-28, r.H-12), b.text, 11, th.Muted, ui.AlignLeft, false)
			case "bullet":
				c.Circle(r.X+8, r.Y+13, 2.5, th.Primary)
				c.TextBlock(ui.R(r.X+24, r.Y+2, r.W-24, r.H-4), b.text, 12, th.Text, ui.AlignLeft, false)
			default:
				c.TextBlock(ui.R(r.X+2, r.Y+2, r.W-4, r.H-4), b.text, 12, th.Text, ui.AlignLeft, false)
			}
		}
		l.Add(n)
	}
	return root
}

func parseMarkdown(src string) []mdBlock {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	var blocks []mdBlock
	inCode := false
	var code []string
	var para []string
	flushPara := func() {
		if len(para) == 0 {
			return
		}
		blocks = append(blocks, mdBlock{kind: "text", text: strings.Join(para, " ")})
		para = nil
	}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			if inCode {
				blocks = append(blocks, mdBlock{kind: "code", text: strings.Join(code, "\n")})
				code = nil
				inCode = false
			} else {
				flushPara()
				inCode = true
			}
			continue
		}
		if inCode {
			code = append(code, line)
			continue
		}
		switch {
		case strings.HasPrefix(trim, "## "):
			flushPara()
			blocks = append(blocks, mdBlock{kind: "h2", text: strings.TrimPrefix(trim, "## ")})
		case strings.HasPrefix(trim, "# "):
			flushPara()
			blocks = append(blocks, mdBlock{kind: "h1", text: strings.TrimPrefix(trim, "# ")})
		case strings.HasPrefix(trim, "- ") || strings.HasPrefix(trim, "* "):
			flushPara()
			blocks = append(blocks, mdBlock{kind: "bullet", text: strings.TrimSpace(trim[2:])})
		case trim == "":
			flushPara()
		default:
			para = append(para, trim)
		}
	}
	if inCode && len(code) > 0 {
		blocks = append(blocks, mdBlock{kind: "code", text: strings.Join(code, "\n")})
	}
	flushPara()
	return blocks
}

func lineHeight(b mdBlock) float32 {
	switch b.kind {
	case "h1":
		return 30
	case "h2":
		return 24
	case "code":
		return 17
	default:
		return 19
	}
}

func estimateLines(b mdBlock, width float32) int {
	size := lineHeight(b) * 0.62
	need := float32(0)
	for _, r := range b.text {
		if r == '\n' {
			need = 0
			continue
		}
		if unicode.Is(unicode.Han, r) || r > 0x2E80 {
			need += size
		} else {
			need += size * 0.56
		}
	}
	lines := int(need/width) + 1
	if lines < 1 {
		lines = 1
	}
	if b.kind == "code" {
		lines = strings.Count(b.text, "\n") + 1
	}
	return lines
}
