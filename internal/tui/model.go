// Package tui — this file contains the central Bubble Tea Model,
// handling all message dispatch, state transitions, and view composition
// for both interactive-chat and autonomous-cowork modes.
package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/raonsama/cowork-agent/internal/agent"
	"github.com/raonsama/cowork-agent/internal/thermal"
	"github.com/raonsama/cowork-agent/internal/tui/styles"
	"github.com/raonsama/cowork-agent/internal/tui/views"
	"github.com/raonsama/cowork-agent/pkg/reporter"
)

// ── Messages (bubbletea Msg types) ───────────────────────────────────────────
type (
	agentEventMsg agent.Event
	thermalMsg    thermal.Status
	tickMsg       time.Time
)

// ── Model ─────────────────────────────────────────────────────────────────────

// Mode controls what the TUI is currently showing.
type Mode int

const (
	ModeChat    Mode = iota // interactive chat
	ModeCowork              // autonomous cowork running
	ModeReport              // final wake-up report
	ModeIndexer             // indexing in progress
)

// Model is the central bubbletea model.
type Model struct {
	mode     Mode
	ag       *agent.Agent
	cancelFn context.CancelFunc

	// Dimensions
	width  int
	height int

	// Sub-views
	chat    views.ChatView
	logs    views.LogPanel
	status  views.StatusBar
	spinner spinner.Model

	// State
	phase       string
	stepCurrent int
	stepTotal   int
	branch      string
	baseBranch  string
	finalReport *reporter.Report
	err         error

	// Thermal
	thermalStatus thermal.Status

	// Flags
	showLogs bool
	ready    bool

	// double esc tracker
	lastEscAt time.Time
	escWindow time.Duration
}

func newModel(ag *agent.Agent, initialTask string) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Points
	sp.Style = styles.SpinnerStyle

	m := Model{
		mode:      ModeChat,
		ag:        ag,
		spinner:   sp,
		phase:     "idle",
		showLogs:  true,
		chat:      views.NewChatView(80, 24),
		escWindow: 500 * time.Millisecond,
	}

	if initialTask != "" {
		m.mode = ModeCowork
		m.chat.AppendMessage("user", initialTask)
	}
	return m
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		tickEvery(2*time.Second),
		// tea.EnterAltScreen,
	)
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.relayout()

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			now := time.Now()
			if now.Sub(m.lastEscAt) <= m.escWindow {
				if m.cancelFn != nil {
					m.cancelFn()
					m.cancelFn = nil
				}
				m.phase = "idle"
				m.mode = ModeChat
				m.logs.Add(views.LevelWarn, "idle", "Response stopped (double ESC)", "")
				m.chat.AppendMessage("system", "Stopped.")
				m.lastEscAt = time.Time{}
			} else {
				firstEsc := m.lastEscAt.IsZero() // hanya hint jika ini benar-benar pertama
				m.lastEscAt = now
				if m.mode == ModeCowork && firstEsc {
					m.chat.AppendMessage("system", "Press ESC again to stop…")
				}
			}
			return m, nil

		case "ctrl+c", "q":
			if m.cancelFn != nil {
				m.cancelFn()
			}
			return m, tea.Quit

		case "ctrl+l":
			m.showLogs = !m.showLogs
			m.relayout()

		case "pgup":
			m.logs.ScrollUp(5)

		case "pgdown":
			m.logs.ScrollDown(5)

		case "enter":
			if m.mode == ModeChat {
				input := m.chat.Input.Value()
				if input != "" {
					m.chat.Input.Reset()
					m.chat.AppendMessage("user", input)
					cmds = append(cmds, m.startCowork(input))
				}
			}

		default:
			if m.mode == ModeChat {
				var inputCmd tea.Cmd
				m.chat.Input, inputCmd = m.chat.Input.Update(msg)
				cmds = append(cmds, inputCmd)
			}
		}

	case agentEventMsg:
		ev := agent.Event(msg)
		m.phase = string(ev.Phase)
		if ev.StepID > 0 {
			m.stepCurrent = ev.StepID
		}

		// ── Final report ──────────────────────────────────
		if ev.Final != nil {
			m.finalReport = ev.Final
			m.mode = ModeReport
			m.phase = "done"
			m.chat.AppendMessage("assistant", ev.Final.Markdown())
		}

		// ── Error ─────────────────────────────────────────
		if ev.Err != nil {
			m.err = ev.Err
			m.phase = "error"
			m.logs.Add(views.LevelError, "error", ev.Err.Error(), "")
			m.chat.AppendMessage("system", "❌ "+ev.Err.Error())
		}

		// ── Chat streaming (conversational mode) ──────────
		// PhaseIdle + Message with no ToolName = streamed chat token.
		if ev.Phase == agent.PhaseIdle && ev.Message != "" && ev.ToolName == "" {
			m.chat.StreamToken(ev.Message)
		}

		// ── Done: finalize streamed chat bubble ───────────
		if ev.Phase == agent.PhaseDone && ev.Final == nil && ev.Message != "" {
			m.chat.FinalizeStream()
		}

		// ── Log panel entries (non-streaming) ─────────────
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

		if ev.Branch != "" {
			m.branch = ev.Branch
		}

		if ev.Phase != agent.PhaseDone && ev.Phase != agent.PhaseError {
			cmds = append(cmds, waitForAgentEvent(m.ag.Events()))
		}

	case thermalMsg:
		m.thermalStatus = thermal.Status(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case tickMsg:
		cmds = append(cmds, tickEvery(2*time.Second))
		// Poll thermal
		cmds = append(cmds, pollThermal(m.ag))
	}

	// Propagate viewport scroll in chat
	var vpCmd tea.Cmd
	m.chat.Viewport, vpCmd = m.chat.Viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	// Update status bar
	m.status = views.StatusBar{
		Width:           m.width,
		Phase:           m.phase,
		Model:           m.ag.Cfg().DefaultModel,
		Branch:          m.branch,
		TempC:           m.thermalStatus.TempCelsius,
		CPUPercent:      m.thermalStatus.CPUPercent,
		Throttled:       m.thermalStatus.Throttled,
		StepCurrent:     m.stepCurrent,
		StepTotal:       m.stepTotal,
		PlannerEnabled:  m.ag.Cfg().PlannerEnabled,
		VerifierEnabled: m.ag.Cfg().VerifierEnabled,
	}

	return m, tea.Batch(cmds...)
}

