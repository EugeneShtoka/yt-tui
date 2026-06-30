# yt-tui

A terminal UI for browsing and downloading YouTube videos, built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Features

- Browse recommended videos and subscriptions (requires browser cookies)
- Search YouTube
- Manage local playlists and a Watch Later queue
- Download videos or audio with [yt-dlp](https://github.com/yt-dlp/yt-dlp), with concurrent download queue
- Automatic [SponsorBlock](https://sponsor.ajay.app/) segment removal
- Feed cache so subscriptions load instantly on reopen
- Browse subscribed channels and their latest videos
- History tracking

## Requirements

- [yt-dlp](https://github.com/yt-dlp/yt-dlp)
- [mpv](https://mpv.io/) (or any player configured in `config.yaml`)
- A Chromium-family browser with an active YouTube login for subscription/recommended feeds

## Installation

```
go install github.com/EugeneShtoka/yt-tui@latest
```

Or build from source:

```
git clone https://github.com/EugeneShtoka/yt-tui
cd yt-tui
go build -o yt-tui .
```

## Configuration

On first run a default config is written to `~/.config/yt-tui/config.yaml`:

```yaml
download_dir: ~/Videos/yt-tui
browser: vivaldi+gnomekeyring   # browser used by yt-dlp for cookie auth
player: mpv
max_concurrent_downloads: 3
sponsorblock: true
sponsorblock_categories:
  - sponsor
  - selfpromo
  - interaction
audio_format: mp3
recommended_max_age_days: 7
tabs:
  - recommended
  - subscriptions
  - playlists
  - search
  - downloading
  - local
  - history
```

**`browser`** — passed to `yt-dlp --cookies-from-browser`. Any value accepted by yt-dlp works (`chrome`, `firefox`, `vivaldi`, `vivaldi+gnomekeyring`, etc.).

**`tabs`** — controls which tabs are shown and their order. Remove any tab name to hide it.

## Keybindings

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `h` / `←` | Move left / back |
| `l` / `→` | Move right |
| `Ctrl+d` / `PgDn` | Page down |
| `Ctrl+u` / `PgUp` | Page up |
| `Tab` / `Shift+Tab` | Next / previous tab |
| `F2`–`F8` | Jump to tab by position |
| `/` | Open search input |
| `s` | Download video |
| `S` | Download audio |
| `p` | Play local file |
| `d` | Delete |
| `w` | Add to Watch Later |
| `a` | Add to playlist |
| `n` | New playlist |
| `t` | Toggle view mode (Subscriptions) |
| `r` | Refresh current tab |
| `?` | Toggle help |
| `q` / `Ctrl+c` | Quit |

## License

MIT
