package api_test

import "github.com/EugeneShtoka/yt-tui/internal/api/apitest"

// nopBackend is the shared zero-value api.Backend fake. It lives in the reusable
// apitest package so the TUI tab tests can embed the same double; the alias
// keeps the existing testBackend embedding (b.nopBackend.X) working.
type nopBackend = apitest.NopBackend
