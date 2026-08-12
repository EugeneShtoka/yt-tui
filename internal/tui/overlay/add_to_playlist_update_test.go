package overlay

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// atpFakeBackend records playlist mutations so the overlay's command wiring can
// be asserted without a real backend. Only the methods AddToPlaylist touches
// are overridden; the rest come from the zero-value NopBackend.
type atpFakeBackend struct {
	apitest.NopBackend

	local []domain.Playlist
	yt    []domain.YTPlaylist

	addedLocalID  string
	addedLocalVid string
	addLocalErr   error

	addedYTID  string
	addedYTVid string

	createdLocalName string
	createdLocalID   string
	createdYTName    string
	createdYTID      string
	createErr        error
}

func (b *atpFakeBackend) LocalPlaylists(context.Context) ([]domain.Playlist, error) {
	return b.local, nil
}
func (b *atpFakeBackend) YTPlaylists(context.Context) ([]domain.YTPlaylist, error) {
	return b.yt, nil
}
func (b *atpFakeBackend) AddToPlaylist(_ context.Context, id string, vid string) error {
	b.addedLocalID, b.addedLocalVid = id, vid
	return b.addLocalErr
}
func (b *atpFakeBackend) AddToYTPlaylist(_ context.Context, id, vid string) error {
	b.addedYTID, b.addedYTVid = id, vid
	return nil
}
func (b *atpFakeBackend) CreatePlaylist(_ context.Context, name string) (string, error) {
	b.createdLocalName = name
	return b.createdLocalID, b.createErr
}
func (b *atpFakeBackend) CreateYTPlaylist(_ context.Context, name string) (string, error) {
	b.createdYTName = name
	return b.createdYTID, b.createErr
}

// openATP builds an AddToPlaylist and delivers the loaded-playlists message so
// the overlay is in its list state, ready for key input.
func openATP(be *atpFakeBackend, video domain.Video) AddToPlaylist {
	atp, _ := NewAddToPlaylist(context.Background(), be, atpKeys(), video, false)
	m, _ := atp.Update(atpPlaylistsLoadedMsg{local: be.local, yt: be.yt})
	return m.(AddToPlaylist)
}

// TestATPLoadPopulatesLists: the load message fills both playlist slices and
// marks YT loaded so the list renders.
func TestATPLoadPopulatesLists(t *testing.T) {
	be := &atpFakeBackend{
		local: []domain.Playlist{{ID: "local:1", Name: "Favs"}},
		yt:    []domain.YTPlaylist{{ID: "PL1", Title: "Watch Later"}},
	}
	atp := openATP(be, domain.Video{ID: "v1"})
	if !atp.ytLoaded || len(atp.localPlaylists) != 1 || len(atp.ytPlaylists) != 1 {
		t.Fatalf("load did not populate lists: %+v", atp)
	}
}

// TestATPAddToExistingLocalPlaylist: with no YT playlists, selecting the first
// row and drilling adds the video to that local playlist and pops the overlay.
func TestATPAddToExistingLocalPlaylist(t *testing.T) {
	be := &atpFakeBackend{local: []domain.Playlist{{ID: "local:42", Name: "Favs"}}}
	atp := openATP(be, domain.Video{ID: "v9"})
	atp.sel = 0

	_, cmd := atp.Update(tea.KeyPressMsg{Text: "enter"}) // DrillDown
	msgs := runBatch(cmd)

	if be.addedLocalID != "local:42" || be.addedLocalVid != "v9" {
		t.Errorf("AddToPlaylist(id,vid) = (%q,%q), want (\"local:42\",\"v9\")", be.addedLocalID, be.addedLocalVid)
	}
	if !hasPopOverlay(msgs) {
		t.Error("adding to a playlist should pop the overlay")
	}
	if !hasStatus(msgs, false) {
		t.Error("expected a success StatusMsg")
	}
}

// TestATPAddToYTPlaylist: when YT playlists are present they take the leading
// list slots; drilling the first adds to that YT playlist.
func TestATPAddToYTPlaylist(t *testing.T) {
	be := &atpFakeBackend{yt: []domain.YTPlaylist{{ID: "PLx", Title: "Music"}}}
	atp := openATP(be, domain.Video{ID: "v1"})
	atp.sel = 0

	_, cmd := atp.Update(tea.KeyPressMsg{Text: "enter"})
	runBatch(cmd)
	if be.addedYTID != "PLx" || be.addedYTVid != "v1" {
		t.Errorf("AddToYTPlaylist = (%q,%q), want (\"PLx\",\"v1\")", be.addedYTID, be.addedYTVid)
	}
}

// TestATPAddFailureReportsError: a failing backend add surfaces an error
// StatusMsg rather than a success message.
func TestATPAddFailureReportsError(t *testing.T) {
	be := &atpFakeBackend{local: []domain.Playlist{{ID: "local:1", Name: "Favs"}}, addLocalErr: errors.New("boom")}
	atp := openATP(be, domain.Video{ID: "v1"})
	atp.sel = 0
	_, cmd := atp.Update(tea.KeyPressMsg{Text: "enter"})
	if !hasStatus(runBatch(cmd), true) {
		t.Error("a failed add must produce an error StatusMsg")
	}
}

