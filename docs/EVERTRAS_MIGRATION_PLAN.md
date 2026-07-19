# Migration Plan: charmbracelet/bubbles/table → evertras/bubble-table

## Goal

Replace `charmbracelet/bubbles/table` with `github.com/evertras/bubble-table` and
introduce a central column-registry architecture. Every tab declares which pre-defined
column types it needs; a shared builder converts domain data into evertras rows.

## Why this migration

`charmbracelet/bubbles/table` calls `runewidth.Truncate(cell, colWidth)` before any
lipgloss rendering. Because `runewidth` counts ANSI escape bytes as visible characters,
embedded dim/bold styling requires inflating column widths by a measured overhead
(`DimCellOverhead = 7`), pre-truncating content to a "safe width", and replacing full
resets with partial `\033[22m]` sequences.

`evertras/bubble-table` uses `ansi.StringWidth` / `ansi.Truncate` (from
`github.com/charmbracelet/x/ansi`) for all width math. ANSI codes are invisible to its
truncator. Row-level `.WithStyle(lipgloss.Style)` and cell-level
`NewStyledCell(value, style)` keep styling metadata separate from content. Every ANSI
workaround (`DimCellOverhead`, column-width inflation, `swapReset`, `dimSwapReset`,
`titleSafeWidth` overhead math) disappears.

---

## Allowed APIs (verified from module v0.22.3 source)

```go
import etable "github.com/evertras/bubble-table/table"

// --- Columns ---
etable.NewColumn(key, title string, width int) etable.Column     // fixed width
etable.NewFlexColumn(key, title string, flexFactor int) etable.Column  // grows to fill

// --- Rows & cells ---
etable.NewRow(data etable.RowData) etable.Row          // RowData = map[string]any
etable.NewStyledCell(data any, style lipgloss.Style) etable.StyledCell
row.WithStyle(style lipgloss.Style) etable.Row         // row-level style

// --- Model construction (value receiver, chainable) ---
etable.New(columns []etable.Column) etable.Model
m.WithRows(rows []etable.Row) etable.Model
m.WithTargetWidth(w int) etable.Model                  // required for flex columns
m.WithTargetHeight(h int) etable.Model                 // visible row area height
m.WithPageSize(n int) etable.Model                     // alternative to WithTargetHeight
m.WithNoPagination() etable.Model
m.WithBaseStyle(style lipgloss.Style) etable.Model
m.HeaderStyle(style lipgloss.Style) etable.Model       // no "With" prefix
m.HighlightStyle(style lipgloss.Style) etable.Model    // no "With" prefix
m.WithRowStyleFunc(f func(RowStyleFuncInput) lipgloss.Style) etable.Model
m.Focused(focused bool) etable.Model                   // no "With" prefix
m.WithKeyMap(km etable.KeyMap) etable.Model
m.WithOuterBorder(show bool) etable.Model
m.WithRowBorder(show bool) etable.Model
m.WithHeaderVisibility(v bool) etable.Model
m.WithHighlightedRow(index int) etable.Model           // programmatic cursor placement
m.PageFirst() etable.Model
m.PageLast() etable.Model

// --- Bubble Tea interface ---
m.Init() tea.Cmd
m.Update(msg tea.Msg) (etable.Model, tea.Cmd)   // returns Model, NOT tea.Model
m.View() string

// --- State queries ---
m.HighlightedRow() etable.Row                          // empty Row if no rows
m.GetHighlightedRowIndex() int                         // 0-based into GetVisibleRows()
m.GetVisibleRows() []etable.Row
m.GetLastUpdateUserEvents() []etable.UserEvent

// --- UserEvent types (type-switch after GetLastUpdateUserEvents) ---
etable.UserEventHighlightedIndexChanged{ PreviousRowIndex, SelectedRowIndex int }

// --- Default KeyMap (to customize/disable conflicting bindings) ---
etable.DefaultKeyMap()   // g/G bound to PageFirst/PageLast — conflicts with gg chord
```

**Anti-patterns (verified absent):**
- No `GotoTop()`, `MoveUp(n)`, `MoveDown(n)`, `SetCursor(n)` — use `WithHighlightedRow(n)`, `PageFirst()`, `PageLast()`
- No `SetRows()` / `SetColumns()` mutating methods — use `m = m.WithRows(...)` (returns new Model)
- `WithTargetWidth` is required for flex columns; omitting it collapses flex columns to width 1
- `WithTargetHeight` and `WithPageSize` are mutually exclusive

