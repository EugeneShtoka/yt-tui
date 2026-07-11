package ui

import (
	"fmt"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// activityView owns the Activity tab's private state. This is the P4 reference
// slice: Activity is the only tab whose state has no external readers or writers,
// so full ownership is achievable here (see docs/TABVIEW_DESIGN.md).
type activityView struct {
	entries []db.ActivityEntry
	cursor  int
	vs      int // viewStart: first visible row
}

// load fetches the activity log, reporting errors via setErr. Cursor is clamped
// to the new length (preserving position across a refresh).
func (v *activityView) load(store Store, setErr func(string)) {
	entries, err := store.GetActivityLog(200)
	if err != nil {
		setErr("activity: " + err.Error())
		return
	}
	v.entries = entries
	v.cursor = clamp(v.cursor, len(entries))
}

// activityNavIntent asks the router to jump to the tab/target for an entry;
// activity navigation writes other tabs' state, which is a router concern.
type activityNavIntent struct{ entry db.ActivityEntry }

func (in activityNavIntent) apply(m *Model) tea.Cmd { return m.navigateToActivity(in.entry) }

// context: Activity has no video/sort semantics, so it reports the default
// context (matching its absence from the pre-interface currentContext switch).
func (v activityView) context() ContextID { return CtxVideoList }

// update handles navigation keys. On drill-down it returns a nav intent for the
// router to perform the cross-tab jump.
func (v *activityView) update(msg tea.KeyMsg, ctx viewCtx) viewIntent {
	n := len(v.entries)
	switch {
	case key.Matches(msg, ctx.keys.Up):
		v.cursor, v.vs = vsMove(v.cursor, v.vs, n, -1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, ctx.keys.Down):
		v.cursor, v.vs = vsMove(v.cursor, v.vs, n, +1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, ctx.keys.PageUp):
		v.cursor, v.vs = vsPage(v.cursor, v.vs, n, -1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, ctx.keys.PageDown):
		v.cursor, v.vs = vsPage(v.cursor, v.vs, n, +1, ctx.pageSize, ctx.circular)
	case key.Matches(msg, ctx.keys.DrillDown), key.Matches(msg, ctx.keys.Right):
		if v.cursor < n {
			return activityNavIntent{entry: v.entries[v.cursor]}
		}
	}
	return nil
}

// render draws the activity list. Moved verbatim from Model.renderActivity.
func (v activityView) render(ctx viewCtx, height int) string {
	width := ctx.width
	header := styleSectionTitle.Render("Activity")
	headerH := lipgloss.Height(header)

	if len(v.entries) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, header,
			styleDim.Render("No activity yet."))
	}

	colType := 16
	colMeta := width - colNum - 1 - 2 - colType - 1
	if colMeta < 20 {
		colMeta = 20
	}

	start, end := scrollWindowAt(v.vs, len(v.entries), height-headerH)
	var rows []string
	for i := start; i < end && i < len(v.entries); i++ {
		e := v.entries[i]

		indicator := "  "
		sep := " "
		numStyle := styleRowNum
		typeStyle := styleWarning.Width(colType)
		if i == v.cursor {
			indicator = styleSelected.Render("▶ ")
			numStyle = numStyle.Background(colorBgSelect)
			sep = lipgloss.NewStyle().Background(colorBgSelect).Render(" ")
			typeStyle = typeStyle.Background(colorBgSelect)
		}
		numStr := numStyle.Render(fmt.Sprintf("%*d", colNum, i+1))

		locality := "remote"
		if e.IsLocal {
			locality = "local"
		}
		typeLabel := typeStyle.Render(e.Type)

		var meta string
		switch e.Type {
		case "subscribe":
			meta = fmt.Sprintf("%s (%s)", e.ChannelName, locality)
		case "create_playlist":
			meta = fmt.Sprintf("%s (%s)", e.PlaylistName, locality)
		case "add_to_playlist":
			meta = fmt.Sprintf("%s → %s (%s)", truncate(e.VideoTitle, colMeta/2), e.PlaylistName, locality)
		default:
			meta = e.Type
		}
		metaStyle := styleNormal.Width(colMeta)
		if i == v.cursor {
			metaStyle = styleSelected.Width(colMeta)
		}
		rows = append(rows, numStr+sep+indicator+typeLabel+sep+metaStyle.Render(truncate(meta, colMeta)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(rows, "\n"))
}
