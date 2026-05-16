// Package tui — model.go contains the central Bubble Tea model.
// Responsibilities:
//   - Rendering the welcome screen (dual-panel) vs. active chat
//   - Dispatching "/" → command menu, "@" → file picker
//   - Managing input history ring-buffer (Up/Down arrow)
//   - Toggling Think Mode and Plan/Code Mode
//   - Proxying all agent events to the chat / log panel
package tui

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/raonsama/cowork-agent/internal/agent"
	"github.com/raonsama/cowork-agent/internal/thermal"
	"github.com/raonsama/cowork-agent/internal/tui/styles"
	"github.com/raonsama/cowork-agent/internal/tui/views"
	"github.com/raonsama/cowork-agent/pkg/reporter"
)

// ── Msg types ─────────────────────────────────────────────────────────────────

type (
	agentEventMsg agent.Event
	thermalMsg    thermal.Status
	tickMsg       time.Time
	fileListMsg   []string
)

// ── Mode ──────────────────────────────────────────────────────────────────────

// Mode controls which primary screen is active.
type Mode int

const (
	ModeWelcome Mode = iota // no messages yet — show dual-panel welcome
	ModeChat                // interactive chat / streaming
	ModeCowork              // agent executing autonomously
	ModeReport              // final report displayed (legacy, now folds into ModeChat)
)

// ── Model ─────────────────────────────────────────────────────────────────────

// Model is the central bubbletea model.
type Model struct {
	mode  Mode
	ag    *agent.Agent
	ready bool

	// Dimensions
	width, height int

	// ── Feature toggles ───────────────────────────────────
	thinkMode bool // Think / reasoning trace active
	planMode  bool // Plan/Code mode (milestones + code blocks)

	// ── Input & history ──────────────────────────────────
	input     textarea.Model
	inputHist []string // submitted prompts, FIFO ring
	histIdx   int      // -1 = live buffer; ≥0 = replaying history
	histBuf   string   // saves current draft while navigating history
	cancelFn  context.CancelFunc

	// ── Overlay menus ─────────────────────────────────────
	menu   views.CommandMenu
	picker views.FilePicker

	// ── Sub-views ─────────────────────────────────────────
	welcome views.WelcomeView
	chat    views.ChatView
	logs    views.LogPanel
	status  views.StatusBar
	spinner spinner.Model

	// ── Agent state ───────────────────────────────────────
	phase       string
	branch      string
	finalReport *reporter.Report
	err         error
	thermalSt   thermal.Status

	// ── Context stats ─────────────────────────────────────
	contextUsed int
	contextMax  int

	// ── Session cost (accumulated, mock in this skeleton) ─
	sessionCostUSD float64

	// ── Git / project ─────────────────────────────────────
	projectPath string
	gitBranch   string
	gitVersion  string

	// ── ESC double-tap ────────────────────────────────────
	lastEscAt time.Time

	// ── Manual log toggle ─────────────────────────────────
	userToggledLogs bool
	showLogs        bool
}

// ── Constructor ───────────────────────────────────────────────────────────────

