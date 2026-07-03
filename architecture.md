# Architecture

## Overview

yt-tui is a terminal UI application built on the [Bubble Tea](https://github.com/charmbracelet/bubbletea) Elm-architecture framework. The app is structured as a set of independent packages wired together at startup in `main.go`.

``` text
main.go
├── internal/config      — TOML config load/save, keybinding defaults
├── internal/db          — SQLite persistence (feeds, history, playlists, channels)
├── internal/downloader  — yt-dlp download queue, progress events
├── internal/player      — video playback, resume-position tracking via MPRIS
├── internal/theme       — color palette loaded from theme.toml
├── internal/youtube     — yt-dlp wrappers, YouTube API calls, Bubble Tea message types
└── internal/ui          — Bubble Tea model, update logic, rendering
    ├── model.go         — Model struct, initialization, sort helpers
    ├── update.go        — Update function, all key/message handlers
    ├── view.go          — View function, all rendering
    ├── keys.go          — Key binding map
    └── styles.go        — Lipgloss style definitions
```

---

## Package responsibilities

### `config`

Loads and saves `~/.config/yt-tui/config.toml` via TOML. Provides defaults for every field so existing configs missing new keys still work. Keybindings are stored as strings and support comma-separated values (`play = "p,enter"`) for multiple keys per action — parsed in `ui/keys.go`.

Config is loaded once at startup and treated as immutable during the session, except for `AddBlacklistedChannel` which appends and saves synchronously when a channel is auto-blacklisted.

### `db`

A thin wrapper around a single SQLite file (`yt-tui.db` in the config dir) using `modernc.org/sqlite` (pure Go, no CGo). All schema migrations are idempotent `CREATE TABLE IF NOT EXISTS` + `ALTER TABLE ADD COLUMN` statements run at open time.

Key tables and their purpose:

| Table | Purpose |
| --- | --- |
| `videos` | Canonical video metadata (id, title, channel, duration, views, upload date, url) |
| `feed_cache` | Serialised feed snapshots (recommended, subscriptions) for instant startup |
| `local_videos` | Downloaded files with path, status, last play position |
| `history` | Every user action (download, play, search, delete) keyed by video_id |
| `subscribed_channels` | Full channel list (id, name, url, subscribers) persisted after fetch |
| `channel_latest` | Latest known video per channel for channel-list display |
| `channel_videos` | All fetched videos per channel |
| `yt_playlists` | Cached YouTube playlist list (id, title) |
| `playlists` / `playlist_videos` | Local playlists |
| `watch_later` | Local Watch Later entries (fallback when no YouTube connection) |
| `hidden_rec_videos` / `channel_removals` | Block lists for recommended feed filtering |

All writes from goroutines (background fetches) go through `go func()` closures — the DB is used directly from goroutines since SQLite with WAL handles concurrent reads/writes safely.

### `downloader`

Manages a bounded concurrent download queue. Each download is a goroutine running `yt-dlp` as a subprocess. The semaphore (`chan struct{}` of size `MaxDownloads`) limits concurrency.

Progress is scraped from yt-dlp's `--newline` stdout: a regex matches percentage/speed/ETA lines, another matches the final output path from merger/destination lines. Events are sent on an internal `eventCh` channel and surfaced to Bubble Tea as `EventMsg`.

Output files are named `%(channel)s - %(title)s.%(ext)s` via yt-dlp's `-o` template. A `sanitizeFilename` helper replaces filesystem-invalid characters in the fallback path (used when yt-dlp output can't be parsed).

On delete (`x`), the downloader cancels the subprocess via `context.CancelFunc`, removes the item from its map, and the UI also deletes the file and DB record.

### `player`

Two backends selectable via config:

- **`mpris`** — launches the player, then polls D-Bus for the MPRIS2 `Position` property while the file is playing. On stop/close it writes `last_position_ms` to `local_videos`. Next time the same file is opened, playback resumes from that offset.
- **`simple`** — spawns the player process with no position tracking.

