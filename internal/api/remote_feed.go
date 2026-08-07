//nolint:wrapcheck // Connect errors are already structured; pass through without re-wrapping.
package api

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/protoconv"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── FeedBackend ──────────────────────────────────────────────────────────────

func (r *Remote) Recommended(ctx context.Context) ([]domain.Video, error) {
	resp, err := r.feed.Recommended(ctx, connect.NewRequest(&v1.RecommendedRequest{}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) GetFeedCache(ctx context.Context, feed string) ([]domain.Video, error) {
	resp, err := r.feed.GetFeedCache(ctx, connect.NewRequest(&v1.GetFeedCacheRequest{Feed: feed}))
	if err != nil {
		return nil, err
	}
	return protoconv.ProtoToVideos(resp.Msg.Videos), nil
}

func (r *Remote) SaveFeedCache(ctx context.Context, feed string, videos []domain.Video) error {
	_, err := r.feed.SaveFeedCache(ctx, connect.NewRequest(&v1.SaveFeedCacheRequest{Feed: feed, Videos: protoconv.VideosToProto(videos)}))
	return err
}

func (r *Remote) PurgeFeedCacheMissingChannelID(ctx context.Context, feed string) error {
	_, err := r.feed.PurgeFeedCache(ctx, connect.NewRequest(&v1.PurgeFeedCacheRequest{Feed: feed}))
	return err
}

func (r *Remote) HideRecVideo(ctx context.Context, videoID string) error {
	_, err := r.feed.HideVideo(ctx, connect.NewRequest(&v1.HideVideoRequest{VideoId: videoID}))
	return err
}

func (r *Remote) HiddenRecVideoIDs(ctx context.Context) (map[string]bool, error) {
	resp, err := r.feed.HiddenVideoIDs(ctx, connect.NewRequest(&v1.HiddenVideoIDsRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Ids, nil
}

func (r *Remote) WatchedVideoIDs(ctx context.Context) (map[string]bool, error) {
	resp, err := r.feed.WatchedVideoIDs(ctx, connect.NewRequest(&v1.WatchedVideoIDsRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Ids, nil
}

func (r *Remote) ClearRecommended(ctx context.Context) error {
	_, err := r.feed.ClearRecommended(ctx, connect.NewRequest(&v1.ClearRecommendedRequest{}))
	return err
}
