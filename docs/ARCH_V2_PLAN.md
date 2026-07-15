# yt-tui v2 — Ground-Up Architecture Plan

**Created:** 2026-07-12 · **Status:** design · **Supersedes-scope:** greenfield successor to the P1–P5 in-place refactors (`docs/REFACTOR_PLAN.md`, `docs/ARCH_REVIEW_PLAN.md`).

**Decisions locked (2026-07-12):**
- **Remote media:** media-server model — the daemon downloads/stores files and serves them over an HTTP range endpoint; the client's player plays daemon-provided URLs (Jellyfin-like). See §2, §8.
- **Cookies on the daemon:** the daemon holds the single browser-cookie source; client devices need no browser or YouTube login. **Provisioned by a one-time GUI login on the daemon host** (xpra / `ssh -X` into a browser, even though the box is headless); the daemon then uses the existing `--cookies-from-browser` path, self-refreshing — no file export, no Syncthing, no re-export. Reuses today's `browser` config verbatim; **no new cookie code**. See §2.
- **`InProc` is permanent, not scaffolding:** single-binary mode is a first-class deployment target, kept alongside `Remote`. See §5.

---

## 0. Thesis

The current app is a mature TUI whose pain is a single ~4,500-line God `Model`
(`model.go` 1,081 · `update.go` 2,422 · `view.go` 1,021) that co-owns view state and
async-written data slices. The domain has already been peeled off into pure packages
(`feed`, `library`, `channels`, `media`) and a `Store` interface exists. v2 keeps those,
dissolves the God struct into a **component tree**, and splits the app into a **headless
media daemon** (source of truth) and a **TUI client** (a view + a playback device) that
talk over **one API contract with two transports** (in-process and remote). The same
contract later serves a web frontend with no backend changes.

## 1. Guiding principles

- **Hexagonal backend** — services depend on ports (interfaces), never on SQLite or yt-dlp directly.
- **BubbleTea composition** — every visual unit is its own `tea.Model`; the root delegates `Update`/`View` down a tree and `tea.Batch`es the returned cmds. No God struct.
- **One contract, two transports** — `api.Backend` interface with an `InProc` impl (co-located, zero network) and a `Remote` impl (Connect/gRPC). The TUI never knows which it uses. This is what makes "single binary now, remote daemon later" free.
- **Daemon owns the truth; TUI is a view.** The TUI subscribes to an event stream and issues commands — a generalization of today's `downloader.EventMsg` loop.

## 2. Capability split (the one genuinely hard part)

Not all work is relocatable to a remote host. Playback and browser cookies are **device
capabilities**, not daemon services.

| Concern | Owner | Why |
|---|---|---|
| DB / persistence | **Daemon** | single source of truth |
| YouTube fetch (yt-dlp, feeds, search) | **Daemon** | network work, cache, rate-limit handling |
| Download queue | **Daemon** | long-running, always-on; files land on daemon host |
| Feed filter/sort/merge (`feed`, `media`) | **Daemon** (domain) | pure logic, belongs with the data |
| **Playback (mpv/vlc/MPRIS)** | **Client/device** | audio+video exit the machine the human sits at |
| **Browser cookies** | **Daemon** | one cookie source; client devices need no browser/login |

**Model:** the daemon is a headless media brain; the client is a *device* that renders UI
and offers a Player capability. To play, the client (a) asks the daemon to resolve a
playable source, (b) launches its local player, (c) streams playback-position events back
so resume-tracking still works.

**Resolve-source, media-server model (locked decision):**
- **Co-located** (same machine): daemon returns a local file path (downloaded) or a
  yt-dlp-resolved stream URL (stream-only). Client plays it directly.
- **Remote:** for downloaded files the daemon serves them over an authenticated HTTP range
  endpoint (`GET /media/{id}`) and returns that URL; for stream-only it returns a resolved
  stream URL. Client's player plays a URL in both cases. Downloads always live on the
  daemon host.