---

## Target Architecture

### New package: `internal/tui/videotable/`

```
internal/tui/videotable/
  context.go    — RenderContext struct
  columns.go    — VideoColumnDef type + pre-defined column vars (Num, Indicator, Title, …)
  builder.go    — BuildVideoRows(), VideoColumns(), isFaded(), videoTitleStyle()
  table.go      — NewVideoTable() constructor with standard styles/keymap
```

### Column registry concept

```go
// VideoCell is the input to every cell function
type VideoCell struct {
    Video domain.Video
    Index int           // 0-based row index (needed for "#" column)
    Ctx   RenderContext
}

// VideoColumnDef pairs an evertras column spec with a cell renderer
type VideoColumnDef struct {
    Col  etable.Column
    Cell func(VideoCell) any   // returns string or etable.StyledCell
}

// Pre-defined columns — tabs pick from these by name
var (
    Num       VideoColumnDef   // fixed 4-wide, right-aligned row number
    Indicator VideoColumnDef   // fixed 3-wide, ●/○/space
    Title     VideoColumnDef   // FLEX — grows to fill; returns NewStyledCell(title, boldOrDim)
    Channel   VideoColumnDef   // fixed 30-wide, string
    Duration  VideoColumnDef   // fixed content-width, right-aligned
    Views     VideoColumnDef   // fixed 8-wide, right-aligned
    Date      VideoColumnDef   // fixed 11-wide
)
```

### How dim/bold styling works (no ANSI tricks needed)

```go
func BuildVideoRows(videos []domain.Video, cols []VideoColumnDef, ctx RenderContext) []etable.Row {
    rows := make([]etable.Row, len(videos))
    for i, v := range videos {
        data := make(etable.RowData, len(cols)+1)
        input := VideoCell{Video: v, Index: i, Ctx: ctx}
        for _, col := range cols {
            data[col.Col.Key()] = col.Cell(input)
        }
        row := etable.NewRow(data)
        if isFaded(v, ctx) {
            row = row.WithStyle(styles.Dim)  // dims ALL columns automatically
        }
        rows[i] = row
    }
    return rows
}
```

Style override order in evertras: **base → column → row → cell**.

- **Faded row** (watched/in-progress): `.WithStyle(styles.Dim)` — all cells go dim, including Duration, Views, Date, with no width tricks.
- **Bold title** (StatusNew, not faded): Title column returns `NewStyledCell(title, styles.Bold)` — cell style overrides any row style.
- **Normal row**: no `.WithStyle`; Title returns `NewStyledCell(title, styles.Normal)` (or just the string).

### Per-tab structure

```go
// Subscriptions example — after migration
var subscriptionCols = []videotable.VideoColumnDef{
    videotable.Num, videotable.Indicator, videotable.Title,
    videotable.Channel, videotable.Duration, videotable.Views, videotable.Date,
}

type Subscriptions struct {
    tbl         etable.Model
    feed        feed.Feed
    positions   map[string]int64
    watched     map[string]bool
    localStatus map[string]domain.VideoStatus
    aliases     map[string]string
    width, height int
    // sort state, chord state, numBuf — unchanged
}

func (t *Subscriptions) rebuildRows() {
    ctx := videotable.RenderContext{Positions: t.positions, Watched: t.watched,
        LocalStatus: t.localStatus, Aliases: t.aliases}
    t.tbl = t.tbl.WithRows(videotable.BuildVideoRows(t.feed.Videos(), subscriptionCols, ctx))
}

func (t *Subscriptions) resize(w, h int) {
    t.width, t.height = w, h
    t.tbl = t.tbl.WithTargetWidth(w).WithTargetHeight(h)
    t.rebuildRows()
}

func (t Subscriptions) currentVideo() (domain.Video, bool) {
    return t.feed.At(t.tbl.GetHighlightedRowIndex())
}
```

### Column widths after migration (no DimCellOverhead)

| Column    | Width | Notes |
|-----------|-------|-------|
| Num       | 4     | fixed, right-aligned |
| Indicator | 3     | fixed |
| Title     | flex  | `NewFlexColumn`, grows to fill `WithTargetWidth` |
| Channel   | 30    | fixed |
| Duration  | `maxLen*2+3` | per active DurFmt; no overhead inflation |
| Views     | 8     | reverted from 15 |
| Date      | 11    | reverted from 18 |

### KeyMap conflict: g/G vs gg-chord

