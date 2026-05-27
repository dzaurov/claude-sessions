// Package config loads ~/.claude/cc-sessions/config.toml, falling back to
// defaults when fields are missing or the file does not exist.
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Keys lets users override default keybindings via config.toml.
// Empty string means use the built-in default.
type Keys struct {
	Up         string `toml:"up"`
	Down       string `toml:"down"`
	Enter      string `toml:"enter"`
	Search     string `toml:"search"`
	Pin        string `toml:"pin"`
	Hide       string `toml:"hide"`
	ToggleHide string `toml:"toggle_hide"`
	Rescan     string `toml:"rescan"`
	Rename     string `toml:"rename"`
	Fork       string `toml:"fork"`
	Help       string `toml:"help"`
	Quit       string `toml:"quit"`
}

type Config struct {
	// Deprecated: prefer DefaultArgs. Kept for backwards compatibility —
	// when set and DefaultArgs is empty, it is converted at load time.
	PermissionMode string `toml:"permission_mode"`

	// DefaultArgs is appended to every "claude --resume <uuid>" invocation.
	// E.g. ["--dangerously-skip-permissions", "--model", "opus"].
	DefaultArgs []string `toml:"default_args"`

	ShowHidden            bool     `toml:"show_hidden"`
	ShowEmpty             bool     `toml:"show_empty"`
	MaxTitleLength        int      `toml:"max_title_length"`
	DateFormat            string   `toml:"date_format"`
	Roots                 []string `toml:"roots"`
	FullScanPaths         []string `toml:"full_scan_paths"`
	FullScanIgnore        []string `toml:"full_scan_ignore"`
	FullScanIntervalHours int      `toml:"full_scan_interval_hours"`

	Keys Keys `toml:"keys"`
}

func Default() Config {
	return Config{
		DefaultArgs:           []string{"--dangerously-skip-permissions"},
		ShowHidden:            false,
		ShowEmpty:             false,
		MaxTitleLength:        80,
		DateFormat:            "2006-01-02 15:04",
		Roots:                 []string{"~/.claude/projects"},
		FullScanPaths:         []string{"~"},
		FullScanIgnore:        DefaultIgnore(),
		FullScanIntervalHours: 24,
	}
}

// DefaultIgnore returns directory basenames to skip during full HOME walks.
// These folders almost never contain Claude Code sessions but often contain
// many unrelated .jsonl files (datasets, package metadata, build artifacts).
func DefaultIgnore() []string {
	return []string{
		".git",
		".svn",
		".hg",
		"node_modules",
		"vendor",
		"__pycache__",
		".venv",
		"venv",
		".next",
		".nuxt",
		"dist",
		"build",
		"target",
		"Pods",
		"DerivedData",
		"Library",
		".Trash",
		".cache",
		"go/pkg",
		".gradle",
		".m2",
		".cargo",
		".rustup",
		".docker",
		"testdata",
	}
}

// Expand replaces a leading "~" with the user's home directory.
func Expand(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~"))
}

// permissionModeToArgs converts a legacy permission_mode string to the
// equivalent argv slice.
func permissionModeToArgs(mode string) []string {
	switch mode {
	case "dangerously-skip":
		return []string{"--dangerously-skip-permissions"}
	case "accept-edits":
		return []string{"--permission-mode", "acceptEdits"}
	case "default", "":
		return nil
	}
	return nil
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

	// Backwards compat: if user has legacy permission_mode and no
	// default_args, convert mode to args.
	switch {
	case len(parsed.DefaultArgs) > 0:
		c.DefaultArgs = parsed.DefaultArgs
	case parsed.PermissionMode != "":
		c.DefaultArgs = permissionModeToArgs(parsed.PermissionMode)
		c.PermissionMode = parsed.PermissionMode
	}

	c.ShowHidden = parsed.ShowHidden
	c.ShowEmpty = parsed.ShowEmpty
	if parsed.MaxTitleLength > 0 {
		c.MaxTitleLength = parsed.MaxTitleLength
	}
	if parsed.DateFormat != "" {
		c.DateFormat = parsed.DateFormat
	}
	if len(parsed.Roots) > 0 {
		c.Roots = parsed.Roots
	}
	if len(parsed.FullScanPaths) > 0 {
		c.FullScanPaths = parsed.FullScanPaths
	}
	if len(parsed.FullScanIgnore) > 0 {
		c.FullScanIgnore = parsed.FullScanIgnore
	}
	if parsed.FullScanIntervalHours > 0 {
		c.FullScanIntervalHours = parsed.FullScanIntervalHours
	}
	c.Keys = parsed.Keys
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
