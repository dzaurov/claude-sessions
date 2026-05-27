// Command ccs is the entry point: load config + meta, scan sessions, render
// the TUI; on ResumeMsg, persist meta and exec into claude.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dzaurov/claude-sessions/internal/cache"
	"github.com/dzaurov/claude-sessions/internal/cchome"
	"github.com/dzaurov/claude-sessions/internal/config"
	"github.com/dzaurov/claude-sessions/internal/launcher"
	"github.com/dzaurov/claude-sessions/internal/meta"
	"github.com/dzaurov/claude-sessions/internal/parser"
	"github.com/dzaurov/claude-sessions/internal/scanner"
	"github.com/dzaurov/claude-sessions/internal/state"
	"github.com/dzaurov/claude-sessions/internal/tui"
)

var (
	flagListJSON = flag.Bool("list-json", false, "print sessions as JSON and exit")
	flagFullScan = flag.Bool("full-scan", false, "run a full-disk discovery scan and exit (synchronous)")
	flagShowID   = flag.Bool("show-id", false, "open TUI as a picker: print selected session UUID to stdout and exit (no resume)")
	flagShowPath = flag.Bool("show-path", false, "open TUI as a picker: print selected session file path to stdout and exit (no resume)")
	flagResume   = flag.Bool("resume", false, "with a file argument, resume that session immediately (skip TUI)")
	flagFork     = flag.Bool("fork", false, "fork the session when resuming (passes --fork-session to claude)")
)

func main() {
	flag.Usage = usage
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ccs:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `ccs — Browse and resume Claude Code sessions

Usage:
  ccs                            Open the TUI
  ccs <file.jsonl>               Print metadata for one session (combine with --resume to launch it)
  ccs --full-scan                Walk the whole disk for sessions outside ~/.claude/projects
  ccs --list-json                Dump the current index as JSON
  ccs --show-id                  TUI picker: print selected UUID and exit
  ccs --show-path                TUI picker: print selected file path and exit

Flags:
`)
	flag.PrintDefaults()
}

func run() error {
	claudeDir, err := cchome.Dir()
	if err != nil {
		return err
	}
	stateDir := filepath.Join(claudeDir, "cc-sessions")

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
	st := state.New(filepath.Join(stateDir, "state.json"))
	if err := st.Load(); err != nil {
		return fmt.Errorf("state: %w", err)
	}

	// Positional argument = path to a single jsonl session file.
	if path := flag.Arg(0); path != "" {
		return handleSingleFile(path, cfg)
	}

	roots := expandAllWithDefault(cfg.Roots, claudeDir)
	scn := scanner.New(roots, c)

	if *flagFullScan {
		return runFullScan(cfg, c, st, roots)
	}

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

	if shouldAutoFullScan(cfg, st) {
		go backgroundFullScan(cfg, c, st, roots)
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
	fm, _ := finalModel.(tui.Model)
	rm := fm.PendingResume()
	if rm == nil {
		return nil
	}
	switch {
	case *flagShowID:
		fmt.Println(rm.UUID)
		return nil
	case *flagShowPath:
		// Look up file path from cache by key.
		key := rm.Cwd + "::" + rm.UUID
		if e, ok := c.Get(key); ok {
			fmt.Println(e.FilePath)
			return nil
		}
		fmt.Println(rm.UUID) // fallback
		return nil
	}
	return launcher.Exec(launcher.Options{
		UUID:        rm.UUID,
		Cwd:         rm.Cwd,
		DefaultArgs: rm.DefaultArgs,
		ForkSession: rm.ForkSession,
	})
}

// handleSingleFile is invoked when ccs is given a .jsonl path as positional
// argument. With --resume it execs into claude; otherwise prints metadata.
func handleSingleFile(path string, cfg config.Config) error {
	if !strings.HasSuffix(path, ".jsonl") {
		return fmt.Errorf("expected a .jsonl file, got %q", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	res, err := parser.ParseFile(abs)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	uuid := strings.TrimSuffix(filepath.Base(abs), ".jsonl")
	cwd := res.Cwd
	if cwd == "" {
		cwd = filepath.Dir(abs)
	}

	if *flagResume {
		return launcher.Exec(launcher.Options{
			UUID:        uuid,
			Cwd:         cwd,
			DefaultArgs: cfg.DefaultArgs,
			ForkSession: *flagFork,
		})
	}

	// Default: print metadata.
	title := res.FirstUserMsg
	if title == "" {
		title = res.CustomTitle
	}
	fmt.Printf("uuid:    %s\n", uuid)
	fmt.Printf("cwd:     %s\n", cwd)
	fmt.Printf("branch:  %s\n", res.GitBranch)
	fmt.Printf("title:   %s\n", title)
	fmt.Printf("count:   %d\n", res.MsgCount)
	fmt.Printf("last:    %s\n", res.LastTimestamp)
	return nil
}

func expandAllWithDefault(paths []string, claudeDir string) []string {
	if len(paths) == 0 {
		return []string{filepath.Join(claudeDir, "projects")}
	}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		// Special-case "~/.claude/projects" → respect CLAUDE_CONFIG_DIR
		if p == "~/.claude/projects" {
			out = append(out, filepath.Join(claudeDir, "projects"))
			continue
		}
		out = append(out, config.Expand(p))
	}
	return out
}

func expandAll(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = config.Expand(p)
	}
	return out
}

func shouldAutoFullScan(cfg config.Config, st *state.Store) bool {
	if cfg.FullScanIntervalHours <= 0 {
		return false
	}
	last := st.LastFullScan()
	if last.IsZero() {
		return true
	}
	return time.Since(last) > time.Duration(cfg.FullScanIntervalHours)*time.Hour
}

func runFullScan(cfg config.Config, c *cache.Cache, st *state.Store, roots []string) error {
	start := time.Now()
	paths := expandAll(cfg.FullScanPaths)
	opts := scanner.DiscoveryOptions{
		Paths:     paths,
		Ignore:    cfg.FullScanIgnore,
		ExtraSkip: roots,
	}
	entries, err := scanner.FullScan(opts, c)
	if err != nil {
		return fmt.Errorf("full-scan: %w", err)
	}
	if err := c.Save(); err != nil {
		return fmt.Errorf("cache save: %w", err)
	}
	st.MarkFullScan(time.Now())
	if err := st.Save(); err != nil {
		return fmt.Errorf("state save: %w", err)
	}
	fmt.Fprintf(os.Stderr, "ccs: full-scan done in %s, %d session(s) discovered\n", time.Since(start).Round(time.Millisecond), len(entries))
	return nil
}

func backgroundFullScan(cfg config.Config, c *cache.Cache, st *state.Store, roots []string) {
	paths := expandAll(cfg.FullScanPaths)
	opts := scanner.DiscoveryOptions{
		Paths:     paths,
		Ignore:    cfg.FullScanIgnore,
		ExtraSkip: roots,
	}
	if _, err := scanner.FullScan(opts, c); err != nil {
		fmt.Fprintln(os.Stderr, "ccs: bg full-scan:", err)
		return
	}
	_ = c.Save()
	st.MarkFullScan(time.Now())
	_ = st.Save()
}
