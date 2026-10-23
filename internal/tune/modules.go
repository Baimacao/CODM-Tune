package tune

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"

	"codmtune/internal/sys"
)

const (
	// ACEWarning is shown unfolded next to the ACE module.
	ACEWarning = "降低 ACE 相关进程的 CPU 优先级与核心范围，不结束、不挂起、不注入；社区测试暂未发现普遍封号，但不排除误封，启用即表示用户自愿承担账号风险。"
	// PowerWarning is shown with the power module.
	PowerWarning = "卓越性能会显著增加功耗和发热，但能减少 CPU 降频，有利于 frametime 稳定性。"
)

const regKeyPath = `SOFTWARE\Tencent\Call-of-Duty`

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// ---- ACE limiter -------------------------------------------------------

func (c *Control) setACE(on bool) {
	c.mu.Lock()
	c.cfg.ACEEnabled = on
	if on {
		c.ace = Status{Text: "正在限制 ACE…", Detail: "Idle 优先级 + 单核亲和性；每 60 秒重新枚举", Tone: ToneNeutral, Busy: true}
	} else {
		c.ace = Status{Text: "未启用", Detail: "默认关闭；启用后以 Idle 优先级与单核亲和性限制 ACE 相关进程", Tone: ToneNeutral}
	}
	c.changed()
	c.mu.Unlock()
	c.saveCfg()
	if on {
		c.logf("ACE 资源限制：启用")
		c.lastACEAt = time.Time{}
		go c.aceTick()
	} else {
		c.logf("ACE 资源限制：关闭，恢复 Normal 优先级与全部核心")
		go c.restoreACE()
	}
}

// aceTick re-enumerates every cycle with one snapshot; no PIDs are cached.
func (c *Control) aceTick() {
	c.mu.Lock()
	if !c.cfg.ACEEnabled {
		c.mu.Unlock()
		return
	}
	names := append([]string{}, c.cfg.ACENames...)
	c.lastACEAt = time.Now()
	c.mu.Unlock()

	procs, err := sys.Snapshot()
	if err != nil {
		c.aceFail = err.Error()
		c.mu.Lock()
		c.ace = Status{Text: "限制失效", Detail: "进程快照失败：" + err.Error(), Tone: ToneBad}
		c.changed()
		c.mu.Unlock()
		return
	}
	var targets []sys.Proc
	for _, p := range procs {
		for _, n := range names {
			if strings.EqualFold(p.Name, n) {
				targets = append(targets, p)
				break
			}
		}
	}
	if len(targets) == 0 {
		c.mu.Lock()
		c.ace = Status{Text: "等待 ACE 启动", Detail: "未发现 SGuard64.exe / SGuardSvc64.exe；游戏启动后自动生效", Tone: ToneNeutral}
		c.changed()
		c.mu.Unlock()
		return
	}
	mask := sys.SingleCore()
	limited := 0
	var errs []string
	for _, p := range targets {
		e1 := sys.SetPriorityClass(p.PID, sys.IDLE_PRIORITY_CLASS)
		e2 := sys.SetAffinity(p.PID, mask)
		if e1 == nil && e2 == nil {
			limited++
		} else {
			errs = append(errs, fmt.Sprintf("%s: %v", p.Name, firstErr(e1, e2)))
		}
	}
	c.mu.Lock()
	switch {
	case limited == 0:
		c.cfg.ACEEnabled = false
		c.ace = Status{Text: "限制失效，已关闭", Detail: "系统拒绝访问：" + strings.Join(errs, "；"), Tone: ToneBad}
		c.changed()
		c.mu.Unlock()
		c.saveCfg()
		c.logf("ACE 限制失效，已关闭：%s", strings.Join(errs, "；"))
		return
	case limited < len(targets):
		c.ace = Status{Text: fmt.Sprintf("部分限制 %d/%d", limited, len(targets)), Detail: "Idle + 单核 " + sys.MaskLabel(mask) + "；" + strings.Join(errs, "；"), Tone: ToneWarn}
	default:
		c.ace = Status{Text: fmt.Sprintf("限制中 · %d 个 ACE 进程", limited), Detail: "Idle 优先级 · 单核亲和性 " + sys.MaskLabel(mask) + " · 每 60 秒重新枚举", Tone: ToneGood}
	}
	c.changed()
	c.mu.Unlock()
	c.logf("ACE 限制：%d/%d 生效，掩码 %s", limited, len(targets), sys.MaskLabel(mask))
}

