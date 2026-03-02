package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Tab       key.Binding
	ShiftTab  key.Binding
	Enter     key.Binding
	Stop      key.Binding
	Save      key.Binding
	Load      key.Binding
	Delete    key.Binding
	Up        key.Binding
	Down      key.Binding
	Left      key.Binding
	Right     key.Binding
	Quit      key.Binding
	Help      key.Binding
}

var keys = keyMap{
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next field"),
	),
	ShiftTab: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev field"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter", "ctrl+r"),
		key.WithHelp("enter/ctrl+r", "run"),
	),
	Stop: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "stop"),
	),
	Save: key.NewBinding(
		key.WithKeys("ctrl+s"),
		key.WithHelp("ctrl+s", "save profile"),
	),
	Load: key.NewBinding(
		key.WithKeys("ctrl+l"),
		key.WithHelp("ctrl+l", "load profile"),
	),
	Delete: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "delete profile"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Left: key.NewBinding(
		key.WithKeys("left", "h"),
		key.WithHelp("←/h", "left"),
	),
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "right"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c", "q"),
		key.WithHelp("q/ctrl+c", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
}
