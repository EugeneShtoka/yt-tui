# Architecture Review — Execution Plan (P5)

**Created:** 2026-07-11 · **Status:** in progress — #1, #2, #6(a) landed (2026-07-12); #3, #5 remain · **Prereq:** P4 tabView migration complete (all 9 tabs behind `tabView`).

**Progress log:**
- ✅ **#1 goroutines → Cmd** (commit `2ee4cb8`): all 13 raw goroutines now `tea.Cmd`; `persistErrMsg` surfaces save failures; `internal/ui/commands.go` + tests.
- ✅ **#2 extract media + feed** (commit `5ed605e`): `internal/media` (SB math, links) + `internal/feed` (filters/merge/sort). `vidSort*` aliases `feed.Sort*`. Table-driven tests added. `feed` is the seed for #5.
- ✅ **#6(a) split parse from exec** (commit `6ebe5b4`): `parseVideoLines`/`parseChannelLines`/`parseMixedLines` take `io.Reader`; fixture tests for all filter branches + `buildArgs`/`isRateLimited`/`retryDelay`.
- ✅ **#4 shell-outs behind boundary**: new `internal/sys` with `EditorCommand` (returns `*exec.Cmd` for `tea.ExecProcess`) + `OpenURL`; editor env-resolution now unit-tested.
- ✅ **#3 inputMode enum**: the 6 mutually-exclusive text-input bools (`cmdMode`/`searchFocused`/`localFilterFocused`/`createMode`/`createTypeMode`/`subChEditMode`) collapsed into one `mode inputMode` (see `input_mode.go`) with `enterMode`/`exitMode`; `handleKey`'s if-ladder is now an exhaustive switch. **Audit finding:** the overlays (add/link/chapter/video-detail) + `showHelp` genuinely *stack* (link/chapter open over video-detail, restored on close by ladder order) so they are NOT mutually exclusive — left as separate bools by design, documented in `input_mode.go`. `subChEditMode`(int) → `subChEditKind` keeps the alias/tags sub-state. **⚠ Needs manual verification** of every mode entry/exit + the video-info→links→esc overlay path (can't drive the TUI in-agent); build/vet/`-race` + a transition controller test pass.
- 🔶 **#5 feed data owner — Recommended tab DONE**: `feed.Feed` now owns the rec video slice + loading/loaded/refreshing/page + the merge/filter pipeline (`Feed.Merge`). Model's 5 loose `rec*` fields collapsed to one `recFeed feed.Feed`; `recPage` gone. `viewCtx` shrank from `recVideos`/`recLoading`/`recRefreshing` (3 fields) to a single `recFeed *feed.Feed` — the plan's success metric. All rec sites route through Feed methods; the `switch m.activeTab` recommended arms now read `m.recFeed` (thin, not deleted — full deletion needs a `tabView.currentVideo()` method across all tabs). Tested. **Remaining #5 tabs: Subscriptions (subVideos, also read by Channels' tagVideos), Local (written from 5 sites), Channels (most complex — leave last or router-owned).**
- 🔶 **#5 — Subscriptions tab DONE**: `m.subVideos` → `m.subFeed feed.Feed` (via new `feed.New` constructor — subs has no independent fetch lifecycle, its display loading stays derived from `subChLoading`). All sites (rebuild, sort, unsubscribe, channel removal, switch arms, Channels' `tagVideos`) route through `subFeed`; `viewCtx` carries `subFeed *feed.Feed`. The cross-tab read (Channels `tagVideos` → `m.subFeed.Videos()`) is the Finding-2 sharing, handled trivially since both live on Model. Tested.
- ⬜ **#5 remaining tabs (Local, Channels) + #6(b)** — Local next if continuing.

This plan operationalizes the architectural review of yt-tui conducted 2026-07-11. It is the direct
follow-on to `docs/REFACTOR_PLAN.md` (P1–P4) and builds on the data-ownership finding recorded in
`docs/TABVIEW_DESIGN.md` (Finding 2) and the memory `p4-tabview-strategy`.

**Read these first when resuming:**
- `docs/TABVIEW_DESIGN.md` — Finding 2 (feed data is written across tab boundaries; only cursor/scroll
  could move into views). This plan's items #2 and #5 are the resolution of that finding.
- `internal/ui/view_tab.go` — the `tabView` / `viewCtx` / `viewIntent` seam that P4 built. Item #5 changes
  what `viewCtx` carries.
- `internal/ui/store.go` — the `Store` interface. Item #6 mirrors this pattern for the process layer.

