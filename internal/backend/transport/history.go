package transport

import (
	"context"

	"connectrpc.com/connect"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
)

type historyHandler struct{ b api.Backend }

var _ backendv1connect.HistoryServiceHandler = (*historyHandler)(nil)

func (h *historyHandler) History(ctx context.Context, req *connect.Request[v1.HistoryRequest]) (*connect.Response[v1.HistoryResponse], error) {
	entries, err := h.b.History(ctx, int(req.Msg.Limit))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pb := make([]*v1.HistoryEntry, len(entries))
	for i := range entries {
		pb[i] = historyEntryToProto(entries[i])
	}
	return connect.NewResponse(&v1.HistoryResponse{Entries: pb}), nil
}

func (h *historyHandler) HistoryVideos(ctx context.Context, req *connect.Request[v1.HistoryVideosRequest]) (*connect.Response[v1.HistoryVideosResponse], error) {
	entries, err := h.b.HistoryVideos(ctx, int(req.Msg.Limit))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pb := make([]*v1.HistoryEntry, len(entries))
	for i := range entries {
		pb[i] = historyEntryToProto(entries[i])
	}
	return connect.NewResponse(&v1.HistoryVideosResponse{Entries: pb}), nil
}

func (h *historyHandler) VideoHistory(ctx context.Context, req *connect.Request[v1.VideoHistoryRequest]) (*connect.Response[v1.VideoHistoryResponse], error) {
	entries, err := h.b.VideoHistory(ctx, req.Msg.VideoId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pb := make([]*v1.HistoryEntry, len(entries))
	for i := range entries {
		pb[i] = historyEntryToProto(entries[i])
	}
	return connect.NewResponse(&v1.VideoHistoryResponse{Entries: pb}), nil
}

func (h *historyHandler) AddHistory(ctx context.Context, req *connect.Request[v1.AddHistoryRequest]) (*connect.Response[v1.AddHistoryResponse], error) {
	if err := h.b.AddHistory(ctx, req.Msg.VideoId, req.Msg.EventType, req.Msg.Details); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.AddHistoryResponse{}), nil
}

func (h *historyHandler) DeleteVideoHistory(ctx context.Context, req *connect.Request[v1.DeleteVideoHistoryRequest]) (*connect.Response[v1.DeleteVideoHistoryResponse], error) {
	if err := h.b.DeleteVideoHistory(ctx, req.Msg.VideoId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.DeleteVideoHistoryResponse{}), nil
}

func (h *historyHandler) DeleteSearchHistory(ctx context.Context, req *connect.Request[v1.DeleteSearchHistoryRequest]) (*connect.Response[v1.DeleteSearchHistoryResponse], error) {
	if err := h.b.DeleteSearchHistory(ctx, req.Msg.Query); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.DeleteSearchHistoryResponse{}), nil
}

func (h *historyHandler) ClearHistory(ctx context.Context, _ *connect.Request[v1.ClearHistoryRequest]) (*connect.Response[v1.ClearHistoryResponse], error) {
	if err := h.b.ClearHistory(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.ClearHistoryResponse{}), nil
}

func (h *historyHandler) ActivityLog(ctx context.Context, req *connect.Request[v1.ActivityLogRequest]) (*connect.Response[v1.ActivityLogResponse], error) {
	entries, err := h.b.ActivityLog(ctx, int(req.Msg.Limit))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pb := make([]*v1.ActivityEntry, len(entries))
	for i := range entries {
		pb[i] = activityEntryToProto(entries[i])
	}
	return connect.NewResponse(&v1.ActivityLogResponse{Entries: pb}), nil
}

func (h *historyHandler) LogActivity(ctx context.Context, req *connect.Request[v1.LogActivityRequest]) (*connect.Response[v1.LogActivityResponse], error) {
	if err := h.b.LogActivity(ctx, protoToActivityEntry(req.Msg.Entry)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.LogActivityResponse{}), nil
}

func (h *historyHandler) SearchQueries(ctx context.Context, _ *connect.Request[v1.SearchQueriesRequest]) (*connect.Response[v1.SearchQueriesResponse], error) {
	queries, err := h.b.SearchQueries(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SearchQueriesResponse{Queries: queries}), nil
}
