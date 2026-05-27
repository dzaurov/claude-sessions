package tui

import (
	"github.com/charmbracelet/bubbles/key"

	"github.com/dzaurov/claude-sessions/internal/config"
)

type KeyMap struct {
	Up         key.Binding
	Down       key.Binding
	Enter      key.Binding
	Search     key.Binding
	Escape     key.Binding
	Pin        key.Binding
	Hide       key.Binding
	ToggleHide key.Binding
	Rescan     key.Binding
	Rename     key.Binding
	Fork       key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func DefaultKeys() KeyMap {
	return KeyMap{
		Up:         key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:       key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Enter:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("⏎", "resume")),
		Search:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Escape:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear")),
		Pin:        key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pin")),
		Hide:       key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "hide")),
		ToggleHide: key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "show hidden")),
		Rescan:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rescan")),
		Rename:     key.NewBinding(key.WithKeys("f2"), key.WithHelp("F2", "rename")),
		Fork:       key.NewBinding(key.WithKeys("ctrl+f"), key.WithHelp("^F", "fork")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:       key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// ApplyOverrides replaces individual bindings with user-provided keys from
// config. Empty strings keep the default for that action.
func (k KeyMap) ApplyOverrides(o config.Keys) KeyMap {
	apply := func(b *key.Binding, override, help string) {
		if override == "" {
			return
		}
		*b = key.NewBinding(key.WithKeys(override), key.WithHelp(override, help))
	}
	apply(&k.Up, o.Up, "up")
	apply(&k.Down, o.Down, "down")
	apply(&k.Enter, o.Enter, "resume")
	apply(&k.Search, o.Search, "search")
	apply(&k.Pin, o.Pin, "pin")
	apply(&k.Hide, o.Hide, "hide")
	apply(&k.ToggleHide, o.ToggleHide, "show hidden")
	apply(&k.Rescan, o.Rescan, "rescan")
	apply(&k.Rename, o.Rename, "rename")
	apply(&k.Fork, o.Fork, "fork")
	apply(&k.Help, o.Help, "help")
	apply(&k.Quit, o.Quit, "quit")
	return k
}

func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Search, k.Pin, k.Hide, k.Rename, k.Fork, k.Rescan, k.Help, k.Quit}
}

func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Escape},
		{k.Search, k.Pin, k.Hide, k.ToggleHide},
		{k.Rename, k.Fork, k.Rescan},
		{k.Help, k.Quit},
	}
}
