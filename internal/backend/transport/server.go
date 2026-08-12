// Package transport mounts the daemon's Connect RPC handlers onto an
// http.ServeMux. Each handler is a thin adapter that decodes a request,
// converts proto messages to domain types via the protoconv package, calls the
// shared api.Backend, and encodes the result back — so the same backend serves
// both the in-process and remote-daemon topologies.
package transport

import (
	"net/http"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
)

// Mount registers all Connect service handlers on mux under their canonical paths.
// token, when non-empty, is used to mint short-lived signed tickets for /media/{id} URLs
// returned by ResolveSource; the same token must be passed to media.Handler.
func Mount(mux *http.ServeMux, b api.Backend, token string) {
	mux.Handle(backendv1connect.NewFeedServiceHandler(&feedHandler{b: b}))
	mux.Handle(backendv1connect.NewChannelServiceHandler(&channelHandler{b: b}))
	mux.Handle(backendv1connect.NewVideoServiceHandler(&videoHandler{b: b, token: token}))
	mux.Handle(backendv1connect.NewLibraryServiceHandler(&libraryHandler{b: b}))
	mux.Handle(backendv1connect.NewPlaylistServiceHandler(&playlistHandler{b: b}))
	mux.Handle(backendv1connect.NewHistoryServiceHandler(&historyHandler{b: b}))
	mux.Handle(backendv1connect.NewPortabilityServiceHandler(&portabilityHandler{b: b}))
	mux.Handle(backendv1connect.NewProfileServiceHandler(&profileHandler{b: b}))
	mux.Handle(backendv1connect.NewHealthServiceHandler(&healthHandler{b: b}))
	mux.Handle(backendv1connect.NewDownloadServiceHandler(&downloadHandler{b: b}))
}
