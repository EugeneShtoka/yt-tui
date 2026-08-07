package transport

import (
	"context"

	"connectrpc.com/connect"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
	"github.com/EugeneShtoka/yt-tui/internal/config"
)

type healthHandler struct{ b api.StatusBackend }

var _ backendv1connect.HealthServiceHandler = (*healthHandler)(nil)

// CheckAvailability runs the daemon-side environment probe (yt-dlp on PATH,
// cookie source resolves) and returns any problems. The InProc backend behind
// it calls the same youtube.Probe the single-binary path uses, so the probe
// logic stays single-sourced — only the transport differs. Issues are tagged
// as daemon-originated by the Remote adapter, not here, keeping this handler a
// pure pass-through (mirroring profileHandler).
func (h *healthHandler) CheckAvailability(ctx context.Context, _ *connect.Request[v1.CheckAvailabilityRequest]) (*connect.Response[v1.CheckAvailabilityResponse], error) {
	issues, err := h.b.CheckAvailability(ctx)
	if err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.CheckAvailabilityResponse{Issues: issuesToProto(issues)}), nil
}

// Capabilities reports daemon-side feature switches (currently whether the
// daemon serves thumbnails) so a client can decide its own thumbnail egress.
func (h *healthHandler) Capabilities(ctx context.Context, _ *connect.Request[v1.CapabilitiesRequest]) (*connect.Response[v1.CapabilitiesResponse], error) {
	caps, err := h.b.Capabilities(ctx)
	if err != nil {
		return nil, rpcErr(err)
	}
	return connect.NewResponse(&v1.CapabilitiesResponse{ThumbnailsEnabled: caps.ThumbnailsEnabled}), nil
}

// issuesToProto maps the config-layer issues onto their wire form.
func issuesToProto(issues []config.ConfigIssue) []*v1.ConfigIssue {
	out := make([]*v1.ConfigIssue, len(issues))
	for i, iss := range issues {
		out[i] = &v1.ConfigIssue{Severity: severityToProto(iss.Severity), Message: iss.Message}
	}
	return out
}

// severityToProto maps a config.Severity onto the proto enum.
func severityToProto(s config.Severity) v1.Severity {
	if s == config.SeverityError {
		return v1.Severity_SEVERITY_ERROR
	}
	return v1.Severity_SEVERITY_WARNING
}
