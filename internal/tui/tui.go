package tui

import tea "github.com/charmbracelet/bubbletea"

// currentProgram holds a reference to the running Bubble Tea program so that
// goroutines spawned by the runner can dispatch messages into the event loop.
var currentProgram *tea.Program

// SetProgram stores the active program reference. Called by the app layer
// before p.Run() so that runner goroutines can inject messages via Send.
func SetProgram(p *tea.Program) {
	currentProgram = p
}
