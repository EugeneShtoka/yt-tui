// Package httpauth provides the bearer-token middleware the daemon wraps its
// whole mux in. It is factored out of cmd/yt-tuid so round-trip tests can wrap
// their test server in the exact same middleware production ships with,
// instead of a hand-rolled approximation that drifts from it.
package httpauth

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/backend/media"
)

// Bearer wraps h to require "Authorization: Bearer <token>" on every request,
// except:
//   - /healthz, so load-balancers and monitoring can reach it without auth.
//   - paths under media.PathPrefix, which media.Handler authenticates itself
//     via a signed ticket (query param) OR a bearer header. Ticket URLs exist
//     precisely so an external player can fetch a media file without setting
//     headers; wrapping them in this middleware too would 401 every such
//     request whenever a token is configured.
//
// When token is empty, all requests are allowed through.
func Bearer(token string, h http.Handler) http.Handler {
	if token == "" {
		return h
	}
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("Authorization"))
		if r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, media.PathPrefix) ||
			subtle.ConstantTimeCompare(got, want) == 1 {
			h.ServeHTTP(w, r)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}
