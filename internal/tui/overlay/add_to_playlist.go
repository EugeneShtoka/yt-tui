package overlay

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

// ── private messages ──────────────────────────────────────────────────────────

type atpPlaylistsLoadedMsg struct {
	tuipkg.OverlayTarget
	local []domain.Playlist
	yt    []domain.YTPlaylist
}

type atpCreatedMsg struct {
	tuipkg.OverlayTarget
	name    string
	id      string // YT playlist ID (empty for local)
	localID int64
	err     error
}

// ── AddToPlaylist ─────────────────────────────────────────────────────────────

// AddToPlaylist is the "add video to playlist" modal overlay.
type AddToPlaylist struct {
	identity
	ctx      context.Context
	backend  api.PlaylistBackend
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
func NewAddToPlaylist(ctx context.Context, backend api.PlaylistBackend, keys keymap.KeyMap, video domain.Video, circular bool) (AddToPlaylist, tea.Cmd) {
	ti := textinput.New()
	ti.Placeholder = "Playlist name…"
	atp := AddToPlaylist{
		identity: newIdentity(),
		ctx:      ctx,
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
		return atp.onCreated(m)

	case tea.KeyPressMsg:
		return atp.handleKey(m)
	}
	return atp, nil
}

// onCreated handles the atpCreatedMsg result: on error it surfaces a status; on
// success it adds the video to the freshly created playlist (the add command
// reports its own success/failure so a failed add isn't shown as success) and
// pops the overlay.
func (atp AddToPlaylist) onCreated(m atpCreatedMsg) (tea.Model, tea.Cmd) {
	if m.err != nil {
		return atp, func() tea.Msg {
			return tuipkg.StatusMsg{Text: "create playlist: " + m.err.Error(), IsErr: true}
		}
	}
	return atp, tea.Batch(
		atp.addToNewPlaylistCmd(m),
		func() tea.Msg { return PopOverlayMsg{} },
	)
}

// addToNewPlaylistCmd builds the command that adds the video to the just-created
// playlist (YouTube or local), or a plain "created" status when there is no id
// to add to.
func (atp AddToPlaylist) addToNewPlaylistCmd(m atpCreatedMsg) tea.Cmd {
	name, vid := m.name, atp.video.ID
	ok := tuipkg.StatusMsg{Text: "Created '" + name + "' and added video"}
	switch {
	case m.id != "":
		id := m.id
		return func() tea.Msg {
			if err := atp.backend.AddToYTPlaylist(atp.ctx, id, vid); err != nil {
				return tuipkg.StatusMsg{Text: "add to '" + name + "': " + err.Error(), IsErr: true}
			}
			return ok
		}
	case m.localID != 0:
		lid := m.localID
		return func() tea.Msg {
			if err := atp.backend.AddToPlaylist(atp.ctx, lid, vid); err != nil {
				return tuipkg.StatusMsg{Text: "add to '" + name + "': " + err.Error(), IsErr: true}
			}
			return ok
		}
	default:
		return func() tea.Msg { return tuipkg.StatusMsg{Text: "Created '" + name + "'"} }
	}
}

func (atp AddToPlaylist) Render(behind string, width, _ int) string {
	const boxW = 40
	const innerW = boxW - render.BorderPad

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
		lines = append(lines, "", styles.Help.Render(render.JustifyEnds(moveHint, closeHint, innerW)))
	}

	return placeOverlayBox(behind, strings.Join(lines, "\n"), width, boxW)
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
		return atp.handleDrillDown()
	}
	return atp, nil
}

// handleDrillDown activates the create form when the selection is one of the
// "Create …" rows, or adds the video to the selected existing playlist.
func (atp AddToPlaylist) handleDrillDown() (tea.Model, tea.Cmd) {
	base := atp.createBase()
	idx := atp.sel

	if idx == base {
		return atp.enterCreateMode(false)
	}
	if atp.ytLoaded && idx == base+1 {
		return atp.enterCreateMode(true)
	}

	addCmd := atp.addToSelectedCmd(idx)
	if addCmd == nil {
		return atp, nil
	}
	return atp, tea.Batch(
		addCmd,
		func() tea.Msg { return PopOverlayMsg{} },
	)
}

// enterCreateMode switches the overlay into the create-playlist form.
func (atp AddToPlaylist) enterCreateMode(createYT bool) (tea.Model, tea.Cmd) {
	atp.createMode = true
	atp.createYT = createYT
	atp.input.SetValue("")
	atp.input.Focus()
	return atp, textinput.Blink
}

// addToSelectedCmd builds the command that adds the video to the playlist at
// list index idx, or nil when idx doesn't map to a real playlist.
func (atp AddToPlaylist) addToSelectedCmd(idx int) tea.Cmd {
	vid := atp.video.ID
	if atp.ytLoaded && len(atp.ytPlaylists) > 0 && idx < len(atp.ytPlaylists) {
		pl := atp.ytPlaylists[idx]
		label, plID := pl.Title, pl.ID
		return func() tea.Msg {
			if err := atp.backend.AddToYTPlaylist(atp.ctx, plID, vid); err != nil {
				return tuipkg.StatusMsg{Text: "add to '" + label + "': " + err.Error(), IsErr: true}
			}
			return tuipkg.StatusMsg{Text: "Added to '" + label + "'"}
		}
	}
	localIdx := idx
	if atp.ytLoaded {
		localIdx -= len(atp.ytPlaylists)
	}
	if localIdx < len(atp.localPlaylists) {
		pl := atp.localPlaylists[localIdx]
		label, plID := pl.Name, pl.ID
		return func() tea.Msg {
			if err := atp.backend.AddToPlaylist(atp.ctx, plID, vid); err != nil {
				return tuipkg.StatusMsg{Text: "add to '" + label + "': " + err.Error(), IsErr: true}
			}
			return tuipkg.StatusMsg{Text: "Added to '" + label + "'"}
		}
	}
	return nil
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
	return moveVertical(sel, n, msg, atp.keys, atp.circular, false)
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
	target := tuipkg.OverlayTarget{ID: atp.ID()}
	return func() tea.Msg {
		ctx := atp.ctx
		local, _ := atp.backend.LocalPlaylists(ctx)
		yt, _ := atp.backend.YTPlaylists(ctx)
		return atpPlaylistsLoadedMsg{OverlayTarget: target, local: local, yt: yt}
	}
}

func (atp AddToPlaylist) createCmd(name string, isYT bool) tea.Cmd {
	target := tuipkg.OverlayTarget{ID: atp.ID()}
	if isYT {
		return func() tea.Msg {
			id, err := atp.backend.CreateYTPlaylist(atp.ctx, name)
			return atpCreatedMsg{OverlayTarget: target, name: name, id: id, err: err}
		}
	}
	return func() tea.Msg {
		id, err := atp.backend.CreatePlaylist(atp.ctx, name)
		return atpCreatedMsg{OverlayTarget: target, name: name, localID: id, err: err}
	}
}
