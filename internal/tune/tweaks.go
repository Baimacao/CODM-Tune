package tune

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// TweakSet is one registry value a tweak changes.
type TweakSet struct {
	Key  string
	Name string
	Kind string // dword | sz
	On   uint32
	OnS  string
}

// TweakDef describes a reversible system tweak.
type TweakDef struct {
	ID     string
	Name   string
	Desc   string
	Sets   []TweakSet
	Reboot bool
}

func regRoot(key string) (registry.Key, string) {
	if strings.HasPrefix(strings.ToUpper(key), "HKLM") {
		return registry.LOCAL_MACHINE, strings.TrimPrefix(key[4:], `\`)
	}
	return registry.CURRENT_USER, strings.TrimPrefix(key[4:], `\`)
}

func regSnap(key, name string) RegSnap {
	s := RegSnap{Key: key, Name: name}
	root, path := regRoot(key)
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return s
	}
	defer k.Close()
	if v, _, err := k.GetIntegerValue(name); err == nil {
		s.Exists, s.Type, s.Dword = true, "dword", uint32(v)
		return s
	}
	if v, _, err := k.GetStringValue(name); err == nil {
		s.Exists, s.Type, s.Str = true, "sz", v
	}
	return s
}

func regApply(s TweakSet) {
	root, path := regRoot(s.Key)
	k, err := registry.OpenKey(root, path, registry.SET_VALUE)
	if err != nil {
		k, _, err = registry.CreateKey(root, path, registry.SET_VALUE)
		if err != nil {
			return
		}
	}
	defer k.Close()
	if s.Kind == "sz" {
		_ = k.SetStringValue(s.Name, s.OnS)
	} else {
		_ = k.SetDWordValue(s.Name, s.On)
	}
}

func regRestore(s RegSnap) {
	root, path := regRoot(s.Key)
	k, err := registry.OpenKey(root, path, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer k.Close()
	if !s.Exists {
		_ = k.DeleteValue(s.Name)
		return
	}
	if s.Type == "sz" {
		_ = k.SetStringValue(s.Name, s.Str)
	} else {
		_ = k.SetDWordValue(s.Name, s.Dword)
	}
}

// tweakDefs is the curated AtlasOS/RyTuneX-inspired set. Everything is a
// user-scope or HKLM value with a saved original; nothing is deleted.
func (c *Control) tweakDefs() map[string]TweakDef {
	c.mu.Lock()
	exe := c.gameExe
	c.mu.Unlock()
	defs := map[string]TweakDef{
		"gamemode": {
			ID: "gamemode", Name: "Windows 游戏模式",
			Desc: "HKCU\\Software\\Microsoft\\GameBar AutoGameModeEnabled=1；让系统把 CPU/GPU 优先给前台游戏。",
			Sets: []TweakSet{{Key: `HKCU\Software\Microsoft\GameBar`, Name: "AutoGameModeEnabled", Kind: "dword", On: 1}},
		},
		"gamedvr": {
			ID: "gamedvr", Name: "关闭后台录制",
			Desc: "关闭 GameDVR 的后台捕获；不删除任何录像文件，随时可恢复。",
			Sets: []TweakSet{
				{Key: `HKCU\System\GameConfigStore`, Name: "GameDVR_Enabled", Kind: "dword", On: 0},
				{Key: `HKCU\Software\Microsoft\Windows\CurrentVersion\GameDVR`, Name: "AppCaptureEnabled", Kind: "dword", On: 0},
			},
		},
		"visualfx": {
			ID: "visualfx", Name: "极简外观（Atlas 风格）",
			Desc: "VisualFXSetting=2、关闭透明效果；减少桌面合成开销，属于高级项，可逆。",
			Sets: []TweakSet{
				{Key: `HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\VisualEffects`, Name: "VisualFXSetting", Kind: "dword", On: 2},
				{Key: `HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, Name: "EnableTransparency", Kind: "dword", On: 0},
			},
		},
		"priosep": {
			ID: "priosep", Name: "前台调度偏好 38",
			Desc: "HKLM\\SYSTEM\\CurrentControlSet\\Control\\PriorityControl Win32PrioritySeparation=38；改变时间片策略，需重启，高级项。",
			Sets: []TweakSet{{Key: `HKLM\SYSTEM\CurrentControlSet\Control\PriorityControl`, Name: "Win32PrioritySeparation", Kind: "dword", On: 38}},
		},
		"hags": {
			ID: "hags", Name: "硬件加速 GPU 调度",
			Desc:   "HKLM\\SYSTEM\\CurrentControlSet\\Control\\GraphicsDrivers HwSchMode=2；需重启，部分驱动会掉帧，高级项。",
			Sets:   []TweakSet{{Key: `HKLM\SYSTEM\CurrentControlSet\Control\GraphicsDrivers`, Name: "HwSchMode", Kind: "dword", On: 2}},
			Reboot: true,
		},
		"fso": {
			ID: "fso", Name: "禁用全屏优化（CODM）",
			Desc: "为 CODM.exe 写入 AppCompatFlags 层，绕过 Windows 的全屏优化合成；可逆，高级项。",
			Sets: []TweakSet{{Key: `HKCU\Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers`, Name: exe, Kind: "sz", OnS: "~ DISABLEDXMAXIMIZEDWINDOWEDMODE"}},
		},
		"gpupref": {
			ID: "gpupref", Name: "GPU 高性能偏好",
			Desc: "在 DirectX UserGpuPreferences 中把 CODM.exe 标记为高性能 GPU；双显卡机器直接受益。",
			Sets: []TweakSet{{Key: `HKCU\Software\Microsoft\DirectX\UserGpuPreferences`, Name: exe, Kind: "sz", OnS: "GpuPreference=2;"}},
		},
	}
	return defs
}

