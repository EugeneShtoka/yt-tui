//nolint:wrapcheck // Connect errors are already structured; pass through without re-wrapping.
package api

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/config"
)

// ── StatusBackend ───────────────────────────────────────────────────────────────
//
// In remote mode the environment that matters (yt-dlp, cookies, files) lives on
// the daemon host, so the probe must run there. CheckAvailability dials the
// daemon's HealthService — which runs the same youtube.Probe the single-binary
// path uses — and tags each issue as daemon-originated so a remote user knows
// the fault is server-side. Issued once on connect alongside the profile fetch.

// daemonIssuePrefix marks an issue as originating on the daemon host rather than
// the client, so "yt-dlp not found" reads as "daemon: yt-dlp not found".
const daemonIssuePrefix = "daemon: "

func (r *Remote) CheckAvailability(ctx context.Context) ([]config.ConfigIssue, error) {
	resp, err := r.health.CheckAvailability(ctx, connect.NewRequest(&v1.CheckAvailabilityRequest{}))
	if err != nil {
		return nil, err
	}
	return issuesFromProto(resp.Msg.Issues), nil
}

func (r *Remote) Capabilities(ctx context.Context) (Capabilities, error) {
	resp, err := r.health.Capabilities(ctx, connect.NewRequest(&v1.CapabilitiesRequest{}))
	if err != nil {
		return Capabilities{}, err
	}
	return Capabilities{ThumbnailsEnabled: resp.Msg.ThumbnailsEnabled}, nil
}

// issuesFromProto maps the wire issues back to config-layer issues, tagging each
// as daemon-originated.
func issuesFromProto(issues []*v1.ConfigIssue) []config.ConfigIssue {
	if len(issues) == 0 {
		return nil
	}
	out := make([]config.ConfigIssue, len(issues))
	for i, iss := range issues {
		out[i] = config.ConfigIssue{
			Severity: severityFromProto(iss.GetSeverity()),
			Message:  daemonIssuePrefix + iss.GetMessage(),
		}
	}
	return out
}

// severityFromProto maps the proto enum back onto config.Severity. An
// unspecified value falls back to the safe warning severity.
func severityFromProto(s v1.Severity) config.Severity {
	if s == v1.Severity_SEVERITY_ERROR {
		return config.SeverityError
	}
	return config.SeverityWarning
}
