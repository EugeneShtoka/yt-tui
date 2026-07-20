package videotable

import (
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"charm.land/bubbles/v2/key"
	etable "github.com/evertras/bubble-table/table"
)

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
		WithBaseStyle(styles.Normal).
		HeaderStyle(styles.ColHeader).
		HighlightStyle(styles.Selected).
		Focused(true)
}

// NewVideoTable constructs a standard evertras table for video lists.
func NewVideoTable(cols []VideoColumnDef) etable.Model {
	return NewTable(cols)
}
