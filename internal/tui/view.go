// Package tui — Bubble Tea View renderer and all layout helpers.
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/raonsama/cowork-agent/internal/tui/styles"
)

const (
	borderSides    = 2
	inputH         = 7
	contentMarginX = 2
)

func (m Model) View() string {
	if !m.ready {
		return styles.Accent.Render("\n  Initializing CoworkAgent…")
	}

	// Popup mode: tampilkan overlay centered, bukan inline.
	// Background disamakan dengan ColorBg agar transisi tidak mencolok.
	if m.menu.Visible {
		popup := m.menu.RenderPopup(min(m.width-8, 60))
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			popup,
			lipgloss.WithWhitespaceBackground(styles.ColorBg),
			lipgloss.WithWhitespaceChars(" "),
		)
	}
	if m.picker.Visible {
		popup := m.picker.RenderPopup(min(m.width-8, 52))
		return lipgloss.Place(
			m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			popup,
			lipgloss.WithWhitespaceBackground(styles.ColorBg),
			lipgloss.WithWhitespaceChars(" "),
		)
	}

	return m.renderBase()
}

func (m Model) renderBase() string {
	header := m.renderHeader()
	statusBar := m.status.Render()

	bodyH := m.height - lipgloss.Height(statusBar)
	panelOuterH := max(bodyH-lipgloss.Height(header)-1-inputH-1, 4)
	panelInnerH := panelOuterH - borderSides
	contentW := m.width - contentMarginX*2 - 2

	wrapper := lipgloss.NewStyle().PaddingLeft(contentMarginX)

	var body string
	var dividerTopMargin int

	if m.mode == ModeWelcome && len(m.chat.Messages) == 0 {
		headerH := lipgloss.Height(header)
		statusH := lipgloss.Height(statusBar)

		// innerH = ruang bersih antara bawah header dan atas input box.
		// 2 = dua divider (atas & bawah input).
		innerH := m.height - headerH - statusH - 2 - inputH

		const naturalH = 18
		welcomeH := min(naturalH, max(innerH, 4))

		m.welcome.Width = m.width - 2
		m.welcome.Height = welcomeH

		// Posisikan welcome box tepat di tengah innerH.
		topPad := max((innerH-welcomeH)/2, 0)

		welcomeBlock := lipgloss.NewStyle().
			PaddingTop(topPad).
			Render(m.welcome.Render())

		body = wrapper.Render(strings.Join([]string{header, welcomeBlock}, "\n"))
		dividerTopMargin = 0

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

		sections := []string{header, panels}
		body = wrapper.Render(strings.Join(sections, "\n"))
	}

	inputSection := wrapper.Render(m.renderInput(contentW))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		body,
		m.renderDivider(dividerTopMargin),
		inputSection,
		m.renderDivider(0),
		statusBar,
	)
}

func (m Model) renderHeader() string {
	ver := m.gitVersion
	if ver == "" {
		ver = "dev"
	}
	line := styles.PanelDividerMute.PaddingLeft(1).Render("───")
	fill := strings.Repeat("─", max(m.width-24-len(ver), 0))
	return lipgloss.JoinHorizontal(
		lipgloss.Left,
		line,
		styles.HeaderBar.Render("Cowork Agent"),
		styles.PanelDividerMute.Render(ver+" "),
		styles.PanelDividerMute.Render(fill),
	)
}

func (m Model) renderInput(width int) string {
	m.input.SetWidth(width - 4)

	active := m.phase != "" && m.phase != "idle" && m.phase != "done" && m.phase != "error"
	phaseLabel := ""
	if active {
		phaseStyle, ok := styles.PhaseBadge[m.phase]
		if !ok {
			phaseStyle = styles.Muted
		}
		icon := styles.PhaseIcon[m.phase]
		phaseLabel = " " + styles.SpinnerStyle.Render(m.spinner.View()) +
			" " + phaseStyle.Render(icon+" "+m.phase)
	}

	prompt := styles.InputPrompt.Render("")
	return styles.InputBox.
		Width(width).
		Render(prompt + phaseLabel + "\n" + m.input.View())
}

func (m Model) renderDivider(topMargin int) string {
	line := strings.Repeat("─", max(m.width, 0))
	return styles.PanelDividerMute.Margin(topMargin, 0, 0, 0).Render(line)
}
