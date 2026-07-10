# yt-tui — Refactor Plan (model-tiered, ROI-ordered)

_Companion to `docs/AUDIT_REPORT.md`. Ordered so highest return-on-investment work appears first._

## How to use this document

Each task is self-contained and written to be executed by a model with the recommended capability tier. Every task lists: **Goal · Files · Steps · Acceptance · Verify · Model**.

**Global rules for every executor**
- Baseline is green: `go build ./...` succeeds before you start. Keep it green.
- After any change run, in order:
  ```
  go build ./...        # must succeed
  go vet ./...          # must be clean
  go test -race ./...   # must pass
  ```
- **Behavior-preserving unless the task says otherwise.** Do not rename user-facing strings, keybindings, or status messages.
- Make **one task = one commit**. Commit message: `refactor(scope): <task id> <summary>`. Do not push unless asked.
- Line numbers are approximate anchors; locate by **function name** (they will drift as you edit).
- If a task's acceptance cannot be met without a design decision not covered here, STOP and leave a `// TODO(refactor): <question>` note rather than guessing.

### Model tiers — answer to "how low can we go?"

| Tier | Use for | Why not lower |
|------|---------|---------------|
| **Haiku** | Single-file, mechanical, fully-specified edits verifiable by `go build`/`go test`. No cross-module design. | Lower tiers risk silent behavior drift in a codebase with no tests. |
| **Sonnet (workhorse minimum)** | Multi-file edits, behavior-preservation reasoning, pattern application across many call sites, writing thorough edge-case tests. | Haiku struggles to hold 6 duplicated call sites consistent and to reason about goroutine ownership. |
| **Opus (reserve for design)** | Architecture with high blast radius and genuine design judgment: the `Model` decomposition and the concurrency/config-safety design. Produce the design + the first vertical slice; Sonnet replicates. | These change the shape of the program; a wrong abstraction is expensive to unwind and un-testable to catch. |

**Short answer to the brief:**
- **Yes, Haiku can own several tasks** — the P0 mechanical fixes and the pure-function test suite (P0.3, P0.1a, P1.1). They are single-file and compiler/test-verifiable.
- **Sonnet is the absolute minimum** for anything touching multiple controllers or concurrency (P0.2, P2.x, P3.x, P1.2 interfaces).
- **Opus is recommended** for exactly two things: **P4 (Model→TabView decomposition)** design + first slice, and the **P0.1b concurrency-serialization design** for config writes. Everything else degrades gracefully to Sonnet.

---

## ROI-ordered master list

| # | Task | ROI | Blast | Model | Depends on | Comments |
|---|---|---|---|---|---|---| 
| P0.1a | Atomic config write (temp+rename) | High | Low | **Haiku** | — | ✅ **DONE** |
| P0.1b | Serialize config saves (single writer + mutex) | High | Med | **Opus** (design) → Sonnet | P0.1a | ✅ **DONE** |
| P0.2 | Fix MPRIS `poll` data race | High | Low | **Sonnet** | — | ✅ **DONE** |
| P0.3 | De-alias filter helpers (`make` not `[:0]`) | High | Low | **Haiku** | — | ✅ **DONE** |
| P1.1 | Unit tests for pure functions + `-race` in CI | High | Low | **Haiku** (scaffold) / Sonnet (edge cases) | — | ✅ **DONE** (scaffold: 22 tests + edge cases complete) |
| P1.2 | `Store` interface over `*db.DB`; inject into `ui` | High | Med | **Sonnet** | P1.1 | ✅ **DONE** |
| P2.1 | Extract shared video-action key helper (6 sites) | Med-High | Med | **Sonnet** | P1.1 | |
| P2.2 | Extract shared overlay-nav helper (3 sites) | Med | Low-Med | **Sonnet** | — | |
| P2.3 | Extract shared "open video detail" helper (2 sites) | Med | Low | **Sonnet** | — | |
| P2.4 | Generic `sortSlice[T]`; merge sortVideos/sortLocalVideos | Med | Low | **Sonnet** | — | |
| P2.5 | Honor `cfg.Keybindings` in input/overlay modes | Med | Med | **Sonnet** | — | |
| P3.1 | Capture yt-dlp stderr into errors | Med | Low | **Haiku** | — | |
| P3.2 | Single source-of-truth tab table; collapse 3 name lists | Med | Med | **Sonnet** | — | |
| P3.3 | Reflection/`cmp.Or` merge for `fillDefaults` | Low-Med | Low | **Sonnet** | P1.1 | |
| P3.4 | Memoize `chordDefs()` + sorted/filtered views | Med | Med | **Sonnet** | P4 ideally | |
| P4 | Decompose `Model` into `TabView` sub-models | Highest (long-term) | Very High | **Opus** (design + 1st slice) → Sonnet (rest) | P1.2 | |

