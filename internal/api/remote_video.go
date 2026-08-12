//nolint:wrapcheck,gosec // Connect errors are already structured; pass through without re-wrapping. gosec G115: proto int32 fields are bounded in practice (durations, counts).
package api

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/protoconv"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── VideoBackend ─────────────────────────────────────────────────────────────
// HasLocalVideo, which VideoBackend also requires, is defined once in
// remote_library.go (it's the same lookup LibraryBackend needs).

func (r *Remote) VideoDetails(ctx context.Context, videoURL string) (domain.VideoDetails, error) {
	resp, err := r.vid.VideoDetails(ctx, connect.NewRequest(&v1.VideoDetailsRequest{VideoUrl: videoURL}))
	if err != nil {
		return domain.VideoDetails{}, err
	}
	return protoconv.ProtoToVideoDetails(resp.Msg.Details), nil
}

func (r *Remote) GetVideoDetailsCache(ctx context.Context, videoID string) (domain.CachedDetails, bool, error) {
	resp, err := r.vid.GetVideoDetailsCache(ctx, connect.NewRequest(&v1.GetVideoDetailsCacheRequest{VideoId: videoID}))
	if err != nil {
		return domain.CachedDetails{}, false, err
	}
	return protoconv.ProtoToCachedDetails(resp.Msg.Details), resp.Msg.Found, nil
}

func (r *Remote) VideoPosition(ctx context.Context, videoID string) (int64, bool, error) {
	resp, err := r.vid.VideoPosition(ctx, connect.NewRequest(&v1.VideoPositionRequest{VideoId: videoID}))
	if err != nil {
		// A transport/RPC failure is not the same as "no position saved" —
		// conflating the two silently restarts playback from 0:00 on any
		// daemon hiccup. (H-8)
		return 0, false, err
	}
	return resp.Msg.PositionMs, resp.Msg.Found, nil
}

