package tune

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/sys/windows/registry"
)

// FOV payloads are 8-byte REG_DWORD blobs. The community-proven unlock changes
// byte 6 only (0xF2 default -> 0xFF) and leaves bytes 0..5/7 intact. Writing a
// float64 over the whole payload (the previous v3 attempt) is wrong: the tail
// bytes are a float scale, not degrees.
func (c *Control) FOVView() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg.FOV
}

func (c *Control) FOVWeapon() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg.FOVWeapon
}

func (c *Control) FOVByte() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg.FOVByte
}

func (c *Control) SetFOVView(v int) {
	c.mu.Lock()
	c.cfg.FOV = v
	c.changed()
	c.mu.Unlock()
	c.saveCfg()
}

func (c *Control) SetFOVWeapon(v int) {
	c.mu.Lock()
	c.cfg.FOVWeapon = v
	c.changed()
	c.mu.Unlock()
	c.saveCfg()
}

func (c *Control) SetFOVByte(v int) {
	if v < 0 || v > 255 {
		return
	}
	c.mu.Lock()
	c.cfg.FOVByte = v
	c.changed()
	c.mu.Unlock()
	c.saveCfg()
}

// FOVKeys lists the registry values for both families.
func (c *Control) FOVKeys() (view, weapon []string) {
	k, err := registry.OpenKey(registry.CURRENT_USER, regKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return nil, nil
	}
	defer k.Close()
	names, _ := k.ReadValueNames(-1)
	return splitFOV(names)
}

// readRawFOV returns the payload bytes of one key.
func readRawFOV(k registry.Key, name string) []byte {
	b := make([]byte, 32)
	n, _, err := k.GetValue(name, b)
	if err != nil || n <= 0 {
		return nil
	}
	return b[:n]
}

// CurrentWeaponFOVByte reports byte 6 of the first WeaponFov payload.
func (c *Control) CurrentWeaponFOVByte() (int, bool) {
	_, weapon := c.FOVKeys()
	if len(weapon) == 0 {
		return 0, false
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, regKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return 0, false
	}
	defer k.Close()
	b := readRawFOV(k, weapon[0])
	if len(b) < 7 {
		return 0, false
	}
	return int(b[6]), true
}

// writeFOVByte changes only byte 6, preserving the 8-byte payload width and
// every other byte. This matches reg_unlock_fov's proven behavior.
func writeFOVByte(k registry.Key, name string, v int) error {
	b := readRawFOV(k, name)
	if len(b) < 7 {
		return fmt.Errorf("unexpected FOV payload length %d", len(b))
	}
	out := append([]byte(nil), b...)
	out[6] = byte(v)
	return k.SetBinaryValue(name, out)
}

// findFPSKeys returns the value and enable keys, matching the full CODM name
// (the names are prefixed with CODM_..., not bare Framerate...).
func findFPSKeys(names []string) (valueKey, enableKey string) {
	for _, n := range names {
		lower := strings.ToLower(n)
		switch {
		case strings.Contains(lower, "frameratecustomizevalue"):
			valueKey = n
		case strings.Contains(lower, "enableframeratecustomize"):
			enableKey = n
		}
	}
	return
}

func (c *Control) setRegBad(msg string) {
	c.mu.Lock()
	c.reg.Text = "未写入"
	c.reg.Detail = msg
	c.changed()
	c.mu.Unlock()
	c.logf("注册表：%s", msg)
}

// ApplyFOV writes weapon byte 6 and optional FPS keys after a backup. It never
// writes the view FOV family: that remains the in-game slider's job.
func (c *Control) ApplyFOV() {
	c.mu.Lock()
	running := c.gameRunning
	fovByte := c.cfg.FOVByte
	unlimited := c.cfg.FPSUnlimited
	c.mu.Unlock()
	if running || c.gameAlive() {
		c.setRegBad("游戏运行中：只读扫描，拒绝写入。请先退出游戏")
		return
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, regKeyPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		c.setRegBad("目标键不存在：请先进入一次游戏，拖动灵敏度/帧数微调")
		return
	}
	defer k.Close()
	names, _ := k.ReadValueNames(-1)
	_, weaponKeys := splitFOV(names)
	fpsKey, fpsEnable := findFPSKeys(names)
	if len(weaponKeys) == 0 && fpsKey == "" && fpsEnable == "" {
		c.setRegBad("目标键存在但缺少可写值：请先在游戏内打开帧数微调并拖动一次 FOV")
		return
	}
	backup := c.cfg.RegistryBackup
	if backup == "" || !fileExists(backup) {
		backup = configDir() + "\\reg-backup-" + time.Now().Format("20060102-150405") + ".reg"
		if err := c.exportRegistry(backup); err != nil {
			c.setRegBad("备份失败，已中止写入：" + err.Error())
			return
		}
	}
	weaponN := 0
	for _, n := range weaponKeys {
		if err := writeFOVByte(k, n, fovByte); err == nil {
			weaponN++
		}
	}
	fpsNote := ""
	if unlimited {
		enabled := false
		if fpsEnable != "" {
			if err := k.SetDWordValue(fpsEnable, 1); err == nil {
				enabled = true
			}
		}
		if fpsKey == "" {
			fpsNote = "；帧率值键缺失"
		} else if err := k.SetDWordValue(fpsKey, 0); err == nil {
			fpsNote = "；帧率=无上限"
			if enabled {
				fpsNote += "（已开帧数微调）"
			}
		} else {
			fpsNote = "；帧率写入失败"
		}
	}
	stateKey := fpsKey
	if stateKey == "" {
		stateKey = fpsEnable
	}
	c.mu.Lock()
	c.cfg.RegistryBackup = backup
	c.reg.Applied = true
	c.reg.Backup = backup
	c.reg.FPSKey = stateKey
	c.reg.FOVKeys = append([]string{}, weaponKeys...)
	c.reg.Text = fmt.Sprintf("持枪 FOV 解锁：字节 0x%02X", fovByte)
	c.reg.Detail = fmt.Sprintf("持枪键 %d · 只改第 7 字节，8 字节 payload 保持不变%s；备份 %s", weaponN, fpsNote, backup)
	c.changed()
	c.mu.Unlock()
	c.saveCfg()
	c.logf("注册表：持枪 FOV byte=0x%02X（%d 键），未写视野键%s", fovByte, weaponN, fpsNote)
}

func splitFOV(names []string) (view, weapon []string) {
	for _, n := range names {
		lower := strings.ToLower(n)
		if strings.Contains(lower, "maincamerafovingame") {
			view = append(view, n)
		} else if strings.Contains(lower, "weaponfov") {
			weapon = append(weapon, n)
		}
	}
	return
}
