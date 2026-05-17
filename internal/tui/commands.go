// Package tui — tea.Cmd helpers: agent lifecycle, polling, file scanning.
package tui

import (
	"context"
	"io/fs"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/raonsama/cowork-agent/internal/agent"
)

// startCowork cancels any in-flight run, opens a fresh event channel,
// launches the agent goroutine, and returns the first-event command.
func (m *Model) startCowork(task string) tea.Cmd {
	m.mode = ModeCowork
	m.phase = string(agent.PhasePlan)

	if m.cancelFn != nil {
		m.cancelFn()
		m.cancelFn = nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFn = cancel

	ch := m.ag.PrepareRun()
	go m.ag.Run(ctx, task)

	return waitForAgentEvent(ch)
}

// waitForAgentEvent blocks until one event arrives then delivers it as a Msg.
func waitForAgentEvent(ch <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return agentEventMsg{Phase: agent.PhaseDone}
		}
		return agentEventMsg(ev)
	}
}

// pollThermal reads the latest thermal snapshot synchronously (cheap).
func pollThermal(ag *agent.Agent) tea.Cmd {
	return func() tea.Msg {
		return thermalMsg(ag.ThermalMonitor().Current())
	}
}

// tickEvery fires a tickMsg at the given interval.
func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// loadFileList walks the project root asynchronously and returns all file paths.
func loadFileList(root string) tea.Cmd {
	return func() tea.Msg {
		var files []string
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			files = append(files, rel)
			return nil
		})
		return fileListMsg(files)
	}
}

// execCommand dispatches slash-command strings entered in the input field.
// Returns (true, cmd) when the command is consumed (i.e. skip agent dispatch).
func (m *Model) execCommand(raw string) (bool, tea.Cmd) {
	if len(raw) < 2 {
		return false, nil
	}
	action := raw[1:] // strip leading "/"
	return m.execCommandAction(action)
}

// execCommandAction maps a bare action string to in-model side effects.
func (m *Model) execCommandAction(action string) (bool, tea.Cmd) {
	switch action {
	case "clear":
		m.chat.Messages = nil
		m.chat.Viewport.SetContent("")
		m.mode = ModeWelcome
	case "think":
		m.thinkMode = !m.thinkMode
		m.welcome.ThinkMode = m.thinkMode
		m.chat.AppendMessage("system", boolLabel(m.thinkMode, "Think Mode ON", "Think Mode OFF"))
	case "plan":
		m.planMode = !m.planMode
		m.welcome.PlanMode = m.planMode
		m.chat.AppendMessage("system", boolLabel(m.planMode, "Plan/Code Mode ON", "Plan/Code Mode OFF"))
	case "help":
		m.chat.AppendMessage("system",
			"Shortcuts: ctrl+t Think · ctrl+p Plan · ctrl+l Logs · pgup/dn scroll · / commands · @ mention file · ↑↓ history")
	case "resume":
		m.mode = ModeWelcome
	default:
		return false, nil
	}
	return true, nil
}

func boolLabel(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}
