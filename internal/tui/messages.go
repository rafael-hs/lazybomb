package tui

import "github.com/rafael-hs/lazybomb/internal/runner"

// tickMsg is dispatched on each metrics refresh tick.
type tickMsg struct {
	snap runner.Snapshot
}

// doneMsg is dispatched when the load test finishes or is stopped.
type doneMsg struct {
	snap    runner.Snapshot
	stopped bool
}

