# Plan: Video Details Popup

A centered overlay popup triggered by `i` on any video row. Shows: thumbnail image (rendered with half-block Unicode chars and true-color ANSI), full video metadata, channel info, and scrollable description. Data is fetched on-demand via `yt-dlp --dump-json` on the video URL.

---

## Phase 0: Documentation Discovery

**yt-dlp JSON output for single video (no `--flat-playlist`):**

Running `yt-dlp --dump-json --no-warnings --quiet <video-url>` returns a single JSON object with:
- `id`, `title`, `channel`, `channel_id`, `duration`, `view_count`, `upload_date`, `webpage_url`
- `description` — full description text
- `thumbnail` — best-quality thumbnail URL (e.g. `https://i.ytimg.com/vi/VIDEO_ID/maxresdefault.jpg`)
- `channel_follower_count` — subscriber count

**Thumbnail image fetching:**
- YouTube thumbnail URLs are public, no auth required
- Standard Go `net/http` + stdlib `image/jpeg` / `image/png` (registered via blank imports) decode them
- `image.Image.At(x, y)` returns each pixel's color
- True-color ANSI: `\x1b[48;2;R;G;Bm` (background), `\x1b[38;2;R;G;Bm` (foreground), `\x1b[0m` (reset)
- Half-block character `▄` (U+2584): top row of the terminal cell = background color, bottom row = foreground color → 2 image rows per terminal row

**Existing overlay pattern (`renderAddOverlay` in view.go:1021):**
- Renders a lipgloss bordered box (rounded border, `colorAccent` border color, padding 1 2)
- Composites it onto the `behind string` by overwriting lines character by character
- Centered with `(m.width - bw) / 2` and `(totalLines - bh) / 2`
- Follow the exact same pattern for the new overlay

**Allowed APIs:**
- `lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorAccent)` — same as existing overlay
- `lipgloss.Width(s)` / `lipgloss.Height(s)` — measure rendered string dimensions
- `lipgloss.JoinVertical(lipgloss.Left, ...)` — stack sections vertically
- `image.Decode(r)` — stdlib, requires `_ "image/jpeg"` and `_ "image/png"` blank imports
- `net/http.Get(url)` — download thumbnail

---

## Phase 1: Data Types and Fetcher

**Files:** `internal/youtube/types.go`, `internal/youtube/fetcher.go`

### 1a — `VideoDetails` struct (`types.go`)

Add after the existing `Channel` struct:

```go
type VideoDetails struct {
    Video
    Description  string
    ThumbnailURL string
    Subscribers  int64
}
```

### 1b — Message type (`fetcher.go`)

Add to the "Bubbletea message types" section:

```go
type VideoDetailsMsg struct {
    Details VideoDetails
    Err     error
}
```

### 1c — `ytdlpDetailEntry` struct and fetch command (`fetcher.go`)

Add a private struct for the non-flat dump-json format:

```go
type ytdlpDetailEntry struct {
    ID                   string  `json:"id"`
    Title                string  `json:"title"`
    Channel              string  `json:"channel"`
    ChannelID            string  `json:"channel_id"`
    Duration             float64 `json:"duration"`
    ViewCount            int64   `json:"view_count"`
    UploadDate           string  `json:"upload_date"`
    WebpageURL           string  `json:"webpage_url"`
    Description          string  `json:"description"`
    Thumbnail            string  `json:"thumbnail"`
    ChannelFollowerCount int64   `json:"channel_follower_count"`
}
```

Add fetch command (does NOT use `buildArgs` since we need `--dump-json` without `--flat-playlist`):

```go
func FetchVideoDetails(cfg *config.Config, videoURL string) tea.Cmd {
    return func() tea.Msg {
        args := []string{
            "--dump-json",
            "--no-warnings",
            "--quiet",
        }
        if cfg.Browser != "" {
            args = append(args, "--cookies-from-browser", cfg.Browser)
        }
        args = append(args, videoURL)

        cmd := exec.Command("yt-dlp", args...)
        out, err := cmd.Output()
        if err != nil {
            return VideoDetailsMsg{Err: fmt.Errorf("yt-dlp: %w", err)}
        }
        var e ytdlpDetailEntry
        if err := json.Unmarshal(out, &e); err != nil {
            return VideoDetailsMsg{Err: fmt.Errorf("parse: %w", err)}
        }
        return VideoDetailsMsg{Details: VideoDetails{
            Video: Video{
                ID:         e.ID,
                Title:      e.Title,
                Channel:    e.Channel,
                ChannelID:  e.ChannelID,
                Duration:   int(e.Duration),
                ViewCount:  e.ViewCount,
                UploadDate: e.UploadDate,
                URL:        e.WebpageURL,
            },
            Description:  e.Description,
            ThumbnailURL: e.Thumbnail,
            Subscribers:  e.ChannelFollowerCount,
        }}
    }
}
```

**Verification:** `go build ./...` passes; no new deps added.

---

## Phase 2: Thumbnail Fetching and Rendering

**File:** `internal/ui/image.go` (new file)

