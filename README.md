# yt-tui

A terminal UI for browsing, searching, and downloading YouTube videos, built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Features

- Browse recommended videos and subscriptions (via browser cookies)
- Subscribed channels view with per-channel latest video, sort by date / name / subscribers
- Search YouTube — with persistent search history navigation (↑/↓ in search box)
- Manage YouTube playlists and Watch Later queue, plus local playlists
- Download videos or audio with [yt-dlp](https://github.com/yt-dlp/yt-dlp); files named `Channel - Title.ext`
- Concurrent download queue with progress display
- Queue a video to auto-play as soon as its download finishes
- Automatic [SponsorBlock](https://sponsor.ajay.app/) segment removal
- All feeds and channel lists load from local cache instantly, refresh in background
- History tracking with per-video event log; search entries replayable from history
- Subscribe / unsubscribe to channels from any tab (requires YouTube connection)
- Block channels from recommended from any tab
- Vim-style navigation, two-level configurable chord system for tab switching and sorting
- Configurable keybindings, hint modes, and color themes

## Requirements

- [yt-dlp](https://github.com/yt-dlp/yt-dlp)
- [mpv](https://mpv.io/) (or any player configured in `config.toml`)
- A Chromium-family browser with an active YouTube login (for recommendations, subscriptions, and YouTube playlist sync)

## Installation

```sh
go install github.com/EugeneShtoka/yt-tui@latest
```

Or build from source:

```sh
git clone https://github.com/EugeneShtoka/yt-tui
cd yt-tui
go build -o yt-tui .
```

## Configuration

On first run a config is written to `~/.config/yt-tui/config.toml`:

```toml
download_dir = "~/Videos/yt-tui"
browser = "vivaldi+gnomekeyring"   # passed to yt-dlp --cookies-from-browser
player = "mpv"
player_backend = "mpris"           # "mpris" or "simple"
max_concurrent_downloads = 3
sponsorblock = true
sponsorblock_categories = ["sponsor", "selfpromo", "interaction"]
audio_format = "mp3"
hint_mode = "full"                 # "full" | "minimal" | "none"
recommended_max_age_days = 7
recommended_fetch_count = 150
recommended_max_pages = 3
# theme = "theme.toml"             # path relative to config dir or absolute

tabs = [
  "recommended", "subscriptions", "playlists",
  "search", "downloading", "local", "history"
]

[keybindings]
# Action keys — comma-separated for multiple bindings, e.g. play = "p,enter"
play           = "p"
play_audio     = "P"
download       = "d"
download_audio = "D"
delete         = "x"
drill_down     = "enter"      # open/select; plays video in video contexts
back           = "h,backspace"  # go back / close pane (← arrow always works too)
filter         = "/"          # activate inline filter
copy_url       = "c"
hide_video     = "b"
hide_channel   = "B"
watch_later    = "w"
add_to_playlist = "a"
new_playlist   = "n"
toggle_mode    = "m"
subscribe      = "S"
unsubscribe    = "u"
help           = "?"
quit           = "q"

# Chord triggers
tab_chord   = "t"   # press t, then a tab_key to switch tabs
sort_chord  = "s"   # press s, then a sort_key to sort (only where sort is available)
goto_prefix = "g"   # press twice (gg) to jump to top
goto_bottom = "G"   # jump to bottom (or {n}G to jump to row n)

[keybindings.tab_keys]
recommended   = "r"
subscriptions = "s"
playlists     = "p"
search        = "S"
downloading   = "d"
local         = "l"
history       = "h"

[keybindings.sort_keys]
date        = "d"
views       = "v"
name        = "n"
channel     = "c"
duration    = "D"
subscribers = "s"   # channel list only
```

**`browser`** — passed directly to `yt-dlp --cookies-from-browser`. Any value yt-dlp accepts works (`chrome`, `firefox`, `vivaldi`, `vivaldi+gnomekeyring`, etc.).

**`player_backend`** — `mpris` tracks playback position via D-Bus so the app can resume from where you left off; `simple` just spawns the player process.

**`tabs`** — controls which tabs are shown and their order. Remove any name to hide that tab.

**`hint_mode`** — controls the status bar hint density:

- `full` — all context-relevant bindings; chords shown as their trigger key (`t: tab  s: sort`)
- `minimal` — only `j/k: move  t: tab  p: play`
- `none` — empty (only `?: help  q: quit` always shown on the right)

When a chord is in progress the completion hint is always shown regardless of `hint_mode`.

**Keybindings** support comma-separated values for multiple keys per action, e.g. `play = "p,enter"`. Chord sub-keys (`tab_keys`, `sort_keys`) support multi-character sequences, e.g. `subscriptions = "su"`.

**`theme`** — path to a `theme.toml` file (relative to `~/.config/yt-tui/` or absolute).

## Keybindings

### Navigation

| Key | Action |
| --- | --- |
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `h` / `←` / `Backspace` | Go back / close pane |
| `l` / `→` | Drill down / open pane |
| `Ctrl+d` / `PgDn` | Page down |
| `Ctrl+u` / `PgUp` | Page up |
| `gg` | Jump to top |
| `G` | Jump to bottom |
| `{n}G` | Jump to row n |
| `Tab` / `Shift+Tab` | Next / previous tab |

### Chord: tab switching — `t` + key

Press `t`; the status bar shows available tab keys. Press the second key to switch.

| Default key | Tab |
| --- | --- |
| `r` | Recommended |
| `s` | Subscriptions |
| `p` | Playlists |
| `S` | Search |
| `d` | Downloading |
| `l` | Local |
| `h` | History |

### Chord: sorting — `s` + key

Press `s`; the status bar shows options valid for the current context. The sort chord is not intercepted on tabs that have no sort actions (History, Downloading, Playlists list).

| Default key | Action | Recommended | Subscriptions | Channel list | Search | Local |
| --- | --- | --- | --- | --- | --- | --- |
| `d` | Sort by date | ✓ | ✓ | ✓ (latest video) | ✓ | ✓ |
| `v` | Sort by views | ✓ | ✓ | ✓ (latest video) | ✓ | ✓ |
| `n` | Sort by name | ✓ | ✓ | ✓ (latest video title) | ✓ | ✓ |
| `c` | Sort by channel | ✓ | ✓ | ✓ (channel name) | ✓ | ✓ |
| `D` | Sort by duration | ✓ | ✓ | ✓ (latest video) | ✓ | ✓ |
| `s` | Sort by subscribers | | | ✓ | | |

### Enter / DrillDown

`Enter` (configurable as `drill_down`) is context-sensitive:

| Where | Action |
| --- | --- |
| Video row (Recommended, Subscriptions, Playlists, channel drill-down) | Play / queue video |
| Channel row (Subscriptions channel pane, Search) | Open channel video list |
| Playlist list | Open playlist |
| History — video entry | Show event detail |
| History — search entry | Jump to Search tab with query pre-filled |

### Video actions

| Key | Action |
| --- | --- |
| `p` | Download and queue for playback |
| `P` | Download audio and queue for playback |
| `d` | Download video |
| `D` | Download audio |
| `x` | Delete local file / remove from queue |
| `c` | Copy video URL to clipboard |
| `b` | Hide video from recommended |
| `B` | Block channel (works from all tabs) |
| `w` | Add to Watch Later |
| `a` | Add to playlist |
| `S` | Subscribe to channel |
| `u` | Unsubscribe from channel |
| `r` | Refresh current tab |

### Search tab

| Key | Action |
| --- | --- |
| `/` | Focus / refocus search input |
| `↑` / `↓` (in input) | Navigate search history |
| `Enter` | Run search |
| `Esc` | Blur input, keep results |

### Subscriptions tab

| Key | Action |
| --- | --- |
| `m` | Toggle between All Videos and Channels view |
| `Enter` / `→` | Open channel (channel pane) or play video (video pane) |
| `h` / `Backspace` / `Esc` | Back to channel list |

### General

| Key | Action |
| --- | --- |
| `?` | Toggle help screen |
| `q` / `Ctrl+c` | Quit |

## License

MIT
