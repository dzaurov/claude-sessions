# ccs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `ccs`, a Go CLI that lists every Claude Code session across all projects with fuzzy search, pin/hide, and resumes the chosen session in the same terminal via `claude --resume <uuid> --dangerously-skip-permissions`.

**Architecture:** Single Go binary with bubbletea TUI. Scans `~/.claude/projects/**/*.jsonl`, caches metadata in `~/.claude/cc-sessions/index.json`, keeps user pins/hides in `meta.json`. On Enter, `os.Chdir` + `syscall.Exec` replaces the TUI process with `claude` — terminal flips seamlessly into the resumed chat.

**Tech Stack:** Go 1.22+, bubbletea/bubbles/lipgloss, sahilm/fuzzy, BurntSushi/toml, gofrs/flock, spf13/afero (tests), charmbracelet/x/exp/teatest (TUI tests).

---

## File Structure

```
cc-sessions/
├── README.md
├── Makefile
├── go.mod / go.sum
├── .gitignore
├── cmd/ccs/main.go
├── internal/
│   ├── paths/paths.go + paths_test.go              # decode folder name → real path
│   ├── parser/parser.go + parser_test.go           # stream JSONL, extract metadata
│   ├── scanner/scanner.go + scanner_test.go        # walk ~/.claude/projects/
│   ├── cache/cache.go + cache_test.go              # ~/.claude/cc-sessions/index.json
│   ├── meta/store.go + store_test.go               # ~/.claude/cc-sessions/meta.json
│   ├── config/config.go + config_test.go           # config.toml
│   ├── launcher/launcher.go + launcher_test.go     # chdir + exec claude
│   ├── session/session.go                          # shared Session type
│   └── tui/
│       ├── model.go + model_test.go
│       ├── view.go
│       └── keys.go
├── testdata/                                       # JSONL fixtures
└── README.md
```

---

## Task 1: Bootstrap project

**Files:**
- Create: `go.mod`, `.gitignore`, `Makefile`, `README.md`

- [ ] **Step 1: Initialize Go module**

Run:
```bash
cd ~/Documents/cc-sessions
go mod init github.com/dzaurov/cc-sessions
```

- [ ] **Step 2: Create `.gitignore`**

```gitignore
/bin/
*.test
*.out
.DS_Store
```

- [ ] **Step 3: Create `Makefile`**

```makefile
.PHONY: build install test lint clean

BIN := bin/ccs
INSTALL_DIR := $(HOME)/.local/bin

build:
	mkdir -p bin
	go build -o $(BIN) ./cmd/ccs

install: build
	mkdir -p $(INSTALL_DIR)
	ln -sfn $(CURDIR)/$(BIN) $(INSTALL_DIR)/ccs
	@echo "Installed: $(INSTALL_DIR)/ccs -> $(CURDIR)/$(BIN)"
	@case ":$$PATH:" in *":$(INSTALL_DIR):"*) ;; *) echo "WARNING: $(INSTALL_DIR) not in PATH. Add to ~/.zshrc: export PATH=\"$$HOME/.local/bin:$$PATH\"" ;; esac

test:
	go test ./...

lint:
	go vet ./...
	gofmt -l . | (! grep .)

clean:
	rm -rf bin
```

- [ ] **Step 4: Create `README.md` skeleton**

```markdown
# ccs — Claude Code Sessions

Browse all your Claude Code sessions across projects from any terminal. Fuzzy
search by topic, pin favorites, hide noise. Press Enter and the resumed
session replaces the TUI in the same window.

## Install

    make build
    make install

Make sure `~/.local/bin` is in your `PATH`.

## Usage

Just run `ccs` anywhere. See `?` for hotkeys.
```

- [ ] **Step 5: Commit**

```bash
git add go.mod .gitignore Makefile README.md
git commit -m "chore: bootstrap project structure"
```

---

## Task 2: Paths package — decode folder names

**Files:**
- Create: `internal/paths/paths.go`, `internal/paths/paths_test.go`

Heuristic note: Claude Code encodes paths as `-` separated segments. We can't
perfectly recover original paths because real folder names may contain `-`
(e.g. `react-dashboard-app`). This decoder is a **best-effort fallback** —
preferred source is `cwd` from the JSONL itself. The decoder works for
display, the JSONL's `cwd` for resume.

- [ ] **Step 1: Write failing tests**

`internal/paths/paths_test.go`:
```go
package paths

import "testing"

func TestDecode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"-Users-alice", "/Users/alice"},
		{"-Users-alice-Documents-myproject", "/Users/alice/Documents/myproject"},
		{"-Users-alice-Documents-multi-word-project", "/Users/alice/Documents/multi/word/project"},
		{"", ""},
		{"no-leading-dash", "no/leading/dash"},
	}
	for _, c := range cases {
		got := Decode(c.in)
		if got != c.want {
			t.Errorf("Decode(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}

func TestEncode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/Users/alice", "-Users-alice"},
		{"/Users/alice/Documents/myproject", "-Users-alice-Documents-myproject"},
	}
	for _, c := range cases {
		got := Encode(c.in)
		if got != c.want {
			t.Errorf("Encode(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/paths/`
Expected: FAIL (Decode/Encode undefined)

- [ ] **Step 3: Implement**

`internal/paths/paths.go`:
```go
// Package paths decodes the encoded project folder names that Claude Code
// uses under ~/.claude/projects/. The encoding is lossy (real "-" in paths
// become indistinguishable from separators), so Decode is best-effort and
// callers should prefer the cwd field from the JSONL itself when available.
package paths

import "strings"

func Decode(encoded string) string {
	if encoded == "" {
		return ""
	}
	if strings.HasPrefix(encoded, "-") {
		return "/" + strings.ReplaceAll(encoded[1:], "-", "/")
	}
	return strings.ReplaceAll(encoded, "-", "/")
}

func Encode(path string) string {
	return strings.ReplaceAll(path, "/", "-")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/paths/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/paths/
git commit -m "feat(paths): decode/encode project folder names"
```

---

## Task 3: Session type — shared domain object

**Files:**
- Create: `internal/session/session.go`

- [ ] **Step 1: Create the type**

