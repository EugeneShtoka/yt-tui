package transport

import (
	"context"

	"connectrpc.com/connect"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
)

type profileHandler struct{ b api.ProfileBackend }

var _ backendv1connect.ProfileServiceHandler = (*profileHandler)(nil)

// ListProfiles returns the names of every daemon-stored config profile.
func (h *profileHandler) ListProfiles(ctx context.Context, _ *connect.Request[v1.ListProfilesRequest]) (*connect.Response[v1.ListProfilesResponse], error) {
	names, err := h.b.ListProfiles(ctx)
	if err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.ListProfilesResponse{Names: names}), nil
}

// GetProfile returns a named profile's opaque JSON bytes; found=false means it
// simply doesn't exist (the client falls back to its on-disk config).
func (h *profileHandler) GetProfile(ctx context.Context, req *connect.Request[v1.GetProfileRequest]) (*connect.Response[v1.GetProfileResponse], error) {
	data, found, err := h.b.GetProfile(ctx, req.Msg.Name)
	if err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.GetProfileResponse{Data: data, Found: found}), nil
}

// SaveProfile persists a profile's bytes under the given name (overwriting).
func (h *profileHandler) SaveProfile(ctx context.Context, req *connect.Request[v1.SaveProfileRequest]) (*connect.Response[v1.SaveProfileResponse], error) {
	if err := h.b.SaveProfile(ctx, req.Msg.Name, req.Msg.Data); err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.SaveProfileResponse{}), nil
}