---

## Why this exists (the one-paragraph thesis)

The P4 refactor partitioned tab state by **widget** (cursor/scroll/sort moved into `*View` structs) but
the real coupling is by **data ownership**. The feed slices (`recVideos`, `subVideos`, `subChannels`,
`localVideos`) are written from multiple tabs + async callbacks, so they could not move into any single
view — they stayed on `Model`. Consequence: `Model` is still a ~180-field God object, the per-tab
`switch m.activeTab` arms survived (`currentVideo` model.go:755, `jumpToLine` :824, `jumpToLast` :856,
`localFilteredVideos` :730), and `viewCtx` is accreting per-tab fields + render-closures, becoming a
second God object. The fix is to introduce the **data-owning service layer** the earlier docs already
called for. Two adjacent issues (raw goroutines in `Update`, unmockable `yt-dlp`) are correctness-grade
and get done first because they're cheap and independently valuable.

**Priority order is deliberate:** 1–4 are low-risk, mechanical, independently shippable. 5–6 are the
architectural payoff and carry more risk; do them last, one tab at a time.

---

## Item #1 — Convert raw goroutines to `tea.Cmd` [CORRECTNESS · do first]

### Problem
13 raw `go func(){}` launches inside the `ui` package, most fired from inside `Update`. Each:
- **Breaks MVU purity** — side effects should live in the returned `tea.Cmd`, which the runtime schedules.
- **Is a latent data race** — the goroutine captures `m.db` / `m.ytClient` (from a value-copy `Model` the
  runtime owns) and runs concurrently with the next `Update` on the *next* copy. Some sites defensively
  copy the slice arg (good); others capture `m` / `m.ytClient` directly.
- **Swallows errors** (`_ = ...`), so failed persists are invisible.

### Exact sites (all in `internal/ui/update.go` unless noted)
| Line | Call | Msg/context handler | Notes |
|------|------|--------------------|-------|
| 85   | `m.db.SaveYTPlaylists(pls)` | `YTPlaylistsMsg` | copies `pls` arg ✓ |
| 101  | `m.db.SaveYTPlaylistVideos(id, v)` | `PlaylistVideosMsg` | copies args ✓ |
| 131  | `m.db.DeleteChannelVideos(msg.ChannelID)` | `UnsubscribeMsg` | bare `go m.db.…` |
| 155  | `m.ytClient.AddToPlaylist(plID, v.ID)` | `CreatePlaylistMsg` (add-after-create) | captures `m.ytClient` |
| 297  | `m.db.SaveSubscribedChannels` + `SaveFeedCache` | `ChannelListMsg` | copies args ✓ |
| 335  | `m.db.SaveChannelVideos(chID, vids)` | `ChannelVideosMsg` ch-background | copies args ✓ |
| 349  | `m.db.SaveChannelVideos(chID, vids)` | `ChannelVideosMsg` stale-response | copies args ✓ |
| 365  | `m.db.SaveChannelVideos(chID, merged)` | `ChannelVideosMsg` merged | copies args ✓ |
| 442  | `m.db.SaveFeedCache("recommended", filtered)` | `handleFetchResult` | bare `go m.db.…` |
| 672  | `os.Remove(p)` loop | `execClear("downloads")` | filesystem side effect |
| 1871 | `m.ytClient.AddToPlaylist(pl.ID, v.ID)` | `handleAddOverlay` | captures `m.ytClient` |
| 2501 | `m.db.DeleteChannelVideos(chID)` | `unsubscribeLocal` | bare `go m.db.…` |

(One more raw goroutine exists elsewhere in `ui/` — `grep -rn 'go func\|go m\.\|go d\.' internal/ui/*.go`
finds 13 total; enumerate before starting.)

### Fix pattern
Each becomes a fire-and-forget `tea.Cmd` returning a message (nil on success, an error msg on failure).
Add a small persist-error message type so failures surface in the status bar:

```go
// update.go (near cmdErrMsg)
type persistErrMsg struct{ err error }

// Update: case persistErrMsg: m.setStatus("save: "+msg.err.Error(), true); return m, nil
```

Cmd helper (co-locate with the fetcher/db-adjacent commands, or a new `internal/ui/commands.go`):

```go
func saveChannelVideosCmd(db Store, chID string, vids []youtube.Video) tea.Cmd {
    return func() tea.Msg {
        if err := db.SaveChannelVideos(chID, vids); err != nil {
            return persistErrMsg{err}
        }
        return nil
    }
}
```

