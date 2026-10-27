package tune

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"codmtune/internal/sys"
)

// Tone is how a status reads.
type Tone int

const (
	ToneNeutral Tone = iota
	ToneGood
	ToneWarn
	ToneBad
)

// Status is one module's visible state.
type Status struct {
	Text   string
	Detail string
	Tone   Tone
	Busy   bool
}

// Candidate is a detected install plus its launcher.
type Candidate struct {
	GameExe    string `json:"game_exe"`
	InstallDir string `json:"install_dir"`
	Launcher   string `json:"launcher"`
}

// PowerState describes the active plan and what the tool did.
type PowerState struct {
	Name      string
	GUID      string
	PriorGUID string
	PriorName string
	Injected  bool
	Note      string
}

// BgProc is one background process candidate row.
type BgProc struct {
	Name     string
	PIDs     int
	CPU      float64
	MemMB    float64
	Selected bool
	Applied  bool
}

// BgState is the background suppression panel state.
type BgState struct {
	Enabled bool
	Applied int
	Text    string
	Detail  string
	Tone    Tone
	Procs   []BgProc
}

// RegState is the external registry tweak panel.
type RegState struct {
	KeyExists bool
	FPSKey    string
	FPSValue  uint32
	FOVKeys   []string
	FOV       int
	Text      string
	Detail    string
	Backup    string
	Applied   bool
}

// GPUAdapter is one adapter row.
type GPUAdapter struct {
	Name   string
	Vendor string
	Driver string
	VRAMGB float64
	Used   bool
}

// GPUEvent is a parsed Event ID 4101.
type GPUEvent struct {
	When   string
	Module string
	Advice string
}

// Dep is a runtime dependency check row.
type Dep struct {
	Name string
	OK   bool
	Path string
}

// GPUState is the driver panel.
type GPUState struct {
	Adapters []GPUAdapter
	Actual   string
	Note     string
	Events   []GPUEvent
	TdrOn    bool
	Deps     []Dep
	Checked  bool
	Busy     bool
}

// NetEndpoint is one TCP peer with an optional handshake time.
type NetEndpoint struct {
	Remote string
	RTT    string
	Err    string
}

// NetState is the read-only network panel.
type NetState struct {
	Host      string
	DNSTime   string
	DNSAddrs  string
	DNSErr    string
	Endpoints []NetEndpoint
	Note      string
	Busy      bool
}

// ToolStat is the tuner's own measured footprint.
type ToolStat struct {
	CPU    float64
	MemMB  float64
	ACECnt int
}

type bgApplied struct {
	pid      uint32
	name     string
	creation uint64
	class    uint32
	mask     uint64
}

type schedApplied struct {
	pid      uint32
	creation uint64
	class    uint32
	mask     uint64
	active   bool
}

// Control is the headless tuner. The UI reads it and calls its actions; all
// long work happens on goroutines and posts back through Notify.
type Control struct {
	mu     sync.Mutex
	cfg    Config
	notify func()
	stop   chan struct{}

	detecting  bool
	scanPath   string
	candidates []Candidate
	gameExe    string
	launcher   string
	installDir string
	launchErr  string
	elevated   bool

	gameRunning bool
	gamePID     uint32

	power   PowerState
	ace     Status
	bg      BgState
	reg     RegState
	sched   Status
	gpu     GPUState
	net     NetState
	runtime []Dep
	tool    ToolStat
	logs    []string
	version int

	bgApplied []bgApplied
	schedApp  schedApplied

	procCPU     map[uint32]procSample
	procSeen    map[string]bool
	lastACEAt   time.Time
	aceFail     string
	aceWaiting  bool
	timerActive bool
	overlay     OverlayStats
	replays     []Replay
}

type procSample struct {
	cpu uint64
	at  time.Time
	ws  uint64
}

var selfSample struct {
	sync.Mutex
	cpu uint64
	at  time.Time
}

