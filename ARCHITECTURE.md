# Architecture

## Overview

yt-tui is a terminal UI for browsing, searching, and downloading YouTube videos,
built on the [Bubble Tea](https://github.com/charmbracelet/bubbletea)
Elm-architecture framework (v2 / `charm.land`). yt-dlp is the network engine and
SQLite the store.

The codebase is organised around a single **`api.Backend` interface** that
sits between the TUI and everything that touches the network, disk, or
subprocesses. This abstraction gives the app two deployment topologies from one
code path:

- **Single-binary** (`yt-tui`) — the TUI talks to an in-process backend
  (`InProc`) that owns the DB, downloader, and yt-dlp directly.
- **Client/daemon** (`yt-tui` + `yt-tuid`) — a headless daemon (`yt-tuid`)
  hosts the backend over [Connect](https://connectrpc.com) RPC; the TUI runs a
  thin RPC client (`Remote`) and holds no DB or subprocess state of its own.

Both binaries build the *same* `InProc` backend at their core; the only
difference is whether the TUI reaches it directly or across the wire.

### Two binaries

``` text
cmd/yt-tui   — the TUI client. buildBackend() chooses InProc or Remote.
cmd/yt-tuid  — the headless daemon. Wraps InProc in Connect handlers + HTTP.
```

`cmd/yt-tui/main.go:buildBackend()` returns a `Remote` when a daemon address is
configured (`--connect` flag or `cfg.DaemonAddr`), otherwise a local `InProc`.
`cmd/yt-tuid/main.go` builds an `InProc` and mounts it behind an HTTP mux
(`buildMux`) with bearer auth, a `/healthz` probe, and a `/media/{id}` file
server.

---

## Repository layout

``` text
api/proto/backend/v1/       — protobuf service + message definitions (source of truth for RPC)
cmd/
├── yt-tui/                 — TUI client binary (main.go, buildBackend)
└── yt-tuid/               — daemon binary (main.go, buildMux)
internal/
├── api/                    — the Backend interface + its three implementations
│   ├── client.go           — 10 role interfaces + composed Backend interface
│   ├── inproc*.go          — InProc: direct in-process backend (single-binary + daemon core)
│   ├── remote*.go          — Remote: Connect RPC client (client/daemon mode)
│   ├── apitest/nop.go      — NopBackend: per-role test doubles
│   └── backend/v1/         — generated Connect code (backendv1connect) + protoconv
├── backend/                — daemon-side implementation (only used by yt-tuid)
│   ├── transport/          — Connect handlers, one per service; Mount()
│   ├── service/            — business logic (FeedService, ChannelService, PortabilityService)
│   ├── media/              — /media/{id} file server + signed tickets
│   ├── httpauth/           — bearer-token middleware
│   ├── enrich/             — background enrichment pass (dates, thumbnails, transcripts, SB)
│   ├── thumbs/             — bounded thumbnail cache (on-disk .jpg store)
│   ├── transcripts/        — transcript store (.srt cache + canonical .md notes)
│   └── profiles/           — daemon-stored named config profiles (opaque JSON blobs)
├── domain/                 — pure domain types + logic (no DB/UI/subprocess deps)
│   ├── channels/           — ChannelSet (subscription state + membership index)
│   ├── feed/               — Feed type, unified sort, pure filter chain
│   ├── library/            — downloaded-video collection + by-ID index
│   ├── media/              — SponsorBlock/chapter/link math (pure functions)
│   └── portability/        — versioned export/import Bundle
├── db/                     — SQLite persistence (modernc pure-Go driver)
├── config/                 — TOML config, keybindings, panels, columns, profile paths
├── downloader/             — yt-dlp download queue + event broadcaster
├── youtube/                — yt-dlp wrappers + internal YouTube API (cookie auth)
├── device/player/          — playback backends (MPRIS position tracking / simple)
├── tui/                    — the Bubble Tea front end (see "TUI layer" below)
├── theme/ text/ procexec/ sys/ debug/  — supporting utilities
```

---

## The Backend interface

`internal/api/client.go` defines **ten role interfaces**, each scoped to one
concern, composed into a single `Backend`:

| Role interface | Responsibility |
| --- | --- |
| `FeedBackend` | Recommended-feed fetch + cache, hide/watched tracking |
| `ChannelBackend` | Subscriptions, search, channel metadata (state/alias/tags), block/unblock |
| `VideoBackend` | Video details + cache, positions, source resolution, thumbnails, transcripts, complete-delete |
| `LibraryBackend` | Downloaded-file inventory |
| `PlaylistBackend` | Local playlists + YouTube playlists / Watch Later |
| `HistoryBackend` | Play/search history + activity log |
| `DownloadBackend` | Download queue management + server-streaming event feed |
| `PortabilityBackend` | Export / import-preview / import-apply of a data bundle |
| `ProfileBackend` | Daemon-stored named config profiles |
| `StatusBackend` | Environment availability probe (yt-dlp, cookies) |

Consumers depend on the *narrowest* role they need rather than the full
composed interface — TUI tabs, transport handlers, and tests each take a small
role interface. Two conventions run throughout:

- **Missing-vs-error semantics.** Lookups like `HasLocalVideo`,
  `VideoPosition`, `GetProfile`, and `GetVideoDetailsCache` return
  `(value, found bool, err error)`. `found=false, err=nil` means "legitimately
  absent"; a non-nil `err` means the lookup itself failed and must not be
  treated as absent.
- **Server-side compound operations.** `DeleteVideoCompletely` removes the
  file, DB row, position, and history in one call so remote mode gets identical
  semantics to in-process — no multi-round-trip races.

### Three implementations

**`InProc`** (`internal/api/inproc*.go`) — the real backend, one adapter file
per role. Holds the `*db.DB`, `*youtube.Client`, `*downloader.Downloader`,
`*config.Config`, and three optional on-disk stores (`thumbs`, `transcripts`,
`profiles`). It also owns three service objects (`FeedService`,
`ChannelService`, `PortabilityService`) that encapsulate the fetch→filter→persist
business logic. Optional stores degrade gracefully: if a store's directory can't
be created the field is left nil and its methods return empty results or
`domain.ErrProfilesUnavailable`. `NewInProc` is the composition root shared by
both binaries. `StartBackgroundEnrichment` launches the enrichment goroutine
(see below).

**`Remote`** (`internal/api/remote*.go`) — a Connect RPC client, one adapter
file per role. Holds one generated `backendv1connect.*ServiceClient` per
service and translates each call to/from proto via `protoconv`. Two notable
details:

- It keeps a *second* DownloadService client with **no HTTP timeout** for the
  `Events()` server-stream, because `http.Client.Timeout` bounds the whole
  request including the streaming body and would otherwise kill the feed.
- `CheckAvailability` calls the daemon's HealthService and prefixes every issue
  with `daemon:` so the user knows the fault is server-side, not local.

**`NopBackend`** (`internal/api/apitest/nop.go`) — per-role zero-value doubles
composed into one struct. Tests embed it and override only the methods under
test, which the role-interface split makes clean.

---

## Transport layer (client/daemon mode)

The wire contract lives in `api/proto/backend/v1/*.proto`. `buf`-generated code
lands in `internal/api/backend/v1/backendv1connect` (clients + handler
interfaces) and `internal/api/backend/v1/protoconv` (domain ↔ proto
conversion). There are **ten services**, mirroring the ten role interfaces:

`FeedService`, `ChannelService`, `VideoService`, `LibraryService`,
`PlaylistService`, `HistoryService`, `DownloadService`, `PortabilityService`,
`ProfileService`, `HealthService`.

Notable RPC design choices:

- **`DownloadService.Events`** is server-streaming (`stream EventsResponse`) —
  it stays open and pushes progress/complete/error events.
- **`PortabilityService` and `ProfileService`** carry their payloads as opaque
  `bytes` (JSON). The bundle/profile schema stays single-sourced in Go and the
  daemon never parses it, so schema evolution doesn't touch the proto.
- **`HealthService.CheckAvailability`** (Phase 20) runs the environment probe on
  the daemon host so a `--connect` client sees server-side faults.

### Daemon wiring (`cmd/yt-tuid`)

`internal/backend/transport/Mount(mux, backend, token)` registers one Connect
handler per service; each handler is a thin shim over a role interface that
converts proto ⇄ domain and wraps domain errors as Connect status codes. On top
of the RPC routes, `buildMux` adds:

- `/healthz` — unauthenticated liveness (for load balancers/monitoring).
- `/media/{id}` — a range-capable file server (`internal/backend/media`) for
  downloaded files, so a remote player can stream from the daemon. It accepts
  either a bearer header *or* a short-lived **signed ticket** query param, so
  external players (mpv/vlc) that can't set headers can still fetch.
- Everything else is guarded by `internal/backend/httpauth.Bearer`
  (constant-time token comparison; no-op when the token is empty).

### Daemon-side stores & services (`internal/backend`)

- **`service/`** — `FeedService` runs the recommend pipeline (fetch → filter by
  age/duration/views, drop downloaded/hidden/blocked/subscribed → cache).
  `ChannelService` merges YouTube fetches with the DB cache and stamps channel
  activity. `PortabilityService` builds/imports the bundle.
- **`thumbs/`** — bounded on-disk `.jpg` cache (one per video ID), sized to
  "newest-N per subscribed channel ∪ recommended" and swept by the enricher.
- **`transcripts/`** — an `.srt` cache (bounded, swept) plus canonical `<id>.md`
  notes (unbounded) with chapters/timestamps/inline images.
- **`profiles/`** — one JSON file per named profile; opaque bytes, global to the
  daemon (matching the single-bearer-token model).
- **`enrich/`** — the background pass that corrects approximate upload dates,
  caches eligible thumbnails, and persists transcript captions + SponsorBlock
  segments. Paced by `EnrichmentDelaySeconds`. Runs inside `InProc`, so it
  executes in single-binary mode and on the daemon alike.

---

## Domain layer (`internal/domain`)

Pure types and logic with no DB/UI/subprocess dependencies — the shared
vocabulary across every layer.

**Core types** (package `domain`): `Video`, `VideoDetails`, `VideoRef`
(ID+URL, used to break a db↔backend import cycle), `LocalVideo`
(+`VideoStatus`), `Channel` (+`SubscriptionState`: `SubNone`/`SubYT`/`SubLocal`,
with the invariant *Blocked ⇒ SubNone*), `HistoryEntry`, `ActivityEntry`,
`Playlist`/`YTPlaylist`/`WatchLaterEntry`, and cache types (`Link`, `Chapter`,
`SBSegment`, `CachedDetails` — whose pointer fields distinguish "never fetched"
from "fetched, empty").

**Subpackages:**

- **`channels`** — `ChannelSet`: the subscribed-channel list plus a membership
  index (by ID and by lowercased name) that survives YouTube re-fetches while
  preserving local annotations (alias/tags) via `SyncFromYT`.
- **`feed`** — the `Feed` type owns a video slice + fetch lifecycle
  (loading/refreshing/paging). `sort.go` is the **unified sort** used by every
  tab: a projection-based `sortByMode[T]` with modes date/views/name/channel/
  duration/subscribers/tags/size, including `SortChannels` (sort channels by
  their latest video). `filter.go` holds the pure recommend filter chain and the
  `Blocklist` (ID exact-match + name-fallback, emitting enrichments that upgrade
  name-only blocks to ID-keyed ones).
- **`library`** — `Library`: downloaded-video collection with a by-ID index kept
  consistent through a single `Set()` reload path.
- **`media`** — pure SponsorBlock/chapter/link math: `ProcessChapters`,
  original↔adjusted timeline conversion, `ExtractLinks`.
- **`portability`** — the versioned `Bundle` (schema v1): channels, blocked
  names, playlists, watch-later, YT playlists, videos, and opt-in personal watch
  data (history/positions), plus an opaque client-config blob.

---

## Data layer (`internal/db`)

A thin wrapper over a single SQLite file (`DataDir/yt-tui.db`) using
`modernc.org/sqlite` (pure Go, no CGo). The connection is opened with
`SetMaxOpenConns(1)` so all access serialises through one connection, plus
`PRAGMA journal_mode=WAL`, `busy_timeout=5000`, and `foreign_keys=ON`.

The schema is managed by **versioned migrations**. `internal/db/migrations/`
holds `NNNN_description.sql` files whose numeric prefix is the target schema
version (`0001_baseline.sql` is the initial schema). At startup `migrate()`
(`internal/db/migrate.go`) reads the database's SQLite `user_version`, then
applies every migration whose version exceeds it — each in its own transaction
that both runs the DDL and stamps `user_version` to that migration's number.
Apply-and-stamp in one transaction means a crash mid-migration rolls back
cleanly and re-runs on the next start, never leaving the schema half-applied. A
fresh database (`user_version` 0) runs the whole chain; an up-to-date one is a
no-op. The loader enforces a gap-free sequence from 1, so `user_version` equals
the number of migrations applied and a mis-numbered file fails loudly at startup.

Because each migration runs exactly once against a known state, migration DDL is
plain `CREATE`/`ALTER` (no `IF NOT EXISTS`): a double-apply is a bug and should
fail rather than be masked. Evolving the schema is now **additive** — add the
next `NNNN_*.sql` file rather than recreating the database. (`DB.SchemaVersion`
reads the stamp back; the `meta` table separately holds a `video_details_cache`
column fingerprint that auto-clears that cache when its columns change.) Startup
maintenance also reconciles downloads against files on disk and prunes stale
rows.

| Table | Purpose |
| --- | --- |
| `videos` | Canonical video metadata; parent table — all paths `UpsertVideo` here first |
| `local_videos` | Downloaded files (path, type, status, `last_played`, `file_size`); FK → `videos` |
| `video_positions` | Last playback position per video; FK → `videos` ON DELETE CASCADE |
| `video_details_cache` | Description, thumbnail URL, subscribers, links/chapters/SB segments (JSON) |
| `history` | Play/stream/search/delete events; nullable `video_id` FK ON DELETE CASCADE |
| `activity_log` | User actions (subscribe, create/add-to playlist) |
| `feed_cache` | Serialised feed snapshots for instant startup |
| `hidden_rec_videos` | Hide-list for recommended filtering |
| `subscribed_channels` | Unified channel table: state enum, blocked flag, alias, tags, refresh/activity timestamps |
| `channel_videos` | All fetched videos per channel; latest-per-channel derived at query time |
| `collections` | Unified local playlists + cached YouTube playlists (`kind` discriminates; `local:N`/YT ids) |
| `collection_videos` | Membership junction for both collection kinds; order is insertion order (rowid) |
| `meta` | Key/value flags (e.g. `video_details_cache` column fingerprint) |

Cascading FKs mean deleting a `videos` row automatically clears its `history`
and `video_positions`. `enrich.go` holds the eligibility CTEs (newest-N per
subscribed channel ∪ recommended) that bound the thumbnail/transcript caches.

---

## Config layer (`internal/config`)

XDG-compliant TOML config, split into two concern groups on one `Config`:

- **`DaemonConfig`** — settings the backend needs: token/TLS, download dir,
  cookie source, `YouTubeEnabled`, `MaxDownloads`, SponsorBlock, recommend/
  channel-refresh tuning, enrichment pacing, artifact dirs, subtitles.
- **`ClientConfig`** — TUI-only settings: `DaemonAddr`/`DaemonToken`/`TLSCACert`
  and `LoadProfile` (remote), player + backend, theme, `HintMode`,
  `DurationFormat`, feed/channels/tags default modes, stale-channel hiding, and
  the two data-driven UI structures below.

**Panels & columns.** The UI is not a fixed tab set. `Panels []Panel` (each with
a stable `Name`, a `Type`, and optional `Mode`/`Sort`) defines which tabs exist
and in what order; `Keybindings.TabKeys` maps hotkeys to panel names; and
`Columns map[string][]string` selects and orders columns per panel (empty = the
panel's full default set). This is what lets a user run, say, two Feed panels
with different modes and columns.

**Keybindings** are stored as strings, comma-separated for multiple keys per
action (`play = "p,enter"`), and include nested chord sub-key maps
(`SortKeys`, `SubscribeKeys`, `PlaylistKeys`). Path helpers resolve
`ThumbnailsPath()`, `TranscriptsPath()`, `TranscriptMarkdownPath()`, and
`ProfilesPath()`. Non-fatal load problems surface as `Issues`, shown in a
startup overlay.

---

## Downloader, YouTube, Player

### `downloader`

An in-memory queue of `yt-dlp` subprocesses bounded by a semaphore
(`MaxDownloads`). Each download runs in its own goroutine; progress is scraped
from `--newline` stdout via regex (percent/speed/ETA, plus the final
merger/destination path). Completed items persist (`UpsertVideo` +
`AddLocalVideo` with file size + `AddHistory`) and auto-evict after a delay;
failures stay visible.

Events fan out through a **broadcaster**: a single reader drains the internal
`eventCh` and non-blockingly copies each event to every subscriber, so each TUI
resubscribe or daemon stream client gets its own feed rather than racing for a
shared channel. Progress events are lossy (drop-newest on a full buffer);
completion is self-healing.

### `youtube`

Two responsibilities. **yt-dlp wrappers** (`fetcher.go`, `parse.go`) run yt-dlp
as a subprocess and parse `--dump-json`, with pagination and rate-limit/retry
handling; recommended, subscribed-channels, channel-videos, channel-latest, and
transcript fetches all live here. **Internal YouTube API** (`ytapi.go`) uses
browser-cookie auth (via yt-dlp `--cookies-from-browser` or a cookies file) and
SAPISIDHASH signing for subscribe/unsubscribe and playlist mutations — no
official API key required.

### `device/player`

`Backend` interface: `Launch`, `LaunchAudio`, `Close`, each returning a
`*Session` that tracks playback lifecycle and position. Two implementations:

- **`mpris`** — launches the player, then polls D-Bus for the MPRIS2 `Position`
  property (resolving the bus name by PID to avoid conflicts). On stop it writes
  the offset so the next open — local *or* streamed — resumes from there.
- **`simple`** — spawns the process with no position tracking; the fallback when
  D-Bus is unavailable or `player_backend = "simple"`.

Player-specific seek flags are handled by per-binary drivers (mpv/vlc/generic).

---

## TUI layer (`internal/tui`)

Strict Elm architecture on Bubble Tea v2. The TUI holds an `api.Backend` (either
`InProc` or `Remote` — it can't tell which) and nothing else stateful about the
outside world.

### Root (`internal/tui/app`)

`Root` (`app/root.go`) is the top model. It owns the ordered `[]tui.Tab`, the
active index, a modal `[]overlay.Overlay` stack, the shared `spinner`, the tab
bar / status bar chrome, the `keymap.KeyMap`, and the command `Registry`. Its
`Update` fans messages out through focused dispatchers (key handling, playback,
channel actions, command palette, misc/broadcast).

**Message routing.** Most messages broadcast to all tabs; private per-tab
messages implement a `TabAddressedMsg` interface carrying a `TabTarget`, so Root
routes them only to the owning tab (avoiding redundant background loads across
tabs). The top overlay always gets first crack at input when it has focus.

**Spinner pacing.** A single app-wide spinner ticks only while some tab or
overlay reports `Loading()`; Root stops the tick loop when everything is idle and
restarts it on demand. (The tick delay lives in the auto-rearm command, not a
busy `.Tick`.)

### Tabs (`internal/tui/tab`)

Every tab implements the `Tab` interface (`internal/tui/tui.go`):

``` go
type Tab interface {
    tea.Model
    ID() TabID
    Title() string
    ShortHelp() []key.Binding
    InterceptsInput() bool          // text input focused → bypass global keys
    SelectedVideo() (domain.Video, bool)
    Loading() bool                  // fetch in flight → keep spinner ticking
}
```

Tabs are value types (Update returns the mutated copy). The tab kinds are
`Feed`, `Channels`, `Tags`, `Playlists`, `Search`, `Downloading`, `Local`,
`History`, `Activity`. Each is built by `app/panels.go:buildPanel()` from a
`config.Panel`, receiving its resolved mode, sort, and per-panel column
selection (`cfg.Columns[name]`, passed as a `wantCols ...string` variadic).

### Shared table (`internal/tui/videotable`)

All list tabs embed **`TableNav`**, a wrapper over
[evertras/bubble-table](https://github.com/Evertras/bubble-table) providing
shared navigation (j/k, page, `gg`/`G`, goto-line, optional circular wrap).
Columns are typed `ColumnDef[T]` values produced by factory functions
(`NumCol`, `IndicatorCol`, `TitleFlexCol`, `ChannelCol`, `DurationCol`,
`ViewsCol`, `DateCol`, `SizeCol`, …), each keyed to a small row interface
(`HasTitle`, `HasChannelInfo`, `HasDuration`, …). `SelectColumns` filters and
reorders a tab's full column set against the configured keys, falling back to
the full set when the config is empty or yields nothing.

> `TableNav.borderPad()` compensates for a bubble-table quirk: it reserves
> `len(cols)+1` columns for borders even when they're empty.

### Sort chords (`internal/tui/tab/sort.go`)

Sort-capable tabs embed `sortState`, which tracks the current mode, whether the
sort chord is armed, and — per Phase 22 — an `enabled map[int]bool` of which
sort modes are reachable given the *visible* columns. A sort key is only honored
when at least one of the columns that expose it is present, so a panel that
hides the views column also can't sort by views. The chord arms on the trigger
key and resolves on the next keystroke.

### Command palette (`internal/tui/command`)

A `:`-triggered command palette backed by a `Registry`. Root registers global
commands at startup; the active tab may expose view-local commands via a
provider interface, which shadow/extend the global set. Completion cycles the
merged set; unknown commands report an error in the status bar. Backed by
`confirm` and `command-help` overlays.

### Overlays (`internal/tui/overlay`)

Modal stack over Root. Each `Overlay` renders on top of the frame and can
capture input:

``` go
type Overlay interface {
    tea.Model
    Render(behind string, width, height int) (view, kittySeq string)
    InterceptsInput() bool
    WidthReduction() int   // columns reserved on the right (0 = centered)
    HasFocus() bool
}
```

Overlays include the **video detail** side panel (description, links, chapters,
transcript — with chapter navigation and inline thumbnail via the kitty graphics
sequence), **help**, the **command bar** + **command help**, **confirm**,
**add-to-playlist**, **export selection** (choose which bundle sections to write;
Space toggles, Enter writes), **import preview**, and **config issues** (startup).
Root opens them on `OpenOverlayMsg` and pops them on Escape / `PopOverlayMsg`.

### Keymap & rendering

`internal/tui/keymap` builds a `KeyMap` of `key.Binding`s from config strings
(splitting comma-separated keys). `internal/tui/render` centralises frame
composition — notably `ClampLine`, which every frame line must pass through
(lipgloss `.Width()` wraps over-wide lines and corrupts the layout).
`internal/tui/styles` and `internal/theme` supply the palette;
`internal/tui/component` holds the stateless tab bar and status bar.

---

## Data flow

### Startup (single-binary)

``` text
cmd/yt-tui/main.go
  config.Load()                       → cfg (+ Issues)
  buildBackend(cfg, connectAddr)
    no daemon addr → db.New(); downloader.New(); youtube.NewClient()
                     api.NewInProc(db, yt, dl, cfg)
                       ↳ FeedService, ChannelService, PortabilityService
                       ↳ thumbs / transcripts / profiles stores (optional)
    inproc.StartBackgroundEnrichment(ctx)   → goroutine: sync subs → backfill → enrich
  tui/app.New(backend, cfg, player, issues)
  tea.NewProgram(root).Run()
    → tabs fetch from cache instantly, refresh in background
```

### Startup (client/daemon)

``` text
yt-tuid: db/downloader/youtube → api.NewInProc → buildMux (bearer, /healthz,
         /media, transport.Mount) → http.Serve

yt-tui:  buildBackend sees DaemonAddr → api.NewRemote(addr, token, httpClient)
         → optional LoadProfileOnConnect (ProfileService)
         → CheckAvailability (HealthService) surfaces daemon-side faults
         → tui/app.New(remote, …) — identical TUI, calls cross the wire
```

### Key press

``` text
tea.KeyMsg → Root.Update
  → overlay (top, focused)?      → overlay handles
  → tab InterceptsInput()?       → active tab handles (text input)
  → global keybinding / chord    → Root dispatch (nav, play, command, …)
  → else                          → active tab handler
  → returned tea.Cmd runs on a goroutine → message → Update → View
```

### Download events

``` text
downloader run() goroutine → emit(EventProgress/Complete/Error)
  → broadcaster fans to every Subscribe()r
  → InProc.Events() (or DownloadService.Events stream in daemon mode)
  → Downloading tab updates queue rows
```

---

## Threading model

The Bubble Tea `Update`/`View` loop runs on one goroutine; every `tea.Cmd` runs
on a framework-managed goroutine. Direct goroutine launches in the codebase are:

- **Fire-and-forget DB writes** (`go func(){ _ = db.Save… }()`) — safe because
  `SetMaxOpenConns(1)` serialises them and they never feed back into the model.
- **The downloader** — one goroutine per active download, one broadcaster
  reader; it reaches the UI only through `Event` values.
- **The MPRIS poll loop** — writes positions to the DB, never back to the model.
- **Background enrichment** — a single paced goroutine inside `InProc`. On start
  it syncs the subscription list from the account (`ChannelService.SubscribedChannels`,
  so a reset DB re-seeds and new subs appear), backfills each channel's videos,
  then runs the enrichment pass.

In client/daemon mode these all run *on the daemon*; the TUI process holds no
subprocess or DB goroutines at all — its only concurrency is the Connect
`Events` stream reader.
