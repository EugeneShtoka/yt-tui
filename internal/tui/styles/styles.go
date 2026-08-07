// Package styles holds the process-wide Lip Gloss styles for the UI and applies
// a loaded theme to them via Init, so rendering code references shared styles
// rather than re-declaring colors.
package styles

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/theme"
)

var (
	ColorAccent   color.Color
	ColorBgSelect color.Color
	ColorBorder   color.Color
	// ColorBorderDim is ColorBorder darkened, used as the "inactive" border for
	// the focused/unfocused panel signal (a focused frame is the normal border;
	// an unfocused one fades to this). Derived in Init so any custom theme's
	// border keeps a single source of truth.
	ColorBorderDim color.Color
)

// ListBorderDimmed, when true, makes the shared table (TableNav.View) render its
// frame in ColorBorderDim instead of ColorBorder. Root sets it per frame so the
// underlying video list reads as "inactive" while the info panel holds focus. It
// is safe as process-wide state because Bubble Tea renders on a single goroutine
// and only the active tab's table is drawn per frame.
var ListBorderDimmed bool

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
	ColorBorder = lipgloss.Color(t.Border)
	ColorBorderDim = lipgloss.Color(darkenHex(t.Border, 0.5))
	accent := ColorAccent
	muted := lipgloss.Color(t.Muted)
	subtle := lipgloss.Color(t.Subtle)
	success := lipgloss.Color(t.Success)
	warning := lipgloss.Color(t.Warning)
	errorC := lipgloss.Color(t.Error)
	border := ColorBorder
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

// darkenHex scales an "#RRGGBB" color toward black by factor (0..1) and returns
// the resulting hex string. A malformed input is returned unchanged so a
// non-hex theme value (e.g. a named or 256-index color) degrades to "no dim"
// rather than a crash — the focus signal then just relies on the panel border.
func darkenHex(hex string, factor float64) string {
	var r, g, b int
	if _, err := fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b); err != nil {
		return hex
	}
	scale := func(v int) int { return int(float64(v) * factor) }
	return fmt.Sprintf("#%02x%02x%02x", scale(r), scale(g), scale(b))
}