func (c *Control) restoreACE() {
	data, _ := sys.Snapshot()
	c.mu.Lock()
	names := append([]string{}, c.cfg.ACENames...)
	c.mu.Unlock()
	full := (uint64(1) << uint(sys.LogicalCPUs())) - 1
	count := 0
	for _, p := range data {
		for _, n := range names {
			if strings.EqualFold(p.Name, n) {
				if sys.SetPriorityClass(p.PID, sys.NORMAL_PRIORITY_CLASS) == nil {
					_ = sys.SetAffinity(p.PID, full)
					count++
				}
			}
		}
	}
	if count > 0 {
		c.logf("ACE 已恢复：%d 个进程", count)
	}
}

// ---- background suppression -------------------------------------------

var sysProcNames = map[string]bool{
	"system": true, "registry": true, "smss.exe": true, "csrss.exe": true,
	"wininit.exe": true, "winlogon.exe": true, "services.exe": true, "lsass.exe": true,
	"svchost.exe": true, "fontdrvhost.exe": true, "dwm.exe": true, "explorer.exe": true,
	"sihost.exe": true, "taskhostw.exe": true, "ctfmon.exe": true,
	"startmenuexperiencehost.exe": true, "shellexperiencehost.exe": true,
	"searchhost.exe": true, "textinputhost.exe": true, "applicationframehost.exe": true,
	"lockapp.exe": true, "runtimebroker.exe": true, "wmiprvse.exe": true,
	"conhost.exe": true, "openconsole.exe": true, "codmtune.exe": true,
	"sguard64.exe": true, "sguardsvc64.exe": true, "ace-service64.exe": true,
	"ace-helper.exe": true, "sguardupdate64.exe": true, "msmpeng.exe": true,
	"securityhealthservice.exe": true, "audiodg.exe": true, "spoolsv.exe": true,
	"dllhost.exe": true, "backgroundtaskhost.exe": true, "crashpad_handler.exe": true,
	"searchindexer.exe": true, "sppsvc.exe": true, "usocoreworker.exe": true,
}

func (c *Control) setBG(on bool) {
	c.mu.Lock()
	c.cfg.BGEnabled = on
	c.mu.Unlock()
	c.saveCfg()
	if !on {
		c.restoreBackground()
		c.mu.Lock()
		c.bg.Enabled = false
		c.bg.Text, c.bg.Detail, c.bg.Tone = "未启用", "游戏运行时临时降低选定后台进程的调度权重，游戏退出自动恢复", ToneNeutral
		c.bg.Applied = 0
		c.changed()
		c.mu.Unlock()
		c.logf("后台抑制：关闭")
		return
	}
	c.logf("后台抑制：启用")
	c.mu.Lock()
	c.bg.Enabled = true
	c.mu.Unlock()
	c.RefreshProcesses()
	c.mu.Lock()
	running := c.gameRunning
	c.mu.Unlock()
	if running {
		c.applyBackground()
	} else {
		c.mu.Lock()
		c.bg.Text = "待游戏启动"
		c.bg.Detail = "游戏运行时应用，退出后自动恢复原优先级与亲和性"
		c.changed()
		c.mu.Unlock()
	}
}

// RefreshProcesses samples windowed processes for the candidate list. It is
// an on-demand UI refresh, not the 30s suppression loop.
func (c *Control) RefreshProcesses() { go c.refreshProcesses() }

func (c *Control) refreshProcesses() {
	pids := sys.VisibleWindowPIDs()
	procs, err := sys.Snapshot()
	if err != nil {
		return
	}
	byPID := map[uint32]sys.Proc{}
	for _, p := range procs {
		byPID[p.PID] = p
	}
	c.mu.Lock()
	gameName := strings.ToLower(filepath.Base(c.gameExe))
	selected := map[string]bool{}
	for _, n := range c.cfg.BGSelected {
		selected[strings.ToLower(n)] = true
	}
	prev := c.procCPU
	c.mu.Unlock()

	now := time.Now()
	next := map[uint32]procSample{}
	agg := map[string]*BgProc{}
	for _, pid := range pids {
		p, ok := byPID[pid]
		if !ok {
			continue
		}
		name := strings.ToLower(p.Name)
		if sysProcNames[name] || name == gameName || name == "" {
			continue
		}
		cpu, err := sys.CPUTime(pid)
		if err != nil {
			continue
		}
		ws, _ := sys.WorkingSet(pid)
		pct := 0.0
		if old, ok := prev[pid]; ok && now.After(old.at) {
			d := float64(cpu-old.cpu) / (float64(now.Sub(old.at))/float64(time.Second)*1e7) / float64(sys.LogicalCPUs()) * 100
			if d >= 0 {
				pct = d
			}
		}
		next[pid] = procSample{cpu: cpu, at: now, ws: ws}
		row := agg[name]
		if row == nil {
			row = &BgProc{Name: p.Name, Selected: selected[name]}
			agg[name] = row
		}
		row.PIDs++
		row.CPU += pct
		row.MemMB += float64(ws) / (1024 * 1024)
	}
	rows := make([]BgProc, 0, len(agg))
	for _, r := range agg {
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CPU != rows[j].CPU {
			return rows[i].CPU > rows[j].CPU
		}
		return rows[i].MemMB > rows[j].MemMB
	})
	if len(rows) > 40 {
		rows = rows[:40]
	}
	c.mu.Lock()
	c.procCPU = next
	c.bg.Procs = rows
	c.changed()
	c.mu.Unlock()
}

