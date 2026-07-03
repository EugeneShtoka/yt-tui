package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/EugeneShtoka/yt-tui/internal/theme"
)

// colorAccent and colorBgSelect are kept as package vars so inline styles in view.go can reference them.
var (
	colorAccent  lipgloss.Color
	colorBgSelect lipgloss.Color
)

var (
	styleTabActive     lipgloss.Style
	styleTabInactive   lipgloss.Style
	styleTabBar        lipgloss.Style
	styleSelected      lipgloss.Style
	styleNormal        lipgloss.Style
	styleBold          lipgloss.Style
	styleDim           lipgloss.Style
	styleChannel       lipgloss.Style
	styleDuration      lipgloss.Style
	styleStatus        lipgloss.Style
	styleError         lipgloss.Style
	styleSuccess       lipgloss.Style
	styleWarning       lipgloss.Style
	styleHeader        lipgloss.Style
	styleHelp          lipgloss.Style
	styleProgressFill  lipgloss.Style
	styleProgressEmpty lipgloss.Style
	stylePendingTag    lipgloss.Style
	styleActiveTag     lipgloss.Style
	styleCompleteTag   lipgloss.Style
	styleFailedTag     lipgloss.Style
	styleRowNum        lipgloss.Style
	styleColHeader     lipgloss.Style
	styleInputPrompt   lipgloss.Style
	styleSectionTitle  lipgloss.Style
)

func init() {
	InitStyles(theme.Default())
}

// InitStyles rebuilds all UI styles from the given theme.
// Call this once at startup after loading the user's theme file.
func InitStyles(t theme.Theme) {
	colorAccent   = lipgloss.Color(t.Accent)
	colorBgSelect = lipgloss.Color(t.BgSelect)
	accent    := colorAccent
	muted     := lipgloss.Color(t.Muted)
	subtle    := lipgloss.Color(t.Subtle)
	success   := lipgloss.Color(t.Success)
	warning   := lipgloss.Color(t.Warning)
	errorC    := lipgloss.Color(t.Error)
	border    := lipgloss.Color(t.Border)
	highlight := lipgloss.Color(t.Highlight)

	styleTabActive = lipgloss.NewStyle().
		Bold(true).
		Foreground(accent).
		Padding(0, 1)

	styleTabInactive = lipgloss.NewStyle().
		Foreground(subtle).
		Padding(0, 1)

	styleTabBar = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(border).
		BorderBottom(true)

	styleSelected = lipgloss.NewStyle().
		Background(colorBgSelect).
		Bold(true)

	styleNormal = lipgloss.NewStyle()

	styleBold = lipgloss.NewStyle().
		Bold(true).
		Foreground(highlight)

	styleDim = lipgloss.NewStyle().
		Faint(true)

	styleChannel = lipgloss.NewStyle().
		Foreground(subtle)

	styleDuration = lipgloss.NewStyle().
		Foreground(muted)

	styleStatus = lipgloss.NewStyle().
		Foreground(subtle)

	styleError = lipgloss.NewStyle().
		Foreground(errorC).
		Bold(true)

	styleSuccess = lipgloss.NewStyle().
		Foreground(success)

	styleWarning = lipgloss.NewStyle().
		Foreground(warning)

	styleHeader = lipgloss.NewStyle().
		Bold(true).
		Foreground(accent)

	styleHelp = lipgloss.NewStyle().
		Foreground(muted)

	styleProgressFill  = lipgloss.NewStyle().Foreground(success)
	styleProgressEmpty = lipgloss.NewStyle().Foreground(muted)

	stylePendingTag  = lipgloss.NewStyle().Foreground(muted)
	styleActiveTag   = lipgloss.NewStyle().Foreground(warning)
	styleCompleteTag = lipgloss.NewStyle().Foreground(success)
	styleFailedTag   = lipgloss.NewStyle().Foreground(errorC)

	styleRowNum = lipgloss.NewStyle().
		Foreground(subtle)

	styleColHeader = lipgloss.NewStyle().
		Foreground(subtle).
		Underline(true)

	styleInputPrompt = lipgloss.NewStyle().
		Foreground(accent).
		Bold(true)

	styleSectionTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(accent).
		MarginBottom(1)
}

func progressBar(pct float64, width int) string {
	if width <= 0 {
		return ""
	}
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	bar := styleProgressFill.Render(repeatStr("█", filled))
	bar += styleProgressEmpty.Render(repeatStr("░", width-filled))
	return "[" + bar + "]"
}

func repeatStr(s string, n int) string {
	if n <= 0 {
		return ""
	}
	result := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		result = append(result, s...)
	}
	return string(result)
}
