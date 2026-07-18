package transport

import (
	"context"

	"connectrpc.com/connect"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

type videoHandler struct {
	b            api.Backend
	mediaBaseURL string
}

var _ backendv1connect.VideoServiceHandler = (*videoHandler)(nil)

func (h *videoHandler) VideoDetails(ctx context.Context, req *connect.Request[v1.VideoDetailsRequest]) (*connect.Response[v1.VideoDetailsResponse], error) {
	vd, err := h.b.VideoDetails(ctx, req.Msg.VideoUrl)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.VideoDetailsResponse{Details: videoDetailsToProto(vd)}), nil
}

func (h *videoHandler) GetVideoDetailsCache(ctx context.Context, req *connect.Request[v1.GetVideoDetailsCacheRequest]) (*connect.Response[v1.GetVideoDetailsCacheResponse], error) {
	cd, found, err := h.b.GetVideoDetailsCache(ctx, req.Msg.VideoId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.GetVideoDetailsCacheResponse{
		Details: cachedDetailsToProto(cd),
		Found:   found,
	}), nil
}

func (h *videoHandler) SaveVideoDetailsCache(ctx context.Context, req *connect.Request[v1.SaveVideoDetailsCacheRequest]) (*connect.Response[v1.SaveVideoDetailsCacheResponse], error) {
	if err := h.b.SaveVideoDetailsCache(ctx, req.Msg.VideoId, req.Msg.Description, req.Msg.ThumbnailUrl, req.Msg.Subscribers); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SaveVideoDetailsCacheResponse{}), nil
}

func (h *videoHandler) ClearVideoDetailsCache(ctx context.Context, _ *connect.Request[v1.ClearVideoDetailsCacheRequest]) (*connect.Response[v1.ClearVideoDetailsCacheResponse], error) {
	if err := h.b.ClearVideoDetailsCache(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.ClearVideoDetailsCacheResponse{}), nil
}

func (h *videoHandler) SaveVideoChapters(ctx context.Context, req *connect.Request[v1.SaveVideoChaptersRequest]) (*connect.Response[v1.SaveVideoChaptersResponse], error) {
	chapters := make([]domain.Chapter, len(req.Msg.Chapters))
	for i, c := range req.Msg.Chapters {
		chapters[i] = domain.Chapter{
			Title:         c.Title,
			OriginalStart: c.OriginalStart,
			OriginalEnd:   c.OriginalEnd,
			AdjustedStart: c.AdjustedStart,
			AdjustedEnd:   c.AdjustedEnd,
		}
	}
	if err := h.b.SaveVideoChapters(ctx, req.Msg.VideoId, chapters); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SaveVideoChaptersResponse{}), nil
}

func (h *videoHandler) SaveVideoSBSegments(ctx context.Context, req *connect.Request[v1.SaveVideoSBSegmentsRequest]) (*connect.Response[v1.SaveVideoSBSegmentsResponse], error) {
	segs := make([]domain.SBSegment, len(req.Msg.Segments))
	for i, s := range req.Msg.Segments {
		segs[i] = domain.SBSegment{Start: s.Start, End: s.End}
	}
	if err := h.b.SaveVideoSBSegments(ctx, req.Msg.VideoId, segs); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SaveVideoSBSegmentsResponse{}), nil
}

func (h *videoHandler) SaveVideoLinks(ctx context.Context, req *connect.Request[v1.SaveVideoLinksRequest]) (*connect.Response[v1.SaveVideoLinksResponse], error) {
	links := make([]domain.Link, len(req.Msg.Links))
	for i, l := range req.Msg.Links {
		links[i] = domain.Link{Label: l.Label, URL: l.Url}
	}
	if err := h.b.SaveVideoLinks(ctx, req.Msg.VideoId, links); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SaveVideoLinksResponse{}), nil
}

func (h *videoHandler) UpsertVideo(ctx context.Context, req *connect.Request[v1.UpsertVideoRequest]) (*connect.Response[v1.UpsertVideoResponse], error) {
	if err := h.b.UpsertVideo(ctx, req.Msg.Id, req.Msg.Title, req.Msg.Channel, req.Msg.ChannelId, int(req.Msg.Duration), req.Msg.ViewCount, req.Msg.UploadDate, req.Msg.Url); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.UpsertVideoResponse{}), nil
}

func (h *videoHandler) SetVideoStatus(ctx context.Context, req *connect.Request[v1.SetVideoStatusRequest]) (*connect.Response[v1.SetVideoStatusResponse], error) {
	if err := h.b.SetVideoStatus(ctx, req.Msg.Id, domain.VideoStatus(req.Msg.Status)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SetVideoStatusResponse{}), nil
}

func (h *videoHandler) VideoPosition(ctx context.Context, req *connect.Request[v1.VideoPositionRequest]) (*connect.Response[v1.VideoPositionResponse], error) {
	pos, found := h.b.VideoPosition(ctx, req.Msg.VideoId)
	return connect.NewResponse(&v1.VideoPositionResponse{PositionMs: pos, Found: found}), nil
}

func (h *videoHandler) AllVideoPositions(ctx context.Context, _ *connect.Request[v1.AllVideoPositionsRequest]) (*connect.Response[v1.AllVideoPositionsResponse], error) {
	positions, err := h.b.AllVideoPositions(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.AllVideoPositionsResponse{Positions: positions}), nil
}

func (h *videoHandler) SaveVideoPosition(ctx context.Context, req *connect.Request[v1.SaveVideoPositionRequest]) (*connect.Response[v1.SaveVideoPositionResponse], error) {
	if err := h.b.SaveVideoPosition(ctx, req.Msg.VideoId, req.Msg.PositionMs); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SaveVideoPositionResponse{}), nil
}

func (h *videoHandler) DeleteVideoPosition(ctx context.Context, req *connect.Request[v1.DeleteVideoPositionRequest]) (*connect.Response[v1.DeleteVideoPositionResponse], error) {
	if err := h.b.DeleteVideoPosition(ctx, req.Msg.VideoId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.DeleteVideoPositionResponse{}), nil
}

func (h *videoHandler) UpdateLastPosition(ctx context.Context, req *connect.Request[v1.UpdateLastPositionRequest]) (*connect.Response[v1.UpdateLastPositionResponse], error) {
	if err := h.b.UpdateLastPosition(ctx, req.Msg.Id, req.Msg.PositionMs); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.UpdateLastPositionResponse{}), nil
}

func (h *videoHandler) ReportPosition(ctx context.Context, req *connect.Request[v1.ReportPositionRequest]) (*connect.Response[v1.ReportPositionResponse], error) {
	if err := h.b.ReportPosition(ctx, req.Msg.VideoId, req.Msg.PositionMs); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.ReportPositionResponse{}), nil
}

func (h *videoHandler) ResolveSource(ctx context.Context, req *connect.Request[v1.ResolveSourceRequest]) (*connect.Response[v1.ResolveSourceResponse], error) {
	if lv, ok := h.b.HasLocalVideo(ctx, req.Msg.VideoId); ok && lv.FilePath != "" {
		uri := h.mediaBaseURL + "/media/" + req.Msg.VideoId
		return connect.NewResponse(&v1.ResolveSourceResponse{Uri: uri}), nil
	}
	return connect.NewResponse(&v1.ResolveSourceResponse{Uri: req.Msg.FallbackUrl}), nil
}