`internal/session/session.go`:
```go
// Package session defines the domain object shared across packages.
package session

import "time"

// Session is one Claude Code chat as displayed in ccs.
// Fields originate from three sources:
//   - Scanner/Parser (filesystem + JSONL): UUID, ProjectPath, Cwd, GitBranch,
//     Title (from JSONL), LastActivity, MsgCount, Mtime, FilePath
//   - User meta (meta.json): Pinned, Tags, Hidden, CustomTitle, Notes
//   - Computed: DisplayTitle (CustomTitle if set, else Title)
type Session struct {
	UUID         string
	ProjectPath  string    // decoded from folder name, may differ from Cwd
	Cwd          string    // from JSONL; what we chdir into before resume
	GitBranch    string
	Title        string    // first real user message (or "(no user message)")
	LastActivity time.Time
	MsgCount     int
	Mtime        time.Time
	FilePath     string    // absolute path to the .jsonl

	Pinned      bool
	Tags        []string
	Hidden      bool
	CustomTitle string
	Notes       string

	Missing bool // true if Cwd no longer exists on disk
}

func (s Session) DisplayTitle() string {
	if s.CustomTitle != "" {
		return s.CustomTitle
	}
	return s.Title
}

func (s Session) Key() string {
	return s.ProjectPath + "::" + s.UUID
}
```

- [ ] **Step 2: Verify compiles**

Run: `go build ./internal/session/`
Expected: no output (success)

- [ ] **Step 3: Commit**

```bash
git add internal/session/
git commit -m "feat(session): shared Session type"
```

---

## Task 4: Parser — fixtures

**Files:**
- Create: `testdata/normal.jsonl`, `testdata/empty.jsonl`, `testdata/no_user.jsonl`,
  `testdata/system_only.jsonl`, `testdata/partial_corrupt.jsonl`,
  `testdata/wrapped_content.jsonl`

- [ ] **Step 1: Create `testdata/normal.jsonl`**

```jsonl
{"type":"custom-title","customTitle":"Cascade merge investigation","sessionId":"abc"}
{"type":"user","isMeta":false,"isSidechain":false,"userType":"external","cwd":"/Users/alice/Documents/example-project","gitBranch":"main","message":{"role":"user","content":"<command-name>/effort</command-name>"},"timestamp":"2026-05-15T14:20:00.000Z"}
{"type":"user","isMeta":false,"isSidechain":false,"userType":"external","cwd":"/Users/alice/Documents/example-project","gitBranch":"main","message":{"role":"user","content":"<local-command-stdout>Set effort level to max</local-command-stdout>"},"timestamp":"2026-05-15T14:20:01.000Z"}
{"type":"user","isMeta":false,"isSidechain":false,"userType":"external","cwd":"/Users/alice/Documents/example-project","gitBranch":"main","message":{"role":"user","content":"How does cascade merge work in GitLab?"},"timestamp":"2026-05-15T14:23:00.000Z"}
{"type":"assistant","message":{"role":"assistant","content":"Cascade merge is..."},"timestamp":"2026-05-15T14:23:45.000Z"}
{"type":"user","isMeta":false,"isSidechain":false,"userType":"external","message":{"role":"user","content":"Got it, thanks"},"timestamp":"2026-05-15T14:25:00.000Z"}
```

- [ ] **Step 2: Create `testdata/empty.jsonl`** — empty file (0 bytes)

```bash
: > testdata/empty.jsonl
```

- [ ] **Step 3: Create `testdata/no_user.jsonl`**

```jsonl
{"type":"custom-title","customTitle":"only-system"}
{"type":"system","subtype":"init","timestamp":"2026-05-01T10:00:00.000Z"}
```

- [ ] **Step 4: Create `testdata/system_only.jsonl`** — user messages all wrapped

```jsonl
{"type":"custom-title","customTitle":"wrappers-only"}
{"type":"user","isMeta":false,"userType":"external","message":{"role":"user","content":"<command-name>/clear</command-name>"},"timestamp":"2026-05-01T10:00:00.000Z"}
{"type":"user","isMeta":true,"userType":"external","message":{"role":"user","content":"this is meta, should skip"},"timestamp":"2026-05-01T10:00:01.000Z"}
{"type":"user","isMeta":false,"isSidechain":true,"userType":"external","message":{"role":"user","content":"sidechain, skip"},"timestamp":"2026-05-01T10:00:02.000Z"}
```

- [ ] **Step 5: Create `testdata/partial_corrupt.jsonl`**

```jsonl
{"type":"custom-title","customTitle":"survives-corruption"}
not valid json {{{
{"type":"user","isMeta":false,"isSidechain":false,"userType":"external","cwd":"/tmp/proj","message":{"role":"user","content":"this should be the title"},"timestamp":"2026-05-02T10:00:00.000Z"}
```

- [ ] **Step 6: Create `testdata/wrapped_content.jsonl`** — content is array of objects

```jsonl
{"type":"user","isMeta":false,"isSidechain":false,"userType":"external","cwd":"/tmp/p","message":{"role":"user","content":[{"type":"text","text":"Hello from array content"}]},"timestamp":"2026-05-03T10:00:00.000Z"}
```

- [ ] **Step 7: Commit fixtures**

```bash
git add testdata/
git commit -m "test(parser): jsonl fixtures"
```

---

## Task 5: Parser — implementation

**Files:**
- Create: `internal/parser/parser.go`, `internal/parser/parser_test.go`

- [ ] **Step 1: Write failing tests**

`internal/parser/parser_test.go`:
```go
package parser

import (
	"strings"
	"testing"
)

func TestParseNormal(t *testing.T) {
	r, err := ParseFile("../../testdata/normal.jsonl")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if r.CustomTitle != "Cascade merge investigation" {
		t.Errorf("CustomTitle=%q", r.CustomTitle)
	}
	if !strings.HasPrefix(r.FirstUserMsg, "How does cascade merge") {
		t.Errorf("FirstUserMsg=%q", r.FirstUserMsg)
	}
	if r.Cwd != "/Users/alice/Documents/example-project" {
		t.Errorf("Cwd=%q", r.Cwd)
	}
	if r.GitBranch != "main" {
		t.Errorf("GitBranch=%q", r.GitBranch)
	}
	if r.MsgCount < 4 {
		t.Errorf("MsgCount=%d, want >=4", r.MsgCount)
	}
	if r.LastTimestamp == "" {
		t.Errorf("LastTimestamp empty")
	}
}

func TestParseEmpty(t *testing.T) {
	r, err := ParseFile("../../testdata/empty.jsonl")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if r.FirstUserMsg != "" || r.CustomTitle != "" || r.MsgCount != 0 {
		t.Errorf("expected zero values, got %+v", r)
	}
}

func TestParseNoUser(t *testing.T) {
	r, err := ParseFile("../../testdata/no_user.jsonl")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if r.CustomTitle != "only-system" {
		t.Errorf("CustomTitle=%q", r.CustomTitle)
	}
	if r.FirstUserMsg != "" {
		t.Errorf("FirstUserMsg=%q, want empty", r.FirstUserMsg)
	}
}

func TestParseSystemOnly_skipsMetaSidechainWrappers(t *testing.T) {
	r, err := ParseFile("../../testdata/system_only.jsonl")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if r.FirstUserMsg != "" {
		t.Errorf("FirstUserMsg=%q, want empty (all should be skipped)", r.FirstUserMsg)
	}
}

func TestParsePartialCorrupt(t *testing.T) {
	r, err := ParseFile("../../testdata/partial_corrupt.jsonl")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if r.FirstUserMsg != "this should be the title" {
		t.Errorf("FirstUserMsg=%q", r.FirstUserMsg)
	}
}

func TestParseWrappedContent(t *testing.T) {
	r, err := ParseFile("../../testdata/wrapped_content.jsonl")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if r.FirstUserMsg != "Hello from array content" {
		t.Errorf("FirstUserMsg=%q", r.FirstUserMsg)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/parser/`