### 2a — Thumbnail fetch command

```go
package ui

import (
    "image"
    _ "image/jpeg"
    _ "image/png"
    "net/http"

    tea "github.com/charmbracelet/bubbletea"
)

type thumbnailLoadedMsg struct {
    img image.Image
}

func loadThumbnailCmd(url string) tea.Cmd {
    return func() tea.Msg {
        resp, err := http.Get(url)
        if err != nil {
            return thumbnailLoadedMsg{}
        }
        defer resp.Body.Close()
        img, _, err := image.Decode(resp.Body)
        if err != nil {
            return thumbnailLoadedMsg{}
        }
        return thumbnailLoadedMsg{img: img}
    }
}
```

Errors are silent (overlay renders without thumbnail — no error shown to user).

### 2b — Half-block renderer

```go
func renderThumbnail(img image.Image, targetW, targetH int) string {
    if img == nil || targetW <= 0 || targetH <= 0 {
        return ""
    }
    // Resize to targetW x (targetH*2) pixels using nearest-neighbor.
    bounds := img.Bounds()
    srcW := bounds.Max.X - bounds.Min.X
    srcH := bounds.Max.Y - bounds.Min.Y

    var sb strings.Builder
    for row := 0; row < targetH; row++ {
        for col := 0; col < targetW; col++ {
            // Map terminal cell (col, row) to two pixel rows: topY and botY.
            topY := (2*row * srcH) / (targetH * 2)
            botY := ((2*row + 1) * srcH) / (targetH * 2)
            px := col * srcW / targetW

            tr, tg, tb, _ := img.At(bounds.Min.X+px, bounds.Min.Y+topY).RGBA()
            br, bg, bb, _ := img.At(bounds.Min.X+px, bounds.Min.Y+botY).RGBA()

            // RGBA returns 16-bit values; shift to 8-bit.
            sb.WriteString(fmt.Sprintf("\x1b[48;2;%d;%d;%dm\x1b[38;2;%d;%d;%dm▄",
                tr>>8, tg>>8, tb>>8,
                br>>8, bg>>8, bb>>8,
            ))
        }
        sb.WriteString("\x1b[0m\n")
    }
    result := sb.String()
    if strings.HasSuffix(result, "\n") {
        result = result[:len(result)-1]
    }
    return result
}
```

**Anti-patterns to avoid:**
- Do NOT use `lipgloss.Width()` on the rendered thumbnail string — ANSI escape sequences inflate the byte count; use `targetW` directly as the column count.
- Do NOT attempt to re-style the thumbnail string with lipgloss — the raw ANSI escapes will be preserved as-is by lipgloss since the string is treated as already-rendered content.

---

## Phase 3: Model State

**File:** `internal/ui/model.go`

### 3a — Add fields to `Model` struct (after the `// ── Shared` section):

```go
// ── Video detail overlay ──────────────────────────────────────────────────
vidDetailOverlay bool
vidDetailVideo   *youtube.VideoDetails
vidDetailLoading bool
vidDetailDescVS  int         // description scroll start line
vidDetailThumb   image.Image // nil until loaded; stays nil if fetch fails
```

Import `"image"` in model.go.

### 3b — Add keybinding

**File:** `internal/ui/keys.go`

Add `VideoInfo key.Binding` to the `keyMap` struct and wire it from `cfg.Keybindings.VideoInfo` (default `"i"`) in `buildKeyMap`.

**File:** `internal/config/config.go`

Add `VideoInfo string` to the `KeyBindings` struct and `"i"` as the default in `fillDefaults`.

---

## Phase 4: Update Handler

**File:** `internal/ui/update.go`

### 4a — Early-exit handler in `handleKey` (before the per-tab dispatch, after `addOverlay` check)

Pattern: mirror the `m.addOverlay` early-exit block at line ~371.

```go
if m.vidDetailOverlay {
    return m.handleVideoDetailKey(msg)
}
```

### 4b — `handleVideoDetailKey` function

```go
func (m Model) handleVideoDetailKey(msg tea.KeyMsg) (Model, tea.Cmd) {
    switch {
    case key.Matches(msg, m.keys.Back), key.Matches(msg, m.keys.Quit),
         msg.String() == "esc":
        m.vidDetailOverlay = false
        m.vidDetailVideo = nil
        m.vidDetailThumb = nil
        m.vidDetailDescVS = 0
        m.vidDetailLoading = false
    case key.Matches(msg, m.keys.Down):
        m.vidDetailDescVS++
    case key.Matches(msg, m.keys.Up):
        if m.vidDetailDescVS > 0 {
            m.vidDetailDescVS--
        }
    }
    return m, nil
}
```

### 4c — Trigger key in per-tab handlers

In `updateRecommended`, `updateSubscriptions`, `updateSearch`, `updatePlaylists`, `updateDownloading`, `updateLocal`, add:

