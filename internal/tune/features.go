package tune

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"

	"codmtune/internal/sys"
)

// ConfigPath is the directory holding config.json, logs and backups.
func ConfigPath() string { return configDir() }

// OpenURL opens a URL with the default browser.
func OpenURL(url string) error { return sys.OpenPath(url) }

var mouseKey = `Control Panel\Mouse`

func readMouseSnap() []RegSnap {
	var out []RegSnap
	for _, name := range []string{"MouseSpeed", "MouseThreshold1", "MouseThreshold2"} {
		snap := RegSnap{Key: mouseKey, Name: name, Type: "sz"}
		k, err := registry.OpenKey(registry.CURRENT_USER, mouseKey, registry.QUERY_VALUE)
		if err == nil {
			if v, _, e := k.GetStringValue(name); e == nil {
				snap.Exists, snap.Str = true, v
			}
			k.Close()
		}
		out = append(out, snap)
	}
	return out
}

// MouseAccelOn reports whether this tool applied the disable action.
func (c *Control) MouseAccelOn() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.cfg.MouseAccelOff }

// MouseAccelSystemOff reports the actual Windows setting, even when the user
// (or another tool) had already disabled acceleration before CODMTune ran.
func (c *Control) MouseAccelSystemOff() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, mouseKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue("MouseSpeed")
	return err == nil && v == "0"
}

// SetMouseAccel disables Windows "enhance pointer precision" and the pointer
// acceleration thresholds, or restores the values captured before.
func (c *Control) SetMouseAccel(on bool) {
	if on {
		snap := readMouseSnap()
		k, err := registry.OpenKey(registry.CURRENT_USER, mouseKey, registry.SET_VALUE|registry.QUERY_VALUE)
		if err != nil {
			c.logf("鼠标加速：无法打开 %s：%v", mouseKey, err)
			return
		}
		for _, name := range []string{"MouseSpeed", "MouseThreshold1", "MouseThreshold2"} {
			_ = k.SetStringValue(name, "0")
		}
		k.Close()
		sys.MouseAcceleration(false)
		c.mu.Lock()
		c.cfg.MouseAccelOff = true
		c.cfg.MouseSnap = snap
		c.changed()
		c.mu.Unlock()
		c.saveCfg()
		c.logf("鼠标加速：已关闭（保存 3 个原值）")
		return
	}
	c.mu.Lock()
	snap := c.cfg.MouseSnap
	c.cfg.MouseAccelOff = false
	c.mu.Unlock()
	if len(snap) == 0 {
		c.logf("鼠标加速：不是由本工具修改，保持系统现状")
		c.saveCfg()
		return
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, mouseKey, registry.SET_VALUE)
	if err == nil {
		for _, s := range snap {
			if s.Exists {
				_ = k.SetStringValue(s.Name, s.Str)
			}
		}
		k.Close()
	}
	// Match the immediate SPI state to the restored registry value.
	restoreOn := true
	for _, s := range snap {
		if s.Name == "MouseSpeed" && s.Exists && s.Str == "0" {
			restoreOn = false
		}
	}
	sys.MouseAcceleration(restoreOn)
	c.saveCfg()
	c.logf("鼠标加速：已恢复")
}

// LaunchArgs returns the extra command line appended to CODM.
func (c *Control) LaunchArgs() string { c.mu.Lock(); defer c.mu.Unlock(); return c.cfg.LaunchArgs }

// SetLaunchToken adds or removes one whitespace-delimited launch argument.
func (c *Control) SetLaunchToken(token string, on bool) {
	c.mu.Lock()
	fields := strings.Fields(c.cfg.LaunchArgs)
	has := false
	for _, f := range fields {
		if f == token {
			has = true
		}
	}
	if on && !has {
		fields = append(fields, token)
	}
	if !on && has {
		out := fields[:0]
		for _, f := range fields {
			if f != token {
				out = append(out, f)
			}
		}
		fields = out
	}
	c.cfg.LaunchArgs = strings.Join(fields, " ")
	c.changed()
	c.mu.Unlock()
	c.saveCfg()
}

// HasLaunchToken reports a token in the launch string.
func (c *Control) HasLaunchToken(token string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, f := range strings.Fields(c.cfg.LaunchArgs) {
		if f == token {
			return true
		}
	}
	return false
}

// TimerBoostOn reports the fine-timer toggle.
func (c *Control) TimerBoostOn() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.cfg.TimerBoost }