func (r *Remote) AllVideoPositions(ctx context.Context) (map[string]int64, error) {
	resp, err := r.vid.AllVideoPositions(ctx, connect.NewRequest(&v1.AllVideoPositionsRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Positions, nil
}

func (r *Remote) UpsertVideo(ctx context.Context, id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error {
	_, err := r.vid.UpsertVideo(ctx, connect.NewRequest(&v1.UpsertVideoRequest{
		Id: id, Title: title, Channel: channel, ChannelId: channelID,
		Duration: int32(duration), ViewCount: viewCount, UploadDate: uploadDate, Url: url,
	}))
	return err
}

func (r *Remote) SetVideoStatus(ctx context.Context, id string, status domain.VideoStatus) error {
	_, err := r.vid.SetVideoStatus(ctx, connect.NewRequest(&v1.SetVideoStatusRequest{Id: id, Status: string(status)}))
	return err
}

func (r *Remote) SaveVideoPosition(ctx context.Context, videoID string, ms int64) error {
	_, err := r.vid.SaveVideoPosition(ctx, connect.NewRequest(&v1.SaveVideoPositionRequest{VideoId: videoID, PositionMs: ms}))
	return err
}

func (r *Remote) DeleteVideoPosition(ctx context.Context, videoID string) error {
	_, err := r.vid.DeleteVideoPosition(ctx, connect.NewRequest(&v1.DeleteVideoPositionRequest{VideoId: videoID}))
	return err
}

func (r *Remote) UpdateLastPosition(ctx context.Context, id string, ms int64) error {
	_, err := r.vid.UpdateLastPosition(ctx, connect.NewRequest(&v1.UpdateLastPositionRequest{Id: id, PositionMs: ms}))
	return err
}

func (r *Remote) SaveVideoDetailsCache(ctx context.Context, videoID, description, thumbnailURL string, subscribers int64) error {
	_, err := r.vid.SaveVideoDetailsCache(ctx, connect.NewRequest(&v1.SaveVideoDetailsCacheRequest{
		VideoId: videoID, Description: description, ThumbnailUrl: thumbnailURL, Subscribers: subscribers,
	}))
	return err
}

func (r *Remote) SaveVideoChapters(ctx context.Context, videoID string, chapters []domain.Chapter) error {
	pb := make([]*v1.Chapter, len(chapters))
	for i, c := range chapters {
		pb[i] = &v1.Chapter{Title: c.Title, OriginalStart: c.OriginalStart, OriginalEnd: c.OriginalEnd, AdjustedStart: c.AdjustedStart, AdjustedEnd: c.AdjustedEnd}
	}
	_, err := r.vid.SaveVideoChapters(ctx, connect.NewRequest(&v1.SaveVideoChaptersRequest{VideoId: videoID, Chapters: pb}))
	return err
}

func (r *Remote) SaveVideoSBSegments(ctx context.Context, videoID string, segs []domain.SBSegment) error {
	pb := make([]*v1.SBSegment, len(segs))
	for i, s := range segs {
		pb[i] = &v1.SBSegment{Start: s.Start, End: s.End}
	}
	_, err := r.vid.SaveVideoSBSegments(ctx, connect.NewRequest(&v1.SaveVideoSBSegmentsRequest{VideoId: videoID, Segments: pb}))
	return err
}

func (r *Remote) SaveVideoLinks(ctx context.Context, videoID string, links []domain.Link) error {
	pb := make([]*v1.Link, len(links))
	for i, l := range links {
		pb[i] = &v1.Link{Label: l.Label, Url: l.URL}
	}
	_, err := r.vid.SaveVideoLinks(ctx, connect.NewRequest(&v1.SaveVideoLinksRequest{VideoId: videoID, Links: pb}))
	return err
}

func (r *Remote) ClearVideoDetailsCache(ctx context.Context) error {
	_, err := r.vid.ClearVideoDetailsCache(ctx, connect.NewRequest(&v1.ClearVideoDetailsCacheRequest{}))
	return err
}

// DeleteVideoCompletely is a single RPC rather than three sequential
// round-trips (DeleteLocalVideo, DeleteVideoHistory, DeleteVideoPosition):
// the composite delete now runs server-side in one call, so a mid-sequence
// network failure can no longer leave the deletion half-applied. (M-23)
func (r *Remote) DeleteVideoCompletely(ctx context.Context, videoID string) error {
	_, err := r.vid.DeleteVideoCompletely(ctx, connect.NewRequest(&v1.DeleteVideoCompletelyRequest{VideoId: videoID}))
	return err
}

func (r *Remote) GetThumbnail(ctx context.Context, videoID, fallbackURL string) ([]byte, bool, error) {
	resp, err := r.vid.GetThumbnail(ctx, connect.NewRequest(&v1.GetThumbnailRequest{VideoId: videoID, FallbackUrl: fallbackURL}))
	if err != nil {
		return nil, false, err
	}
	return resp.Msg.Data, resp.Msg.Found, nil
}

func (r *Remote) GetTranscript(ctx context.Context, videoID, videoURL string) (string, bool, error) {
	resp, err := r.vid.GetTranscript(ctx, connect.NewRequest(&v1.GetTranscriptRequest{VideoId: videoID, VideoUrl: videoURL}))
	if err != nil {
		return "", false, err
	}
	return resp.Msg.Text, resp.Msg.Found, nil
}

func (r *Remote) EligibleThumbnailIDs(ctx context.Context) (map[string]bool, error) {
	resp, err := r.vid.EligibleThumbnailIDs(ctx, connect.NewRequest(&v1.EligibleThumbnailIDsRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Ids, nil
}

func (r *Remote) ResolveSource(ctx context.Context, videoID, fallbackURL string) (PlayableSource, error) {
	resp, err := r.vid.ResolveSource(ctx, connect.NewRequest(&v1.ResolveSourceRequest{VideoId: videoID, FallbackUrl: fallbackURL}))
	if err != nil {
		return PlayableSource{}, err
	}
	uri := resp.Msg.Uri
	if len(uri) > 0 && uri[0] == '/' {
		uri = r.baseURL + uri
	}
	return PlayableSource{URI: uri}, nil
}