**Cookie provisioning (locked):** the daemon uses the existing `--cookies-from-browser`
path against a browser profile *on the daemon host*. On a headless server the profile is
created by a **one-time GUI login over a remote display** — `xpra start ssh:host
--start=chromium` or `ssh -X host` → `chromium` — after which the box is terminal-only
again. yt-dlp reads live cookies each fetch (self-refreshing; long-lived Google SID
cookies), so there is **no file export, no Syncthing, no periodic re-export**. Config reuses
today's `browser = "..."` field verbatim (drop the keyring suffix on headless hosts — they
fall back to Chromium's `basic`/plaintext store, so extraction needs no keyring daemon).
Net effect on the design: **no new cookie code** — the `source/youtube` adapter keeps
today's behavior; the only new artifact is an ops note for the one-time login.

## 3. Process & module topology

```
cmd/
  yt-tuid/            # daemon binary (headless)
  yt-tui/             # TUI client binary
                      #   yt-tui            → co-located: constructs in-proc daemon
                      #   yt-tui --connect  → remote: dials a daemon

internal/
  domain/             # pure types + rules, zero deps: Video, Channel, Playlist,
                      #   HistoryEvent, DownloadState, SortMode …
                      #   ← feed/, library/, channels/, media/ move here ~intact

  backend/            # daemon guts (application services + adapters)
    service/          #   FeedService, LibraryService, DownloadService,
                      #     ChannelService, PlaylistService, HistoryService
    store/sqlite/     #   Store impl (today's internal/db) behind repo ports
    source/youtube/   #   yt-dlp + internal-API adapter (today's internal/youtube)
    download/         #   queue (today's internal/downloader)
    media/            #   HTTP range file server (media-server model)
    transport/        #   Connect/gRPC handlers wrapping services

  api/                # THE CONTRACT
    backendv1/        #   generated Connect/proto code
    client.go         #   Backend interface + Remote client + InProc client

  tui/
    app/              #   root model: focus, key routing, deployment wiring
    component/        #   reusable: tabbar, statusbar, commandbar, overlaystack
    tab/              #   one package per tab (recommended, subscriptions, …)
    theme/            #   today's internal/theme
    keymap/           #   keys + chord resolver (today's keys.go + chord system)
    command/          #   Command type, registry, global command set (§7)

  device/
    player/           #   mpv/vlc/mpris (today's internal/player), now client-side
```

`domain/` is the shared vocabulary imported by both sides. `backend/` and `tui/` never
import each other — they meet only at `api/`.

## 4. Backend: services + DI

Each **application service** is a use-case boundary taking dependencies as ports:

```go
type FeedService struct {
    store  FeedRepo        // port → store/sqlite
    source RecommendSource // port → source/youtube
    events EventBus        // fan-out to subscribers
}
```

Ports live next to the service that needs them; adapters implement them. Wiring happens
once in `cmd/yt-tuid/main.go` (and in the in-proc constructor) — plain constructor
injection, no DI framework. Services expose:

- **Commands/queries** — `Subscribe(ctx, channelID)`, `Recommended(ctx, opts) ([]Video, error)`.
- **Event streams** — `Subscribe() <-chan Event` for download progress, background feed
  refreshes, new-video arrivals. Replaces today's scattered `Background bool` flags and
  per-operation msg types.

## 5. API contract (one definition, two transports)

```go
// api/client.go
type Backend interface {
    Recommended(ctx, RecommendedReq) (Feed, error)
    Subscriptions(ctx) (Feed, error)
    Search(ctx, query) (SearchResult, error)
    Subscribe(ctx, channelID) error
    Enqueue(ctx, DownloadReq) error
    ResolveSource(ctx, videoID, Quality) (PlayableSource, error)  // file path or URL
    ReportPosition(ctx, videoID, pos) error
    Events(ctx) (<-chan Event, error)                             // server-streaming
    // …playlists, history, channels
}
```

Two implementations behind it:
- `InProc{svcs …}` — calls services directly (single-binary mode; zero serialization;
  trivial to test the TUI against).
- `Remote{client}` — dials the daemon.

**`InProc` is a permanent deployment mode, not a migration crutch.** "Backend fully
separated" means the `backend/service/*` packages sit behind `api.Backend` with no
dependency on the TUI — it does *not* mean they're only reachable over a socket. `InProc`
is the TUI binary importing those same service packages and calling them through the
interface directly. It stays because the common case is one machine: a user who runs
`yt-tui` and quits shouldn't have to spawn a daemon, manage a socket, or pay
serialization + a hop to reach code in the same process. It is nearly free — a thin adapter
over the *same* service structs the `Remote` handlers wrap (one backend, two front doors),
and it lets tabs be tested with no server lifecycle. **Rejected alternative:** "always
client/server, even locally" (dial the daemon over a Unix socket even on one machine) —
one code path, but every launch must spawn-or-attach a daemon and serialize even when
nothing is remote. For a launch-and-quit TUI, `InProc` wins.

