package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dzaurov/claude-sessions/internal/cache"
)

func TestFullScan_findsSessionInArbitraryPath(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "some", "random", "place")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("../../testdata/normal.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deep, "stray.jsonl"), src, 0o644); err != nil {
		t.Fatal(err)
	}

	c := cache.New(filepath.Join(t.TempDir(), "idx.json"))
	c.Load()
	entries, err := FullScan(DiscoveryOptions{Paths: []string{root}}, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].UUID != "stray" {
		t.Errorf("UUID=%q", entries[0].UUID)
	}
}

func TestFullScan_skipsNonSessionJsonl(t *testing.T) {
	root := t.TempDir()
	garbage, _ := os.ReadFile("../../testdata/not_a_session.jsonl")
	os.WriteFile(filepath.Join(root, "junk.jsonl"), garbage, 0o644)
	src, _ := os.ReadFile("../../testdata/normal.jsonl")
	os.WriteFile(filepath.Join(root, "real.jsonl"), src, 0o644)

	c := cache.New(filepath.Join(t.TempDir(), "idx.json"))
	c.Load()
	entries, _ := FullScan(DiscoveryOptions{Paths: []string{root}}, c)
	if len(entries) != 1 {
		t.Errorf("expected 1 (only real session), got %d", len(entries))
	}
}

func TestFullScan_honorsIgnore(t *testing.T) {
	root := t.TempDir()
	ignored := filepath.Join(root, "node_modules")
	os.MkdirAll(ignored, 0o755)
	src, _ := os.ReadFile("../../testdata/normal.jsonl")
	os.WriteFile(filepath.Join(ignored, "hidden.jsonl"), src, 0o644)

	c := cache.New(filepath.Join(t.TempDir(), "idx.json"))
	c.Load()
	entries, _ := FullScan(DiscoveryOptions{Paths: []string{root}, Ignore: []string{"node_modules"}}, c)
	if len(entries) != 0 {
		t.Errorf("expected 0 (ignored), got %d", len(entries))
	}
}

func TestFullScan_honorsExtraSkip(t *testing.T) {
	root := t.TempDir()
	skipped := filepath.Join(root, "projects")
	os.MkdirAll(skipped, 0o755)
	src, _ := os.ReadFile("../../testdata/normal.jsonl")
	os.WriteFile(filepath.Join(skipped, "skip.jsonl"), src, 0o644)

	c := cache.New(filepath.Join(t.TempDir(), "idx.json"))
	c.Load()
	entries, _ := FullScan(DiscoveryOptions{Paths: []string{root}, ExtraSkip: []string{skipped}}, c)
	if len(entries) != 0 {
		t.Errorf("expected 0 (extra-skipped), got %d", len(entries))
	}
}
