package tab

import "github.com/EugeneShtoka/yt-tui/internal/tui/videotable"

// PanelColumnKeys returns the column keys a panel type offers for its primary,
// configurable list, in natural (default) order — the list its sort chord acts
// on where it has one. It is the single source of truth for column-selection
// validation (see app.ValidateColumns) and is derived from the very builders the
// tab constructors use, so the offered set and the rendered set can never drift.
// An unknown type returns nil (no columns offered).
func PanelColumnKeys(panelType string) []string {
	switch panelType {
	case "feed":
		return videotable.ColumnKeys(feedColumns())
	case "local":
		return videotable.ColumnKeys(localColumns())
	case "channels":
		return videotable.ColumnKeys(channelColumns())
	case "tags":
		return videotable.ColumnKeys(tagColumns())
	case "playlists":
		return videotable.ColumnKeys(playlistColumns())
	case "history":
		return videotable.ColumnKeys(historyColumns())
	case "activity":
		return videotable.ColumnKeys(activityColumns())
	case "downloading":
		return videotable.ColumnKeys(downloadingColumns())
	case "search":
		return videotable.ColumnKeys(searchVideoColumns())
	}
	return nil
}
