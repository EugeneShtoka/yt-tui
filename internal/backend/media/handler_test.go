package media_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/backend/media"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

type fakeStore struct {
	videos map[string]domain.LocalVideo
	err    error
}

func (f *fakeStore) HasLocalVideo(_ context.Context, id string) (domain.LocalVideo, bool, error) {
	if f.err != nil {
		return domain.LocalVideo{}, false, f.err
	}
	v, ok := f.videos[id]
	return v, ok, nil
}

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "video-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

func newHandlerSrv(t *testing.T, store media.LocalVideoStore, token string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(media.Handler(store, token))
	t.Cleanup(srv.Close)
	return srv
}

func TestHandlerNoTokenOpen(t *testing.T) {
	path := writeTempFile(t, "video data")
	store := &fakeStore{videos: map[string]domain.LocalVideo{"v1": {FilePath: path}}}
	srv := newHandlerSrv(t, store, "")

	resp, err := http.Get(srv.URL + "/media/v1") //nolint:noctx
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestHandlerBearerAccepted(t *testing.T) {
	path := writeTempFile(t, "content")
	store := &fakeStore{videos: map[string]domain.LocalVideo{"v1": {FilePath: path}}}
	const token = "s3cr3t"
	srv := newHandlerSrv(t, store, token)

	req, err := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/media/v1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestHandlerBearerRejected(t *testing.T) {
	path := writeTempFile(t, "content")
	store := &fakeStore{videos: map[string]domain.LocalVideo{"v1": {FilePath: path}}}
	srv := newHandlerSrv(t, store, "s3cr3t")

	for _, tc := range []struct {
		name   string
		header string
	}{
		{"no auth", ""},
		{"wrong token", "Bearer wrong"},
		{"bad scheme", "Basic s3cr3t"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/media/v1", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("want 401, got %d", resp.StatusCode)
			}
		})
	}
}

func TestHandlerTicketAccepted(t *testing.T) {
	path := writeTempFile(t, "content")
	store := &fakeStore{videos: map[string]domain.LocalVideo{"v1": {FilePath: path}}}
	const token = "s3cr3t"
	srv := newHandlerSrv(t, store, token)

	ticket := media.MintTicket(token, "v1")
	resp, err := http.Get(srv.URL + "/media/v1?t=" + ticket) //nolint:noctx
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200, got %d", resp.StatusCode)
	}
}

func TestHandlerTicketRejected(t *testing.T) {
	path := writeTempFile(t, "content")
	store := &fakeStore{videos: map[string]domain.LocalVideo{"v1": {FilePath: path}}}
	const token = "s3cr3t"
	srv := newHandlerSrv(t, store, token)

	for _, tc := range []struct {
		name   string
		ticket string
	}{
		{"wrong video", media.MintTicket(token, "other")},
		{"past expiry", "fakesig.0"},
		{"malformed", "badticket"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(srv.URL + "/media/v1?t=" + tc.ticket) //nolint:noctx
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Errorf("want 401, got %d", resp.StatusCode)
			}
		})
	}
}

func TestHandlerPathTraversal(t *testing.T) {
	srv := newHandlerSrv(t, &fakeStore{}, "")

	for _, path := range []string{"/media/", "/media/a/b"} {
		resp, err := http.Get(srv.URL + path) //nolint:noctx
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("path %s: want 404, got %d", path, resp.StatusCode)
		}
	}
}

func TestHandlerMissingVideo(t *testing.T) {
	srv := newHandlerSrv(t, &fakeStore{}, "")

	resp, err := http.Get(srv.URL + "/media/unknown") //nolint:noctx
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestHandlerMissingFile(t *testing.T) {
	store := &fakeStore{videos: map[string]domain.LocalVideo{
		"v1": {FilePath: "/nonexistent/path/video.mp4"},
	}}
	srv := newHandlerSrv(t, store, "")

	resp, err := http.Get(srv.URL + "/media/v1") //nolint:noctx
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", resp.StatusCode)
	}
}

func TestHandlerStoreError(t *testing.T) {
	srv := newHandlerSrv(t, &fakeStore{err: errors.New("db unavailable")}, "")

	resp, err := http.Get(srv.URL + "/media/v1") //nolint:noctx
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("want 500 on lookup failure (not 404, which would look like a legitimate miss), got %d", resp.StatusCode)
	}
}

func TestHandlerRangeRequest(t *testing.T) {
	path := writeTempFile(t, "0123456789")
	store := &fakeStore{videos: map[string]domain.LocalVideo{"v1": {FilePath: path}}}
	srv := newHandlerSrv(t, store, "")

	req, err := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/media/v1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=2-5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		t.Fatalf("want 206, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "2345" {
		t.Errorf("want body '2345', got %q", body)
	}
}
