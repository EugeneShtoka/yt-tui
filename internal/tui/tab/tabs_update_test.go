package tab

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// runCmd executes a tea.Cmd and returns its message (nil-safe).
func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func sized(w, h int) tuipkg.ContentSizeMsg { return tuipkg.ContentSizeMsg{Width: w, Height: h} }

// ── Local: delete flow (H-6 regression) ───────────────────────────────────────

func TestLocalDeleteSuccessReloadsWithStatus(t *testing.T) {
	var deletedID string
	fb := &fakeBackend{
		deleteLocalVideo: func(_ context.Context, id string) error { deletedID = id; return nil },
		localVideos:      func(context.Context) ([]domain.LocalVideo, error) { return nil, nil },
	}
	lc := NewLocal(context.Background(), fb, testKeys(), false, "")
	lc, _ = updateLocal(lc, sized(80, 24))
	lc, _ = updateLocal(lc, localLoadedMsg{videos: []domain.LocalVideo{{ID: "v1", Title: "My Vid"}}})

	_, cmd := lc.Update(tea.KeyPressMsg{Text: "x"}) // Delete
	msg := runCmd(cmd)
	loaded, ok := msg.(localLoadedMsg)
	if !ok {
		t.Fatalf("want localLoadedMsg after delete, got %T", msg)
	}
	if !strings.Contains(loaded.status, "Deleted") {
		t.Errorf("status %q missing 'Deleted'", loaded.status)
	}
	if deletedID != "v1" {
		t.Errorf("backend delete id = %q, want v1", deletedID)
	}
}

func TestLocalDeleteFailureSurfacesError(t *testing.T) {
	fb := &fakeBackend{deleteLocalVideo: func(context.Context, string) error { return errors.New("nope") }}
	lc := NewLocal(context.Background(), fb, testKeys(), false, "")
	lc, _ = updateLocal(lc, sized(80, 24))
	lc, _ = updateLocal(lc, localLoadedMsg{videos: []domain.LocalVideo{{ID: "v1", Title: "My Vid"}}})

	_, cmd := lc.Update(tea.KeyPressMsg{Text: "x"})
	sm, ok := runCmd(cmd).(tuipkg.StatusMsg)
	if !ok || !sm.IsErr {
		t.Fatalf("want error StatusMsg on failed delete, got %#v", runCmd(cmd))
	}
}

func updateLocal(l Local, msg tea.Msg) (Local, tea.Cmd) {
	m, cmd := l.Update(msg)
	return m.(Local), cmd
}

// Phase 11: the s-z sort chord orders the Local tab by file size (largest
// first), and the human-readable size renders in the table.
func TestLocalSortBySizeChord(t *testing.T) {
	lc := NewLocal(context.Background(), &fakeBackend{}, testKeys(), false, "")
	lc, _ = updateLocal(lc, sized(120, 24))
	lc, _ = updateLocal(lc, localLoadedMsg{videos: []domain.LocalVideo{
		{ID: "small", Title: "Small", FileSize: 5 << 20},     // 5 MiB
		{ID: "big", Title: "Big", FileSize: 2 << 30},         // 2 GiB
		{ID: "medium", Title: "Medium", FileSize: 700 << 20}, // 700 MiB
	}})

	// s then z → sort by size desc. (The size-glyph formatting itself is pinned
	// in render's format tests; this test asserts only on the resulting order so
	// it stays decoupled from render.Size's exact output.)
	lc, _ = updateLocal(lc, tea.KeyPressMsg{Text: "s"})
	lc, _ = updateLocal(lc, tea.KeyPressMsg{Text: "z"})

	got := []string{lc.videos[0].ID, lc.videos[1].ID, lc.videos[2].ID}
	want := []string{"big", "medium", "small"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("size-sorted order = %v, want %v", got, want)
		}
	}
}

// ── Loaded-message population across the remaining full-interface tabs ─────────

func TestActivityLoadedPopulatesEntries(t *testing.T) {
	act := NewActivity(context.Background(), &fakeBackend{}, testKeys(), false)
	m, _ := act.Update(actLoadedMsg{entries: []domain.ActivityEntry{{Type: "subscribe"}}})
	got := m.(Activity)
	if !got.loaded || len(got.entries) != 1 {
		t.Fatalf("loaded=%v entries=%d, want true/1", got.loaded, len(got.entries))
	}
}

func TestPlaylistsLocalLoadedPopulates(t *testing.T) {
	pl := NewPlaylists(context.Background(), &fakeBackend{}, testKeys(), false, "")
	m, _ := pl.Update(plLocalLoadedMsg{playlists: []domain.Playlist{{ID: 1, Name: "P"}}})
	if got := len(m.(Playlists).localPlaylists); got != 1 {
		t.Fatalf("want 1 local playlist, got %d", got)
	}
}

