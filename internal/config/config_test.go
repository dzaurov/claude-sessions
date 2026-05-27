package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	c := Default()
	if len(c.DefaultArgs) == 0 || c.DefaultArgs[0] != "--dangerously-skip-permissions" {
		t.Errorf("DefaultArgs=%v", c.DefaultArgs)
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
	if len(c.DefaultArgs) == 0 {
		t.Errorf("default DefaultArgs not applied")
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected config file written, got %v", err)
	}
}

func TestLoadLegacyPermissionMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`permission_mode = "accept-edits"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.DefaultArgs) < 2 || c.DefaultArgs[0] != "--permission-mode" || c.DefaultArgs[1] != "acceptEdits" {
		t.Errorf("legacy mode not migrated to DefaultArgs: %v", c.DefaultArgs)
	}
}

func TestLoadDefaultArgs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `default_args = ["--dangerously-skip-permissions", "--model", "opus"]` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--dangerously-skip-permissions", "--model", "opus"}
	if len(c.DefaultArgs) != 3 {
		t.Fatalf("DefaultArgs=%v, want %v", c.DefaultArgs, want)
	}
	for i, v := range want {
		if c.DefaultArgs[i] != v {
			t.Errorf("DefaultArgs[%d]=%q, want %q", i, c.DefaultArgs[i], v)
		}
	}
}

func TestExpandTilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	cases := map[string]string{
		"~":             home,
		"~/foo":         home + "/foo",
		"/abs/path":     "/abs/path",
		"relative/path": "relative/path",
	}
	for in, want := range cases {
		got := Expand(in)
		if got != want {
			t.Errorf("Expand(%q)=%q, want %q", in, got, want)
		}
	}
}