Expected: FAIL (parser package empty)

- [ ] **Step 3: Implement**

`internal/parser/parser.go`:
```go
// Package parser streams a Claude Code session .jsonl file and extracts
// metadata needed for the ccs index: custom title, first real user message,
// cwd, git branch, message count, and last activity timestamp.
//
// "Real" user message = type:"user" with isMeta=false, isSidechain=false,
// non-empty content that isn't wrapped in <command-name>, <local-command-*>,
// or <system-reminder> tags.
package parser

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
)

const (
	maxLineSize    = 16 * 1024 * 1024 // 16 MiB: some lines (e.g. file snapshots) are large
	titleMaxLength = 200              // raw cap; UI truncates further for display
)

type Result struct {
	CustomTitle   string
	FirstUserMsg  string
	Cwd           string
	GitBranch     string
	LastTimestamp string
	MsgCount      int
}

type rawLine struct {
	Type         string          `json:"type"`
	CustomTitle  string          `json:"customTitle"`
	IsMeta       bool            `json:"isMeta"`
	IsSidechain  bool            `json:"isSidechain"`
	UserType     string          `json:"userType"`
	Cwd          string          `json:"cwd"`
	GitBranch    string          `json:"gitBranch"`
	Timestamp    string          `json:"timestamp"`
	Message      json.RawMessage `json:"message"`
}

type rawMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func ParseFile(path string) (Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	return parse(f)
}

func parse(r io.Reader) (Result, error) {
	var res Result
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), maxLineSize)
	for sc.Scan() {
		var rl rawLine
		if err := json.Unmarshal(sc.Bytes(), &rl); err != nil {
			continue // skip malformed lines
		}
		if rl.Timestamp != "" {
			res.LastTimestamp = rl.Timestamp
		}
		switch rl.Type {
		case "custom-title":
			if rl.CustomTitle != "" {
				res.CustomTitle = rl.CustomTitle
			}
		case "user":
			res.MsgCount++
			if rl.IsMeta || rl.IsSidechain {
				continue
			}
			if res.FirstUserMsg != "" {
				continue
			}
			text := extractText(rl.Message)
			if text == "" || isWrapperText(text) {
				continue
			}
			if len(text) > titleMaxLength {
				text = text[:titleMaxLength]
			}
			res.FirstUserMsg = text
			if rl.Cwd != "" {
				res.Cwd = rl.Cwd
			}
			if rl.GitBranch != "" {
				res.GitBranch = rl.GitBranch
			}
		case "assistant":
			res.MsgCount++
		}
		// Capture cwd/gitBranch from any record that has them, as fallback
		if res.Cwd == "" && rl.Cwd != "" {
			res.Cwd = rl.Cwd
		}
		if res.GitBranch == "" && rl.GitBranch != "" {
			res.GitBranch = rl.GitBranch
		}
	}
	if err := sc.Err(); err != nil {
		return res, err
	}
	return res, nil
}

func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m rawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	// content may be string or array of blocks
	if len(m.Content) == 0 {
		return ""
	}
	if m.Content[0] == '"' {
		var s string
		if err := json.Unmarshal(m.Content, &s); err == nil {
			return s
		}
		return ""
	}
	if m.Content[0] == '[' {
		var blocks []contentBlock
		if err := json.Unmarshal(m.Content, &blocks); err != nil {
			return ""
		}
		parts := make([]string, 0, len(blocks))
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// isWrapperText returns true for command/system-tag wrappers that aren't real
// user input.
func isWrapperText(s string) bool {
	t := strings.TrimSpace(s)
	prefixes := []string{
		"<command-name>",
		"<command-message>",
		"<local-command-stdout>",
		"<local-command-stderr>",
		"<local-command-caveat>",
		"<system-reminder>",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/parser/ -v`
Expected: PASS for all six tests

- [ ] **Step 5: Commit**

```bash
git add internal/parser/
git commit -m "feat(parser): stream jsonl and extract session metadata"
```

---

## Task 6: Cache — load/save index.json

**Files:**
- Create: `internal/cache/cache.go`, `internal/cache/cache_test.go`

- [ ] **Step 1: Write failing tests**

`internal/cache/cache_test.go`:
```go
package cache

import (
	"os"
	"path/filepath"
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
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file leftover: %s", e.Name())
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cache/`
Expected: FAIL (cache package empty)

- [ ] **Step 3: Implement**

`internal/cache/cache.go`:
```go
// Package cache persists parsed JSONL metadata so ccs doesn't re-scan
// unchanged session files on every launch. Inval idation is by mtime — the
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
		// Corrupt — reset rather than fail. The scanner will rebuild.
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cache/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cache/
git commit -m "feat(cache): persist session metadata to index.json"
```

---

## Task 7: Meta store — pin/hide with flock

**Files:**
- Create: `internal/meta/store.go`, `internal/meta/store_test.go`

- [ ] **Step 1: Add `gofrs/flock` dependency**

Run:
```bash
go get github.com/gofrs/flock
```

- [ ] **Step 2: Write failing tests**

`internal/meta/store_test.go`:
```go
package meta

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestPinUnpin(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "meta.json"))
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	s.SetPinned("k1", true)
	if !s.Get("k1").Pinned {
		t.Error("expected pinned")
	}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s2 := New(filepath.Join(dir, "meta.json"))
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}
	if !s2.Get("k1").Pinned {
		t.Error("expected persisted pin")
	}
}

func TestHide(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "meta.json"))
	s.Load()
	s.SetHidden("k", true)
	if !s.Get("k").Hidden {
		t.Error("expected hidden")
	}
}

func TestConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.json")
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := New(path)
			s.Load()
			s.SetPinned("shared", true)
			s.Save()
		}(i)
	}
	wg.Wait()
	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatal(err)
	}
	if !s.Get("shared").Pinned {
		t.Error("expected pinned after concurrent writes")
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/meta/`
Expected: FAIL

- [ ] **Step 4: Implement**

