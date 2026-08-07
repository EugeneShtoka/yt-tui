package tab

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// loadedLocalPlaylists builds a Playlists tab with the given local playlists
// loaded and the layout sized, ready for key-handler assertions.
func loadedLocalPlaylists(t *testing.T, pls ...domain.Playlist) Playlists {
	t.Helper()
	pl := NewPlaylists(context.Background(), &fakeBackend{}, testKeys(), false, "")
	pl, _ = updatePlaylists(pl, sized(80, 24))
	pl, _ = updatePlaylists(pl, plLocalLoadedMsg{playlists: pls})
	return pl
}

// Drilling into a local playlist (DrillDown) enters the video pane and records
// the active playlist key, then issues the local-videos fetch.
func TestPlaylistsDrillIntoLocalPlaylist(t *testing.T) {
	pl := loadedLocalPlaylists(t, domain.Playlist{ID: 7, Name: "Keepers"})

	pl, cmd := updatePlaylists(pl, tea.KeyPressMsg{Code: tea.KeyEnter})

	if pl.pane != 1 {
		t.Fatalf("drill should enter the video pane (pane 1), got pane %d", pl.pane)
	}
	if pl.activePlaylistID != "local:7" {
		t.Errorf("activePlaylistID = %q, want %q", pl.activePlaylistID, "local:7")
	}
	if cmd == nil {
		t.Error("drill into a local playlist should issue a local-videos fetch command")
	}
}

// Drilling into a YouTube playlist enters the video pane keyed by the YT id and
// issues a drilldown fetch.
func TestPlaylistsDrillIntoYTPlaylist(t *testing.T) {
	pl := NewPlaylists(context.Background(), &fakeBackend{}, testKeys(), false, "")
	pl, _ = updatePlaylists(pl, sized(80, 24))
	pl, _ = updatePlaylists(pl, plYTLoadedMsg{playlists: []domain.YTPlaylist{{ID: "PL1", Title: "Mix"}}})

	pl, cmd := updatePlaylists(pl, tea.KeyPressMsg{Code: tea.KeyEnter})

	if pl.pane != 1 {
		t.Fatalf("drill should enter the video pane, got pane %d", pl.pane)
	}
	if pl.activePlaylistID != "PL1" {
		t.Errorf("activePlaylistID = %q, want %q", pl.activePlaylistID, "PL1")
	}
	if cmd == nil {
		t.Error("drill into a YT playlist should issue a drilldown fetch command")
	}
}

// In the video pane, the Play key emits a PlayVideoMsg for the highlighted video.
func TestPlaylistsVideoPanePlayEmitsPlayVideoMsg(t *testing.T) {
	pl := loadedLocalPlaylists(t, domain.Playlist{ID: 7, Name: "Keepers"})
	pl, _ = updatePlaylists(pl, tea.KeyPressMsg{Code: tea.KeyEnter}) // drill → pane 1
	pl.vidCache["local:7"] = []domain.Video{{ID: "v1"}, {ID: "v2"}}

	_, cmd := updatePlaylists(pl, tea.KeyPressMsg{Text: "p"})

	msg := runCmd(cmd)
	play, ok := msg.(tuipkg.PlayVideoMsg)
	if !ok {
		t.Fatalf("Play should emit PlayVideoMsg, got %#v", msg)
	}
	if play.Video.ID != "v1" {
		t.Errorf("played video = %q, want v1", play.Video.ID)
	}
}

// In the video pane, the back key (Escape) returns to the playlist list pane.
func TestPlaylistsVideoPaneBackReturnsToList(t *testing.T) {
	pl := loadedLocalPlaylists(t, domain.Playlist{ID: 7, Name: "Keepers"})
	pl, _ = updatePlaylists(pl, tea.KeyPressMsg{Code: tea.KeyEnter}) // drill → pane 1
	if pl.pane != 1 {
		t.Fatalf("setup: expected to be in the video pane")
	}

	pl, _ = updatePlaylists(pl, tea.KeyPressMsg{Code: tea.KeyEscape})

	if pl.pane != 0 {
		t.Errorf("Escape in the video pane should return to the list pane, got pane %d", pl.pane)
	}
}

// refs() lists YouTube playlists before local ones, and selectedPlaylistKey
// tracks the list cursor across that unified ordering.
func TestPlaylistsRefsOrderingAndSelectedKey(t *testing.T) {
	pl := NewPlaylists(context.Background(), &fakeBackend{}, testKeys(), false, "")
	pl, _ = updatePlaylists(pl, sized(80, 24))
	pl, _ = updatePlaylists(pl, plYTLoadedMsg{playlists: []domain.YTPlaylist{{ID: "PL1", Title: "YT"}}})
	pl, _ = updatePlaylists(pl, plLocalLoadedMsg{playlists: []domain.Playlist{{ID: 7, Name: "Local"}}})

	if got := pl.plCount(); got != 2 {
		t.Fatalf("plCount = %d, want 2", got)
	}
	refs := pl.refs()
	if refs[0].key != "PL1" || refs[1].key != "local:7" {
		t.Fatalf("refs order = [%q, %q], want [PL1, local:7]", refs[0].key, refs[1].key)
	}
	if got := pl.selectedPlaylistKey(); got != "PL1" {
		t.Errorf("cursor 0 selectedPlaylistKey = %q, want PL1", got)
	}

	// Move the cursor down to the local playlist.
	pl, _ = updatePlaylists(pl, tea.KeyPressMsg{Text: "j"})
	if got := pl.selectedPlaylistKey(); got != "local:7" {
		t.Errorf("cursor 1 selectedPlaylistKey = %q, want local:7", got)
	}
}

func TestPlLocalID(t *testing.T) {
	cases := map[string]int64{
		"local:42": 42,
		"local:0":  0,
		"PL1":      0, // YT id → not local
		"WL":       0, // Watch Later → not local
		"":         0,
		"local:xx": 0, // unparseable → 0
	}
	for in, want := range cases {
		if got := plLocalID(in); got != want {
			t.Errorf("plLocalID(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestYTPlaylistSetChanged(t *testing.T) {
	a := []domain.YTPlaylist{{ID: "1"}, {ID: "2"}}
	if ytPlaylistSetChanged(a, a) {
		t.Error("identical sets should not be reported as changed")
	}
	if ytPlaylistSetChanged(a, []domain.YTPlaylist{{ID: "1"}}) != true {
		t.Error("different lengths should be reported as changed")
	}
	if !ytPlaylistSetChanged(a, []domain.YTPlaylist{{ID: "1"}, {ID: "3"}}) {
		t.Error("same length with a different id should be reported as changed")
	}
	// Order-independent: same ids in a different order is not a change.
	if ytPlaylistSetChanged(a, []domain.YTPlaylist{{ID: "2"}, {ID: "1"}}) {
		t.Error("same id set in different order should not be reported as changed")
	}
}
