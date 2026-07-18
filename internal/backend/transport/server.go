package transport

import (
	"net/http"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
)

// Mount registers all Connect service handlers on mux under their canonical paths.
func Mount(mux *http.ServeMux, b api.Backend) {
	mux.Handle(backendv1connect.NewFeedServiceHandler(&feedHandler{b: b}))
	mux.Handle(backendv1connect.NewChannelServiceHandler(&channelHandler{b: b}))
	mux.Handle(backendv1connect.NewVideoServiceHandler(&videoHandler{b: b}))
	mux.Handle(backendv1connect.NewLibraryServiceHandler(&libraryHandler{b: b}))
	mux.Handle(backendv1connect.NewPlaylistServiceHandler(&playlistHandler{b: b}))
	mux.Handle(backendv1connect.NewHistoryServiceHandler(&historyHandler{b: b}))
	mux.Handle(backendv1connect.NewDownloadServiceHandler(&downloadHandler{b: b}))
}
