package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up        key.Binding
	Down      key.Binding
	Enter     key.Binding
	Resume    key.Binding
	New       key.Binding
	Filter    key.Binding
	Tab       key.Binding
	Group     key.Binding
	Refresh   key.Binding
	OpenFiles key.Binding
	Escape    key.Binding
	Quit      key.Binding
}

// ShortHelp implements help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Tab, k.Quit}
}

// FullHelp implements help.KeyMap.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Tab},                  // navigation
		{k.Filter, k.Resume, k.New, k.Group, k.Refresh}, // actions
		{k.OpenFiles, k.Escape, k.Quit},                 // misc
	}
}

// listKeys returns a keyMap with all bindings enabled (for the list pane).
func listKeys() keyMap {
	k := keys
	return k
}

// detailKeys returns a keyMap scoped to the detail pane.
func detailKeys() keyMap {
	k := keys
	k.Filter.SetEnabled(false)
	k.Resume.SetEnabled(false)
	k.New.SetEnabled(false)
	k.Group.SetEnabled(false)
	k.Refresh.SetEnabled(false)
	return k
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Resume: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "resume"),
	),
	New: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "new session"),
	),
	Filter: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch pane"),
	),
	Group: key.NewBinding(
		key.WithKeys("g"),
		key.WithHelp("g", "group"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("R"),
		key.WithHelp("R", "refresh"),
	),
	OpenFiles: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "open files"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "close detail"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
}
