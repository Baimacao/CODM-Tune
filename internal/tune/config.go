package tune

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// RegSnap is one saved registry value, used to make tweaks reversible.
type RegSnap struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Type   string `json:"type"` // dword | sz
	Dword  uint32 `json:"dword"`
	Str    string `json:"str"`
	Exists bool   `json:"exists"`
}

// Config is persisted between runs under %APPDATA%\CODMTune.
type Config struct {
	Version    int    `json:"version"`
	GameExe    string `json:"game_exe"`
	Launcher   string `json:"launcher"`
	InstallDir string `json:"install_dir"`

	PowerEnabled    bool `json:"power_enabled"`
	ScheduleEnabled bool `json:"schedule_enabled"`
	ACEEnabled      bool `json:"ace_enabled"`
	BGEnabled       bool `json:"bg_enabled"`

	ACENames   []string `json:"ace_names"`
	BGSelected []string `json:"bg_selected"`

	FOV          int  `json:"fov"`
	FOVWeapon    int  `json:"fov_weapon"`
	FOVByte      int  `json:"fov_byte"`
	WeaponFixed  bool `json:"weapon_fov_fixed"`
	FPSUnlimited bool `json:"fps_unlimited"`

	TdrDelay      bool   `json:"tdr_delay"`
	TdrHad        bool   `json:"tdr_had"`
	TdrValue      uint32 `json:"tdr_value"`
	PowerPrior    string `json:"power_prior"`
	PowerInjected string `json:"power_injected"`

	RegistryBackup string `json:"registry_backup"`
	DNSTarget      string `json:"dns_target"`

	// Appearance and interaction.
	Theme      string  `json:"ui_theme"` // dark | light
	Style      string  `json:"ui_style"` // standard | ark
	Font       string  `json:"ui_font"`
	Radius     float64 `json:"ui_radius"`
	Background string  `json:"ui_background"`
	Advanced   bool    `json:"advanced"`
	Accent     string  `json:"accent"`
	AccentHex  string  `json:"accent_hex"`
	Sound      bool    `json:"sound"`
	SoundVol   int     `json:"sound_volume"`
	// SoundKinds is four flags: startup, move, confirm, launch. Empty means all
	// on, so existing configs gain the new cues without a migration.
	SoundKinds string `json:"sound_kinds"`

	// Game launch.
	LaunchArgs    string    `json:"launch_args"`
	MouseAccelOff bool      `json:"mouse_accel_off"`
	MouseSnap     []RegSnap `json:"mouse_snap"`
	TimerBoost    bool      `json:"timer_boost"`

	// Tweak backups, keyed by tweak id.
	TweakSnaps map[string][]RegSnap `json:"tweak_snaps"`
	Tweaks     map[string]bool      `json:"tweaks"`

	// Overlay.
	OverlayEnabled bool   `json:"overlay_enabled"`
	OverlayIdle    bool   `json:"overlay_idle"`
	OverlayStyle   string `json:"overlay_style"` // compact | full
	OverlayOpacity int    `json:"overlay_opacity"`
	OverlayCorner  string `json:"overlay_corner"` // tr | tl | br | bl | custom
	OverlayX       int    `json:"overlay_x"`
	OverlayY       int    `json:"overlay_y"`
	OverlayFPS     bool   `json:"overlay_fps"`
	OverlayLow     bool   `json:"overlay_low"`
	OverlayStutter bool   `json:"overlay_stutter"`
	OverlayLoss    bool   `json:"overlay_loss"`
	OverlayText    bool   `json:"overlay_text"`
	OverlayKeys    bool   `json:"overlay_keys"`
	OverlayNet     bool   `json:"overlay_net"`
	OverlayServer  bool   `json:"overlay_server"`

	// Crosshair overlay and first-run wizard.
	OOBEDone           bool   `json:"oobe_done"`
	CrosshairEnabled   bool   `json:"crosshair_enabled"`
	CrosshairStyle     string `json:"crosshair_style"` // cross | dot | circle | t
	CrosshairLength    int    `json:"crosshair_length"`
	CrosshairThickness int    `json:"crosshair_thickness"`
	CrosshairGap       int    `json:"crosshair_gap"`
	CrosshairColor     string `json:"crosshair_color"`
	CrosshairAlpha     int    `json:"crosshair_alpha"`
	CrosshairDot       bool   `json:"crosshair_dot"`
	CrosshairOutline   bool   `json:"crosshair_outline"`
	CrosshairOffsetX   int    `json:"crosshair_offset_x"`
	CrosshairOffsetY   int    `json:"crosshair_offset_y"`
	CrosshairCode      string `json:"crosshair_code"`

	// Replay and knowledge.
	ReplayDir    string `json:"replay_dir"`
	KnowledgeDir string `json:"knowledge_dir"`
}