// SetTimerBoost applies or releases the 0.5 ms timer request while a game is
// running; the setting persists for the next session.
func (c *Control) SetTimerBoost(on bool) {
	c.mu.Lock()
	c.cfg.TimerBoost = on
	running := c.gameRunning
	c.mu.Unlock()
	c.saveCfg()
	if on && running {
		c.applyTimer()
	}
	if !on {
		c.releaseTimer()
	}
	c.changed()
	c.logf("计时器精度：%v", on)
}

func (c *Control) applyTimer() {
	if c.timerActive {
		return
	}
	cur, err := sys.TimerResolution(5000, true)
	if err != nil {
		c.logf("计时器精度：请求失败 %v", err)
		return
	}
	c.timerActive = true
	c.logf("计时器精度：0.5ms 已请求（当前 %d ×100ns）", cur)
}

func (c *Control) releaseTimer() {
	if !c.timerActive {
		return
	}
	if _, err := sys.TimerResolution(5000, false); err == nil {
		c.logf("计时器精度：已释放")
	}
	c.timerActive = false
}

// SensitivitySnapshot saves every registry value whose name mentions
// sensitivity, FOV or ADS for the selected install.
func (c *Control) SensitivitySnapshot() (int, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, regKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return 0, errors.New("目标键不存在")
	}
	defer k.Close()
	names, _ := k.ReadValueNames(-1)
	out := map[string]uint32{}
	fov := map[string]uint32{}
	for _, n := range names {
		lower := strings.ToLower(n)
		if !strings.Contains(lower, "sensitivity") && !strings.Contains(lower, "fov") && !strings.Contains(lower, "ads") {
			continue
		}
		if v, _, err := k.GetIntegerValue(n); err == nil {
			if strings.Contains(lower, "sensitivity") || strings.Contains(lower, "ads") {
				out[n] = uint32(v)
			} else {
				fov[n] = uint32(v)
			}
		}
	}
	doc := map[string]any{"saved": nowStamp(), "sensitivity": out, "fov": fov}
	b, _ := json.MarshalIndent(doc, "", "  ")
	path := filepath.Join(configDir(), "sensitivity-snapshot.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return 0, err
	}
	c.logf("灵敏度快照：保存 %d 个灵敏度值、%d 个 FOV 值 → %s", len(out), len(fov), path)
	return len(out) + len(fov), nil
}

// RestoreSensitivity writes the saved sensitivity values back.
func (c *Control) RestoreSensitivity() (int, error) {
	path := filepath.Join(configDir(), "sensitivity-snapshot.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, errors.New("没有灵敏度快照")
	}
	var doc struct {
		Sensitivity map[string]uint32 `json:"sensitivity"`
		FOV         map[string]uint32 `json:"fov"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return 0, err
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, regKeyPath, registry.SET_VALUE)
	if err != nil {
		return 0, err
	}
	defer k.Close()
	n := 0
	for name, v := range doc.Sensitivity {
		if err := k.SetDWordValue(name, v); err == nil {
			n++
		}
	}
	for name, v := range doc.FOV {
		if err := k.SetDWordValue(name, v); err == nil {
			n++
		}
	}
	c.logf("灵敏度快照：还原 %d 个值", n)
	return n, nil
}

func nowStamp() string { return time.Now().Format("2006-01-02 15:04:05") }
func timeFileStamp() string { return time.Now().Format("20060102-150405") }

// ExclusiveShortcut creates a double-clickable launcher that asks Unity for
// exclusive fullscreen. It writes a .bat rather than touching the game.
func (c *Control) ExclusiveShortcut() (string, error) {
	c.mu.Lock()
	exe := c.gameExe
	c.mu.Unlock()
	if exe == "" {
		return "", fmt.Errorf("尚未探测到 CODM")
	}
	dir := desktopDir()
	if dir == "" {
		return "", fmt.Errorf("无法定位桌面目录")
	}
	path := filepath.Join(dir, "CODM-Exclusive-Fullscreen.bat")
	body := "@echo off\r\nstart \"\" \"" + exe + "\" -window-mode exclusive 5\r\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	c.LogLine("已创建独占全屏快捷方式：" + path)
	return path, nil
}

func desktopDir() string {
	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\Shell Folders`, registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetStringValue("Desktop"); err == nil && v != "" {
			k.Close()
			return v
		}
		k.Close()
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Desktop")
	}
	return ""
}