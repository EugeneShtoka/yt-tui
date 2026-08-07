package tab

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// updatePlaylists mirrors updateLocal/updateSearch: drives one message through
// the Playlists Update and re-asserts the concrete type for chaining.
func updatePlaylists(p Playlists, msg tea.Msg) (Playlists, tea.Cmd) {
	m, cmd := p.Update(msg)
	return m.(Playlists), cmd
}

// ── Search: drill into a channel → results → back ─────────────────────────────

// Drilling into a channel result issues a channel-videos fetch, the async
// result populates the drill pane, and the back key (Left) pops out of it.
func TestSearchDrillIntoChannelAndBack(t *testing.T) {
	var gotChannelID string
	fb := &fakeBackend{
		search: func(_ context.Context, _ string) ([]domain.Channel, []domain.Video, error) {
			return []domain.Channel{{ID: "UC1", Name: "Chan One", URL: "http://x"}}, nil, nil
		},
		channelVideos: func(_ context.Context, _, id string) ([]domain.Video, error) {
			gotChannelID = id
			return []domain.Video{{ID: "cv1"}, {ID: "cv2"}}, nil
		},
	}
	s := NewSearch(context.Background(), fb, testKeys(), false)
	s, _ = updateSearch(s, sized(80, 24))
	// Run a real query so the input blurs and the channel pane takes focus.
	s, cmdQ := updateSearch(s, tuipkg.SearchActivateMsg{Query: "go"})
	res, ok := runCmd(cmdQ).(srchResultMsg)
	if !ok {
		t.Fatalf("want srchResultMsg from query, got %T", runCmd(cmdQ))
	}
	// Channels-only result → channel pane is focused (onVideos == false).
	s, _ = updateSearch(s, res)
	if s.onVideos {
		t.Fatal("channel pane should be focused when there are no video results")
	}

	// DrillDown (enter) into the highlighted channel.
	s, cmd := updateSearch(s, tea.KeyPressMsg{Code: tea.KeyEnter})
	if s.drill.ch == nil || s.drill.ch.ID != "UC1" {
		t.Fatalf("drill.ch = %#v, want channel UC1", s.drill.ch)
	}
	if !s.drill.loading {
		t.Error("drill should be loading right after drill-in")
	}
	msg := runCmd(cmd)
	cvMsg, ok := msg.(srchChannelVideosMsg)
	if !ok {
		t.Fatalf("want srchChannelVideosMsg from drill-in cmd, got %T", msg)
	}
	if gotChannelID != "UC1" {
		t.Errorf("backend ChannelVideos id = %q, want UC1", gotChannelID)
	}

	// Feed the async channel-videos result → drill pane populates.
	s, _ = updateSearch(s, cvMsg)
	if s.drill.loading {
		t.Error("drill.loading should clear after channel videos arrive")
	}
	if len(s.drill.videos) != 2 {
		t.Fatalf("drill.videos = %d, want 2", len(s.drill.videos))
	}

	// Back (Left / "h") pops out of the drill pane back to results.
	s, _ = updateSearch(s, tea.KeyPressMsg{Text: "h"})
	if s.drill.ch != nil {
		t.Fatalf("drill.ch should be nil after back, got %#v", s.drill.ch)
	}
	if s.drill.videos != nil {
		t.Errorf("drill.videos should be cleared after back, got %d", len(s.drill.videos))
	}
	// Results survive the back-out.
	if len(s.channels) != 1 {
		t.Errorf("channels should still be present after back, got %d", len(s.channels))
	}
}

// A failed channel-videos fetch surfaces an error status.
func TestSearchChannelVideosErrorSurfaces(t *testing.T) {
	fb := &fakeBackend{
		search: func(_ context.Context, _ string) ([]domain.Channel, []domain.Video, error) {
			return []domain.Channel{{ID: "UC1", Name: "Chan One"}}, nil, nil
		},
		channelVideos: func(_ context.Context, _, _ string) ([]domain.Video, error) {
			return nil, errors.New("boom")
		},
	}
	s := NewSearch(context.Background(), fb, testKeys(), false)
	s, _ = updateSearch(s, sized(80, 24))
	s, cmdQ := updateSearch(s, tuipkg.SearchActivateMsg{Query: "go"})
	s, _ = updateSearch(s, runCmd(cmdQ).(srchResultMsg))
	s, cmd := updateSearch(s, tea.KeyPressMsg{Code: tea.KeyEnter})
	cvMsg, ok := runCmd(cmd).(srchChannelVideosMsg)
	if !ok {
		t.Fatalf("want srchChannelVideosMsg, got %T", runCmd(cmd))
	}
	if cvMsg.err == nil {
		t.Fatal("channel-videos msg should carry the backend error")
	}

	_, cmd2 := updateSearch(s, cvMsg)
	sm, ok := runCmd(cmd2).(tuipkg.StatusMsg)
	if !ok || !sm.IsErr {
		t.Fatalf("want error StatusMsg on failed channel videos, got %#v", runCmd(cmd2))
	}
}

