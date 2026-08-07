//nolint:wrapcheck // Connect errors are already structured; pass through without re-wrapping.
package api

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
)

// ── ProfileBackend ─────────────────────────────────────────────────────────────
//
// Profiles cross the wire as opaque JSON bytes, so the whole config-profile
// schema stays single-sourced in the client (internal/tui/app.configProfile)
// and never needs a proto change.

func (r *Remote) ListProfiles(ctx context.Context) ([]string, error) {
	resp, err := r.profile.ListProfiles(ctx, connect.NewRequest(&v1.ListProfilesRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.Names, nil
}

func (r *Remote) GetProfile(ctx context.Context, name string) ([]byte, bool, error) {
	resp, err := r.profile.GetProfile(ctx, connect.NewRequest(&v1.GetProfileRequest{Name: name}))
	if err != nil {
		return nil, false, err
	}
	return resp.Msg.Data, resp.Msg.Found, nil
}

func (r *Remote) SaveProfile(ctx context.Context, name string, data []byte) error {
	_, err := r.profile.SaveProfile(ctx, connect.NewRequest(&v1.SaveProfileRequest{Name: name, Data: data}))
	return err
}
