package tune

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"

	"codmtune/internal/sys"
)

// DiskState is what the disk page renders: the volumes, one live I/O sample
// and the result of the last maintenance run.
type DiskState struct {
	Volumes  []sys.Volume
	Disks    []sys.DiskIO
	IOErr    string
	Target   string
	TrimOn   bool
	TrimRead bool
	Busy     string // non-empty while a defrag/retrim is running
	Last     sys.VolumeReport
	HasLast  bool
	SysMain  string // 运行中 / 已停止 / 已禁用 / 未知
	Prefetch string // 只读说明，来自注册表
	IOPid    uint32
	IONote   string
}

// diskBusy is the in-flight maintenance label; empty means idle.
var diskBusy string

// diskSnapshot is the slow half of the page: enumerating volumes opens a handle
// and runs two ioctls per letter, the service and prefetch rows hit the SCM and
// the registry, and reading the game's I/O priority opens a process handle.
// None of that belongs on the paint path — Disk() is called by every row on
// every frame — so all of it is cached and refreshed off the UI goroutine.
type diskSnapshot struct {
	at       time.Time
	volumes  []sys.Volume
	trimOn   bool
	trimRead bool
	sysmain  string
	prefetch string
	ioPid    uint32
	ioNote   string
}

// readDiskSnapshot collects one snapshot of the slow rows.
func (c *Control) readDiskSnapshot() diskSnapshot {
	f := diskSnapshot{at: time.Now(), volumes: sys.Volumes()}
	if on, known := sys.TrimEnabled(); known {
		f.trimOn, f.trimRead = on, true
	}
	f.sysmain = serviceState("SysMain")
	f.prefetch = prefetchNote()
	f.ioPid, f.ioNote = c.ioBoostState()
	return f
}

// DiskIO is re-exported so the page can describe a sample without importing
// the sys layer just for a type.
type DiskIO = sys.DiskIO

// DiskIOName is the UI label for one I/O priority level.
func DiskIOName(level int) string { return sys.IOPriorityName(level) }

// DiskIOLevels are the levels the page offers, lowest first. The critical
// level is left out on purpose: NtSetInformationProcess rejects it for an
// ordinary process, so offering it would only produce a failure.
var DiskIOLevels = []int{sys.IOLow, sys.IONormal, sys.IOHigh}

// Disk returns the current disk panel state.
func (c *Control) Disk() DiskState {
	c.mu.Lock()
	target := c.cfg.DiskTarget
	last := c.diskLast
	hasLast := c.diskHasLast
	busy := diskBusy
	snap := c.diskSnap
	c.mu.Unlock()
	if snap == nil {
		// First call builds the page, so paying for the scan here is the one
		// place it cannot be deferred; later refreshes run in the background.
		f := c.readDiskSnapshot()
		snap = &f
		c.mu.Lock()
		c.diskSnap = snap
		c.mu.Unlock()
	} else if time.Since(snap.at) > 3*time.Second {
		if c.diskFacting.CompareAndSwap(false, true) {
			go func() {
				defer c.diskFacting.Store(false)
				f := c.readDiskSnapshot()
				c.mu.Lock()
				c.diskSnap = &f
				c.mu.Unlock()
				c.changed()
			}()
		}
	}

	st := DiskState{Target: target, Last: last, HasLast: hasLast, Busy: busy}
	st.Volumes = snap.volumes
	st.TrimOn, st.TrimRead = snap.trimOn, snap.trimRead
	st.SysMain = snap.sysmain
	st.Prefetch = snap.prefetch
	if len(st.Volumes) > 0 && !hasVolumeLetter(st.Volumes, target) {
		// The remembered volume is gone (unplugged drive, remounted letter):
		// fall back to the game's own drive, then to the first volume.
		target = c.gameDrive()
		if target == "" || !hasVolumeLetter(st.Volumes, target) {
			target = st.Volumes[0].Letter
		}
		st.Target = target
	}
	st.Disks, st.IOErr = c.diskSample()
	st.IOPid, st.IONote = snap.ioPid, snap.ioNote
	return st
}