Both backends implement a `Backend` interface with a single `Play(filePath, startAt)` method.

### `youtube`

Two sub-responsibilities:

**yt-dlp wrappers** (`fetcher.go`) — every network operation runs `yt-dlp` as a subprocess and parses its JSON output. Functions return Bubble Tea `tea.Cmd` values (closures that return a message) so they integrate cleanly with the update loop. Rate-limit detection and retry logic is built into the core fetch helpers.

**YouTube API calls** (`ytapi.go`) — subscribe, unsubscribe, create playlist, add to Watch Later. These use the internal YouTube API (extracted from browser cookies via yt-dlp) rather than the official API, which requires no API key.

All Bubble Tea message types are defined alongside the fetch functions. The `Background bool` field on `ChannelListMsg` and `YTPlaylistsMsg` lets the update handler distinguish a silent background refresh from a user-triggered foreground fetch, so it can suppress error toasts and loading spinners.

### `ui`

The Bubble Tea model. Follows the Elm architecture strictly: `Model` → `Update` → `View`, no shared mutable state between them.

#### `model.go`

The `Model` struct holds all application state. Notable design choices:

- **Per-tab sort state** (`recSort`, `subSort`, `searchSort`, `localSort`) rather than a single sort mode, so switching tabs preserves each tab's sort independently.
- **Chord system** (`pendingChord string`, `chordBuffer string`, `gPending bool`, `numPrefix string`) captures multi-key sequences without a separate input mode. `pendingChord` holds the trigger key currently awaiting completion; `chordBuffer` accumulates subsequent keys for prefix-matching. The status bar renders context-aware hints when a chord is in progress.
- **`ContextID` enum** maps the current tab + sub-mode + pane to a context used by the sort matrix and `DrillDown` dispatch.
- **`subChLatest map[string]youtube.Video`** — a channel-ID-keyed map of the latest known video per channel, loaded from DB at startup and updated by background fetches. The channel list renders from this map without needing to open each channel.
- **`playAfterDownload map[string]bool`** — video IDs queued for auto-play on download completion.

Sort functions (`sortVideos`, `sortLocalVideos`, `sortedChannels`) are pure functions over slices — they never mutate model state directly; callers assign the result back.

##### Chord system

Two-level chord architecture. All trigger keys and sub-keys are configurable via TOML.

**Flow:**

1. User presses a chord trigger (e.g. `t`) → `pendingChord` is set, `chordBuffer` cleared, status bar switches to chord hint.
2. Subsequent keypresses accumulate into `chordBuffer`.
3. On each keypress, the resolver checks `chordBuffer` against configured keys:
   - **Exact match** → fire action, clear pending state.
   - **Prefix of a configured key** → remain pending (multi-char support, e.g. `su` for subscriptions).
   - **No match and no prefix** → silently cancel.

**`t` chord (tab switch)** — sub-keys from `[keybindings.tab_keys]`:

| Config key | Default | Tab |
| --- | --- | --- |
| `recommended` | `r` | Recommended |
| `subscriptions` | `s` | Subscriptions |
| `playlists` | `p` | Playlists |
| `search` | `S` | Search |
| `downloading` | `d` | Downloading |
| `local` | `l` | Local |
| `history` | `h` | History |

**`s` chord (sort)** — sub-keys from `[keybindings.sort_keys]`, filtered by current context:

| Config key | Default | Action | Recommended | Subscriptions | Channel list | Search video | Local |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `date` | `d` | sort by date | ✓ | ✓ | ✓ (latest video) | ✓ | ✓ |
| `views` | `v` | sort by views | ✓ | ✓ | ✓ (latest video) | ✓ | ✓ |
| `name` | `n` | sort by name | ✓ | ✓ | ✓ (latest video) | ✓ | ✓ |
| `channel` | `c` | sort by channel | ✓ | ✓ | ✓ (channel name) | ✓ | ✓ |
| `duration` | `D` | sort by duration | ✓ | ✓ | ✓ (latest video) | ✓ | ✓ |
| `subscribers` | `s` | sort by subscribers | | | ✓ | | |

