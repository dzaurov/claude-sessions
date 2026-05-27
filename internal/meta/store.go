// Package meta persists user-specific state for each session (pin, hide,
// tags, custom title, notes) to ~/.claude/cc-sessions/meta.json. Concurrent
// writes from multiple ccs instances are serialized with an OS file lock.
package meta

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/gofrs/flock"
)

const schemaVersion = 1

type Entry struct {
	Pinned      bool     `json:"pinned"`
	Tags        []string `json:"tags,omitempty"`
	Hidden      bool     `json:"hidden"`
	CustomTitle string   `json:"custom_title,omitempty"`
	Notes       string   `json:"notes,omitempty"`
}

type file struct {
	Version int              `json:"version"`
	Entries map[string]Entry `json:"entries"`
}

type Store struct {
	path    string
	mu      sync.RWMutex
	entries map[string]Entry
}

func New(path string) *Store {
	return &Store{path: path, entries: make(map[string]Entry)}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		s.entries = make(map[string]Entry)
		return nil
	}
	if err != nil {
		return err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		s.entries = make(map[string]Entry)
		return nil
	}
	if f.Entries == nil {
		f.Entries = make(map[string]Entry)
	}
	s.entries = f.Entries
	return nil
}

func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	lock := flock.New(s.path + ".lock")
	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()

	s.mu.RLock()
	f := file{Version: schemaVersion, Entries: s.entries}
	s.mu.RUnlock()
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Get(key string) Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.entries[key]
}

func (s *Store) All() map[string]Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Entry, len(s.entries))
	for k, v := range s.entries {
		out[k] = v
	}
	return out
}

func (s *Store) SetPinned(key string, v bool) { s.update(key, func(e *Entry) { e.Pinned = v }) }
func (s *Store) SetHidden(key string, v bool) { s.update(key, func(e *Entry) { e.Hidden = v }) }
func (s *Store) SetCustomTitle(key, title string) {
	s.update(key, func(e *Entry) { e.CustomTitle = title })
}
func (s *Store) SetTags(key string, tags []string) { s.update(key, func(e *Entry) { e.Tags = tags }) }
func (s *Store) SetNotes(key, notes string)        { s.update(key, func(e *Entry) { e.Notes = notes }) }

func (s *Store) update(key string, fn func(*Entry)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[key]
	fn(&e)
	s.entries[key] = e
}