`internal/meta/store.go`:
```go
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
	s.mu.RLock()
	f := file{Version: schemaVersion, Entries: s.entries}
	s.mu.RUnlock()
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	lock := flock.New(s.path + ".lock")
	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()
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

func (s *Store) SetPinned(key string, v bool)        { s.update(key, func(e *Entry) { e.Pinned = v }) }
func (s *Store) SetHidden(key string, v bool)        { s.update(key, func(e *Entry) { e.Hidden = v }) }
func (s *Store) SetCustomTitle(key, title string)    { s.update(key, func(e *Entry) { e.CustomTitle = title }) }
func (s *Store) SetTags(key string, tags []string)   { s.update(key, func(e *Entry) { e.Tags = tags }) }
func (s *Store) SetNotes(key, notes string)          { s.update(key, func(e *Entry) { e.Notes = notes }) }

func (s *Store) update(key string, fn func(*Entry)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entries[key]
	fn(&e)
	s.entries[key] = e
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/meta/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/meta/ go.mod go.sum
git commit -m "feat(meta): persist user pins/hides/tags with file lock"
```

---

## Task 8: Config — TOML with defaults

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`

- [ ] **Step 1: Add toml dependency**

Run:
```bash
go get github.com/BurntSushi/toml
```

- [ ] **Step 2: Write failing tests**

`internal/config/config_test.go`:
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	c := Default()
	if c.PermissionMode != "dangerously-skip" {
		t.Errorf("PermissionMode=%q", c.PermissionMode)
	}
	if c.MaxTitleLength != 80 {
		t.Errorf("MaxTitleLength=%d", c.MaxTitleLength)
	}
}

func TestLoadMissingCreatesDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.PermissionMode != "dangerously-skip" {
		t.Errorf("default not applied")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected config file written, got %v", err)
	}
}

func TestLoadExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`permission_mode = "accept-edits"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.PermissionMode != "accept-edits" {
		t.Errorf("PermissionMode=%q", c.PermissionMode)
	}
	if c.MaxTitleLength != 80 {
		t.Errorf("expected default MaxTitleLength fallback, got %d", c.MaxTitleLength)
	}
}
```

- [ ] **Step 3: Implement**

`internal/config/config.go`:
```go
// Package config loads ~/.claude/cc-sessions/config.toml, falling back to
// defaults when fields are missing or the file does not exist.
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	PermissionMode string `toml:"permission_mode"` // "dangerously-skip" | "accept-edits" | "default"
	ShowHidden     bool   `toml:"show_hidden"`
	MaxTitleLength int    `toml:"max_title_length"`
	DateFormat     string `toml:"date_format"`
}

func Default() Config {
	return Config{
		PermissionMode: "dangerously-skip",
		ShowHidden:     false,
		MaxTitleLength: 80,
		DateFormat:     "2006-01-02 15:04",
	}
}

func Load(path string) (Config, error) {
	c := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, write(path, c)
	}
	if err != nil {
		return c, err
	}
	var parsed Config
	if _, err := toml.Decode(string(data), &parsed); err != nil {
		return c, err
	}
	// Overlay: only override defaults where parsed is non-zero
	if parsed.PermissionMode != "" {
		c.PermissionMode = parsed.PermissionMode
	}
	c.ShowHidden = parsed.ShowHidden
	if parsed.MaxTitleLength > 0 {
		c.MaxTitleLength = parsed.MaxTitleLength
	}
	if parsed.DateFormat != "" {
		c.DateFormat = parsed.DateFormat
	}
	return c, nil
}

func write(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/config/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat(config): toml config with defaults"
```

---

## Task 9: Scanner — walk filesystem and index

**Files:**
- Create: `internal/scanner/scanner.go`, `internal/scanner/scanner_test.go`

- [ ] **Step 1: Write failing tests**

`internal/scanner/scanner_test.go`:
```go
package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dzaurov/cc-sessions/internal/cache"
)

