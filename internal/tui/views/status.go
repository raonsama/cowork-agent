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

// StatusBar holds all data needed to render the bottom status strip.
type StatusBar struct {
	Width int

	// Left segment
	ProjectPath string // e.g. "~/my-project"

	// Center segments
	ModelName   string // e.g. "Opus 4.6"
	ContextUsed int    // tokens used
	ContextMax  int    // tokens available
	CostUSD     float64

	// Right segments
	GitBranch string // e.g. "feature/auth(+3)"
	ThinkMode bool   // true = "high effort" / "think"
	PlanMode  bool

	// Thermal (shown in muted if not throttled)
	TempC      float64
	CPUPercent float64
	Throttled  bool
}

const (
	sepChar  = " | "
	barWidth = 14
)

// Render returns the full-width status bar string.
// Fix #3: add thermal indicator when throttled; all segments sourced from
// live StatusBar fields rebuilt every tick via rebuildStatus().
func (s *StatusBar) Render() string {
	sep := styles.StatusSep.Render(sepChar)

	pathSeg := styles.StatusPath.Render("■ " + s.ProjectPath)
	modelSeg := styles.StatusModel.Render(s.ModelName)
	ctxSeg := s.renderContext()
	costSeg := styles.StatusCost.Render(fmt.Sprintf("$ Cost:$%.2f", s.CostUSD))
	branchSeg := styles.StatusBranch.Render("🌿 " + s.GitBranch)

	// Fix #3: show live temperature / CPU when throttled.
	thermalSeg := ""
	if s.Throttled {
		thermalSeg = sep + styles.StatusCost.Render(
			fmt.Sprintf("🌡%.0f°C  CPU%.0f%%", s.TempC, s.CPUPercent),
		)
	} else if s.TempC > 0 {
		thermalSeg = sep + styles.StatusPath.Render(
			fmt.Sprintf("%.0f°C", s.TempC),
		)
	}

	modeSeg := s.renderMode()

	left := pathSeg + sep + modelSeg + sep + ctxSeg + sep + costSeg + sep + branchSeg + thermalSeg
	right := modeSeg

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	pad := max(s.Width-leftW-rightW-2, 1)

	row := " " + left + strings.Repeat(" ", pad) + right + " "
	return styles.StatusBar.Width(s.Width).Render(row)
}

// renderContext produces "Context: [████░░░░░░] 35%".
func (s *StatusBar) renderContext() string {
	if s.ContextMax == 0 {
		return styles.StatusPath.Render("Context: –")
	}
	pct := float64(s.ContextUsed) / float64(s.ContextMax)
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * barWidth)
	empty := barWidth - filled

	bar := styles.ContextBarFill.Render(strings.Repeat("█", filled)) +
		styles.ContextBarEmpty.Render(strings.Repeat("░", empty))

	pctLabel := styles.StatusPath.Render(fmt.Sprintf(" %d%%", int(pct*100)))

	return styles.StatusPath.Render("Context: [") + bar +
		styles.StatusPath.Render("]") + pctLabel
}

// renderMode produces the right-most "● high /effort" or "● no-think" indicator.
func (s *StatusBar) renderMode() string {
	if s.ThinkMode {
		dot := styles.ThinkOn.Render("●")
		label := styles.ThinkOn.Render(" high /effort")
		return dot + label
	}
	dot := styles.ThinkOff.Render("●")
	label := styles.ThinkOff.Render(" no-think")
	return dot + label
}
