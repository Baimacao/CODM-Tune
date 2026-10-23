package tune

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Replay is one detected match recording.
type Replay struct {
	Path string
	Name string
	Size int64
	When time.Time
}

var replayExts = map[string]bool{".replay": true, ".rec": true, ".rep": true, ".demo": true, ".codm": true, ".mtreplay": true}

// ScanReplays looks in the configured directory, likely game subdirectories and
// the user's Documents/Videos folder. It is deliberately conservative: a file
// must have a replay extension or live under a replay-named folder.
func (c *Control) ScanReplays() []Replay {
	c.mu.Lock()
	root := c.cfg.ReplayDir
	install := c.installDir
	c.mu.Unlock()
	var roots []string
	if root != "" {
		roots = append(roots, root)
	}
	if install != "" {
		for _, sub := range []string{"PersistentData", "cache", "CODM_Data"} {
			p := filepath.Join(install, sub)
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				roots = append(roots, p)
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, "Documents"), filepath.Join(home, "Videos"))
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		roots = append(roots, filepath.Join(local, "CODMLauncher"))
	}
	seen := map[string]bool{}
	var out []Replay
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				rel, _ := filepath.Rel(root, path)
				if strings.Count(rel, string(os.PathSeparator)) > 4 {
					return filepath.SkipDir
				}
				return nil
			}
			name := d.Name()
			lower := strings.ToLower(name)
			dirLower := strings.ToLower(filepath.Dir(path))
			isReplay := replayExts[filepath.Ext(lower)]
			if !isReplay {
				for _, hint := range []string{"replay", "record", "回放"} {
					if strings.Contains(dirLower, hint) && (strings.HasSuffix(lower, ".mp4") || strings.HasSuffix(lower, ".bin") || strings.HasSuffix(lower, ".dat")) {
						isReplay = true
					}
				}
			}
			if !isReplay {
				return nil
			}
			info, err := d.Info()
			if err != nil || info.Size() < 32*1024 || seen[path] {
				return nil
			}
			seen[path] = true
			out = append(out, Replay{Path: path, Name: name, Size: info.Size(), When: info.ModTime()})
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].When.After(out[j].When) })
	if len(out) > 200 {
		out = out[:200]
	}
	c.mu.Lock()
	c.replays = out
	c.changed()
	c.mu.Unlock()
	c.logf("回放：扫描到 %d 个文件（根目录 %d 个）", len(out), len(roots))
	return out
}

// Replays returns the last scan result.
func (c *Control) Replays() []Replay {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Replay, len(c.replays))
	copy(out, c.replays)
	return out
}

// BackupReplays copies every detected replay into the config directory. It
// never deletes the originals.
func (c *Control) BackupReplays(list []Replay) (int, error) {
	if len(list) == 0 {
		list = c.Replays()
	}
	dir := filepath.Join(configDir(), "replays", time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	n := 0
	for _, r := range list {
		dst := filepath.Join(dir, r.Name)
		if err := copyFile(r.Path, dst); err == nil {
			n++
		}
	}
	c.logf("回放：备份 %d/%d 个到 %s", n, len(list), dir)
	return n, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// SetReplayDir persists a manually chosen replay folder and rescans.
func (c *Control) SetReplayDir(dir string) {
	c.mu.Lock()
	c.cfg.ReplayDir = dir
	c.changed()
	c.mu.Unlock()
	c.saveCfg()
	c.ScanReplays()
}