evertras default binds `g` → `PageFirst` and `G` → `PageLast`.
The project uses `g` as the gg-chord prefix (double-g = GotoTop).

Resolution: supply a custom KeyMap with `g`/`G` unbound at the table level; the tab
intercepts `g` for the chord before forwarding to `tbl.Update`.

```go
km := etable.DefaultKeyMap()
km.PageFirst = key.NewBinding()   // unbind g
km.PageLast  = key.NewBinding()   // unbind G
tbl = tbl.WithKeyMap(km)
```

GotoTop at tab level becomes:
```go
t.tbl = t.tbl.WithHighlightedRow(0).PageFirst()
```

GotoBottom:
```go
t.tbl = t.tbl.WithHighlightedRow(len(rows) - 1).PageLast()
```

---

## Phases

---

### Phase 0 — Preparation (no code changes)

**Verify dependency is added:**
```
grep "evertras/bubble-table" go.mod
```
It was added during API discovery. Pin the version:
```bash
go get github.com/evertras/bubble-table@v0.22.3
go mod tidy
```

**Verify evertras source is readable:**
```bash
find /home/eugene/go/pkg/mod/github.com/evertras -name "*.go" -maxdepth 4 | head -10
```

---

### Phase 1 — Build `internal/tui/videotable/` package (no tab changes yet)

**Goal:** Create the central registry. No existing code is modified.

#### 1a. `context.go`

```go
package videotable

import "github.com/EugeneShtoka/yt-tui/internal/domain"

type RenderContext struct {
    Positions   map[string]int64
    Watched     map[string]bool
    LocalStatus map[string]domain.VideoStatus
    Aliases     map[string]string
}
```

#### 1b. `columns.go`

Define `VideoCell`, `VideoColumnDef`, and all pre-defined column vars.

Column-function logic to port from `table_helpers.go`:
- `videoTitleStyle()` → becomes `titleStyle(VideoCell) lipgloss.Style`
- `videoIndicator()` → becomes content string in Indicator.Cell
- `isFaded()` → package-level func

Pre-defined column vars — copy widths from current `render` constants (BEFORE the
DimCellOverhead inflation; use original values: ColViews=8, ColDate=11, ColDuration
from formula without +DimCellOverhead).

Title must be `NewFlexColumn(KeyTitle, "Title", 1)`.

Channel cell: raw string (no pre-truncation; evertras truncates ANSI-aware).

#### 1c. `builder.go`

```go
func VideoColumns(cols []VideoColumnDef) []etable.Column { ... }
func BuildVideoRows(videos []domain.Video, cols []VideoColumnDef, ctx RenderContext) []etable.Row { ... }
```

`BuildVideoRows`: iterate, call each `col.Cell(VideoCell{v, i, ctx})`, set `.WithStyle(styles.Dim)` if faded.

#### 1d. `table.go`

`NewVideoTable(cols []VideoColumnDef) etable.Model` — constructs table with standard styles (header, highlight, base), custom KeyMap (g/G unbound), outer border off, row border off, focused.

**Verification:**
```bash
go build ./internal/tui/videotable/...
```
(No callers yet — build just checks compilation.)

---

### Phase 2 — Revert ANSI workarounds from `render.go` and `table_helpers.go`

**Goal:** Remove all overhead code that becomes dead once evertras is in use.

Changes in `render.go`:
- Remove `DimCellOverhead = 7` constant
- Revert `ColViews = 8` (remove `+ DimCellOverhead`)
- Revert `ColDate = 11` (remove `+ DimCellOverhead`)
- Revert `SetDurFmt` formula: `ColDuration = maxLen*2 + 3` (remove `+ DimCellOverhead`)
- Revert default `ColDuration` initial value to `8*2 + 3` (= 19)

Changes in `table_helpers.go`:
- Remove `dimSwapReset` function
- Remove `dimCellOverhead` / `dimOverhead` usage in `toVideoRows`
- Remove `chSafeWidth` block
- `swapReset` may remain temporarily (used in `styledTitle`); marked for deletion in Phase 3

**Note:** At this point `table_helpers.go` still compiles because charmbracelet table is still imported. Verify:
```bash
go build ./...
```

---

### Phase 3 — Migrate video tabs: Subscriptions, Recommended, Local

**Goal:** Three tabs switch to evertras + videotable package. They all use `toVideoRows`
directly, making them the cleanest migration targets.

#### For each tab:

1. Replace `table table.Model` field with `tbl etable.Model`
2. Replace `newTable()` call in constructor with `videotable.NewVideoTable(tabCols)`
3. Declare package-level column slice `var xyzCols = []videotable.VideoColumnDef{...}`
4. Replace `computeVideoColumns(w, showCh)` call with `tbl.WithTargetWidth(w)` (flex handles title)
5. Replace `toVideoRows(...)` call with `videotable.BuildVideoRows(...)`
6. Replace `t.table.SetColumns(...)` + `t.table.SetRows(...)` with `t.tbl = t.tbl.WithRows(...)`
7. Replace `t.table.SetHeight(h)` with `t.tbl = t.tbl.WithTargetHeight(h)`
8. Replace `t.table.Cursor()` with `t.tbl.GetHighlightedRowIndex()`
9. Replace `t.table.GotoTop()` with `t.tbl = t.tbl.WithHighlightedRow(0).PageFirst()`
10. Replace `t.table.GotoBottom()` with `t.tbl = t.tbl.WithHighlightedRow(maxIdx).PageLast()`
11. Replace `t.table.SetCursor(n)` with `t.tbl = t.tbl.WithHighlightedRow(n)`
12. Replace `t.table.MoveUp/MoveDown` calls — REMOVE; let `tbl.Update(msg)` handle navigation
13. Replace `t.table.Height()` usage (page-size) — REMOVE; evertras tracks this internally
14. Replace `t.table.Update(msg)` with `t.tbl, cmd = t.tbl.Update(msg)`
15. Replace `t.table.View()` with `t.tbl.View()`

**Key difference for Local tab:** `local.go` has its own `toLocalRows()` (uses `domain.LocalVideo`).
`domain.LocalVideo` has the same video fields as `domain.Video` plus `Status`, `LastPositionMs`, etc.
Options:
  a. Add a `LocalVideoColumnDef` type parallel to `VideoColumnDef` in videotable
  b. Embed `domain.LocalVideo` fields into a `domain.Video` adapter before passing to BuildVideoRows
  c. Keep `toLocalRows()` as-is but return `[]etable.Row` instead of `[]table.Row`

Recommended (c): Port `toLocalRows()` to return `[]etable.Row` using `etable.NewRow` and
`.WithStyle(styles.Dim)` for watched local videos. Styling is simple — no cell-level ANSI needed.

**Verification per tab:**
```bash
go build ./...   # after each tab
```
Run the app and exercise the tab: scroll, sort, gg, goto-line, select video.

---

### Phase 4 — Migrate Channels tab

**Goal:** `channels.go` has four sub-tables. Migrate all four.

Sub-tables:
- `chVidTable` / `tagVidTable` — `domain.Video` → use `videotable.BuildVideoRows` (Phase 3 logic)
- `chTable` — `domain.Channel` → custom row builder inline in channels.go (simple, no registry needed)
- `tagTable` — string tags → trivial custom builder

For `chTable`, define a local `channelRow(ch domain.Channel, idx int) etable.Row` function
and `channelColumns(width int) []etable.Column`. No dim/bold styling needed — channels are
never faded. Width is manually computed (no flex needed since the channel table is a fixed set
of columns).

**Verification:**
```bash
go build ./...
```
Test: open channels tab, navigate between channel list, tag list, video sub-tables.

---

### Phase 5 — Migrate remaining tabs

Order: History → Search → Playlists → Activity → Downloading

#### History (`history.go`)

Domain type: `domain.HistoryEntry`. Columns: Num, Type(status icon), Title, Channel, Duration, Views, Date.

Port `toHistRows()` → `histRows() []etable.Row`:
- Type cell: `etable.NewStyledCell(render.FormatEvent(e.EventType), styles.Warning)` — no `swapReset` needed
- Title: plain string (no bold/dim logic for history entries currently)
- Other cells: plain strings via render formatters
- No row-level `.WithStyle` needed (history entries have no watch-state fading)

`detailTable` similarly: port `toDetailRows()` to return `[]etable.Row`.

#### Search (`search.go`)

Three sub-tables: `chTable` (channels), `vidTable` (videos), `drillTable` (videos).

`chTable` → inline channel row builder (same pattern as Channels tab Phase 4).
`vidTable` / `drillTable` → `videotable.BuildVideoRows`.

Height management: `applyResultHeights()` splits available height between chTable and vidTable.
Replace `SetHeight` calls with `WithTargetHeight` on each sub-table model.

#### Playlists (`playlists.go`)