// ── Playlists: delete-with-confirm flow ───────────────────────────────────────

// Deleting a local playlist removes it from the list optimistically and issues
// a plDeletedMsg carrying the backend result (nil err on success).
func TestPlaylistsDeleteLocalSuccess(t *testing.T) {
	var deletedID int64
	fb := &fakeBackend{
		deletePlaylist: func(_ context.Context, id int64) error { deletedID = id; return nil },
	}
	pl := NewPlaylists(context.Background(), fb, testKeys(), false, "")
	pl, _ = updatePlaylists(pl, sized(80, 24))
	pl, _ = updatePlaylists(pl, plLocalLoadedMsg{playlists: []domain.Playlist{
		{ID: 7, Name: "Keepers"},
		{ID: 8, Name: "Trash"},
	}})
	if len(pl.localPlaylists) != 2 {
		t.Fatalf("setup: want 2 local playlists, got %d", len(pl.localPlaylists))
	}

	// Delete ("x") the highlighted (first) playlist.
	pl, cmd := updatePlaylists(pl, tea.KeyPressMsg{Text: "x"})
	if len(pl.localPlaylists) != 1 || pl.localPlaylists[0].ID != 8 {
		t.Fatalf("optimistic removal failed: %+v", pl.localPlaylists)
	}
	del, ok := runCmd(cmd).(plDeletedMsg)
	if !ok {
		t.Fatalf("want plDeletedMsg from delete, got %T", runCmd(cmd))
	}
	if del.err != nil {
		t.Errorf("delete should succeed, got err %v", del.err)
	}
	if del.isYT {
		t.Error("deleted playlist should be local, not YT")
	}
	if deletedID != 7 {
		t.Errorf("backend DeletePlaylist id = %d, want 7", deletedID)
	}
}

// A failed delete re-inserts the playlist and surfaces an error status (H-6).
func TestPlaylistsDeleteLocalFailureRestoresAndSurfaces(t *testing.T) {
	fb := &fakeBackend{
		deletePlaylist: func(_ context.Context, _ int64) error { return errors.New("nope") },
	}
	pl := NewPlaylists(context.Background(), fb, testKeys(), false, "")
	pl, _ = updatePlaylists(pl, sized(80, 24))
	pl, _ = updatePlaylists(pl, plLocalLoadedMsg{playlists: []domain.Playlist{{ID: 7, Name: "Trash"}}})

	pl, cmd := updatePlaylists(pl, tea.KeyPressMsg{Text: "x"})
	// Optimistically gone.
	if len(pl.localPlaylists) != 0 {
		t.Fatalf("want optimistic removal, got %d", len(pl.localPlaylists))
	}
	del, ok := runCmd(cmd).(plDeletedMsg)
	if !ok || del.err == nil {
		t.Fatalf("want failing plDeletedMsg, got %#v", runCmd(cmd))
	}

	// Feeding the failed delete back restores the playlist and emits an error.
	pl, cmd2 := updatePlaylists(pl, del)
	if len(pl.localPlaylists) != 1 || pl.localPlaylists[0].ID != 7 {
		t.Fatalf("failed delete should restore playlist, got %+v", pl.localPlaylists)
	}
	sm, ok := runCmd(cmd2).(tuipkg.StatusMsg)
	if !ok || !sm.IsErr {
		t.Fatalf("want error StatusMsg after failed delete, got %#v", runCmd(cmd2))
	}
}

// Watch Later cannot be deleted — the delete key short-circuits to an error
// status without issuing a backend call or a plDeletedMsg.
func TestPlaylistsDeleteWatchLaterRejected(t *testing.T) {
	pl := NewPlaylists(context.Background(), &fakeBackend{}, testKeys(), false, "")
	pl, _ = updatePlaylists(pl, sized(80, 24))
	// A non-cache YT load moves ytPlLoad to loaded, so the WL playlist is a real ref.
	pl, _ = updatePlaylists(pl, plYTLoadedMsg{playlists: []domain.YTPlaylist{
		{ID: ytWatchLaterID, Title: "Watch Later"},
	}})
	if pl.plCount() != 1 {
		t.Fatalf("setup: want 1 playlist ref, got %d", pl.plCount())
	}

	_, cmd := updatePlaylists(pl, tea.KeyPressMsg{Text: "x"})
	sm, ok := runCmd(cmd).(tuipkg.StatusMsg)
	if !ok || !sm.IsErr {
		t.Fatalf("want error StatusMsg for Watch Later delete, got %#v", runCmd(cmd))
	}
	if sm.Text != "Cannot delete Watch Later" {
		t.Errorf("status text = %q, want 'Cannot delete Watch Later'", sm.Text)
	}
}

