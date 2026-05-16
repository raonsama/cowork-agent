// Package tui — Bubble Tea Update handler and all keyboard/event sub-handlers.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/raonsama/cowork-agent/internal/agent"
	"github.com/raonsama/cowork-agent/internal/thermal"
	"github.com/raonsama/cowork-agent/internal/tui/views"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		if !m.userToggledLogs {
			m.showLogs = m.width >= 130
		}

	case fileListMsg:
		m.picker = views.NewFilePicker([]string(msg))

	case tea.KeyMsg:
		if m.menu.Visible {
			return m.updateMenu(msg)
		}
		if m.picker.Visible {
			return m.updateFilePicker(msg)
		}
		return m.updateNormal(msg)

	case agentEventMsg:
		ev := agent.Event(msg)
		m.phase = string(ev.Phase)

		if ev.Branch != "" {
			m.branch = ev.Branch
			m.gitBranch = ev.Branch
		}

		switch ev.Phase {
		case agent.PhasePlan:
			if ev.Message != "" {
				m.chat.AppendMessage("system", "🧠 "+ev.Message)
			}
		case agent.PhaseSearch:
			if ev.StepID > 0 && ev.Message != "" {
				m.chat.AppendMessage("system", "🔍 "+ev.Message)
			}
		case agent.PhaseExecute:
			if ev.ToolName != "" {
				m.chat.AppendMessage("system",
					fmt.Sprintf("⚙️  [%s] %s", ev.ToolName, ev.StepDesc))
			}
		case agent.PhaseVerify:
			if ev.Message != "" {
				m.chat.AppendMessage("system", "🔬 "+ev.Message)
			}
		case agent.PhaseCommit:
			if ev.Message != "" {
				m.chat.AppendMessage("system", "📦 "+ev.Message)
			}
		case agent.PhaseThrottle:
			if ev.Message != "" {
				m.chat.AppendMessage("system", "🌡 "+ev.Message)
			}
		case agent.PhaseIdle:
			// Streaming chat token from runChat — no badge, no tool label.
			if ev.Message != "" && ev.ToolName == "" {
				m.chat.StreamToken(ev.Message)
			}
		case agent.PhaseError:
			if ev.Err != nil {
				m.err = ev.Err
				m.mode = ModeChat
				m.phase = "error"
				m.logs.Add(views.LevelError, "error", ev.Err.Error(), "")
				m.chat.AppendMessage("system", "✗ Error: "+ev.Err.Error())
				cmds = append(cmds, m.input.Focus())
			}
		}

		if ev.Final != nil {
			m.finalReport = ev.Final
			m.mode = ModeChat
			m.phase = "done"
			m.chat.AppendMessage("assistant", ev.Final.Markdown())
			cmds = append(cmds, m.input.Focus())
		}

		// runChat done: finalize the streamed partial message.
		if ev.Phase == agent.PhaseDone && ev.Final == nil && ev.Message != "" {
			m.chat.FinalizeStream()
			m.mode = ModeChat
			m.phase = "done"
			cmds = append(cmds, m.input.Focus())
		}

		// Log every named event except raw streaming tokens.
		if ev.Message != "" && (ev.Phase != agent.PhaseIdle || ev.ToolName != "") {
			level := views.LevelInfo
			switch {
			case ev.Phase == agent.PhaseDone:
				level = views.LevelSuccess
			case ev.Phase == agent.PhaseError:
				level = views.LevelError
			case ev.ToolName != "":
				level = views.LevelTool
			}
			m.logs.Add(level, string(ev.Phase), ev.Message, ev.ToolOutput)
		}

		// Re-queue only while the agent run is still live.
		if ev.Phase != agent.PhaseDone && ev.Phase != agent.PhaseError {
			cmds = append(cmds, waitForAgentEvent(m.ag.Events()))
		}

	case thermalMsg:
		m.thermalSt = thermal.Status(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case tickMsg:
		cmds = append(cmds, tickEvery(2*time.Second), pollThermal(m.ag))
	}

	var vpCmd tea.Cmd
	m.chat.Viewport, vpCmd = m.chat.Viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	m.rebuildStatus()
	return m, tea.Batch(cmds...)
}

