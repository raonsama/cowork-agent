package styles

import "github.com/charmbracelet/lipgloss"

// ── Palette ───────────────────────────────────────────────────────────────────
// Dark terminal palette inspired by claude-code / crush aesthetic.

var (
	ColorBg      = lipgloss.Color("#0D1117") // near-black background
	ColorSurface = lipgloss.Color("#161B22") // panel surface
	ColorBorder  = lipgloss.Color("#30363D") // subtle border
	ColorMuted   = lipgloss.Color("#484F58") // dimmed text
	ColorText    = lipgloss.Color("#C9D1D9") // primary text
	ColorSubtle  = lipgloss.Color("#8B949E") // secondary text
	ColorAccent  = lipgloss.Color("#58A6FF") // blue accent (claude-blue)
	ColorGreen   = lipgloss.Color("#3FB950") // success
	ColorYellow  = lipgloss.Color("#D29922") // warning / throttle
	ColorRed     = lipgloss.Color("#F85149") // error
	ColorPurple  = lipgloss.Color("#BC8CFF") // planning phase
	ColorCyan    = lipgloss.Color("#39C5CF") // search phase
	ColorOrange  = lipgloss.Color("#FFA657") // execute phase
)

// ── Base styles ───────────────────────────────────────────────────────────────

var (
	Base = lipgloss.NewStyle().
		Foreground(ColorText)

	Muted = lipgloss.NewStyle().
		Foreground(ColorMuted)

	Subtle = lipgloss.NewStyle().
		Foreground(ColorSubtle)

	Bold = lipgloss.NewStyle().
		Foreground(ColorText).
		Bold(true)

	Accent = lipgloss.NewStyle().
		Foreground(ColorAccent).
		Bold(true)
)

// ── Layout ────────────────────────────────────────────────────────────────────

var (
	AppBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder)

	Panel = lipgloss.NewStyle().
		// Background(ColorSurface).
		Padding(0, 1)

	PanelTitle = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true).
			PaddingLeft(1).
			MarginBottom(1)

	HeaderBar = lipgloss.NewStyle().
		// Background(ColorSurface).
		Padding(0, 1).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(ColorBorder)
)

// ── Chat ──────────────────────────────────────────────────────────────────────

var (
	UserBubble = lipgloss.NewStyle().
			Foreground(ColorText).
			Background(lipgloss.Color("#1C2128")).
			Padding(0, 1).
			MarginBottom(1).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(ColorAccent)

	AssistantBubble = lipgloss.NewStyle().
			Foreground(ColorText).
			Background(ColorSurface).
			Padding(0, 1).
			MarginBottom(1).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(ColorGreen)

	UserLabel = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	AssistantLabel = lipgloss.NewStyle().
			Foreground(ColorGreen).
			Bold(true)

	InputBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent).
			Padding(0, 1)

	InputBoxFocused = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent).
			Padding(0, 1)

	Placeholder = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true)
)

// ── Status bar ────────────────────────────────────────────────────────────────

var (
	StatusBar = lipgloss.NewStyle().
		// Background(ColorSurface).
		Foreground(ColorSubtle).
		Padding(0, 1)

	StatusPhase = lipgloss.NewStyle().
			Foreground(ColorBg).
			Background(ColorAccent).
			Bold(true).
			Padding(0, 1)

	StatusOK = lipgloss.NewStyle().
			Foreground(ColorGreen).
			Bold(true)

	StatusWarn = lipgloss.NewStyle().
			Foreground(ColorYellow).
			Bold(true)

	StatusErr = lipgloss.NewStyle().
			Foreground(ColorRed).
			Bold(true)

	StatusThrottle = lipgloss.NewStyle().
			Foreground(ColorBg).
			Background(ColorYellow).
			Bold(true).
			Padding(0, 1)

	TokenUsage = lipgloss.NewStyle().
			Foreground(ColorMuted)
)

// ── Phase badges ─────────────────────────────────────────────────────────────

var PhaseBadge = map[string]lipgloss.Style{
	"idle":       lipgloss.NewStyle().Foreground(ColorMuted),
	"planning":   lipgloss.NewStyle().Foreground(ColorPurple).Bold(true),
	"searching":  lipgloss.NewStyle().Foreground(ColorCyan).Bold(true),
	"executing":  lipgloss.NewStyle().Foreground(ColorOrange).Bold(true),
	"verifying":  lipgloss.NewStyle().Foreground(ColorYellow).Bold(true),
	"committing": lipgloss.NewStyle().Foreground(ColorAccent).Bold(true),
	"done":       lipgloss.NewStyle().Foreground(ColorGreen).Bold(true),
	"error":      lipgloss.NewStyle().Foreground(ColorRed).Bold(true),
	"throttled":  lipgloss.NewStyle().Foreground(ColorYellow).Bold(true),
}

// PhaseIcon maps phase names to emoji indicators.
var PhaseIcon = map[string]string{
	"idle":       "💤",
	"planning":   "🧠",
	"searching":  "🔍",
	"executing":  "⚙️ ",
	"verifying":  "🔬",
	"committing": "📦",
	"done":       "✅",
	"error":      "❌",
	"throttled":  "🌡 ",
}

// ── Log panel ─────────────────────────────────────────────────────────────────

var (
	LogTimestamp = lipgloss.NewStyle().
			Foreground(ColorMuted).
			PaddingRight(1)

	LogInfo = lipgloss.NewStyle().
		Foreground(ColorSubtle)

	LogSuccess = lipgloss.NewStyle().
			Foreground(ColorGreen)

	LogWarning = lipgloss.NewStyle().
			Foreground(ColorYellow)

	LogError = lipgloss.NewStyle().
			Foreground(ColorRed)

	LogTool = lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true)

	LogOutput = lipgloss.NewStyle().
			Foreground(ColorMuted).
			PaddingLeft(2)
)

// ── Spinners / progress ───────────────────────────────────────────────────────

var SpinnerStyle = lipgloss.NewStyle().Foreground(ColorAccent)

// ── Report ────────────────────────────────────────────────────────────────────

var (
	ReportTitle = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(ColorBorder).
			MarginBottom(1)

	ReportSection = lipgloss.NewStyle().
			Foreground(ColorSubtle).
			Bold(true).
			MarginTop(1)

	ReportCode = lipgloss.NewStyle().
			Background(ColorSurface).
			Foreground(ColorGreen).
			Padding(0, 1)
)