> Sequencing note: do **P0 → P1 → P2/P3 → P4**. P4 is the highest long-term ROI but is deliberately last because P1 (tests) and P1.2 (interfaces) are what make it *safe*. Do not start P4 without the test net.

---

# Tier 1 — Haiku tasks (mechanical, single-file, compiler-verifiable)

## P0.1a — Atomic config write — ✅ DONE
**Goal.** `Config.save` must never leave a truncated/empty file.
**As built** (`internal/config/config.go`). `save` now encodes to `os.CreateTemp(dir, ".config-*.tmp")` in the target's directory, `Close`s, then `os.Rename`s over the target; on any error it removes the temp and returns. Same-directory temp keeps the rename atomic. `Save`/`Load` signatures unchanged. Verified: `go build ./...`, `go test -race ./internal/config/...` (see `TestAtomicSaveLeavesValidFile`).
**Model.** Haiku (as planned).

## P0.3 — De-alias filter helpers — ✅ DONE
**Goal.** Stop filters writing into the caller's backing array.
**As built** (`internal/ui/update.go`). All four filter functions (`filterByAge`, `filterDownloaded`, `filterHidden`, `filterBlacklisted`) now use explicit `make([]youtube.Video, 0, len(videos))` instead of aliasing the input slice. No mutations of caller's backing arrays.
**Verified.** `go build ./...` and `go test -race ./internal/ui` pass.
**Model.** Haiku (as planned).

## P3.1 — Capture yt-dlp stderr into errors
**Goal.** Download failures report a cause, not just an exit code.
**Files.** `internal/downloader/downloader.go` (`run`, ~lines 139-180).
**Steps.**
1. Replace the discard goroutine:
   ```go
   go func() {
       sc := bufio.NewScanner(stderr)
       for sc.Scan() {}
   }()
   ```
   with a bounded ring buffer capturing the last ~20 stderr lines under a local mutex or a `sync`-free single-goroutine slice, then close over it.
2. In the `cmd.Wait()` error branch, append the captured tail:
   ```go
   d.fail(item, fmt.Errorf("yt-dlp: %w: %s", err, strings.TrimSpace(lastStderr())))
   ```
   Keep the message to a few hundred chars max (truncate).
**Acceptance.** A forced failure (e.g. invalid URL) surfaces yt-dlp's real error text in the status bar.
**Verify.** `go build ./...`.
**Model.** Haiku. _(Escalate to Sonnet only if the goroutine/closure synchronization is unclear.)_

---

# Tier 2 — Haiku-scaffold / Sonnet-finish (tests)

## P1.1 — Unit tests for pure functions + `-race` gate — ✅ DONE
**Goal.** Establish a safety net before structural work; catch P0 regressions.
**As built** (`internal/ui/pure_test.go`, `internal/downloader/sanitize_test.go`, `internal/config/config_test.go` seeded):
- **Scaffold (Haiku):** 22 table-driven tests covering `mergeVideos`, `filterSubscribed`, `removeVideoByID`, `extractLinks`, `filterByAge`, `sanitizeFilename`, `cmdCompletionsFor`.
- **Edge cases (Sonnet):** `vsMove` (n=0, clamp high/low, circular wrap forward/backward, viewport invariants), `vsPage` (clamp at start/end, circular wrap both directions), `vsJump` (negative/over-end clamp, centering, near-top/bottom vs pinning), SponsorBlock (round-trip before/after/multi-segment, monotonicity, inside-segment maps to start).
- **Total:** 34+ tests; all pass under `-race`.
**Verified.** `go test -race ./internal/ui ./internal/downloader` passes.
**Model.** Haiku (scaffold) / Sonnet (edge cases).