func newModel(ag *agent.Agent, initialTask string) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Points
	sp.Style = styles.SpinnerStyle

	ta := textarea.New()
	ta.Placeholder = "Ask anything… (/ for commands, @ to mention files)"
	ta.CharLimit = 8000
	ta.SetHeight(2)
	ta.ShowLineNumbers = false
	ta.Focus()

	projectPath := ag.Cfg().ProjectRoot
	if abs, err := filepath.Abs(projectPath); err == nil {
		if home, err := os.UserHomeDir(); err == nil {
			if rel, err := filepath.Rel(home, abs); err == nil && !strings.HasPrefix(rel, "..") {
				projectPath = "~/" + rel
			}
		}
	}

	m := Model{
		mode:        ModeWelcome,
		ag:          ag,
		spinner:     sp,
		input:       ta,
		histIdx:     -1,
		phase:       "idle",
		showLogs:    false,
		contextMax:  ag.Cfg().ContextWindow,
		projectPath: projectPath,
		gitBranch:   readGitBranch(ag.Cfg().ProjectRoot),
		gitVersion:  readGitVersion(),
		menu:        views.NewCommandMenu(),
		picker:      views.NewFilePicker(nil),
		welcome:     views.NewWelcomeView(80, 20),
	}
	m.welcome.Username = "RAON" // override from config/env if available
	m.welcome.ThinkMode = m.thinkMode
	m.welcome.PlanMode = m.planMode
	m.welcome.ProjectPath = projectPath
	m.welcome.ModelName = ag.Cfg().DefaultModel

	m.chat = views.NewChatView(80, 24)

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
		loadFileList(m.ag.Cfg().ProjectRoot),
	)
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	// ── Window resize ─────────────────────────────────────────────────────────
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		if !m.userToggledLogs {
			m.showLogs = m.width >= 130
		}
		m.relayout()

	// ── File list (loaded asynchronously) ─────────────────────────────────────
	case fileListMsg:
		m.picker = views.NewFilePicker([]string(msg))

	// ── Keyboard ──────────────────────────────────────────────────────────────
	case tea.KeyMsg:
		// Overlays capture keys first.
		if m.menu.Visible {
			return m.updateMenu(msg)
		}
		if m.picker.Visible {
			return m.updateFilePicker(msg)
		}
		return m.updateNormal(msg)

		// ── Agent events ──────────────────────────────────────────────────────────
	case agentEventMsg:
		ev := agent.Event(msg)
		m.phase = string(ev.Phase)

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
				desc := fmt.Sprintf("⚙️  [%s] %s", ev.ToolName, ev.StepDesc)
				m.chat.AppendMessage("system", desc)
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
		case agent.PhaseError:
			if ev.Err != nil {
				m.err = ev.Err
				m.mode = ModeChat
				m.logs.Add(views.LevelError, "error", ev.Err.Error(), "")
				m.chat.AppendMessage("system", "✗ Error: "+ev.Err.Error())
				// Fix #1: restore focus so user can react.
				focusCmd := m.input.Focus()
				cmds = append(cmds, focusCmd)
			}
		}

		if ev.Final != nil {
			m.finalReport = ev.Final
			m.mode = ModeChat
			m.phase = "done"
			m.chat.AppendMessage("assistant", ev.Final.Markdown())
			// Fix #1: Focus() returns a blink-init cmd — must be batched.
			focusCmd := m.input.Focus()
			cmds = append(cmds, focusCmd)
		}

		// Streaming chat tokens (conversational mode).
		if ev.Phase == agent.PhaseIdle && ev.Message != "" && ev.ToolName == "" {
			m.chat.StreamToken(ev.Message)
		}

		if ev.Phase == agent.PhaseDone && ev.Final == nil {
			if ev.Message != "" {
				m.chat.FinalizeStream()
			}
			m.mode = ModeChat
			m.phase = "done"
			focusCmd := m.input.Focus()
			cmds = append(cmds, focusCmd)
		}

		// Log panel (unchanged).
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
		m.thermalSt = thermal.Status(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case tickMsg:
		cmds = append(cmds, tickEvery(2*time.Second), pollThermal(m.ag))
	}

	// Propagate scroll to chat viewport
	var vpCmd tea.Cmd
	m.chat.Viewport, vpCmd = m.chat.Viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	// Rebuild status bar every frame
	m.rebuildStatus()

	return m, tea.Batch(cmds...)
}

