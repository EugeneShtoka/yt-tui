package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/theme"
)

var (
	ColorAccent   color.Color
	ColorBgSelect color.Color
)

var (
	TabActive    lipgloss.Style
	TabInactive  lipgloss.Style
	TabBar       lipgloss.Style
	Selected     lipgloss.Style
	Normal       lipgloss.Style
	Bold         lipgloss.Style
	Dim          lipgloss.Style
	Channel      lipgloss.Style
	Duration     lipgloss.Style
	Error        lipgloss.Style
	Success      lipgloss.Style
	Warning      lipgloss.Style
	Help         lipgloss.Style
	RowNum       lipgloss.Style
	ColHeader    lipgloss.Style
	SectionTitle lipgloss.Style
)

func init() {
	Init(theme.Default())
}

// Init rebuilds all styles from the given theme. Call after loading a user theme.
func Init(t theme.Theme) {
	ColorAccent = lipgloss.Color(t.Accent)
	ColorBgSelect = lipgloss.Color(t.BgSelect)
	accent := ColorAccent
	muted := lipgloss.Color(t.Muted)
	subtle := lipgloss.Color(t.Subtle)
	success := lipgloss.Color(t.Success)
	warning := lipgloss.Color(t.Warning)
	errorC := lipgloss.Color(t.Error)
	border := lipgloss.Color(t.Border)
	highlight := lipgloss.Color(t.Highlight)

	TabActive = lipgloss.NewStyle().Bold(true).Foreground(accent).Padding(0, 1)
	TabInactive = lipgloss.NewStyle().Foreground(subtle).Padding(0, 1)
	TabBar = lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(border).
		BorderBottom(true)

	Selected = lipgloss.NewStyle().Background(ColorBgSelect).Bold(true)
	Normal = lipgloss.NewStyle()
	Bold = lipgloss.NewStyle().Bold(true).Foreground(highlight)
	Dim = lipgloss.NewStyle().Faint(true)
	Channel = lipgloss.NewStyle().Foreground(subtle)
	Duration = lipgloss.NewStyle().Foreground(muted)
	Error = lipgloss.NewStyle().Foreground(errorC).Bold(true)
	Success = lipgloss.NewStyle().Foreground(success)
	Warning = lipgloss.NewStyle().Foreground(warning)
	Help = lipgloss.NewStyle().Foreground(muted)
	RowNum = lipgloss.NewStyle().Foreground(subtle)
	ColHeader = lipgloss.NewStyle().Foreground(subtle).Underline(true)
	SectionTitle = lipgloss.NewStyle().Bold(true).Foreground(accent).PaddingLeft(1).MarginBottom(1)
}
