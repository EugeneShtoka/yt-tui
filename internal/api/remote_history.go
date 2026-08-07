//nolint:wrapcheck,gosec // Connect errors are already structured; pass through without re-wrapping. gosec G115: proto int32 fields are bounded in practice (durations, counts).
package api

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/protoconv"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── HistoryBackend ───────────────────────────────────────────────────────────

func (r *Remote) History(ctx context.Context, limit int) ([]domain.HistoryEntry, error) {
	resp, err := r.history.History(ctx, connect.NewRequest(&v1.HistoryRequest{Limit: int32(limit)}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToHistoryEntries(resp.Msg.Entries), nil
}

func (r *Remote) HistoryVideos(ctx context.Context, limit int) ([]domain.HistoryEntry, error) {
	resp, err := r.history.HistoryVideos(ctx, connect.NewRequest(&v1.HistoryVideosRequest{Limit: int32(limit)}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToHistoryEntries(resp.Msg.Entries), nil
}

func (r *Remote) VideoHistory(ctx context.Context, videoID string) ([]domain.HistoryEntry, error) {
	resp, err := r.history.VideoHistory(ctx, connect.NewRequest(&v1.VideoHistoryRequest{VideoId: videoID}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToHistoryEntries(resp.Msg.Entries), nil
}

func (r *Remote) ActivityLog(ctx context.Context, limit int) ([]domain.ActivityEntry, error) {
	resp, err := r.history.ActivityLog(ctx, connect.NewRequest(&v1.ActivityLogRequest{Limit: int32(limit)}))
	if err != nil {
		return nil, err
	}
	out := make([]domain.ActivityEntry, len(resp.Msg.Entries))
	for i, pb := range resp.Msg.Entries {
		out[i] = protoconv.ProtoToActivityEntry(pb)
	}
	return out, nil
}

func (r *Remote) SearchQueries(ctx context.Context) ([]string, error) {
	resp, err := r.history.SearchQueries(ctx, connect.NewRequest(&v1.SearchQueriesRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Queries, nil
}

func (r *Remote) AddHistory(ctx context.Context, videoID, eventType, details string) error {
	_, err := r.history.AddHistory(ctx, connect.NewRequest(&v1.AddHistoryRequest{VideoId: videoID, EventType: eventType, Details: details}))
	return err
}

func (r *Remote) LogActivity(ctx context.Context, e domain.ActivityEntry) error {
	_, err := r.history.LogActivity(ctx, connect.NewRequest(&v1.LogActivityRequest{Entry: protoconv.ActivityEntryToProto(e)}))
	return err
}

func (r *Remote) DeleteVideoHistory(ctx context.Context, videoID string) error {
	_, err := r.history.DeleteVideoHistory(ctx, connect.NewRequest(&v1.DeleteVideoHistoryRequest{VideoId: videoID}))
	return err
}

func (r *Remote) DeleteSearchHistory(ctx context.Context, query string) error {
	_, err := r.history.DeleteSearchHistory(ctx, connect.NewRequest(&v1.DeleteSearchHistoryRequest{Query: query}))
	return err
}

func (r *Remote) ClearHistory(ctx context.Context) error {
	_, err := r.history.ClearHistory(ctx, connect.NewRequest(&v1.ClearHistoryRequest{}))
	return err
}