// ToggleProcess selects a process name for suppression.
func (c *Control) ToggleProcess(name string, on bool) {
	lower := strings.ToLower(name)
	c.mu.Lock()
	var keep []string
	for _, n := range c.cfg.BGSelected {
		if strings.ToLower(n) != lower {
			keep = append(keep, n)
		}
	}
	if on {
		keep = append(keep, name)
	}
	c.cfg.BGSelected = keep
	for i := range c.bg.Procs {
		if strings.EqualFold(c.bg.Procs[i].Name, name) {
			c.bg.Procs[i].Selected = on
		}
	}
	c.changed()
	running := c.gameRunning
	enabled := c.cfg.BGEnabled
	c.mu.Unlock()
	c.saveCfg()
	if running && enabled {
		c.restoreBackground()
		c.applyBackground()
	}
}

func lowMask() uint64 {
	n := sys.LogicalCPUs()
	if n <= 2 {
		return 1
	}
	return 3 << uint(n-2)
}

func (c *Control) applyBackground() {
	c.mu.Lock()
	enabled := c.cfg.BGEnabled
	name := strings.ToLower(filepath.Base(c.gameExe))
	names := append([]string{}, c.cfg.BGSelected...)
	c.mu.Unlock()
	if !enabled {
		return
	}
	if len(names) == 0 {
		c.mu.Lock()
		c.bg.Text = "已启用 · 尚未选择进程"
		c.bg.Detail = "在进程列表按 Enter 选择需要抑制的后台程序"
		c.bg.Tone = ToneNeutral
		c.changed()
		c.mu.Unlock()
		return
	}
	procs, err := sys.Snapshot()
	if err != nil {
		return
	}
	mask := lowMask()
	appliedNow := 0
	for _, n := range names {
		for _, p := range procs {
			if !strings.EqualFold(p.Name, n) || strings.ToLower(p.Name) == name {
				continue
			}
			c.mu.Lock()
			skip := false
			for _, a := range c.bgApplied {
				if a.pid == p.PID {
					skip = true
				}
			}
			c.mu.Unlock()
			if skip {
				continue
			}
			creation, err := sys.CreationTime(p.PID)
			if err != nil {
				continue
			}
			class, err := sys.PriorityClass(p.PID)
			if err != nil {
				continue
			}
			oldMask, _, err := sys.Affinity(p.PID)
			if err != nil {
				oldMask = mask
			}
			if err := sys.SetPriorityClass(p.PID, sys.IDLE_PRIORITY_CLASS); err != nil {
				c.logf("后台抑制 %s 失败：%v", p.Name, err)
				continue
			}
			_ = sys.SetAffinity(p.PID, mask)
			c.mu.Lock()
			c.bgApplied = append(c.bgApplied, bgApplied{pid: p.PID, name: p.Name, creation: creation, class: class, mask: oldMask})
			c.mu.Unlock()
			appliedNow++
		}
	}
	c.mu.Lock()
	c.bg.Applied = len(c.bgApplied)
	if len(c.bgApplied) > 0 {
		c.bg.Text = fmt.Sprintf("已抑制 %d 个进程", len(c.bgApplied))
		c.bg.Detail = "Idle 优先级 · 低亲和性 " + sys.MaskLabel(mask) + "；游戏退出后恢复"
		c.bg.Tone = ToneGood
	} else {
		c.bg.Text = "没有可抑制的已选进程"
		c.bg.Detail = "请选择正在运行的后台程序"
		c.bg.Tone = ToneWarn
	}
	c.changed()
	c.mu.Unlock()
	if appliedNow > 0 {
		c.logf("后台抑制：新应用 %d 个进程（共 %d）", appliedNow, len(c.bgApplied))
	}
}

