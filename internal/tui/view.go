// Package tui — Bubble Tea View renderer and layout helpers.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/raonsama/cowork-agent/internal/tui/styles"
)

const (
	borderSides    = 2
	contentMarginX = 2
)

// View is the root Bubble Tea render entry point.
func (m Model) View() string {
	if !m.ready {
		return styles.Accent.Render("\n  Initializing CoworkAgent…")
	}

	// Overlays: dim the entire terminal, centre the popup over a dark backdrop.
	if m.menu.Visible {
		return m.renderOverlay(m.menu.RenderPopup(min(m.width-8, 60)))
	}
	if m.picker.Visible {
		return m.renderOverlay(m.picker.RenderPopup(min(m.width-8, 52)))
	}

	return m.renderBase()
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
	contentW := m.width - contentMarginX*2 - 2

	wrapper := lipgloss.NewStyle().PaddingLeft(contentMarginX)

	var body string

	if m.mode == ModeWelcome && len(m.chat.Messages) == 0 {
		headerH := lipgloss.Height(header)
		statusH := lipgloss.Height(statusBar)

		// Available vertical space between header and input box.
		innerH := m.height - headerH - statusH - inputSectH

		const naturalH = 18
		welcomeH := min(naturalH, max(innerH, 4))

		m.welcome.Width = m.width - 2
		m.welcome.Height = welcomeH

		// Place welcome block flush below the header (no centering gap).
		welcomeBlock := m.welcome.Render()
		body = wrapper.PaddingTop((innerH - welcomeH) / 2).Render(header + "\n" + welcomeBlock)
	} else {
		chatW := contentW
		logW := 0

		if m.showLogs && m.width >= 100 && len(m.logs.Entries) > 0 {
			logW = min(50, m.width/4)
			chatW = contentW - logW - 2
		}

		m.chat.Width = chatW
		m.chat.Height = panelOuterH
		m.chat.Viewport.Width = chatW - borderSides
		m.chat.Viewport.Height = panelInnerH

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

		body = wrapper.Render(strings.Join([]string{header, panels}, "\n"))
	}

	inputSection := wrapper.Render(m.renderInput(contentW))

	// Small top margin on the pre-input divider gives breathing room.
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
	m.input.SetWidth(width - 6)

	// Build the prompt glyph line: "›" + optional phase badge.
	promptGlyph := styles.InputPrompt.Render("")

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
