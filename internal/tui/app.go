// Package tui is the top-level TUI controller for CoworkAgent.
// It wires together the Bubble Tea program, the agent, and the indexer,
// and selects the correct run mode (interactive, cowork, or index).
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/raonsama/cowork-agent/internal/agent"
	"github.com/raonsama/cowork-agent/internal/config"
	"github.com/raonsama/cowork-agent/internal/indexer"
)

// App is the top-level TUI controller.
type App struct {
	cfg         *config.Config
	initialTask string
	indexPath   string
	mode        appMode
}

type appMode int

const (
	appModeInteractive appMode = iota
	appModeCowork
	appModeIndex
)

// New creates a standard interactive App (REPL mode).
func New(cfg *config.Config) *App {
	return &App{cfg: cfg, mode: appModeInteractive}
}

// NewWithTask creates an App that immediately starts cowork mode.
func NewWithTask(cfg *config.Config, task string) *App {
	return &App{cfg: cfg, initialTask: task, mode: appModeCowork}
}

// NewIndexer creates an App that runs the file indexer.
func NewIndexer(cfg *config.Config, path string) *App {
	return &App{cfg: cfg, indexPath: path, mode: appModeIndex}
}

// Run starts the bubbletea program and blocks until exit.
func (a *App) Run() error {
	switch a.mode {
	case appModeIndex:
		return a.runIndexer()
	default:
		return a.runTUI()
	}
}

func (a *App) runTUI() error {
	ag, err := agent.New(a.cfg)
	if err != nil {
		return fmt.Errorf("agent init: %w", err)
	}
	defer ag.Close()

	model := newModel(ag, a.initialTask)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}

func (a *App) runIndexer() error {
	db, err := indexer.OpenDB(a.cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	idx := indexer.NewIndexer(db, a.cfg.IgnoredDirs, a.cfg.SupportedExts, 2)
	errCh := make(chan error, 1)
	go func() { errCh <- idx.IndexProject(a.indexPath) }()

	for prog := range idx.Progress() {
		if prog.Error != nil {
			fmt.Printf("  ⚠  %s\n", prog.Error)
		} else {
			bar := mkBar(prog.Done, prog.Total, 30)
			fmt.Printf("\r  %s  %d/%d files  %d symbols", bar, prog.Done, prog.Total, prog.Symbols)
		}
	}
	fmt.Println()
	if err := <-errCh; err != nil {
		return err
	}
	files, symbols := db.Stats()
	fmt.Printf("\n  ✅  Indexed %d files · %d symbols\n", files, symbols)
	return nil
}

func mkBar(done, total, width int) string {
	fill := func(s string, n int) string {
		var r strings.Builder
		for range n {
			r.WriteString(s)
		}
		return r.String()
	}
	if total == 0 {
		return "[" + fill("·", width) + "]"
	}
	f := min(width*done/total, width)
	return "[" + fill("█", f) + fill("·", width-f) + "]"
}
