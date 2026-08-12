package media

import (
	"context"
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// LocalVideoStore is the subset of the backend needed to look up downloaded files.
type LocalVideoStore interface {
	HasLocalVideo(ctx context.Context, videoID string) (domain.LocalVideo, bool, error)
}

// Handler returns an http.Handler that serves downloaded video files at
// GET /media/{id}. When token is non-empty, each request must present either:
//   - "Authorization: Bearer <token>" header, OR
//   - a valid ?t=<ticket> query param minted by MintTicket.
//
// Range requests are supported via http.ServeContent.
func Handler(store LocalVideoStore, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, PathPrefix)
		if id == "" || strings.Contains(id, "/") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if token != "" {
			want := []byte("Bearer " + token)
			bearer := subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) == 1
			ticket := ValidateTicket(token, id, r.URL.Query().Get("t"))
			if !bearer && !ticket {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}

		lv, ok, err := store.HasLocalVideo(r.Context(), id)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
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
