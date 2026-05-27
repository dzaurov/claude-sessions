// Package test contains end-to-end integration tests that drive the ccs
// binary against a synthesized ~/.claude/projects/ directory.
package test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// buildBinary compiles cmd/ccs once and returns the path.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "ccs")
	cmd := exec.Command("go", "build", "-o", bin, "../cmd/ccs")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build: %v", err)
	}
	return bin
}

// setupSandbox creates an isolated CLAUDE_CONFIG_DIR with a single fixture
// session and a mock `claude` binary that logs its argv to a file.
type sandbox struct {
	home       string
	mockBinDir string
	claudeLog  string
	jsonlPath  string
	uuid       string
	sessionCwd string
}

func newSandbox(t *testing.T) sandbox {
	t.Helper()
	home := t.TempDir()
	mockBin := filepath.Join(home, "mock-bin")
	logFile := filepath.Join(home, "claude.log")
	work := filepath.Join(home, "work", "app")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mockBin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/bash\necho \"args: $@\" >> " + logFile + "\necho \"pwd: $PWD\" >> " + logFile + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(mockBin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	projDir := filepath.Join(home, ".claude", "projects", "-Users-fake-app")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	uuid := "abcd1234-abcd-1234-abcd-123456789012"
	jsonlPath := filepath.Join(projDir, uuid+".jsonl")
	body := `{"type":"custom-title","customTitle":"Test session"}` + "\n" +
		`{"type":"user","isMeta":false,"isSidechain":false,"userType":"external","cwd":"` + work + `","gitBranch":"main","message":{"role":"user","content":"Fix the bug"},"timestamp":"2026-05-20T10:00:00.000Z"}` + "\n"
	if err := os.WriteFile(jsonlPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return sandbox{
		home:       home,
		mockBinDir: mockBin,
		claudeLog:  logFile,
		jsonlPath:  jsonlPath,
		uuid:       uuid,
		sessionCwd: work,
	}
}

func (s sandbox) cmd(t *testing.T, bin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(),
		"HOME="+s.home,
		"CLAUDE_CONFIG_DIR="+filepath.Join(s.home, ".claude"),
		"PATH="+s.mockBinDir+":"+os.Getenv("PATH"),
	)
	stdout, err := cmd.Output()
	var stderr string
	if ee, ok := err.(*exec.ExitError); ok {
		stderr = string(ee.Stderr)
	}
	return string(stdout), stderr, err
}

// TestEndToEnd_listJsonEmpty regression: --list-json must print `[]` not
// `null` when the cache is empty.
func TestEndToEnd_listJsonEmpty(t *testing.T) {
	bin := buildBinary(t)
	emptyHome := t.TempDir()
	cmd := exec.Command(bin, "--list-json")
	cmd.Env = append(os.Environ(),
		"HOME="+emptyHome,
		"CLAUDE_CONFIG_DIR="+filepath.Join(emptyHome, ".claude"),
	)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != "[]" {
		t.Errorf("--list-json on empty cache: got %q, want []", got)
	}
}

// TestEndToEnd_flagOrder regression: `ccs <file> --resume` must work the
// same as `ccs --resume <file>`. Go's stdlib flag.Parse stops at the first
// non-flag, so we reorder os.Args in main.
func TestEndToEnd_flagOrder(t *testing.T) {
	bin := buildBinary(t)
	sb := newSandbox(t)

	// file BEFORE flag should still resume
	stdout, stderr, err := sb.cmd(t, bin, sb.jsonlPath, "--resume")
	if err != nil {
		t.Fatalf("run: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	body, err := os.ReadFile(sb.claudeLog)
	if err != nil {
		t.Fatalf("claude.log missing — --resume was silently dropped: %v", err)
	}
	if !strings.Contains(string(body), "--resume "+sb.uuid) {
		t.Errorf("claude was not invoked with --resume <uuid>; log: %q", body)
	}
}

// TestEndToEnd_directFile_rejectsJunk regression: passing a .jsonl that's
// not a Claude Code session should error rather than print empty metadata.
func TestEndToEnd_directFile_rejectsJunk(t *testing.T) {
	bin := buildBinary(t)
	tmp := t.TempDir()
	junk := filepath.Join(tmp, "junk.jsonl")
	if err := os.WriteFile(junk, []byte(`{"event":"log"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, junk)
	cmd.Env = append(os.Environ(),
		"HOME="+tmp,
		"CLAUDE_CONFIG_DIR="+filepath.Join(tmp, ".claude"),
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit on junk file, got 0; output: %s", out)
	}
	if !strings.Contains(string(out), "does not look like a Claude Code session") {
		t.Errorf("expected signature-failure message, got: %s", out)
	}
}

// TestEndToEnd_fullScanIncludesFastScan regression: --full-scan must NOT
// skip the fast scan over Claude Code's projects directory.
func TestEndToEnd_fullScanIncludesFastScan(t *testing.T) {
	bin := buildBinary(t)
	sb := newSandbox(t)

	stdout, stderr, err := sb.cmd(t, bin, "--full-scan")
	if err != nil {
		t.Fatalf("run: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Index must now contain the fixture from ~/.claude/projects.
	indexPath := filepath.Join(sb.home, ".claude", "cc-sessions", "index.json")
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("index.json missing: %v", err)
	}
	var idx struct {
		Entries map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(raw, &idx); err != nil {
		t.Fatalf("index.json malformed: %v", err)
	}
	if len(idx.Entries) == 0 {
		t.Fatalf("--full-scan didn't include fast scan — index is empty")
	}
}