// A failed delete must surface an error status (H-6).
func TestPlaylistsDeletedErrorSurfaces(t *testing.T) {
	pl := NewPlaylists(context.Background(), &fakeBackend{}, testKeys(), false, "")
	_, cmd := pl.Update(plDeletedMsg{err: errors.New("boom")})
	sm, ok := runCmd(cmd).(tuipkg.StatusMsg)
	if !ok || !sm.IsErr {
		t.Fatalf("want error StatusMsg, got %#v", runCmd(cmd))
	}
}

func TestChannelsLoadedClearsLoading(t *testing.T) {
	ch := NewChannels(context.Background(), &fakeBackend{}, testKeys(), false, ChannelsOpts{LatestCount: 30, RefreshMinutes: 60, View: "subscribed", HideStale: false, StaleDays: 30})
	m, _ := ch.Update(chsLoadedMsg{chans: []domain.Channel{{ID: "UC1", Name: "C"}}, latest: map[string]domain.Video{}})
	got := m.(Channels)
	if got.loading {
		t.Error("loading should be false after chsLoadedMsg")
	}
	if n := len(got.subs.Channels()); n != 1 {
		t.Fatalf("want 1 channel, got %d", n)
	}
}

func TestTagsDataPopulates(t *testing.T) {
	tg := NewTags(context.Background(), &fakeBackend{}, testKeys(), false, TagsOpts{Mode: "subscribed", StaleDays: 30})
	m, _ := tg.Update(tagsDataMsg{
		chans:     []domain.Channel{{ID: "UC1", Tags: []string{"go"}}},
		subVideos: []domain.Video{{ID: "v1"}},
	})
	got := m.(Tags)
	if got.loading || len(got.subVideos) != 1 {
		t.Fatalf("loading=%v subVideos=%d, want false/1", got.loading, len(got.subVideos))
	}
}

func TestSearchResultPopulates(t *testing.T) {
	s := NewSearch(context.Background(), &fakeBackend{}, testKeys(), false)
	s, _ = updateSearch(s, sized(80, 24))
	m, _ := s.Update(srchResultMsg{
		query:    "go",
		channels: []domain.Channel{{ID: "UC1"}},
		videos:   []domain.Video{{ID: "v1"}, {ID: "v2"}},
	})
	got := m.(Search)
	if got.loading {
		t.Error("loading should be false after srchResultMsg")
	}
	if len(got.videos) != 2 || len(got.channels) != 1 {
		t.Fatalf("videos=%d channels=%d, want 2/1", len(got.videos), len(got.channels))
	}
}

func updateSearch(s Search, msg tea.Msg) (Search, tea.Cmd) {
	m, cmd := s.Update(msg)
	return m.(Search), cmd
}

func TestChannelStaleThrottle(t *testing.T) {
	ch := NewChannels(context.Background(), &fakeBackend{}, testKeys(), false, ChannelsOpts{LatestCount: 3, RefreshMinutes: 60, View: "subscribed", HideStale: false, StaleDays: 30})
	// Never fetched → stale.
	if !ch.channelStale("UC1") {
		t.Error("unfetched channel should be stale")
	}
	// Just fetched → not stale.
	ch.lastRefresh["UC1"] = time.Now()
	if ch.channelStale("UC1") {
		t.Error("recently fetched channel should not be stale")
	}
	// Fetched long ago → stale.
	ch.lastRefresh["UC1"] = time.Now().Add(-2 * time.Hour)
	if !ch.channelStale("UC1") {
		t.Error("channel fetched 2h ago should be stale (interval 60m)")
	}
	// Zero interval disables throttling → always stale.
	ch.refreshInterval = 0
	ch.lastRefresh["UC1"] = time.Now()
	if !ch.channelStale("UC1") {
		t.Error("zero interval should always be stale")
	}
}

func TestChannelCachedMsgSkipsRefreshWhenFresh(t *testing.T) {
	ch := NewChannels(context.Background(), &fakeBackend{}, testKeys(), false, ChannelsOpts{LatestCount: 3, RefreshMinutes: 60, View: "subscribed", HideStale: false, StaleDays: 30})
	ch.activeChID = "UC1"
	ch.lastRefresh["UC1"] = time.Now() // fresh

	_, cmd := ch.Update(chVideosCachedMsg{channelID: "UC1", videos: []domain.Video{{ID: "v1"}}})
	if cmd != nil {
		t.Error("fresh channel must not trigger an auto-refresh command")
	}

	// Stale channel triggers a refresh.
	ch2 := NewChannels(context.Background(), &fakeBackend{}, testKeys(), false, ChannelsOpts{LatestCount: 3, RefreshMinutes: 60, View: "subscribed", HideStale: false, StaleDays: 30})
	ch2.activeChID = "UC2"
	_, cmd2 := ch2.Update(chVideosCachedMsg{channelID: "UC2", videos: []domain.Video{{ID: "v1"}}})
	if cmd2 == nil {
		t.Error("stale channel must trigger an auto-refresh command")
	}
}
