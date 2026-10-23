// Package hud owns the floating HUD process and the state handoff from the
// main app. Running the layered window in a separate process is deliberate:
// in the main process a second WS_EX_LAYERED window breaks the GDI blits that
// paint the main window.
package hud

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"codmtune/internal/sys"
	"codmtune/internal/ui"
	"codmtune/internal/win32"
)

// State is written by the main process every 500 ms and read by the HUD.
type State struct {
	Show      bool    `json:"show"`
	Game      bool    `json:"game"`
	Ready     bool    `json:"ready"`
	FPS       float64 `json:"fps"`
	Low       float64 `json:"low"`
	Stutter   float64 `json:"stutter"`
	RTT       uint32  `json:"rtt"`
	Jitter    uint32  `json:"jitter"`
	Loss      float64 `json:"loss"`
	LossKnown bool    `json:"loss_known"`
	ServerIP  string  `json:"server_ip"`
	Province  string  `json:"province"`
	Style     string  `json:"style"`
	Opacity   int     `json:"opacity"`
	Corner    string  `json:"corner"`
	HudX      int     `json:"hud_x"`
	HudY      int     `json:"hud_y"`
	ShowFPS   bool    `json:"show_fps"`
	ShowLow   bool    `json:"show_low"`
	ShowStut  bool    `json:"show_stutter"`
	ShowLoss  bool    `json:"show_loss"`
	ShowText  bool    `json:"show_text"`
	ShowKeys  bool    `json:"show_keys"`
	ShowNet   bool    `json:"show_net"`
	ShowProv  bool    `json:"show_province"`
	Theme     string  `json:"theme"`
	Accent    string  `json:"accent"`
	SavedUnix int64   `json:"saved_unix"`
	ParentPID int     `json:"parent_pid"`
}

var (
	pathOnce sync.Once
	stateMu  sync.RWMutex
	current  State
	childMu  sync.Mutex
	child    *exec.Cmd
)

func statePath() string {
	var base string
	if alt := os.Getenv("CODMTUNE_HOME"); alt != "" {
		base = alt
	} else if dir, err := os.UserConfigDir(); err == nil {
		base = filepath.Join(dir, "CODMTune")
	} else {
		base = os.TempDir()
	}
	_ = os.MkdirAll(base, 0o755)
	return filepath.Join(base, "overlay-state.json")
}

// Write persists the state atomically for the HUD process.
func Write(st State) error {
	pathOnce.Do(func() {})
	st.SavedUnix = time.Now().Unix()
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	path := statePath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// EnsureChild starts or stops the HUD child process.
func EnsureChild(on bool) {
	childMu.Lock()
	defer childMu.Unlock()
	if !on {
		stopLocked()
		return
	}
	if child != nil && child.Process != nil {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "--overlay")
	cmd.Env = append(os.Environ(), "CODMTUNE_HUD=1")
	cmd.SysProcAttr = sys.HiddenAttr()
	if err := cmd.Start(); err != nil {
		return
	}
	child = cmd
	go func(c *exec.Cmd) {
		_ = c.Wait()
		childMu.Lock()
		if child == c {
			child = nil
		}
		childMu.Unlock()
	}(cmd)
}

func stopLocked() {
	if child != nil && child.Process != nil {
		_ = child.Process.Kill()
	}
	child = nil
}

// StopChild is deferred by the main app.
func StopChild() {
	childMu.Lock()
	defer childMu.Unlock()
	stopLocked()
}

// Run is the HUD process entry point. It blocks in the Win32 message loop.
func Run() error {
	if !win32.AcquireSingleInstance("CODMTuneHUD") {
		return nil
	}
	load()
	go func() {
		t := time.NewTicker(250 * time.Millisecond)
		for range t.C {
			load()
		}
	}()
	return win32.RunHUD(320, 216, draw)
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
	// Exit when the main app stopped writing. This also reaps orphans.
	if st.SavedUnix > 0 && time.Since(time.Unix(st.SavedUnix, 0)) > 12*time.Second {
		os.Exit(0)
	}
	// Older builds wrote no parent_pid. A stale child from one of those builds
	// can otherwise keep showing yesterday's corner/theme forever because the
	// single-instance mutex is already held.
	if st.ParentPID == 0 && st.SavedUnix > 0 && time.Since(time.Unix(st.SavedUnix, 0)) > 3*time.Second {
		os.Exit(0)
	}
	// A HUD child belongs to exactly one main process. If a stale child from a
	// previous run is still alive, the new main process rewrites parent_pid;
	// the stale child sees a mismatch and exits instead of drawing an old
	// corner/opacity/style forever.
	if st.ParentPID > 0 {
		if parent := os.Getppid(); parent > 0 && parent != st.ParentPID {
			os.Exit(0)
		}
	}
	stateMu.Lock()
	current = st
	stateMu.Unlock()
	win32.HUDAlpha(uint8(clamp(st.Opacity, 178, 255)))
	if st.Corner == "custom" {
		win32.HUDPositionCustom(st.HudX, st.HudY)
	} else {
		win32.HUDPosition(st.Corner)
	}
	win32.HUDShow(st.Show)
}

func snapshot() State {
	stateMu.RLock()
	defer stateMu.RUnlock()
	return current
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

var _ = fmt.Sprintf
var _ = ui.RGB
