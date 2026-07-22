package overlay

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

// Overlay is a modal component drawn on top of the active tab's content.
// Implementations are value types; Update returns a mutated copy cast to Overlay.
type Overlay interface {
	tea.Model
	// Render places the overlay on top of behind and returns the composed view.
	// kittySeq is an optional Kitty Graphics Protocol terminal escape to append
	// after the frame (used by the video-detail thumbnail).
	Render(behind string, width, height int) (view, kittySeq string)
	// InterceptsInput reports whether the overlay owns a focused text input.
	InterceptsInput() bool
	// WidthReduction is columns reserved on the right edge (0 for centered modals;
	// non-zero for the video-detail side panel).
	WidthReduction() int
	// HasFocus reports whether the overlay is currently capturing keyboard input.
	HasFocus() bool
}

// PopOverlayMsg is emitted by an overlay when it wants Root to close it.
type PopOverlayMsg struct{}

// FocusSwitchMsg is sent by Root to the top overlay to toggle its focus state.
type FocusSwitchMsg struct{}

// placeOverlayBox renders content inside a rounded bordered box and centers it
// over behind, composing the two strings by overwriting matching character cells.
func placeOverlayBox(behind, content string, totalWidth, boxWidth int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorAccent).
		Padding(1, 2).
		Width(boxWidth).
		Render(content)

	bw := lipgloss.Width(box)
	bh := lipgloss.Height(box)

	x := (totalWidth - bw) / 2
	if x < 0 {
		x = 0
	}

	behindLines := strings.Split(behind, "\n")
	totalLines := len(behindLines)
	y := (totalLines - bh) / 2
	if y < 0 {
		y = 0
	}

	for i, ol := range strings.Split(box, "\n") {
		lineIdx := y + i
		if lineIdx >= totalLines {
			behindLines = append(behindLines, "")
			totalLines++
		}
		row := behindLines[lineIdx]
		if visW := lipgloss.Width(row); visW < x {
			row += strings.Repeat(" ", x-visW)
		}
		left := ansi.Truncate(row, x, "")
		right := ansi.TruncateLeft(row, x+bw, "")
		behindLines[lineIdx] = left + ol + right
	}
	return strings.Join(behindLines, "\n")
}