The sort chord trigger is only intercepted when the current context supports at least one sort action (e.g. History and Downloading tabs do not intercept `s`).

**`DrillDown` key** (default `enter`) — context-sensitive action:

| Context | Action |
| --- | --- |
| Channel list (subs channel pane) | Open channel → video pane |
| Playlist list | Open playlist → video pane |
| Video list (rec, subs-all, channel drill-down, playlist vids) | Play/queue video |
| Search channel row | Open channel drill-down |
| Search video row | Play/queue video |
| History video entry | Show detail view |
| History search entry | Jump to Search tab with query pre-filled |

**Hint modes** (`hint_mode` config, default `full`):

| Mode | Status bar shows |
| --- | --- |
| `full` | All context-relevant bindings; chords shown as trigger only (`t: tab  s: sort`) |
| `minimal` | Only: `j/k: move  t: tab  p: play` |
| `none` | Empty left side (only `?: help  q: quit` on right) |

When a chord is pending, the hint always expands to show completions regardless of mode.

#### `update.go`

The `Update` function dispatches on message type. Key structural decisions:

**Chord system** — `pendingChord` and `chordBuffer` replace the old `tPending`/`sPending` booleans. Any configurable trigger key sets `pendingChord`; subsequent keys accumulate in `chordBuffer`. `resolveChord` is fully generic: it walks the `chordDefs()` registry (built from config on each keypress), filters actions by `currentContext()`, and prefix-matches `chordBuffer` against the valid entries — firing on exact match, staying pending on a valid prefix, cancelling otherwise. Adding a new chord requires only a new entry in `chordDefs()`; no new dispatch function is needed. The sort chord is only intercepted when `contextSupportsSorting()` is true, so tabs without sort actions (History, Downloading) do not capture the sort trigger key.

**Background vs foreground fetches** — for channels, playlists, and subscriptions, the tab activation logic checks whether cached data exists. If it does, a `Background: true` variant is fired (no spinner, no error toasts, UI only updates if the result differs). If not, the foreground variant fires (spinner shown, errors surfaced).

**Change detection** — `channelSetChanged` and `ytPlaylistSetChanged` compare ID sets before updating `m.subChannels` / `m.ytPlaylists`. This prevents cursor resets and unnecessary re-renders when a background refresh returns the same data.

**`currentVideo()`** — a single function that returns the focused video regardless of active tab and sub-mode (recommended, subscriptions all-videos, subscriptions channel drill-down, search results, search channel drill-down, playlists). This lets action handlers (`d`, `p`, `c`, `S`, `B`, etc.) be written once and work everywhere.

**`currentChannelInfo()`** — similarly returns the channel ID/name for the focused item, with a special case for the subscriptions channels pane (where the item is a `Channel`, not a `Video`) and the history tab (where channel info comes from `HistoryEntry.ChannelID`).

**`hideChannel()`** — shared helper called from recommended, downloading, and history tab handlers. Writes to DB, filters the in-memory recommended feed, triggers auto-blacklist check.

#### `view.go`

Pure rendering — reads model state, produces a string. Never modifies state.

**Status bar** — two zones: left (context help / chord hint / status message), right (`?: help  q: quit`). Context help reads from `m.keys` and `m.cfg` rather than hardcoded strings, so remapped keys appear correctly.

**`chordHint()`** — generic: iterates `chordDefs()`, filters by `currentContext()`, formats as `trigger → key: label  key: label …`. No per-chord special cases.

**`contextHelpRaw()`** — dispatches on `cfg.HintMode`:

- `full` → `fullHintRaw()`: per-tab hint with action keys. Chord triggers are included as `trigger: name` pairs generated from `chordDefs()`, filtered to the current context, so unavailable chords are automatically omitted.
- `minimal` → `minimalHintRaw()`: `j/k: move  t: tab  p: play`.
- `none` → empty string.

Pending-chord hints always override the normal hint regardless of mode.

