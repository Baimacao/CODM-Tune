package appui

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"codmtune/internal/sys"
	"codmtune/internal/tune"
	"golang.org/x/sys/windows/registry"
)

// exportDiagnostics writes a single text bundle the testers can send back.
// It contains paths, counters and FOV bytes only; no credentials or process
// memory.
func (s *screens) exportDiagnostics() {
	var b strings.Builder
	fmt.Fprintf(&b, "CODMTune v8 diagnostic\n")
	fmt.Fprintf(&b, "time=%s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(&b, "go=%s os=%s arch=%s pid=%d admin=%v\n", runtime.Version(), runtime.GOOS, runtime.GOARCH, os.Getpid(), sys.IsElevated())
	fmt.Fprintf(&b, "config_dir=%s\n", tune.ConfigPath())
	cfg := s.c.Cfg()
	if raw, err := json.MarshalIndent(cfg, "", "  "); err == nil {
		b.WriteString("\n[config]\n")
		b.Write(raw)
		b.WriteByte('\n')
	}
	g := s.c.GPU()
	n := s.c.Net()
	p := s.c.Power()
	fmt.Fprintf(&b, "\n[state]\ngpu=%q\ntdr=%v\ndns=%s dns_err=%q\npower=%q prior=%q injected=%v\nace=%q sched=%q\n",
		g.Actual, g.TdrOn, n.DNSTime, n.DNSErr, p.Name, p.PriorGUID, p.Injected, s.c.ACE().Text, s.c.Sched().Text)
	if d := s.c.OverlayData(); true {
		fmt.Fprintf(&b, "overlay={ready:%v fps:%.1f low:%.1f stutter:%.1f rtt:%d jitter:%d loss:%.0f server:%s}\n",
			d.Ready, d.FPS, d.LowFPS, d.Stutter, d.RTT, d.Jitter, d.Loss, d.ServerIP)
	}
	b.WriteString("\n[registry]\n")
	if k, err := registry.OpenKey(registry.CURRENT_USER, `SOFTWARE\Tencent\Call-of-Duty`, registry.QUERY_VALUE); err == nil {
		names, _ := k.ReadValueNames(-1)
		for _, name := range names {
			lower := strings.ToLower(name)
			if !strings.Contains(lower, "fov") && !strings.Contains(lower, "framerate") {
				continue
			}
			buf := make([]byte, 32)
			if sz, _, err := k.GetValue(name, buf); err == nil && sz > 0 {
				fmt.Fprintf(&b, "  bytes=%s\n", strings.ToUpper(hex.EncodeToString(buf[:sz])))
			}
		}
		k.Close()
	} else {
		fmt.Fprintf(&b, "open key failed: %v\n", err)
	}
	if raw, err := os.ReadFile(filepath.Join(tune.ConfigPath(), "overlay-state.json")); err == nil {
		b.WriteString("\n[hud state]\n")
		b.Write(raw)
		b.WriteByte('\n')
	}
	b.WriteString("\n[logs]\n")
	logs := s.c.Logs()
	if len(logs) > 200 {
		logs = logs[len(logs)-200:]
	}
	for _, line := range logs {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	path := filepath.Join(tune.ConfigPath(), "diagnostic-"+time.Now().Format("20060102-150405")+".txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		s.c.LogLine("诊断导出失败：" + err.Error())
		return
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		s.c.LogLine("诊断导出失败：" + err.Error())
		return
	}
	s.c.LogLine("诊断报告已导出：" + path)
	_ = openPath(path)
}
