//nolint:wrapcheck // pass-through adapter; errors from backend/db/yt are already contextual
package api

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── HistoryBackend ───────────────────────────────────────────────────────────

func (p *InProc) History(ctx context.Context, limit int) ([]domain.HistoryEntry, error) {
	return p.db.History(ctx, limit)
}

func (p *InProc) HistoryVideos(ctx context.Context, limit int) ([]domain.HistoryEntry, error) {
	return p.db.HistoryVideos(ctx, limit)
}

func (p *InProc) VideoHistory(ctx context.Context, videoID string) ([]domain.HistoryEntry, error) {
	return p.db.VideoHistory(ctx, videoID)
}

func (p *InProc) ActivityLog(ctx context.Context, limit int) ([]domain.ActivityEntry, error) {
	return p.db.GetActivityLog(ctx, limit)
}

func (p *InProc) SearchQueries(ctx context.Context) ([]string, error) {
	return p.db.SearchQueries(ctx)
}

func (p *InProc) AddHistory(ctx context.Context, videoID, eventType, details string) error {
	return p.db.AddHistory(ctx, videoID, eventType, details)
}

func (p *InProc) LogActivity(ctx context.Context, e domain.ActivityEntry) error {
	return p.db.LogActivity(ctx, e)
}

func (p *InProc) DeleteVideoHistory(ctx context.Context, videoID string) error {
	return p.db.DeleteVideoHistory(ctx, videoID)
}

func (p *InProc) DeleteSearchHistory(ctx context.Context, query string) error {
	return p.db.DeleteSearchHistory(ctx, query)
}

func (p *InProc) ClearHistory(ctx context.Context) error {
	return p.db.ClearHistory(ctx)
}