func defaultConfig() Config {
	return Config{
		Version:            12,
		PowerEnabled:       true,
		ScheduleEnabled:    true,
		ACENames:           []string{"SGuard64.exe", "SGuardSvc64.exe"},
		FOV:                90,
		FOVWeapon:          96,
		FOVByte:            0xF2,
		WeaponFixed:        true,
		DNSTarget:          "codm.qq.com",
		Theme:              "light",
		Style:              "ark",
		Font:               "MiSans",
		Accent:             "red",
		AccentHex:          "",
		Radius:             2,
		Sound:              true,
		SoundVol:           55,
		SoundKinds:         "1111",
		OverlayStyle:       "compact",
		OverlayIdle:        true,
		OverlayOpacity:     245,
		OverlayCorner:      "tr",
		OverlayX:           -1,
		OverlayY:           -1,
		OverlayFPS:         true,
		OverlayLow:         true,
		OverlayStutter:     true,
		OverlayLoss:        false,
		OverlayKeys:        true,
		OverlayNet:         true,
		OverlayServer:      true,
		CrosshairStyle:     "cross",
		CrosshairLength:    10,
		CrosshairThickness: 2,
		CrosshairGap:       5,
		CrosshairColor:     "#39FF14",
		CrosshairAlpha:     230,
		CrosshairDot:       true,
		CrosshairOutline:   true,
		TweakSnaps:         map[string][]RegSnap{},
		Tweaks:             map[string]bool{},
	}
}

func configDir() string {
	if alt := os.Getenv("CODMTUNE_HOME"); alt != "" {
		_ = os.MkdirAll(alt, 0o755)
		return alt
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "CODMTune")
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func loadConfig() Config {
	cfg := defaultConfig()
	b, err := os.ReadFile(filepath.Join(configDir(), "config.json"))
	if err != nil {
		return cfg
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}) // tolerate editors that add a BOM
	if err := json.Unmarshal(b, &cfg); err != nil {
		return defaultConfig()
	}
	if cfg.Version < 9 {
		// The HUD split 1% low / stutter / loss into independent switches in
		// v9. Older configs had them implicitly on; preserve that intent for
		// low/stutter, but loss stays off by default after v11.
		cfg.OverlayLow = true
		cfg.OverlayStutter = true
		cfg.Version = 9
	}
	if cfg.Version < 11 {
		// Loss is opt-in: it is an estimate and was previously mistaken for a
		// fixed number. Existing configs get one clean default-off migration.
		cfg.OverlayLoss = false
		cfg.Version = 11
	}
	if cfg.Version < 12 {
		// Province is now a first-class HUD line; turn the opt-in lookup on for
		// existing configs as well.
		cfg.OverlayServer = true
		cfg.Version = 12
	}
	if len(cfg.ACENames) == 0 {
		cfg.ACENames = defaultConfig().ACENames
	}
	if cfg.DNSTarget == "" {
		cfg.DNSTarget = "codm.qq.com"
	}
	if cfg.FOV < 60 || cfg.FOV > 140 {
		cfg.FOV = 90
	}
	if cfg.FOVByte < 0 || cfg.FOVByte > 255 {
		cfg.FOVByte = 0xF2
	}
	if cfg.FOVWeapon < 50 || cfg.FOVWeapon > 140 {
		cfg.FOVWeapon = 96
	}
	if cfg.Style != "ark" {
		cfg.Style = "standard"
	}
	if cfg.Theme != "light" && cfg.Theme != "oled" {
		cfg.Theme = "dark"
	}
	switch cfg.Accent {
	case "mint", "amber", "violet", "red", "custom":
	default:
		cfg.Accent = "blue"
	}
	if cfg.Font == "" {
		cfg.Font = "MiSans"
	}
	if cfg.Style == "ark" {
		if cfg.Radius < 2 || cfg.Radius > 34 {
			cfg.Radius = 2
		}
	} else if cfg.Radius < 10 || cfg.Radius > 34 {
		cfg.Radius = 22
	}
	if cfg.SoundVol < 0 || cfg.SoundVol > 100 {
		cfg.SoundVol = 55
	}
	if cfg.CrosshairStyle == "" {
		cfg.CrosshairStyle = "cross"
	}
	if cfg.CrosshairLength <= 0 || cfg.CrosshairLength > 64 {
		cfg.CrosshairLength = 10
	}
	if cfg.CrosshairThickness <= 0 || cfg.CrosshairThickness > 12 {
		cfg.CrosshairThickness = 2
	}
	if cfg.CrosshairGap < 0 || cfg.CrosshairGap > 40 {
		cfg.CrosshairGap = 5
	}
	if len(cfg.CrosshairColor) != 7 || cfg.CrosshairColor[0] != '#' {
		cfg.CrosshairColor = "#39FF14"
	}
	if cfg.CrosshairAlpha < 40 || cfg.CrosshairAlpha > 255 {
		cfg.CrosshairAlpha = 230
	}
	if len(cfg.SoundKinds) != 4 {
		cfg.SoundKinds = "1111"
	}
	if cfg.OverlayOpacity < 178 {
		cfg.OverlayOpacity = 178
	}
	if cfg.OverlayOpacity > 255 {
		cfg.OverlayOpacity = 255
	}
	if cfg.OverlayStyle != "full" {
		cfg.OverlayStyle = "compact"
	}
	switch cfg.OverlayCorner {
	case "tl", "br", "bl", "custom":
	default:
		cfg.OverlayCorner = "tr"
	}
	if cfg.OverlayX < 0 {
		cfg.OverlayX = 1100
	}
	if cfg.OverlayY < 0 {
		cfg.OverlayY = 300
	}
	if cfg.TweakSnaps == nil {
		cfg.TweakSnaps = map[string][]RegSnap{}
	}
	if cfg.Tweaks == nil {
		cfg.Tweaks = map[string]bool{}
	}
	return cfg
}

var saveMu sync.Mutex

func saveConfig(cfg Config) error {
	saveMu.Lock()
	defer saveMu.Unlock()
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(configDir(), "config.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
