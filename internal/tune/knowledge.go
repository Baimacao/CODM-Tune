package tune

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// KnowledgeFile is one markdown note.
type KnowledgeFile struct {
	Path  string
	Name  string
	Bytes int64
}

// KnowledgeDir returns the notes folder, creating it on first use.
func (c *Control) KnowledgeDir() string {
	c.mu.Lock()
	dir := c.cfg.KnowledgeDir
	c.mu.Unlock()
	if dir == "" {
		dir = filepath.Join(configDir(), "knowledge")
	}
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

// KnowledgeFiles lists markdown notes in the knowledge folder.
func (c *Control) KnowledgeFiles() []KnowledgeFile {
	dir := c.KnowledgeDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []KnowledgeFile
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		info, _ := e.Info()
		size := int64(0)
		if info != nil {
			size = info.Size()
		}
		out = append(out, KnowledgeFile{Path: filepath.Join(dir, e.Name()), Name: strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())), Bytes: size})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ReadKnowledge returns the raw markdown text.
func ReadKnowledge(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

// SetKnowledgeDir persists a custom notes folder.
func (c *Control) SetKnowledgeDir(dir string) {
	c.mu.Lock()
	c.cfg.KnowledgeDir = dir
	c.changed()
	c.mu.Unlock()
	c.saveCfg()
}
