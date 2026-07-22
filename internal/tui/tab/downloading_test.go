package tab

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// fakeDlBackend implements downloadingBackend with optional func fields.
type fakeDlBackend struct {
	downloadItems  func(ctx context.Context) ([]api.DownloadItem, error)
	events         func(ctx context.Context) (<-chan api.Event, error)
	cancelDownload func(ctx context.Context, videoID string) error
	hasLocalVideo  func(ctx context.Context, videoID string) (domain.LocalVideo, bool)
}

func (f *fakeDlBackend) DownloadItems(ctx context.Context) ([]api.DownloadItem, error) {
	if f.downloadItems != nil {
		return f.downloadItems(ctx)
	}
	return nil, nil
}

func (f *fakeDlBackend) Events(ctx context.Context) (<-chan api.Event, error) {
	if f.events != nil {
		return f.events(ctx)
	}
	ch := make(chan api.Event)
	close(ch)
	return ch, nil
}

func (f *fakeDlBackend) CancelDownload(ctx context.Context, videoID string) error {
	if f.cancelDownload != nil {
		return f.cancelDownload(ctx, videoID)
	}
	return nil
}

func (f *fakeDlBackend) HasLocalVideo(ctx context.Context, videoID string) (domain.LocalVideo, bool) {
	if f.hasLocalVideo != nil {
		return f.hasLocalVideo(ctx, videoID)
	}
	return domain.LocalVideo{}, false
}

func applyDlMsg(dl Downloading, msg tea.Msg) (Downloading, tea.Cmd) {
	m, cmd := dl.Update(msg)
	return m.(Downloading), cmd
}

func TestDownloadingItemsMsg(t *testing.T) {
	items := []api.DownloadItem{{VideoID: "v1", Title: "T", Status: api.DownloadActive}}
	dl := NewDownloading(&fakeDlBackend{}, testKeys(), false)
	model, _ := dl.Update(dlItemsMsg{items: items})
	got := model.(Downloading)
	if got.loading {
		t.Error("loading should be false after dlItemsMsg")
	}
	if len(got.items) != 1 {
		t.Fatalf("want 1 item, got %d", len(got.items))
	}
}

func TestDownloadingContentSizeMsg(t *testing.T) {
	dl := NewDownloading(&fakeDlBackend{}, testKeys(), false)
	model, _ := dl.Update(tuipkg.ContentSizeMsg{Width: 80, Height: 30})
	got := model.(Downloading)
	if got.height != 30 {
		t.Errorf("want height=30, got %d", got.height)
	}
}

func TestDownloadingCancelKey(t *testing.T) {
	var cancelledID string
	fb := &fakeDlBackend{
		cancelDownload: func(_ context.Context, id string) error {
			cancelledID = id
			return nil
		},
		downloadItems: func(_ context.Context) ([]api.DownloadItem, error) {
			return nil, nil
		},
	}
	items := []api.DownloadItem{{VideoID: "v2", Status: api.DownloadActive}}
	dl := NewDownloading(fb, testKeys(), false)
	dl, _ = applyDlMsg(dl, dlItemsMsg{items: items})
	dl, _ = applyDlMsg(dl, tuipkg.ContentSizeMsg{Width: 80, Height: 24})
	_, cmd := dl.Update(tea.KeyPressMsg{Text: "x"})
	if cmd == nil {
		t.Fatal("expected cmd from Delete key")
	}
	// The Delete handler returns a tea.Batch of two cmds.
	// Execute the batch message to run sub-cmds.
	batchMsg := cmd()
	if batchedCmds, ok := batchMsg.(tea.BatchMsg); ok {
		for _, c := range batchedCmds {
			if c != nil {
				c()
			}
		}
	}
	if cancelledID != "v2" {
		t.Errorf("expected cancelledID=v2, got %q", cancelledID)
	}
}
