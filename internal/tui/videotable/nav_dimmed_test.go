package videotable

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

// TestBorderDimmedChangesRender proves SetBorderDimmed actually affects the
// rendered frame (it recolors the border to styles.ColorBorderDim), so the
// instance flag that replaced the styles.ListBorderDimmed global drives the same
// visible effect. Verifies by rendering, not by inspecting the field.
func TestBorderDimmedChangesRender(t *testing.T) {
	styles.ColorBorderDim = lipgloss.Color("9") // distinct, non-default border color

	nav := makeNav(3, false, 2)

	nav.SetBorderDimmed(false)
	normal := nav.View()

	nav.SetBorderDimmed(true)
	dimmed := nav.View()

	if normal == dimmed {
		t.Error("SetBorderDimmed(true) did not change the rendered frame")
	}
}