func TestScanFindsJsonl(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "-Users-test-Documents-proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	// Copy fixture
	src, err := os.ReadFile("../../testdata/normal.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	jsonlPath := filepath.Join(proj, "abc-uuid.jsonl")
	if err := os.WriteFile(jsonlPath, src, 0o644); err != nil {
		t.Fatal(err)
	}
	// Also drop a noise file and a subdirectory to ensure they're ignored
	os.WriteFile(filepath.Join(proj, "abc-uuid"), []byte("garbage"), 0o644)
	os.MkdirAll(filepath.Join(proj, "memory"), 0o755)
	os.WriteFile(filepath.Join(proj, "memory", "note.md"), []byte("x"), 0o644)

	c := cache.New(filepath.Join(t.TempDir(), "idx.json"))
	c.Load()
	s := New(root, c)
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
	s := New(root, c)
	first, _ := s.Scan()
	// Mutate the cached title; if Scan re-parses, this gets overwritten.
	e := first[0]
	e.Title = "CACHED MARKER"
	c.Set(e)

	second, _ := s.Scan()
	if len(second) != 1 || second[0].Title != "CACHED MARKER" {
		t.Errorf("expected cached title, got %q", second[0].Title)
	}
}

func TestScanRepasesWhenMtimeNewer(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "-Users-test-proj")
	os.MkdirAll(proj, 0o755)
	src, _ := os.ReadFile("../../testdata/normal.jsonl")
	jsonlPath := filepath.Join(proj, "u1.jsonl")
	os.WriteFile(jsonlPath, src, 0o644)

	c := cache.New(filepath.Join(t.TempDir(), "idx.json"))
	c.Load()
	s := New(root, c)
	s.Scan()

	// Touch file: bump mtime by writing again
	os.WriteFile(jsonlPath, src, 0o644)
	future := mustParseTime(t, "2099-01-01T00:00:00Z")
	os.Chtimes(jsonlPath, future, future)

	// Plant a fake cached title; scanner should overwrite it
	got, _ := c.Get("/Users/test/proj::u1")
	got.Title = "STALE"
	c.Set(got)

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
	s := New(root, c)
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
```

Helper at the bottom of `scanner_test.go`:
```go
func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
```

Add `import "time"` to the test file.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/scanner/`
Expected: FAIL

- [ ] **Step 3: Implement**

`internal/scanner/scanner.go`:
```go
// Package scanner walks ~/.claude/projects/ and produces cache entries for
// every session .jsonl, using the cache to skip files whose mtime hasn't
// changed since the last scan.
package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dzaurov/cc-sessions/internal/cache"
	"github.com/dzaurov/cc-sessions/internal/parser"
	"github.com/dzaurov/cc-sessions/internal/paths"
)

type Scanner struct {
	root  string
	cache *cache.Cache
}

func New(root string, c *cache.Cache) *Scanner {
	return &Scanner{root: root, cache: c}
}

func (s *Scanner) Scan() ([]cache.Entry, error) {
	projects, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var out []cache.Entry

	for _, pd := range projects {
		if !pd.IsDir() {
			continue
		}
		projDir := filepath.Join(s.root, pd.Name())
		projectPath := paths.Decode(pd.Name())

		files, err := os.ReadDir(projDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			uuid := strings.TrimSuffix(f.Name(), ".jsonl")
			filePath := filepath.Join(projDir, f.Name())
			info, err := f.Info()
			if err != nil {
				continue
			}
			key := projectPath + "::" + uuid
			seen[key] = struct{}{}

			cached, hit := s.cache.Get(key)
			if hit && !info.ModTime().After(cached.Mtime) {
				out = append(out, cached)
				continue
			}

			res, err := parser.ParseFile(filePath)
			if err != nil {
				// Keep going — log path could be plumbed here later
				continue
			}
			entry := cache.Entry{
				Key:          key,
				UUID:         uuid,
				ProjectPath:  projectPath,
				Title:        res.FirstUserMsg,
				CustomTitle:  res.CustomTitle,
				LastActivity: parseTimestamp(res.LastTimestamp, info.ModTime()),
				Mtime:        info.ModTime(),
				MsgCount:     res.MsgCount,
				Cwd:          firstNonEmpty(res.Cwd, projectPath),
				GitBranch:    res.GitBranch,
				FilePath:     filePath,
			}
			if entry.Title == "" && entry.CustomTitle == "" {
				entry.Title = "(no user message)"
			}
			s.cache.Set(entry)
			out = append(out, entry)
		}
	}

	// Evict entries for files that disappeared
	for _, e := range s.cache.All() {
		if _, ok := seen[e.Key]; !ok {
			s.cache.Delete(e.Key)
		}
	}
	return out, nil
}

func parseTimestamp(s string, fallback time.Time) time.Time {
	if s == "" {
		return fallback
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return fallback
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/scanner/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/scanner/
git commit -m "feat(scanner): walk ~/.claude/projects with cache-aware reparse"
```

---

## Task 10: Launcher — build claude resume command

**Files:**
- Create: `internal/launcher/launcher.go`, `internal/launcher/launcher_test.go`

We split the launcher into a pure function `BuildArgs` (testable) and `Exec`
(does the chdir + syscall.Exec).

- [ ] **Step 1: Write failing tests**

`internal/launcher/launcher_test.go`:
```go
package launcher

import (
	"reflect"
	"testing"
)

func TestBuildArgs_dangerouslySkip(t *testing.T) {
	got := BuildArgs("u1", "dangerously-skip")
	want := []string{"claude", "--resume", "u1", "--dangerously-skip-permissions"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuildArgs_acceptEdits(t *testing.T) {
	got := BuildArgs("u1", "accept-edits")
	want := []string{"claude", "--resume", "u1", "--permission-mode", "acceptEdits"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuildArgs_default(t *testing.T) {
	got := BuildArgs("u1", "default")
	want := []string{"claude", "--resume", "u1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestBuildArgs_unknownFallsBackToDefault(t *testing.T) {
	got := BuildArgs("u1", "wat")
	want := []string{"claude", "--resume", "u1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: Run to verify fail**

Run: `go test ./internal/launcher/`
Expected: FAIL

- [ ] **Step 3: Implement**

`internal/launcher/launcher.go`:
```go
// Package launcher constructs and executes the `claude --resume <uuid>`
// invocation. Exec replaces the current process so the TUI vanishes and the
// resumed chat takes over the same terminal window.
package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func BuildArgs(uuid, mode string) []string {
	args := []string{"claude", "--resume", uuid}
	switch mode {
	case "dangerously-skip":
		args = append(args, "--dangerously-skip-permissions")
	case "accept-edits":
		args = append(args, "--permission-mode", "acceptEdits")
	}
	return args
}

func Exec(cwd, uuid, mode string) error {
	if cwd != "" {
		if _, err := os.Stat(cwd); err != nil {
			return fmt.Errorf("cwd %q not accessible: %w", cwd, err)
		}
		if err := os.Chdir(cwd); err != nil {
			return err
		}
	}
	bin, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found in PATH: %w", err)
	}
	args := BuildArgs(uuid, mode)
	return syscall.Exec(bin, args, os.Environ())
}
```

- [ ] **Step 4: Run tests pass**

Run: `go test ./internal/launcher/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/launcher/
git commit -m "feat(launcher): build resume args and exec claude in cwd"
```

---

## Task 11: TUI keys + help

**Files:**
- Create: `internal/tui/keys.go`

- [ ] **Step 1: Implement keymap**

`internal/tui/keys.go`:
```go
package tui

import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
	Up         key.Binding
	Down       key.Binding
	Enter      key.Binding
	Search     key.Binding
	Escape     key.Binding
	Pin        key.Binding
	Hide       key.Binding
	ToggleHide key.Binding
	Rescan     key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func DefaultKeys() KeyMap {
	return KeyMap{
		Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "resume")),
		Search:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Escape:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear")),
		Pin:        key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pin")),
		Hide:       key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hide")),
		ToggleHide: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "show hidden")),
		Rescan:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rescan")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Search, k.Pin, k.Hide, k.Rescan, k.Help, k.Quit}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Escape},
		{k.Search, k.Pin, k.Hide, k.ToggleHide, k.Rescan},
		{k.Help, k.Quit},
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/tui/keys.go
git commit -m "feat(tui): default keymap"
```

---

## Task 12: TUI model — render and navigate

**Files:**
- Create: `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/model_test.go`

- [ ] **Step 1: Add bubbletea deps**

Run:
```bash
go get github.com/charmbracelet/bubbletea github.com/charmbracelet/bubbles github.com/charmbracelet/lipgloss github.com/sahilm/fuzzy
```

- [ ] **Step 2: Implement model**

`internal/tui/model.go`:
```go
// Package tui is the bubbletea Model for ccs.
package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"

	"github.com/dzaurov/cc-sessions/internal/cache"
	"github.com/dzaurov/cc-sessions/internal/config"
	"github.com/dzaurov/cc-sessions/internal/meta"
	"github.com/dzaurov/cc-sessions/internal/session"
)

type ResumeMsg struct {
	Cwd  string
	UUID string
	Mode string
}

type rescanFn func() ([]cache.Entry, error)

type Model struct {
	cfg        config.Config
	meta       *meta.Store
	rescan     rescanFn
	all        []session.Session // unfiltered (already hidden-applied)
	filtered   []session.Session // after search
	cursor     int
	search     textinput.Model
	searchMode bool
	showHidden bool
	helpView   help.Model
	showHelp   bool
	width      int
	height     int
	err        error
	keys       KeyMap
}

