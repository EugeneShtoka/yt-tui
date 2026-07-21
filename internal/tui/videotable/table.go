package videotable

import (
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"charm.land/bubbles/v2/key"
  "charm.land/lipgloss/v2"
	etable "github.com/evertras/bubble-table/table"
)

var titleOnlyBorder = etable.Border{
	Top:            "",
	Left:           "",
	Right:          "",
	Bottom:         "─", // This acts as the horizontal line right below the column titles
	TopRight:       "",
	TopLeft:        "",
	BottomRight:    "",
	BottomLeft:     "",
	TopJunction:    "",
	BottomJunction: "",
	LeftJunction:   "",
	RightJunction:  "",
	InnerJunction:  "",
	InnerDivider:   "", // Removes vertical column separation lines
}

// newEtable constructs the standard evertras table shared by all tabs.
// g/G are unbound — tabs handle gg chord and G via TableNav.
func newEtable(cols []etable.Column) etable.Model {
	km := etable.DefaultKeyMap()
	km.PageFirst = key.NewBinding()
	km.PageLast = key.NewBinding()

	return etable.New(cols).
		WithKeyMap(km).
		WithNoPagination().
		WithFooterVisibility(false).
		WithOuterBorder(false).
		WithRowBorder(false).
		Border(titleOnlyBorder).
		WithBorderForeground(lipgloss.Color("240")). // A muted gray color (adjust index as needed)
		WithBaseStyle(styles.Normal).
		HeaderStyle(styles.ColHeader.UnsetUnderline()).
		HighlightStyle(styles.Selected).
		Focused(true)
}

// NewVideoTable constructs a standard evertras table for VideoData lists.
func NewVideoTable(cols []ColumnDef[VideoData]) etable.Model {
	return NewTable(cols)
}
