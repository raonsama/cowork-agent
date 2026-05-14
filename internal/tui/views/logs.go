package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/raonsama/cowork-agent/internal/tui/styles"
)

// LogLevel indicates severity.
type LogLevel int

const (
	LevelInfo LogLevel = iota
	LevelSuccess
	LevelWarn
	LevelError
	LevelTool
)

// LogEntry is one line in the log panel.
type LogEntry struct {
	At      time.Time
	Level   LogLevel
	Phase   string
	Message string
	Detail  string
}

// LogPanel is a scrollable log view.
type LogPanel struct {
	Width    int
	Height   int
	Entries  []LogEntry
	scrollTo int
}

// Add appends a new log entry and auto-scrolls to the bottom.
func (lp *LogPanel) Add(level LogLevel, phase, message, detail string) {
	lp.Entries = append(lp.Entries, LogEntry{
		At:      time.Now(),
		Level:   level,
		Phase:   phase,
		Message: message,
		Detail:  detail,
	})
	if len(lp.Entries) > 500 {
		lp.Entries = lp.Entries[len(lp.Entries)-500:]
	}
	lp.scrollTo = len(lp.Entries) - 1
}

// ScrollUp moves the view up by n lines.
func (lp *LogPanel) ScrollUp(n int) {
	lp.scrollTo -= n
	if lp.scrollTo < 0 {
		lp.scrollTo = 0
	}
}

// ScrollDown moves the view down by n lines.
func (lp *LogPanel) ScrollDown(n int) {
	lp.scrollTo += n
	if lp.scrollTo >= len(lp.Entries) {
		lp.scrollTo = len(lp.Entries) - 1
	}
}

// Render produces the log panel string.
func (lp *LogPanel) Render() string {
	title := styles.PanelTitle.Render("  Logs")
	availH := lp.Height - 2 // subtract title + border

	rendered := make([]string, 0, len(lp.Entries))
	for _, e := range lp.Entries {
		rendered = append(rendered, lp.renderEntry(e))
	}

	// Trim to visible window
	visible := rendered
	if len(visible) > availH {
		start := lp.scrollTo - availH + 1
		if start < 0 {
			start = 0
		}
		end := start + availH
		if end > len(visible) {
			end = len(visible)
			start = end - availH
			if start < 0 {
				start = 0
			}
		}
		visible = visible[start:end]
	}

	body := strings.Join(visible, "\n")
	return styles.Panel.
		Width(lp.Width - 2). // kurangi 1px border × 2 dari AppBorder yang sudah wrap ini
		Height(lp.Height - 2).
		Render(title + "\n" + body)
}

func (lp *LogPanel) renderEntry(e LogEntry) string {
	ts := styles.LogTimestamp.Render(e.At.Format("15:04:05"))

	msgStyle := styles.LogInfo
	var prefix string
	switch e.Level {
	case LevelSuccess:
		msgStyle = styles.LogSuccess
		prefix = "✓ "
	case LevelWarn:
		msgStyle = styles.LogWarning
		prefix = "⚠ "
	case LevelError:
		msgStyle = styles.LogError
		prefix = "✗ "
	case LevelTool:
		msgStyle = styles.LogTool
		prefix = "⚙ "
	default:
		prefix = "· "
	}

	phase := ""
	if e.Phase != "" {
		pStyle, ok := styles.PhaseBadge[e.Phase]
		if ok {
			phase = pStyle.Render(fmt.Sprintf("[%s]", e.Phase)) + " "
		}
	}

	msg := ts + " " + phase + msgStyle.Render(prefix+e.Message)

	if e.Detail != "" {
		detail := trimLines(e.Detail, 3, lp.Width-6)
		msg += "\n" + styles.LogOutput.Render(detail)
	}

	return msg
}

// trimLines limits detail output to maxLines, each truncated to maxWidth.
func trimLines(s string, maxLines, maxWidth int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, "…")
	}
	for i, l := range lines {
		if len(l) > maxWidth {
			lines[i] = l[:maxWidth] + "…"
		}
	}
	return strings.Join(lines, "\n")
}
