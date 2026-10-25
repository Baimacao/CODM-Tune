package tune

import (
	"bytes"
	_ "embed"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"codmtune/internal/sys"
)

//go:embed PresentMon.exe
var presentMonBytes []byte

// OverlayStats is the snapshot the floating window renders.
type OverlayStats struct {
	Game      bool
	Ready     bool
	FPS       float64
	LowFPS    float64
	Stutter   float64
	RTT       uint32
	Jitter    uint32
	Loss      float64
	LossKnown bool
	ServerIP  string
	Province  string
	Frames    int
}

type frameRow struct {
	t  float64 // seconds
	ms float64
}

type presentMon struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	csv     string
	frames  []frameRow
	ready   bool
	started time.Time
	blocked bool
}

var pm *presentMon

// OverlayData returns the current HUD values.
func (c *Control) OverlayData() OverlayStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.overlay
}

// overlayLoop keeps PresentMon alive with the game and refreshes RTT/server.
func (c *Control) overlayLoop() {
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()
	var lastRTT []uint32
	for {
		select {
		case <-c.stop:
			stopPresentMon()
			return
		case <-tick.C:
			cfg := c.Cfg()
			c.mu.Lock()
			running := c.gameRunning
			pid := c.gamePID
			name := filepath.Base(c.gameExe)
			c.mu.Unlock()
			stats := OverlayStats{Game: running}
			recActive, _ := c.FPSRecordState()
			if !running {
				if pm != nil {
					pm.blocked = false
					pm.frames = nil
					pm.ready = false
				}
			}
			if running && (cfg.OverlayEnabled || recActive) && name != "" {
				stats = c.collectOverlay(cfg, pid, name)
			}
			if running && pid != 0 {
				if rtt, ip, prov := c.sampleNetwork(cfg, pid, &lastRTT); ip != "" {
					stats.ServerIP = ip
					stats.RTT = rtt
					stats.Province = prov
				}
			}
			stats.Jitter = jitterOf(lastRTT)
			stats.LossKnown = stats.RTT > 0 || stats.Stutter > 0
			stats.Loss = estimateLoss(stats)
			c.recordSample(stats)
			c.mu.Lock()
			if c.overlay != stats {
				c.overlay = stats
				c.changed()
			}
			c.mu.Unlock()
		}
	}
}

func (c *Control) collectOverlay(cfg Config, pid uint32, name string) OverlayStats {
	stats := OverlayStats{Game: true}
	if pm == nil {
		pm = &presentMon{}
	}
	if pm.blocked {
		return stats
	}
	dir := filepath.Join(configDir(), "overlay")
	_ = os.MkdirAll(dir, 0o755)
	exe := filepath.Join(dir, "PresentMon.exe")
	if st, err := os.Stat(exe); err != nil || st.Size() != int64(len(presentMonBytes)) {
		_ = os.WriteFile(exe, presentMonBytes, 0o755)
	}
	if pm == nil || pm.cmd == nil || pm.cmd.Process == nil {
		csv := filepath.Join(dir, "live.csv")
		_ = os.Remove(csv)
		args := []string{
			"--output_stdout",
			"--no_console_stats",
			"--stop_existing_session",
			"--terminate_on_proc_exit",
			// Display/GPU/input tracking floods DxgKrnl on this machine and
			// causes tens of thousands of ETW events to be dropped before any
			// Present event reaches us. Present timing only needs the frame
			// present events themselves.
			"--no_track_display",
			"--no_track_gpu",
			"--no_track_input",
			"--set_circular_buffer_size", "262144",
		}
		if pid != 0 {
			args = append(args, "--process_id", strconv.FormatUint(uint64(pid), 10))
		} else {
			args = append(args, "--process_name", name)
		}
		cmd := exec.Command(exe, args...)
		cmd.SysProcAttr = sys.HiddenAttr()
		out, err := os.Create(csv)
		if err != nil {
			c.logf("PresentMon 输出文件创建失败：%v", err)
			return stats
		}
		cmd.Stdout = out
		if err := cmd.Start(); err != nil {
			_ = out.Close()
			c.logf("PresentMon 启动失败：%v", err)
			return stats
		}
		_ = out.Close() // the child owns its inherited handle now
		pm = &presentMon{cmd: cmd, csv: csv, started: time.Now()}
		c.logf("PresentMon：开始采集 %s", name)
		go func(p *presentMon) {
			_ = p.cmd.Wait()
			p.cmd = nil
		}(pm)
	}
	if pm == nil {
		return stats
	}
	// PresentMon writes an append-only CSV. Restarting it periodically keeps the
	// file bounded; otherwise a long session grows to hundreds of MB and every
	// poll parses the whole file, which looks like a memory/CPU leak.
	if !pm.started.IsZero() && time.Since(pm.started) > 120*time.Second {
		if pm.cmd != nil && pm.cmd.Process != nil {
			_ = pm.cmd.Process.Kill()
		}
		pm = nil
		return stats
	}
	if !pm.started.IsZero() && time.Since(pm.started) > 12*time.Second && pm.frameCount() == 0 {
		if pm.cmd != nil && pm.cmd.Process != nil {
			_ = pm.cmd.Process.Kill()
			pm.cmd = nil
		}
		pm.blocked = true
		c.logf("PresentMon：本机 ETW 12 秒内没有 Present 事件，已停止采集（FPS 显示 --）")
		return stats
	}
	b, err := os.ReadFile(pm.csv)
	if err != nil {
		return stats
	}
	frames := parseOutput(b)
	ready := len(frames) >= 8
	pm.frames = frames
	pm.ready = ready
	stats.Ready = ready
	stats.Frames = len(frames)
	if ready {
		stats.FPS, stats.LowFPS, stats.Stutter = frameStats(frames)
	}
	return stats
}