**Transport: [Connect](https://connectrpc.com/)** over raw gRPC. Connect (`connect-go`, by
Buf) generates from one Protobuf schema a server that speaks gRPC, gRPC-Web, *and* its own
plain HTTP/1.1+JSON protocol on the same endpoint — so it's `curl`-able and browser-callable
without a gRPC-Web proxy, unlike raw gRPC. Built on standard `net/http` (ordinary TLS /
middleware / auth). Server-streaming carries `Events`. That directly serves the "other
interfaces next" goal: a web frontend consumes the same service over HTTP/JSON, no second
API. **Alternatives:** raw gRPC (no browser without a proxy, not `curl`-able) or hand-rolled
HTTP+JSON (no codegen/streaming ergonomics); the `api.Backend` interface lets the transport
be swapped later. Auth = bearer token in config; TLS for remote.

In BubbleTea the event stream becomes a `tea.Cmd` that reads one event off the channel and
re-arms itself — today's `downloader` pattern generalized into the single daemon→TUI spine.

## 6. TUI architecture

### Component contract

```go
type Component interface {
    tea.Model            // Init / Update / View
    SetSize(w, h int)
}

type Tab interface {
    Component
    Title() string
    ShortHelp() []key.Binding   // feeds help/status bar
    Context() KeyContext        // drives chord/sort dispatch (today's ContextID)
}
```

This is the natural continuation of the P4 `tabView` work (`internal/ui/view_tab.go`,
`docs/TABVIEW_DESIGN.md`): that refactor identified the behavioral interface; here it is
real from day one instead of retrofitted onto a shared struct.

### Component tree

```
app.Root                         (owns focus, size, deployment client)
├── component.TabBar
├── tab.Container                (holds active Tab, lazy-inits others)
│   ├── tab/recommended          (bubbles/list)
│   ├── tab/subscriptions        (split: channel list ⇆ video list)
│   ├── tab/channels
│   ├── tab/playlists            (split panes)
│   ├── tab/search               (textinput + results + history)
│   ├── tab/downloading          (progress list; subscribes to Events)
│   ├── tab/local
│   ├── tab/history
│   └── tab/activity
├── component.OverlayStack       (modal manager — today's overlay stack)
│   ├── overlay/videodetails
│   ├── overlay/links
│   ├── overlay/chapters
│   ├── overlay/addtoplaylist
│   └── overlay/help
├── component.CommandBar         (:cmd + completions)
└── component.StatusBar          (status text, spinner, chord/hint line)
```

### Message routing & focus (the crucial discipline)

- **`WindowSizeMsg` flows top-down:** root computes each region's box and calls `SetSize`;
  no child measures the terminal itself.
- **`tea.KeyMsg` is not broadcast.** Root runs it through the **keymap/chord resolver
  first** (global keys: tab switch, quit, command mode, chords). If unconsumed it goes to
  **exactly one focus target**: top overlay if the stack is non-empty, else the command bar
  if active, else the active tab. This eliminates the "which input owns the keyboard" class
  of bugs that today's `inputMode` scalar exists to patch.
- **All other msgs** (data, events, ticks) fan out to interested components; each returns
  its own cmd; root `tea.Batch`es them.
- **Cross-component comms via typed messages, never sibling pointers.** A tab wanting a
  detail overlay returns `OpenOverlayMsg{VideoDetails, id}`; root handles it. A tab never
  holds a pointer to the status bar.

### Owned vs shared state

Today's pain is async-written shared slices (`subChVideos`, `playlistVidCache`,
`searchVideos`, …) living on the God struct. In v2 the **daemon owns that data**; a tab
holds only *view state* (cursor, scroll, sort, its `bubbles/list`). On change the daemon
emits an event → root routes it to the owning tab → tab updates its list. Tabs become
independent and unit-testable against a fake `Backend`.

## 7. Cross-cutting components

