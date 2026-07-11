# TabView Decomposition — Design & Findings (P4)

_Companion to `docs/REFACTOR_PLAN.md` P4. Written at the start of P4 execution (Opus)._

## Goal (unchanged from the plan)

Replace the monolithic `Model` + ~13 parallel `switch m.activeTab` sites with
per-view units, keeping `Model` as a thin router over shared services. The
long-term end-state is a `TabView` interface; this document records the design
**and the findings from the first slice that revise how we get there.**

---

## Findings from mapping the current code

Before writing any abstraction I traced the real state ownership. Three findings
change the plan's recommended approach.

### Finding 1 — The tabs are wildly asymmetric

- **Isolated single-pane lists:** Activity, History, Local, Downloading — a
  slice + cursor + viewStart, one render function, one key handler.
- **Multi-pane / multi-mode:** Channels (flat vs. tags-grouped, 2 panes, alias/tag
  edit inputs, drill-down), Search (channel results + video results + channel
  drill-down), Playlists (list pane + video pane, create dialog, add overlay).

A single `TabView` interface must serve both. The simple tabs need ~4 methods;
the complex tabs pull in overlays, sub-panes, and text inputs. An interface
designed from a simple tab will be wrong for the complex ones, and vice-versa.

### Finding 2 — "Move the tab's data into the view" is NOT achievable tab-by-tab

The plan's reference-slice acceptance says a migrated tab's state "no longer
appears in the global `Model` struct." Empirically this holds for **cursor /
scroll / sort / and truly-private data**, but **not for the feed-data slices**,
which are written across tab boundaries:

- `m.localVideos` (the Local tab's data) is written from **5 sites outside the
  Local handler**: the `clear downloads` command (`update.go` `execClear`), the
  **Downloading** tab's `Delete` handler, `refresh`, and two other delete paths.
- `m.localVideoIDs` / `m.streamedVideoIDs` / `m.videoPositions` are read by
  Recommended, Downloading, **and** Local for row indicators.
- `m.subChannels` / `m.subscribedChannelIDs` are shared by Subscriptions,
  Channels, Search (subscribe chord), and Recommended (feed filtering).

**Conclusion:** feed/library data that is written across tabs must live in a
shared **services** layer, not be "owned" by one view. Only the genuinely-private
state (cursor, viewStart, per-tab sort mode, and data with no external writer)
can move into a view. This revises the plan's per-tab-ownership acceptance.

### Finding 3 — The test net is thinner than P4's prerequisite assumes

P1.1 delivered **pure-function** tests only (sort, filter, merge, viewport math,
SponsorBlock). There are **no controller/Update tests**. The plan says "do not
start P4 without the test net"; the net that exists does not cover `Update`
behavior. Mitigation: **every slice must add a controller test for its `Update`**,
built on the existing `fakeStore` (`store_test.go`). This slice does so.

---

## Design decision: group-into-sub-struct first, extract interface later

Committing to a `TabView` **interface** from one migrated tab is the exact
"wrong abstraction is expensive to unwind" risk the plan flags. With one data
point the shared method set is a guess. So the first phase is deliberately
**pre-interface**:

1. **Group** each tab's private state into a small view struct (`activityView`,
   `localView`, …) held as a field on `Model`.
2. **Delegate** each parallel-switch arm to a method on that struct.
3. Once **3 tabs** (one isolated, one list-with-actions, one multi-pane) are
   grouped, the common method set is *observed*, not guessed — then extract the
   `TabView` interface with confidence.

This is safe (mechanical, reversible), testable (each view gets a unit test),
and it still collapses the parallel switches — the actual pain P4 targets.

### The end-state interface (the target, once 3 tabs are grouped)

```go
type tabView interface {
    // Update handles a key for this tab. Cross-tab navigation is NOT performed
    // here; the view returns a nav intent (or a tea.Cmd carrying a message) and
    // the router owns the transition.
    Update(msg tea.KeyMsg, svc *services) (tabView, tea.Cmd)
    View(svc *services, height int) string
    Context() ContextID
    CurrentVideo(svc *services) (youtube.Video, bool)
    OnActivate(svc *services) (tabView, tea.Cmd)
    Refresh(svc *services) (tabView, tea.Cmd)
}
```

### The `services` struct (the DIP fix — and a caution)

```go
type services struct {
    cfg    *config.Config
    db     Store
    dl     *downloader.Downloader
    player player.Backend
    yt     *youtube.YTClient
    // + shared feed data written across tabs (see Finding 2):
    //   localVideoIDs, streamedVideoIDs, videoPositions, subscribedChannelIDs, …
    // + callbacks the router owns: setStatus, pageSize
}
```

**Caution recorded for future executors:** for the entangled tabs, `services`
trends toward "most of `Model`." That is acceptable (it is an explicit seam and
kills the implicit global coupling), but do not pretend the complex tabs become
independent — they share a data layer by nature. Keep `services` a real struct
of shared collaborators, not a dumping ground for one tab's private state.

### Ownership split (router vs. view)

- **Router (`Model`) keeps:** the tab bar, all overlays (video-detail, link,
  chapter, add-to-playlist), `:`-command mode, chords/goto, help, local-filter,
  cross-tab navigation, and the **shared feed data** from Finding 2.
- **Each view owns:** its cursor, viewStart, per-tab sort mode, and any data with
  no external writer.

---

## Revised migration order (was: "start with Local")

Local is a **poor** first slice (Finding 2: its data is externally written).
Ordered by increasing entanglement:

1. **Activity** ← reference slice (this commit). Fully isolated: its state has
   zero external readers/writers. Proves the group-and-delegate mechanism and the
   controller-test pattern end-to-end.
2. **History** — isolated list with a detail sub-view.
3. **Downloading + Local together** — they share `localVideos` / `localVideoIDs`
   / the downloader; migrate as one unit into services-backed views.
4. **Recommended, Subscriptions** — video lists sharing feed filters.
5. **Search** — multi-result-type single tab.
6. **Channels, Playlists** — the multi-pane tabs; do these last, and only after
   the `services` shape is proven. Revisit whether the `tabView` interface should
   model sub-panes explicitly.

**After ~3 tabs are grouped, pause and extract the interface** (or decide it is
not worth it — grouping alone may deliver most of the value).

---

## Reference slice as built (Activity)

- New `internal/ui/view_activity.go`: `type activityView struct { entries; cursor; vs }`
  with `load`, `update` (returns the selected entry + a bool nav-intent; the
  router performs the cross-tab jump via the existing `navigateToActivity`), and
  `render`.
- `Model.actEntries/actCursor/actVS` collapsed to a single `activity activityView`
  field.
- The 5 Activity switch-arms (`Update` dispatch, `renderContent`, `onTabActivated`,
  `refresh`, plus `loadActivity`) now delegate to the view. `renderActivity`
  moved off `Model` into `activityView.render`.
- Activity was already **absent** from `currentVideo`, `jumpToLine`, `jumpToLast`,
  `applySortAction`, and `currentContext` (it has no video/sort semantics — a
  detail worth noting: goto-line/goto-bottom do not act on Activity today, and
  that behavior is preserved).
- New `internal/ui/view_activity_test.go`: controller test for cursor movement,
  page movement, and drill-down nav-intent using `fakeStore`.

**Behavior:** byte-identical. No user-facing string, key, or layout changed.
