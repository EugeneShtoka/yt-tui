//nolint:wrapcheck // Connect errors are already structured; pass through without re-wrapping.
package api

import (
	"context"
	"encoding/json"
	"fmt"

	"connectrpc.com/connect"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
)

// ── PortabilityBackend ─────────────────────────────────────────────────────────

// Export asks the daemon to assemble the bundle and returns it decoded. The
// bundle travels as opaque JSON bytes so the schema stays single-sourced in Go.
func (r *Remote) Export(ctx context.Context, opts portability.ExportOptions) (portability.Bundle, error) {
	resp, err := r.port.Export(ctx, connect.NewRequest(&v1.ExportRequest{IncludeWatchData: opts.IncludeWatchData}))
	if err != nil {
		return portability.Bundle{}, err
	}
	var b portability.Bundle
	if err := json.Unmarshal(resp.Msg.Bundle, &b); err != nil {
		return portability.Bundle{}, fmt.Errorf("Export: decode bundle: %w", err)
	}
	return b, nil
}

// ImportPreview marshals the bundle to JSON, ships it with the opt-in flags, and
// decodes the daemon's ImportPlan (also carried as JSON bytes).
func (r *Remote) ImportPreview(ctx context.Context, bundle portability.Bundle, opts portability.ImportOptions) (portability.ImportPlan, error) {
	data, err := json.Marshal(bundle)
	if err != nil {
		return portability.ImportPlan{}, fmt.Errorf("ImportPreview: encode bundle: %w", err)
	}
	resp, err := r.port.ImportPreview(ctx, connect.NewRequest(&v1.ImportPreviewRequest{
		Bundle:           data,
		ConvertYtToLocal: opts.ConvertYTToLocal,
		IncludeWatchData: opts.IncludeWatchData,
	}))
	if err != nil {
		return portability.ImportPlan{}, err
	}
	var plan portability.ImportPlan
	if err := json.Unmarshal(resp.Msg.Plan, &plan); err != nil {
		return portability.ImportPlan{}, fmt.Errorf("ImportPreview: decode plan: %w", err)
	}
	return plan, nil
}

// ImportApply ships the bundle + flags to the daemon and decodes the
// ImportResult it reports.
func (r *Remote) ImportApply(ctx context.Context, bundle portability.Bundle, opts portability.ImportOptions) (portability.ImportResult, error) {
	data, err := json.Marshal(bundle)
	if err != nil {
		return portability.ImportResult{}, fmt.Errorf("ImportApply: encode bundle: %w", err)
	}
	resp, err := r.port.ImportApply(ctx, connect.NewRequest(&v1.ImportApplyRequest{
		Bundle:           data,
		ConvertYtToLocal: opts.ConvertYTToLocal,
		IncludeWatchData: opts.IncludeWatchData,
	}))
	if err != nil {
		return portability.ImportResult{}, err
	}
	var res portability.ImportResult
	if err := json.Unmarshal(resp.Msg.Result, &res); err != nil {
		return portability.ImportResult{}, fmt.Errorf("ImportApply: decode result: %w", err)
	}
	return res, nil
}