// updateNormal handles keys when no overlay is open.
func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg.String() {
	case "ctrl+c":
		if m.cancelFn != nil {
			m.cancelFn()
		}
		return m, tea.Quit

	case "esc":
		now := time.Now()
		if now.Sub(m.lastEscAt) < 500*time.Millisecond {
			if m.cancelFn != nil {
				m.cancelFn()
				m.cancelFn = nil
			}
			m.phase = "idle"
			m.mode = ModeChat
			m.logs.Add(views.LevelWarn, "idle", "Stopped (double ESC)", "")
			m.chat.AppendMessage("system", "Stopped.")
			m.lastEscAt = time.Time{}
		} else {
			m.lastEscAt = now
		}
		return m, nil

	case "ctrl+l":
		m.showLogs = !m.showLogs
		m.userToggledLogs = true
		return m, nil

	case "ctrl+t":
		m.thinkMode = !m.thinkMode
		m.welcome.ThinkMode = m.thinkMode
		m.chat.AppendMessage("system", boolLabel(m.thinkMode, "Think Mode ON", "Think Mode OFF"))
		return m, nil

	case "ctrl+p":
		m.planMode = !m.planMode
		m.welcome.PlanMode = m.planMode
		m.chat.AppendMessage("system", boolLabel(m.planMode, "Plan/Code Mode ON", "Plan/Code Mode OFF"))
		return m, nil

	case "pgup":
		m.logs.ScrollUp(5)
		m.chat.Viewport.ScrollUp(5)
		return m, nil

	case "pgdown":
		m.logs.ScrollDown(5)
		m.chat.Viewport.ScrollDown(5)
		return m, nil

	case "up":
		if m.input.Line() == 0 && len(m.inputHist) > 0 {
			if m.histIdx == -1 {
				m.histBuf = m.input.Value()
				m.histIdx = len(m.inputHist) - 1
			} else if m.histIdx > 0 {
				m.histIdx--
			}
			m.input.SetValue(m.inputHist[m.histIdx])
			return m, nil
		}
		var c tea.Cmd
		m.input, c = m.input.Update(msg)
		return m, c

	case "down":
		if m.histIdx >= 0 {
			m.histIdx++
			if m.histIdx >= len(m.inputHist) {
				m.histIdx = -1
				m.input.SetValue(m.histBuf)
			} else {
				m.input.SetValue(m.inputHist[m.histIdx])
			}
			return m, nil
		}
		var c tea.Cmd
		m.input, c = m.input.Update(msg)
		return m, c

	case "enter":
		raw := strings.TrimSpace(m.input.Value())
		if raw == "" {
			break
		}
		m.input.Reset()
		m.input.SetHeight(2)
		m.inputLineCount = 2
		m.histIdx = -1
		m.histBuf = ""

		if strings.HasPrefix(raw, "/") {
			if handled, cmd := m.execCommand(raw); handled {
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				return m, tea.Batch(cmds...)
			}
		}

		m.pushHistory(raw)
		m.chat.AppendMessage("user", raw)
		cmds = append(cmds, m.startCowork(raw))
		return m, tea.Batch(cmds...)

		// Buka popup — visibility flag ditangkap oleh View(), bukan dirender inline.
	case "/":
		if m.input.Value() == "" {
			m.menu.Visible = true
			m.menu.Filter = ""
			m.menu.Cursor = 0
			return m, nil
		}
	case "@":
		if m.input.Value() == "" {
			m.picker.Visible = true
			m.picker.Filter = ""
			m.picker.Cursor = 0
			return m, nil
		}
	}

	var inputCmd tea.Cmd
	m.input, inputCmd = m.input.Update(msg)
	cmds = append(cmds, inputCmd)

	newLineCount := strings.Count(m.input.Value(), "\n") + 1
	newLineCount = max(min(newLineCount, 8), 2)
	if newLineCount != m.inputLineCount {
		m.inputLineCount = newLineCount
		m.input.SetHeight(newLineCount)
	}

	return m, tea.Batch(cmds...)
}

// updateMenu handles keys while the "/" command overlay is open.
func (m Model) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.menu.Visible = false
	case "up":
		m.menu.MoveUp()
	case "down":
		m.menu.MoveDown()
	case "enter":
		if sel := m.menu.Selected(); sel != nil {
			m.menu.Visible = false
			m.input.Reset()
			m.execCommandAction(sel.Action)
		}
	case "backspace":
		if len(m.menu.Filter) == 0 {
			m.menu.Visible = false
		} else {
			m.menu.Filter = m.menu.Filter[:len(m.menu.Filter)-1]
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.menu.Filter += msg.String()
		}
	}
	return m, nil
}

// updateFilePicker handles keys while the "@" file picker overlay is open.
func (m Model) updateFilePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.picker.Visible = false
	case "up":
		m.picker.MoveUp()
	case "down":
		m.picker.MoveDown()
	case "enter":
		if sel := m.picker.Selected(); sel != "" {
			m.picker.Visible = false
			base := strings.TrimRight(m.input.Value(), " ")
			m.input.SetValue(base + sel + " ")
		}
	case "backspace":
		if len(m.picker.Filter) > 0 {
			m.picker.Filter = m.picker.Filter[:len(m.picker.Filter)-1]
		} else {
			m.picker.Visible = false
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.picker.Filter += msg.String()
		}
	}
	return m, nil
}
