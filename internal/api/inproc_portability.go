//nolint:wrapcheck // pass-through adapter; errors from the service are already contextual
package api

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
)

// ── PortabilityBackend ─────────────────────────────────────────────────────────

func (p *InProc) Export(ctx context.Context, opts portability.ExportOptions) (portability.Bundle, error) {
	return p.port.Export(ctx, opts)
}

func (p *InProc) ImportPreview(ctx context.Context, bundle portability.Bundle, opts portability.ImportOptions) (portability.ImportPlan, error) {
	return p.port.ImportPreview(ctx, bundle, opts)
}

func (p *InProc) ImportApply(ctx context.Context, bundle portability.Bundle, opts portability.ImportOptions) (portability.ImportResult, error) {
	return p.port.ImportApply(ctx, bundle, opts)
}
