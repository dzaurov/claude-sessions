package cchome

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir_default(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	d, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if d != filepath.Join(home, ".claude") {
		t.Errorf("Dir()=%q, want %q", d, filepath.Join(home, ".claude"))
	}
}

func TestDir_envOverride(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/custom/claude")
	d, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if d != "/custom/claude" {
		t.Errorf("Dir()=%q, want /custom/claude", d)
	}
}

func TestProjectsDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/x")
	p, _ := ProjectsDir()
	if p != "/x/projects" {
		t.Errorf("ProjectsDir=%q", p)
	}
}

func TestStateDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/x")
	s, _ := StateDir()
	if s != "/x/cc-sessions" {
		t.Errorf("StateDir=%q", s)
	}
}