func (c *Control) restoreBackground() {
	c.mu.Lock()
	list := c.bgApplied
	c.bgApplied = nil
	c.mu.Unlock()
	if len(list) == 0 {
		return
	}
	restored := 0
	for _, a := range list {
		creation, err := sys.CreationTime(a.pid)
		if err != nil || creation != a.creation {
			continue
		}
		if err := sys.SetPriorityClass(a.pid, a.class); err != nil {
			continue
		}
		_ = sys.SetAffinity(a.pid, a.mask)
		restored++
	}
	c.logf("后台抑制恢复：%d/%d 个进程", restored, len(list))
	c.mu.Lock()
	c.bg.Applied = 0
	if c.cfg.BGEnabled {
		c.bg.Text = "已恢复"
		c.bg.Detail = "优先级与亲和性已还原"
		c.bg.Tone = ToneNeutral
	}
	c.changed()
	c.mu.Unlock()
}

func (c *Control) RestoreBackgroundNow() { c.restoreBackground() }

// ---- system scheduling -------------------------------------------------

func (c *Control) applySchedule() {
	c.mu.Lock()
	if !c.cfg.ScheduleEnabled || !c.gameRunning {
		c.mu.Unlock()
		return
	}
	pid := c.gamePID
	cur := c.schedApp
	c.mu.Unlock()
	if cur.active && cur.pid == pid {
		return
	}
	if cur.active {
		c.restoreSchedApp(cur)
	}
	creation, err := sys.CreationTime(pid)
	if err != nil {
		c.setSchedBad("读取游戏进程失败：" + err.Error())
		return
	}
	class, err := sys.PriorityClass(pid)
	if err != nil {
		c.setSchedBad("读取优先级失败：" + err.Error())
		return
	}
	oldMask, _, err := sys.Affinity(pid)
	if err != nil {
		c.setSchedBad("读取亲和性失败：" + err.Error())
		return
	}
	if err := sys.SetPriorityClass(pid, sys.HIGH_PRIORITY_CLASS); err != nil {
		c.setSchedBad("设置 High 优先级失败：" + err.Error())
		return
	}
	mask := sys.GameMask()
	if err := sys.SetAffinity(pid, mask); err != nil {
		_ = sys.SetPriorityClass(pid, class)
		c.setSchedBad("设置亲和性失败：" + err.Error())
		return
	}
	c.mu.Lock()
	c.schedApp = schedApplied{pid: pid, creation: creation, class: class, mask: oldMask, active: true}
	c.sched = Status{Text: "已应用 · High · 排除 CPU 0", Detail: "亲和性 " + sys.MaskLabel(mask) + "；游戏退出自动回滚", Tone: ToneGood}
	c.changed()
	c.mu.Unlock()
	c.logf("系统调度：CODM pid %d → High，亲和性 %s", pid, sys.MaskLabel(mask))
}

func (c *Control) setSchedBad(msg string) {
	c.mu.Lock()
	c.sched = Status{Text: "调度未生效", Detail: msg, Tone: ToneBad}
	c.changed()
	c.mu.Unlock()
	c.logf("系统调度失败：%s", msg)
}

func (c *Control) restoreSchedApp(a schedApplied) {
	creation, err := sys.CreationTime(a.pid)
	if err == nil && creation == a.creation {
		_ = sys.SetPriorityClass(a.pid, a.class)
		_ = sys.SetAffinity(a.pid, a.mask)
		c.logf("系统调度回滚：pid %d", a.pid)
	}
}

func (c *Control) releaseSchedule() {
	c.mu.Lock()
	a := c.schedApp
	c.schedApp = schedApplied{}
	if c.cfg.ScheduleEnabled {
		c.sched = Status{Text: "等待游戏启动", Detail: "游戏运行时将 CODM 设为 High 并排除 CPU 0，退出自动回滚", Tone: ToneNeutral}
	}
	c.changed()
	c.mu.Unlock()
	if a.active {
		c.restoreSchedApp(a)
	}
}

// ---- power plan --------------------------------------------------------