- **`keymap`** — the two-level chord system + configurable bindings as a standalone
  resolver the root consults, returning a typed action. Decoupled from any model.
- **`theme`** — unchanged; injected at construction.
- **`OverlayStack`** — today's insight that links/chapters stack over video-detail becomes
  a first-class `[]Overlay` where each overlay is a `Component`. Top overlay gets focus and
  renders last (composited over tab content).
- **`StatusBar` / `CommandBar`** — pure presentational; fed by messages, hold no app state.

### Command system (`:` across all views)

Generalizes today's `commands.go` / `cmdInput` / `cmdCompletions` into a first-class
registry owned by the **root**, sitting *above* the tab tree so every view gets `:` for
free. Two scopes:

- **Global** — registered once at the root, work in any view: `:q`, `:help`, `:tab search`,
  `:download <url>`, `:theme <name>`, `:connect <addr>`, `:sort date`.
- **View-local** — contributed by the active tab/overlay: `:delete` in Local, `:new` in
  Playlists, `:block` in Subscriptions.

```go
type Scope int
const ( ScopeGlobal Scope = iota; ScopeView )

type Command struct {
    Name     string
    Aliases  []string
    Help     string
    Scope    Scope
    Complete func(prefix string) []string  // arg completion
    Run      func(args []string) tea.Cmd   // returns a cmd; its msg flows through Update
}

// A tab or overlay optionally contributes local commands:
type CommandProvider interface { Commands() []Command }
```

**Mechanics:**
1. `component/commandbar` (root-owned) captures `:` input + completion.
2. On open, root builds a **merged registry** = global set + `active.(CommandProvider)`
   commands (if implemented) + top overlay's commands. The palette is always
   "global ∪ current context."
3. On submit, root resolves name/alias in the merged set and calls `Run(args)`, which
   returns a `tea.Cmd`. **`Run` never mutates state** — its resulting `tea.Msg` flows back
   through the normal `Update` path, so a view-local `:delete` just emits the same
   `DeleteMsg` the `x` key would (the owning view handles it); global commands' cmds
   typically call `api.Backend` (e.g. `:download` → `backend.Enqueue`). Elm loop intact,
   no reaching into a view's private state.

**Precedence:** flat merged namespace; **view-local shadows global** while that view is
active (vim buffer-local-mapping model). A startup lint flags accidental global collisions.

**Focus interaction:** `:` opens the command bar only when a *normal-mode* view has focus;
if a text input owns the keyboard (search, filter, rename) `:` types a literal colon. Same
single-focus arbitration as §6 — the command bar is just another focus target.

**Client/server note:** command handlers split naturally — pure-UI commands (`:tab`,
`:theme`) mutate client state via a message; action commands (`:download`, `:subscribe`)
return a cmd that calls `api.Backend`, so they work identically in co-located and remote
modes.

## 8. Deployment modes (all from the same two binaries)

1. **Single binary** (`yt-tui`) — root constructs `InProc`; player in-process. Feels like today.
2. **Local daemon** — `yt-tuid` runs backgrounded (systemd user unit); `yt-tui --connect
   localhost` dials it. Downloads survive TUI restarts.
3. **Remote daemon** — same over TLS+token; daemon serves media via HTTP range
   (`GET /media/{id}`); client plays URLs.
4. **(Later) Web / other frontends** — consume the same Connect service. No backend changes.

## 9. Phased migration (reuse, don't rewrite from zero)

Most non-UI packages survive. Phases 1–3 are **shippable in-place refactors of the current
tree** (one PR each, `go build`/`go vet`/`-race` green after every commit — the P5 working
style); the app keeps running as a single binary throughout. Phases 4–6 introduce the new
architecture. Each phase below lists *goal · concrete file work · verification · why it's
independently shippable*.

Dependency order: **1 → 2 → 3** are strictly sequential (each unblocks the next). **4**
depends on 2 (needs `api.Backend`) but not 3 (can start against the InProc seam while
services are still thin). **5** depends on 2+4. **6** depends on 5.

---

### Phase 1 — Extract `internal/domain/` (pure vocabulary)

**Goal:** one dependency-free package both sides import; nothing depends on SQLite or
bubbletea to name a `Video`.

