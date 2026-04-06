package app

import (
	"log/slog"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/rafael-hs/lazybomb/internal/tui"
)

const logFile = "/tmp/lazybomb.log"

// Run initialises logging, then starts the terminal application, blocking until the user quits.
func Run() error {
	if err := setupLogging(); err != nil {
		slog.Warn("could not open log file, logging to stderr", "err", err)
	}

	slog.Info("starting lazybomb")

	m, err := tui.InitialModel()
	if err != nil {
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	tui.SetProgram(p)
	_, err = p.Run()
	return err
}

// setupLogging configures slog to write structured text logs to logFile.
// Tail it in another terminal: tail -f /tmp/lazybomb.log
func setupLogging() error {
	f, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler))
	return nil
}
