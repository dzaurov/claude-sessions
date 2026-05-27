// Package cchome resolves Claude Code's home directory, honoring the
// CLAUDE_CONFIG_DIR environment variable that Claude Code itself supports.
package cchome

import (
	"os"
	"path/filepath"
)

// Dir returns the Claude Code config directory. Respects CLAUDE_CONFIG_DIR
// if set, otherwise falls back to ~/.claude.
func Dir() (string, error) {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

// ProjectsDir returns Dir()/projects.
func ProjectsDir() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "projects"), nil
}

// StateDir returns Dir()/cc-sessions, where ccs's own state lives.
func StateDir() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "cc-sessions"), nil
}