Then at each site, **replace `go …` with returning the cmd batched onto the existing return**:

```go
// before:  go func(chID string, vids []youtube.Video) { _ = m.db.SaveChannelVideos(chID, vids) }(id, v)
//          return m, nil
// after:
return m, saveChannelVideosCmd(m.db, id, v)
// or when already returning a cmd:
return m, tea.Batch(existingCmd, saveChannelVideosCmd(m.db, id, v))
```

### Care points
- **Two args at :297** (SaveSubscribedChannels + SaveFeedCache) → one cmd that does both sequentially,
  or `tea.Batch` two cmds. Preserve current ordering (channels then feed) if it matters — it doesn't
  functionally, but keep it for diff clarity.
- **:155 and :1871 `AddToPlaylist`** capture `m.ytClient` (a `*YTClient`, safe to share) — pass it in as
  a param so the cmd doesn't close over `m`. There is already a `youtube.RemoveYTPlaylistVideo(client, …)`
  cmd constructor (ytapi.go:338) to mirror; consider adding `youtube.AddToPlaylistCmd(client, plID, vID)`
  in the `youtube` package instead of an inline closure, for consistency.
- **:672 `os.Remove` loop** — filesystem, not DB. Same treatment: `deleteFilesCmd(paths []string) tea.Cmd`.
  Collect per-file errors into one message or ignore individually (current behavior ignores).
- **Do NOT change** the `Downloader.WaitForEvent()` pattern (update.go:389) — that's the *correct* model
  and the template for all of this.

### Verification
- `go build ./... && go vet ./...`
- `go test -race ./...` — the race detector is the whole point; run the download/subscribe/channel-video
  paths. Add a controller test on `fakeStore` asserting the cmd calls the right `Store` method.
- Manual: subscribe/unsubscribe, open a channel, clear downloads — status bar should show a save error if
  the DB is made to fail (temporarily inject an error in `fakeStore`).

### Scope: `internal/ui/update.go` (+ maybe new `commands.go`, + `youtube/ytapi.go` for AddToPlaylistCmd).
### Est: ~half a day. Fully mechanical.

---

## Item #2 — Extract pure domain logic out of `update.go` [STRUCTURE · low risk]

### Problem
`update.go` (2,866 lines) contains pure, UI-free, business-critical functions buried where they can't be
found or tested in isolation. These are the highest-value-to-test code in the app.

### Move to a new `internal/media` package (pure, no UI/tea imports):
| Function | Line | What it does |
|----------|------|-------------|
| `processChapters` | 2605 | filter SponsorBlock chapters, adjust timecodes → returns `[]db.Chapter, []db.SBSegment` |
| `originalToAdjustedSec` | 2647 | SponsorBlock timeline math |
| `originalToAdjustedMs` | 2663 | SponsorBlock timeline math |
| `adjustedToOriginalMs` | 2681 | SponsorBlock timeline math (used in `positionTick`, update.go:48) |
| `extractLinks` | 1819 | parse URLs from description |

**Why these first:** the SB time-conversion functions are a nasty bug class (off-by-seconds in playback
resume). They're pure `int64→int64` / `float64→float64`. Table-driven tests are trivial and high-value.

### Move to a new `internal/feed` package (pure filter/sort pipeline):
| Function | Line |
|----------|------|
| `filterByMinDuration` | 2167 |
| `filterByMinViews` | 2182 |
| `filterByAge` | 2197 |
| `filterDownloaded` | 2233 |
| `filterHidden` | 2244 |
| `filterBlacklisted` | 2256 |
| `matchBlacklisted` | 2273 |
| `filterSubscribed` | 2308 |
| `mergeVideos` | 2217 |
| `preserveCursor` | 2152 |
| `removeVideoByID` / `removeChannelVideos` / `removeChannelByID` | 2286 / 2296 / 2359 |
| `sortVideos` / `sortLocalVideos` / `sortByMode` | model.go 972 / 978 / 952 |

**Note the dependency:** `filterBlacklisted` / `matchBlacklisted` take `config.BlacklistedChannel` and
`*config.Config` — `feed` will import `config` (fine, `config` is a leaf). `sortByMode` is generic; keep
it exported so both `youtube.Video` and `db.LocalVideo` callers work.

### Approach
1. Create `internal/media` and `internal/feed` packages. Move functions verbatim; export them (capitalize).
2. Update call sites in `update.go` / `model.go` to `feed.FilterByAge(…)`, `media.ProcessChapters(…)`, etc.
3. This is a **behavior-preserving move** — the diff should be pure relocation + qualifier prefixes.
4. Add `internal/media/*_test.go` and `internal/feed/*_test.go` — table-driven. This is where the payoff is.