func (c *Control) applyPower() {
	prior, err := sys.ActiveScheme()
	if err != nil {
		c.mu.Lock()
		c.power = PowerState{Note: "无法读取当前电源方案：" + err.Error()}
		c.changed()
		c.mu.Unlock()
		c.logf("电源：读取失败 %v", err)
		return
	}
	ult, ok := sys.FindUltimate()
	if ok && strings.EqualFold(prior.GUID, ult.GUID) {
		c.mu.Lock()
		c.power = PowerState{Name: prior.Name, GUID: prior.GUID, PriorGUID: prior.GUID, PriorName: prior.Name, Injected: false,
			Note: "系统已在使用卓越性能，无需注入；已记录当前 GUID 供还原"}
		c.cfg.PowerPrior = prior.GUID
		c.cfg.PowerEnabled = true
		c.changed()
		c.mu.Unlock()
		c.saveCfg()
		c.logf("电源：卓越性能已在用（%s），未改动", prior.GUID)
		return
	}
	scheme, created, err := sys.InjectUltimate()
	if err != nil {
		c.mu.Lock()
		c.power = PowerState{Name: prior.Name, GUID: prior.GUID, PriorGUID: prior.GUID, Note: "注入失败：" + err.Error()}
		c.changed()
		c.mu.Unlock()
		c.logf("电源：注入失败 %v", err)
		return
	}
	// Switching plans resets screen-off/sleep to the new plan's defaults.
	// Capture the user's current idle policy and put it back on the new plan.
	idle := sys.CurrentTimeouts()
	kept := true
	if err := sys.ApplyTimeouts(idle); err != nil {
		kept = false
		c.logf("电源：熄屏/休眠时间保留失败 %v", err)
	}
	note := "已注入并启用；功耗与发热上升，frametime 更稳"
	if kept {
		note += "；已保留熄屏/休眠设置"
	} else {
		note += "；熄屏/休眠设置未能保留"
	}
	c.mu.Lock()
	c.power = PowerState{Name: scheme.Name, GUID: scheme.GUID, PriorGUID: prior.GUID, PriorName: prior.Name, Injected: true,
		Note: note}
	c.cfg.PowerPrior = prior.GUID
	c.cfg.PowerInjected = scheme.GUID
	c.cfg.PowerEnabled = true
	c.changed()
	c.mu.Unlock()
	c.saveCfg()
	verb := "复用已安装方案"
	if created {
		verb = "已解锁并注入新方案"
	}
	c.logf("电源：%s %s（原方案 %s）", verb, scheme.GUID, prior.GUID)
}

func (c *Control) RestorePower() { c.restorePower(true) }

func (c *Control) restorePower(explicit bool) {
	c.mu.Lock()
	prior := c.cfg.PowerPrior
	c.mu.Unlock()
	if prior == "" {
		c.mu.Lock()
		c.power.Note = "没有记录到注入前的方案 GUID，无法还原"
		c.cfg.PowerEnabled = false
		c.changed()
		c.mu.Unlock()
		c.saveCfg()
		return
	}
	if err := sys.SetActive(prior); err != nil {
		c.mu.Lock()
		c.power.Note = "还原失败：" + err.Error()
		c.changed()
		c.mu.Unlock()
		c.logf("电源还原失败：%v", err)
		return
	}
	name := ""
	if list, err := sys.ListSchemes(); err == nil {
		for _, s := range list {
			if strings.EqualFold(s.GUID, prior) {
				name = s.Name
			}
		}
	}
	c.mu.Lock()
	c.power = PowerState{Name: name, GUID: prior, PriorGUID: prior, Note: "已还原注入前的电源方案"}
	c.cfg.PowerEnabled = false
	c.changed()
	c.mu.Unlock()
	c.saveCfg()
	c.logf("电源：已还原 %s (%s)", prior, name)
}

// ---- registry tweaks ---------------------------------------------------

func (c *Control) refreshRegistryState() {
	k, err := registry.OpenKey(registry.CURRENT_USER, regKeyPath, registry.QUERY_VALUE)
	if err != nil {
		c.mu.Lock()
		c.reg.KeyExists = false
		c.reg.Text = "未检测到设置键"
		c.reg.Detail = `HKCU\SOFTWARE\Tencent\Call-of-Duty 不存在；请先正常进入一次游戏`
		c.changed()
		c.mu.Unlock()
		return
	}
	defer k.Close()
	names, _ := k.ReadValueNames(-1)
	_, fovKeys := splitFOV(names)
	fpsValue, fpsEnable := findFPSKeys(names)
	fpsKey := fpsValue
	if fpsKey == "" {
		fpsKey = fpsEnable
	}
	value := uint32(0)
	if fpsValue != "" {
		if v, _, err := k.GetIntegerValue(fpsValue); err == nil {
			value = uint32(v)
		}
	}
	c.mu.Lock()
	c.reg.KeyExists = true
	c.reg.FPSKey = fpsKey
	c.reg.FPSValue = value
	c.reg.FOVKeys = fovKeys
	c.reg.FOV = c.cfg.FOV
	c.reg.Backup = c.cfg.RegistryBackup
	c.reg.Text = fpsStatusText(fpsKey, len(fovKeys))
	if fpsKey == "" {
		c.reg.Detail = "帧率微调键缺失：请先在游戏内打开“帧数微调”并拖动一次；FOV 键可直接调"
	} else {
		c.reg.Detail = "写入前自动 reg export 备份；游戏运行中只读；FOV 恢复请在游戏内拖动滑块"
	}
	c.changed()
	c.mu.Unlock()
}

