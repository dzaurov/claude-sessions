package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if !s.LastFullScan().IsZero() {
		t.Errorf("expected zero LastFullScan on fresh load")
	}
	now := time.Now().Truncate(time.Second).UTC()
	s.MarkFullScan(now)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	s2 := New(path)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	if !s2.LastFullScan().Equal(now) {
		t.Errorf("got %v, want %v", s2.LastFullScan(), now)
	}
}

func TestLoadCorruptResets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := writeFile(path, "not json"); err != nil {
		t.Fatal(err)
	}
	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatalf("expected no error on corrupt, got %v", err)
	}
	if !s.LastFullScan().IsZero() {
		t.Error("expected zero state after corrupt load")
	}
}

func writeFile(path, content string) error {
	return os_WriteFile(path, []byte(content))
}