---

# Tier 3 — Sonnet tasks (multi-file, behavior-preserving)

## P0.2 — Fix MPRIS poll data race
**Goal.** No unsynchronized access to `b.stopCh`; `-race` clean.
**Files.** `internal/player/mpris.go`.
**Steps.**
1. Change `poll` to take its channel as a parameter: `func (b *mprisBackend) poll(stopCh chan struct{})`.
2. In `exec`, after creating the new channel under lock, capture it into a local and pass it: `stop := b.stopCh` (inside the locked section), then `go b.poll(stop)` after unlock.
3. In `poll`, `select { case <-stopCh: ... }` — never read `b.stopCh` directly.
4. Confirm `Close` still closes the *current* `b.stopCh` under lock; each poller owns the channel it was handed, so a superseded poller exits when `exec`→`Close` closes the old channel first (exec already calls `b.Close()` at its top).
**Acceptance.** `go build ./...`; reasoning holds that two rapid `Launch` calls terminate the first poller. No field read of `b.stopCh` inside `poll`.
**Verify.** `go vet ./...`; if feasible add a tiny `-race` test that constructs the backend with a stub driver and calls `exec` twice (guard with build tag if D-Bus is required).
**Model.** Sonnet.

## P0.1b — Serialize config saves — ✅ DONE
**Goal.** Never run two `cfg.Save()` concurrently; never mutate `BlacklistedChannels` while encoding.
**As built** (`internal/config/config.go`, call sites in `internal/ui/update.go`):
1. Added `mu sync.Mutex` (guards mutable fields + serializes writes) and `saveReq chan struct{}` (1-deep, coalesces async saves) to `Config`. Both are unexported → ignored by the TOML encoder.
2. `Save()` now locks `mu`, resolves the path (prefers `c.ConfigFile`), and calls the atomic `save`. `save` itself does **not** lock (internal helper; the single-threaded `Load` startup call uses it directly).
3. Added `SaveAsync()` — non-blocking, sends to `saveReq` with a `default` drop to coalesce; falls back to `go c.Save()` if the worker isn't started. A `saveWorker()` goroutine (started at the end of `Load`, once `ConfigFile` is set) drains `saveReq` and calls `Save`.
4. Added locked `SetBlacklistID(idx, id)`; `AddBlacklistedChannel` now locks `mu`.
5. Call sites: `filterBlacklisted` uses `cfg.SetBlacklistID(bl, …)` + `cfg.SaveAsync()`; `hideChannel` uses `m.cfg.SaveAsync()`. No more `go cfg.Save()` / direct slice writes.
**Why this design.** The only cross-goroutine access to `BlacklistedChannels` is the worker encoding (read) vs. UI-thread mutations (write); both now hold `mu`, so it's data-race-free. Coalescing avoids goroutine pile-up when `filterBlacklisted` fires per-feed-filter. Worker lives for process lifetime (no shutdown needed for a TUI).
**Verified.** `go test -race ./internal/config/...` passes (`TestConcurrentBlacklistSaves`: 50 concurrent `AddBlacklistedChannel`+`Save`, clean under `-race`, final file valid TOML with all 50 entries).
**Note for future executors.** `Config` now holds a `sync.Mutex` → it must **never** be copied by value. It is always passed as `*config.Config` today; keep it that way (a value copy will trip `go vet -copylocks`).
**Model.** Design + implementation done at **Opus** level (concurrency contract). This is the kind of task the plan reserves Opus for.

## P2.1 — Shared video-action key helper
**Goal.** Remove the 6-way duplicated action block.
**Files.** `internal/ui/update.go` (`updateRecommended`, `updateSubAll`, `updateSubChannelsTags`, `updateSubChVideoPane`, `updateSearch`, `updatePlaylists`).
**Steps.**
1. Add:
   ```go
   // handleVideoAction runs the shared video actions (play/download/add/copy).
   // Returns handled=true if msg matched one of them.
   func (m *Model) handleVideoAction(msg tea.KeyMsg) (handled bool) { ... }
   ```
   Move the common `Play/PlayAudio/Download/DownloadAudio/AddList/CopyURL/DrillDown→downloadAndPlay` cases into it, using `m.currentVideo()`. Return `true` when a case fires.