func fpsStatusText(fpsKey string, fovCount int) string {
	if fpsKey == "" {
		return fmt.Sprintf("FOV 键 %d 个 · 帧率键缺失", fovCount)
	}
	return fmt.Sprintf("FOV 键 %d 个 · 帧率键已发现", fovCount)
}

func classifyRegValues(names []string) (fpsKey string, fovKeys []string) {
	for _, n := range names {
		lower := strings.ToLower(n)
		if strings.Contains(lower, "frameratecustomizevalue") {
			fpsKey = n
		}
		if strings.Contains(lower, "maincamerafovingame") {
			fovKeys = append(fovKeys, n)
		}
	}
	sort.Strings(fovKeys)
	return
}

func (c *Control) exportRegistry(path string) error {
	cmd := exec.Command("reg", "export", `HKCU\`+regKeyPath, path, "/y")
	sys.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v %s", err, strings.TrimSpace(sys.DecodeConsole(out)))
	}
	return nil
}

// gameAlive re-checks the process table right before a write, so a game that
// started since the last monitor tick still blocks the registry module.
func (c *Control) gameAlive() bool {
	c.mu.Lock()
	name := filepath.Base(c.gameExe)
	dir := strings.ToLower(c.installDir)
	c.mu.Unlock()
	if name == "" {
		return c.gameRunning
	}
	procs, err := sys.Snapshot()
	if err != nil {
		return c.gameRunning
	}
	for _, p := range procs {
		if !strings.EqualFold(p.Name, name) {
			continue
		}
		if dir == "" {
			return true
		}
		if path, err := sys.ImagePath(p.PID); err == nil && strings.HasPrefix(strings.ToLower(path), dir) {
			return true
		}
	}
	return false
}

// ApplyRegistry writes the FOV and (when selected) the unlimited frame rate.
// It refuses while the game runs.
func (c *Control) ApplyRegistry() {
	// Keep the old entry point safe: the v3 DWORD writer is gone, and the
	// proven weapon-byte unlock lives in ApplyFOV.
	c.ApplyFOV()
}

func (c *Control) RestoreRegistry() {
	c.logf("注册表：收到还原请求")
	c.mu.Lock()
	running := c.gameRunning
	backup := c.cfg.RegistryBackup
	c.mu.Unlock()
	if running || c.gameAlive() {
		c.setRegBad("游戏运行中：拒绝还原。请先退出游戏")
		return
	}
	if backup == "" || !fileExists(backup) {
		c.setRegBad("没有可用的注册表备份")
		return
	}
	cmd := exec.Command("reg", "import", backup)
	sys.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		c.setRegBad("还原失败：" + strings.TrimSpace(sys.DecodeConsole(out)))
		return
	}
	c.mu.Lock()
	c.reg.Applied = false
	c.reg.Text = "已从备份还原"
	c.reg.Detail = backup
	c.cfg.RegistryBackup = ""
	c.reg.Backup = ""
	c.changed()
	c.mu.Unlock()
	c.saveCfg()
	c.logf("注册表：已从备份还原 %s", backup)
	c.refreshRegistryState()
}

// ---- TdrDelay ----------------------------------------------------------

func (c *Control) SetTdr(on bool) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\GraphicsDrivers`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		c.mu.Lock()
		c.gpu.Note = "TdrDelay 写入失败（需要管理员）：" + err.Error()
		c.changed()
		c.mu.Unlock()
		return
	}
	defer k.Close()
	if on {
		had := false
		var old uint64
		if v, _, err := k.GetIntegerValue("TdrDelay"); err == nil {
			had, old = true, v
		}
		if err := k.SetDWordValue("TdrDelay", 8); err != nil {
			c.mu.Lock()
			c.gpu.Note = "TdrDelay 写入失败：" + err.Error()
			c.changed()
			c.mu.Unlock()
			return
		}
		c.mu.Lock()
		c.cfg.TdrHad, c.cfg.TdrValue = had, uint32(old)
		c.cfg.TdrDelay = true
		c.gpu.TdrOn = true
		c.gpu.Note = "TdrDelay=8 秒已写入；这是缓冲，不是修复"
		c.changed()
		c.mu.Unlock()
		c.logf("TdrDelay：写入 8（原值存在=%v 值=%d）", had, old)
	} else {
		c.mu.Lock()
		had, old := c.cfg.TdrHad, c.cfg.TdrValue
		c.mu.Unlock()
		if had {
			_ = k.SetDWordValue("TdrDelay", old)
		} else {
			_ = k.DeleteValue("TdrDelay")
		}
		c.mu.Lock()
		c.cfg.TdrDelay = false
		c.gpu.TdrOn = false
		c.gpu.Note = "TdrDelay 已还原"
		c.changed()
		c.mu.Unlock()
		c.logf("TdrDelay：已还原")
	}
	c.saveCfg()
	go c.refreshGPU()
}

