package transport

import (
	"net/http"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
)

// Mount registers all Connect service handlers on mux under their canonical paths.
// mediaBaseURL is the daemon's own base URL (e.g. "http://localhost:7373") used
// to construct /media/{id} URLs returned by ResolveSource for remote clients.
func Mount(mux *http.ServeMux, b api.Backend, mediaBaseURL string) {
	mux.Handle(backendv1connect.NewFeedServiceHandler(&feedHandler{b: b}))
	mux.Handle(backendv1connect.NewChannelServiceHandler(&channelHandler{b: b}))
	mux.Handle(backendv1connect.NewVideoServiceHandler(&videoHandler{b: b, mediaBaseURL: mediaBaseURL}))
	mux.Handle(backendv1connect.NewLibraryServiceHandler(&libraryHandler{b: b}))
	mux.Handle(backendv1connect.NewPlaylistServiceHandler(&playlistHandler{b: b}))
	mux.Handle(backendv1connect.NewHistoryServiceHandler(&historyHandler{b: b}))
	mux.Handle(backendv1connect.NewDownloadServiceHandler(&downloadHandler{b: b}))
}
