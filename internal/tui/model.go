// Package tui — Model struct, constructor, Init, and stateful helpers.
// Update, View, and tea.Cmd helpers live in update.go, view.go, commands.go.
package tui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/raonsama/cowork-agent/internal/agent"
	"github.com/raonsama/cowork-agent/internal/thermal"
	"github.com/raonsama/cowork-agent/internal/tui/styles"
	"github.com/raonsama/cowork-agent/internal/tui/views"
	"github.com/raonsama/cowork-agent/pkg/reporter"
)

// ── Screen mode ───────────────────────────────────────────────────────────────

type Mode int

const (
	ModeWelcome Mode = iota // no messages — show dual-panel welcome
	ModeChat                // interactive chat / streaming response
	ModeCowork              // agent executing autonomously
	ModeReport              // legacy alias, folds into ModeChat
)

// ── Model ─────────────────────────────────────────────────────────────────────

type Model struct {
	// Core
	mode  Mode
	ag    *agent.Agent
	ready bool

	// Terminal dimensions
	width, height int

	// Feature toggles
	thinkMode bool
	planMode  bool

	// Input and history ring-buffer
	input     textarea.Model
	inputHist []string
	histIdx   int    // -1 = live; ≥0 = replaying history
	histBuf   string // draft saved while navigating history
	cancelFn  context.CancelFunc

	// Overlays
	menu   views.CommandMenu
	picker views.FilePicker

	// Sub-views
	welcome views.WelcomeView
	chat    views.ChatView
	logs    views.LogPanel
	status  views.StatusBar
	spinner spinner.Model

	// Agent state
	phase       string
	branch      string
	finalReport *reporter.Report
	err         error
	thermalSt   thermal.Status

	// Context window tracking
	contextUsed int
	contextMax  int

	// Project / git metadata
	projectPath string
	gitBranch   string
	gitVersion  string

	// Session cost (populated by provider when billing data is available)
	sessionCostUSD float64

	// ESC double-tap detection
	lastEscAt time.Time

	// Log panel visibility
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

	projectPath := resolveProjectPath(ag.Cfg().ProjectRoot)

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

	m.welcome.Username = "RAON"
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

// ── Helpers ───────────────────────────────────────────────────────────────────

// pushHistory appends a prompt to the history ring-buffer (max 200 entries).
func (m *Model) pushHistory(prompt string) {
	const maxHist = 200
	m.inputHist = append(m.inputHist, prompt)
	if len(m.inputHist) > maxHist {
		m.inputHist = m.inputHist[len(m.inputHist)-maxHist:]
	}
}

// rebuildStatus recomputes the status bar from current model state.
// Called on every Update tick so the bar is always fresh.
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
		GitBranch:   m.gitBranch, // updated live in Update on branch events
		ThinkMode:   m.thinkMode,
		PlanMode:    m.planMode,
		TempC:       m.thermalSt.TempCelsius,
		CPUPercent:  m.thermalSt.CPUPercent,
		Throttled:   m.thermalSt.Throttled,
	}
}

// resolveProjectPath converts a root path to a ~-relative display string.
func resolveProjectPath(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return root
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return abs
	}
	rel, err := filepath.Rel(home, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return abs
	}
	return "~/" + rel
}

// readGitBranch returns the current branch name for dir, or "main" on error.
func readGitBranch(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "main"
	}
	return strings.TrimSpace(string(out))
}

// readGitVersion returns the nearest git tag or short SHA for display.
func readGitVersion() string {
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return "dev"
	}
	return "v" + strings.TrimSpace(string(out))
}
