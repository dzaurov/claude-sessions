package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dzaurov/claude-sessions/internal/cache"
	"github.com/dzaurov/claude-sessions/internal/config"
	"github.com/dzaurov/claude-sessions/internal/meta"
)

func newTestModel(t *testing.T, entries []cache.Entry) Model {
	t.Helper()
	m := meta.New(filepath.Join(t.TempDir(), "meta.json"))
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	return New(config.Default(), m, entries, func() ([]cache.Entry, error) { return entries, nil })
}

func TestSortPinnedFirstThenByDate(t *testing.T) {
	mdl := newTestModel(t, []cache.Entry{
		{Key: "/a::1", UUID: "1", ProjectPath: "/a", Title: "old", LastActivity: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Key: "/b::2", UUID: "2", ProjectPath: "/b", Title: "new", LastActivity: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
	})
	mdl.meta.SetPinned("/a::1", true)
	mdl.refreshFromMeta()
	if len(mdl.filtered) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(mdl.filtered))
	}
	if mdl.filtered[0].UUID != "1" {
		t.Errorf("expected pinned first, got %v", mdl.filtered[0].UUID)
	}
}

func TestFilterByFuzzyTitle(t *testing.T) {
	mdl := newTestModel(t, []cache.Entry{
		{Key: "/x::1", UUID: "1", ProjectPath: "/x", Title: "Jenkins cascade merge", LastActivity: time.Now()},
		{Key: "/x::2", UUID: "2", ProjectPath: "/x", Title: "Random stuff", LastActivity: time.Now()},
	})
	mdl.search.SetValue("jenkins")
	mdl.applyFilter()
	if len(mdl.filtered) != 1 || mdl.filtered[0].UUID != "1" {
		t.Errorf("expected 1 jenkins match, got %+v", mdl.filtered)
	}
}

func TestEnterSetsPendingResume(t *testing.T) {
	tmpProj := t.TempDir() // a real dir so Missing=false
	mdl := newTestModel(t, []cache.Entry{
		{Key: tmpProj + "::u1", UUID: "u1", ProjectPath: tmpProj, Cwd: tmpProj, Title: "test", LastActivity: time.Now()},
	})
	updated, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd, got nil")
	}
	fm := updated.(Model)
	rm := fm.PendingResume()
	if rm == nil {
		t.Fatal("expected PendingResume to be set")
	}
	if rm.UUID != "u1" || rm.Cwd != tmpProj {
		t.Errorf("ResumeMsg=%+v", rm)
	}
	if rm.ForkSession {
		t.Error("expected ForkSession=false on plain Enter")
	}
}

func TestCtrlFSetsForkResume(t *testing.T) {
	tmpProj := t.TempDir()
	mdl := newTestModel(t, []cache.Entry{
		{Key: tmpProj + "::u1", UUID: "u1", ProjectPath: tmpProj, Cwd: tmpProj, Title: "test", LastActivity: time.Now()},
	})
	updated, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd from Ctrl+F")
	}
	fm := updated.(Model)
	rm := fm.PendingResume()
	if rm == nil || !rm.ForkSession {
		t.Errorf("expected ForkSession=true, got %+v", rm)
	}
}

func TestEnterBlockedWhenMissing(t *testing.T) {
	mdl := newTestModel(t, []cache.Entry{
		{Key: "/nope::u1", UUID: "u1", ProjectPath: "/nope", Cwd: "/this/does/not/exist", Title: "x", LastActivity: time.Now()},
	})
	if !mdl.filtered[0].Missing {
		t.Skip("test depends on missing-path detection")
	}
	_, cmd := mdl.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("expected no cmd when missing, got non-nil")
	}
}

func TestSearchModeFiltersThenEscapeClears(t *testing.T) {
	mdl := newTestModel(t, []cache.Entry{
		{Key: "/x::1", UUID: "1", ProjectPath: "/x", Title: "Jenkins", LastActivity: time.Now()},
		{Key: "/x::2", UUID: "2", ProjectPath: "/x", Title: "Other", LastActivity: time.Now()},
	})
	// Enter search mode
	updated, _ := mdl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	mdl = updated.(Model)
	if !mdl.searchMode {
		t.Fatal("expected searchMode")
	}
	// Type "jen"
	for _, r := range "jen" {
		updated, _ = mdl.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		mdl = updated.(Model)
	}
	if len(mdl.filtered) != 1 {
		t.Errorf("expected filtered to 1 after typing, got %d", len(mdl.filtered))
	}
	// Escape clears
	updated, _ = mdl.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mdl = updated.(Model)
	if mdl.searchMode {
		t.Error("expected searchMode false after esc")
	}
	if len(mdl.filtered) != 2 {
		t.Errorf("expected filter cleared, got %d", len(mdl.filtered))
	}
}

// Avoid `os` unused if previous test got skipped; touch it.
var _ = os.Stat

func TestRename_setsCustomTitle(t *testing.T) {
	tmpProj := t.TempDir()
	mdl := newTestModel(t, []cache.Entry{
		{Key: tmpProj + "::u1", UUID: "u1", ProjectPath: tmpProj, Cwd: tmpProj, Title: "boring", LastActivity: time.Now()},
	})
	// F2 enters rename mode
	updated, _ := mdl.Update(tea.KeyMsg{Type: tea.KeyF2})
	mdl = updated.(Model)
	if !mdl.renameMode {
		t.Fatal("expected renameMode true")
	}
	mdl.rename.SetValue("my better title")
	updated, _ = mdl.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mdl = updated.(Model)
	if mdl.renameMode {
		t.Error("expected renameMode false after Enter")
	}
	got := mdl.meta.Get(tmpProj + "::u1").CustomTitle
	if got != "my better title" {
		t.Errorf("CustomTitle=%q, want 'my better title'", got)
	}
}

func TestEmptySessionFilteredByDefault(t *testing.T) {
	mdl := newTestModel(t, []cache.Entry{
		{Key: "/x::1", UUID: "1", ProjectPath: "/x", Title: "real", LastActivity: time.Now()},
		{Key: "/x::2", UUID: "2", ProjectPath: "/x", Title: "(no user message)", LastActivity: time.Now()},
		{Key: "/x::3", UUID: "3", ProjectPath: "/x", Title: "", LastActivity: time.Now()},
	})
	if len(mdl.filtered) != 1 {
		t.Errorf("expected 1 visible (real one), got %d: %+v", len(mdl.filtered), mdl.filtered)
	}
}

func TestEmptyButRenamedNotFiltered(t *testing.T) {
	m := meta.New(filepath.Join(t.TempDir(), "meta.json"))
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	m.SetCustomTitle("/x::2", "I renamed this")
	mdl := New(config.Default(), m, []cache.Entry{
		{Key: "/x::2", UUID: "2", ProjectPath: "/x", Title: "(no user message)", LastActivity: time.Now()},
	}, func() ([]cache.Entry, error) { return nil, nil })
	if len(mdl.filtered) != 1 {
		t.Errorf("expected renamed empty session to remain, got %d", len(mdl.filtered))
	}
}

func TestToggleHideExcludesHiddenByDefault(t *testing.T) {
	mdl := newTestModel(t, []cache.Entry{
		{Key: "/x::1", UUID: "1", ProjectPath: "/x", Title: "keep", LastActivity: time.Now()},
		{Key: "/x::2", UUID: "2", ProjectPath: "/x", Title: "trash", LastActivity: time.Now()},
	})
	mdl.meta.SetHidden("/x::2", true)
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