// ── View ──────────────────────────────────────────────────────────────────────

const (
	borderX = 2 // 1px border × 2 sisi AppBorder
	padX    = 2 // 1px padding × 2 sisi dari Panel style
	inputH  = 7 // 3 (textarea) + 1 (sep) + 1 (inputLabel) + 2 (rounded border)
)

func (m Model) View() string {
	if !m.ready {
		return styles.Accent.Render("\n  Initializing CoworkAgent…")
	}

	// Header
	header := m.renderHeader()

	// Body: chat + optional log panel
	var body string
	chatH := m.height - lipgloss.Height(header) - lipgloss.Height(m.status.Render()) - 1

	if m.showLogs {
		chatW := m.width * 7 / 10
		logW := (m.width - 4) - chatW - 1

		m.chat.Width = chatW
		m.chat.Height = chatH
		m.chat.Viewport.Width = chatW - borderX - padX
		m.chat.Viewport.Height = chatH - borderX - inputH - 2

		m.logs.Width = logW
		m.logs.Height = chatH

		chatPanel := styles.AppBorder.Width(chatW - 2).Height(chatH - 2).Render(m.chat.Render())
		logPanel := styles.AppBorder.Width(logW - 2).Height(chatH - 2).Render(m.logs.Render())
		body = lipgloss.JoinHorizontal(lipgloss.Top, chatPanel, " ", logPanel)
	} else {
		m.chat.Width = m.width
		m.chat.Height = chatH
		m.chat.Viewport.Width = m.width - borderX - padX
		m.chat.Viewport.Height = chatH - 6

		body = styles.AppBorder.Width(m.width - 2).Height(chatH - 2).Render(m.chat.Render())
	}

	content := lipgloss.NewStyle().
		Padding(0, 2).
		Render(lipgloss.JoinVertical(lipgloss.Left, body, m.status.Render()))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		content,
	)
}

func (m Model) renderHeader() string {
	title := styles.Accent.Render("  CoworkAgent")
	sub := styles.Muted.Render(" — Senior Ghost Developer  ")

	hint := styles.Muted.Render("ctrl+l logs · pgup/dn scroll · q quit")

	spin := ""
	if m.phase != "idle" && m.phase != "done" && m.phase != "error" {
		spin = " " + styles.SpinnerStyle.Render(m.spinner.View())
	}

	left := title + sub + spin
	right := hint

	pad := max(m.width-lipgloss.Width(left)-lipgloss.Width(right)-4, 0)

	row := left + fmt.Sprintf("%*s", pad, "") + right
	return styles.HeaderBar.Width(m.width).Render(row)
}

func (m *Model) relayout() {
	// Called on window resize — let View() handle dimensions dynamically.
}

// ── Commands ──────────────────────────────────────────────────────────────────

func (m *Model) startCowork(task string) tea.Cmd {
	m.mode = ModeCowork
	m.phase = "planning"
	m.stepTotal = 0
	m.stepCurrent = 0

	if m.cancelFn != nil {
		m.cancelFn() // terminasi run sebelumnya
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelFn = cancel

	ch := m.ag.PrepareRun()
	go m.ag.Run(ctx, task)

	return waitForAgentEvent(ch)
}

func waitForAgentEvent(ch <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return agentEventMsg{Phase: agent.PhaseDone}
		}
		return agentEventMsg(ev)
	}
}

func pollThermal(ag *agent.Agent) tea.Cmd {
	return func() tea.Msg {
		return thermalMsg(ag.ThermalMonitor().Current())
	}
}

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
