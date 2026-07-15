package ui

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// recordingStore embeds fakeStore (satisfying Store) and records the
// persistence calls exercised by the Cmds in commands.go. Setting err makes
// every overridden method fail, so tests can assert the error surfaces.
type recordingStore struct {
	*fakeStore
	err error

	savedYTPlaylists      []domain.YTPlaylist
	savedYTPlVideosID     string
	savedYTPlVideos       []domain.Video
	savedChanVideosID     string
	savedChanVideos       []domain.Video
	deletedChannelID      string
	savedFeedName         string
	savedFeedVideos       []domain.Video
	savedSubscribedChans  []domain.Channel
	saveSubsCalled        bool
	saveFeedAfterSubsSeen bool
}

func (r *recordingStore) SaveYTPlaylists(pls []domain.YTPlaylist) error {
	r.savedYTPlaylists = pls
	return r.err
}
func (r *recordingStore) SaveYTPlaylistVideos(id string, v []domain.Video) error {
	r.savedYTPlVideosID, r.savedYTPlVideos = id, v
	return r.err
}
func (r *recordingStore) SaveChannelVideos(id string, v []domain.Video) error {
	r.savedChanVideosID, r.savedChanVideos = id, v
	return r.err
}
func (r *recordingStore) DeleteChannelVideos(id string) error {
	r.deletedChannelID = id
	return r.err
}
func (r *recordingStore) SaveFeedCache(feed string, v []domain.Video) error {
	if r.saveSubsCalled {
		r.saveFeedAfterSubsSeen = true
	}
	r.savedFeedName, r.savedFeedVideos = feed, v
	return r.err
}
func (r *recordingStore) SaveSubscribedChannels(ch []domain.Channel) error {
	r.saveSubsCalled = true
	r.savedSubscribedChans = ch
	return r.err
}

func newRecordingStore() *recordingStore { return &recordingStore{fakeStore: &fakeStore{}} }

// assertPersistErr runs a Cmd and asserts it produced a persistErrMsg wrapping want.
func assertPersistErr(t *testing.T, cmd tea.Cmd, want error) {
	t.Helper()
	msg := cmd()
	pe, ok := msg.(persistErrMsg)
	if !ok {
		t.Fatalf("expected persistErrMsg, got %T (%v)", msg, msg)
	}
	if !errors.Is(pe.err, want) {
		t.Fatalf("persistErrMsg wraps %v, want %v", pe.err, want)
	}
}

// assertNilMsg runs a Cmd and asserts it produced no message (success path).
func assertNilMsg(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if msg := cmd(); msg != nil {
		t.Fatalf("expected nil msg on success, got %T (%v)", msg, msg)
	}
}

func TestSaveYTPlaylistsCmd(t *testing.T) {
	s := newRecordingStore()
	pls := []domain.YTPlaylist{{ID: "p1", Title: "One"}}
	assertNilMsg(t, saveYTPlaylistsCmd(s, pls))
	if len(s.savedYTPlaylists) != 1 || s.savedYTPlaylists[0].ID != "p1" {
		t.Fatalf("SaveYTPlaylists got %+v", s.savedYTPlaylists)
	}

	boom := errors.New("boom")
	assertPersistErr(t, saveYTPlaylistsCmd(&recordingStore{fakeStore: &fakeStore{}, err: boom}, pls), boom)
}

func TestSaveYTPlaylistVideosCmd(t *testing.T) {
	s := newRecordingStore()
	vids := []domain.Video{{ID: "v1"}}
	assertNilMsg(t, saveYTPlaylistVideosCmd(s, "pl", vids))
	if s.savedYTPlVideosID != "pl" || len(s.savedYTPlVideos) != 1 {
		t.Fatalf("SaveYTPlaylistVideos got id=%q vids=%+v", s.savedYTPlVideosID, s.savedYTPlVideos)
	}
}

func TestSaveChannelVideosCmd(t *testing.T) {
	s := newRecordingStore()
	vids := []domain.Video{{ID: "v1"}, {ID: "v2"}}
	assertNilMsg(t, saveChannelVideosCmd(s, "ch1", vids))
	if s.savedChanVideosID != "ch1" || len(s.savedChanVideos) != 2 {
		t.Fatalf("SaveChannelVideos got id=%q vids=%+v", s.savedChanVideosID, s.savedChanVideos)
	}

	boom := errors.New("db down")
	assertPersistErr(t, saveChannelVideosCmd(&recordingStore{fakeStore: &fakeStore{}, err: boom}, "ch1", vids), boom)
}

func TestDeleteChannelVideosCmd(t *testing.T) {
	s := newRecordingStore()
	assertNilMsg(t, deleteChannelVideosCmd(s, "chX"))
	if s.deletedChannelID != "chX" {
		t.Fatalf("DeleteChannelVideos got %q, want chX", s.deletedChannelID)
	}
}

func TestSaveFeedCacheCmd(t *testing.T) {
	s := newRecordingStore()
	vids := []domain.Video{{ID: "v1"}}
	assertNilMsg(t, saveFeedCacheCmd(s, "recommended", vids))
	if s.savedFeedName != "recommended" || len(s.savedFeedVideos) != 1 {
		t.Fatalf("SaveFeedCache got feed=%q vids=%+v", s.savedFeedName, s.savedFeedVideos)
	}
}

func TestSaveSubsAndFeedCmd(t *testing.T) {
	s := newRecordingStore()
	chans := []domain.Channel{{ID: "c1"}}
	vids := []domain.Video{{ID: "v1"}}
	assertNilMsg(t, saveSubsAndFeedCmd(s, chans, vids))
	if !s.saveSubsCalled || len(s.savedSubscribedChans) != 1 {
		t.Fatalf("SaveSubscribedChannels not recorded: %+v", s.savedSubscribedChans)
	}
	if s.savedFeedName != "recommended" || !s.saveFeedAfterSubsSeen {
		t.Fatalf("expected feed saved as 'recommended' after subs; feed=%q afterSubs=%v", s.savedFeedName, s.saveFeedAfterSubsSeen)
	}

	// On the first failure the combined Cmd must stop and surface the error.
	boom := errors.New("subs fail")
	assertPersistErr(t, saveSubsAndFeedCmd(&recordingStore{fakeStore: &fakeStore{}, err: boom}, chans, vids), boom)
}

func TestDeleteFilesCmd(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for _, name := range []string{"a.mp4", "b.mp4"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	// A missing path must not cause failure (current behavior ignores per-file errors).
	paths = append(paths, filepath.Join(dir, "gone.mp4"))

	assertNilMsg(t, deleteFilesCmd(paths))
	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("file %q still exists after deleteFilesCmd (err=%v)", p, err)
		}
	}
}
