package videotable

import (
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"charm.land/bubbles/v2/key"
	etable "github.com/evertras/bubble-table/table"
)

// NewVideoTable constructs a standard evertras table for video lists.
//
// The g/G bindings are removed from the default KeyMap because the project
// uses a gg-chord for GotoTop; each tab intercepts g before forwarding to
// tbl.Update, and calls tbl.WithHighlightedRow(0).PageFirst() explicitly.
//
// Callers must still call WithTargetWidth/WithTargetHeight and WithRows after
// construction (typically in the tab's resize/reload methods).
func NewVideoTable(cols []VideoColumnDef) etable.Model {
	km := etable.DefaultKeyMap()
	km.PageFirst = key.NewBinding() // unbind g — tab handles gg chord
	km.PageLast = key.NewBinding()  // unbind G

	return etable.New(VideoColumns(cols)).
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
