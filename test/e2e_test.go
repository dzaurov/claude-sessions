// Package test contains end-to-end integration tests that drive the ccs
// binary against a synthesized ~/.claude/projects/ directory.
package test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// e2eEntry mirrors cache.Entry's JSON shape — we don't import the package to
// keep this test as a pure integration test against the compiled binary.
type e2eEntry struct {
	Key         string `json:"key"`
	UUID        string `json:"uuid"`
	ProjectPath string `json:"project_path"`
	Title       string `json:"title"`
	CustomTitle string `json:"custom_title"`
	MsgCount    int    `json:"msg_count"`
	Cwd         string `json:"cwd"`
	GitBranch   string `json:"git_branch"`
}

func TestEndToEnd_listJson(t *testing.T) {
	// Build a fake HOME with synthetic .claude/projects
	tmpHome := t.TempDir()
	projDir := filepath.Join(tmpHome, ".claude", "projects", "-Users-fake-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Copy fixture
	src, err := os.ReadFile("../testdata/normal.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "session-a.jsonl"), src, 0o644); err != nil {
		t.Fatal(err)
	}

	// Build binary
	bin := filepath.Join(t.TempDir(), "ccs")
	cmd := exec.Command("go", "build", "-o", bin, "../cmd/ccs")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build: %v", err)
	}

	// Run with HOME override
	run := exec.Command(bin, "--list-json")
	run.Env = append(os.Environ(), "HOME="+tmpHome)
	out, err := run.Output()
	if err != nil {
		t.Fatalf("run: %v\nstderr: %s", err, run.Stderr)
	}

	var entries []e2eEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		t.Fatalf("json: %v\nraw: %s", err, out)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.UUID != "session-a" {
		t.Errorf("UUID=%q", e.UUID)
	}
	if e.CustomTitle != "Cascade merge investigation" {
		t.Errorf("CustomTitle=%q", e.CustomTitle)
	}
	if e.Title == "" {
		t.Errorf("Title empty, expected first user message")
	}
	if e.Cwd != "/Users/alice/Documents/example-project" {
		t.Errorf("Cwd=%q", e.Cwd)
	}

	// Verify state files were created
	for _, f := range []string{"config.toml", "index.json"} {
		path := filepath.Join(tmpHome, ".claude", "cc-sessions", f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s, got %v", f, err)
		}
	}
}
