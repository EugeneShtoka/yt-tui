package httpauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// okHandler is the protected handler; a 200 means the middleware let the request
// through, any other status means it was blocked before reaching here.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestBearer(t *testing.T) {
	const token = "s3cret"

	tests := []struct {
		name       string
		token      string // token the middleware is configured with
		path       string
		authHeader string
		want       int
	}{
		{name: "no token configured lets everything through", token: "", path: "/rpc", authHeader: "", want: http.StatusOK},
		{name: "correct bearer", token: token, path: "/rpc", authHeader: "Bearer " + token, want: http.StatusOK},
		{name: "incorrect bearer", token: token, path: "/rpc", authHeader: "Bearer wrong", want: http.StatusUnauthorized},
		{name: "missing bearer", token: token, path: "/rpc", authHeader: "", want: http.StatusUnauthorized},
		{name: "token without Bearer prefix", token: token, path: "/rpc", authHeader: token, want: http.StatusUnauthorized},
		{name: "healthz exempt", token: token, path: "/healthz", authHeader: "", want: http.StatusOK},
		{name: "media prefix exempt", token: token, path: "/media/abc123", authHeader: "", want: http.StatusOK},
		{name: "media prefix bare (no id) still exempt", token: token, path: "/media/", authHeader: "", want: http.StatusOK},
		// A path that merely starts similar to the media prefix must NOT be exempt.
		{name: "media lookalike is not exempt", token: token, path: "/media-secret/x", authHeader: "", want: http.StatusUnauthorized},
		{name: "healthz lookalike is not exempt", token: token, path: "/healthz-extra", authHeader: "", want: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := Bearer(tt.token, okHandler)
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, tt.path, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Errorf("path %q auth %q: status = %d, want %d", tt.path, tt.authHeader, rec.Code, tt.want)
			}
		})
	}
}
