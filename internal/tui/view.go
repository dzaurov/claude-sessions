package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/dzaurov/claude-sessions/internal/session"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	pinnedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	selectedStyle = lipgloss.NewStyle().Reverse(true)
	missingStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)

func (m Model) View() string {
	var b strings.Builder
	header := titleStyle.Render("cc-sessions")
	count := dimStyle.Render(fmt.Sprintf("  %d sessions", len(m.filtered)))
	b.WriteString(header + count + "\n")

	if m.renameMode {
		b.WriteString(m.rename.View() + dimStyle.Render("  (enter to save, esc to cancel)") + "\n")
	} else if m.searchMode || m.search.Value() != "" {
		b.WriteString(m.search.View() + "\n")
	}
	if m.err != nil {
		b.WriteString(errorStyle.Render("error: "+m.err.Error()) + "\n")
	}

	if len(m.filtered) == 0 {
		b.WriteString(dimStyle.Render("\n  (no sessions match)\n"))
	}

	for i, s := range m.filtered {
		row := formatRow(s, m.cfg.DateFormat, m.cfg.MaxTitleLength)
		if i == m.cursor {
			row = selectedStyle.Render(row)
		}
		b.WriteString(row + "\n")
	}

	if m.showHelp {
		b.WriteString("\n" + m.helpView.View(m.keys))
	} else {
		b.WriteString("\n" + dimStyle.Render(m.helpView.ShortHelpView(m.keys.ShortHelp())))
	}
	return b.String()
}

func formatRow(s session.Session, dateFmt string, maxTitle int) string {
	pin := "   "
	if s.Pinned {
		pin = pinnedStyle.Render(" ★ ")
	}
	when := s.LastActivity.Local().Format(dateFmt)
	proj := projectShortName(firstNonEmpty(s.Cwd, s.ProjectPath))
	title := s.DisplayTitle()
	if maxTitle > 0 && len([]rune(title)) > maxTitle {
		title = string([]rune(title)[:maxTitle]) + "…"
	}
	flags := ""
	if s.Hidden {
		flags += dimStyle.Render(" [hidden]")
	}
	if s.Missing {
		flags += missingStyle.Render(" (missing)")
	}
	return fmt.Sprintf("%s%s  %-22s  %s%s",
		pin,
		dimStyle.Render(when),
		proj,
		title,
		flags,
	)
}

func projectShortName(p string) string {
	if p == "" {
		return "(unknown)"
	}
	parts := strings.Split(strings.TrimRight(p, "/"), "/")
	return parts[len(parts)-1]
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