### Care points
- `handleFetchResult` (update.go:420-457) chains six of these filters — it stays in `ui` but now calls
  `feed.*`. That chain is the reference for what the `feed` package's public surface must support.
- `checkVideoHideAutoBlacklist` (2577) and `removeChannelFromFeeds` (2589) mutate `Model` — leave in `ui`.
- Do NOT move rendering, `tea.Cmd`, or `Model`-receiver functions.

### Verification: `go build/vet/test`; new package tests must cover every filter branch + SB math edges
(zero-length segs, segment at t=0, overlapping segments).
### Scope: new `internal/media`, `internal/feed`; edits to `update.go`, `model.go`.
### Est: ~half a day. This item is a **prerequisite framing for #5** (the `feed` package will grow into the
data owner).

---

## Item #3 — Collapse input-mode booleans into one `inputMode` enum [CLARITY · low risk]

### Problem
`handleKey` (update.go:723-772) gates input on ~11 independent booleans that are actually **mutually
exclusive modes**, enforced only by if-ladder *order*:

```
cmdMode, localFilterFocused, searchFocused, createTypeMode, createMode,
subChEditMode(int), addOverlay, linkOverlay, chapterOverlay, vidDetailOverlay, showHelp
```

Setting two by accident = silent priority bug. No compiler help when adding a mode.

### Fix
Introduce one field + enum:

```go
type inputMode int
const (
    modeNormal inputMode = iota
    modeCommand
    modeSearchInput
    modeLocalFilter
    modeCreateType
    modeCreatePlaylist
    modeChannelEdit
    modeAddOverlay
    modeLinkOverlay
    modeChapterOverlay
    modeVideoDetail
)
```

`handleKey` head becomes an exhaustive switch:

```go
switch m.mode {
case modeCommand:       return m.handleCmdInput(msg)
case modeLocalFilter:   return m.handleLocalFilter(msg)
case modeSearchInput:   return m.handleSearchInput(msg)
// … one arm per mode …
case modeNormal:        // fall through to chord/goto/nav dispatch (rest of handleKey)
}
```

### Care points (this one has traps — go slow)
- **`showHelp` is a toggle, not a strict mode** (any key dismisses, handleKey:768). Decide: fold into the
  enum (`modeHelp`) or keep as a separate bool that's checked first. Recommend `modeHelp` for uniformity.
- **`subChEditMode` is an `int`** (0/1/2 = none/alias/tags), not a bool — the *which-field* sub-state must
  survive. Keep a separate `subChEditKind` field; `mode == modeChannelEdit` replaces the `!= 0` check.
- **Overlays can stack**: video-detail → open links/chapters overlay → back to video-detail. Current code
  handles this with independent bools + the render layer drawing whichever is set. If you go single-enum,
  you need a **small `[]inputMode` stack** (push on open, pop on close) OR keep video-detail as the base
  and let link/chapter be a sub-overlay field. **Audit the open/close transitions first**:
  `openVideoDetail` (2707), `openLinksForVideo` (2762), `openChaptersForVideo` (2737),
  `closeVideoDetail` (1625), `handleVideoDetailKey` (1640), `moveOverlayCursor` (1719). Map every
  set-true / set-false of these bools before touching them.
- **The render layer (`view.go`) also reads these bools** to decide overlays. Every read site must migrate
  to `m.mode ==` / stack-contains. `grep -n 'Overlay\|Mode\|Focused\|showHelp' internal/ui/view.go`.
- Setters are scattered: search each bool's assignments (`grep -n 'searchFocused =' internal/ui/`) — every
  `= true` becomes `m.enterMode(modeSearchInput)`, every `= false` becomes a return to `modeNormal` (or
  stack pop). Consider `enterMode`/`exitMode` helpers to centralize.

### Verification: exhaustive manual pass through every mode entry/exit + the overlay-stack path
(video info → links → esc → still in video info → esc → normal). `go test` (add a controller test that
drives mode transitions if feasible).
### Scope: `model.go` (field + enum), `update.go` (handleKey + all setters), `view.go` (all readers).
### Est: ~1 day. **Higher touch than #1/#2 despite being "clarity" — the overlay stack is the risk.**

---

## Item #4 — Move shell-outs behind a boundary [STRUCTURE · tiny]

