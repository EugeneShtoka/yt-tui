package transport

import (
	"context"

	"connectrpc.com/connect"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/protoconv"
	"github.com/EugeneShtoka/yt-tui/internal/backend/media"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

type videoHandler struct {
	b     api.VideoBackend
	token string
}

var _ backendv1connect.VideoServiceHandler = (*videoHandler)(nil)

func (h *videoHandler) VideoDetails(ctx context.Context, req *connect.Request[v1.VideoDetailsRequest]) (*connect.Response[v1.VideoDetailsResponse], error) {
	vd, err := h.b.VideoDetails(ctx, req.Msg.GetVideoUrl())
	if err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.VideoDetailsResponse{Details: protoconv.VideoDetailsToProto(vd)}), nil
}

func (h *videoHandler) GetVideoDetailsCache(ctx context.Context, req *connect.Request[v1.GetVideoDetailsCacheRequest]) (*connect.Response[v1.GetVideoDetailsCacheResponse], error) {
	cd, found, err := h.b.GetVideoDetailsCache(ctx, req.Msg.GetVideoId())
	if err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.GetVideoDetailsCacheResponse{
		Details: protoconv.CachedDetailsToProto(cd),
		Found:   found,
	}), nil
}

func (h *videoHandler) SaveVideoDetailsCache(ctx context.Context, req *connect.Request[v1.SaveVideoDetailsCacheRequest]) (*connect.Response[v1.SaveVideoDetailsCacheResponse], error) {
	if err := h.b.SaveVideoDetailsCache(ctx, req.Msg.GetVideoId(), req.Msg.GetDescription(), req.Msg.GetThumbnailUrl(), req.Msg.GetSubscribers()); err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.SaveVideoDetailsCacheResponse{}), nil
}

func (h *videoHandler) ClearVideoDetailsCache(ctx context.Context, _ *connect.Request[v1.ClearVideoDetailsCacheRequest]) (*connect.Response[v1.ClearVideoDetailsCacheResponse], error) {
	if err := h.b.ClearVideoDetailsCache(ctx); err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.ClearVideoDetailsCacheResponse{}), nil
}

func (h *videoHandler) SaveVideoChapters(ctx context.Context, req *connect.Request[v1.SaveVideoChaptersRequest]) (*connect.Response[v1.SaveVideoChaptersResponse], error) {
	chapters := make([]domain.Chapter, len(req.Msg.GetChapters()))
	for i, c := range req.Msg.GetChapters() {
		chapters[i] = domain.Chapter{
			Title:         c.GetTitle(),
			OriginalStart: c.GetOriginalStart(),
			OriginalEnd:   c.GetOriginalEnd(),
			AdjustedStart: c.GetAdjustedStart(),
			AdjustedEnd:   c.GetAdjustedEnd(),
		}
	}
	if err := h.b.SaveVideoChapters(ctx, req.Msg.GetVideoId(), chapters); err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.SaveVideoChaptersResponse{}), nil
}

func (h *videoHandler) SaveVideoSBSegments(ctx context.Context, req *connect.Request[v1.SaveVideoSBSegmentsRequest]) (*connect.Response[v1.SaveVideoSBSegmentsResponse], error) {
	segs := make([]domain.SBSegment, len(req.Msg.GetSegments()))
	for i, s := range req.Msg.GetSegments() {
		segs[i] = domain.SBSegment{Start: s.GetStart(), End: s.GetEnd()}
	}
	if err := h.b.SaveVideoSBSegments(ctx, req.Msg.GetVideoId(), segs); err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.SaveVideoSBSegmentsResponse{}), nil
}

func (h *videoHandler) SaveVideoLinks(ctx context.Context, req *connect.Request[v1.SaveVideoLinksRequest]) (*connect.Response[v1.SaveVideoLinksResponse], error) {
	links := make([]domain.Link, len(req.Msg.GetLinks()))
	for i, l := range req.Msg.GetLinks() {
		links[i] = domain.Link{Label: l.GetLabel(), URL: l.GetUrl()}
	}
	if err := h.b.SaveVideoLinks(ctx, req.Msg.GetVideoId(), links); err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.SaveVideoLinksResponse{}), nil
}

func (h *videoHandler) UpsertVideo(ctx context.Context, req *connect.Request[v1.UpsertVideoRequest]) (*connect.Response[v1.UpsertVideoResponse], error) {
	if err := h.b.UpsertVideo(ctx, req.Msg.GetId(), req.Msg.GetTitle(), req.Msg.GetChannel(), req.Msg.GetChannelId(), int(req.Msg.GetDuration()), req.Msg.GetViewCount(), req.Msg.GetUploadDate(), req.Msg.GetUrl()); err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.UpsertVideoResponse{}), nil
}

