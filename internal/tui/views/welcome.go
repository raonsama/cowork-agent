// Package views — welcome.go renders the dual-panel welcome screen shown before
// the user sends their first message. Layout mirrors the Claude-Code TUI:
//
//	left panel  → identity (welcome text + mascot + model info)
//	right panel → recent activity + what's new
package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/raonsama/cowork-agent/internal/tui/styles"
)

// ActivityEntry is one line in the "Recent activity" list.
type ActivityEntry struct {
	When time.Time
	Desc string
}

// WhatsNewEntry is one bullet in the "What's new" section.
type WhatsNewEntry struct {
	Text string
}

// WelcomeView renders the full-width dual-panel welcome screen.
type WelcomeView struct {
	Width  int
	Height int

	// Identity
	Username    string
	ModelName   string
	ThinkMode   bool
	PlanMode    bool
	ProjectPath string

	// Content
	Activity []ActivityEntry
	WhatsNew []WhatsNewEntry
}

// NewWelcomeView constructs a WelcomeView with placeholder data.
func NewWelcomeView(w, h int) WelcomeView {
	return WelcomeView{
		Width: w, Height: h,
		Username:    "RAON",
		ModelName:   "qwen3.5-uncen:2b",
		ProjectPath: "~/project",
		Activity: []ActivityEntry{
			{When: time.Now().Add(-3 * time.Minute), Desc: "Updated status line configuration with cost..."},
			{When: time.Now().Add(-1 * time.Hour), Desc: "Fixed auth middleware to handle expired tokens..."},
			{When: time.Now().Add(-2 * time.Hour), Desc: "Created feature/auth branch and scaffolded..."},
		},
		WhatsNew: []WhatsNewEntry{
			{Text: "Added @ file mention — type @ to insert file context"},
			{Text: "Added / command menu — type / to see available commands"},
			{Text: "Think Mode toggle — reasoning trace is now collapsible"},
		},
	}
}

// robot returns the orange pixel-art mascot as a multi-line string.
func robot(color lipgloss.Color) string {
	orange := lipgloss.NewStyle().Foreground(color)
	lines := []string{
		" ▄██████▄ ",
		"███░░░░███",
		"███░██░███",
		"███░░░░███",
		" ▀██████▀ ",
		"  ██  ██  ",
		" ████████ ",
		" ██    ██ ",
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(orange.Render(l) + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// timeAgo formats a duration as "3m ago", "1h ago", etc.
func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// Render produces the full welcome string sized to Width × Height.
func (v *WelcomeView) Render() string {
	if v.Width < 40 {
		return styles.Accent.Render(" Welcome back " + v.Username + "!")
	}

	// Outer border wraps both panels.
	innerW := v.Width - 4  // 2px border + 2px outer pad
	innerH := v.Height - 2 // top/bottom border

	leftW := innerW * 2 / 5
	rightW := innerW - leftW - 1 // -1 for the divider

	left := v.renderLeft(leftW, innerH)
	divider := v.renderDivider(innerH)
	right := v.renderRight(rightW, innerH)

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)

	return styles.AppBorder.
		Width(v.Width - 4).
		Height(innerH).
		Render(body)
}

func (v *WelcomeView) renderLeft(w, h int) string {
	welcome := lipgloss.NewStyle().
		Bold(true).Foreground(styles.ColorText).
		Width(w).Align(lipgloss.Center).
		Render("Welcome back " + v.Username + "!")

	mascot := lipgloss.NewStyle().
		Width(w).Align(lipgloss.Center).
		Render(robot(styles.ColorOrange))

	modeLabel := styles.Muted.Render("No Think")
	if v.ThinkMode {
		modeLabel = styles.Blue.Render("Think")
	}
	if v.PlanMode {
		modeLabel += styles.Muted.Render(" · ") +
			lipgloss.NewStyle().Foreground(styles.ColorCyan).Render("Plan/Code")
	}

	modelLine := styles.Subtle.Render(v.ModelName) +
		styles.Muted.Render(" with ") + modeLabel
	pathLine := styles.Blue.Render(v.ProjectPath)

	info := lipgloss.NewStyle().Width(w).Align(lipgloss.Center).
		Render(modelLine + "\n" + pathLine)

	inner := lipgloss.JoinVertical(
		lipgloss.Center,
		welcome,
		"",
		mascot,
		"",
		info,
	)

	// Fix #4: vertically center the left panel block.
	innerH := lipgloss.Height(inner)
	topPad := max((h-innerH)/2, 0)
	btmPad := max(h-innerH-topPad, 0)

	return lipgloss.NewStyle().
		Width(w).
		PaddingTop(topPad).
		PaddingBottom(btmPad).
		Render(inner)
}

func (v *WelcomeView) renderDivider(h int) string {
	bar := strings.Repeat("│\n", h)
	return styles.PanelDivider.Render(strings.TrimRight(bar, "\n"))
}

func (v *WelcomeView) renderRight(w, h int) string {
	var sb strings.Builder

	// ── Recent activity ─────────────────────────────────
	sb.WriteString(styles.PanelTitle.Render("Recent activity") + "\n")

	for _, a := range v.Activity {
		ago := styles.Muted.Render(fmt.Sprintf("%-6s", timeAgo(a.When)))
		desc := styles.Subtle.Render(truncLine(a.Desc, w-9))
		sb.WriteString("  " + ago + "  " + desc + "\n")
	}
	sb.WriteString("  " + styles.Muted.Italic(true).Render("/resume for more") + "\n")

	sb.WriteString("\n")

	// ── What's new ──────────────────────────────────────
	sb.WriteString(styles.PanelTitle.Render("What's new") + "\n")

	for _, n := range v.WhatsNew {
		line := styles.Subtle.Render(truncLine(n.Text, w-4))
		sb.WriteString("  " + line + "\n")
	}
	sb.WriteString("  " + styles.Muted.Italic(true).Render("/release-notes for more") + "\n")

	return lipgloss.NewStyle().Width(w).Height(h).
		Padding(0, 1).Render(sb.String())
}

func truncLine(s string, max int) string {
	if max < 4 {
		return s
	}
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