### Problem
Two `exec.Command` calls sit in `update.go`:
- `openConfigInEditor` (update.go:705) — spawns `$EDITOR`.
- `handleLinkOverlay` (update.go:1764) — `xdg-open <url>`.

Small, but they're OS shell-outs living in the UI dispatch file, and they're not testable.

### Fix
Fold into item #6's `CommandRunner` if doing #6; otherwise a tiny `internal/sys` package with
`OpenInEditor(path string) error` and `OpenURL(url string) error`. Low priority — bundle with #6 or #2.

### Est: ~1 hour. Optional / opportunistic.

---

## Item #5 — Introduce `internal/feed` as the data owner (complete P4) [ARCHITECTURE · higher risk]

### This is the payoff. Do it LAST and ONE TAB AT A TIME.

### Problem restated
Views own cursor/scroll but not their data, so `Model` still owns `recVideos` + lifecycle flags, the
per-tab `switch m.activeTab` arms survive, and `viewCtx` keeps growing. The `tabView` interface is
"done" structurally but hasn't delivered the decoupling it promised.

### Target
Grow the `internal/feed` package from item #2 into a **stateful feed owner**. A `feed.Feed` owns the
slice + its lifecycle flags + the filter pipeline, takes a `Store`, and exposes query methods the view
needs:

```go
type Feed struct {
    videos     []youtube.Video
    loading, loaded, refreshing bool
    sort       int
    // …
}
func (f *Feed) Videos() []youtube.Video
func (f *Feed) At(i int) (youtube.Video, bool)
func (f *Feed) Len() int
func (f *Feed) Merge(incoming []youtube.Video, cfg …) // the handleFetchResult pipeline
```

Then `recommendedView` holds a `*feed.Feed` (or Model holds `feed.Feed` and the view references it), and
`recommendedView` can answer `currentVideo()` / `jumpToLast()` / `context()` **itself** — deleting the
`tabRecommended` arm from `currentVideo` (model.go:764), `jumpToLine` (:827), `jumpToLast` (:859),
`localFilteredVideos` (:733). `viewCtx` stops needing `recVideos`/`recLoading`/`recRefreshing`.

### Migration order (mirror P4's incremental approach — memory `p4-tabview-strategy`)
1. **Recommended first** — most entangled (6-filter merge pipeline in `handleFetchResult`), so it proves
   the `Feed` API is expressive enough. If Recommended fits, the rest are easier.
2. Then **Subscriptions** (`subVideos`) — but note `subVideos` is *also* read by Channels' tag-video
   aggregation (`tagVideos`, model.go:583). The `Feed` for subs must be shareable/readable by Channels.
   This is the cross-tab-write reality of Finding 2 — handle it explicitly.
