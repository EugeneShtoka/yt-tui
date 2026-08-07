//nolint:wrapcheck,gosec // Connect errors are already structured; pass through without re-wrapping. gosec G115: proto int32 fields are bounded in practice (durations, counts).
package api

import (
	"net/http"

	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
)

// Remote implements Backend by dialing a yt-tuid daemon over Connect (HTTP/2
// or HTTP/1.1). Its methods are split across remote_{feed,channel,video,
// library,playlist,history,download}.go, one file per Backend role interface
// (H-7).
type Remote struct {
	baseURL  string
	feed     backendv1connect.FeedServiceClient
	ch       backendv1connect.ChannelServiceClient
	vid      backendv1connect.VideoServiceClient
	lib      backendv1connect.LibraryServiceClient
	playlist backendv1connect.PlaylistServiceClient
	history  backendv1connect.HistoryServiceClient
	port     backendv1connect.PortabilityServiceClient
	profile  backendv1connect.ProfileServiceClient
	health   backendv1connect.HealthServiceClient
	dl       backendv1connect.DownloadServiceClient
	// dlStream is a second DownloadService client used only for the
	// long-lived Events stream. It shares dl's transport but never carries a
	// client-level Timeout (see NewRemote): http.Client.Timeout bounds the
	// whole request including reading the response body, so applying a
	// caller-set timeout to it would kill the stream after that duration
	// regardless of how active it is. (M-24)
	dlStream backendv1connect.DownloadServiceClient
}

// authTransport injects a bearer token into every outbound request.
type authTransport struct {
	base  http.RoundTripper
	token string
}

func (a authTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if a.token != "" {
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+a.token)
	}
	return a.base.RoundTrip(r)
}

// NewRemote dials baseURL (e.g. "http://localhost:7373") with the given HTTP client and
// optional bearer token. Pass token="" for unauthenticated connections.
func NewRemote(baseURL, token string, httpClient *http.Client) *Remote {
	base := httpClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	streamClient := &http.Client{Transport: base}
	if token != "" {
		httpClient = &http.Client{
			Transport: authTransport{base: base, token: token},
			Timeout:   httpClient.Timeout,
		}
		streamClient.Transport = authTransport{base: base, token: token}
	}
	return &Remote{
		baseURL:  baseURL,
		feed:     backendv1connect.NewFeedServiceClient(httpClient, baseURL),
		ch:       backendv1connect.NewChannelServiceClient(httpClient, baseURL),
		vid:      backendv1connect.NewVideoServiceClient(httpClient, baseURL),
		lib:      backendv1connect.NewLibraryServiceClient(httpClient, baseURL),
		playlist: backendv1connect.NewPlaylistServiceClient(httpClient, baseURL),
		history:  backendv1connect.NewHistoryServiceClient(httpClient, baseURL),
		port:     backendv1connect.NewPortabilityServiceClient(httpClient, baseURL),
		profile:  backendv1connect.NewProfileServiceClient(httpClient, baseURL),
		health:   backendv1connect.NewHealthServiceClient(httpClient, baseURL),
		dl:       backendv1connect.NewDownloadServiceClient(httpClient, baseURL),
		dlStream: backendv1connect.NewDownloadServiceClient(streamClient, baseURL),
	}
}