// updateNormal handles key events when no overlay is visible.
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
		label := "Think Mode OFF"
		if m.thinkMode {
			label = "Think Mode ON"
		}
		m.chat.AppendMessage("system", label)
		return m, nil

	case "ctrl+p":
		m.planMode = !m.planMode
		m.welcome.PlanMode = m.planMode
		label := "Plan/Code Mode OFF"
		if m.planMode {
			label = "Plan/Code Mode ON"
		}
		m.chat.AppendMessage("system", label)
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
		if (m.mode == ModeChat || m.mode == ModeWelcome) &&
			m.input.Line() == 0 && len(m.inputHist) > 0 {
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
		if (m.mode == ModeChat || m.mode == ModeWelcome) && m.histIdx >= 0 {
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
		if m.mode == ModeChat || m.mode == ModeWelcome {
			raw := strings.TrimSpace(m.input.Value())
			if raw == "" {
				break
			}
			m.input.Reset()
			m.histIdx = -1
			m.histBuf = ""

			if strings.HasPrefix(raw, "/") {
				if handled, cmd := m.execCommand(raw); handled {
					cmds = append(cmds, cmd)
					return m, tea.Batch(cmds...)
				}
			}

			m.pushHistory(raw)
			m.mode = ModeCowork
			m.chat.AppendMessage("user", raw)
			cmds = append(cmds, m.startCowork(raw))
			return m, tea.Batch(cmds...)
		}
	}

	// Default input handling — only when in an interactive mode.
	if m.mode == ModeChat || m.mode == ModeWelcome {
		if msg.String() == "/" && m.input.Value() == "" {
			m.menu.Visible = true
			m.menu.Filter = ""
			m.menu.Cursor = 0
			return m, nil
		}
		if msg.String() == "@" {
			m.picker.Visible = true
			m.picker.Filter = ""
			m.picker.Cursor = 0
			return m, nil
		}

		val := m.input.Value()
		if idx := strings.LastIndex(val, "@"); idx >= 0 && m.picker.Visible {
			after := val[idx+1:]
			if !strings.ContainsAny(after, " \t\n") {
				m.picker.Filter = after
			}
		}
		if strings.HasPrefix(val, "/") && m.menu.Visible {
			m.menu.Filter = val[1:]
		}

		var inputCmd tea.Cmd
		m.input, inputCmd = m.input.Update(msg)
		cmds = append(cmds, inputCmd)
	}

	return m, tea.Batch(cmds...)
}

// updateMenu handles keystrokes while the slash-command overlay is open.
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
			if handled, cmd := m.execCommandAction(sel.Action); handled {
				return m, cmd
			}
		}
	default:
		// Typing after "/" refines the filter
		if msg.Type == tea.KeyRunes {
			m.menu.Filter += msg.String()
		} else if msg.String() == "backspace" && len(m.menu.Filter) > 0 {
			m.menu.Filter = m.menu.Filter[:len(m.menu.Filter)-1]
		}
		if m.menu.Filter == "" && msg.String() == "backspace" {
			m.menu.Visible = false
		}
	}
	return m, nil
}

// updateFilePicker handles keystrokes while the "@" file picker overlay is open.
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
			val := strings.TrimRight(m.input.Value(), " ")
			m.input.SetValue(val + sel + " ")
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

// execCommand handles slash commands typed into the input field.
// Returns (true, cmd) if the command was handled (i.e. don't send to agent).
func (m *Model) execCommand(raw string) (bool, tea.Cmd) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return false, nil
	}
	return m.execCommandAction(strings.TrimPrefix(parts[0], "/"))
}

func (m *Model) execCommandAction(action string) (bool, tea.Cmd) {
	switch action {
	case "clear":
		m.chat.Messages = nil
		m.chat.Viewport.SetContent("")
		m.mode = ModeWelcome
		return true, nil
	case "think":
		m.thinkMode = !m.thinkMode
		m.welcome.ThinkMode = m.thinkMode
		label := "Think Mode OFF"
		if m.thinkMode {
			label = "Think Mode ON"
		}
		m.chat.AppendMessage("system", label)
		return true, nil
	case "plan":
		m.planMode = !m.planMode
		m.welcome.PlanMode = m.planMode
		label := "Plan/Code Mode OFF"
		if m.planMode {
			label = "Plan/Code Mode ON"
		}
		m.chat.AppendMessage("system", label)
		return true, nil
	case "help":
		help := "Shortcuts: ctrl+t Think · ctrl+p Plan · ctrl+l Logs · pgup/dn scroll · / commands · @ mention file · ↑↓ history"
		m.chat.AppendMessage("system", help)
		return true, nil
	case "resume":
		m.mode = ModeWelcome
		return true, nil
	}
	return false, nil
}

// pushHistory appends a prompt to the history ring-buffer (max 200 entries).
func (m *Model) pushHistory(prompt string) {
	const maxHist = 200
	m.inputHist = append(m.inputHist, prompt)
	if len(m.inputHist) > maxHist {
		m.inputHist = m.inputHist[len(m.inputHist)-maxHist:]
	}
}