// TweakOn reports whether the tweak is applied by this tool.
func (c *Control) TweakOn(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg.Tweaks[id]
}

// TweakDefs exposes the definitions to the UI.
func (c *Control) TweakDefs() map[string]TweakDef { return c.tweakDefs() }

// SetTweak applies or reverts one tweak with a full snapshot of originals.
func (c *Control) SetTweak(id string, on bool) {
	defs := c.tweakDefs()
	def, ok := defs[id]
	if !ok {
		return
	}
	if (id == "fso" || id == "gpupref") && c.Cfg().GameExe == "" {
		c.logf("系统项：%s 需要先探测到 CODM 路径", def.Name)
		return
	}
	if on {
		snaps := make([]RegSnap, 0, len(def.Sets))
		for _, s := range def.Sets {
			snaps = append(snaps, regSnap(s.Key, s.Name))
		}
		for _, s := range def.Sets {
			regApply(s)
		}
		c.mu.Lock()
		c.cfg.Tweaks[id] = true
		c.cfg.TweakSnaps[id] = snaps
		c.changed()
		c.mu.Unlock()
		c.saveCfg()
		c.logf("系统项：%s 已应用（%d 个值）", def.Name, len(def.Sets))
		return
	}
	c.mu.Lock()
	snaps := c.cfg.TweakSnaps[id]
	c.cfg.Tweaks[id] = false
	c.changed()
	c.mu.Unlock()
	for _, s := range snaps {
		regRestore(s)
	}
	c.saveCfg()
	c.logf("系统项：%s 已还原", def.Name)
}

// TweakNote returns a short status line for the UI.
func (c *Control) TweakNote(id string) string {
	def, ok := c.tweakDefs()[id]
	if !ok {
		return ""
	}
	if c.TweakOn(id) {
		s := "已应用 · 可还原"
		if def.Reboot {
			s += " · 需重启"
		}
		return s
	}
	return "未应用"
}

// FormatTweakPath shortens a key for log/UI use.
func FormatTweakPath(key string) string { return filepath.ToSlash(key) }

var _ = fmt.Sprintf