// ---- GPU / driver panel -------------------------------------------------

// RefreshGPU reloads adapters, driver versions, reset events and runtimes.
func (c *Control) RefreshGPU() { go c.refreshGPU() }

func (c *Control) refreshGPU() {
	c.mu.Lock()
	c.gpu.Busy = true
	c.changed()
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.gpu.Busy = false
		c.changed()
		c.mu.Unlock()
	}()
	adapters, err := sys.Adapters()
	if err != nil {
		c.logf("DXGI 枚举失败：%v", err)
	}
	drivers := driverVersions()
	rows := make([]GPUAdapter, 0, len(adapters))
	seenName := map[string]bool{}
	for _, a := range adapters {
		if a.Software {
			continue
		}
		key := fmt.Sprintf("%s|%d|%d", a.Name, a.VendorID, a.DeviceID)
		if seenName[key] {
			continue
		}
		seenName[key] = true
		rows = append(rows, GPUAdapter{
			Name:   a.Name,
			Vendor: sys.VendorName(a.VendorID),
			Driver: matchDriver(drivers, a.Name),
			VRAMGB: float64(a.DedicatedVRAM) / (1024 * 1024 * 1024),
		})
	}
	actual := ""
	note := "启动游戏后可标注 CODM 实际使用的 GPU"
	c.mu.Lock()
	running, pid := c.gameRunning, c.gamePID
	c.mu.Unlock()
	if running && pid != 0 && err == nil {
		if engines, e := sys.GPUUsage(); e == nil {
			usage := map[[2]uint32]float64{}
			for _, en := range engines {
				if en.PID != pid {
					continue
				}
				usage[[2]uint32{en.LUIDHigh, en.LUIDLow}] += en.Usage
			}
			best := 0.0
			var bestL [2]uint32
			for l, u := range usage {
				if u > best {
					best, bestL = u, l
				}
			}
			for i, a := range adapters {
				if uint32(a.LUIDHigh) == bestL[0] && a.LUIDLow == bestL[1] {
					actual = adapters[i].Name
					for j := range rows {
						if rows[j].Name == adapters[i].Name {
							rows[j].Used = true
						}
					}
				}
			}
			if actual == "" && len(usage) > 0 {
				note = "已读取 GPU 引擎计数，但未匹配到适配器 LUID"
			} else if actual != "" {
				note = fmt.Sprintf("CODM 实际使用：%s（GPU 引擎利用率 %.1f%%）", actual, best)
			}
		} else {
			note = "无法读取每进程 GPU 引擎计数（PDH 不可用），不标注实际使用"
		}
	}
	events := readGPUResets()
	deps := checkRuntimeDeps()
	c.mu.Lock()
	c.gpu = GPUState{Adapters: rows, Actual: actual, Note: note, Events: events, Deps: deps, Checked: true, TdrOn: c.cfg.TdrDelay}
	c.runtime = deps
	c.changed()
	c.mu.Unlock()
	c.logf("驱动面板：%d 个适配器，实际使用=%q", len(rows), actual)
}