3. **Local** (`localVideos`) — written from 5 sites *outside* the Local handler (clear-downloads,
   Downloading's delete, `refresh`, download-complete `handleDownloadEvent`:463). The `feed`/`library`
   owner must expose a `Reload(Store)` these sites call, instead of each mutating the slice.
4. Channels' `subChannels` + `subChVideos` + `subChLatest` — most complex; do last or leave router-owned.

### Care points (this is where it gets hard)
- **Bubble Tea value-copy semantics**: `Model` is copied by value every `Update`. If `Feed` is a value
  field on `Model`, pointer-method mutations persist (this is the same trick P4 used — see
  `activeView()` returning `&m.field`, view_tab.go:146). If `Feed` holds a slice, the slice header copies
  cheaply but the backing array is shared — fine for reads, care on concurrent writes (there are none if
  #1 is done, because all writes go through `Update`).
- **Async writes**: `ChannelVideosMsg` / `FetchResultMsg` handlers write these slices. After #1 they're
  synchronous within `Update`; they call `feed.Merge(...)` instead of inline filter chains.
- **Do NOT try to move all four feeds at once.** One tab, green build + tests, commit, next tab. Exactly
  how P4 was done (Activity → History → … → Playlists).
- **`viewCtx` shrinkage is the success metric**: after Recommended migrates, `recVideos`/`recLoading`/
  `recRefreshing` should be deletable from `viewCtx` (view_tab.go:26,52,53) and the `renderList` closure
  should read from the `Feed`. If `viewCtx` isn't shrinking, the seam is still wrong.

### Verification: per-tab, `go test -race`; assert the deleted `switch m.activeTab` arm's behavior is now
in the view/feed (add view tests). Manual: full nav + refresh + filter on each migrated tab.
### Scope: `internal/feed` (grows), `model.go`, `update.go`, `view_tab.go`, per-tab `view_*.go` + tests.
### Est: ~1 day per feed, 3–4 days total. **Highest risk; gated behind #1–#2 landing green.**

---

## Item #6 — `CommandRunner` seam for the process layer [TESTABILITY · optional/fuller]

### Problem
`yt-dlp` and the player are invoked via direct `exec.Command` in ~8 places with no seam:
- `internal/youtube/fetcher.go`: lines 115, 172, 366, 509, 550
- `internal/youtube/ytapi.go`: line 48
- `internal/player/`: `simple.go:16`, `mpris.go:44`

Cannot unit-test the retry/rate-limit logic (`runAndParseVideos` fetcher.go:152, `isRateLimited`,
`retryDelay`) or the JSON/progress parsing against fixtures without a live `yt-dlp` binary.

### Two levels — do (a) cheaply even if you skip (b)

**(a) Split parse from exec [cheap, high value].** `tryParseVideos` (fetcher.go:114) does both. Extract
the pure scanner loop:

```go
func parseVideoLines(r io.Reader) ([]Video, error) { /* the scanner + filters */ }
```

Feed `strings.NewReader(fixture)` in tests → cover every filter branch (`IEKey=="YoutubeTab"`,
`ViewCount==0`, malformed JSON, empty ID/Title — fetcher.go:130-146) with zero process spawns. Do the
same for `tryParseChannels` (172), `tryParseMixed` (365), the download progress regex (downloader.go:96,
166). **This alone covers most of the risk.**

**(b) Abstract the runner [fuller].** One seam the whole app shares, mirroring `Store`:

```go
type CommandRunner interface {
    Run(ctx context.Context, name string, args ...string) (stdout io.ReadCloser, err error)
}
```

Inject into `fetcher`, `downloader`, `player`. Production impl wraps `exec.CommandContext`; test impl
replays canned stdout/stderr + chosen exit code. Now the retry loop, rate-limit handling, and download
progress parsing are deterministically testable. Note the asymmetry today: `Store` mocks the DB, nothing
mocks the process layer — that gap is the coverage ceiling.

### Care points
- `buildArgs` (fetcher.go:95) and `downloader.buildArgs` (downloader.go:233) are already pure and testable
  today — add tests for them cheaply (assert cookie/subtitle/SponsorBlock flags per config).
- The downloader's stdout scanning is stateful (progress → events on a channel); level (a) extraction
  there means parsing a line → `Event`, which is testable independently of the goroutine.

### Verification: new fetcher/downloader parse tests on fixtures (capture real `yt-dlp --dump-json` output
once, commit as testdata). `go test ./internal/youtube/... ./internal/downloader/...`.
### Scope: `internal/youtube`, `internal/downloader`, `internal/player`; optional new `internal/sys`.
### Est: (a) ~half a day; (b) ~1 day. Do (a) with #2; defer (b) unless coverage is a goal.

---

## Recommended execution order for next session

1. **#1 goroutines → Cmd** (½ day) — correctness, mechanical, `-race` verifies. **Start here.**
2. **#2 extract `media` + `feed` pure funcs** (½ day) — free tests, sets up #5.
3. **#6(a) split parse from exec** (½ day) — free tests, bundle with #2's testing push.
4. **#3 inputMode enum** (1 day) — do carefully, audit overlay stack first.
5. **#5 feed data owner, Recommended tab only** (1 day) — prove the seam, then decide on the rest.
6. Defer: #4 (fold into #6), #6(b), remaining #5 tabs.

Items 1–3 are a clean, low-risk first session that leaves the tree green and materially better. Item 5 is
a separate, riskier session gated on 1–2 being solid.

## Global verification gate (run after every item)
```
go build ./... && go vet ./... && gofmt -l . && go test -race ./...
```
All must be clean before commit. Follow the P4 per-slice rule: **add a controller test on `fakeStore`
(`internal/ui/store_test.go` pattern) or a pure-package test for every change.**

## Cross-references
- `docs/REFACTOR_PLAN.md` — P1–P4 (this is P5).
- `docs/TABVIEW_DESIGN.md` — Finding 2, the data-ownership constraint #5 resolves.
- memory `p4-tabview-strategy` — the incremental per-tab discipline #5 must follow.
- `internal/ui/store.go` — the interface pattern #6 mirrors.
- `internal/downloader/downloader.go:342` (`WaitForEvent`) — the *correct* Cmd pattern #1 emulates.
</content>
</invoke>