// ── View ──────────────────────────────────────────────────────────────────────

const (
	borderSides    = 2
	inputH         = 7
	contentMarginX = 2 // left+right margin applied ONCE to the whole body
)

func (m Model) View() string {
	if !m.ready {
		return styles.Accent.Render("\n  Initializing Cowork Agent…")
	}

	header := m.renderHeader()
	statusBar := m.status.Render()
	// bodyH = ruang yang tersisa setelah header dan status bar.
	bodyH := m.height - lipgloss.Height(header) - lipgloss.Height(statusBar)
	contentW := (m.width - contentMarginX*2) - 2

	overlay := m.renderOverlay()
	overlayH := 0
	if overlay != "" {
		overlayH = lipgloss.Height(overlay)
	}

	// strings.Join(sections, "\n") menambah 1 "\n" per pasangan elemen.
	//   Tanpa overlay: panels \n input          → 1 separator
	//   Dengan overlay: panels \n overlay \n input → 2 separator
	separators := 1
	if overlayH > 0 {
		separators = 2
	}

	// panelOuterH = tinggi total panel (termasuk border AppBorder).
	// Rumus: bodyH = panelOuterH + separators + overlayH + inputH
	panelOuterH := max(bodyH-separators-overlayH-inputH, 4)
	// AppBorder.Height() menerima tinggi INNER; outer = inner + 2 (border atas+bawah).
	panelInnerH := panelOuterH - borderSides

	wrapper := lipgloss.NewStyle().PaddingLeft(contentMarginX)

	var body string
	var marginI int

	if m.mode == ModeWelcome && len(m.chat.Messages) == 0 {
		// Welcome: tinggi natural, bukan stretch penuh.
		const naturalH = 18
		marginI = 5
		welcomeH := min(naturalH, panelOuterH)
		m.welcome.Width = m.width - 2 // welcome.Render() subtracts 4 for border internally
		m.welcome.Height = welcomeH

		// Pusatkan welcome panel secara vertikal dalam ruang panelOuterH.
		topPad := max((panelOuterH-welcomeH)/2, 0)

		var sections []string
		if topPad > 0 {
			// Tambah baris kosong di atas (lipgloss.Height(" ") = 1 per baris)
			sections = append(sections, strings.Repeat(" \n", topPad))
		}
		sections = append(sections, header)
		sections = append(sections, m.welcome.Render())
		if overlay != "" {
			sections = append(sections, overlay)
		}

		body = wrapper.Render(strings.Join(sections, "\n"))

	} else {
		// Chat / Cowork mode.
		chatW := contentW
		logW := 0
		marginI = 0

		// Tampilkan log panel hanya jika ada entri DAN layar cukup lebar.
		if m.showLogs && m.width >= 100 && len(m.logs.Entries) > 0 {
			logW = min(50, m.width/4)
			chatW = contentW - logW - 2
		}

		m.chat.Width = chatW
		m.chat.Height = panelOuterH
		m.chat.Viewport.Width = chatW - borderSides
		m.chat.Viewport.Height = panelInnerH // viewport mengisi seluruh inner panel

		// AppBorder.Height(panelInnerH) → outer height = panelInnerH + 2 = panelOuterH.
		chatPanel := styles.AppBorder.
			Width(chatW).
			Height(panelInnerH).
			Render(m.chat.Render())

		var panels string
		if logW > 0 {
			m.logs.Width = logW
			m.logs.Height = panelOuterH
			logPanel := styles.AppBorder.
				Width(logW - 2).
				Height(panelInnerH).
				Render(m.logs.Render())
			panels = lipgloss.JoinHorizontal(lipgloss.Top, chatPanel, "  ", logPanel)
		} else {
			panels = chatPanel
		}

		var sections []string
		sections = append(sections, header)
		sections = append(sections, panels)
		if overlay != "" {
			sections = append(sections, overlay)
		}

		// strings.Join memastikan total body = panelOuterH + sep + overlayH + inputH = bodyH
		body = wrapper.Render(strings.Join(sections, "\n"))
	}
	inputSection := wrapper.Render(m.renderInput(contentW))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		body,
		m.renderDivider(marginI),
		inputSection,
		m.renderDivider(0),
		statusBar,
	)
}

