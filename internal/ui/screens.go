package ui

import "codmtune/internal/spring"

// Screen is one entry of the navigation stack. The content owns absolute
// rects; only the host slide is animated here.
type Screen struct {
	Content *Node
	OX      spring.Value
	Restore *Node
	exiting bool
}

// Screens is the stack. The new page settles from a short 12 px offset; the
// previous page stays put so no background seam or double slide is visible.
type Screens struct {
	Hosts []*Screen
	W     float32
}

func (s *Screens) Top() *Screen {
	for i := len(s.Hosts) - 1; i >= 0; i-- {
		if !s.Hosts[i].exiting {
			return s.Hosts[i]
		}
	}
	return nil
}

func (s *Screens) Push(content *Node) {
	h := &Screen{Content: content}
	h.OX.SetSpring(spring.Smooth)
	if top := s.Top(); top != nil {
		top.Restore = nil
		top.OX.Set(0)
		h.OX.Jump(12)
	}
	s.Hosts = append(s.Hosts, h)
	h.OX.Set(0)
	Invalidate()
}

func (s *Screens) Pop() bool {
	top := s.Top()
	if top == nil || len(s.Hosts) == 1 {
		return false
	}
	prev := s.below(top)
	if prev == nil {
		return false
	}
	top.exiting = true
	top.OX.Set(12)
	prev.OX.Set(0)
	Invalidate()
	return true
}

func (s *Screens) below(h *Screen) *Screen {
	found := false
	for i := len(s.Hosts) - 1; i >= 0; i-- {
		switch {
		case s.Hosts[i] == h:
			found = true
		case found && !s.Hosts[i].exiting:
			return s.Hosts[i]
		}
	}
	return nil
}

func (s *Screens) update() {
	for i := 0; i < len(s.Hosts); {
		h := s.Hosts[i]
		if h.exiting && h.OX.Settled() {
			s.Hosts = append(s.Hosts[:i], s.Hosts[i+1:]...)
			continue
		}
		i++
	}
}

// dropExiting removes transition leftovers immediately after a category
// switch, so the next frame owns one host and no invisible old screen keeps
// receiving input or update work.
func (s *Screens) dropExiting() {
	out := s.Hosts[:0]
	for _, h := range s.Hosts {
		if !h.exiting {
			out = append(out, h)
		}
	}
	s.Hosts = out
}

// offset is the host slide for the top screen.
func (s *Screens) offset() float32 {
	if t := s.Top(); t != nil {
		return t.OX.Get()
	}
	return 0
}
