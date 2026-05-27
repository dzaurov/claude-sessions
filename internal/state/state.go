// Package state persists small bookkeeping data for ccs — currently just
// the timestamp of the last full-disk scan, so we know when to trigger the
// next background sweep.
package state

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type State struct {
	LastFullScan time.Time `json:"last_full_scan"`
}

type Store struct {
	path string
	mu   sync.Mutex
	data State
}

func New(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		s.data = State{}
		return nil
	}
	if err != nil {
		return err
	}
	var d State
	if err := json.Unmarshal(raw, &d); err != nil {
		s.data = State{}
		return nil
	}
	s.data = d
	return nil
}

func (s *Store) Save() error {
	s.mu.Lock()
	d := s.data
	s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) LastFullScan() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.LastFullScan
}

func (s *Store) MarkFullScan(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.LastFullScan = t
}