func (p *presentMon) frameCount() int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.frames)
}

func (p *presentMon) readLocked() ([]frameRow, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.frames, p.ready
}

// consume parses PresentMon's stdout. The CSV is streamed instead of written
// to a live file because PresentMon keeps the output file locked exclusively.
func (p *presentMon) poll() {
	dbg := func(s string) {
		// PresentMon debug logging was useful during bring-up but it writes a
		// growing file on every 300 ms poll. Keep the hook for tests, not disk.
		_ = s
	}
	dbg("poll start " + p.csv)
	t := time.NewTicker(500 * time.Millisecond)
	for range t.C {
		b, err := os.ReadFile(p.csv)
		if err != nil {
			dbg("read err " + err.Error())
			continue
		}
		if len(b) == 0 {
			continue
		}
		rows := parseOutput(b)
		dbg(fmt.Sprintf("poll bytes=%d rows=%d", len(b), len(rows)))
		p.mu.Lock()
		p.frames = rows
		p.ready = len(rows) >= 8
		p.mu.Unlock()
	}
}

func parseOutput(raw []byte) []frameRow {
	// PresentMon writes UTF-16LE with a BOM when stdout is redirected to a
	// file. Decode it before handing the CSV to encoding/csv.
	if len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE {
		u := make([]uint16, 0, (len(raw)-2)/2)
		for i := 2; i+1 < len(raw); i += 2 {
			u = append(u, uint16(raw[i])|uint16(raw[i+1])<<8)
		}
		raw = []byte(string(utf16.Decode(u)))
	}
	cr := csv.NewReader(bytes.NewReader(raw))
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return nil
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	ti, okT := idx["timeinseconds"]
	timeScale := 1.0
	if !okT {
		// PresentMon 2.6 default (v2) calls the column TimeInMs.
		ti, okT = idx["timeinms"]
		timeScale = 0.001
	}
	mi, okM := idx["msbetweenpresents"]
	if !okM {
		mi, okM = idx["msbetweendisplaychange"]
	}
	if !okT || !okM {
		return nil
	}
	var rows []frameRow
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(rec) <= ti || len(rec) <= mi {
			continue
		}
		t, e1 := strconv.ParseFloat(rec[ti], 64)
		ms, e2 := strconv.ParseFloat(rec[mi], 64)
		if e1 != nil || e2 != nil || ms <= 0 || ms > 2000 {
			continue
		}
		rows = append(rows, frameRow{t: t * timeScale, ms: ms})
	}
	if len(rows) > 1200 {
		rows = rows[len(rows)-1200:]
	}
	return rows
}