// NewControl loads configuration and prepares the module state.
func NewControl() *Control {
	c := &Control{cfg: loadConfig(), stop: make(chan struct{}), version: 1}
	c.gameExe = c.cfg.GameExe
	c.launcher = c.cfg.Launcher
	c.installDir = c.cfg.InstallDir
	c.elevated = sys.IsElevated()
	c.ace = Status{Text: "未启用", Detail: "默认关闭；启用后仅降低 ACE 相关进程的 Idle 优先级与单核亲和性，不改游戏进程", Tone: ToneNeutral}
	c.bg = BgState{Text: "未启用", Detail: "游戏运行时临时降低指定后台进程的调度权重，游戏退出自动恢复"}
	c.sched = Status{Text: "等待游戏启动", Detail: "游戏运行时将 CODM 设为 High 并排除 CPU 0, 退出自动恢复", Tone: ToneNeutral}
	c.reg = RegState{FOV: c.cfg.FOV}
	c.procCPU = map[uint32]procSample{}
	c.net.Note = "只探测：不修改 hosts/DNS，不安装虚拟网卡，不集成加速器"
	c.updateSelfStat(nil)
	c.aceWaiting = false
	if s, err := sys.ActiveScheme(); err == nil {
		c.power = PowerState{Name: s.Name, GUID: s.GUID, Note: "当前电源方案（只读）"}
	}
	c.logf("CODMTune 启动 · 管理员=%v · 逻辑核心=%d", c.elevated, sys.LogicalCPUs())
	return c
}

// SetNotify registers the UI invalidation callback.
func (c *Control) SetNotify(fn func()) { c.mu.Lock(); c.notify = fn; c.mu.Unlock() }

func (c *Control) changed() {
	c.version++
	if c.notify != nil {
		c.notify()
	}
}

// saveCfg snapshots the config under the lock so concurrent module updates
// cannot write a torn struct.
func (c *Control) saveCfg() {
	c.mu.Lock()
	cfg := c.cfg
	c.mu.Unlock()
	_ = saveConfig(cfg)
}

// Version grows whenever visible state changed; screens rebuild on change.
func (c *Control) Version() int { c.mu.Lock(); defer c.mu.Unlock(); return c.version }

// Cfg returns a copy of the persisted configuration.
func (c *Control) Cfg() Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

// Update mutates the config under the lock, saves it and repaints. Callers use
// it for UI-only settings; side-effecting modules have their own methods.
func (c *Control) Update(fn func(*Config)) {
	c.mu.Lock()
	fn(&c.cfg)
	cfg := c.cfg
	c.changed()
	c.mu.Unlock()
	_ = saveConfig(cfg)
}

// SoundKindOn reports one of the four cue gates: startup, move, confirm,
// launch. A mask of the wrong length means every cue is on.
func SoundKindOn(mask string, i int) bool {
	if i < 0 || i > 3 {
		return false
	}
	if len(mask) != 4 {
		return true
	}
	return mask[i] == '1'
}

// ToggleSoundKind flips one cue gate and persists it.
func (c *Control) ToggleSoundKind(i int) {
	c.Update(func(cfg *Config) {
		if len(cfg.SoundKinds) != 4 {
			cfg.SoundKinds = "1111"
		}
		b := []byte(cfg.SoundKinds)
		if i >= 0 && i < 4 {
			if b[i] == '1' {
				b[i] = '0'
			} else {
				b[i] = '1'
			}
		}
		cfg.SoundKinds = string(b)
	})
}

func (c *Control) logf(format string, args ...any) {
	line := time.Now().Format("15:04:05") + "  " + fmt.Sprintf(format, args...)
	c.mu.Lock()
	c.logs = append(c.logs, line)
	if len(c.logs) > 240 {
		c.logs = c.logs[len(c.logs)-240:]
	}
	c.mu.Unlock()
	f, err := os.OpenFile(filepath.Join(configDir(), "codmtune.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		fmt.Fprintln(f, line)
		f.Close()
	}
}

// Logs returns a copy of the in-memory log.
func (c *Control) Logs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.logs))
	copy(out, c.logs)
	return out
}

// LogLine appends one diagnostic line, used for startup privilege state.
func (c *Control) LogLine(s string) { c.logf("%s", s) }

