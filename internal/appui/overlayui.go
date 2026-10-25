package appui

import (
	"os"
	"time"

	"codmtune/internal/hud"
)

// overlayDriver hands the HUD state to the dedicated layered-window process.
// The main process never creates a layered window.
func (s *screens) overlayDriver() {
	tick := time.NewTicker(500 * time.Millisecond)
	for range tick.C {
		cfg := s.c.Cfg()
		d := s.c.OverlayData()
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