2. In each controller, call it first: `if m.handleVideoAction(msg) { return m, nil }` (adapt to each controller's existing tea.Cmd return; where a case returns a cmd, keep those specific cases out of the shared helper).
3. **Caution:** some controllers have tab-specific overrides (e.g. search `DrillDown` on a channel row drills instead of plays; playlists has `Delete`). Keep those cases in the controller *before* calling the shared helper so they win.
**Acceptance.** Every tab's play/download/copy behavior is byte-identical to before (verify each manually). Net deletion ~70-80 lines.
**Verify.** `go build ./...`; manual smoke of each tab's `p`, `d`, `y`, `a`.
**Model.** Sonnet (needs to preserve per-tab overrides).

## P2.2 — Shared overlay-nav helper
**Goal.** De-duplicate `gPending`/`GotoBottom`/`Up`/`Down`/circular logic in `handleLinkOverlay`, `handleChapterOverlay`, `handleAddOverlay`.
**Files.** `internal/ui/update.go`.
**Steps.** Add `func (m *Model) moveOverlayCursor(sel, n int, msg tea.KeyMsg) (newSel int, handled bool)` implementing up/down/goto-top(`gg`)/goto-bottom(`G`) with `m.cfg.CircularNav`. Replace the three hand-rolled blocks; keep overlay-specific actions (open URL, play-from-chapter, copy) in place.
**Acceptance.** Navigation identical; `gg`/`G`/wrap behave as before in all three overlays.
**Verify.** `go build ./...`; manual smoke.
**Model.** Sonnet.

## P2.3 — Shared "open video detail" helper
**Goal.** Remove the ~22-line duplication between `handleKey`'s `VideoInfo` case and `updateSubChannelsTags`.
**Files.** `internal/ui/update.go`.
**Steps.** Extract `func (m *Model) openVideoDetail(v youtube.Video) tea.Cmd` containing the reset-state + cache-lookup + fetch logic. Call it from both sites.
**Acceptance.** Both entry points behave identically; note the tags-mode copy currently omits `vidDetailChapters` reset — unify to the `handleKey` version (which resets it) and confirm no regression.
**Verify.** `go build ./...`; open detail (`i`) from Recommended and from a tag's video list.
**Model.** Sonnet.

## P2.4 — Generic sort
**Goal.** Merge `sortVideos`/`sortLocalVideos`.
**Files.** `internal/ui/model.go`.
**Steps.** Introduce a generic keyed sorter or a small interface both `youtube.Video` and `db.LocalVideo` satisfy for the five sort keys. Simplest with Go 1.26 generics: extract comparators keyed by a `sortMode`, parameterized over a type constraint exposing `ViewCount/UploadDate/Title/Channel/Duration`. If the two structs don't share field access cleanly, keep two thin wrappers over one generic `sortByMode[T any](s []T, mode int, less map[int]func(a,b T) bool)`.
**Acceptance.** Identical ordering for both types across all modes; `vidSortNone` still no-ops.
**Verify.** `go build ./...`; add a test asserting parity with the old behavior on a fixed slice.
**Model.** Sonnet.

## P2.5 — Honor keybindings in input/overlay modes
**Goal.** Stop hardcoding `esc/enter/up/down/f2–f8`.
**Files.** `internal/ui/update.go` (`handleSearchInput`, `handleCmdInput`, `handleLocalFilter`, `handleCreateInput`, `handleChannelEditInput`).
**Steps.** Replace string comparisons with `key.Matches(msg, m.keys.Close)` / `m.keys.DrillDown` etc. where a configured binding exists. Keep `esc` always-cancel and `enter` always-accept semantics (config already guarantees `Close` includes `esc`). Leave `f2–f8` quick-jumps but document them, or gate behind a config flag — **do not silently remove** them.
**Acceptance.** Rebinding a key in config changes behavior consistently in these modes; default config behaves exactly as today.
**Verify.** `go build ./...`; smoke with default config.
**Model.** Sonnet.

## P3.2 — Single source-of-truth tab table
**Goal.** Collapse `tabNames`, `tabDebugNames`, `tabIDByName`, `DefaultTabs` into one table.
**Files.** `internal/ui/model.go`, `internal/ui/update.go`, `internal/config/config.go` (DefaultTabs).
**Steps.** Define `var tabMeta = []struct{ id int; name, display string }{...}` and derive the maps/arrays from it in an `init()` or package-level builders. Keep exported/name strings identical.
**Acceptance.** All lookups (`tabName`, `tabIDByName`, tab bar labels) produce identical output.
**Verify.** `go build ./...`; diff tab bar rendering before/after.
**Model.** Sonnet.

## P3.3 — Reduce `fillDefaults` boilerplate
**Goal.** Replace 60 hand-written nil-checks.
**Files.** `internal/config/config.go`.
**Steps.** Either (a) reflection walk over `KeyBindings` string fields filling empties from `defaultKeyBindings()`, or (b) keep explicit but generate via a table `[]struct{ get func(*KeyBindings)*string; def string }`. Prefer (a) only if a test locks behavior first.
**Acceptance.** A config missing any subset of bindings ends up identical to today's `fillDefaults` output. **Write the test first** (feed partial structs, compare to expected).
**Verify.** `go test ./internal/config/...`.
**Model.** Sonnet (depends on P1.1-style test to lock behavior).

## P3.4 — Memoize hot derived views
**Goal.** Stop rebuilding the chord registry and re-sorting on every keypress/render.
**Files.** `internal/ui/model.go`, `internal/ui/update.go`.
**Steps.**
1. Cache `chordDefs()` result on `Model` (or a pointer field); invalidate when `cfg`/`tabs` change (they never change at runtime today → build once in `NewModel`).
2. Cache sorted channels / tag videos behind a dirty flag set when the underlying slice or sort mode changes; clear on mutation.
**Acceptance.** No behavioral change; measurably fewer allocations (spot-check with `-benchmem` micro-bench or pprof). Be careful: `Model` is passed by value through Bubble Tea — cache via pointer-held structures (maps/slices) or recompute-on-write, not value fields that get copied stale.
**Verify.** `go build ./...`; optional benchmark.
**Model.** Sonnet. _(Ideally after P4, which changes where this state lives.)_

## P1.2 — `Store` interface over `*db.DB`
**Goal.** Give `ui` a seam to inject a fake DB (unlocks controller tests and P4).
**Files.** `internal/ui/*` (new `store.go`), `internal/db/db.go` (unchanged — it already satisfies the interface).
**Steps.**
1. In `ui`, declare a `Store interface { ... }` listing exactly the methods `Model` calls on `*db.DB` (grep `m.db.` to enumerate). Keep it narrow.
2. Change `Model.db` field type to `Store`; `NewModel` still receives `*db.DB` (satisfies it).
3. No behavior change; this is a type-widening.
**Acceptance.** `go build ./...`; a trivial `fakeStore` in a test can be substituted.
**Verify.** `go build ./...`; add one test constructing `Model` with a fake store.
**Model.** Sonnet (mechanical but ~40-method interface; needs completeness).

---

# Tier 4 — Opus tasks (architecture; design + first slice, then hand to Sonnet)

## P4 — Decompose `Model` into `TabView` sub-models
**Why Opus.** This reshapes the program; the abstraction choice determines whether the other ~10 `switch m.activeTab` sites collapse cleanly. A wrong seam is expensive and, pre-P1, un-testable to catch. Do the **design + one reference implementation**, then Sonnet ports the remaining tabs mechanically.

**Goal.** Replace the monolithic `Model` + parallel switches with per-view sub-models behind a small interface, keeping `Model` as a thin router over shared services.

**Prerequisites.** P1.1 (tests) and P1.2 (`Store` interface) landed.

**Design deliverables (Opus produces these first, as a short `docs/TABVIEW_DESIGN.md`):**
1. Interface, e.g.:
   ```go
   type TabView interface {
       Update(msg tea.Msg) (TabView, tea.Cmd)
       View(width, height int) string
       Context() ContextID          // for chord/sort dispatch
       CurrentVideo() (youtube.Video, bool)
   }
   ```
2. A `services` struct (`cfg *config.Config`, `db Store`, `dl *downloader.Downloader`, `player player.Backend`, `yt *youtube.YTClient`) passed to each view — this is the DIP fix.
3. Decide ownership of cross-cutting state: chord/goto/command-mode and overlays. Recommendation: keep global concerns (chords, `:`-command, help, video-detail overlay) on the router; move tab-local cursor/scroll/sort/data into each view.
4. Migration order (one commit per tab): start with the **simplest** tab (Local or Downloading) as the reference slice.

**Reference slice (Opus implements one tab end-to-end):**
- Create `internal/ui/view_local.go` with `type localView struct {...}` implementing `TabView`.
- Router `Update`/`View` delegate to the active view for that tab; all others stay on the legacy path during migration (adapter shim acceptable).
- Prove `currentVideo`, sort, and key handling for that tab now live in the view, and delete that tab's arm from the global switches.

**Acceptance (reference slice).** The migrated tab behaves identically; its state no longer appears in the global `Model` struct or the parallel switches; tests exist for its `Update`.

**Then (Sonnet, one task per remaining tab).** Port `recommended`, `subscriptions`, `channels`, `playlists`, `search`, `history`, `activity` following the reference. Each is its own commit + smoke test. When all are ported, delete the now-empty `switch m.activeTab` blocks in `currentVideo`, `jumpToLine`, `jumpToLast`, `currentContext`, `applySortAction`, `onTabActivated`, `refresh`, `forceRefresh`.

**Verify.** `go build ./... && go test -race ./...` after each slice; manual smoke of the ported tab.
**Model.** **Opus** for design doc + reference slice; **Sonnet** for each subsequent tab port.

---

## Suggested execution order (waves)

1. **Wave 1 (parallelizable, mostly Haiku):** ~~P0.1a~~ ✅, P0.3, P3.1, P1.1 scaffold. _(P0.1a and P0.1b already landed.)_
2. **Wave 2 (Sonnet + one Opus design):** P0.2, ~~P0.1b~~ ✅, finish P1.1 edge cases.
3. **Wave 3 (Sonnet):** P1.2, then P2.1–P2.5, P3.2, P3.3.
4. **Wave 4 (Opus design → Sonnet ports):** P4, then P3.4 memoization on the new structure.

Each wave ends green (`go build`, `go vet`, `go test -race`). Do not begin Wave 4 until Wave 1's tests exist.

## Progress log
- **2026-07-10 9:51pm — P0.1a + P0.1b landed (Opus).** Atomic config write + serialized/coalesced saves + `SetBlacklistID`; call sites in `update.go` updated. Seeded the test suite with `internal/config/config_test.go` (race-clean).
- **2026-07-10 10:25pm — P0.3 + P1.1 scaffold landed (Haiku).** Filter de-aliasing applied to all four filter functions. Test scaffold complete: 22 tests for pure functions (no edge cases). Formatting committed separately. All tests pass under `-race`.
- **2026-07-10 10:45pm — P0.2 + P1.1 edge cases landed (Sonnet).** MPRIS race fixed: `poll` now takes `stopCh` as parameter (captured under lock in `exec`); `Close` also captures `stopCh` under lock. P1.1 edge cases added: `vs*` boundary invariants (n=0, clamp, circular wrap, viewport) and SponsorBlock round-trip/monotonicity. 34+ tests total, all passing under `-race`. Wave 1 complete.
- **2026-07-10 11:00pm — P1.2 landed (Sonnet).** `Store` interface (52 methods) declared in `internal/ui/store.go`. `Model.db`, `mustWatchedIDs`, `mustVideoPositions` widened to `Store`. `NewModel` signature unchanged. Compile-time assertions for both `*db.DB` and `fakeStore` in `store_test.go`. Next: Wave 3 — P2.1 shared video-action helper.