// TestATPEnterCreateModeLocal: selecting the "Create local playlist" row drills
// into the create form and switches input interception on.
func TestATPEnterCreateModeLocal(t *testing.T) {
	be := &atpFakeBackend{local: []domain.Playlist{{ID: "local:1", Name: "Favs"}}}
	atp := openATP(be, domain.Video{ID: "v1"})
	atp.sel = atp.createBase() // the "Create local playlist" row

	m, _ := atp.Update(tea.KeyPressMsg{Text: "enter"})
	got := m.(AddToPlaylist)
	if !got.createMode || got.createYT {
		t.Fatalf("expected local create mode, got createMode=%v createYT=%v", got.createMode, got.createYT)
	}
	if !got.InterceptsInput() {
		t.Error("create mode must intercept input (typing a name)")
	}
}

// TestATPCreateLocalFlow: typing a name and confirming in create mode issues a
// CreatePlaylist command; the resulting atpCreatedMsg then adds the video and
// pops the overlay.
func TestATPCreateLocalFlow(t *testing.T) {
	be := &atpFakeBackend{createdLocalID: "local:7"}
	atp := openATP(be, domain.Video{ID: "v1"})
	atp.sel = atp.createBase()
	m, _ := atp.Update(tea.KeyPressMsg{Text: "enter"}) // enter create mode
	atp = m.(AddToPlaylist)
	atp.input.SetValue("New List")

	m, cmd := atp.Update(tea.KeyPressMsg{Text: "enter"}) // confirm create
	atp = m.(AddToPlaylist)
	if atp.createMode {
		t.Error("confirming create should leave create mode")
	}
	created, ok := runMsg(cmd).(atpCreatedMsg)
	if !ok || created.name != "New List" || created.localID != "local:7" {
		t.Fatalf("want atpCreatedMsg{name:New List, localID:local:7}, got %#v", runMsg(cmd))
	}
	if be.createdLocalName != "New List" {
		t.Errorf("CreatePlaylist called with %q", be.createdLocalName)
	}

	// Feeding the created message back adds the video and pops the overlay.
	_, addCmd := atp.Update(created)
	msgs := runBatch(addCmd)
	if be.addedLocalID != "local:7" || be.addedLocalVid != "v1" {
		t.Errorf("created playlist add = (%q,%q), want (\"local:7\",\"v1\")", be.addedLocalID, be.addedLocalVid)
	}
	if !hasPopOverlay(msgs) {
		t.Error("create+add should pop the overlay")
	}
}

// TestATPCreateEmptyNameCancels: confirming create with a blank name does not
// call the backend and simply leaves create mode.
func TestATPCreateEmptyNameCancels(t *testing.T) {
	be := &atpFakeBackend{}
	atp := openATP(be, domain.Video{ID: "v1"})
	atp.sel = atp.createBase()
	m, _ := atp.Update(tea.KeyPressMsg{Text: "enter"})
	atp = m.(AddToPlaylist)
	atp.input.SetValue("   ") // whitespace only

	m, cmd := atp.Update(tea.KeyPressMsg{Text: "enter"})
	if got := m.(AddToPlaylist); got.createMode {
		t.Error("blank-name confirm should exit create mode")
	}
	if cmd != nil {
		t.Errorf("blank name must not issue a create command, got %#v", runMsg(cmd))
	}
	if be.createdLocalName != "" {
		t.Error("CreatePlaylist must not be called for a blank name")
	}
}

// TestATPCreatedErrorReportsStatus: an atpCreatedMsg carrying an error becomes
// an error StatusMsg and does not attempt an add.
func TestATPCreatedErrorReportsStatus(t *testing.T) {
	be := &atpFakeBackend{}
	atp := openATP(be, domain.Video{ID: "v1"})
	_, cmd := atp.Update(atpCreatedMsg{name: "X", err: errors.New("nope")})
	if !hasStatus(runBatch(cmd), true) {
		t.Error("a create error should surface an error StatusMsg")
	}
	if be.addedLocalID != "" || be.addedYTID != "" {
		t.Error("a failed create must not add the video anywhere")
	}
}

// TestATPEscapePopsOverlay: Escape in the list state closes the overlay.
func TestATPEscapePopsOverlay(t *testing.T) {
	atp := openATP(&atpFakeBackend{local: []domain.Playlist{{ID: "local:1", Name: "Favs"}}}, domain.Video{ID: "v1"})
	_, cmd := atp.Update(tea.KeyPressMsg{Text: "esc"})
	if _, ok := runMsg(cmd).(PopOverlayMsg); !ok {
		t.Errorf("Escape should pop the overlay, got %#v", runMsg(cmd))
	}
}

// TestATPEscapeInCreateModeReturnsToList: Escape while in create mode backs out
// to the list rather than closing the overlay.
func TestATPEscapeInCreateModeReturnsToList(t *testing.T) {
	be := &atpFakeBackend{local: []domain.Playlist{{ID: "local:1", Name: "Favs"}}}
	atp := openATP(be, domain.Video{ID: "v1"})
	atp.sel = atp.createBase()
	m, _ := atp.Update(tea.KeyPressMsg{Text: "enter"}) // into create mode
	atp = m.(AddToPlaylist)

	m, cmd := atp.Update(tea.KeyPressMsg{Text: "esc"})
	got := m.(AddToPlaylist)
	if got.createMode {
		t.Error("Escape in create mode should exit create mode")
	}
	if cmd != nil {
		if _, ok := runMsg(cmd).(PopOverlayMsg); ok {
			t.Error("Escape in create mode must not close the overlay")
		}
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func hasPopOverlay(msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(PopOverlayMsg); ok {
			return true
		}
	}
	return false
}

func hasStatus(msgs []tea.Msg, wantErr bool) bool {
	for _, m := range msgs {
		if s, ok := m.(tuipkg.StatusMsg); ok && s.IsErr == wantErr {
			return true
		}
	}
	return false
}