// Start applies the default-on modules and begins monitoring.
func (c *Control) Start() {
	go c.detectJob()
	go func() { _ = sys.SetSelfBelowNormal() }()
	go c.monitor()
	go c.bgLoop()
	go c.overlayLoop()
	go c.memLoop()
	go c.startupPower()
}

// bgLoop reapplies suppression to newly started instances of the selected
// process names every 30 seconds while the game is running.
func (c *Control) bgLoop() {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-tick.C:
			c.mu.Lock()
			on := c.cfg.BGEnabled
			running := c.gameRunning
			c.mu.Unlock()
			if on && running {
				c.applyBackground()
			}
		}
	}
}

func (c *Control) Close() {
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
	c.releaseSchedule()
	c.restoreBackground()
}

func (c *Control) startupPower() {
	if !c.cfg.PowerEnabled {
		c.mu.Lock()
		c.power.Note = "未启用：开机后注入卓越性能"
		c.changed()
		c.mu.Unlock()
		return
	}
	c.applyPower()
}

// Game returns the selected install and run state.
func (c *Control) Game() (exe, install, launcher string, running bool, pid uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gameExe, c.installDir, c.launcher, c.gameRunning, c.gamePID
}

// Candidates lists detected installs.
func (c *Control) Candidates() ([]Candidate, bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Candidate, len(c.candidates))
	copy(out, c.candidates)
	return out, c.detecting, c.scanPath
}

// SelectCandidate persists one install and selects it.
func (c *Control) SelectCandidate(cand Candidate) {
	c.mu.Lock()
	c.gameExe, c.installDir, c.launcher = cand.GameExe, cand.InstallDir, cand.Launcher
	c.cfg.GameExe, c.cfg.InstallDir, c.cfg.Launcher = cand.GameExe, cand.InstallDir, cand.Launcher
	c.launchErr = ""
	c.changed()
	c.mu.Unlock()
	c.saveCfg()
	c.logf("选择安装：%s", cand.GameExe)
	c.refreshRegistryState()
}

// Detect rescans all fixed drives.
func (c *Control) Detect() {
	c.mu.Lock()
	if c.detecting {
		c.mu.Unlock()
		return
	}
	c.detecting = true
	c.scanPath = ""
	c.changed()
	c.mu.Unlock()
	go c.detectJob()
}

func (c *Control) detectJob() {
	var found []Candidate
	if c.cfg.GameExe != "" && fileExists(c.cfg.GameExe) {
		found = append(found, Candidate{GameExe: c.cfg.GameExe, InstallDir: filepath.Dir(c.cfg.GameExe), Launcher: c.cfg.Launcher})
	}
	var scanned []Candidate
	scanned = scanGames(func(p string) {
		c.mu.Lock()
		c.scanPath = p
		c.mu.Unlock()
		c.changed()
	})
	for _, s := range scanned {
		dup := false
		for _, f := range found {
			if strings.EqualFold(f.GameExe, s.GameExe) {
				dup = true
			}
		}
		if !dup {
			found = append(found, s)
		}
	}
	if len(found) == 0 {
		if running, _ := c.runningGamePath(); running != "" {
			found = append(found, Candidate{GameExe: running, InstallDir: filepath.Dir(running), Launcher: findLauncher(filepath.Dir(running))})
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		return strings.HasPrefix(strings.ToUpper(found[i].GameExe), "D:")
	})
	c.mu.Lock()
	c.candidates = found
	c.detecting = false
	c.scanPath = ""
	if len(found) == 1 {
		c.gameExe, c.installDir, c.launcher = found[0].GameExe, found[0].InstallDir, found[0].Launcher
		c.cfg.GameExe, c.cfg.InstallDir, c.cfg.Launcher = found[0].GameExe, found[0].InstallDir, found[0].Launcher
	}
	c.changed()
	c.mu.Unlock()
	if len(found) == 1 {
		c.saveCfg()
		c.logf("自动探测到 CODM：%s", found[0].GameExe)
	} else if len(found) > 1 {
		c.logf("探测到 %d 个安装，需要选择", len(found))
	} else {
		c.logf("未探测到 CODM，请手动选择 D 盘安装目录")
	}
	c.refreshRegistryState()
	if c.gpu.Checked {
		go c.refreshGPU()
	}
}

