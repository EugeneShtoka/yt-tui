package tab

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

// fakeHistBackend implements historyBackend with optional func fields.
type fakeHistBackend struct {
	historyVideos         func(ctx context.Context, limit int) ([]domain.HistoryEntry, error)
	videoHistory          func(ctx context.Context, videoID string) ([]domain.HistoryEntry, error)
	deleteVideoCompletely func(ctx context.Context, videoID string) error
	allVideoPositions     func(ctx context.Context) (map[string]int64, error)
	watchedVideoIDs       func(ctx context.Context) (map[string]bool, error)
	localVideos           func(ctx context.Context) ([]domain.LocalVideo, error)
	getSubscribedChannels func(ctx context.Context) ([]domain.Channel, error)
}

func (f *fakeHistBackend) HistoryVideos(ctx context.Context, limit int) ([]domain.HistoryEntry, error) {
	if f.historyVideos != nil {
		return f.historyVideos(ctx, limit)
	}
	return nil, nil
}

func (f *fakeHistBackend) VideoHistory(ctx context.Context, videoID string) ([]domain.HistoryEntry, error) {
	if f.videoHistory != nil {
		return f.videoHistory(ctx, videoID)
	}
	return nil, nil
}

func (f *fakeHistBackend) DeleteVideoCompletely(ctx context.Context, videoID string) error {
	if f.deleteVideoCompletely != nil {
		return f.deleteVideoCompletely(ctx, videoID)
	}
	return nil
}

func (f *fakeHistBackend) AllVideoPositions(ctx context.Context) (map[string]int64, error) {
	if f.allVideoPositions != nil {
		return f.allVideoPositions(ctx)
	}
	return nil, nil
}

func (f *fakeHistBackend) WatchedVideoIDs(ctx context.Context) (map[string]bool, error) {
	if f.watchedVideoIDs != nil {
		return f.watchedVideoIDs(ctx)
	}
	return nil, nil
}

func (f *fakeHistBackend) LocalVideos(ctx context.Context) ([]domain.LocalVideo, error) {
	if f.localVideos != nil {
		return f.localVideos(ctx)
	}
	return nil, nil
}

func (f *fakeHistBackend) GetSubscribedChannels(ctx context.Context) ([]domain.Channel, error) {
	if f.getSubscribedChannels != nil {
		return f.getSubscribedChannels(ctx)
	}
	return nil, nil
}

func testKeys() keymap.KeyMap {
	kb := config.KeyBindings{
		Up:            "k",
		Down:          "j",
		Right:         "l",
		Back:          "h",
		PageUp:        "ctrl+u",
		PageDown:      "ctrl+d",
		GotoPrefix:    "g",
		GotoBottom:    "G",
		GotoLine:      "G",
		Play:          "p",
		PlayAudio:     "P",
		DrillDown:     "enter",
		Delete:        "x",
		CopyURL:       "y",
		Close:         "esc",
		SortChord:     "s",
		HideChannel:   "B",
		HideVideo:     "b",
		Refresh:       "r",
		ForceRefresh:  "R",
		Filter:        "/",
		TabChord:      "t",
		ToggleMode:    "m",
		Subscribe:     "S",
		Unsubscribe:   "u",
		RenameChannel: "A",
		TagChannel:    "T",
		AddToPlaylist: "a",
		NewPlaylist:   "n",
		VideoInfo:     "i",
		OpenLinks:     "L",
		OpenChapters:  "C",
		Download:      "d",
		DownloadAudio: "D",
		Help:          "?",
		Quit:          "q",
		SortKeys: config.SortKeys{
			Date:        "d",
			Views:       "v",
			Name:        "n",
			Channel:     "c",
			Duration:    "D",
			Subscribers: "s",
			Tags:        "t",
		},
		SubscribeKeys: config.SubscribeKeys{Remote: "r", Local: "l"},
		PlaylistKeys:  config.PlaylistKeys{Remote: "r", Local: "l"},
	}
	return keymap.Build(kb)
}

func newTestHistory(b historyBackend) History {
	return NewHistory(b, testKeys(), false)
}

func applyHistMsg(h History, msg tea.Msg) (History, tea.Cmd) {
	m, cmd := h.Update(msg)
	return m.(History), cmd
}

func TestHistoryLoadedMsg(t *testing.T) {
	entries := []domain.HistoryEntry{
		{VideoID: "v1", Title: "Video 1", EventType: "streamVideo"},
		{VideoID: "v2", Title: "Video 2", EventType: "playVideo"},
	}
	h := newTestHistory(&fakeHistBackend{})
	model, cmd := h.Update(histLoadedMsg{entries: entries})
	tab := model.(History)
	if len(tab.entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(tab.entries))
	}
	if cmd != nil {
		t.Error("expected no cmd after histLoadedMsg")
	}
}

func TestHistoryDeleteEmitsStatusMsg(t *testing.T) {
	h := newTestHistory(&fakeHistBackend{})
	_, cmd := h.Update(histDeletedMsg{title: "My Video"})
	if cmd == nil {
		t.Fatal("expected cmd from histDeletedMsg")
	}
	msg := cmd()
	sm, ok := msg.(tuipkg.StatusMsg)
	if !ok {
		t.Fatalf("expected StatusMsg, got %T", msg)
	}
	if !strings.Contains(sm.Text, "My Video") {
		t.Errorf("status text %q doesn't mention title", sm.Text)
	}
}

func TestHistoryAuxDataUpdatesRows(t *testing.T) {
	entries := []domain.HistoryEntry{{VideoID: "v1", Title: "T"}}
	h := newTestHistory(&fakeHistBackend{})
	h, _ = applyHistMsg(h, histLoadedMsg{entries: entries})
	h, _ = applyHistMsg(h, videotable.AuxDataMsg{Positions: map[string]int64{"v1": 5000}})
	if h.aux.Positions["v1"] != 5000 {
		t.Error("aux positions not stored")
	}
}

func TestHistoryDeleteKeyCallsBackend(t *testing.T) {
	var deletedID string
	fb := &fakeHistBackend{
		deleteVideoCompletely: func(_ context.Context, id string) error {
			deletedID = id
			return nil
		},
	}
	entries := []domain.HistoryEntry{{VideoID: "v1", Title: "T"}}
	h := newTestHistory(fb)
	h, _ = applyHistMsg(h, histLoadedMsg{entries: entries})
	h, _ = applyHistMsg(h, tuipkg.ContentSizeMsg{Width: 80, Height: 24})
	_, cmd := h.Update(tea.KeyPressMsg{Text: "x"})
	if cmd == nil {
		t.Fatal("expected cmd from Delete key")
	}
	cmd()
	if deletedID != "v1" {
		t.Errorf("expected deletedID=v1, got %q", deletedID)
	}
}
