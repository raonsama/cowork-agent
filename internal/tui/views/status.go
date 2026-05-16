// Package views — status.go implements the bottom status bar that matches
// the Claude-Code TUI screenshot layout:
//
//	[ path ]  [ model ]  Context: [████░░] 35%  [ cost ]  [ branch ]  ● mode
package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/raonsama/cowork-agent/internal/tui/styles"
)

// StatusBar holds all runtime data rendered in the bottom strip.
type StatusBar struct {
	Width int

	// Left
	ProjectPath string

	// Centre
	ModelName   string
	Phase       string
	ContextUsed int
	ContextMax  int
	CostUSD     float64

	// Right
	GitBranch string
	ThinkMode bool
	PlanMode  bool

	// Thermal
	TempC      float64
	CPUPercent float64
	Throttled  bool
}

const (
	barWidth = 14
)

// Render returns the full-width status bar string, rebuilt on every frame.
func (s *StatusBar) Render() string {
	sep := styles.StatusSep.Render(" │ ")

	// --- left cluster ---
	pathSeg := styles.StatusPath.Render("■ " + s.ProjectPath)
	modelSeg := styles.StatusModel.Render(s.ModelName)

	// Live phase badge — only shown when the agent is actively running.
	phaseSeg := ""
	if s.Phase != "" && s.Phase != "idle" && s.Phase != "done" && s.Phase != "error" {
		if ps, ok := styles.PhaseBadge[s.Phase]; ok {
			phaseSeg = sep + ps.Render(styles.PhaseIcon[s.Phase]+" "+s.Phase)
		}
	}

	ctxSeg := s.renderContext()
	costSeg := styles.StatusCost.Render(fmt.Sprintf("$%.4f", s.CostUSD))
	branchSeg := styles.StatusBranch.Render("⎇ " + s.GitBranch)

	thermalSeg := s.renderThermal(sep)
	modeSeg := s.renderMode()

	left := strings.Join([]string{
		pathSeg, sep, modelSeg, phaseSeg, sep, ctxSeg, sep, costSeg, sep, branchSeg, thermalSeg,
	}, "")

	// Pad between left and right.
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(modeSeg)
	pad := max(s.Width-leftW-rightW-2, 1)

	row := " " + left + strings.Repeat(" ", pad) + modeSeg + " "
	return styles.StatusBar.Width(s.Width).Render(row)
}

// renderContext produces "Context [████░░] 35%".
func (s *StatusBar) renderContext() string {
	if s.ContextMax == 0 {
		return styles.StatusPath.Render("Ctx –")
	}
	pct := min(float64(s.ContextUsed)/float64(s.ContextMax), 1.0)
	filled := int(pct * barWidth)
	empty := barWidth - filled

	bar := styles.ContextBarFill.Render(strings.Repeat("█", filled)) +
		styles.ContextBarEmpty.Render(strings.Repeat("░", empty))

	return styles.StatusPath.Render("Ctx [") + bar +
		styles.StatusPath.Render(fmt.Sprintf("] %d%%", int(pct*100)))
}

// renderThermal returns a thermal segment when temperature data is available.
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

// renderMode returns the rightmost think/plan indicator dot.
func (s *StatusBar) renderMode() string {
	if s.ThinkMode {
		return styles.ThinkOn.Render("● high /effort")
	}
	return styles.ThinkOff.Render("● no-think")
}