func driverVersions() map[string]string {
	out := map[string]string{}
	base := `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}`
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, base, registry.READ|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return out
	}
	names, _ := k.ReadSubKeyNames(-1)
	k.Close()
	for _, n := range names {
		if len(n) != 4 {
			continue
		}
		sub, err := registry.OpenKey(registry.LOCAL_MACHINE, base+`\`+n, registry.READ)
		if err != nil {
			continue
		}
		desc, _, _ := sub.GetStringValue("DriverDesc")
		ver, _, _ := sub.GetStringValue("DriverVersion")
		sub.Close()
		if desc != "" && ver != "" {
			out[strings.ToLower(desc)] = ver
		}
	}
	return out
}

func matchDriver(drivers map[string]string, name string) string {
	lower := strings.ToLower(name)
	for desc, ver := range drivers {
		if strings.Contains(lower, desc) || strings.Contains(desc, lower) {
			return ver
		}
	}
	return ""
}

var gpuModules = []struct {
	key   string
	advice string
}{
	{"nvlddmkm.sys", "NVIDIA 驱动超时恢复：更新驱动、关闭超频、检查供电与散热"},
	{"atikmpag.sys", "AMD 驱动超时恢复：清洁安装驱动、关闭超频"},
	{"amdwddmg.sys", "AMD 驱动超时恢复：清洁安装驱动、关闭超频"},
	{"amdkmdag.sys", "AMD 驱动超时恢复：清洁安装驱动、关闭超频"},
	{"igdkmd64.sys", "Intel 核显驱动超时：更新核显驱动，或为 CODM 指定独显"},
}

func readGPUResets() []GPUEvent {
	cmd := exec.Command("wevtutil", "qe", "System", `/q:*[System[(EventID=4101)]]`, "/c:6", "/f:text", "/rd:true")
	sys.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil
	}
	text := sys.DecodeConsole(out)
	blocks := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n")
	var events []GPUEvent
	for _, b := range blocks {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		when := strings.SplitN(b, "\n", 2)[0]
		if len(when) > 40 {
			when = when[:40]
		}
		lower := strings.ToLower(b)
		found := ""
		for _, m := range gpuModules {
			if strings.Contains(lower, m.key) {
				found = m.key
				events = append(events, GPUEvent{When: strings.TrimSpace(when), Module: m.key, Advice: m.advice})
				break
			}
		}
		if found == "" {
			events = append(events, GPUEvent{When: strings.TrimSpace(when), Module: "未知模块", Advice: "显示驱动超时恢复（4101）：更新显卡驱动并检查散热/供电"})
		}
		if len(events) >= 6 {
			break
		}
	}
	return events
}

func checkRuntimeDeps() []Dep {
	sysdir := filepath.Join(os.Getenv("SystemRoot"), "System32")
	files := []string{"vcruntime140.dll", "vcruntime140_1.dll", "msvcp140.dll", "d3d11.dll", "dxgi.dll", "d3dcompiler_47.dll"}
	out := make([]Dep, 0, len(files))
	for _, f := range files {
		p := filepath.Join(sysdir, f)
		out = append(out, Dep{Name: f, OK: fileExists(p), Path: p})
	}
	return out
}

// ---- network diagnostics ----------------------------------------------

// RefreshNet runs the read-only DNS/TCP probes off the UI goroutine.
func (c *Control) RefreshNet() { go c.refreshNet() }

func (c *Control) refreshNet() {
	c.mu.Lock()
	host := c.cfg.DNSTarget
	pid := c.gamePID
	running := c.gameRunning
	c.net.Busy = true
	c.changed()
	c.mu.Unlock()

	d, addrs, err := sys.DNSLookup(host, 2*time.Second)
	state := NetState{Host: host, Note: "只读探测：不修改 hosts/DNS，不安装虚拟网卡，不集成加速器"}
	state.DNSTime = d.Round(time.Millisecond).String()
	if err != nil {
		state.DNSErr = err.Error()
	} else {
		state.DNSAddrs = strings.Join(addrs, ", ")
	}
	if running && pid != 0 {
		eps, e := sys.TCPEndpoints(pid)
		if e == nil {
			seen := map[string]bool{}
			for _, ep := range eps {
				if ep.RemoteIP == "0.0.0.0" || seen[ep.RemoteIP] {
					continue
				}
				seen[ep.RemoteIP] = true
				row := NetEndpoint{Remote: fmt.Sprintf("%s:%d", ep.RemoteIP, ep.RemotePort)}
				if len(state.Endpoints) < 5 {
					if rtt, err := sys.TCPRTT(row.Remote, 700*time.Millisecond); err == nil {
						row.RTT = rtt.Round(time.Millisecond).String()
					} else {
						row.Err = "TCP 不可达（可能仅 UDP）"
					}
				}
				state.Endpoints = append(state.Endpoints, row)
			}
		}
	}
	c.mu.Lock()
	c.net = state
	c.changed()
	c.mu.Unlock()
}
