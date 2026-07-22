package overlay

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

// ── private messages ──────────────────────────────────────────────────────────

type atpPlaylistsLoadedMsg struct {
	local []domain.Playlist
	yt    []domain.YTPlaylist
}

type atpCreatedMsg struct {
	name    string
	id      string // YT playlist ID (empty for local)
	localID int64
	err     error
}

// ── AddToPlaylist ─────────────────────────────────────────────────────────────

// AddToPlaylist is the "add video to playlist" modal overlay.
type AddToPlaylist struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	video domain.Video

	localPlaylists []domain.Playlist
	ytPlaylists    []domain.YTPlaylist
	ytLoaded       bool

	sel        int
	createMode bool
	createYT   bool
	input      textinput.Model
}

// NewAddToPlaylist creates an AddToPlaylist overlay for the given video and
// immediately kicks off a background playlist load.
func NewAddToPlaylist(backend api.Backend, keys keymap.KeyMap, video domain.Video, circular bool) (AddToPlaylist, tea.Cmd) {
	ti := textinput.New()
	ti.Placeholder = "Playlist name…"
	atp := AddToPlaylist{
		backend:  backend,
		keys:     keys,
		circular: circular,
		video:    video,
		input:    ti,
	}
	return atp, atp.loadPlaylistsCmd()
}

// ── overlay.Overlay interface ─────────────────────────────────────────────────

func (atp AddToPlaylist) InterceptsInput() bool { return atp.createMode }
func (atp AddToPlaylist) WidthReduction() int   { return 0 }
func (atp AddToPlaylist) HasFocus() bool        { return true }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (atp AddToPlaylist) Init() tea.Cmd  { return nil }
func (atp AddToPlaylist) View() tea.View { return tea.NewView("") } // rendering done via Render(behind,...)

func (atp AddToPlaylist) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case atpPlaylistsLoadedMsg:
		atp.localPlaylists = m.local
		atp.ytPlaylists = m.yt
		atp.ytLoaded = true
		if atp.sel >= atp.listCount() {
			atp.sel = 0
		}

	case atpCreatedMsg:
		if m.err != nil {
			return atp, func() tea.Msg { return tuipkg.StatusMsg{Text: "create playlist: " + m.err.Error(), IsErr: true} }
		}
		// Add video to the freshly created playlist.
		var addCmd tea.Cmd
		if m.id != "" {
			id, vid := m.id, atp.video.ID
			addCmd = func() tea.Msg {
				_ = atp.backend.AddToYTPlaylist(context.Background(), id, vid)
				return nil
			}
		} else if m.localID != 0 {
			lid, vid := m.localID, atp.video.ID
			addCmd = func() tea.Msg {
				_ = atp.backend.AddToPlaylist(context.Background(), lid, vid)
				return nil
			}
		}
		return atp, tea.Batch(
			addCmd,
			func() tea.Msg { return PopOverlayMsg{} },
			func() tea.Msg { return tuipkg.StatusMsg{Text: "Created '" + m.name + "' and added video"} },
		)

	case tea.KeyPressMsg:
		return atp.handleKey(m)
	}
	return atp, nil
}

func (atp AddToPlaylist) Render(behind string, width, _ int) (string, string) {
	const boxW = 40
	const innerW = boxW - 6 // padding 2 each side + border 1 each side

	var lines []string
	if atp.createMode {
		label := "New local playlist:"
		if atp.createYT {
			label = "New YouTube playlist:"
		}
		lines = []string{
			styles.Bold.Render("Create playlist"),
			"",
			styles.Help.Render(label),
			atp.input.View(),
			"",
			styles.Help.Render("enter: confirm  esc: back"),
		}
	} else {
		lines = []string{styles.Bold.Render("Add to playlist"), ""}
		base := atp.createBase()
		if atp.ytLoaded && len(atp.ytPlaylists) > 0 {
			for i, pl := range atp.ytPlaylists {
				label := "  " + render.Truncate(pl.Title, innerW-4)
				if atp.sel == i {
					label = styles.Selected.Render("▶ " + render.Truncate(pl.Title, innerW-4))
				}
				lines = append(lines, label)
			}
		} else {
			for i, pl := range atp.localPlaylists {
				label := "  " + render.Truncate(pl.Name, innerW-4)
				if atp.sel == i {
					label = styles.Selected.Render("▶ " + render.Truncate(pl.Name, innerW-4))
				}
				lines = append(lines, label)
			}
		}
		localLabel := "  Create local playlist"
		if atp.sel == base {
			localLabel = styles.Selected.Render("▶ Create local playlist")
		}
		lines = append(lines, localLabel)
		if atp.ytLoaded {
			remoteLabel := "  Create YouTube playlist"
			if atp.sel == base+1 {
				remoteLabel = styles.Selected.Render("▶ Create YouTube playlist")
			}
			lines = append(lines, remoteLabel)
		}
		moveHint := "j/k: move  enter: confirm"
		closeHint := atp.keys.Escape.Help().Key + ": cancel"
		space := innerW - lipgloss.Width(moveHint) - lipgloss.Width(closeHint)
		if space < 1 {
			space = 1
		}
		lines = append(lines, "", styles.Help.Render(moveHint+strings.Repeat(" ", space)+closeHint))
	}

	return placeOverlayBox(behind, strings.Join(lines, "\n"), width, boxW), ""
}