```go
case key.Matches(msg, m.keys.VideoInfo):
    if v, ok := m.currentVideo(); ok {
        m.vidDetailOverlay = true
        m.vidDetailLoading = true
        m.vidDetailVideo = nil
        m.vidDetailThumb = nil
        m.vidDetailDescVS = 0
        return m, youtube.FetchVideoDetails(m.cfg, v.URL)
    }
```

Since `currentVideo()` already covers all tabs, this can alternatively be placed in a shared handler called from each tab's key handler. Follow the existing pattern for `d`/`D` (download) which is repeated per-tab.

### 4d — Message handlers in `Update`

Add two new `case` blocks in the main `Update` switch (alongside `FetchResultMsg`, `SearchResultMsg`, etc.):

```go
case youtube.VideoDetailsMsg:
    if msg.Err != nil {
        m.vidDetailLoading = false
        m.setStatus("Could not load video details: "+msg.Err.Error(), true)
        m.vidDetailOverlay = false
        return m, nil
    }
    details := msg.Details
    m.vidDetailVideo = &details
    m.vidDetailLoading = false
    if details.ThumbnailURL != "" {
        return m, loadThumbnailCmd(details.ThumbnailURL)
    }
    return m, nil

case thumbnailLoadedMsg:
    m.vidDetailThumb = msg.img
    return m, nil
```

---

## Phase 5: View

**File:** `internal/ui/view.go`

### 5a — Hook into `View()`

After the `m.addOverlay` block (line ~29), add:

```go
if m.vidDetailOverlay {
    content = m.renderVideoDetailOverlay(content)
}
```

### 5b — `renderVideoDetailOverlay` function

Layout within the bordered box (width = min(m.width-4, 90), height = min(contentH-4, 40)):

```
┌── Video Details ───────────────────────────────────────────────────┐
│                                                                    │
│  [thumbnail]  Title: Full Video Title Here                         │
│  [  30×15  ]  Channel: Channel Name  (1.2M subscribers)            │
│  [   cols  ]  Duration: 12:34   Views: 4.5M   Date: 12/06/2024     │
│               URL: https://youtube.com/watch?v=...                 │
│                                                                    │
│  Description                                                       │
│  First line of description text here...                            │
│  Second line...                                                    │
│  ...                                                               │
│                                                                    │
│  j/k: scroll  esc: close                                           │
└────────────────────────────────────────────────────────────────────┘
```

**Key rendering details:**

- Thumbnail area: `thumbW = 30` columns, `thumbH = 15` rows. Call `renderThumbnail(m.vidDetailThumb, thumbW, thumbH)`. If `m.vidDetailThumb == nil`, fill with `thumbH` lines of `strings.Repeat("░", thumbW)`.
- Metadata area: `metaW = boxW - thumbW - 5` (subtract padding + separator). Build with `lipgloss.NewStyle().Width(metaW)`.
- Join thumbnail and metadata side by side by splitting each into lines and joining line-by-line.
- Description: split by `\n`, apply scroll offset `m.vidDetailDescVS`, render at most `descH` lines. Wrap long lines at `boxW - 4`.
- Loading state: if `m.vidDetailLoading`, show centered spinner + "Loading video details…" inside the box.
- The outer compositing follows `renderAddOverlay` exactly: split `behind` into lines, overwrite lines starting at `y` from column `x`.

**Anti-patterns to avoid:**
- Do NOT call `lipgloss.Width()` on thumbnail strings containing raw ANSI — measure by `thumbW` directly.
- Do NOT render the entire description into the box without a scroll window — long descriptions (hundreds of lines) will overflow.

### 5c — Update `fullHintRaw` 

Add `i: info` (or the configured key) to the hint for tabs where `currentVideo()` returns a value (all video-list contexts). This means adding it for `tabRecommended`, `tabSubscriptions` (both modes), `tabSearch`, `tabPlaylists`, `tabDownloading`.

---

## Phase 6: Verification

**Functional checks:**
1. Press `i` on any video in any tab → overlay appears with spinner, then populates with details
2. Description longer than overlay height → j/k scrolls it
3. Press `esc` or `h` → overlay closes, main list visible again
4. If yt-dlp fails → status bar shows error, overlay does not open
5. If thumbnail fetch fails → overlay shows placeholder `░` pattern, no error shown
6. `go build ./...` passes with no new external dependencies

**Grep checks (anti-pattern guards):**
```
grep -n "lipgloss.Width.*thumb" internal/ui/view.go     # should return nothing
grep -n "renderThumbnail" internal/ui/                  # should appear in image.go + view.go only
grep -n "VideoInfo" internal/config/config.go           # should appear once in struct + once in fillDefaults
grep -n "VideoDetailsMsg" internal/ui/update.go         # should appear once as case handler
```

---

## Execution Order

| Phase | Description | Depends on |
|---|---|---|
| 1 | Data types and fetcher | — |
| 2 | Thumbnail fetch + render (`image.go`) | — |
| 3 | Model fields + keybinding + config | — |
| 4 | Update handlers | 1, 2, 3 |
| 5 | View overlay | 1, 2, 3 |
| 6 | Verification | 4, 5 |

Phases 1–3 are independent and can be done in any order.