// runningGamePath reads the image path of a running CODM from a snapshot.
func (c *Control) runningGamePath() (string, uint32) {
	procs, err := sys.Snapshot()
	if err != nil {
		return "", 0
	}
	gameName := strings.ToLower(filepath.Base(c.gameExe))
	for _, p := range procs {
		if p.PID == uint32(os.Getpid()) || strings.Contains(strings.ToLower(p.Name), "codmtune") {
			continue
		}
		n := strings.ToLower(p.Name)
		if gameName != "" && n != gameName {
			continue
		}
		if gameName == "" && !isGameExe(n) {
			continue
		}
		path, err := sys.ImagePath(p.PID)
		if err != nil {
			continue
		}
		if c.installDir == "" || strings.HasPrefix(strings.ToLower(path), strings.ToLower(c.installDir)) {
			return path, p.PID
		}
	}
	return "", 0
}

// Launch starts CODM directly, falling back to the launcher.
func (c *Control) Launch() {
	c.mu.Lock()
	exe := c.gameExe
	launcher := c.launcher
	c.launchErr = ""
	c.changed()
	c.mu.Unlock()
	if exe == "" {
		c.mu.Lock()
		c.launchErr = "尚未探测到 CODM，请先选择安装包"
		c.changed()
		c.mu.Unlock()
		return
	}
	target := exe
	if !fileExists(target) {
		target = launcher
	}
	if target == "" || !fileExists(target) {
		c.mu.Lock()
		c.launchErr = "找不到可执行文件：" + exe
		c.changed()
		c.mu.Unlock()
		c.logf("启动失败：找不到 %s", exe)
		return
	}
	c.mu.Lock()
	args := strings.Fields(c.cfg.LaunchArgs)
	c.mu.Unlock()
	cmd := exec.Command(target, args...)
	cmd.Dir = filepath.Dir(target)
	if err := cmd.Start(); err != nil {
		c.mu.Lock()
		c.launchErr = err.Error()
		c.changed()
		c.mu.Unlock()
		c.logf("启动失败：%v", err)
		return
	}
	c.logf("已启动 %s (pid %d)", target, cmd.Process.Pid)
}

// OpenFolder opens the game directory in Explorer.
func (c *Control) OpenFolder() {
	c.mu.Lock()
	dir := c.installDir
	c.mu.Unlock()
	if dir != "" {
		_ = exec.Command("explorer", dir).Start()
	}
}

// SelectDir picks a manually browsed directory by finding CODM.exe in it or
// in one immediate child directory.
func (c *Control) SelectDir(dir string) {
	exe := ""
	if p := filepath.Join(dir, "CODM.exe"); fileExists(p) {
		exe = p
	}
	if exe == "" {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				if p := filepath.Join(dir, e.Name(), "CODM.exe"); fileExists(p) {
					exe = p
					break
				}
			}
		}
	}
	if exe == "" {
		c.logf("手动选择目录中未找到 CODM.exe：%s", dir)
		return
	}
	c.SelectCandidate(Candidate{GameExe: exe, InstallDir: filepath.Dir(exe), Launcher: findLauncher(filepath.Dir(exe))})
}

// RefreshRegistry re-reads the PlayerPrefs keys on demand.
func (c *Control) RefreshRegistry() { go c.refreshRegistryState() }

func (c *Control) setStatus(target *Status, text, detail string, tone Tone) {
	target.Text, target.Detail, target.Tone = text, detail, tone
	c.changed()
}