// ── Playlists: add-to-playlist key flow ───────────────────────────────────────

// Inside a playlist's video pane, the add-to-playlist key ("a") opens the
// add-to-playlist overlay for the highlighted video.
func TestPlaylistsAddToPlaylistOpensOverlay(t *testing.T) {
	var gotID int64
	fb := &fakeBackend{
		localPlaylistVideos: func(_ context.Context, id int64) ([]domain.Video, error) {
			gotID = id
			return []domain.Video{{ID: "v1", Title: "One"}}, nil
		},
	}
	pl := NewPlaylists(context.Background(), fb, testKeys(), false, "")
	pl, _ = updatePlaylists(pl, sized(80, 24))
	pl, _ = updatePlaylists(pl, plLocalLoadedMsg{playlists: []domain.Playlist{{ID: 3, Name: "Mine"}}})

	// Drill into the playlist (enter) → pane 1, issues a local-videos fetch.
	pl, cmd := updatePlaylists(pl, tea.KeyPressMsg{Code: tea.KeyEnter})
	if pl.pane != 1 {
		t.Fatalf("drill-in should switch to video pane, pane = %d", pl.pane)
	}
	vidMsg, ok := runCmd(cmd).(plVideosLoadedMsg)
	if !ok {
		t.Fatalf("want plVideosLoadedMsg from drill-in, got %T", runCmd(cmd))
	}
	if gotID != 3 {
		t.Errorf("backend LocalPlaylistVideos id = %d, want 3", gotID)
	}

	// Feed the videos into the pane.
	pl, _ = updatePlaylists(pl, vidMsg)
	if got := len(pl.vidCache[pl.activePlaylistID]); got != 1 {
		t.Fatalf("want 1 cached video, got %d", got)
	}

	// AddToPlaylist ("a") opens the overlay for the selected video.
	_, cmd2 := updatePlaylists(pl, tea.KeyPressMsg{Text: "a"})
	open, ok := runCmd(cmd2).(tuipkg.OpenOverlayMsg)
	if !ok {
		t.Fatalf("want OpenOverlayMsg from add-to-playlist key, got %#v", runCmd(cmd2))
	}
	if open.Kind != tuipkg.OverlayAddToPlaylist {
		t.Errorf("overlay kind = %v, want OverlayAddToPlaylist", open.Kind)
	}
	if open.Video.ID != "v1" {
		t.Errorf("overlay video = %q, want v1", open.Video.ID)
	}
}

// Removing a video from a playlist that fails re-inserts it and surfaces an
// error status (mirrors the delete failure path for the in-playlist list).
func TestPlaylistsRemoveVideoFailureSurfaces(t *testing.T) {
	fb := &fakeBackend{
		localPlaylistVideos: func(_ context.Context, _ int64) ([]domain.Video, error) {
			return []domain.Video{{ID: "v1"}, {ID: "v2"}}, nil
		},
		removeFromPlaylist: func(_ context.Context, _ int64, _ string) error { return errors.New("nope") },
	}
	pl := NewPlaylists(context.Background(), fb, testKeys(), false, "")
	pl, _ = updatePlaylists(pl, sized(80, 24))
	pl, _ = updatePlaylists(pl, plLocalLoadedMsg{playlists: []domain.Playlist{{ID: 3, Name: "Mine"}}})
	pl, cmd := updatePlaylists(pl, tea.KeyPressMsg{Code: tea.KeyEnter})
	vidMsg := runCmd(cmd).(plVideosLoadedMsg)
	pl, _ = updatePlaylists(pl, vidMsg)

	key := pl.activePlaylistID
	// Delete ("x") the highlighted video → optimistic removal + plRemovedMsg.
	pl, cmd2 := updatePlaylists(pl, tea.KeyPressMsg{Text: "x"})
	if got := len(pl.vidCache[key]); got != 1 {
		t.Fatalf("optimistic video removal failed, got %d", got)
	}
	rm, ok := runCmd(cmd2).(plRemovedMsg)
	if !ok || rm.err == nil {
		t.Fatalf("want failing plRemovedMsg, got %#v", runCmd(cmd2))
	}

	// Feeding the failed removal restores the video and surfaces an error.
	pl, cmd3 := updatePlaylists(pl, rm)
	if got := len(pl.vidCache[key]); got != 2 {
		t.Fatalf("failed removal should restore video, got %d", got)
	}
	sm, ok := runCmd(cmd3).(tuipkg.StatusMsg)
	if !ok || !sm.IsErr {
		t.Fatalf("want error StatusMsg after failed removal, got %#v", runCmd(cmd3))
	}
}
