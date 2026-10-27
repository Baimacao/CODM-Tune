package tune

import (
	"strconv"
	"time"

	"codmtune/internal/sys"
)

// MemState is what the system page shows about reclamation.
type MemState struct {
	Stat       sys.MemStat
	LastUnix   int64
	LastFreed  int64
	TotalFreed int64
	Runs       int
	Auto       bool
	Interval   int // effective minutes for the current mode
	NextUnix   int64
	Failed     []string
	Note       string
}

// MemStat returns the live physical-memory snapshot.
func (c *Control) MemStat() sys.MemStat { return sys.MemoryStat() }

// MemState returns the last reclamation result plus the schedule.
func (c *Control) MemState() MemState {
	cfg := c.Cfg()
	c.mu.Lock()
	st := MemState{
		LastUnix:   cfg.MemLastUnix,
		LastFreed:  cfg.MemLastFreed,
		TotalFreed: cfg.MemTotalFreed,
		Runs:       cfg.MemRuns,
		Auto:       cfg.MemAuto,
		Interval:   cfg.MemInterval,
	}
	gameRunning := c.gameRunning
	c.mu.Unlock()
	st.Stat = sys.MemoryStat()
	if gameRunning {
		st.Interval = cfg.MemGameGap
	}
	if st.Auto {
		base := cfg.MemLastUnix
		if base == 0 {
			base = time.Now().Unix()
		}
		st.NextUnix = base + int64(st.Interval)*60
	}
	switch {
	case !st.Auto:
		st.Note = "自动回收关闭，仅手动执行"
	case gameRunning:
		st.Note = "游戏运行中按更短间隔回收（不影响游戏进程本身）"
	default:
		st.Note = "空闲时按设定间隔回收"
	}
	return st
}

// RunMemory performs one reclamation pass with the configured mask and records
// the outcome. It never touches another process: the whole operation is a set
// of documented NT memory-list calls.
func (c *Control) RunMemory(mask sys.MemMask) MemState {
	if mask == 0 {
		mask = sys.MemMask(c.Cfg().MemMask)
	}
	// SeProfileSingleProcessPrivilege is required for the standby-list purge;
	// without it Windows returns STATUS_PRIVILEGE_NOT_HELD and the pass is a
	// no-op. Enabling it is a token change on this process only.
	if err := sys.EnableMemPrivileges(); err != nil {
		c.logf("内存回收：权限未取得（%v），将以普通权限尝试", err)
	}
	before, after, failed := sys.FlushMemory(mask)
	freed := int64(after.Avail) - int64(before.Avail)
	if freed < 0 {
		freed = 0
	}
	now := time.Now().Unix()
	c.mu.Lock()
	c.cfg.MemMask = int(mask)
	c.cfg.MemLastUnix = now
	c.cfg.MemLastFreed = freed
	c.cfg.MemTotalFreed += freed
	c.cfg.MemRuns++
	c.mu.Unlock()
	c.saveCfg()
	detail := "释放 " + humanMB(freed) + " · 可用 " +
		humanMB(int64(after.Avail)) + " / " + humanMB(int64(after.Total))
	if len(failed) > 0 {
		detail += " · 跳过 " + joinNames(failed)
	}
	c.logf("内存回收：%s", detail)
	st := c.MemState()
	st.Failed = failed
	c.changed()
	return st
}

// humanMB formats a byte count the way the rail does.
func humanMB(b int64) string {
	if b < 0 {
		b = 0
	}
	mb := b / (1024 * 1024)
	if mb >= 1024 {
		return strconv.FormatFloat(float64(mb)/(1024), 'f', 2, 64) + " GB"
	}
	return strconv.FormatInt(mb, 10) + " MB"
}

func joinNames(v []string) string {
	out := ""
	for i, s := range v {
		if i > 0 {
			out += "/"
		}
		out += s
	}
	return out
}

// memLoop runs the automatic schedule. The ticker is one minute: the interval
// is user-configurable in minutes, and a one-minute tick keeps the loop itself
// nearly free (two config reads per minute).
func (c *Control) memLoop() {
	tick := time.NewTicker(60 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-tick.C:
			cfg := c.Cfg()
			if !cfg.MemAuto {
				continue
			}
			c.mu.Lock()
			running := c.gameRunning
			c.mu.Unlock()
			gap := cfg.MemInterval
			if running {
				gap = cfg.MemGameGap
			}
			if gap < 2 {
				gap = 2
			}
			if cfg.MemLastUnix != 0 && time.Since(time.Unix(cfg.MemLastUnix, 0)) < time.Duration(gap)*time.Minute {
				continue
			}
			// Do not run while the game window is in the foreground and under
			// load: the purge itself is cheap, but a standby purge mid-match
			// can cost one frame on a machine that is already short on RAM.
			if running && cfg.MemGameGap >= 60 {
				continue
			}
			c.RunMemory(sys.MemMask(cfg.MemMask))
		}
	}
}