// ── key handling ──────────────────────────────────────────────────────────────

func (atp AddToPlaylist) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if atp.createMode {
		return atp.handleCreateKey(msg)
	}
	return atp.handleListKey(msg)
}

func (atp AddToPlaylist) handleListKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := atp.keys
	n := atp.listCount()

	if newSel, consumed := atp.moveSelector(atp.sel, n, msg); consumed {
		atp.sel = newSel
		return atp, nil
	}
	switch {
	case key.Matches(msg, keys.Escape), key.Matches(msg, keys.Quit):
		return atp, func() tea.Msg { return PopOverlayMsg{} }

	case key.Matches(msg, keys.DrillDown):
		base := atp.createBase()
		idx := atp.sel

		if idx == base {
			atp.createMode = true
			atp.createYT = false
			atp.input.SetValue("")
			atp.input.Focus()
			return atp, textinput.Blink
		}
		if atp.ytLoaded && idx == base+1 {
			atp.createMode = true
			atp.createYT = true
			atp.input.SetValue("")
			atp.input.Focus()
			return atp, textinput.Blink
		}

		v := atp.video
		var addCmd tea.Cmd
		var label string
		if atp.ytLoaded && len(atp.ytPlaylists) > 0 && idx < len(atp.ytPlaylists) {
			pl := atp.ytPlaylists[idx]
			label = pl.Title
			plID := pl.ID
			addCmd = func() tea.Msg {
				_ = atp.backend.AddToYTPlaylist(context.Background(), plID, v.ID)
				return nil
			}
		} else {
			localIdx := idx
			if atp.ytLoaded {
				localIdx -= len(atp.ytPlaylists)
			}
			if localIdx < len(atp.localPlaylists) {
				pl := atp.localPlaylists[localIdx]
				label = pl.Name
				plID := pl.ID
				addCmd = func() tea.Msg {
					_ = atp.backend.AddToPlaylist(context.Background(), plID, v.ID)
					return nil
				}
			}
		}
		if addCmd == nil {
			return atp, nil
		}
		msg := fmt.Sprintf("Added to '%s'", label)
		return atp, tea.Batch(
			addCmd,
			func() tea.Msg { return PopOverlayMsg{} },
			func() tea.Msg { return tuipkg.StatusMsg{Text: msg} },
		)
	}
	return atp, nil
}

func (atp AddToPlaylist) handleCreateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := atp.keys
	switch {
	case key.Matches(msg, keys.Escape):
		atp.createMode = false
		atp.input.Blur()
	case key.Matches(msg, keys.DrillDown):
		name := strings.TrimSpace(atp.input.Value())
		atp.createMode = false
		atp.input.Blur()
		if name == "" {
			return atp, nil
		}
		isYT := atp.createYT
		return atp, atp.createCmd(name, isYT)
	default:
		var cmd tea.Cmd
		atp.input, cmd = atp.input.Update(msg)
		return atp, cmd
	}
	return atp, nil
}

func (atp AddToPlaylist) moveSelector(sel, n int, msg tea.KeyPressMsg) (int, bool) {
	keys := atp.keys
	switch {
	case key.Matches(msg, keys.Up):
		if sel > 0 {
			return sel - 1, true
		}
		if atp.circular && n > 0 {
			return n - 1, true
		}
		return sel, true
	case key.Matches(msg, keys.Down):
		if sel < n-1 {
			return sel + 1, true
		}
		if atp.circular {
			return 0, true
		}
		return sel, true
	}
	return sel, false
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (atp AddToPlaylist) listCount() int {
	if atp.ytLoaded && len(atp.ytPlaylists) > 0 {
		return len(atp.ytPlaylists) + 2 // playlists + "create local" + "create YT"
	}
	return len(atp.localPlaylists) + 1 // playlists + "create local"
}

func (atp AddToPlaylist) createBase() int {
	if atp.ytLoaded && len(atp.ytPlaylists) > 0 {
		return len(atp.ytPlaylists)
	}
	return len(atp.localPlaylists)
}

func (atp AddToPlaylist) loadPlaylistsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		local, _ := atp.backend.LocalPlaylists(ctx)
		yt, _ := atp.backend.YTPlaylists(ctx)
		return atpPlaylistsLoadedMsg{local: local, yt: yt}
	}
}

func (atp AddToPlaylist) createCmd(name string, isYT bool) tea.Cmd {
	if isYT {
		return func() tea.Msg {
			id, err := atp.backend.CreateYTPlaylist(context.Background(), name)
			return atpCreatedMsg{name: name, id: id, err: err}
		}
	}
	return func() tea.Msg {
		id, err := atp.backend.CreatePlaylist(context.Background(), name)
		return atpCreatedMsg{name: name, localID: id, err: err}
	}
}