// hasVolumeLetter reports whether letter is among the mounted volumes.
func hasVolumeLetter(v []sys.Volume, letter string) bool {
	for _, x := range v {
		if strings.EqualFold(x.Letter, letter) {
			return true
		}
	}
	return false
}

// gameDrive is the drive letter the game is installed on.
func (c *Control) gameDrive() string {
	c.mu.Lock()
	exe := c.gameExe
	c.mu.Unlock()
	if len(exe) >= 2 && exe[1] == ':' {
		return strings.ToUpper(exe[:2])
	}
	return ""
}

// diskSample returns the cached sample and refreshes it in the background when
// it is older than two seconds. PDH needs a 250 ms collection window, so it can
// never run on the paint path.
func (c *Control) diskSample() ([]sys.DiskIO, string) {
	c.mu.Lock()
	disks, at, err := c.diskIO, c.diskIOAt, c.diskIOErr
	stale := time.Since(at) > 2*time.Second
	c.mu.Unlock()
	if !stale {
		return disks, err
	}
	if !c.diskSampling.CompareAndSwap(false, true) {
		return disks, err
	}
	go func() {
		defer c.diskSampling.Store(false)
		out, e := sys.DiskUsage()
		msg := ""
		if e != nil {
			msg = e.Error()
		}
		c.mu.Lock()
		c.diskIO, c.diskIOAt, c.diskIOErr = out, time.Now(), msg
		c.mu.Unlock()
		c.changed()
	}()
	return disks, err
}

// SelectDiskTarget remembers which volume the maintenance rows act on.
func (c *Control) SelectDiskTarget(letter string) {
	c.Update(func(cfg *Config) { cfg.DiskTarget = strings.ToUpper(letter) })
	c.logf("磁盘：目标卷切换为 %s", letter)
}

// RunVolumeAction starts one defrag operation on the target volume. It runs on
// a goroutine because a real defrag takes minutes; the UI shows the busy label
// from Disk().Busy until it returns.
func (c *Control) RunVolumeAction(action string) {
	target := c.resolveTarget()
	if target == "" {
		c.logf("磁盘：没有可用卷，%s 取消", actionLabel(action))
		return
	}
	c.mu.Lock()
	if diskBusy != "" {
		c.mu.Unlock()
		return
	}
	diskBusy = actionLabel(action) + " " + target
	c.mu.Unlock()
	c.changed()

	go func() {
		rep := sys.RunDefrag(target, action)
		c.mu.Lock()
		diskBusy = ""
		c.diskLast, c.diskHasLast = rep, true
		if action == "optimize" {
			c.cfg.DiskLastOpt = time.Now().Unix()
		}
		if action == "retrim" {
			c.cfg.DiskLastTrim = time.Now().Unix()
		}
		c.mu.Unlock()
		c.saveCfg()
		switch {
		case rep.Err != "":
			c.logf("磁盘：%s %s 失败 · %s", actionLabel(action), target, rep.Err)
		case rep.FragPct >= 0:
			c.logf("磁盘：%s %s 完成 · 碎片 %d%%", actionLabel(action), target, rep.FragPct)
		default:
			c.logf("磁盘：%s %s 完成", actionLabel(action), target)
		}
		c.changed()
	}()
}

// resolveTarget picks the volume the maintenance rows act on: the remembered
// one when it still exists, otherwise the game's drive, otherwise the first
// mounted volume. Disk() applies the same order against its cached volume list,
// so the row the user sees as "current target" is the volume defrag is given.
func (c *Control) resolveTarget() string {
	c.mu.Lock()
	target := c.cfg.DiskTarget
	c.mu.Unlock()
	vols := sys.Volumes()
	if target != "" && hasVolumeLetter(vols, target) {
		return target
	}
	if d := c.gameDrive(); d != "" && hasVolumeLetter(vols, d) {
		return d
	}
	if len(vols) > 0 {
		return vols[0].Letter
	}
	return ""
}

func actionLabel(a string) string {
	switch a {
	case "analyze":
		return "分析"
	case "optimize":
		return "优化"
	case "retrim":
		return "重新剪裁"
	}
	return a
}

