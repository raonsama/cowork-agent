// Package views — menu.go implements two overlay components:
//
//	CommandMenu  → shown when user types "/" at start of input
//	FilePicker   → shown when user types "@" (file mention)
package views

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/raonsama/cowork-agent/internal/tui/styles"
)

// ── Command Menu ──────────────────────────────────────────────────────────────

// Command is one entry in the slash-command menu.
type Command struct {
	Name string
	Desc string
	// Action is an opaque tag consumed by the model's Update.
	Action string
}

// DefaultCommands is the built-in slash-command list.
var DefaultCommands = []Command{
	{Name: "/clear", Desc: "Clear conversation history", Action: "clear"},
	{Name: "/help", Desc: "Show keybindings and tips", Action: "help"},
	{Name: "/think", Desc: "Toggle Think Mode on/off", Action: "think"},
	{Name: "/plan", Desc: "Toggle Plan/Code Mode on/off", Action: "plan"},
	{Name: "/model", Desc: "Switch active LLM model", Action: "model"},
	{Name: "/settings", Desc: "Open settings panel", Action: "settings"},
	{Name: "/resume", Desc: "Show recent activity", Action: "resume"},
}

// CommandMenu renders a floating popover above the input line.
type CommandMenu struct {
	Items   []Command
	Cursor  int
	Visible bool
	Filter  string // text after "/"
}

// NewCommandMenu builds a CommandMenu with the default commands.
func NewCommandMenu() CommandMenu {
	return CommandMenu{Items: DefaultCommands}
}

// Filtered returns Items matching the current Filter prefix.
func (m *CommandMenu) Filtered() []Command {
	if m.Filter == "" {
		return m.Items
	}
	var out []Command
	for _, c := range m.Items {
		if strings.HasPrefix(c.Name, "/"+m.Filter) {
			out = append(out, c)
		}
	}
	return out
}

// MoveUp moves the cursor up (wraps).
func (m *CommandMenu) MoveUp() {
	items := m.Filtered()
	if len(items) == 0 {
		return
	}
	m.Cursor = (m.Cursor - 1 + len(items)) % len(items)
}

// MoveDown moves the cursor down (wraps).
func (m *CommandMenu) MoveDown() {
	items := m.Filtered()
	if len(items) == 0 {
		return
	}
	m.Cursor = (m.Cursor + 1) % len(items)
}

// Selected returns the currently highlighted Command, or nil.
func (m *CommandMenu) Selected() *Command {
	items := m.Filtered()
	if len(items) == 0 || m.Cursor >= len(items) {
		return nil
	}
	c := items[m.Cursor]
	return &c
}

// Render draws the menu as a floating box; maxW constrains its width.
func (m *CommandMenu) Render(maxW int) string {
	items := m.Filtered()
	if len(items) == 0 || !m.Visible {
		return ""
	}

	var sb strings.Builder
	for i, c := range items {
		name := lipgloss.NewStyle().Foreground(styles.ColorOrange).Bold(true).Render(c.Name)
		desc := styles.Muted.Render("  " + c.Desc)
		line := name + desc
		if i == m.Cursor {
			line = styles.MenuItemSelected.Render(c.Name) + styles.Muted.Render("  "+c.Desc)
		}
		sb.WriteString("  " + line + "\n")
	}

	return styles.MenuBox.Width(min(maxW, 52)).Render(strings.TrimRight(sb.String(), "\n"))
}

// ── File Picker ───────────────────────────────────────────────────────────────

// FilePicker renders a floating list of files for "@" mention completion.
type FilePicker struct {
	Files   []string
	Cursor  int
	Visible bool
	Filter  string // text after "@"
}

// NewFilePicker creates a FilePicker with the given file list.
func NewFilePicker(files []string) FilePicker {
	return FilePicker{Files: files}
}

// Filtered returns files matching the current Filter prefix.
func (fp *FilePicker) Filtered() []string {
	if fp.Filter == "" {
		return fp.Files
	}
	var out []string
	filter := strings.ToLower(fp.Filter)
	for _, f := range fp.Files {
		if strings.Contains(strings.ToLower(f), filter) {
			out = append(out, f)
		}
	}
	return out
}

// MoveUp / MoveDown navigate the list.
func (fp *FilePicker) MoveUp() {
	items := fp.Filtered()
	if len(items) == 0 {
		return
	}
	fp.Cursor = (fp.Cursor - 1 + len(items)) % len(items)
}

func (fp *FilePicker) MoveDown() {
	items := fp.Filtered()
	if len(items) == 0 {
		return
	}
	fp.Cursor = (fp.Cursor + 1) % len(items)
}

// Selected returns the highlighted filename.
func (fp *FilePicker) Selected() string {
	items := fp.Filtered()
	if len(items) == 0 || fp.Cursor >= len(items) {
		return ""
	}
	return items[fp.Cursor]
}

// Render draws the file picker box.
func (fp *FilePicker) Render(maxW int) string {
	items := fp.Filtered()
	if len(items) == 0 || !fp.Visible {
		return ""
	}

	const maxShow = 8
	start := 0
	if fp.Cursor >= maxShow {
		start = fp.Cursor - maxShow + 1
	}
	end := min(start+maxShow, len(items))

	var sb strings.Builder
	sb.WriteString(styles.PanelTitle.Render(" @ Mention file") + "\n")

	for i := start; i < end; i++ {
		f := items[i]
		line := styles.Subtle.Render(f)
		if i == fp.Cursor {
			line = styles.MenuItemSelected.Render(" " + f)
		} else {
			line = "  " + line
		}
		sb.WriteString(line + "\n")
	}

	if len(items) > maxShow {
		sb.WriteString(styles.Muted.Render(
			"  … " + itoa(len(items)) + " files total\n",
		))
	}

	return styles.MenuBox.Width(min(maxW, 46)).Render(strings.TrimRight(sb.String(), "\n"))
}

func itoa(n int) string {
	if n < 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if s == "" {
		return "0"
	}
	return s
}
