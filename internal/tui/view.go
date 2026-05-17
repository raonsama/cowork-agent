// Package tui — Bubble Tea View renderer and layout helpers.
package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/raonsama/cowork-agent/internal/tui/styles"
)

const (
	borderSides    = 2
	contentMarginX = 2
	minContentW    = 20
	minInputW      = 10
)

// View is the root Bubble Tea render entry point.
func (m Model) View() tea.View {
	var content string
	switch {
	case !m.ready:
		content = styles.Accent.Render("\n  Initializing CoworkAgent…")
	case m.menu.Visible:
		content = m.renderOverlay(m.menu.RenderPopup(min(m.width-8, 60)))
	case m.picker.Visible:
		content = m.renderOverlay(m.picker.RenderPopup(min(m.width-8, 52)))
	default:
		content = m.renderBase()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// renderOverlay centres popup over a dimmed, stippled background.
func (m Model) renderOverlay(popup string) string {
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		popup,
	)
}

// renderBase composes the full layout: header → content → divider → input → divider → status.
func (m Model) renderBase() string {
	header := m.renderHeader()
	statusBar := m.status.Render()

	bodyH := m.height - lipgloss.Height(statusBar)
	inputSectH := m.dynamicInputH()
	panelOuterH := max(bodyH-lipgloss.Height(header)-1-inputSectH-1, 4)
	panelInnerH := panelOuterH - borderSides

	// Guard against narrow terminals: never let the content column go negative.
	contentW := max(m.width-contentMarginX*2-2, minContentW)

	wrapper := lipgloss.NewStyle().PaddingLeft(contentMarginX)

	var body string

	if m.mode == ModeWelcome && len(m.chat.Messages) == 0 {
		headerH := lipgloss.Height(header)
		statusH := lipgloss.Height(statusBar)

		innerH := m.height - headerH - statusH - inputSectH

		const naturalH = 18
		welcomeH := min(naturalH, max(innerH, 4))

		m.welcome.Width = max(m.width-2, 40)
		m.welcome.Height = welcomeH

		welcomeBlock := m.welcome.Render()

		// max(,0) prevents negative PaddingTop when terminal is very short.
		topPad := max((innerH-welcomeH)/2, 0)

		body = wrapper.PaddingTop(topPad).Render(header + "\n" + welcomeBlock)
	} else {
		chatW := contentW
		logW := 0

		if m.showLogs && m.width >= 100 && len(m.logs.Entries) > 0 {
			logW = min(50, m.width/4)

			// Guard: chatW must stay positive even if logW is unexpectedly large.
			chatW = max(contentW-logW-2, minContentW/2)
		}

		m.chat.Width = chatW
		m.chat.Height = panelOuterH
		m.chat.Viewport.SetWidth(max(chatW-borderSides, 1))
		m.chat.Viewport.SetHeight(max(panelInnerH, 1))

		chatPanel := styles.AppBorder.
			Width(chatW).
			Height(panelInnerH).
			Render(m.chat.Render())

		var panels string
		if logW > 0 {
			m.logs.Width = logW
			m.logs.Height = panelOuterH
			logPanel := styles.AppBorder.
				Width(max(logW-2, 1)).
				Height(panelInnerH).
				Render(m.logs.Render())
			panels = lipgloss.JoinHorizontal(lipgloss.Top, chatPanel, "  ", logPanel)
		} else {
			panels = chatPanel
		}

		body = wrapper.Render(strings.Join([]string{header, panels}, "\n"))
	}

	inputSection := wrapper.Render(m.renderInput(contentW))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		body,
		m.renderDivider(1),
		inputSection,
		m.renderDivider(0),
		statusBar,
	)
}

// renderHeader renders the full-width title rule: "── CoworkAgent vX.Y ────".
func (m Model) renderHeader() string {
	ver := m.gitVersion
	if ver == "" {
		ver = "dev"
	}
	left := styles.PanelDividerMute.PaddingLeft(1).Render("───")
	fill := strings.Repeat("─", max(m.width-23-len(ver), 0))
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		left,
		styles.HeaderBar.Render("CoworkAgent"),
		styles.PanelDividerMute.Render(ver+" "),
		styles.PanelDividerMute.Render(fill),
	)
}

// renderInput renders a minimal terminal-style prompt line with optional
// live phase badge, followed by the textarea on the next line.
func (m Model) renderInput(width int) string {
	// Guard: textarea width must be positive.
	m.input.SetWidth(max(width-6, minInputW))

	promptGlyph := styles.InputPrompt.Render("")

	phasePart := ""
	if active := m.phase != "" && m.phase != "idle" && m.phase != "done" && m.phase != "error"; active {
		phaseStyle, ok := styles.PhaseBadge[m.phase]
		if !ok {
			phaseStyle = styles.Muted
		}
		icon := styles.PhaseIcon[m.phase]
		phasePart = "  " + styles.SpinnerStyle.Render(m.spinner.View()) +
			" " + phaseStyle.Render(icon+" "+m.phase)
	}

	promptLine := promptGlyph + phasePart

	return styles.InputArea.
		Width(width).
		Render(promptLine + "\n" + m.input.View())
}

// renderDivider renders a full-width horizontal rule with an optional top margin.
func (m Model) renderDivider(topMargin int) string {
	line := strings.Repeat("─", max(m.width, 0))
	return lipgloss.NewStyle().
		Foreground(styles.ColorMuted).
		Margin(topMargin, 0, 0, 0).
		Render(line)
}