`plTable` → custom builder for `domain.YTPlaylist` / `domain.Playlist` (simple string rows).
`vidTable` → `videotable.BuildVideoRows`.

`scrollToPlaylist()` uses `t.plTable.SetCursor(offset + i)` → replace with
`t.plTable = t.plTable.WithHighlightedRow(offset + i)`.

#### Activity (`activity.go`)

Simple: port `toActivityRows()` to return `[]etable.Row`. Type cell gets `styles.Warning` style
via `NewStyledCell`. No `swapReset`.

#### Downloading (`downloading.go`)

Simple: port `toDownloadRows()` to return `[]etable.Row`. Status column uses `renderStatus()`
(already has ANSI progress bar) — evertras handles ANSI width correctly, no changes needed to
the status renderer.

**Verification:**
```bash
go build ./...
```
Run app and test each migrated tab.

---

### Phase 6 — Cleanup

1. **Delete `table_helpers.go`** (or whatever remains of it).
   - `goto-top chord helpers` (`handleGotoPrefix`, `checkGotoNum`, `applyGoto`, `gotoLineView`) —
     move to `internal/tui/tab/chords.go` or keep inline per-tab if small.
   - `tableStyles`, `newTable`, `computeVideoColumns`, `toVideoRows`, `swapReset`, `dimSwapReset`,
     `styledTitle`, `titleSafeWidth`, `ralign`, `rowNum`, `videoTitleStyle`, `videoIndicator` — all
     replaced by `videotable` package; delete.

2. **Remove `charmbracelet/bubbles/table` import** from all tab files. Verify nothing imports it:
   ```bash
   grep -r "charmbracelet/bubbles/table" internal/
   ```

3. **Remove from `go.mod`** (if unused elsewhere):
   ```bash
   go mod tidy
   grep "charmbracelet/bubbles" go.mod   # bubbles has other components (key, help, etc.) — keep the module
   ```
   Note: `charmbracelet/bubbles` also provides `key`, `help`, `viewport` — only the `table` sub-package
   is removed. The module stays; the import path `charmbracelet/bubbles/table` is simply no longer used.

4. **Update `render.go`**: DimCellOverhead removed in Phase 2. Verify no references remain:
   ```bash
   grep -r "DimCellOverhead\|dimOverhead\|swapReset\|dimSwapReset\|chSafeWidth" internal/
   ```

5. **Update `docs/ansi-in-bubbles-table.md`**: Add a note that this document describes the
   workaround that was in place before the evertras migration. Keep it for historical reference.

6. **Run tests:**
   ```bash
   go test ./...
   ```

---

## Risk Register

| Risk | Mitigation |
|------|------------|
| evertras border overhead changes total width | Measure `tbl.View()` output width; adjust `WithTargetWidth` if needed |
| `g` key intercepted by tab before table gets it — gg chord works but PageFirst never fires | gg chord calls `WithHighlightedRow(0).PageFirst()` explicitly; table's g binding is removed from KeyMap |
| `WithTargetHeight` vs `WithPageSize` semantics differ per version | Test: set height to 10 rows, scroll to row 20, verify viewport follows |
| `GetHighlightedRowIndex()` is a pointer receiver method (`*Model`) but `Update` returns value | Always store tbl as value: `t.tbl, cmd = t.tbl.Update(msg)`; call pointer methods via `t.tbl.GetHighlightedRowIndex()` |
| evertras default footer (page N/M) takes 1 line of height | Call `.WithFooterVisibility(false)` in `NewVideoTable` |
| Local tab uses `domain.LocalVideo` not `domain.Video` | Port `toLocalRows()` inline with evertras rows (option c above) |
| History Type cell used `swapReset(styles.Warning.Render(...))` | Replace with `NewStyledCell(text, styles.Warning)` — no reset needed |
| sort chord rebuilds rows and must preserve cursor position | After `WithRows`, call `WithHighlightedRow(savedIdx)` |

---

## Files Changed Per Phase

| Phase | Created | Modified | Deleted |
|-------|---------|----------|---------|
| 0 | — | go.mod, go.sum | — |
| 1 | videotable/context.go, videotable/columns.go, videotable/builder.go, videotable/table.go | — | — |
| 2 | — | render.go, table_helpers.go | — |
| 3 | — | subscriptions.go, recommended.go, local.go | — |
| 4 | — | channels.go | — |
| 5 | — | history.go, search.go, playlists.go, activity.go, downloading.go | — |
| 6 | tab/chords.go (optional) | render.go | table_helpers.go |
