# yt-tui — Architectural & Code-Level Audit

_Date: 2026-07-10 · Scope: full repository (~10.8k LOC, 0 tests) · Reviewer role: Principal Architect_

## Stack & shape

Go 1.26 · Bubble Tea (Elm-architecture TUI) · lipgloss · modernc SQLite · `yt-dlp` + D-Bus/MPRIS subprocess integration.

Packages: `config`, `db`, `downloader`, `youtube`, `player`, `ui`, `theme`, `debug`, `image`.

**Verdict.** Feature-rich and reasonably layered at the *package* boundary. The `player` package is genuinely well-designed. The `ui` package has collapsed into a **God module**: `Model` (~200 fields, `internal/ui/model.go:117-315`) plus `update.go` (3,558 lines) concentrate state, IO orchestration, business logic, and view helpers in one type. This is the dominant technical debt and the highest-ROI target. **Zero tests** across 10.8k LOC is the top reliability risk.

---

## 1. Architectural & Design Principles

### SOLID

- **SRP — severely violated (top priority).** `Model` spans nine tabs, six overlays, chord state, sort state, playback state, and pre-rendered thumbnail caches. `update.go` mixes event routing, key dispatch, per-tab controllers, business rules, SponsorBlock math, and side-effecting IO.
- **OCP — violated by tab design; strong counter-example exists.** Adding a tab edits ~10 parallel `switch m.activeTab` blocks: `currentVideo` (`model.go:770`), `jumpToLine` (`839`), `jumpToLast` (`878`), `currentContext` (`1027`), `applySortAction` (`update.go:1074`), `onTabActivated`, `refresh`, `forceRefresh`, `handleKey`, `localFilteredVideos`. **Counter-example done right:** the data-driven chord registry (`chordDefs`, `model.go:1097-1310`) extends without touching the dispatcher — this is the template to propagate.
- **DIP — good in `player`, poor in `ui`.** `player.Backend`/`player.Driver` are clean, injectable abstractions with a graceful MPRIS→simple fallback (`player.go:50-65`). `Model` instead depends on concrete `*db.DB`, `*downloader.Downloader`, and package-level `youtube.*` functions — none injectable (root cause of untestability).
- **ISP / LSP — fine.** `Backend` is small; the three drivers are substitutable.

### DRY — high-value duplications

- **Video-action key block** (`Play/PlayAudio/Download/DownloadAudio/AddList/CopyURL`) copy-pasted across 6 controllers (~15 lines each): `updateRecommended`, `updateSubAll`, `updateSubChannelsTags`, `updateSubChVideoPane`, `updateSearch`, `updatePlaylists`.
- **Video-detail-open logic** duplicated verbatim (~22 lines) in `handleKey` (`update.go:882-908`) and `updateSubChannelsTags` (`1610-1634`).
- **Overlay nav boilerplate** re-implemented in `handleLinkOverlay`, `handleChapterOverlay`, `handleAddOverlay`.
- `sortVideos` vs `sortLocalVideos` (`model.go:975`/`1001`) — identical but for element type (use generics).
- `config.fillDefaults` (`config.go:155-217`) — 60 lines of manual `if x=="" {x=d.x}`.
- Three parallel tab-name tables: `tabNames` (`model.go:37`), `tabDebugNames` (`update.go:717`), `DefaultTabs`/`tabIDByName`.

### Clean Code
Naming and comments are good. Problems are **size and cyclomatic complexity** (`Update`, `handleKey`, `handleFetchResult`, the `ChannelVideosMsg` case) and **primitive obsession** (tabs/panes/sort/edit modes as bare `int`).

---

## 2. Code Quality & Reliability

### Bugs & races

1. **Config corruption (High).** `save` (`config.go:342`) truncates via `os.Create` then streams TOML. `filterBlacklisted` (`update.go:2983-2985`) and `hideChannel` (`3293-3294`) fire `go cfg.Save()` while mutating `cfg.BlacklistedChannels` — concurrent non-atomic writes + slice data race. A crash mid-write empties the config.
2. **MPRIS `poll` data race (Med-High).** `poll` reads field `b.stopCh` in its `select` (`mpris.go:77`) with no lock while `exec` reassigns it under `b.mu` (`mpris.go:53`). Fails `-race`; rapid `Launch` can strand pollers.
3. **In-place filter aliasing (Med, latent).** `filterByAge/Downloaded/Hidden/Blacklisted` use `out := videos[:0]` (`update.go:2925,2957,2968,2980`), appending into the caller's backing array. Masked today only because callers pass a fresh `mergeVideos` slice.
4. **Opaque download failures (Med).** `run` discards stderr (`downloader.go:149-153`); users see only `yt-dlp: exit status 1`.
5. **Hardcoded keys in input modes (Med).** `handleSearchInput` et al. hardcode `esc/enter/up/f2–f8` (`update.go:2105-2125`) instead of `cfg.Keybindings`.

### Error handling
Pervasive `_ =` swallowing of DB errors (`AddHistory`, `SaveVideoPosition`, `SetChannelAlias`, all `NewModel` loads) → silent data loss. Raw `err.Error()` shown in status bar (acceptable for a local single-user TUI).

### Testability
Zero tests. Blockers: concrete deps in `Model`, fat controller. **Opportunity:** many pure functions are trivially testable now — `vsMove/vsPage/vsJump`, `filterByAge`, `mergeVideos`, `extractLinks`, `sanitizeFilename`, `cmdCompletionsFor`, and the SponsorBlock conversions (round-trip-checkable).

---

## 3. Maintainability & Scalability

- **God object / God file** (above).
- **Inappropriate intimacy:** `ui` encodes `db` cache-key formats (`"local:%d"`) and `"name:"+lower` sentinels inline; YouTube URLs built ad hoc in many places.
- **Primitive obsession:** bare `int` modes invite mix-ups; named types would let the compiler help.

### Performance (hot-path allocations; not fatal at expected sizes)
- `chordDefs()` rebuilds the whole registry (slices + closures) on **every keypress and render** (`model.go:1133`).
- `currentVideo()` in tags view calls `tagVideos()` (`model.go:590`) → alloc + `O(n log n)` re-sort per action key; `sortedChannels()` copies+sorts per render.
- `launchVideo`/`handleDownloadEvent` reload the entire local-video table + rebuild the ID map after every play/delete.

---

## 4. What to keep (working well)

- `player` Strategy/Backend abstraction + D-Bus fallback.
- Data-driven chord registry (the extensibility template).
- WAL + `SetMaxOpenConns(1)` DB serialization (`db.go:82-84`); fingerprint-based cache invalidation.
- Subprocess args passed as argv (no shell) — `yt-dlp`/`xdg-open` inputs are **not** shell-injectable.

---

See `docs/REFACTOR_PLAN.md` for the prioritized, model-tiered execution plan.
