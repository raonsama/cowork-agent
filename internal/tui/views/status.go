package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/raonsama/cowork-agent/internal/tui/styles"
)

// StatusBar renders the bottom status strip.
type StatusBar struct {
	Width       int
	Phase       string
	Model       string
	Branch      string
	TempC       float64
	CPUPercent  float64
	Throttled   bool
	TokensUsed  int
	TokensMax   int
	StepCurrent int
	StepTotal   int
}

// Render produces the status bar string for the current terminal width.
func (s *StatusBar) Render() string {
	// Left: phase badge + step counter
	icon := styles.PhaseIcon[s.Phase]
	if icon == "" {
		icon = "·"
	}
	phaseStyle, ok := styles.PhaseBadge[s.Phase]
	if !ok {
		phaseStyle = styles.Subtle
	}
	phaseLabel := phaseStyle.Render(fmt.Sprintf("%s %s", icon, strings.ToUpper(s.Phase)))

	stepInfo := ""
	if s.StepTotal > 0 {
		stepInfo = styles.Subtle.Render(fmt.Sprintf("  step %d/%d", s.StepCurrent, s.StepTotal))
	}

	left := phaseLabel + stepInfo

	// Center: model + branch
	model := styles.Subtle.Render(s.Model)
	branch := ""
	if s.Branch != "" {
		branch = styles.Muted.Render("  " + branchIcon + " " + s.Branch)
	}
	center := model + branch

	// Right: thermal + tokens
	thermal := ""
	if s.Throttled {
		thermal = styles.StatusThrottle.Width(18).Render(fmt.Sprintf("🌡 %.0f°C THROTTLED", s.TempC))
	} else if s.TempC > 0 {
		thermal = styles.Muted.Width(18).Render(fmt.Sprintf("%.0f°C  CPU %.0f%%", s.TempC, s.CPUPercent))
	}

	tokens := ""
	if s.TokensMax > 0 {
		pct := int(float64(s.TokensUsed) / float64(s.TokensMax) * 100)
		tStyle := styles.TokenUsage
		if pct > 80 {
			tStyle = styles.StatusWarn
		}
		tokens = tStyle.Render(fmt.Sprintf("  ctx %d%%", pct))
	}
	right := thermal + tokens

	// Assemble with padding
	leftW := lipgloss.Width(left)
	centerW := lipgloss.Width(center)
	rightW := lipgloss.Width(right)

	totalUsed := leftW + centerW + rightW
	padLeft := (s.Width - totalUsed) / 2
	padRight := s.Width - totalUsed - padLeft
	if padLeft < 1 {
		padLeft = 1
	}
	if padRight < 1 {
		padRight = 1
	}

	row := left +
		strings.Repeat(" ", padLeft) +
		center +
		strings.Repeat(" ", padRight) +
		right

	return styles.StatusBar.Width(s.Width).Render(row)
}

const branchIcon = ""
