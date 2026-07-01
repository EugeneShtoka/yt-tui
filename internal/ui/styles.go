package ui

import "github.com/charmbracelet/lipgloss"

var (
	colorAccent   = lipgloss.Color("#FF6B6B")
	colorMuted    = lipgloss.Color("#666666")
	colorSubtle   = lipgloss.Color("#888888")
	colorGreen    = lipgloss.Color("#4CAF50")
	colorYellow   = lipgloss.Color("#FFC107")
	colorBg       = lipgloss.Color("#1a1a2e")
	colorSelected = lipgloss.Color("#2a2a4e")
	colorBorder   = lipgloss.Color("#333355")
	colorNew      = lipgloss.Color("#7EC8E3")

	styleTabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			Padding(0, 1)

	styleTabInactive = lipgloss.NewStyle().
				Foreground(colorSubtle).
				Padding(0, 1)

	styleTabBar = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorBorder).
			BorderBottom(true)

	styleSelected = lipgloss.NewStyle().
			Background(colorSelected).
			Bold(true)

	styleNormal = lipgloss.NewStyle()

	styleBold = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorNew)

	styleDim = lipgloss.NewStyle().
			Faint(true)

	styleChannel = lipgloss.NewStyle().
			Foreground(colorSubtle)

	styleDuration = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleStatus = lipgloss.NewStyle().
			Foreground(colorSubtle)

	styleError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF4444")).
			Bold(true)

	styleSuccess = lipgloss.NewStyle().
			Foreground(colorGreen)

	styleWarning = lipgloss.NewStyle().
			Foreground(colorYellow)

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	styleHelp = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleProgressFill  = lipgloss.NewStyle().Foreground(colorGreen)
	styleProgressEmpty = lipgloss.NewStyle().Foreground(colorMuted)

	stylePendingTag  = lipgloss.NewStyle().Foreground(colorMuted)
	styleActiveTag   = lipgloss.NewStyle().Foreground(colorYellow)
	styleCompleteTag = lipgloss.NewStyle().Foreground(colorGreen)
	styleFailedTag   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4444"))

	styleRowNum = lipgloss.NewStyle().
			Foreground(colorSubtle)

	styleColHeader = lipgloss.NewStyle().
			Foreground(colorBorder).
			Underline(true)

	styleInputPrompt = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	styleSectionTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorAccent).
				MarginBottom(1)
)

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
