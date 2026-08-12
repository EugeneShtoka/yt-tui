package videotable

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestTableFillsWidth(t *testing.T) {
	cols := []ColumnDef[VideoData]{
		NumCol[VideoData](), IndicatorCol[VideoData](), TitleFlexCol[VideoData](),
		DurationCol[VideoData](), ViewsCol[VideoData](), DateCol[VideoData](),
	}
	nav := NewTableNav(cols, false, 4)
	nav.Resize(120, 20)
	nav.SetRows(BuildVideoRows([]VideoData{{}}, cols))
	for _, line := range strings.Split(nav.View(), "\n") {
		if w := lipgloss.Width(line); w != 0 && w != 120 {
			t.Errorf("line width = %d, want 120: %q", w, line)
		}
	}
}
