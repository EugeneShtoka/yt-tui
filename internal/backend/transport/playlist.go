package transport

import (
	"context"

	"connectrpc.com/connect"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

type playlistHandler struct{ b api.Backend }

var _ backendv1connect.PlaylistServiceHandler = (*playlistHandler)(nil)

func (h *playlistHandler) LocalPlaylists(ctx context.Context, _ *connect.Request[v1.LocalPlaylistsRequest]) (*connect.Response[v1.LocalPlaylistsResponse], error) {
	pls, err := h.b.LocalPlaylists(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pb := make([]*v1.Playlist, len(pls))
	for i := range pls {
		pb[i] = playlistToProto(pls[i])
	}
	return connect.NewResponse(&v1.LocalPlaylistsResponse{Playlists: pb}), nil
}

func (h *playlistHandler) LocalPlaylistVideos(ctx context.Context, req *connect.Request[v1.LocalPlaylistVideosRequest]) (*connect.Response[v1.LocalPlaylistVideosResponse], error) {
	vids, err := h.b.LocalPlaylistVideos(ctx, req.Msg.PlaylistId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.LocalPlaylistVideosResponse{Videos: videosToProto(vids)}), nil
}

func (h *playlistHandler) PlaylistVideoIDs(ctx context.Context, req *connect.Request[v1.PlaylistVideoIDsRequest]) (*connect.Response[v1.PlaylistVideoIDsResponse], error) {
	ids, err := h.b.PlaylistVideoIDs(ctx, req.Msg.PlaylistId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.PlaylistVideoIDsResponse{Ids: ids}), nil
}

func (h *playlistHandler) CreatePlaylist(ctx context.Context, req *connect.Request[v1.CreatePlaylistRequest]) (*connect.Response[v1.CreatePlaylistResponse], error) {
	id, err := h.b.CreatePlaylist(ctx, req.Msg.Name)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.CreatePlaylistResponse{Id: id}), nil
}

func (h *playlistHandler) DeletePlaylist(ctx context.Context, req *connect.Request[v1.DeletePlaylistRequest]) (*connect.Response[v1.DeletePlaylistResponse], error) {
	if err := h.b.DeletePlaylist(ctx, req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.DeletePlaylistResponse{}), nil
}

func (h *playlistHandler) AddToPlaylist(ctx context.Context, req *connect.Request[v1.AddToPlaylistRequest]) (*connect.Response[v1.AddToPlaylistResponse], error) {
	if err := h.b.AddToPlaylist(ctx, req.Msg.PlaylistId, req.Msg.VideoId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.AddToPlaylistResponse{}), nil
}

func (h *playlistHandler) RemoveFromPlaylist(ctx context.Context, req *connect.Request[v1.RemoveFromPlaylistRequest]) (*connect.Response[v1.RemoveFromPlaylistResponse], error) {
	if err := h.b.RemoveFromPlaylist(ctx, req.Msg.PlaylistId, req.Msg.VideoId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.RemoveFromPlaylistResponse{}), nil
}

func (h *playlistHandler) WatchLater(ctx context.Context, _ *connect.Request[v1.WatchLaterRequest]) (*connect.Response[v1.WatchLaterResponse], error) {
	entries, err := h.b.WatchLater(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pb := make([]*v1.WatchLaterEntry, len(entries))
	for i := range entries {
		pb[i] = watchLaterEntryToProto(entries[i])
	}
	return connect.NewResponse(&v1.WatchLaterResponse{Entries: pb}), nil
}

func (h *playlistHandler) AddWatchLater(ctx context.Context, req *connect.Request[v1.AddWatchLaterRequest]) (*connect.Response[v1.AddWatchLaterResponse], error) {
	if err := h.b.AddWatchLater(ctx, req.Msg.Id, req.Msg.Title, req.Msg.Channel, req.Msg.Url); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.AddWatchLaterResponse{}), nil
}

func (h *playlistHandler) RemoveWatchLater(ctx context.Context, req *connect.Request[v1.RemoveWatchLaterRequest]) (*connect.Response[v1.RemoveWatchLaterResponse], error) {
	if err := h.b.RemoveWatchLater(ctx, req.Msg.Id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.RemoveWatchLaterResponse{}), nil
}

func (h *playlistHandler) YTPlaylists(ctx context.Context, _ *connect.Request[v1.YTPlaylistsRequest]) (*connect.Response[v1.YTPlaylistsResponse], error) {
	pls, err := h.b.YTPlaylists(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pb := make([]*v1.YTPlaylist, len(pls))
	for i := range pls {
		pb[i] = ytPlaylistToProto(pls[i])
	}
	return connect.NewResponse(&v1.YTPlaylistsResponse{Playlists: pb}), nil
}

func (h *playlistHandler) YTPlaylistVideos(ctx context.Context, req *connect.Request[v1.YTPlaylistVideosRequest]) (*connect.Response[v1.YTPlaylistVideosResponse], error) {
	vids, err := h.b.YTPlaylistVideos(ctx, req.Msg.PlaylistId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.YTPlaylistVideosResponse{Videos: videosToProto(vids)}), nil
}

func (h *playlistHandler) GetYTPlaylists(ctx context.Context, _ *connect.Request[v1.GetYTPlaylistsRequest]) (*connect.Response[v1.GetYTPlaylistsResponse], error) {
	pls, err := h.b.GetYTPlaylists(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	pb := make([]*v1.YTPlaylist, len(pls))
	for i := range pls {
		pb[i] = ytPlaylistToProto(pls[i])
	}
	return connect.NewResponse(&v1.GetYTPlaylistsResponse{Playlists: pb}), nil
}

func (h *playlistHandler) GetYTPlaylistVideos(ctx context.Context, req *connect.Request[v1.GetYTPlaylistVideosRequest]) (*connect.Response[v1.GetYTPlaylistVideosResponse], error) {
	vids, err := h.b.GetYTPlaylistVideos(ctx, req.Msg.PlaylistId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.GetYTPlaylistVideosResponse{Videos: videosToProto(vids)}), nil
}

func (h *playlistHandler) SaveYTPlaylists(ctx context.Context, req *connect.Request[v1.SaveYTPlaylistsRequest]) (*connect.Response[v1.SaveYTPlaylistsResponse], error) {
	pls := make([]domain.YTPlaylist, len(req.Msg.Playlists))
	for i, p := range req.Msg.Playlists {
		if p != nil {
			pls[i] = domain.YTPlaylist{ID: p.Id, Title: p.Title}
		}
	}
	if err := h.b.SaveYTPlaylists(ctx, pls); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SaveYTPlaylistsResponse{}), nil
}

func (h *playlistHandler) SaveYTPlaylistVideos(ctx context.Context, req *connect.Request[v1.SaveYTPlaylistVideosRequest]) (*connect.Response[v1.SaveYTPlaylistVideosResponse], error) {
	if err := h.b.SaveYTPlaylistVideos(ctx, req.Msg.PlaylistId, protoToVideos(req.Msg.Videos)); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.SaveYTPlaylistVideosResponse{}), nil
}

func (h *playlistHandler) InitYTClient(ctx context.Context, _ *connect.Request[v1.InitYTClientRequest]) (*connect.Response[v1.InitYTClientResponse], error) {
	if err := h.b.InitYTClient(ctx); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.InitYTClientResponse{}), nil
}

