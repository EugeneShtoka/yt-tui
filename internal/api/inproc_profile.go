//nolint:wrapcheck // pass-through adapter; errors from the profile store are already contextual
package api

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ── ProfileBackend ─────────────────────────────────────────────────────────────
//
// The profile store is opaque about its payload: it persists the client's
// portable config profile as JSON bytes without parsing it. When the store
// failed to initialize (nil), profiles behave as an empty, read-only set and
// saves report domain.ErrProfilesUnavailable rather than panicking.

func (p *InProc) ListProfiles(ctx context.Context) ([]string, error) {
	if p.profiles == nil {
		return nil, nil
	}
	return p.profiles.List()
}

func (p *InProc) GetProfile(ctx context.Context, name string) ([]byte, bool, error) {
	if p.profiles == nil {
		return nil, false, nil
	}
	return p.profiles.Get(name)
}

func (p *InProc) SaveProfile(ctx context.Context, name string, data []byte) error {
	if p.profiles == nil {
		return domain.ErrProfilesUnavailable
	}
	return p.profiles.Save(name, data)
}