// SetModule toggles one module by name.
func (c *Control) SetModule(name string, on bool) {
	switch name {
	case "ace":
		c.setACE(on)
	case "bg":
		c.setBG(on)
	case "power":
		if on {
			c.applyPower()
		} else {
			c.restorePower(false)
		}
	case "schedule":
		c.mu.Lock()
		c.cfg.ScheduleEnabled = on
		if on {
			c.sched.Text, c.sched.Detail, c.sched.Tone = "等待游戏启动", "游戏运行时将 CODM 设为 High 并排除 CPU 0，退出自动恢复", ToneNeutral
		} else {
			c.sched.Text, c.sched.Detail, c.sched.Tone = "未启用", "不修改游戏进程调度", ToneNeutral
		}
		c.changed()
		c.mu.Unlock()
		c.saveCfg()
		if !on {
			c.releaseSchedule()
		}
		c.logf("系统调度模块：%v", on)
	}
}

// ModuleOn reports the configured state.
func (c *Control) ModuleOn(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch name {
	case "ace":
		return c.cfg.ACEEnabled
	case "bg":
		return c.cfg.BGEnabled
	case "power":
		return c.cfg.PowerEnabled
	case "schedule":
		return c.cfg.ScheduleEnabled
	}
	return false
}

func (c *Control) ACE() Status { c.mu.Lock(); defer c.mu.Unlock(); return c.ace }
func (c *Control) BG() BgState {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.bg
	out.Procs = make([]BgProc, len(c.bg.Procs))
	copy(out.Procs, c.bg.Procs)
	return out
}
func (c *Control) Power() PowerState { c.mu.Lock(); defer c.mu.Unlock(); return c.power }
func (c *Control) Reg() RegState {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.reg
	out.FOVKeys = append([]string{}, c.reg.FOVKeys...)
	return out
}
func (c *Control) Sched() Status { c.mu.Lock(); defer c.mu.Unlock(); return c.sched }
func (c *Control) GPU() GPUState {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.gpu
	out.Adapters = append([]GPUAdapter{}, c.gpu.Adapters...)
	out.Events = append([]GPUEvent{}, c.gpu.Events...)
	out.Deps = append([]Dep{}, c.gpu.Deps...)
	return out
}
func (c *Control) Net() NetState {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.net
	out.Endpoints = append([]NetEndpoint{}, c.net.Endpoints...)
	return out
}
func (c *Control) Tool() ToolStat      { c.mu.Lock(); defer c.mu.Unlock(); return c.tool }
func (c *Control) Elevated() bool      { return c.elevated }
func (c *Control) LaunchError() string { c.mu.Lock(); defer c.mu.Unlock(); return c.launchErr }
func (c *Control) FOV() int            { c.mu.Lock(); defer c.mu.Unlock(); return c.cfg.FOV }

// SetFOV changes the pending FOV value without writing.
func (c *Control) SetFOV(v int) {
	c.mu.Lock()
	c.cfg.FOV = v
	c.reg.FOV = v
	c.changed()
	c.mu.Unlock()
	c.saveCfg()
}

// SetFPSUnlimited changes the pending unlimited-frame toggle.
func (c *Control) SetFPSUnlimited(on bool) {
	c.mu.Lock()
	c.cfg.FPSUnlimited = on
	c.changed()
	c.mu.Unlock()
	c.saveCfg()
}

func (c *Control) FPSUnlimited() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg.FPSUnlimited
}

// monitor polls once every two seconds with a single snapshot.
func (c *Control) monitor() {
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-tick.C:
			c.monitorTick()
		}
	}
}

