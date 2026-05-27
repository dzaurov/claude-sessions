// Package tui is the bubbletea Model for ccs.
package tui

import (
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sahilm/fuzzy"

	"github.com/dzaurov/claude-sessions/internal/cache"
	"github.com/dzaurov/claude-sessions/internal/config"
	"github.com/dzaurov/claude-sessions/internal/meta"
	"github.com/dzaurov/claude-sessions/internal/session"
)

// ResumeMsg is set on the Model when the user picks a session to resume.
// Main inspects it after the TUI program exits.
type ResumeMsg struct {
	Cwd         string
	UUID        string
	DefaultArgs []string
	ForkSession bool
}

type RescanFn func() ([]cache.Entry, error)

type Model struct {
	cfg           config.Config
	meta          *meta.Store
	rescan        RescanFn
	all           []session.Session
	filtered      []session.Session
	cursor        int
	search        textinput.Model
	searchMode    bool
	rename        textinput.Model
	renameMode    bool
	renameTarget  string // session Key being renamed
	showHidden    bool
	helpView      help.Model
	showHelp      bool
	width         int
	height        int
	err           error
	keys          KeyMap
	pendingResume *ResumeMsg
}

func New(cfg config.Config, m *meta.Store, entries []cache.Entry, rescan RescanFn) Model {
	ti := textinput.New()
	ti.Placeholder = "fuzzy search…"
	ti.Prompt = "/ "
	ti.CharLimit = 256

	rn := textinput.New()
	rn.Prompt = "rename: "
	rn.CharLimit = 200

	mdl := Model{
		cfg:        cfg,
		meta:       m,
		rescan:     rescan,
		search:     ti,
		rename:     rn,
		helpView:   help.New(),
		keys:       DefaultKeys().ApplyOverrides(cfg.Keys),
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
		s.Missing = !pathExists(s.Cwd)
		// Empty sessions: nothing parseable as a title AND user hasn't
		// renamed them. These are usually just /clear or aborted starts.
		// Filter them out unless config opts in.
		if !m.cfg.ShowEmpty && isEmptySession(s) {
			continue
		}
		all = append(all, s)
	}
	sortSessions(all)
	m.all = all
	m.applyFilter()
}

func isEmptySession(s session.Session) bool {
	if s.CustomTitle != "" {
		return false
	}
	if s.Title == "" || s.Title == "(no user message)" {
		return true
	}
	return false
}

func sortSessions(all []session.Session) {
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Pinned != all[j].Pinned {
			return all[i].Pinned
		}
		return all[i].LastActivity.After(all[j].LastActivity)
	})
}

func pickCustomTitle(userOverride, jsonlTitle string) string {
	if userOverride != "" {
		return userOverride
	}
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
	if m.renameMode {
		switch {
		case key.Matches(msg, m.keys.Escape):
			m.renameMode = false
			m.rename.Blur()
			return m, nil
		}
		if msg.Type == tea.KeyEnter {
			newTitle := m.rename.Value()
			m.meta.SetCustomTitle(m.renameTarget, newTitle)
			_ = m.meta.Save()
			m.refreshFromMeta()
			m.renameMode = false
			m.rename.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.rename, cmd = m.rename.Update(msg)
		return m, cmd
	}
	if m.searchMode {
		switch {
		case key.Matches(msg, m.keys.Escape):
			m.searchMode = false
			m.search.Blur()
			m.search.SetValue("")
			m.applyFilter()
			return m, nil
		}
		if msg.Type == tea.KeyEnter {
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
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		return m, nil
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case key.Matches(msg, m.keys.Search):
		m.searchMode = true
		m.search.Focus()
		return m, nil
	case key.Matches(msg, m.keys.Pin):
		if s, ok := m.current(); ok {
			m.meta.SetPinned(s.Key(), !s.Pinned)
			_ = m.meta.Save()
			m.refreshFromMeta()
		}
	case key.Matches(msg, m.keys.Hide):
		if s, ok := m.current(); ok {
			m.meta.SetHidden(s.Key(), !s.Hidden)
			_ = m.meta.Save()
			m.refreshFromMeta()
		}
	case key.Matches(msg, m.keys.ToggleHide):
		m.showHidden = !m.showHidden
		m.applyFilter()
	case key.Matches(msg, m.keys.Rescan):
		if m.rescan != nil {
			entries, err := m.rescan()
			if err != nil {
				m.err = err
			} else {
				m.setEntries(entries)
			}
		}
	case key.Matches(msg, m.keys.Enter):
		if s, ok := m.current(); ok && !s.Missing {
			rm := ResumeMsg{Cwd: s.Cwd, UUID: s.UUID, DefaultArgs: m.cfg.DefaultArgs}
			m.pendingResume = &rm
			return m, tea.Quit
		}
	case key.Matches(msg, m.keys.Fork):
		if s, ok := m.current(); ok && !s.Missing {
			rm := ResumeMsg{Cwd: s.Cwd, UUID: s.UUID, DefaultArgs: m.cfg.DefaultArgs, ForkSession: true}
			m.pendingResume = &rm
			return m, tea.Quit
		}
	case key.Matches(msg, m.keys.Rename):
		if s, ok := m.current(); ok {
			m.renameMode = true
			m.renameTarget = s.Key()
			m.rename.SetValue(s.DisplayTitle())
			m.rename.Focus()
			return m, nil
		}
	}
	return m, nil
}

func (m *Model) refreshFromMeta() {
	for i := range m.all {
		e := m.meta.Get(m.all[i].Key())
		m.all[i].Pinned = e.Pinned
		m.all[i].Hidden = e.Hidden
		m.all[i].Tags = e.Tags
		m.all[i].Notes = e.Notes
		if e.CustomTitle != "" {
			m.all[i].CustomTitle = e.CustomTitle
		}
	}
	sortSessions(m.all)
	m.applyFilter()
}

func (m Model) current() (session.Session, bool) {
	if len(m.filtered) == 0 {
		return session.Session{}, false
	}
	return m.filtered[m.cursor], true
}

func (m Model) PendingResume() *ResumeMsg { return m.pendingResume }

func pathExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}