func (h *videoHandler) SetVideoStatus(ctx context.Context, req *connect.Request[v1.SetVideoStatusRequest]) (*connect.Response[v1.SetVideoStatusResponse], error) {
	if err := h.b.SetVideoStatus(ctx, req.Msg.GetId(), domain.VideoStatus(req.Msg.GetStatus())); err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.SetVideoStatusResponse{}), nil
}

func (h *videoHandler) VideoPosition(ctx context.Context, req *connect.Request[v1.VideoPositionRequest]) (*connect.Response[v1.VideoPositionResponse], error) {
	pos, found, err := h.b.VideoPosition(ctx, req.Msg.GetVideoId())
	if err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.VideoPositionResponse{PositionMs: pos, Found: found}), nil
}

func (h *videoHandler) AllVideoPositions(ctx context.Context, _ *connect.Request[v1.AllVideoPositionsRequest]) (*connect.Response[v1.AllVideoPositionsResponse], error) {
	positions, err := h.b.AllVideoPositions(ctx)
	if err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.AllVideoPositionsResponse{Positions: positions}), nil
}

func (h *videoHandler) SaveVideoPosition(ctx context.Context, req *connect.Request[v1.SaveVideoPositionRequest]) (*connect.Response[v1.SaveVideoPositionResponse], error) {
	if err := h.b.SaveVideoPosition(ctx, req.Msg.GetVideoId(), req.Msg.GetPositionMs()); err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.SaveVideoPositionResponse{}), nil
}

func (h *videoHandler) DeleteVideoPosition(ctx context.Context, req *connect.Request[v1.DeleteVideoPositionRequest]) (*connect.Response[v1.DeleteVideoPositionResponse], error) {
	if err := h.b.DeleteVideoPosition(ctx, req.Msg.GetVideoId()); err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.DeleteVideoPositionResponse{}), nil
}

func (h *videoHandler) UpdateLastPosition(ctx context.Context, req *connect.Request[v1.UpdateLastPositionRequest]) (*connect.Response[v1.UpdateLastPositionResponse], error) {
	if err := h.b.UpdateLastPosition(ctx, req.Msg.GetId(), req.Msg.GetPositionMs()); err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.UpdateLastPositionResponse{}), nil
}

func (h *videoHandler) ResolveSource(ctx context.Context, req *connect.Request[v1.ResolveSourceRequest]) (*connect.Response[v1.ResolveSourceResponse], error) {
	lv, ok, err := h.b.HasLocalVideo(ctx, req.Msg.GetVideoId())
	if err != nil {
		return nil, rpcErr(err)
	}
	if ok && lv.FilePath != "" {
		uri := media.MediaURL(h.token, req.Msg.GetVideoId())
		return connect.NewResponse(&v1.ResolveSourceResponse{Uri: uri}), nil
	}
	return connect.NewResponse(&v1.ResolveSourceResponse{Uri: req.Msg.GetFallbackUrl()}), nil
}

func (h *videoHandler) GetThumbnail(ctx context.Context, req *connect.Request[v1.GetThumbnailRequest]) (*connect.Response[v1.GetThumbnailResponse], error) {
	data, found, err := h.b.GetThumbnail(ctx, req.Msg.GetVideoId(), req.Msg.GetFallbackUrl())
	if err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.GetThumbnailResponse{Data: data, Found: found}), nil
}

func (h *videoHandler) EligibleThumbnailIDs(ctx context.Context, _ *connect.Request[v1.EligibleThumbnailIDsRequest]) (*connect.Response[v1.EligibleThumbnailIDsResponse], error) {
	ids, err := h.b.EligibleThumbnailIDs(ctx)
	if err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.EligibleThumbnailIDsResponse{Ids: ids}), nil
}

func (h *videoHandler) GetTranscript(ctx context.Context, req *connect.Request[v1.GetTranscriptRequest]) (*connect.Response[v1.GetTranscriptResponse], error) {
	text, found, err := h.b.GetTranscript(ctx, req.Msg.GetVideoId(), req.Msg.GetVideoUrl())
	if err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.GetTranscriptResponse{Text: text, Found: found}), nil
}

func (h *videoHandler) DeleteVideoCompletely(ctx context.Context, req *connect.Request[v1.DeleteVideoCompletelyRequest]) (*connect.Response[v1.DeleteVideoCompletelyResponse], error) {
	if err := h.b.DeleteVideoCompletely(ctx, req.Msg.GetVideoId()); err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.DeleteVideoCompletelyResponse{}), nil
}