**`contextHelp()`** — conditionally omits YouTube-requiring actions (`S: sub`, `w: later`, `a: playlist`) when `m.ytClient == nil`, so the hint only shows what actually works.

**`renderVideoList()`** — shared video table renderer used by recommended, subscriptions (all-videos), and subscriptions channel drill-down. Columns: row number, status indicator, title, channel, duration, views, upload date. Column widths are fixed constants; title gets the remainder.

**History rendering** — the main history list shows one row per video (most recent event wins, using `id DESC` as tiebreaker for same-second events). Search events appear as their own rows with the query text instead of video metadata. Pressing Enter on a search row navigates to the Search tab with the query pre-filled.

#### `keys.go`

Builds a `keyMap` from config keybindings. The helper `b()` splits on commas before calling `key.WithKeys(...)`, so `play = "p,enter"` binds both keys to the same action. Notable bindings:

- **`DrillDown`** — replaces the old hardcoded `Enter`; context-sensitive (plays video in video contexts, opens in drill-down contexts).
- **`Left`** — built from the configurable `back` keys (`h,backspace` default) plus `←` arrow always appended; covers both keyboard back-navigation and arrow key.
- **`Filter`** — configurable key to activate the inline filter input (default `/`).
- **`GotoBottom`** — configurable key for jump-to-bottom / jump-to-Nth-row (default `G`).

Chord trigger keys (`tab_chord`, `sort_chord`, `goto_prefix`) are not `key.Binding` values — they are matched by raw string comparison against `msg.String()` in `handleKey` so that the pending-state machinery fires before any binding lookup.

#### `styles.go`

Lipgloss style definitions initialised from the theme. `InitStyles(theme)` is called at startup after the theme is loaded, replacing the default color constants. All styles are package-level variables so they're accessible from view.go without threading them through the model.

---

## Data flow

### Startup

``` text
main.go
  config.Load()           → cfg
  db.New()                → database (runs migrations)
  downloader.New()        → dl
  theme.Load() → ui.InitStyles()
  ui.NewModel(cfg, database, dl)
    database.GetFeedCache("recommended")   → recVideos  (immediate)
    database.GetFeedCache("subscriptions") → subVideos  (immediate)
    database.GetSubscribedChannels()       → subChannels (immediate)
    database.GetChannelLatest()            → subChLatest (immediate)
    database.GetYTPlaylists()              → ytPlaylists (immediate)
    database.LocalVideos()                 → localVideos (immediate)
  tea.NewProgram(model).Run()
    → Init() fires background fetches for rec/subs/channels
```

### Key press

``` text
User presses key
  → tea.KeyMsg delivered to Update()
  → handleKey() dispatches:
      chord pending?  → handleSortChord / handleTabChord
      search focused? → handleSearchInput
      overlay active? → overlay handler
      else            → tab-specific handler (updateRecommended, updateSubscriptions, …)
  → handler returns (Model, tea.Cmd)
  → tea.Cmd (if any) fires as goroutine → returns message → Update() again
  → View() renders updated model
```

### Background fetch completing

``` text
FetchChannelLatest goroutine completes
  → returns ChannelVideosMsg{Source: "ch-background"}
  → Update() handler:
      if newer video found: m.subChLatest[id] = newest; persist to DB
      else: no-op
  → View() re-renders channel list with updated latest-video data
```

---

## Threading model

Bubble Tea runs the `Update` / `View` loop on a single goroutine. All `tea.Cmd` functions run on separate goroutines managed by the framework. The only direct goroutine launches in the codebase are fire-and-forget DB writes (`go func() { _ = m.db.Save... }()`), which is safe because SQLite WAL mode allows concurrent writes and these never read back into the model.

The downloader maintains its own goroutine per active download, plus a semaphore goroutine. It communicates with the UI exclusively through `EventMsg` values sent via the Bubble Tea event channel.

The MPRIS player backend runs a polling goroutine that writes `last_position_ms` to the DB when playback ends — again, a fire-and-forget write that never feeds back into the model.
