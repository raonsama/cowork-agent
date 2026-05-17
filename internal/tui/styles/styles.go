// Package styles defines CoworkAgent's colour palette and Lipgloss primitives.
// Palette mirrors the Claude-Code–style TUI: dark blue-grey background,
// warm orange/amber border and accent throughout.
package styles

import "charm.land/lipgloss/v2"

// ── Colour palette ────────────────────────────────────────────────────────────

var (
	ColorBg       = lipgloss.Color("#1B1E2D") // main background
	ColorBorder   = lipgloss.Color("#C97038") // primary orange border / accent
	ColorBlue     = lipgloss.Color("#5B9BD5") // path / info segments
	ColorCyan     = lipgloss.Color("#39C5CF") // search / tool labels
	ColorGold     = lipgloss.Color("#D4A845") // cost / warning
	ColorGreen    = lipgloss.Color("#3CC46C") // success / context bar fill
	ColorMuted    = lipgloss.Color("#5C6370") // dimmed / disabled text
	ColorOrange   = lipgloss.Color("#D4834A") // header / title
	ColorOverlay  = lipgloss.Color("#0D0F1A") // semi-dark backdrop for popups
	ColorPurple   = lipgloss.Color("#BC8CFF") // planning phase
	ColorRed      = lipgloss.Color("#C94040") // errors
	ColorStatusBg = lipgloss.Color("#161925") // status bar background
	ColorSubtle   = lipgloss.Color("#8890A4") // secondary text
	ColorSurface  = lipgloss.Color("#1E2235") // panel / bubble surface
	ColorText     = lipgloss.Color("#D4D8E8") // primary text
)

// ── Base text styles ──────────────────────────────────────────────────────────

var (
	Accent = lipgloss.NewStyle().Foreground(ColorOrange).Bold(true)
	Base   = lipgloss.NewStyle().Foreground(ColorText)
	Blue   = lipgloss.NewStyle().Foreground(ColorBlue)
	Bold   = lipgloss.NewStyle().Foreground(ColorText).Bold(true)
	Muted  = lipgloss.NewStyle().Foreground(ColorMuted)
	Subtle = lipgloss.NewStyle().Foreground(ColorSubtle)
)

// ── Layout ────────────────────────────────────────────────────────────────────

var (
	// AppBorder wraps content panels with a thin rounded orange border.
	AppBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder)

	// HeaderBar renders the top "── CoworkAgent vX.Y ──" line.
	HeaderBar = lipgloss.NewStyle().
			Foreground(ColorOrange).
			Padding(0, 1)

	Panel = lipgloss.NewStyle().Padding(0, 1)

	PanelDivider = lipgloss.NewStyle().
			Foreground(ColorBorder)

	PanelDividerMute = lipgloss.NewStyle().
				Foreground(ColorMuted)

	PanelTitle = lipgloss.NewStyle().
			Foreground(ColorOrange).Bold(true).
			PaddingLeft(1).MarginBottom(1)
)

// ── Chat bubbles ──────────────────────────────────────────────────────────────

var (
	AssistantBubble = lipgloss.NewStyle().
			Foreground(ColorText).Background(ColorSurface).
			Padding(0, 1).MarginBottom(1).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(ColorGreen)

	AssistantLabel = lipgloss.NewStyle().Foreground(ColorGreen).Bold(true)

	InputPrompt = lipgloss.NewStyle().
			Background(ColorBg).
			Foreground(ColorSubtle).
			Bold(false)
	InputArea = lipgloss.NewStyle().
			Background(ColorBg).
			Padding(0, 2)
	InputPhaseLabel = lipgloss.NewStyle().
			Foreground(ColorMuted)

	UserBubble = lipgloss.NewStyle().
			Foreground(ColorText).Background(ColorSurface).
			Padding(0, 1).MarginBottom(1).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(ColorBlue)

	UserLabel = lipgloss.NewStyle().Foreground(ColorBlue).Bold(true)
)

