// Package scanner walks Claude Code project directories and produces cache
// entries for every session .jsonl. Uses the cache to skip files whose mtime
// hasn't changed since the last scan. Parsing of changed files happens in a
// worker pool sized to NumCPU.
package scanner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dzaurov/claude-sessions/internal/cache"
	"github.com/dzaurov/claude-sessions/internal/parser"
	"github.com/dzaurov/claude-sessions/internal/paths"
)

type Scanner struct {
	roots []string
	cache *cache.Cache
}

// New creates a Scanner over one or more project roots. Each root is treated
// as a Claude Code projects directory (children are encoded project folders,
// grandchildren are <uuid>.jsonl session files).
func New(roots []string, c *cache.Cache) *Scanner {
	return &Scanner{roots: roots, cache: c}
}

type candidate struct {
	key         string
	uuid        string
	projectPath string
	filePath    string
	mtime       time.Time
}

func (s *Scanner) Scan() ([]cache.Entry, error) {
	var todo []candidate
	seen := make(map[string]struct{})

	for _, root := range s.roots {
		projects, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, pd := range projects {
			if !pd.IsDir() {
				continue
			}
			projDir := filepath.Join(root, pd.Name())
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
				todo = append(todo, candidate{
					key:         key,
					uuid:        uuid,
					projectPath: projectPath,
					filePath:    filePath,
					mtime:       info.ModTime(),
				})
			}
		}
	}

	entries := parseAllParallel(todo, s.cache)

	// Eviction: only delete an entry if (a) its FilePath is inside one of
	// our roots and (b) we didn't see it this scan. Entries discovered by
	// FullScan from outside the roots are preserved. Entries whose file is
	// gone from disk get evicted on the stat check below.
	for _, e := range s.cache.All() {
		if _, ok := seen[e.Key]; ok {
			continue
		}
		if isUnderAny(e.FilePath, s.roots) {
			s.cache.Delete(e.Key)
			continue
		}
		if _, err := os.Stat(e.FilePath); err != nil {
			s.cache.Delete(e.Key)
		}
	}

	// Return everything currently visible: roots' entries plus any extra
	// (full-scan) cache entries whose files still exist.
	out := entries
	rootKeys := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		rootKeys[e.Key] = struct{}{}
	}
	for _, e := range s.cache.All() {
		if _, ok := rootKeys[e.Key]; ok {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func isUnderAny(path string, roots []string) bool {
	for _, r := range roots {
		// Treat root prefix match as "under": e.g. /a/b under /a/.
		// Add trailing slash check to avoid /ab matching /a.
		if path == r || strings.HasPrefix(path, r+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// parseAllParallel runs parser.ParseFile in a worker pool for every
// candidate whose mtime is newer than its cached counterpart. Cached entries
// with current mtime are returned untouched.
func parseAllParallel(todo []candidate, c *cache.Cache) []cache.Entry {
	if len(todo) == 0 {
		return nil
	}
	workers := runtime.NumCPU()
	if workers > len(todo) {
		workers = len(todo)
	}
	in := make(chan candidate, len(todo))
	out := make(chan cache.Entry, len(todo))
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for cd := range in {
			cached, hit := c.Get(cd.key)
			if hit && !cd.mtime.After(cached.Mtime) {
				out <- cached
				continue
			}
			res, err := parser.ParseFile(cd.filePath)
			if err != nil {
				if hit {
					out <- cached
				}
				continue
			}
			entry := cache.Entry{
				Key:          cd.key,
				UUID:         cd.uuid,
				ProjectPath:  cd.projectPath,
				Title:        res.FirstUserMsg,
				CustomTitle:  res.CustomTitle,
				LastActivity: parseTimestamp(res.LastTimestamp, cd.mtime),
				Mtime:        cd.mtime,
				MsgCount:     res.MsgCount,
				Cwd:          firstNonEmpty(res.Cwd, cd.projectPath),
				GitBranch:    res.GitBranch,
				FilePath:     cd.filePath,
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
	for _, cd := range todo {
		in <- cd
	}
	close(in)
	wg.Wait()
	close(out)

	entries := make([]cache.Entry, 0, len(todo))
	for e := range out {
		entries = append(entries, e)
	}
	return entries
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
