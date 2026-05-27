// Package cache persists parsed JSONL metadata so ccs doesn't re-scan
// unchanged session files on every launch. Invalidation is by mtime — the
// scanner compares file mtime to the cached Mtime and re-parses if newer.
package cache

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const schemaVersion = 1

type Entry struct {
	Key          string    `json:"key"`
	UUID         string    `json:"uuid"`
	ProjectPath  string    `json:"project_path"`
	Title        string    `json:"title"`
	CustomTitle  string    `json:"custom_title"`
	LastActivity time.Time `json:"last_activity"`
	Mtime        time.Time `json:"mtime"`
	MsgCount     int       `json:"msg_count"`
	Cwd          string    `json:"cwd"`
	GitBranch    string    `json:"git_branch"`
	FilePath     string    `json:"file_path"`
}

type file struct {
	Version int              `json:"version"`
	Entries map[string]Entry `json:"entries"`
}

type Cache struct {
	path    string
	mu      sync.RWMutex
	entries map[string]Entry
}

func New(path string) *Cache {
	return &Cache{path: path, entries: make(map[string]Entry)}
}

func (c *Cache) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, err := os.ReadFile(c.path)
	if errors.Is(err, fs.ErrNotExist) {
		c.entries = make(map[string]Entry)
		return nil
	}
	if err != nil {
		return err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		c.entries = make(map[string]Entry)
		return nil
	}
	if f.Entries == nil {
		f.Entries = make(map[string]Entry)
	}
	c.entries = f.Entries
	return nil
}

func (c *Cache) Save() error {
	c.mu.RLock()
	f := file{Version: schemaVersion, Entries: c.entries}
	c.mu.RUnlock()
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

func (c *Cache) Get(key string) (Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	return e, ok
}

func (c *Cache) Set(e Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[e.Key] = e
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

func (c *Cache) All() []Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Entry, 0, len(c.entries))
	for _, e := range c.entries {
		out = append(out, e)
	}
	return out
}

func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
