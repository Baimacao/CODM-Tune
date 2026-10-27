package tune

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func TestParseCSVBothColumnLayouts(t *testing.T) {
	v2 := "Application,ProcessID,TimeInMs,MsBetweenPresents\ncod.exe,42,1000.0,7.0\ncod.exe,42,1007.0,6.5\n"
	rows, header := parseCSV([]byte(v2))
	if len(rows) != 2 || header == "" {
		t.Fatalf("v2: rows=%d header=%q", len(rows), header)
	}
	if rows[0].t < 0.99 || rows[0].t > 1.01 || rows[0].ms != 7.0 {
		t.Fatalf("v2 first row = %+v, want t=1.0 ms=7.0", rows[0])
	}
	// PresentMon 1.x names the column TimeInSeconds; there it is already seconds.
	v1 := "Application,ProcessID,TimeInSeconds,MsBetweenPresents\ncod.exe,42,2.0,8.0\n"
	rows, _ = parseCSV([]byte(v1))
	if len(rows) != 1 || rows[0].t != 2.0 {
		t.Fatalf("v1 rows=%+v, want t=2.0", rows)
	}
	if rows, _ := parseCSV([]byte("nonsense\n1,2\n")); rows != nil {
		t.Fatalf("missing columns must yield no rows, got %+v", rows)
	}
}

func TestReadTailIgnoresBOMAndPartialFirstLine(t *testing.T) {
	p := &presentMon{csv: filepath.Join(t.TempDir(), "frames.csv")}
	var sb strings.Builder
	sb.WriteString("Application,ProcessID,TimeInMs,MsBetweenPresents\n")
	for i := 0; i < 40000; i++ { // ~1.6 MB, well past the tail window
		sb.WriteString("cod.exe,42,")
		sb.WriteString(itoaTest(i * 7))
		sb.WriteString(".0,7.0\n")
	}
	if err := os.WriteFile(p.csv, encodeUTF16Test(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := p.readTail()
	if len(rows) < 100 {
		t.Fatalf("tail read returned %d rows; the header must be re-attached", len(rows))
	}
	// The tail is parsed without touching the head, yet every row still decoded
	// as UTF-16 and resolved its columns.
	last := rows[len(rows)-1]
	if last.ms != 7.0 {
		t.Fatalf("last row ms=%v, want 7.0", last.ms)
	}
	if !p.encKnown || !p.utf16 {
		t.Fatalf("encoding probe = known:%v wide:%v", p.encKnown, p.utf16)
	}
	if p.headerLine == "" {
		t.Fatal("header line must be cached for later tail reads")
	}
	// A second read reuses the cached header instead of re-reading the head.
	if rows2 := p.readTail(); len(rows2) != len(rows) {
		t.Fatalf("second read rows=%d, want %d", len(rows2), len(rows))
	}
}

func encodeUTF16Test(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 0, len(u)*2+2)
	b = append(b, 0xFF, 0xFE)
	for _, v := range u {
		b = append(b, byte(v), byte(v>>8))
	}
	return b
}

func itoaTest(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestFrameStatsUsesRecentContinuousRun(t *testing.T) {
	var rows []frameRow
	now := 0.0
	// Old, slow menu run.
	for i := 0; i < 120; i++ {
		rows = append(rows, frameRow{t: now, ms: 66})
		now += 0.066
	}
	// A long pause starts a new run.
	now += 1.5
	start := now
	// Recent gameplay run: ~143 FPS, with one 40 ms hitch.
	for i := 0; i < 120; i++ {
		ms := 7.0
		if i == 60 {
			ms = 40
		}
		rows = append(rows, frameRow{t: now, ms: ms})
		now += ms / 1000
	}
	fps, low, stutter := frameStats(rows)
	if fps < 100 || fps > 190 {
		t.Fatalf("fps=%.1f, want recent run near 140", fps)
	}
	if low <= 0 || low > fps {
		t.Fatalf("low=%.1f fps=%.1f; 1%% low must be in (0, fps]", low, fps)
	}
	if stutter < 0 || stutter > 5 {
		t.Fatalf("stutter=%.2f%% out of range", stutter)
	}
	if now-start <= 0 {
		t.Fatal("bad test clock")
	}
}

// TestReadTailKeepsEncodingWhenFirstPollIsEmpty covers the bug that turned FPS
// into a permanent N/A: the first poll ran before PresentMon wrote anything,
// the BOM probe read zero bytes, and the "not UTF-16" answer was cached, so
// every later poll decoded UTF-16LE as raw bytes and found no header.
func TestReadTailKeepsEncodingWhenFirstPollIsEmpty(t *testing.T) {
	p := &presentMon{csv: filepath.Join(t.TempDir(), "frames.csv")}
	if err := os.WriteFile(p.csv, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if rows := p.readTail(); len(rows) != 0 {
		t.Fatalf("empty file must yield no rows, got %d", len(rows))
	}
	if p.encKnown {
		t.Fatal("an empty file must not settle the encoding probe")
	}
	var sb strings.Builder
	sb.WriteString("Application,ProcessID,TimeInMs,MsBetweenPresents\n")
	for i := 0; i < 400; i++ {
		sb.WriteString("cod.exe,42,")
		sb.WriteString(itoaTest(i * 5))
		sb.WriteString(".0,5.5\n")
	}
	if err := os.WriteFile(p.csv, encodeUTF16Test(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := p.readTail()
	if len(rows) < 100 {
		t.Fatalf("after the file fills, rows=%d; want the UTF-16 header to resolve", len(rows))
	}
	if !p.utf16 {
		t.Fatal("UTF-16 must be detected once real bytes exist")
	}
	if fps, _, _ := frameStats(rows); fps < 150 || fps > 200 {
		t.Fatalf("fps=%.1f, want about 181", fps)
	}
}

// A no-Present verdict is a pause, not a verdict: the game may still be
// loading. The collector has to come back on its own instead of leaving the
// panel on "--" for the rest of the session.
func TestBlockedCollectorRetriesAfterCoolDown(t *testing.T) {
	now := time.Now()
	p := &presentMon{blocked: true, blockedAt: now}

	if !p.paused(now) {
		t.Fatal("a collector blocked a moment ago must still be paused")
	}
	if !p.paused(now.Add(pmCoolDown - time.Second)) {
		t.Fatal("cool-down must last the whole interval")
	}
	if p.paused(now.Add(pmCoolDown)) {
		t.Fatal("cool-down must expire so collection can resume")
	}
	if p.paused(now.Add(pmCoolDown + time.Minute)) {
		t.Fatal("an expired cool-down must stay expired")
	}
}

// Stopping the collector for a game exit must not carry the pause into the
// next launch: a fresh game should start measuring immediately.
func TestStopPresentMonClearsPause(t *testing.T) {
	saved := pm
	defer func() { pm = saved }()

	pm = &presentMon{blocked: true, blockedAt: time.Now()}
	stopPresentMon()
	if pm.blocked || !pm.blockedAt.IsZero() {
		t.Fatalf("stop must clear the pause, got blocked=%t at=%v", pm.blocked, pm.blockedAt)
	}
}