// ---- game I/O priority -------------------------------------------------

// ioBoostState reports the current I/O priority of the game process.
func (c *Control) ioBoostState() (uint32, string) {
	c.mu.Lock()
	pid := c.gamePID
	running := c.gameRunning
	enabled := c.cfg.DiskIOBoost
	c.mu.Unlock()
	if !running || pid == 0 {
		if enabled {
			return 0, "游戏未运行；启动后自动应用"
		}
		return 0, "关闭"
	}
	lvl, err := sys.GetIOPriority(pid)
	if err != nil {
		return pid, "读取失败：" + err.Error()
	}
	now := sys.IOPriorityName(lvl)
	if enabled && lvl != c.Cfg().DiskIOLevel {
		return pid, "当前 " + now + " · 目标 " + sys.IOPriorityName(c.Cfg().DiskIOLevel) + "（下次检测生效）"
	}
	return pid, "当前 " + now
}

// applyDiskIO raises the game's I/O priority. It is called when the game is
// first seen running, and again whenever the level changes while it runs.
func (c *Control) applyDiskIO() {
	c.mu.Lock()
	if !c.cfg.DiskIOBoost || !c.gameRunning || c.gamePID == 0 {
		c.mu.Unlock()
		return
	}
	pid, level := c.gamePID, c.cfg.DiskIOLevel
	c.mu.Unlock()
	if err := sys.EnableIOPriorityPrivilege(); err != nil {
		c.logf("磁盘：SeIncreaseBasePriorityPrivilege 未取得（%v），I/O 优先级可能只升不降", err)
	}
	if err := sys.SetIOPriority(pid, level); err != nil {
		c.logf("磁盘：设置游戏 I/O 优先级失败 · %v", err)
		c.changed()
		return
	}
	c.logf("磁盘：游戏 I/O 优先级设为 %s", sys.IOPriorityName(level))
	c.changed()
}

// restoreDiskIO puts the game process back to Normal when the boost is turned
// off while the game is still running. Nothing has to be restored when the
// game exits: the process object is gone and its I/O priority with it.
func (c *Control) restoreDiskIO() {
	c.mu.Lock()
	pid := c.gamePID
	c.mu.Unlock()
	if pid == 0 {
		return
	}
	if err := sys.SetIOPriority(pid, sys.IONormal); err != nil {
		c.logf("磁盘：恢复 I/O 优先级失败 · %v", err)
		return
	}
	c.logf("磁盘：游戏 I/O 优先级恢复普通")
	c.changed()
}

// SetDiskIOBoost flips the game I/O boost and applies or reverts it at once.
func (c *Control) SetDiskIOBoost(on bool) {
	c.Update(func(cfg *Config) { cfg.DiskIOBoost = on })
	if on {
		c.applyDiskIO()
		return
	}
	c.restoreDiskIO()
}

// SetDiskIOLevel changes the level used the next time the boost is applied.
func (c *Control) SetDiskIOLevel(level int) {
	if level < sys.IOLow {
		level = sys.IOLow
	}
	if level > sys.IOHigh {
		level = sys.IOHigh
	}
	c.Update(func(cfg *Config) { cfg.DiskIOLevel = level })
	c.applyDiskIO()
}

// ---- read-only facts ---------------------------------------------------

// prefetchNote reads the prefetch/superfetch switches. They are reported, not
// written: on an SSD the modern default is already correct and a wrong value
// costs boot time for no frame-time gain.
func prefetchNote() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management\PrefetchParameters`,
		registry.QUERY_VALUE)
	if err != nil {
		return "—"
	}
	defer k.Close()
	read := func(name string) string {
		v, _, err := k.GetIntegerValue(name)
		if err != nil {
			return "—"
		}
		return fmt.Sprint(v)
	}
	return fmt.Sprintf("Prefetch=%s · Superfetch=%s（只读）", read("EnablePrefetcher"), read("EnableSuperfetch"))
}

// serviceState reports one service's state in the page's own words.
func serviceState(name string) string {
	out, err := sys.QueryService(name)
	if err != nil {
		return "未知"
	}
	return out
}