**Work:**
- Create `internal/domain/`. Move the already-pure packages in as subpackages:
  `feed/`, `library/`, `channels/`, `media/` → `domain/feed`, `domain/library`, etc.
  (mechanical import-path rewrite; their tests move with them).
- Lift the pure *types* out of mixed packages:
  - `internal/youtube/types.go` → `domain`: `Video`, `Channel`, `VideoDetails`,
    `YTPlaylist`. Leave the yt-dlp *fetching* in `youtube` (becomes an adapter in Phase 2).
  - `internal/db`: `Playlist`, `LocalVideo`, `Link`, `Chapter`, `SBSegment`, history-event
    struct → `domain`. `db` keeps only persistence, now returning `domain` types.
- Update all imports (`internal/ui/*`, `db`, `downloader`, `player`) to the new paths.

**Verification:** build + `go test ./...` + `-race`. No behavior change — this is a move.

**Shippable:** yes; pure refactor, single binary unchanged. Highest churn, lowest risk.

---

### Phase 2 — Define `api.Backend` + `InProc`; de-bubbletea the fetchers

**Goal:** the contract exists and the TUI talks to the backend only through it — while
still one process.

**Work:**
- New `internal/api/client.go`: the `Backend` interface (§5) + `Event` types.
- **Decouple `youtube` from bubbletea.** Today `youtube/fetcher.go` returns
  `func(...) tea.Cmd` closures emitting `tea.Msg` (e.g. `FetchResultMsg`,
  `ChannelListMsg`, `SearchResultMsg`, `VideoDetailsMsg`, `YTPlaylistsMsg`,
  `PlaylistVideosMsg`). Convert each to a transport-agnostic method returning
  `(result, error)`; delete the `Msg` structs and the `Background bool` flag (background
  vs. foreground becomes a caller concern / event kind).
- New `internal/api.InProc`: a struct holding the existing collaborators (`db.Store`,
  `downloader.Downloader`, the `youtube` client) that implements `Backend` by calling them
  directly. `Events()` returns a channel fed by the downloader's existing `eventCh`
  (generalized to carry feed-refresh/new-video events later).
- In `internal/ui`: add thin `tea.Cmd` adapters that wrap `Backend` calls
  (`func fetchCmd(b Backend) tea.Cmd { return func() tea.Msg { v,err := b.Recommended(...) ; return recMsg{v,err} } }`).
  Replace direct `youtube.FetchRecommended(...)` etc. call sites in `update.go` with these.
- `main.go` constructs `InProc` and hands it to the model.

**Verification:** build/test/`-race`; manual smoke of every fetch path (recommended, subs,
search, playlists, video-details) since the msg plumbing changed. The `pure_test.go` /
`commands_test.go` suites guard the adapters.

**Shippable:** yes; still one binary, `InProc` is the only implementation. This is the
load-bearing seam — do it carefully.

---

### Phase 3 — Split services out of `ui/update.go` (dissolve the God `Update`)

**Goal:** business logic leaves the 2,422-line `update.go`; handlers shrink to
"call backend → apply result to view state."

**Work:** create `internal/backend/service/*` behind ports, moving the *decision* logic
currently inline in `update.go` handlers:
- `ChannelService` — subscribe/unsubscribe (partly in `domain/channels` already), block,
  alias/tag edits, background channel-video refresh.
- `DownloadService` — enqueue, cancel, play-after-download bookkeeping, delete-file+DB.
- `LibraryService` — local-video reloads (funnels the `library.Set` sites).
- `PlaylistService` — create local/YT, add-to-playlist, watch-later fallback.
- `HistoryService` — record events, position save/restore.
- `FeedService` — merge/filter/sort orchestration over `domain/feed`.
- Define ports next to each (`FeedRepo`, `RecommendSource`, `DownloadQueue`, …);
  `db`/`youtube`/`downloader` become the adapters implementing them. `InProc` now delegates
  to services instead of raw collaborators.

**Verification:** build/test/`-race`; each service gets a unit test with a fake port (the
payoff — logic testable without a TUI or DB). Manual smoke of each mutating action.

**Shippable:** yes; one binary. After this, `update.go` is routing + view-state only.

---

### Phase 4 — Rebuild the TUI as a component tree

**Goal:** replace the God `Model` with `tui/app.Root` + per-tab components, each a
`tea.Model` talking only to `api.Backend`.

