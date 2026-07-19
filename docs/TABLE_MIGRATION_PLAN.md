# Tab → `bubbles/table` Migration Plan

**Goal:** Replace the manual `cursor/vs` + nav switch blocks + custom row rendering in every tab with `bubbles/table`. Eliminates ~10 lines of identical nav code per tab, all `xxxPageHeight()` methods, and the `renderVideoRows`/`renderVideoList` helpers.

---

## What changes per tab

### Fields
```go
// Before
cursor, vs int

// After
table table.Model
```

### On data load
```go
// Before
t.cursor, t.vs = 0, 0  // or clamp cursor

// After
t.table.SetRows(toRows(t.videos))
t.table.GotoTop()
```

### On resize (`ContentSizeMsg`)
```go
// Before
t.width, t.height = m.Width, m.Height  // used later in View()

// After
t.width, t.height = m.Width, m.Height
t.table.SetColumns(computeColumns(t.width))
t.table.SetHeight(contentHeight(t.height))
t.table.SetRows(toRows(t.data))  // re-render with new widths
```

### Key handling
```go
// Before — 10 lines removed from every tab:
case key.Matches(msg, keys.Up):
    t.cursor, t.vs = nav.Move(t.cursor, t.vs, n, -1, pageH, t.circular)
case key.Matches(msg, keys.Down):
    t.cursor, t.vs = nav.Move(t.cursor, t.vs, n, +1, pageH, t.circular)
case key.Matches(msg, keys.PageUp):
    t.cursor, t.vs = nav.Page(t.cursor, t.vs, n, -1, pageH, t.circular)
case key.Matches(msg, keys.PageDown):
    t.cursor, t.vs = nav.Page(t.cursor, t.vs, n, +1, pageH, t.circular)
case key.Matches(msg, keys.GotoBottom):
    t.cursor, t.vs = nav.Jump(n-1, n, pageH)

// After — forwarded to table, which handles all of the above:
var cmd tea.Cmd
t.table, cmd = t.table.Update(msg)
return t, cmd
```

For non-nav keys (play, download, etc.) read the current item via:
```go
row := t.table.SelectedRow()   // table.Row = []string
// or for typed access, keep the original slice and use t.table.Cursor() as index
video := t.videos[t.table.Cursor()]
```

### View
```go
// Before
body := renderVideoRows(ctx, t.videos, t.cursor, t.vs, listH)

// After
body := t.table.View()
```

The `xxxPageHeight()` method is deleted. `table.SetHeight` replaces it.

---

## Row conversion: the indicator column

The `▶ / ● / ○` indicator (watch status, selected state) becomes a 2-char string column at index 0.

```go
func indicatorCell(v domain.Video, positions map[string]int64, watched map[string]bool, localStatus map[string]domain.VideoStatus) string {
    if _, hasPos := positions[v.ID]; hasPos {
        return "○ "
    }
    if watched[v.ID] {
        return "○ "
    }
    if st, ok := localStatus[v.ID]; ok && st == domain.StatusNew {
        return "● "
    }
    return "  "
}
```

`bubbles/table` renders the selected-row highlight as a single `Selected` style over the whole row — so the indicator coloring for the selected row comes for free. For non-selected rows the colored ● / ○ needs to be embedded in the pre-rendered string (lipgloss ANSI codes survive in table cells).

---

## Column width pattern

Every tab follows the same formula:

```go
func computeVideoColumns(width int, showChannel bool) []table.Column {
    titleW := width - colNum - 2 - colDuration - colViews - colDate - gaps
    if showChannel { titleW -= colChannel }
    if titleW < 20 { titleW = 20 }
    cols := []table.Column{
        {Title: " ", Width: 2},           // indicator
        {Title: "Title", Width: titleW},
    }
    if showChannel {
        cols = append(cols, table.Column{Title: "Channel", Width: colChannel})
    }
    return append(cols,
        table.Column{Title: "Duration", Width: colDuration},
        table.Column{Title: "Views",    Width: colViews},
        table.Column{Title: "Date",     Width: colDate},
    )
}
```

Non-video tabs (History, Activity, Channels list, Downloading) each get their own `computeXxxColumns(width int)` with the same pattern but different column sets.

---

## Styling

```go
func tableStyles() table.Styles {
    s := table.DefaultStyles()
    s.Header   = styles.ColHeader          // existing style
    s.Cell     = styles.Normal
    s.Selected = styles.Selected
    return s
}
```

Set once at construction; colors match the existing theme.

---

## `circular` navigation

`bubbles/table` does not wrap around at top/bottom. If `circular` is true, intercept `Up` at row 0 and `Down` at the last row before forwarding to the table:

```go
case key.Matches(msg, keys.Up):
    if t.circular && t.table.Cursor() == 0 {
        t.table.GotoBottom()
        return t, nil
    }
case key.Matches(msg, keys.Down):
    if t.circular && t.table.Cursor() == len(t.videos)-1 {
        t.table.GotoTop()
        return t, nil
    }
```

Then fall through to `t.table, cmd = t.table.Update(msg)`.

---

## Tabs in scope

| Tab | List type | Notes |
|---|---|---|
| Recommended | `[]domain.Video` | uses `renderVideoList` today |
| Subscriptions | `[]domain.Video` | uses `renderVideoList` today |
| History | `[]domain.HistoryEntry` | custom Type column; detail pane stays manual |
| Local | `[]domain.LocalVideo` | custom indicator logic |
| Activity | `[]domain.ActivityEntry` | two columns only |
| Downloading | `[]api.DownloadItem` | status column has progress bar string |
| Channels — channel list | `[]domain.Channel` | 7 columns; two-pane tab |
| Channels — video pane | `[]domain.Video` | reuses video columns |
| Channels — tag video pane | `[]domain.Video` | reuses video columns |
| Playlists — video pane | `[]domain.Video` | reuses video columns |
| Search — results | `[]domain.Video` / `[]domain.Channel` | two result types |

`Playlists` playlist-list pane has only 1 meaningful column (name) — trivial.  
`Search` has two separate result lists (channels + videos) — two table instances or a toggle.

---

## What is deleted after migration

- `renderVideoList`, `renderVideoRows`, `renderVideoColHeader`, `renderVideoRow` in `video_list.go` — replaced by `bubbles/table`
- All `xxxPageHeight()` methods across all tabs
- The 5-case nav switch block in every tab key handler
- `cursor, vs int` fields in every tab
- `nav.Move`, `nav.Page`, `nav.Jump`, `nav.Window` call sites (the `nav` package itself may become unused)

---

## Sequencing

1. Add `tableStyles()` and `computeVideoColumns()` helpers to `video_list.go` (or a new `table_helpers.go`)
2. Migrate leaf tabs first (Activity, History, Local, Downloading) — single list, no panes
3. Migrate Recommended and Subscriptions — use video columns
4. Migrate Channels — three lists (channel list + two video panes), handle circular
5. Migrate Playlists — playlist pane + video pane
6. Migrate Search — two table instances
7. Delete `renderVideoList`, `renderVideoRows`, `nav` call sites, `xxxPageHeight` methods
8. Assess whether `internal/tui/nav` package is still needed (Window may remain for non-table uses)
