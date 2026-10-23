package tune

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"

	"codmtune/internal/sys"
)

var scanSkip = map[string]bool{
	"$recycle.bin": true, "system volume information": true, "windows": true,
	"program files": true, "program files (x86)": true, "programdata": true,
	"users": true, "recovery": true, "perflogs": true, "msocache": true,
	"windows.old": true, "documents and settings": true, "appdata": true,
	"node_modules": true, ".git": true, "$windows.~bt": true, "$windows.~ws": true,
}

func isGameExe(name string) bool {
	n := strings.ToLower(name)
	if !strings.HasSuffix(n, ".exe") {
		return false
	}
	if strings.Contains(n, "unins") || strings.Contains(n, "sguard") || strings.Contains(n, "ace-") {
		return false
	}
	return strings.HasPrefix(n, "codm") || strings.HasPrefix(n, "call of duty") || strings.Contains(n, "使命召唤")
}

// scanGames walks the likely game folders first, then falls back to a full
// bounded walk only when the hints find nothing.
func scanGames(progress func(string)) []Candidate {
	drives := sys.FixedDrives()
	// C is filled with unrelated software; look there last.
	if len(drives) > 1 {
		for i, d := range drives {
			if strings.EqualFold(d, "C:") {
				drives = append(append(drives[:i], drives[i+1:]...), d)
				break
			}
		}
	}
	var out []Candidate
	seen := map[string]bool{}
	for _, drive := range drives {
		if len(out) > 0 {
			break
		}
		root := drive + `\`
		if progress != nil {
			progress(root)
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || !hintDir(e.Name()) {
				continue
			}
			sub := filepath.Join(root, e.Name())
			if progress != nil {
				progress(sub)
			}
			out = append(out, walkForGame(sub, 7, seen)...)
			if len(out) >= 4 {
				break
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, drive := range drives {
		if len(out) >= 4 {
			break
		}
		root := drive + `\`
		if progress != nil {
			progress(root)
		}
		out = append(out, walkForGame(root, 7, seen)...)
	}
	return out
}

// hintDir matches the folder names a CODM install usually sits under.
func hintDir(name string) bool {
	n := strings.ToLower(name)
	for _, h := range []string{"codm", "call of duty", "callofduty", "使命召唤", "tencent", "games", "game", "游戏", "高清", "模拟器"} {
		if strings.Contains(n, h) {
			return true
		}
	}
	return false
}

func walkForGame(root string, maxDepth int, seen map[string]bool) []Candidate {
	var out []Candidate
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(out) >= 4 {
			return filepath.SkipAll
		}
		if d.IsDir() {
			base := strings.ToLower(d.Name())
			if scanSkip[base] {
				return filepath.SkipDir
			}
			if rel, err := filepath.Rel(root, path); err == nil {
				if strings.Count(rel, string(os.PathSeparator)) >= maxDepth {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !isGameExe(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() < 20*1024*1024 {
			return nil
		}
		dir := filepath.Dir(path)
		if !hasDataDir(dir) {
			return nil
		}
		key := strings.ToLower(dir)
		if seen[key] {
			return nil
		}
		seen[key] = true
		out = append(out, Candidate{GameExe: path, InstallDir: dir, Launcher: findLauncher(dir)})
		return nil
	})
	return out
}

func hasDataDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() && strings.EqualFold(e.Name(), "CODM_Data") {
			return true
		}
	}
	return false
}

// findLauncher looks next to the game, then in the uninstall registry, then in
// the known CODMLauncher location.
func findLauncher(gameDir string) string {
	for _, name := range []string{"launcher.exe", "CODM_Launcher.exe"} {
		p := filepath.Join(gameDir, name)
		if fileExists(p) {
			return p
		}
	}
	if p := launcherFromUninstall(); p != "" {
		return p
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		p := filepath.Join(local, "CODMLauncher", "launcher.exe")
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func launcherFromUninstall() string {
	type root struct {
		key  registry.Key
		path string
	}
	roots := []root{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
	}
	for _, r := range roots {
		k, err := registry.OpenKey(r.key, r.path, registry.READ|registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			continue
		}
		names, _ := k.ReadSubKeyNames(-1)
		k.Close()
		for _, name := range names {
			sub, err := registry.OpenKey(r.key, r.path+`\`+name, registry.READ)
			if err != nil {
				continue
			}
			display, _, _ := sub.GetStringValue("DisplayName")
			icon, _, _ := sub.GetStringValue("DisplayIcon")
			loc, _, _ := sub.GetStringValue("InstallLocation")
			sub.Close()
			lower := strings.ToLower(display)
			if !strings.Contains(lower, "codm") && !strings.Contains(display, "使命召唤") &&
				!strings.Contains(lower, "call of duty") && !strings.Contains(display, "模拟器") {
				continue
			}
			for _, p := range []string{cleanIcon(icon), filepath.Join(loc, "launcher.exe")} {
				if p != "" && strings.HasSuffix(strings.ToLower(p), ".exe") && fileExists(p) {
					return p
				}
			}
		}
	}
	return ""
}

func cleanIcon(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, ","); i > 0 {
		s = s[:i]
	}
	return strings.Trim(strings.TrimSpace(s), `"`)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