**Work (incremental, tab by tab — the old and new UIs need not coexist; build the new tree
in `internal/tui` and switch `main.go` over once it reaches parity):**
1. **Scaffolding first:** `tui/app.Root` (focus, size, key routing), `keymap` resolver
   (port today's `keys.go` + chord system), `tui/command` registry + global command set
   (§7), `component/statusbar`, `component/tabbar`, `component/commandbar`,
   `component/overlaystack`, the `Tab`/`Component`/`CommandProvider` interfaces (§6–7).
2. **First tab: History** (leaf, read-only) — validates `Tab` + `bubbles/list` + `SetSize`
   + single-focus routing on the simplest case.
3. **Local**, then **Recommended** (validates event-driven data updates from `Backend`).
4. **Split-pane tabs:** Subscriptions, Playlists, Channels (the entangled ones — now clean
   because the daemon owns the shared slices; the tab holds only cursor/scroll/sort).
5. **Search** (input + history + drill-down), **Downloading** (Events subscription),
   **Activity**.
6. **Overlays** as `Component`s in the stack: videodetails, links, chapters, addtoplaylist,
   help. Port the Kitty/half-block thumbnail render from `ui/image.go`.
7. Delete `internal/ui/` once `main.go` points at `tui/app.Root`.

**Verification:** per-tab unit tests against a fake `Backend`; manual parity pass against
the feature list in `README.md`; `teatest` golden tests for a couple of tabs if useful.

**Shippable:** yes, once parity is reached; still single binary (InProc). This is the
biggest phase — sequence it so `main.go` can flip only when the new tree is at parity.

---

### Phase 5 — Remote transport + daemon binary

**Goal:** the same `Backend` reachable over the network; player moves client-side.

**Work:**
- Author a Connect/proto schema mirroring `Backend`; generate into `api/backendv1`.
  Server-streaming RPC for `Events`.
- `backend/transport/`: Connect handlers wrapping the Phase-3 services.
- `internal/api.Remote`: Connect client implementing `Backend`.
- `cmd/yt-tuid/main.go` (daemon: constructs services + transport, opens DB, starts
  download queue). `cmd/yt-tui/main.go` (client: `InProc` by default, `Remote` when
  `--connect <addr>`).
- Move `internal/player` → `internal/device/player`. Add `Backend.ReportPosition`; the
  client's player calls it on stop/seek so resume-tracking survives the split.
- Config split: daemon config (DB path, download dir, `browser`) vs. client config (theme,
  keybindings, player).

**Verification:** run daemon + `--connect` client on one host; exercise fetch/subscribe/
download/play; confirm positions persist round-trip. Co-located `InProc` path unchanged.

**Shippable:** yes — two binaries; `InProc` remains the default single-binary mode.

---

### Phase 6 — Media serving + auth/TLS (unblocks remote play & web)

**Goal:** downloaded files on a remote daemon are playable by a client; secure the wire.

**Work:**
- `backend/media/`: authenticated HTTP range server, `GET /media/{id}`.
- `Backend.ResolveSource` returns a local path when co-located, a `/media/{id}` URL (or
  resolved stream URL) when remote; client's player plays whichever.
- Bearer token (config) + TLS for `Remote` + the media endpoint.

**Verification:** remote daemon, client on a different host: play a downloaded file and a
stream-only video; range seeks work; unauthorized requests rejected.

**Shippable:** yes. With the Connect service + media endpoint in place, a web frontend is
now purely additive (§8, mode 4) — no backend changes.

## 10. BubbleTea best-practices this locks in

- Composition over a monolith `Model`; `tea.Batch` for concurrent cmds.
- `bubbles/list`, `viewport`, `textinput`, `help`, `key.Binding` instead of hand-rolled
  cursor/scroll math per tab.
- Single-focus key routing; size via `SetSize` top-down.
- Async work only ever as `tea.Cmd`; daemon events as a self-re-arming subscription cmd.
- No shared mutable state between Update and View — the tree enforces it structurally.

## 11. Open questions (deferred)

- **Multi-client** — if two clients attach to one daemon, event fan-out and per-client
  focus/position need thought (out of scope for v1 of v2).
- **Config split** — server config (DB path, download dir, source auth) vs client config
  (theme, keybindings, player) once they can live on different hosts.
