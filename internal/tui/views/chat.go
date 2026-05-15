// Package views contains the individual UI panels rendered by the TUI model.
// This file implements ChatView: the conversation viewport and textarea input.
package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/raonsama/cowork-agent/internal/tui/styles"
)

// ChatMessage is a single message in the conversation display.
type ChatMessage struct {
	Role    string // "user" | "assistant" | "system"
	Content string
	Partial bool // true while streaming
}

// ChatView renders the conversation history and input box.
type ChatView struct {
	Width    int
	Height   int
	Messages []ChatMessage
	Input    textarea.Model
	Viewport viewport.Model
	Focused  bool
}

// NewChatView constructs a ChatView with sensible defaults.
func NewChatView(width, height int) ChatView {
	ta := textarea.New()
	ta.Placeholder = "Type a task… (Enter to send, Ctrl+C to quit)"
	ta.CharLimit = 4000
	ta.SetWidth(width - 4)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.Focus()

	vp := viewport.New(width-2, height-8)
	vp.SetContent("")

	return ChatView{
		Width:    width,
		Height:   height,
		Input:    ta,
		Viewport: vp,
		Focused:  true,
	}
}

// AppendMessage adds a complete message and refreshes the viewport.
func (cv *ChatView) AppendMessage(role, content string) {
	cv.Messages = append(cv.Messages, ChatMessage{Role: role, Content: content})
	cv.refreshViewport()
}

// StreamToken appends a token to the last assistant message (for live streaming).
func (cv *ChatView) StreamToken(token string) {
	if len(cv.Messages) == 0 || cv.Messages[len(cv.Messages)-1].Role != "assistant" {
		cv.Messages = append(cv.Messages, ChatMessage{Role: "assistant", Partial: true})
	}
	last := &cv.Messages[len(cv.Messages)-1]
	last.Content += token
	last.Partial = true
	cv.refreshViewport()
}

// FinalizeStream marks the last streaming message as complete.
func (cv *ChatView) FinalizeStream() {
	if len(cv.Messages) > 0 {
		cv.Messages[len(cv.Messages)-1].Partial = false
	}
	cv.refreshViewport()
}

// refreshViewport re-renders all messages and scrolls to the bottom.
func (cv *ChatView) refreshViewport() {
	cv.Viewport.SetContent(cv.renderMessages())
	cv.Viewport.GotoBottom()
}

// renderContent splits content on code fences, adding line numbers to code blocks.
func renderContent(content string, maxWidth int) string {
	if maxWidth < 4 {
		maxWidth = 4
	}
	var sb strings.Builder
	// Split on ``` boundaries; even indices = prose, odd = code
	parts := strings.Split(content, "```")
	for i, part := range parts {
		if i%2 == 0 {
			// Prose
			if part != "" {
				sb.WriteString(wrapText(part, maxWidth))
			}
		} else {
			// Code block: first line may be language tag
			before, after, ok := strings.Cut(part, "\n")
			lang := ""
			code := part
			if ok {
				lang = strings.TrimSpace(before)
				code = after
			}
			sb.WriteString(renderCodeBlock(lang, strings.TrimRight(code, "\n"), maxWidth))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// renderCodeBlock formats a code snippet with gutter line numbers.
func renderCodeBlock(lang, code string, maxWidth int) string {
	lines := strings.Split(code, "\n")
	numW := len(fmt.Sprintf("%d", len(lines))) // gutter width
	barW := maxWidth - numW - 3                // content area: " N │ code"

	var sb strings.Builder

	// Header bar  ──  lang  ────────────
	langLabel := lang
	if langLabel == "" {
		langLabel = "text"
	}
	headerPad := max(maxWidth-len(langLabel)-2, 0)
	sb.WriteString(
		styles.Muted.Render(" "+langLabel+strings.Repeat("─", headerPad)) + "\n",
	)

	for i, line := range lines {
		// Truncate long lines to avoid viewport overflow
		if len(line) > barW && barW > 3 {
			line = line[:barW-1] + "…"
		}
		gutter := styles.Muted.Render(fmt.Sprintf("%*d │ ", numW, i+1))
		code := styles.ReportCode.Render(line)
		sb.WriteString(gutter + code + "\n")
	}

	// Footer separator
	sb.WriteString(styles.Muted.Render(strings.Repeat("─", maxWidth)) + "\n")
	return sb.String()
}

// renderMessages re-renders all chat messages into a single string for the viewport.
func (cv *ChatView) renderMessages() string {
	var sb strings.Builder
	// Use viewport width for accurate wrapping; subtract bubble's left border + padding
	msgWidth := max(cv.Viewport.Width-2, 20)

	for _, m := range cv.Messages {
		switch m.Role {
		case "user":
			label := styles.UserLabel.Render("  you")
			content := renderContent(m.Content, msgWidth)
			bubble := styles.UserBubble.Width(msgWidth).Render(content)
			sb.WriteString(label + "\n" + bubble + "\n")

		case "assistant":
			label := styles.AssistantLabel.Render("  cowork")
			if m.Partial {
				label = styles.AssistantLabel.Render("  cowork ▌")
			}
			content := renderContent(m.Content, msgWidth)
			bubble := styles.AssistantBubble.Width(msgWidth).Render(content)
			sb.WriteString(label + "\n" + bubble + "\n")

		case "system":
			msg := styles.Muted.Render("  · " + m.Content)
			sb.WriteString(msg + "\n")
		}
	}
	return sb.String()
}

// Render draws the full chat view (viewport + input).

const (
	borderX = 2 // 1px border × 2 sisi AppBorder
	padX    = 2 // 1px padding × 2 sisi dari Panel style
)

func (cv *ChatView) Render() string {
	cv.Input.SetWidth(cv.Width - 8)
	sep := styles.Subtle.Render(strings.Repeat("─", cv.Width-2))

	inputLabel := styles.Subtle.Render("  ❯ ")
	inputBox := styles.InputBoxFocused.
		Width(cv.Width - borderX - padX - 2). // -2 = border input itu sendiri
		Render(cv.Input.View())

	return strings.Join([]string{
		cv.Viewport.View(),
		sep,
		inputLabel,
		inputBox,
	}, "\n")
}

// wrapText does simple word-wrap at maxWidth characters.
func wrapText(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	var result strings.Builder
	for line := range strings.SplitSeq(s, "\n") {
		if len(line) <= maxWidth {
			result.WriteString(line + "\n")
			continue
		}
		words := strings.Fields(line)
		cur := ""
		for _, w := range words {
			if len(cur)+len(w)+1 > maxWidth {
				result.WriteString(strings.TrimRight(cur, " ") + "\n")
				cur = ""
			}
			cur += w + " "
		}
		if cur != "" {
			result.WriteString(strings.TrimRight(cur, " ") + "\n")
		}
	}
	return strings.TrimRight(result.String(), "\n")
}
