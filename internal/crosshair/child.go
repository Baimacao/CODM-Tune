package crosshair

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"codmtune/internal/sys"
	"codmtune/internal/ui"
	"codmtune/internal/win32"
)

// State is the hand-off file between the console and the layered helper.
type State struct {
	Show     bool     `json:"show"`
	Settings Settings `json:"settings"`
	Saved    int64    `json:"saved_unix"`
}

var (
	mu    sync.RWMutex
	state = State{Settings: Default()}
	child *exec.Cmd
)

func statePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "CODMTune")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "crosshair-state.json")
}

// Write updates the state file. It is the only way the main UI talks to the
// crosshair helper, keeping GDI/GDI+ in one process per window.
func Write(s Settings, show bool) error {
	st := State{Show: show, Settings: s, Saved: time.Now().Unix()}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(), b, 0o644)
}

// EnsureChild starts or stops the crosshair helper.
func EnsureChild(on bool) {
	mu.Lock()
	defer mu.Unlock()
	if !on {
		if child != nil && child.Process != nil {
			_ = child.Process.Kill()
		}
		child = nil
		return
	}
	if child != nil {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "--crosshair")
	cmd.Env = append(os.Environ(), "CODMTUNE_CROSS=1")
	cmd.SysProcAttr = sys.HiddenAttr()
	if cmd.Start() != nil {
		return
	}
	child = cmd
	go func(c *exec.Cmd) {
		_ = c.Wait()
		mu.Lock()
		if child == c {
			child = nil
		}
		mu.Unlock()
	}(cmd)
}

// StopChild kills the helper at app shutdown.
func StopChild() {
	EnsureChild(false)
}

// RunChild is the `--crosshair` entry point.
func RunChild() error {
	if !win32.AcquireSingleInstance("CODMTuneCross") {
		return nil
	}
	load()
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		for range t.C {
			load()
		}
	}()
	win32.CrossAlpha(uint8(state.Settings.Alpha))
	return win32.RunCrosshair(420, 420, draw)
}

func load() {
	b, err := os.ReadFile(statePath())
	if err != nil {
		return
	}
	var st State
	if json.Unmarshal(b, &st) != nil {
		return
	}
	// The helper is disposable: if the main app stopped writing, go away.
	if st.Saved > 0 && time.Since(time.Unix(st.Saved, 0)) > 8*time.Second {
		os.Exit(0)
	}
	mu.Lock()
	state = st
	mu.Unlock()
	win32.CrossAlpha(uint8(st.Settings.Alpha))
	win32.CrossShow(st.Show)
}

func draw(c *win32.Canvas) {
	mu.RLock()
	st := state
	mu.RUnlock()
	if !st.Show {
		return
	}
	s := st.Settings
	col, _ := parseColor(s.Color)
	col = ui.RGBA(col.R(), col.G(), col.B(), uint8(s.Alpha))
	w, h := float32(420), float32(420)
	cx, cy := w/2+float32(s.OffsetX), h/2+float32(s.OffsetY)
	th := float32(s.Thickness)
	ln := float32(s.Length)
	gap := float32(s.Gap)
	black := ui.RGBA(0, 0, 0, 255)
	draw := func(r ui.Rect) {
		if s.Outline {
			c.FillRect(ui.R(r.X-1, r.Y-1, r.W+2, r.H+2), black)
		}
		c.FillRect(r, col)
	}
	switch s.Style {
	case "dot":
		sz := th*2 + 1
		draw(ui.R(cx-sz/2, cy-sz/2, sz, sz))
	case "circle":
		if s.Outline {
			c.StrokeCircle(cx, cy, ln+gap+th, th+2, black)
		}
		c.StrokeCircle(cx, cy, ln+gap+th, th, col)
		if s.Dot {
			draw(ui.R(cx-th/2, cy-th/2, th, th))
		}
	case "t":
		draw(ui.R(cx-th/2, cy-gap-ln, th, ln))
		draw(ui.R(cx-gap-ln, cy-th/2, ln, th))
		draw(ui.R(cx+gap, cy-th/2, ln, th))
		if s.Dot {
			draw(ui.R(cx-th/2, cy-th/2, th, th))
		}
	default:
		draw(ui.R(cx-th/2, cy-gap-ln, th, ln))
		draw(ui.R(cx-th/2, cy+gap, th, ln))
		draw(ui.R(cx-gap-ln, cy-th/2, ln, th))
		draw(ui.R(cx+gap, cy-th/2, ln, th))
		if s.Dot {
			draw(ui.R(cx-th/2, cy-th/2, th, th))
		}
	}
}

func parseColor(s string) (ui.Color, int) {
	if len(s) != 7 || s[0] != '#' {
		return ui.RGBA(57, 255, 20, 255), 255
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return ui.RGBA(57, 255, 20, 255), 255
	}
	return ui.RGBA(uint8(v>>16), uint8(v>>8), uint8(v), 255), 255
}
