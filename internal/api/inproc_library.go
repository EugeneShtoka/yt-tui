//nolint:wrapcheck // pass-through adapter; errors from backend/db/yt are already contextual
package api

import (
	"context"
	"fmt"
	"os"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── LibraryBackend ───────────────────────────────────────────────────────────

func (p *InProc) LocalVideos(ctx context.Context) ([]domain.LocalVideo, error) {
	return p.db.LocalVideos(ctx)
}

// HasLocalVideo also satisfies VideoBackend (see inproc_video.go) — the two
// role interfaces share this lookup by design.
func (p *InProc) HasLocalVideo(ctx context.Context, videoID string) (domain.LocalVideo, bool, error) {
	return p.db.HasLocalVideo(ctx, videoID)
}

func (p *InProc) AddLocalVideo(ctx context.Context, v domain.LocalVideo) error {
	return p.db.AddLocalVideo(ctx, v)
}

func (p *InProc) DeleteLocalVideo(ctx context.Context, id string) error {
	if lv, ok, err := p.db.HasLocalVideo(ctx, id); err == nil && ok && lv.FilePath != "" {
		_ = os.Remove(lv.FilePath) // best-effort; ignore if already gone
	}
	return p.db.DeleteLocalVideo(ctx, id)
}

// DeleteAllLocalFiles is the bulk "reclaim disk space" action. It loops the
// per-video DeleteLocalVideo (file + row) over every local video, writing a
// delete history event for each — the same event the Local-tab single delete
// records — so the download/delete lifecycle stays closed for every removed
// file. Returns the count removed. A per-video error aborts and reports how
// many were removed before the failure.
func (p *InProc) DeleteAllLocalFiles(ctx context.Context) (int, error) {
	videos, err := p.db.LocalVideos(ctx)
	if err != nil {
		return 0, fmt.Errorf("DeleteAllLocalFiles: %w", err)
	}
	deleted := 0
	for i := range videos {
		id := videos[i].ID
		if err := p.DeleteLocalVideo(ctx, id); err != nil {
			return deleted, fmt.Errorf("DeleteAllLocalFiles %s: %w", id, err)
		}
		_ = p.db.AddHistory(ctx, id, "delete", "") // history is best-effort, mirrors Local-tab delete
		deleted++
	}
	return deleted, nil
}
