// Package styles defines CoworkAgent's colour palette and Lipgloss primitives.
// Palette is extracted from the Claude-Code–style TUI screenshot:
//   - Dark blue-grey background, warm orange/amber border + accent.
package styles

import "github.com/charmbracelet/lipgloss"

// ── Palette ───────────────────────────────────────────────────────────────────

var (
	ColorBg       = lipgloss.Color("#1B1E2D") // main background
	ColorSurface  = lipgloss.Color("#1E2235") // panel / bubble surface
	ColorStatusBg = lipgloss.Color("#161925") // status bar background
	ColorBorder   = lipgloss.Color("#C97038") // orange border (primary accent)
	ColorMuted    = lipgloss.Color("#5C6370") // dimmed / disabled text
	ColorSubtle   = lipgloss.Color("#8890A4") // secondary text
	ColorText     = lipgloss.Color("#D4D8E8") // primary text
	ColorOrange   = lipgloss.Color("#D4834A") // header / title orange
	ColorGreen    = lipgloss.Color("#3CC46C") // success / context bar fill
	ColorBlue     = lipgloss.Color("#5B9BD5") // path / info blue
	ColorGold     = lipgloss.Color("#D4A845") // cost / warning gold
	ColorRed      = lipgloss.Color("#C94040") // error red
	ColorCyan     = lipgloss.Color("#39C5CF") // search / tool cyan
	ColorPurple   = lipgloss.Color("#BC8CFF") // planning purple
)

// ── Base styles ───────────────────────────────────────────────────────────────

var (
	Base   = lipgloss.NewStyle().Foreground(ColorText)
	Muted  = lipgloss.NewStyle().Foreground(ColorMuted)
	Subtle = lipgloss.NewStyle().Foreground(ColorSubtle)
	Bold   = lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	Accent = lipgloss.NewStyle().Foreground(ColorOrange).Bold(true)
	Blue   = lipgloss.NewStyle().Foreground(ColorBlue)
)

// ── Layout ────────────────────────────────────────────────────────────────────

var (
	// AppBorder wraps the dual-panel area — thin orange rounded border.
	AppBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder)

	Panel = lipgloss.NewStyle().Padding(0, 1)

	PanelTitle = lipgloss.NewStyle().
			Foreground(ColorOrange).Bold(true).
			PaddingLeft(1).MarginBottom(1)

	// HeaderBar — "── Cowork Agent vX.Y.Z ────────────"
	HeaderBar = lipgloss.NewStyle().
			Foreground(ColorOrange).
			Padding(0, 1)

	// PanelDivider — vertical separator between left and right panels.
	PanelDivider = lipgloss.NewStyle().
			Foreground(ColorBorder)
	PanelDividerMute = lipgloss.NewStyle().
				Foreground(ColorMuted)
)

// ── Chat bubbles ──────────────────────────────────────────────────────────────

var (
	UserBubble = lipgloss.NewStyle().
			Foreground(ColorText).Background(ColorSurface).
			Padding(0, 1).MarginBottom(1).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(ColorBlue)

	AssistantBubble = lipgloss.NewStyle().
			Foreground(ColorText).Background(ColorSurface).
			Padding(0, 1).MarginBottom(1).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(ColorGreen)

	UserLabel      = lipgloss.NewStyle().Foreground(ColorBlue).Bold(true)
	AssistantLabel = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)

	InputPrompt = lipgloss.NewStyle().Foreground(ColorOrange).Bold(true) // "› "
	InputBox    = lipgloss.NewStyle().
		// Border(lipgloss.RoundedBorder()).BorderForeground(ColorBorder).
		Padding(0, 1)
)

// ── Status bar ────────────────────────────────────────────────────────────────

var (
	StatusBar = lipgloss.NewStyle().
			Background(ColorStatusBg).Foreground(ColorSubtle).
			Padding(0, 1)

	StatusSep = lipgloss.NewStyle().
			Background(ColorStatusBg).Foreground(ColorBorder)

	StatusPath   = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorSubtle)
	StatusModel  = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorText).Bold(true)
	StatusCost   = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorGold)
	StatusBranch = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorGreen)

	// ThinkOn / ThinkOff — right-most mode indicator dot.
	ThinkOn  = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorGreen).Bold(true)
	ThinkOff = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorMuted)

	ContextBarFill  = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorGreen)
	ContextBarEmpty = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorMuted)
)

// ── Menu / overlay ────────────────────────────────────────────────────────────

var (
	MenuBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(ColorBorder).
		Background(ColorSurface).Padding(0, 1)

	MenuItem = lipgloss.NewStyle().
			Foreground(ColorText).PaddingLeft(1)

	MenuItemSelected = lipgloss.NewStyle().
				Background(ColorBorder).Foreground(ColorBg).
				Bold(true).PaddingLeft(1)

	MenuItemDesc = lipgloss.NewStyle().Foreground(ColorMuted).PaddingLeft(2)
)

// ── Phase badges ──────────────────────────────────────────────────────────────

var PhaseBadge = map[string]lipgloss.Style{
	"idle":       Muted,
	"planning":   lipgloss.NewStyle().Foreground(ColorPurple).Bold(true),
	"searching":  lipgloss.NewStyle().Foreground(ColorCyan).Bold(true),
	"executing":  lipgloss.NewStyle().Foreground(ColorOrange).Bold(true),
	"verifying":  lipgloss.NewStyle().Foreground(ColorGold).Bold(true),
	"committing": lipgloss.NewStyle().Foreground(ColorBlue).Bold(true),
	"done":       lipgloss.NewStyle().Foreground(ColorGreen).Bold(true),
	"error":      lipgloss.NewStyle().Foreground(ColorRed).Bold(true),
	"throttled":  lipgloss.NewStyle().Foreground(ColorGold).Bold(true),
}

var PhaseIcon = map[string]string{
	"idle": "·", "planning": "🧠", "searching": "🔍",
	"executing": "⚙ ", "verifying": "🔬", "committing": "📦",
	"done": "✓", "error": "✗", "throttled": "🌡",
}

// ── Log panel ─────────────────────────────────────────────────────────────────

var (
	LogTimestamp = lipgloss.NewStyle().Foreground(ColorMuted).PaddingRight(1)
	LogInfo      = lipgloss.NewStyle().Foreground(ColorSubtle)
	LogSuccess   = lipgloss.NewStyle().Foreground(ColorGreen)
	LogWarning   = lipgloss.NewStyle().Foreground(ColorGold)
	LogError     = lipgloss.NewStyle().Foreground(ColorRed)
	LogTool      = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	LogOutput    = lipgloss.NewStyle().Foreground(ColorMuted).PaddingLeft(2)
)

// ── Spinner ───────────────────────────────────────────────────────────────────

var SpinnerStyle = lipgloss.NewStyle().Foreground(ColorOrange)

// ── Code block ────────────────────────────────────────────────────────────────

var (
	CodeGutter = Muted
	CodeLine   = lipgloss.NewStyle().Background(ColorSurface).Foreground(ColorGreen).Padding(0, 1)
	CodeHeader = Muted
)

// ── Report ────────────────────────────────────────────────────────────────────

var (
	ReportTitle = lipgloss.NewStyle().Foreground(ColorOrange).Bold(true).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(ColorBorder).MarginBottom(1)
	ReportCode = lipgloss.NewStyle().Background(ColorSurface).Foreground(ColorGreen).Padding(0, 1)
)
