// Package views — menu.go implements two overlay components:
//
//	CommandMenu  → shown when user types "/" at start of input
//	FilePicker   → shown when user types "@" (file mention)
package views

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
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

// RenderPopup draws the command popup centered over maxW.
// Items are capped at 8 visible entries to avoid overflow on small terminals.
func (m *CommandMenu) RenderPopup(width int) string {
	items := m.Filtered()

	var sb strings.Builder

	// Title + filter hint dalam satu baris.
	title := styles.PanelTitle.Render("  Commands")
	hint := ""
	if m.Filter != "" {
		hint = styles.Muted.Render("  /" + m.Filter)
	} else {
		hint = styles.Muted.Render("  type to filter")
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Left, title, hint) + "\n\n")

	const maxVisible = 8
	visible := items
	if len(visible) > maxVisible {
		visible = visible[:maxVisible]
	}

	if len(visible) == 0 {
		sb.WriteString(styles.Muted.Render("  no commands match") + "\n")
	} else {
		for i, c := range visible {
			if i == m.Cursor {
				// Baris terpilih: background highlight, full width.
				row := styles.MenuItemSelected.
					Width(width - 4).
					Render(c.Name + "   " + c.Desc)
				sb.WriteString(row + "\n")
			} else {
				name := lipgloss.NewStyle().
					Foreground(styles.ColorOrange).Bold(true).
					Render("  " + c.Name)
				desc := styles.Muted.Render("   " + c.Desc)
				sb.WriteString(name + desc + "\n")
			}
		}
	}

	if len(items) > maxVisible {
		sb.WriteString("\n" + styles.Muted.Render(
			fmt.Sprintf("  … %d more", len(items)-maxVisible),
		) + "\n")
	}

	// Footer hint.
	sb.WriteString("\n" + styles.Muted.Render("  ↑↓ navigate   enter select   esc close"))

	return styles.MenuBox.Width(width).Render(strings.TrimRight(sb.String(), "\n"))
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

// RenderPopup draws the file picker popup centered over maxW.
func (fp *FilePicker) RenderPopup(width int) string {
	items := fp.Filtered()

	var sb strings.Builder

	title := styles.PanelTitle.Render("  Mention file")
	hint := ""
	if fp.Filter != "" {
		hint = styles.Muted.Render("  @" + fp.Filter)
	} else {
		hint = styles.Muted.Render("  type to filter")
	}
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Left, title, hint) + "\n\n")

	if len(items) == 0 {
		sb.WriteString(styles.Muted.Render("  no files match") + "\n")
	} else {
		const maxShow = 8
		start := max(fp.Cursor-maxShow+1, 0)
		end := min(start+maxShow, len(items))

		for i := start; i < end; i++ {
			if i == fp.Cursor {
				sb.WriteString(
					styles.MenuItemSelected.Width(width-4).Render(" "+items[i]) + "\n",
				)
			} else {
				sb.WriteString("  " + styles.Subtle.Render(items[i]) + "\n")
			}
		}

		if len(items) > maxShow {
			sb.WriteString("\n" + styles.Muted.Render(
				fmt.Sprintf("  … %d files total", len(items)),
			) + "\n")
		}
	}

	sb.WriteString("\n" + styles.Muted.Render("  ↑↓ navigate   enter insert   esc close"))

	return styles.MenuBox.Width(width).Render(strings.TrimRight(sb.String(), "\n"))
}
