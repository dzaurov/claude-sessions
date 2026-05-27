package scanner

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dzaurov/claude-sessions/internal/cache"
)

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestScanFindsJsonl(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "-Users-test-Documents-proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("../../testdata/normal.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	jsonlPath := filepath.Join(proj, "abc-uuid.jsonl")
	if err := os.WriteFile(jsonlPath, src, 0o644); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(proj, "abc-uuid"), []byte("garbage"), 0o644)
	os.MkdirAll(filepath.Join(proj, "memory"), 0o755)
	os.WriteFile(filepath.Join(proj, "memory", "note.md"), []byte("x"), 0o644)

	c := cache.New(filepath.Join(t.TempDir(), "idx.json"))
	if err := c.Load(); err != nil {
		t.Fatal(err)
	}
	s := New([]string{root}, c)
	entries, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].UUID != "abc-uuid" {
		t.Errorf("UUID=%q", entries[0].UUID)
	}
	if entries[0].Title == "" {
		t.Error("title empty")
	}
}

func TestScanUsesCacheWhenMtimeUnchanged(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "-Users-test-proj")
	os.MkdirAll(proj, 0o755)
	src, _ := os.ReadFile("../../testdata/normal.jsonl")
	jsonlPath := filepath.Join(proj, "u1.jsonl")
	os.WriteFile(jsonlPath, src, 0o644)

	c := cache.New(filepath.Join(t.TempDir(), "idx.json"))
	c.Load()
	s := New([]string{root}, c)
	first, _ := s.Scan()
	if len(first) != 1 {
		t.Fatalf("first scan: expected 1, got %d", len(first))
	}
	e := first[0]
	e.Title = "CACHED MARKER"
	c.Set(e)

	second, _ := s.Scan()
	if len(second) != 1 || second[0].Title != "CACHED MARKER" {
		t.Errorf("expected cached title preserved, got title=%q", second[0].Title)
	}
}

func TestScanReparsesWhenMtimeNewer(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "-Users-test-proj")
	os.MkdirAll(proj, 0o755)
	src, _ := os.ReadFile("../../testdata/normal.jsonl")
	jsonlPath := filepath.Join(proj, "u1.jsonl")
	os.WriteFile(jsonlPath, src, 0o644)

	c := cache.New(filepath.Join(t.TempDir(), "idx.json"))
	c.Load()
	s := New([]string{root}, c)
	s.Scan()

	// Plant a stale title in cache and bump file mtime
	got, _ := c.Get("/Users/test/proj::u1")
	got.Title = "STALE"
	c.Set(got)
	future := mustParseTime(t, "2099-01-01T00:00:00Z")
	os.Chtimes(jsonlPath, future, future)

	entries, _ := s.Scan()
	if entries[0].Title == "STALE" {
		t.Errorf("expected re-parsed title, got stale")
	}
}

func TestScanRemovesMissingFiles(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "-Users-test-proj")
	os.MkdirAll(proj, 0o755)
	src, _ := os.ReadFile("../../testdata/normal.jsonl")
	jsonlPath := filepath.Join(proj, "u1.jsonl")
	os.WriteFile(jsonlPath, src, 0o644)

	c := cache.New(filepath.Join(t.TempDir(), "idx.json"))
	c.Load()
	s := New([]string{root}, c)
	s.Scan()

	os.Remove(jsonlPath)
	entries, _ := s.Scan()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after removal, got %d", len(entries))
	}
	if c.Len() != 0 {
		t.Errorf("expected cache emptied, got %d", c.Len())
	}
}
