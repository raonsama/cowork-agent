// Package views — status.go renders the bottom status bar:
//
//	[path] │ [model] │ Context [████░░] 35% │ [$cost] │ [⎇ branch] │ ● mode
package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/raonsama/cowork-agent/internal/tui/styles"
)

const ctxBarWidth = 14

// StatusBar holds all runtime data needed to render the bottom strip.
// Rebuild this struct on every Update tick via model.rebuildStatus().
type StatusBar struct {
	// Thermal
	CPUPercent float64
	TempC      float64
	Throttled  bool

	// Context window
	ContextMax  int
	ContextUsed int

	// Session
	CostUSD   float64
	GitBranch string
	ModelName string

	// Active phase — drives the live phase badge.
	Phase string

	// Mode toggles
	PlanMode  bool
	ThinkMode bool

	// Layout
	ProjectPath string
	Width       int
}

// Render returns the full-width status bar, rebuilt every frame.
func (s *StatusBar) Render() string {
	sep := styles.StatusSep.Render(" │ ")

	pathSeg := styles.StatusPath.Render("■ " + s.ProjectPath)
	modelSeg := styles.StatusModel.Render(s.ModelName)
	phaseSeg := s.renderPhase(sep)
	ctxSeg := s.renderContext()
	costSeg := styles.StatusCost.Render(fmt.Sprintf("$%.4f", s.CostUSD))
	branchSeg := styles.StatusBranch.Render("⎇ " + s.GitBranch)
	thermalSeg := s.renderThermal(sep)
	modeSeg := s.renderMode()

	left := pathSeg + sep + modelSeg + phaseSeg + sep + ctxSeg + sep + costSeg + sep + branchSeg + thermalSeg
	pad := max(s.Width-lipgloss.Width(left)-lipgloss.Width(modeSeg)-2, 1)

	row := " " + left + strings.Repeat(" ", pad) + modeSeg + " "
	return styles.StatusBar.Width(s.Width).Render(row)
}

// renderPhase returns a coloured phase badge when the agent is active.
func (s *StatusBar) renderPhase(sep string) string {
	switch s.Phase {
	case "", "idle", "done", "error":
		return ""
	}
	ps, ok := styles.PhaseBadge[s.Phase]
	if !ok {
		return ""
	}
	return sep + ps.Render(styles.PhaseIcon[s.Phase]+" "+s.Phase)
}

// renderContext produces "Context [████░░] 35%".
func (s *StatusBar) renderContext() string {
	if s.ContextMax == 0 {
		return styles.StatusPath.Render("Ctx –")
	}
	pct := min(float64(s.ContextUsed)/float64(s.ContextMax), 1.0)
	filled := int(pct * ctxBarWidth)
	empty := ctxBarWidth - filled

	bar := styles.ContextBarFill.Render(strings.Repeat("█", filled)) +
		styles.ContextBarEmpty.Render(strings.Repeat("░", empty))

	return styles.StatusPath.Render("Context [") + bar +
		styles.StatusPath.Render(fmt.Sprintf("] %d%%", int(pct*100)))
}

// renderThermal appends temperature/CPU data when available.
func (s *StatusBar) renderThermal(sep string) string {
	if s.Throttled {
		return sep + styles.StatusCost.Render(
			fmt.Sprintf("🌡%.0f°C CPU%.0f%%", s.TempC, s.CPUPercent),
		)
	}
	if s.TempC > 0 {
		return sep + styles.StatusPath.Render(fmt.Sprintf("%.0f°C", s.TempC))
	}
	return ""
}

// renderMode renders the rightmost think/plan indicator dot.
func (s *StatusBar) renderMode() string {
	if s.ThinkMode {
		return styles.ThinkOn.Render("● high /effort")
	}
	return styles.ThinkOff.Render("● no-think")
}
