package tune

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// FPSRecord is one manual recording session. Samples are one per second while
// the game is running, so the waveform is deliberately a low-rate view rather
// than a frame-by-frame trace (the HUD already exposes the instantaneous FPS).
type FPSRecord struct {
	Samples  []float64
	Duration float64
	Average  float64
	Low      float64
	Jitter   float64
	Drops    float64
	Score    float64
	Verdict  string
	Note     string
	Started  time.Time
	Ended    time.Time
}

type fpsRecorder struct {
	mu      sync.Mutex
	active  bool
	started time.Time
	ended   time.Time
	samples []float64
	last    time.Time
}

var recorder fpsRecorder

// StartFPSRecord begins a manual session. Existing samples are discarded.
func (c *Control) StartFPSRecord() {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.active = true
	recorder.started = time.Now()
	recorder.ended = time.Time{}
	recorder.last = time.Time{}
	recorder.samples = nil
}

// StopFPSRecord ends the session and computes a smoothness report.
func (c *Control) StopFPSRecord() FPSRecord {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.active = false
	recorder.ended = time.Now()
	return summarizeRecord(recorder.started, recorder.ended, recorder.samples)
}

// FPSRecordState returns whether a recording is active and its wall duration.
func (c *Control) FPSRecordState() (bool, float64) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if !recorder.active || recorder.started.IsZero() {
		return false, 0
	}
	return true, time.Since(recorder.started).Seconds()
}

// LastFPSRecord is the most recent completed report (zero before the first).
func (c *Control) LastFPSRecord() FPSRecord {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	end := recorder.ended
	if end.IsZero() {
		end = time.Now()
	}
	return summarizeRecord(recorder.started, end, recorder.samples)
}

func (c *Control) recordSample(stats OverlayStats) {
	if !stats.Game || !stats.Ready || stats.FPS <= 0 || math.IsNaN(stats.FPS) || math.IsInf(stats.FPS, 0) {
		return
	}
	now := time.Now()
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if !recorder.active {
		return
	}
	// The overlay loop is 1 Hz; guard against duplicate ticks from a rebuilt
	// controller without dropping genuine seconds.
	if !recorder.last.IsZero() && now.Sub(recorder.last) < 700*time.Millisecond {
		return
	}
	recorder.last = now
	recorder.samples = append(recorder.samples, stats.FPS)
	if len(recorder.samples) > 600 { // ten minutes at 1 Hz
		recorder.samples = append([]float64{}, recorder.samples[len(recorder.samples)-600:]...)
	}
}

// summarizeRecord intentionally does not invent frame data. The score is
// derived from the sampled FPS series: average, 1% low, spread and time below
// 80% of average. A short session gets a clear "sample too short" verdict.
func summarizeRecord(start, end time.Time, in []float64) FPSRecord {
	r := FPSRecord{Started: start, Ended: end}
	r.Samples = append([]float64{}, in...)
	if !start.IsZero() {
		r.Duration = end.Sub(start).Seconds()
	}
	n := len(in)
	if n < 3 {
		r.Verdict = "样本不足"
		r.Note = "至少需要 3 个有效秒样本；请在进入对局后重新开始。"
		return r
	}
	var sum float64
	for _, v := range in {
		sum += v
	}
	r.Average = sum / float64(n)
	sorted := append([]float64{}, in...)
	sort.Float64s(sorted)
	cut := n / 100
	if cut < 1 {
		cut = 1
	}
	var lowSum float64
	for _, v := range sorted[:cut] {
		lowSum += v
	}
	r.Low = lowSum / float64(cut)
	var varSum float64
	for _, v := range in {
		d := v - r.Average
		varSum += d * d
	}
	sd := math.Sqrt(varSum / float64(n))
	r.Jitter = sd / r.Average * 100
	drops := 0
	for _, v := range in {
		if v < r.Average*0.8 {
			drops++
		}
	}
	r.Drops = float64(drops) / float64(n) * 100
	lowRatio := r.Low / r.Average
	if lowRatio > 1 {
		lowRatio = 1
	}
	r.Score = 100 - (1-lowRatio)*65 - r.Jitter*1.8 - r.Drops*0.8
	if r.Score < 0 {
		r.Score = 0
	}
	if r.Score > 100 {
		r.Score = 100
	}
	switch {
	case r.Score >= 85:
		r.Verdict = "极流畅"
	case r.Score >= 70:
		r.Verdict = "流畅"
	case r.Score >= 55:
		r.Verdict = "一般"
	default:
		r.Verdict = "卡顿明显"
	}
	r.Note = fmt.Sprintf("基于 %d 个 1 秒样本；1%% low 为最低 1%% 秒均值，评分越低说明波动/掉帧越明显。", n)
	return r
}
