package scanner

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/dzaurov/claude-sessions/internal/cache"
	"github.com/dzaurov/claude-sessions/internal/parser"
)

// DiscoveryOptions configures FullScan.
type DiscoveryOptions struct {
	// Paths is a list of root directories to walk.
	Paths []string
	// Ignore is a list of directory basenames to skip while walking
	// (e.g. "node_modules", ".git").
	Ignore []string
	// ExtraSkip lets callers skip already-known roots (typically
	// ~/.claude/projects) — those are scanned by the fast scanner anyway.
	ExtraSkip []string
}

// FullScan walks every Paths root, skipping Ignore basenames, looking for
// .jsonl files. Each candidate is sniffed with parser.IsSessionFile to
// determine whether it's a Claude Code session log. Confirmed sessions are
// parsed in a worker pool and merged into the cache.
//
// Files inside ExtraSkip prefixes are not parsed here — the fast scanner is
// expected to have already covered them.
//
// FullScan never deletes cache entries — it only adds/updates. Eviction is
// the fast Scan's job.
func FullScan(opts DiscoveryOptions, c *cache.Cache) ([]cache.Entry, error) {
	if len(opts.Paths) == 0 {
		return nil, nil
	}
	ignored := toSet(opts.Ignore)
	skipPrefix := opts.ExtraSkip

	var candidates []string
	for _, root := range opts.Paths {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if ignored[d.Name()] {
					return fs.SkipDir
				}
				for _, skip := range skipPrefix {
					if path == skip {
						return fs.SkipDir
					}
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".jsonl") {
				return nil
			}
			candidates = append(candidates, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return parseDiscovered(candidates, c), nil
}

func parseDiscovered(candidates []string, c *cache.Cache) []cache.Entry {
	if len(candidates) == 0 {
		return nil
	}
	workers := runtime.NumCPU()
	if workers > len(candidates) {
		workers = len(candidates)
	}
	in := make(chan string, len(candidates))
	out := make(chan cache.Entry, len(candidates))
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for path := range in {
			if !parser.IsSessionFile(path) {
				continue
			}
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			res, err := parser.ParseFile(path)
			if err != nil {
				continue
			}
			uuid := strings.TrimSuffix(filepath.Base(path), ".jsonl")
			project := firstNonEmpty(res.Cwd, filepath.Dir(path))
			key := project + "::" + uuid
			entry := cache.Entry{
				Key:          key,
				UUID:         uuid,
				ProjectPath:  project,
				Title:        res.FirstUserMsg,
				CustomTitle:  res.CustomTitle,
				LastActivity: parseTimestamp(res.LastTimestamp, info.ModTime()),
				Mtime:        info.ModTime(),
				MsgCount:     res.MsgCount,
				Cwd:          firstNonEmpty(res.Cwd, project),
				GitBranch:    res.GitBranch,
				FilePath:     path,
			}
			if entry.Title == "" && entry.CustomTitle == "" {
				entry.Title = "(no user message)"
			}
			c.Set(entry)
			out <- entry
		}
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go worker()
	}
	for _, p := range candidates {
		in <- p
	}
	close(in)
	wg.Wait()
	close(out)

	entries := make([]cache.Entry, 0, len(candidates))
	for e := range out {
		entries = append(entries, e)
	}
	return entries
}

func toSet(ss []string) map[string]bool {
	out := make(map[string]bool, len(ss))
	for _, s := range ss {
		out[s] = true
	}
	return out
}
