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
	flagFullScan = flag.Bool("full-scan", false, "run a full-disk discovery scan in addition to the fast scan")
	flagShowID   = flag.Bool("show-id", false, "open TUI as a picker: print selected session UUID to stdout and exit (no resume)")
	flagShowPath = flag.Bool("show-path", false, "open TUI as a picker: print selected session file path to stdout and exit (no resume)")
	flagResume   = flag.Bool("resume", false, "with a file argument, resume that session immediately (skip TUI)")
	flagFork     = flag.Bool("fork", false, "fork the session when resuming (passes --fork-session to claude)")
)

func main() {
	flag.Usage = usage
	// Reorder os.Args so that flags work no matter where they appear
	// relative to a positional argument. Go's stdlib flag package stops
	// parsing at the first non-flag argument, which makes invocations like
	// `ccs file.jsonl --resume` silently ignore the flag.
	os.Args = reorderArgs(os.Args)
	flag.Parse()
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ccs:", err)
		os.Exit(1)
	}
}

// reorderArgs moves all flag-looking tokens (those starting with "-") to
// the front, preserving relative order, so they're parsed by flag.Parse
// regardless of where the user placed them. This only works correctly
// because every ccs flag is a boolean — there are no `--flag value` pairs
// that could be split by reordering.
func reorderArgs(args []string) []string {
	if len(args) <= 1 {
		return args
	}
	out := make([]string, 0, len(args))
	out = append(out, args[0])
	var flags, positional []string
	for _, a := range args[1:] {
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
		} else {
			positional = append(positional, a)
		}
	}
	out = append(out, flags...)
	out = append(out, positional...)
	return out
}

func usage() {
	fmt.Fprintf(os.Stderr, `ccs — Browse and resume Claude Code sessions

Usage:
  ccs                            Open the TUI
  ccs <file.jsonl>               Print metadata for one session
  ccs <file.jsonl> --resume      Resume that session immediately
  ccs <file.jsonl> --resume --fork  Fork-resume that session
  ccs --full-scan                Fast scan + full-disk discovery
  ccs --list-json                Dump the current index as JSON
  ccs --show-id                  TUI picker: print selected UUID and exit
  ccs --show-path                TUI picker: print selected file path and exit

Flags can appear before or after the positional argument.

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

	// Direct file mode: positional argument is a path to a single jsonl.
	if path := flag.Arg(0); path != "" {
		return handleSingleFile(path, cfg)
	}

	roots := expandAllWithDefault(cfg.Roots, claudeDir)
	scn := scanner.New(roots, c)

	// Always run the fast scan first. --full-scan adds discovery on top,
	// it does not replace the fast scan.
	entries, err := scn.Scan()
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	if *flagFullScan {
		extra, err := runFullScan(cfg, c, st, roots)
		if err != nil {
			return err
		}
		// Re-fetch the combined view from cache after FullScan added entries.
		entries = mergeNew(entries, extra)
	}

	if err := c.Save(); err != nil {
		fmt.Fprintln(os.Stderr, "ccs: warning: cache save:", err)
	}

	if *flagListJSON {
		if entries == nil {
			entries = []cache.Entry{}
		}
		return json.NewEncoder(os.Stdout).Encode(entries)
	}

	// --full-scan is allowed to exit without opening the TUI; the user
	// asked for a scan, not a picker.
	if *flagFullScan {
		return nil
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
		fmt.Println(rm.FilePath)
		return nil
	}
	return launcher.Exec(launcher.Options{
		UUID:        rm.UUID,
		Cwd:         rm.Cwd,
		DefaultArgs: rm.DefaultArgs,
		ForkSession: rm.ForkSession,
	})
}

// mergeNew unions two entry slices by Key, preferring the right side's
// values. Used to combine fast-scan and full-scan results.
func mergeNew(a, b []cache.Entry) []cache.Entry {
	idx := make(map[string]int, len(a)+len(b))
	out := make([]cache.Entry, 0, len(a)+len(b))
	for _, e := range a {
		idx[e.Key] = len(out)
		out = append(out, e)
	}
	for _, e := range b {
		if i, ok := idx[e.Key]; ok {
			out[i] = e
			continue
		}
		idx[e.Key] = len(out)
		out = append(out, e)
	}
	return out
}

// handleSingleFile is invoked when ccs is given a .jsonl path as positional
// argument. With --resume it execs into claude; otherwise prints metadata.
// Refuses files that aren't Claude Code session JSONL (signature check).
func handleSingleFile(path string, cfg config.Config) error {
	if !strings.HasSuffix(path, ".jsonl") {
		return fmt.Errorf("expected a .jsonl file, got %q", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return err
	}
	if !parser.IsSessionFile(abs) {
		return fmt.Errorf("%q does not look like a Claude Code session (no sessionId or known message types in the first lines)", abs)
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

func runFullScan(cfg config.Config, c *cache.Cache, st *state.Store, roots []string) ([]cache.Entry, error) {
	start := time.Now()
	paths := expandAll(cfg.FullScanPaths)
	opts := scanner.DiscoveryOptions{
		Paths:     paths,
		Ignore:    cfg.FullScanIgnore,
		ExtraSkip: roots,
	}
	entries, err := scanner.FullScan(opts, c)
	if err != nil {
		return nil, fmt.Errorf("full-scan: %w", err)
	}
	st.MarkFullScan(time.Now())
	if err := st.Save(); err != nil {
		return entries, fmt.Errorf("state save: %w", err)
	}
	fmt.Fprintf(os.Stderr, "ccs: full-scan added %d session(s) in %s\n", len(entries), time.Since(start).Round(time.Millisecond))
	return entries, nil
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