// ── Status bar ────────────────────────────────────────────────────────────────

var (
	ContextBarEmpty = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorMuted)
	ContextBarFill  = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorGreen)

	StatusBar    = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorSubtle).Padding(0, 1)
	StatusBranch = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorGreen)
	StatusCost   = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorGold)
	StatusModel  = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorText).Bold(true)
	StatusPath   = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorSubtle)
	StatusSep    = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorBorder)

	ThinkOff = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorMuted)
	ThinkOn  = lipgloss.NewStyle().Background(ColorStatusBg).Foreground(ColorGreen).Bold(true)
)

// ── Popup / overlay ───────────────────────────────────────────────────────────

var (
	MenuBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(ColorBorder).
		Background(ColorSurface).Padding(0, 1)

	MenuItemDesc = lipgloss.NewStyle().Foreground(ColorMuted).PaddingLeft(2)

	MenuItem = lipgloss.NewStyle().
			Foreground(ColorText).PaddingLeft(1)

	MenuItemSelected = lipgloss.NewStyle().
				Background(ColorBorder).Foreground(ColorBg).
				Bold(true).PaddingLeft(1)
)

// ── Phase badges ──────────────────────────────────────────────────────────────

var PhaseBadge = map[string]lipgloss.Style{
	"committing": lipgloss.NewStyle().Foreground(ColorBlue).Bold(true),
	"done":       lipgloss.NewStyle().Foreground(ColorGreen).Bold(true),
	"error":      lipgloss.NewStyle().Foreground(ColorRed).Bold(true),
	"executing":  lipgloss.NewStyle().Foreground(ColorOrange).Bold(true),
	"idle":       Muted,
	"planning":   lipgloss.NewStyle().Foreground(ColorPurple).Bold(true),
	"searching":  lipgloss.NewStyle().Foreground(ColorCyan).Bold(true),
	"throttled":  lipgloss.NewStyle().Foreground(ColorGold).Bold(true),
	"verifying":  lipgloss.NewStyle().Foreground(ColorGold).Bold(true),
}

var PhaseIcon = map[string]string{
	"committing": "📦",
	"done":       "✓",
	"error":      "✗",
	"executing":  "⚙ ",
	"idle":       "·",
	"planning":   "🧠",
	"searching":  "🔍",
	"throttled":  "🌡",
	"verifying":  "🔬",
}

// ── Log panel ─────────────────────────────────────────────────────────────────

var (
	LogError     = lipgloss.NewStyle().Foreground(ColorRed)
	LogInfo      = lipgloss.NewStyle().Foreground(ColorSubtle)
	LogOutput    = lipgloss.NewStyle().Foreground(ColorMuted).PaddingLeft(2)
	LogSuccess   = lipgloss.NewStyle().Foreground(ColorGreen)
	LogTimestamp = lipgloss.NewStyle().Foreground(ColorMuted).PaddingRight(1)
	LogTool      = lipgloss.NewStyle().Foreground(ColorCyan).Bold(true)
	LogWarning   = lipgloss.NewStyle().Foreground(ColorGold)
)

// ── Code blocks ───────────────────────────────────────────────────────────────

var (
	CodeGutter = Muted
	CodeHeader = Muted
	CodeLine   = lipgloss.NewStyle().Background(ColorSurface).Foreground(ColorGreen).Padding(0, 1)
)

// ── Report ────────────────────────────────────────────────────────────────────

var (
	ReportCode  = lipgloss.NewStyle().Background(ColorSurface).Foreground(ColorGreen).Padding(0, 1)
	ReportTitle = lipgloss.NewStyle().Foreground(ColorOrange).Bold(true).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(ColorBorder).MarginBottom(1)
)

// ── Spinner ───────────────────────────────────────────────────────────────────

var SpinnerStyle = lipgloss.NewStyle().Foreground(ColorOrange)
