package render

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// BorderPad is the horizontal chrome a bordered overlay box adds around its
// content: 2 columns of padding + 1 column of border on each side. Inner
// content width is therefore the box width minus BorderPad.
const BorderPad = 6

// JustifyEnds lays out left and right on a single line of width columns, with
// left flush to the start, right flush to the end, and spaces between. Widths
// are measured with ansi.StringWidth (the terminal/lipgloss width authority) so
// ANSI escapes and wide runes are accounted for — fixing the class of bug where
// len() over-counts styled or CJK text. At least one space always separates the
// two, even when they would otherwise overflow the width.
func JustifyEnds(left, right string, width int) string {
	space := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if space < 1 {
		space = 1
	}
	return left + strings.Repeat(" ", space) + right
}

// ModalBox computes the outer and inner width for a centered modal box. boxW is
// width*ratioTenths/10, then clamped so it never exceeds max, never comes within
// 4 columns of the screen edge (leaving a 2-col margin each side), and never
// drops below 32. innerW is boxW-BorderPad, the width available to content
// inside the border+padding. Centralizes the geometry the scrollable overlays
// (Help, CommandHelp, ConfigIssues) previously re-derived by hand.
func ModalBox(width, ratioTenths, max int) (boxW, innerW int) {
	boxW = width * ratioTenths / 10
	if boxW > max {
		boxW = max
	}
	if boxW > width-4 {
		boxW = width - 4
	}
	if boxW < 32 {
		boxW = 32
	}
	return boxW, boxW - BorderPad
}