func (c *Control) monitorTick() {
	procs, err := sys.Snapshot()
	if err != nil {
		return
	}
	c.updateSelfStat(procs)

	c.mu.Lock()
	gameName := strings.ToLower(filepath.Base(c.gameExe))
	running := false
	var pid uint32
	if gameName != "" {
		for _, p := range procs {
			if p.PID == uint32(os.Getpid()) || strings.Contains(strings.ToLower(p.Name), "codmtune") {
				continue
			}
			if strings.ToLower(p.Name) != gameName {
				continue
			}
			if c.installDir != "" {
				if path, err := sys.ImagePath(p.PID); err != nil || !strings.HasPrefix(strings.ToLower(path), strings.ToLower(c.installDir)+`\`) {
					continue
				}
			}
			pid = p.PID
			running = true
			break
		}
		// Some launchers rename the render process. Match the game family
		// instead of silently treating a running game as OFFLINE, but never
		// mistake this tool for the game (its name also contains "codm").
		if !running {
			for _, p := range procs {
				n := strings.ToLower(p.Name)
				if p.PID == uint32(os.Getpid()) || strings.Contains(n, "codmtune") || strings.Contains(n, "launcher") {
					continue
				}
				if !strings.Contains(n, "codm") {
					continue
				}
				if c.installDir != "" {
					if path, err := sys.ImagePath(p.PID); err != nil || !strings.HasPrefix(strings.ToLower(path), strings.ToLower(c.installDir)+`\`) {
						continue
					}
				}
				pid = p.PID
				running = true
				break
			}
		}
	}
	prev := c.gameRunning
	prevPID := c.gamePID
	c.gameRunning = running
	c.gamePID = pid
	if running && !prev {
		c.changed()
	} else if !running && prev {
		c.changed()
	}
	schedOn := c.cfg.ScheduleEnabled
	aceOn := c.cfg.ACEEnabled
	timerOn := c.cfg.TimerBoost
	c.mu.Unlock()

	if running && !prev {
		c.logf("检测到 CODM 运行 (pid %d)", pid)
		c.refreshRegistryState()
		if schedOn {
			c.applySchedule()
		}
		if timerOn {
			c.applyTimer()
		}
		if bgOn := c.ModuleOn("bg"); bgOn {
			c.applyBackground()
		}
	}
	if running && schedOn && prevPID != pid {
		c.applySchedule()
	}
	if !running && prev {
		c.logf("CODM 已退出，恢复后台进程")
		c.releaseSchedule()
		c.restoreBackground()
		c.releaseTimer()
		c.refreshRegistryState()
		if bgOn := c.ModuleOn("bg"); bgOn {
			c.mu.Lock()
			c.bg.Text, c.bg.Detail, c.bg.Tone = "待游戏启动", "游戏运行时应降低后台调度权重，退出后自动恢复", ToneNeutral
			c.bg.Applied = 0
			c.mu.Unlock()
			c.changed()
		}
	}
	if aceOn {
		if time.Since(c.lastACEAt) > 60*time.Second {
			go c.aceTick()
		}
	}
}

func (c *Control) updateSelfStat(procs []sys.Proc) {
	selfCPU, _ := sys.CPUTime(uint32(os.Getpid()))
	ws, _ := sys.WorkingSet(uint32(os.Getpid()))
	now := time.Now()
	selfSample.Lock()
	cpuPct := 0.0
	if !selfSample.at.IsZero() {
		deltaCPU := float64(selfCPU - selfSample.cpu)
		deltaT := float64(now.Sub(selfSample.at)) / float64(time.Second) * 1e7
		if deltaT > 0 {
			cpuPct = deltaCPU / deltaT / float64(sys.LogicalCPUs()) * 100
		}
	}
	selfSample.cpu, selfSample.at = selfCPU, now
	selfSample.Unlock()
	aceCnt := 0
	for _, p := range procs {
		n := strings.ToLower(p.Name)
		if n == "sguard64.exe" || n == "sguardsvc64.exe" || n == "ace-service64.exe" {
			aceCnt++
		}
	}
	memMB := float64(ws) / (1024 * 1024)
	c.mu.Lock()
	// The sampling window is ten seconds, so a repaint is only worth asking
	// for when a number the footer prints actually moved. Without this the
	// floating-point jitter of cpuPct invalidated the whole window every tick
	// for a pixel-identical picture.
	moved := math.Abs(c.tool.CPU-cpuPct) >= 0.1 ||
		math.Abs(c.tool.MemMB-memMB) >= 1 ||
		c.tool.ACECnt != aceCnt
	c.tool.CPU = cpuPct
	c.tool.MemMB = memMB
	c.tool.ACECnt = aceCnt
	c.mu.Unlock()
	if moved {
		c.changed()
	}
}
