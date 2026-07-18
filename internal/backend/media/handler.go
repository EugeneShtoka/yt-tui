package media

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// LocalVideoStore is the subset of the backend needed to look up downloaded files.
type LocalVideoStore interface {
	HasLocalVideo(ctx context.Context, videoID string) (domain.LocalVideo, bool)
}

// Handler returns an http.Handler that serves downloaded video files at
// GET /media/{id}. The token parameter, when non-empty, requires every request
// to carry an "Authorization: Bearer <token>" header; unauthenticated requests
// receive 401. Range requests are supported via http.ServeContent.
func Handler(store LocalVideoStore, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/media/")
		if id == "" || strings.Contains(id, "/") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		lv, ok := store.HasLocalVideo(r.Context(), id)
		if !ok || lv.FilePath == "" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		f, err := os.Open(lv.FilePath)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer f.Close()

		fi, err := f.Stat()
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		http.ServeContent(w, r, fi.Name(), time.Time{}, f)
	})
}