func New(cfg config.Config, m *meta.Store, entries []cache.Entry, rescan rescanFn) Model {
	ti := textinput.New()
	ti.Placeholder = "fuzzy search…"
	ti.Prompt = "/ "
	ti.CharLimit = 256
	mdl := Model{
		cfg:        cfg,
		meta:       m,
		rescan:     rescan,
		search:     ti,
		helpView:   help.New(),
		keys:       DefaultKeys(),
		showHidden: cfg.ShowHidden,
	}
	mdl.setEntries(entries)
	return mdl
}

func (m *Model) setEntries(entries []cache.Entry) {
	all := make([]session.Session, 0, len(entries))
	for _, e := range entries {
		ent := m.meta.Get(e.Key)
		s := session.Session{
			UUID:         e.UUID,
			ProjectPath:  e.ProjectPath,
			Cwd:          e.Cwd,
			GitBranch:    e.GitBranch,
			Title:        e.Title,
			LastActivity: e.LastActivity,
			MsgCount:     e.MsgCount,
			Mtime:        e.Mtime,
			FilePath:     e.FilePath,
			CustomTitle:  pickCustomTitle(ent.CustomTitle, e.CustomTitle),
			Pinned:       ent.Pinned,
			Tags:         ent.Tags,
			Hidden:       ent.Hidden,
			Notes:        ent.Notes,
		}
		// Mark missing if cwd no longer exists
		// (cheap stat — fine at startup)
		s.Missing = !pathExists(s.Cwd)
		all = append(all, s)
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Pinned != all[j].Pinned {
			return all[i].Pinned
		}
		return all[i].LastActivity.After(all[j].LastActivity)
	})
	m.all = all
	m.applyFilter()
}

func pickCustomTitle(userOverride, jsonlTitle string) string {
	if userOverride != "" {
		return userOverride
	}
	// jsonlTitle is e.g. just the project name; only useful when no Title.
	return jsonlTitle
}

func (m *Model) applyFilter() {
	visible := make([]session.Session, 0, len(m.all))
	for _, s := range m.all {
		if !m.showHidden && s.Hidden {
			continue
		}
		visible = append(visible, s)
	}
	q := strings.TrimSpace(m.search.Value())
	if q == "" {
		m.filtered = visible
		m.clampCursor()
		return
	}
	candidates := make([]string, len(visible))
	for i, s := range visible {
		candidates[i] = s.DisplayTitle() + " " + s.ProjectPath + " " + strings.Join(s.Tags, " ")
	}
	matches := fuzzy.Find(q, candidates)
	out := make([]session.Session, 0, len(matches))
	for _, mm := range matches {
		out = append(out, visible[mm.Index])
	}
	m.filtered = out
	m.clampCursor()
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.helpView.Width = msg.Width
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.searchMode {
		switch {
		case keyMatches(msg, m.keys.Escape):
			m.searchMode = false
			m.search.Blur()
			m.search.SetValue("")
			m.applyFilter()
			return m, nil
		case msg.Type == tea.KeyEnter:
			m.searchMode = false
			m.search.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		m.applyFilter()
		return m, cmd
	}

	switch {
	case keyMatches(msg, m.keys.Quit):
		return m, tea.Quit
	case keyMatches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		return m, nil
	case keyMatches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case keyMatches(msg, m.keys.Down):
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case keyMatches(msg, m.keys.Search):
		m.searchMode = true
		m.search.Focus()
		return m, nil
	case keyMatches(msg, m.keys.Pin):
		if s, ok := m.current(); ok {
			m.meta.SetPinned(sessionKey(s), !s.Pinned)
			m.meta.Save()
			m.refreshFromMeta()
		}
	case keyMatches(msg, m.keys.Hide):
		if s, ok := m.current(); ok {
			m.meta.SetHidden(sessionKey(s), !s.Hidden)
			m.meta.Save()
			m.refreshFromMeta()
		}
	case keyMatches(msg, m.keys.ToggleHide):
		m.showHidden = !m.showHidden
		m.applyFilter()
	case keyMatches(msg, m.keys.Rescan):
		entries, err := m.rescan()
		if err != nil {
			m.err = err
		} else {
			m.setEntries(entries)
		}
	case keyMatches(msg, m.keys.Enter):
		if s, ok := m.current(); ok && !s.Missing {
			return m, func() tea.Msg {
				return ResumeMsg{Cwd: s.Cwd, UUID: s.UUID, Mode: m.cfg.PermissionMode}
			}
		}
	}
	return m, nil
}

func (m *Model) refreshFromMeta() {
	for i := range m.all {
		e := m.meta.Get(sessionKey(m.all[i]))
		m.all[i].Pinned = e.Pinned
		m.all[i].Hidden = e.Hidden
		m.all[i].Tags = e.Tags
		m.all[i].Notes = e.Notes
		if e.CustomTitle != "" {
			m.all[i].CustomTitle = e.CustomTitle
		}
	}
	sort.SliceStable(m.all, func(i, j int) bool {
		if m.all[i].Pinned != m.all[j].Pinned {
			return m.all[i].Pinned
		}
		return m.all[i].LastActivity.After(m.all[j].LastActivity)
	})
	m.applyFilter()
}

func (m Model) current() (session.Session, bool) {
	if len(m.filtered) == 0 {
		return session.Session{}, false
	}
	return m.filtered[m.cursor], true
}

func sessionKey(s session.Session) string {
	return s.ProjectPath + "::" + s.UUID
}

func pathExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := osStat(p)
	return err == nil
}

// indirection so tests can stub if they want; default is os.Stat
var osStat = func(p string) (interface{}, error) {
	return nil, fmt.Errorf("stub: %s", p)
}
```

Wait — `osStat` indirection is overkill and breaks. Use `os.Stat` directly:

Replace the bottom of `model.go` with:
```go
func pathExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}
```

And add `"os"` to imports.

Helper `keyMatches`:
```go
func keyMatches(msg tea.KeyMsg, b key.Binding) bool {
	return key.Matches(msg, b)
}
```

Add to imports: `"github.com/charmbracelet/bubbles/key"`.

- [ ] **Step 3: Implement view**

`internal/tui/view.go`:
```go
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/dzaurov/cc-sessions/internal/session"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	pinnedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	selectedStyle = lipgloss.NewStyle().Reverse(true)
	missingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func (m Model) View() string {
	var b strings.Builder
	header := titleStyle.Render("cc-sessions")
	count := dimStyle.Render(fmt.Sprintf("  %d sessions", len(m.filtered)))
	b.WriteString(header + count + "\n")

	if m.searchMode || m.search.Value() != "" {
		b.WriteString(m.search.View() + "\n")
	}
	if m.err != nil {
		b.WriteString(errorStyle.Render("error: "+m.err.Error()) + "\n")
	}

	if len(m.filtered) == 0 {
		b.WriteString(dimStyle.Render("\n  (no sessions match)\n"))
	}

	for i, s := range m.filtered {
		row := formatRow(s, m.cfg.DateFormat, m.cfg.MaxTitleLength)
		if i == m.cursor {
			row = selectedStyle.Render(row)
		}
		b.WriteString(row + "\n")
	}

	if m.showHelp {
		b.WriteString("\n" + m.helpView.View(m.keys))
	} else {
		b.WriteString("\n" + dimStyle.Render(m.helpView.ShortHelpView(m.keys.ShortHelp())))
	}
	return b.String()
}

