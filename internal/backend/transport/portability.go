package transport

import (
	"context"
	"encoding/json"
	"fmt"

	"connectrpc.com/connect"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
)

type portabilityHandler struct{ b api.PortabilityBackend }

var _ backendv1connect.PortabilityServiceHandler = (*portabilityHandler)(nil)

// Export assembles the bundle on the daemon (where the DB lives) and returns it
// as opaque JSON bytes; the Remote client decodes them back into a Bundle.
func (h *portabilityHandler) Export(ctx context.Context, req *connect.Request[v1.ExportRequest]) (*connect.Response[v1.ExportResponse], error) {
	bundle, err := h.b.Export(ctx, portability.ExportOptions{IncludeWatchData: req.Msg.IncludeWatchData})
	if err != nil {
		return nil, rpcErr(err)
	}
	data, err := json.Marshal(bundle)
	if err != nil {
		return nil, rpcErr(fmt.Errorf("encode bundle: %w", err))
	}
	return connect.NewResponse(&v1.ExportResponse{Bundle: data}), nil
}

// ImportPreview decodes the incoming bundle, runs the dry-run diff on the
// daemon, and returns the ImportPlan as opaque JSON bytes.
func (h *portabilityHandler) ImportPreview(ctx context.Context, req *connect.Request[v1.ImportPreviewRequest]) (*connect.Response[v1.ImportPreviewResponse], error) {
	bundle, err := decodeBundle(req.Msg.Bundle)
	if err != nil {
		return nil, rpcErr(err)
	}
	plan, err := h.b.ImportPreview(ctx, bundle, portability.ImportOptions{
		ConvertYTToLocal: req.Msg.ConvertYtToLocal,
		IncludeWatchData: req.Msg.IncludeWatchData,
	})
	if err != nil {
		return nil, rpcErr(err)
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, rpcErr(fmt.Errorf("encode plan: %w", err))
	}
	return connect.NewResponse(&v1.ImportPreviewResponse{Plan: data}), nil
}

// ImportApply decodes the bundle and applies it on the daemon, returning the
// ImportResult as opaque JSON bytes.
func (h *portabilityHandler) ImportApply(ctx context.Context, req *connect.Request[v1.ImportApplyRequest]) (*connect.Response[v1.ImportApplyResponse], error) {
	bundle, err := decodeBundle(req.Msg.Bundle)
	if err != nil {
		return nil, rpcErr(err)
	}
	res, err := h.b.ImportApply(ctx, bundle, portability.ImportOptions{
		ConvertYTToLocal: req.Msg.ConvertYtToLocal,
		IncludeWatchData: req.Msg.IncludeWatchData,
	})
	if err != nil {
		return nil, rpcErr(err)
	}
	data, err := json.Marshal(res)
	if err != nil {
		return nil, rpcErr(fmt.Errorf("encode result: %w", err))
	}
	return connect.NewResponse(&v1.ImportApplyResponse{Result: data}), nil
}

// decodeBundle unmarshals the opaque JSON bundle bytes shared by both import RPCs.
func decodeBundle(data []byte) (portability.Bundle, error) {
	var b portability.Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return portability.Bundle{}, fmt.Errorf("decode bundle: %w", err)
	}
	return b, nil
}