func frameStats(rows []frameRow) (fps, low, stutter float64) {
	if len(rows) < 2 {
		return
	}
	// PresentMon's CSV accumulates for the whole capture. Average over the
	// entire file mixes menus, loading screens and gameplay, and can produce
	// impossible pairs such as "15 FPS / 58 FPS 1% low". Keep the newest
	// contiguous run (a pause longer than 500 ms starts a new run) and cap it
	// to two seconds of samples.
	start := 0
	for i := len(rows) - 1; i > 0; i-- {
		if rows[i].t-rows[i-1].t > 0.5 {
			start = i
			break
		}
	}
	rows = rows[start:]
	if len(rows) > 240 {
		rows = rows[len(rows)-240:]
	}
	if len(rows) < 2 {
		return
	}
	ms := make([]float64, 0, len(rows))
	var sum float64
	for _, r := range rows {
		if r.ms <= 0.01 || r.ms > 1000 {
			continue
		}
		ms = append(ms, r.ms)
		sum += r.ms
	}
	if len(ms) < 2 || sum <= 0 {
		return
	}
	fps = 1000.0 / (sum / float64(len(ms)))
	sorted := append([]float64{}, ms...)
	sort.Float64s(sorted)
	median := sorted[len(sorted)/2]
	cut := len(ms) / 100
	if cut < 1 {
		cut = 1
	}
	worst := append([]float64{}, ms...)
	sort.Sort(sort.Reverse(sort.Float64Slice(worst)))
	worst = worst[:cut]
	var wsum float64
	for _, v := range worst {
		wsum += v
	}
	low = 1000.0 / (wsum / float64(len(worst)))
	if low > fps {
		low = fps
	}
	threshold := median * 2
	if threshold < 33.3 {
		threshold = 33.3
	}
	bad := 0
	for _, v := range ms {
		if v > threshold {
			bad++
		}
	}
	stutter = float64(bad) / float64(len(ms)) * 100
	return
}

func stopPresentMon() {
	if pm == nil {
		return
	}
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.cmd != nil && pm.cmd.Process != nil {
		_ = pm.cmd.Process.Kill()
		pm.cmd = nil
	}
}

func (c *Control) sampleNetwork(cfg Config, pid uint32, rtts *[]uint32) (uint32, string, string) {
	eps, _ := sys.TCPEndpoints(pid)
	ip, port := "", uint16(0)
	for _, e := range eps {
		if isPublicIPv4(e.RemoteIP) {
			ip, port = e.RemoteIP, e.RemotePort
			break
		}
	}
	if ip == "" {
		return 0, "", ""
	}
	var dst [4]byte
	parsed := net.ParseIP(ip).To4()
	if parsed != nil {
		copy(dst[:], parsed)
	}
	rtt, ok := sys.IcmpRTT(dst, 700)
	if !ok && port != 0 {
		// Many games / ISPs drop ICMP. A TCP handshake to the game's own
		// remote port is still read-only and gives a usable RTT.
		if d, err := sys.TCPRTT(net.JoinHostPort(ip, strconv.Itoa(int(port))), 800*time.Millisecond); err == nil {
			rtt, ok = uint32(d/time.Millisecond), true
		}
	}
	if ok {
		*rtts = append(*rtts, rtt)
		if len(*rtts) > 12 {
			*rtts = (*rtts)[len(*rtts)-12:]
		}
	} else {
		rtt = 0
	}
	province := ""
	if cfg.OverlayServer {
		province = lookupProvince(ip)
	}
	return rtt, ip, province
}

func isPublicIPv4(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() == nil {
		return false
	}
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return false
	}
	return true
}

var (
	provMu    sync.Mutex
	provCache = map[string]string{}
)

// lookupProvince uses a public geo-IP endpoint, only when the user opted in.
func lookupProvince(ip string) string {
	provMu.Lock()
	if v, ok := provCache[ip]; ok {
		provMu.Unlock()
		return v
	}
	provMu.Unlock()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://ip-api.com/json/" + ip + "?lang=zh-CN&fields=status,regionName,city,isp")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return ""
	}
	text := string(body)
	if !strings.Contains(text, `"status":"success"`) {
		return ""
	}
	region := jsonField(text, "regionName")
	city := jsonField(text, "city")
	out := strings.TrimSpace(region)
	if out == "" {
		out = strings.TrimSpace(city)
	}
	if out == "" {
		out = "未知"
	}
	provMu.Lock()
	provCache[ip] = out
	provMu.Unlock()
	return out
}

func jsonField(s, key string) string {
	needle := `"` + key + `":"`
	i := strings.Index(s, needle)
	if i < 0 {
		return ""
	}
	rest := s[i+len(needle):]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		return ""
	}
	return rest[:j]
}

func jitterOf(samples []uint32) uint32 {
	if len(samples) < 2 {
		return 0
	}
	var min, max uint32 = samples[0], samples[0]
	var sum uint64
	for _, v := range samples {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		sum += uint64(v)
	}
	avg := sum / uint64(len(samples))
	_ = avg
	return max - min
}

// estimateLoss is explicitly a heuristic, not a hit-registration measurement:
// it combines ICMP jitter/RTT and frame stutter into a 0..35% risk number.
func estimateLoss(s OverlayStats) float64 {
	if s.RTT == 0 && s.Stutter == 0 {
		return 0
	}
	v := (float64(s.RTT)-35)/8 + float64(s.Jitter)/12 + s.Stutter*0.5
	if v < 0 {
		v = 0
	}
	if v > 35 {
		v = 35
	}
	return v
}

var _ = fmt.Sprintf