func (m *Model) renderDivider(t int) string {
	lines := strings.Repeat("─", max(m.width, 0))
	return styles.PanelDividerMute.Margin(t, 0, 0, 0).Render(lines)
}

// renderOverlay produces the menu/picker string that sits ABOVE the input box.
func (m Model) renderOverlay() string {
	menuW := m.width - contentMarginX*2 - 4
	if m.menu.Visible {
		return m.menu.Render(menuW) + "\n"
	}
	if m.picker.Visible {
		return m.picker.Render(menuW) + "\n"
	}
	return ""
}

func (m Model) renderHeader() string {
	ver := m.gitVersion
	if ver == "" {
		ver = "dev"
	}
	line := styles.PanelDividerMute.PaddingLeft(1).Render("───")
	line1 := strings.Repeat("─", max(m.width-24-len(ver), 0))
	lines := styles.PanelDividerMute.Render(line1)
	coworkName := styles.HeaderBar.Render("Cowork Agent")
	version := styles.PanelDividerMute.Render(ver + " ")
	return lipgloss.JoinHorizontal(lipgloss.Left, line, coworkName, version, lines)
}

// renderInput renders the prompt + textarea box at the given outer width.
// No internal margin — the caller's wrapper provides the left offset.
func (m Model) renderInput(width int) string {
	// InputBox: border(1+1) + padding(1+1) = 4 consumed → textarea = width-4.
	m.input.SetWidth(width - 4)

	active := m.phase != "idle" && m.phase != "done" && m.phase != "error" && m.phase != ""
	phaseLabel := ""
	if active {
		phaseStyle, ok := styles.PhaseBadge[m.phase]
		icon := styles.PhaseIcon[m.phase]
		if !ok {
			phaseStyle = styles.Muted
		}
		phaseLabel = " " + styles.SpinnerStyle.Render(m.spinner.View()) +
			" " + phaseStyle.Render(icon+" "+m.phase)
	}

	prompt := styles.InputPrompt.Render("")
	return styles.InputBox.
		Width(width).
		Render(prompt + phaseLabel + "\n" + m.input.View())
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func (m *Model) relayout() {
	// Called on resize; View() reads current dimensions directly.
}

func (m *Model) rebuildStatus() {
	used := 0
	for _, msg := range m.chat.Messages {
		used += len([]rune(msg.Content))/4 + 4
	}
	m.contextUsed = used

	m.status = views.StatusBar{
		Width:       m.width,
		ProjectPath: m.projectPath,
		ModelName:   m.ag.Cfg().DefaultModel,
		ContextUsed: m.contextUsed,
		ContextMax:  m.contextMax,
		CostUSD:     m.sessionCostUSD,
		GitBranch:   m.gitBranch,
		ThinkMode:   m.thinkMode,
		PlanMode:    m.planMode,
		TempC:       m.thermalSt.TempCelsius,
		CPUPercent:  m.thermalSt.CPUPercent,
		Throttled:   m.thermalSt.Throttled,
	}
}

// ── Commands ──────────────────────────────────────────────────────────────────

func (m *Model) startCowork(task string) tea.Cmd {
	m.mode = ModeCowork
	m.phase = "planning"
	if m.cancelFn != nil {
		m.cancelFn()
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
	return func() tea.Msg { return thermalMsg(ag.ThermalMonitor().Current()) }
}

func tickEvery(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// loadFileList asynchronously walks the project root and returns a list of files.
func loadFileList(root string) tea.Cmd {
	return func() tea.Msg {
		var files []string
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error,
		) error {
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

// readGitBranch returns the current git branch name for the given directory.
func readGitBranch(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "main"
	}
	return strings.TrimSpace(string(out))
}

// readGitVersion returns the nearest git tag or short SHA.
func readGitVersion() string {
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return "dev"
	}
	return "v" + strings.TrimSpace(string(out))
}
