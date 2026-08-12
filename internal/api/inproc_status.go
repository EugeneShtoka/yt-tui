package api

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

// ── StatusBackend ───────────────────────────────────────────────────────────────
//
// Single-binary mode probes the real host environment. The check is local-only
// (yt-dlp on PATH, cookie source resolves) and never fails as an operation, so
// the error return is always nil — problems come back as issues.

func (p *InProc) CheckAvailability(ctx context.Context) ([]config.ConfigIssue, error) {
	return youtube.Probe(p.cfg), nil
}

// Capabilities reports the daemon's feature switches. ThumbnailsEnabled mirrors
// whether the on-disk thumbnail store came up (see ThumbnailsEnabled).
func (p *InProc) Capabilities(context.Context) (Capabilities, error) {
	return Capabilities{ThumbnailsEnabled: p.ThumbnailsEnabled()}, nil
}
