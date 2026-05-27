package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.json")
	c := New(path)
	c.Set(Entry{
		Key:          "/p::u1",
		UUID:         "u1",
		ProjectPath:  "/p",
		Title:        "hello",
		LastActivity: time.Date(2026, 5, 15, 14, 0, 0, 0, time.UTC),
		Mtime:        time.Date(2026, 5, 15, 14, 0, 0, 0, time.UTC),
		MsgCount:     3,
		Cwd:          "/p",
	})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2 := New(path)
	if err := c2.Load(); err != nil {
		t.Fatal(err)
	}
	e, ok := c2.Get("/p::u1")
	if !ok {
		t.Fatal("entry missing")
	}
	if e.Title != "hello" || e.MsgCount != 3 {
		t.Errorf("entry mismatch: %+v", e)
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	c := New(filepath.Join(dir, "doesnotexist.json"))
	if err := c.Load(); err != nil {
		t.Fatalf("Load on missing file should not error, got %v", err)
	}
	if c.Len() != 0 {
		t.Errorf("expected empty cache")
	}
}

func TestLoadCorruptResetsCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := New(path)
	if err := c.Load(); err != nil {
		t.Fatalf("Load on corrupt file should not error (should reset), got %v", err)
	}
	if c.Len() != 0 {
		t.Errorf("expected empty cache after corrupt load")
	}
}

func TestAtomicWrite_noTempLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.json")
	c := New(path)
	c.Set(Entry{Key: "k", UUID: "u"})
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file leftover: %s", e.Name())
		}
	}
}