func formatRow(s session.Session, dateFmt string, maxTitle int) string {
	pin := "   "
	if s.Pinned {
		pin = pinnedStyle.Render(" ★ ")
	}
	when := s.LastActivity.Local().Format(dateFmt)
	proj := projectShortName(s.ProjectPath)
	title := s.DisplayTitle()
	if maxTitle > 0 && len([]rune(title)) > maxTitle {
		title = string([]rune(title)[:maxTitle]) + "…"
	}
	flags := ""
	if s.Hidden {
		flags += dimStyle.Render(" [hidden]")
	}
	if s.Missing {
		flags += missingStyle.Render(" (missing)")
	}
	return fmt.Sprintf("%s%s  %-22s  %s%s",
		pin,
		dimStyle.Render(when),
		proj,
		title,
		flags,
	)
}

func projectShortName(p string) string {
	if p == "" {
		return "(unknown)"
	}
	parts := strings.Split(p, "/")
	return parts[len(parts)-1]
}
```

- [ ] **Step 4: Write a basic model test**

`internal/tui/model_test.go`:
```go
package tui

import (
	"testing"
	"time"

	"github.com/dzaurov/cc-sessions/internal/cache"
	"github.com/dzaurov/cc-sessions/internal/config"
	"github.com/dzaurov/cc-sessions/internal/meta"
)

func newTestModel(t *testing.T, entries []cache.Entry) Model {
	t.Helper()
	m := meta.New(t.TempDir() + "/meta.json")
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	return New(config.Default(), m, entries, func() ([]cache.Entry, error) { return entries, nil })
}

func TestSortPinnedFirstThenByDate(t *testing.T) {
	mdl := newTestModel(t, []cache.Entry{
		{Key: "a::1", UUID: "1", ProjectPath: "a", Title: "old", LastActivity: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Key: "b::2", UUID: "2", ProjectPath: "b", Title: "new", LastActivity: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
	})
	// pin the older
	mdl.meta.SetPinned("a::1", true)
	mdl.refreshFromMeta()
	if mdl.filtered[0].UUID != "1" {
		t.Errorf("expected pinned first, got %v", mdl.filtered[0].UUID)
	}
}

func TestFilterByFuzzyTitle(t *testing.T) {
	mdl := newTestModel(t, []cache.Entry{
		{Key: "a::1", UUID: "1", ProjectPath: "/x", Title: "Jenkins cascade merge", LastActivity: time.Now()},
		{Key: "a::2", UUID: "2", ProjectPath: "/x", Title: "Random stuff", LastActivity: time.Now()},
	})
	mdl.search.SetValue("jenkins")
	mdl.applyFilter()
	if len(mdl.filtered) != 1 || mdl.filtered[0].UUID != "1" {
		t.Errorf("expected 1 jenkins match, got %+v", mdl.filtered)
	}
}

func TestToggleHideExcludesHiddenByDefault(t *testing.T) {
	mdl := newTestModel(t, []cache.Entry{
		{Key: "a::1", UUID: "1", ProjectPath: "/x", Title: "keep", LastActivity: time.Now()},
		{Key: "a::2", UUID: "2", ProjectPath: "/x", Title: "trash", LastActivity: time.Now()},
	})
	mdl.meta.SetHidden("a::2", true)
	mdl.refreshFromMeta()
	if len(mdl.filtered) != 1 {
		t.Errorf("expected hidden filtered out, got %d", len(mdl.filtered))
	}
	mdl.showHidden = true
	mdl.applyFilter()
	if len(mdl.filtered) != 2 {
		t.Errorf("expected hidden included, got %d", len(mdl.filtered))
	}
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/tui/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tui/ go.mod go.sum
git commit -m "feat(tui): bubbletea model with search, pin, hide"
```

---

## Task 13: cmd/ccs/main.go — wire everything

**Files:**
- Create: `cmd/ccs/main.go`

- [ ] **Step 1: Implement entry point**

`cmd/ccs/main.go`:
```go
// Command ccs is the entry point: load config + meta, scan sessions, render
// the TUI; on ResumeMsg, persist meta and exec into claude.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dzaurov/cc-sessions/internal/cache"
	"github.com/dzaurov/cc-sessions/internal/config"
	"github.com/dzaurov/cc-sessions/internal/launcher"
	"github.com/dzaurov/cc-sessions/internal/meta"
	"github.com/dzaurov/cc-sessions/internal/scanner"
	"github.com/dzaurov/cc-sessions/internal/tui"
)

var (
	flagListJSON = flag.Bool("list-json", false, "print sessions as JSON and exit (for tests/scripting)")
)

func main() {
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ccs:", err)
		os.Exit(1)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	stateDir := filepath.Join(home, ".claude", "cc-sessions")
	projectsRoot := filepath.Join(home, ".claude", "projects")

	cfg, err := config.Load(filepath.Join(stateDir, "config.toml"))
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	c := cache.New(filepath.Join(stateDir, "index.json"))
	if err := c.Load(); err != nil {
		return fmt.Errorf("cache: %w", err)
	}
	mtaStore := meta.New(filepath.Join(stateDir, "meta.json"))
	if err := mtaStore.Load(); err != nil {
		return fmt.Errorf("meta: %w", err)
	}

	scn := scanner.New(projectsRoot, c)
	entries, err := scn.Scan()
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	if err := c.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "ccs: warning: cache save:", err)
	}

	if *flagListJSON {
		return json.NewEncoder(os.Stdout).Encode(entries)
	}

	rescan := func() ([]cache.Entry, error) {
		out, err := scn.Scan()
		if err != nil {
			return nil, err
		}
		_ = c.Save()
		return out, nil
	}

	model := tui.New(cfg, mtaStore, entries, rescan)
	prog := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := prog.Run()
	if err != nil {
		return err
	}
	if fm, ok := finalModel.(tui.Model); ok {
		_ = fm
	}

	// On exit, if the model emitted a ResumeMsg, the bubbletea program
	// quits without an explicit signal. We need a different approach:
	// listen for ResumeMsg inside the program by returning tea.Quit and
	// stashing the request. See refactor below.
	return nil
}
```

⚠ The straightforward bubbletea approach to "do X then quit" is:
1. Model returns `tea.Quit` from Update when ResumeMsg arrives, but first stashes the request on itself.
2. After `prog.Run()`, the caller inspects the final model and acts.

Let me revise the model + main accordingly.

- [ ] **Step 2: Update model to capture resume request**

Add to `Model` in `internal/tui/model.go`:
```go
type Model struct {
	// ...existing fields...
	pendingResume *ResumeMsg
}
```

In `handleKey` for the `Enter` case, replace:
```go
case keyMatches(msg, m.keys.Enter):
    if s, ok := m.current(); ok && !s.Missing {
        return m, func() tea.Msg {
            return ResumeMsg{Cwd: s.Cwd, UUID: s.UUID, Mode: m.cfg.PermissionMode}
        }
    }