func (h *playlistHandler) CreateYTPlaylist(ctx context.Context, req *connect.Request[v1.CreateYTPlaylistRequest]) (*connect.Response[v1.CreateYTPlaylistResponse], error) {
	id, err := h.b.CreateYTPlaylist(ctx, req.Msg.Name)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.CreateYTPlaylistResponse{Id: id}), nil
}

func (h *playlistHandler) DeleteYTPlaylist(ctx context.Context, req *connect.Request[v1.DeleteYTPlaylistRequest]) (*connect.Response[v1.DeleteYTPlaylistResponse], error) {
	if err := h.b.DeleteYTPlaylist(ctx, req.Msg.PlaylistId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.DeleteYTPlaylistResponse{}), nil
}

func (h *playlistHandler) AddToYTPlaylist(ctx context.Context, req *connect.Request[v1.AddToYTPlaylistRequest]) (*connect.Response[v1.AddToYTPlaylistResponse], error) {
	if err := h.b.AddToYTPlaylist(ctx, req.Msg.PlaylistId, req.Msg.VideoId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.AddToYTPlaylistResponse{}), nil
}

func (h *playlistHandler) RemoveFromYTPlaylist(ctx context.Context, req *connect.Request[v1.RemoveFromYTPlaylistRequest]) (*connect.Response[v1.RemoveFromYTPlaylistResponse], error) {
	if err := h.b.RemoveFromYTPlaylist(ctx, req.Msg.PlaylistId, req.Msg.VideoId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&v1.RemoveFromYTPlaylistResponse{}), nil
}
