package appui

import (
	"os"
	"time"

	"codmtune/internal/hud"
	"codmtune/internal/win32"
)

// overlayDriver hands the HUD state to the dedicated layered-window process.
// The main process never creates a layered window.
func (s *screens) overlayDriver() {
	tick := time.NewTicker(500 * time.Millisecond)
	for range tick.C {
		cfg := s.c.Cfg()
		d := s.c.OverlayData()
		// Frame budget follows the workload this app actually has. Resting is
		// 20 Hz; with CODM running it drops to 10 Hz, because a repaint that
		// steals a timeslice from the renderer is worse than a slightly
		// later label. Both are user-visible values on the 设置 page.
		rest := cfg.IdleFPS
		if rest < 10 || rest > 60 {
			rest = 20
		}
		if d.Game {
			rest = cfg.GameIdleFPS
			if rest < 5 || rest > 30 {
				rest = 10
			}
		}
		win32.SetIdleInterval(1000 / rest)
		// Animation budget. Idle already sleeps at the interval above; this
		// only caps a frame in flight. 30 fps is enough for the spring
		// transitions, and while CODM is running the app drops to 15 fps so
		// that even a page switch cannot take a full core from the renderer.
		frame := 33
		if d.Game {
			frame = 66
		}
		win32.SetFrameInterval(frame)
		show := cfg.OverlayEnabled && (cfg.OverlayIdle || d.Game) && s.uiReady.Load()
		if !show {
			hud.EnsureChild(false)
			continue
		}
		_ = hud.Write(hud.State{
			Show:      show,
			Game:      d.Game,
			Ready:     d.Ready,
			FPS:       d.FPS,
			Low:       d.LowFPS,
			Stutter:   d.Stutter,
			RTT:       d.RTT,
			Jitter:    d.Jitter,
			Loss:      d.Loss,
			LossKnown: d.LossKnown,
			ServerIP:  d.ServerIP,
			Province:  d.Province,
			Style:     cfg.OverlayStyle,
			Opacity:   cfg.OverlayOpacity,
			Corner:    cfg.OverlayCorner,
			HudX:      cfg.OverlayX,
			HudY:      cfg.OverlayY,
			HudScale:  cfg.OverlayScale,
			ShowFPS:   cfg.OverlayFPS,
			ShowLow:   cfg.OverlayLow,
			ShowStut:  cfg.OverlayStutter,
			ShowLoss:  cfg.OverlayLoss,
			ShowText:  cfg.OverlayText,
			ShowKeys:  cfg.OverlayKeys,
			ShowNet:   cfg.OverlayNet,
			ShowProv:  cfg.OverlayServer,
			Theme:     cfg.Theme,
			ParentPID: os.Getpid(),
		})
		hud.EnsureChild(true)
	}
}