```

With:
```go
case keyMatches(msg, m.keys.Enter):
    if s, ok := m.current(); ok && !s.Missing {
        rm := ResumeMsg{Cwd: s.Cwd, UUID: s.UUID, Mode: m.cfg.PermissionMode}
        m.pendingResume = &rm
        return m, tea.Quit
    }
```

Add accessor at the bottom of `model.go`:
```go
func (m Model) PendingResume() *ResumeMsg { return m.pendingResume }
```

- [ ] **Step 3: Update `cmd/ccs/main.go`**

Replace the tail of `run()` after `prog.Run()`:
```go
	finalModel, err := prog.Run()
	if err != nil {
		return err
	}
	fm, _ := finalModel.(tui.Model)
	if rm := fm.PendingResume(); rm != nil {
		// Exec replaces this process — no return on success.
		return launcher.Exec(rm.Cwd, rm.UUID, rm.Mode)
	}
	return nil
}
```

- [ ] **Step 4: Build**

Run: `make build`
Expected: builds without errors → `bin/ccs`

- [ ] **Step 5: Sanity-check JSON output on real data**

Run: `./bin/ccs --list-json | python3 -m json.tool | head -40`
Expected: JSON array of entries with titles, UUIDs, projects from your actual `~/.claude/projects/`.

- [ ] **Step 6: Commit**

```bash
git add cmd/
git commit -m "feat(cmd): wire scanner, tui, launcher"
```

---

## Task 14: Manual smoke test on real data

- [ ] **Step 1: Run TUI**

Run: `./bin/ccs`
Verify visually:
- Список появляется со всеми сессиями из 12 проектов
- Заголовки из первых user-сообщений (не UUID)
- Даты сортированы убыв
- Стрелки навигируют
- `/` фильтрует по подстроке (например `jenkins`, `audit`, `cors`)
- `p` ставит ★, после перезапуска ★ остаётся
- `h` прячет, `t` показывает скрытые, `h` возвращает
- `r` пересканирует
- `?` показывает справку
- `q` выходит

- [ ] **Step 2: Test resume**

В `ccs`, выбрать любую сессию, нажать `Enter`. Должно:
- TUI исчезнуть
- В том же терминале запуститься `claude --resume <uuid> --dangerously-skip-permissions`
- Сессия открыться в её исходной директории (проверить `pwd` после)
- Должен сохраниться контекст исходного чата

- [ ] **Step 3: Cleanup if needed**

If anything mis-behaves, document the issue, add a regression test, fix, recommit.

- [ ] **Step 4: Install**

Run: `make install`
Expected: symlink в `~/.local/bin/ccs` создан; `which ccs` показывает путь.

- [ ] **Step 5: Verify from random directory**

```bash
cd /tmp && ccs --list-json | jq 'length'
```
Expected: число > 0 (видит все сессии независимо от cwd).

- [ ] **Step 6: Commit any fixes from smoke test**

Если были правки — отдельный commit.

---

## Task 15: Polish README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Replace skeleton with full doc**

```markdown
# ccs — Claude Code Sessions

Браузер всех сессий Claude Code. Запускаешь `ccs` в любом терминале → fuzzy-
поиск по всем чатам всех проектов → `Enter` → текущий терминал
переключается в выбранную сессию.

## Зачем

Claude Code хранит сессии как `~/.claude/projects/<encoded-path>/<uuid>.jsonl`.
Имена сессий — UUID. Найти нужный чат среди десятков проектов невозможно.

`ccs` индексирует все сессии, показывает осмысленные заголовки (первое
user-сообщение каждой сессии), позволяет пинить важное и прятать мусор.

## Установка

```bash
cd ~/Documents/cc-sessions
make build
make install
```

Бинарь поставится симлинком в `~/.local/bin/ccs`. Убедись, что эта папка в
PATH:
```bash
echo $PATH | tr ':' '\n' | grep -F "$HOME/.local/bin" || \
  echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
```

## Использование

`ccs` — открыть TUI

Внутри TUI:
| Клавиша | Действие |
|---------|----------|
| `↑`/`↓` или `k`/`j` | Навигация |
| `Enter` | Резюмить сессию |
| `/` | Поиск (fuzzy) |
| `Esc` | Сброс поиска |
| `p` | Pin/unpin (★) |
| `h` | Скрыть/вернуть |
| `t` | Toggle отображения скрытых |
| `r` | Принудительный rescan |
| `?` | Полная справка |
| `q` или `Ctrl-C` | Выход |

## Конфигурация

`~/.claude/cc-sessions/config.toml`:
```toml
permission_mode = "dangerously-skip"   # "dangerously-skip" | "accept-edits" | "default"
show_hidden = false
max_title_length = 80
date_format = "2006-01-02 15:04"
```

## Где что хранится

- `~/.claude/projects/` — данные сессий (источник истины, управляется Claude Code)
- `~/.claude/cc-sessions/index.json` — кеш разобранных метаданных
- `~/.claude/cc-sessions/meta.json` — пин/скрытие/теги
- `~/.claude/cc-sessions/config.toml` — настройки

## Разработка

```bash
make test
make lint
```
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: full README"
```

---

## Self-Review Notes

- **Spec coverage:** All 8 components from spec have a task (paths=2, parser=5, cache=6, meta=7, config=8, scanner=9, launcher=10, tui=11-12). Bootstrap=1, cmd=13. Manual test=14. README=15.
- **Placeholders:** None — every step contains actual code or commands.
- **Type consistency:** `cache.Entry` shared between scanner, cache, tui. `meta.Entry` only in meta package. `session.Session` composed in tui. `ResumeMsg` defined in tui, consumed in cmd.
- **Order:** paths → session (types) → parser → cache → meta → config → scanner (depends on cache, parser, paths) → launcher → tui (depends on cache, meta, config, session) → cmd (wires all).
