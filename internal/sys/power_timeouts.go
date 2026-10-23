package sys

import (
	"regexp"
	"strconv"
	"strings"
)

// PowerTimeouts is the user's current screen-off / sleep configuration. It is
// captured before switching to Ultimate Performance and re-applied to the new
// plan, so switching plans never resets the user's idle policy.
type PowerTimeouts struct {
	MonitorAC   uint32
	MonitorDC   uint32
	StandbyAC   uint32
	StandbyDC   uint32
	HibernateAC uint32
	HibernateDC uint32
}

var hexRE = regexp.MustCompile(`0x[0-9a-fA-F]+`)

// settingIndex queries one power setting and returns its AC/DC values.
func settingIndex(sub, name string) (ac, dc uint32, ok bool) {
	out, err := runPowercfg("-q", "SCHEME_CURRENT", sub, name)
	if err != nil {
		return 0, 0, false
	}
	vals := []uint32{}
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "0x") {
			continue
		}
		m := hexRE.FindString(line)
		v, err := strconv.ParseUint(strings.TrimPrefix(m, "0x"), 16, 32)
		if err == nil {
			vals = append(vals, uint32(v))
		}
	}
	if len(vals) < 2 {
		return 0, 0, false
	}
	// The last two hex values are the AC and DC current indexes.
	return vals[len(vals)-2], vals[len(vals)-1], true
}

// CurrentTimeouts reads monitor, sleep and hibernate idle values in seconds.
func CurrentTimeouts() PowerTimeouts {
	var t PowerTimeouts
	if ac, dc, ok := settingIndex("SUB_VIDEO", "VIDEOIDLE"); ok {
		t.MonitorAC, t.MonitorDC = ac, dc
	}
	if ac, dc, ok := settingIndex("SUB_SLEEP", "STANDBYIDLE"); ok {
		t.StandbyAC, t.StandbyDC = ac, dc
	}
	if ac, dc, ok := settingIndex("SUB_SLEEP", "HIBERNATEIDLE"); ok {
		t.HibernateAC, t.HibernateDC = ac, dc
	}
	return t
}

func minutes(sec uint32) string {
	if sec == 0 {
		return "0"
	}
	m := (sec + 59) / 60
	return strconv.FormatUint(uint64(m), 10)
}

// ApplyTimeouts writes the captured idle policy to the active plan.
func ApplyTimeouts(t PowerTimeouts) error {
	type kv struct{ name, ac, dc string }
	sets := []kv{
		{"monitor-timeout", minutes(t.MonitorAC), minutes(t.MonitorDC)},
		{"standby-timeout", minutes(t.StandbyAC), minutes(t.StandbyDC)},
		{"hibernate-timeout", minutes(t.HibernateAC), minutes(t.HibernateDC)},
	}
	for _, s := range sets {
		if _, err := runPowercfg("-change", s.name+"-ac", s.ac); err != nil {
			return err
		}
		if _, err := runPowercfg("-change", s.name+"-dc", s.dc); err != nil {
			return err
		}
	}
	return nil
}
