package tab

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// activityWith returns a loaded Activity tab sitting on the given entries with
// the cursor at row 0 — the state key handlers operate on.
func activityWith(be *fakeBackend, entries []domain.ActivityEntry) Activity {
	a := NewActivity(context.Background(), be, testKeys(), false)
	m, _ := a.Update(actLoadedMsg{entries: entries})
	return m.(Activity)
}

// TestActivityDrillEmitsNavigate is table-driven over the entry types the
// Activity tab knows how to navigate into: a subscribe row jumps to the
// channel, playlist rows jump to the playlist, and unknown rows emit nothing.
func TestActivityDrillEmitsNavigate(t *testing.T) {
	cases := []struct {
		name  string
		entry domain.ActivityEntry
		want  func(tea.Msg) bool
	}{
		{
			name:  "subscribe → channel",
			entry: domain.ActivityEntry{Type: "subscribe", ChannelID: "c1", ChannelName: "Chan"},
			want: func(msg tea.Msg) bool {
				n, ok := msg.(tuipkg.NavigateToChannelMsg)
				return ok && n.ChannelID == "c1" && n.ChannelName == "Chan"
			},
		},
		{
			name:  "create_playlist → playlist",
			entry: domain.ActivityEntry{Type: "create_playlist", PlaylistLocalID: 7, PlaylistName: "Favs"},
			want: func(msg tea.Msg) bool {
				n, ok := msg.(tuipkg.NavigateToPlaylistMsg)
				return ok && n.PlaylistLocalID == 7 && n.PlaylistName == "Favs"
			},
		},
		{
			name:  "add_to_playlist → playlist",
			entry: domain.ActivityEntry{Type: "add_to_playlist", PlaylistID: "PL1", PlaylistName: "YT PL"},
			want: func(msg tea.Msg) bool {
				n, ok := msg.(tuipkg.NavigateToPlaylistMsg)
				return ok && n.PlaylistID == "PL1"
			},
		},
		{
			name:  "unknown type → no navigation",
			entry: domain.ActivityEntry{Type: "mystery"},
			want:  func(msg tea.Msg) bool { return msg == nil },
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := activityWith(&fakeBackend{}, []domain.ActivityEntry{c.entry})
			_, cmd := a.Update(tea.KeyPressMsg{Text: "enter"}) // DrillDown
			if got := runCmd(cmd); !c.want(got) {
				t.Errorf("drill on %q emitted %#v", c.entry.Type, got)
			}
		})
	}
}

// TestActivityDrillOnEmptyIsNoop: pressing DrillDown with no entries must not
// panic on the empty slice or emit a command.
func TestActivityDrillOnEmptyIsNoop(t *testing.T) {
	a := activityWith(&fakeBackend{}, nil)
	if _, cmd := a.Update(tea.KeyPressMsg{Text: "enter"}); cmd != nil {
		t.Errorf("drill with no entries should be a no-op, got %#v", runCmd(cmd))
	}
}

// TestActivityRefreshReloads: the Refresh key re-issues the load command, which
// reads the activity log from the backend.
func TestActivityRefreshReloads(t *testing.T) {
	called := false
	be := &fakeBackend{activityLog: func(context.Context, int) ([]domain.ActivityEntry, error) {
		called = true
		return []domain.ActivityEntry{{Type: "subscribe"}}, nil
	}}
	a := activityWith(be, nil)
	_, cmd := a.Update(tea.KeyPressMsg{Text: "r"}) // Refresh
	msg := runCmd(cmd)
	if !called {
		t.Fatal("Refresh must call ActivityLog")
	}
	loaded, ok := msg.(actLoadedMsg)
	if !ok || len(loaded.entries) != 1 {
		t.Fatalf("want actLoadedMsg with 1 entry, got %#v", msg)
	}
}

// TestActivityLoadCmdError: a backend failure surfaces as an error StatusMsg
// rather than an actLoadedMsg, so the list isn't wiped by a transient failure.
func TestActivityLoadCmdError(t *testing.T) {
	be := &fakeBackend{activityLog: func(context.Context, int) ([]domain.ActivityEntry, error) {
		return nil, errors.New("boom")
	}}
	a := NewActivity(context.Background(), be, testKeys(), false)
	msg := runCmd(a.actLoadCmd())
	s, ok := msg.(tuipkg.StatusMsg)
	if !ok || !s.IsErr {
		t.Fatalf("want error StatusMsg, got %#v", msg)
	}
}

// TestActivityLoadedResetsCursor: an actLoadedMsg marks the tab loaded and
// snaps the cursor to the top so a refresh doesn't strand it past the new list.
func TestActivityLoadedResetsCursor(t *testing.T) {
	a := activityWith(&fakeBackend{}, []domain.ActivityEntry{{Type: "subscribe"}, {Type: "subscribe"}})
	if !a.loaded {
		t.Error("actLoadedMsg should mark the tab loaded")
	}
	if got := a.nav.Index(); got != 0 {
		t.Errorf("cursor should reset to 0 on load, got %d", got)
	}
}
