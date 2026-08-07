//nolint:wrapcheck // test stub — delegates to the apitest fake; errors are irrelevant
package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/backend/httpauth"
	"github.com/EugeneShtoka/yt-tui/internal/backend/media"
	"github.com/EugeneShtoka/yt-tui/internal/backend/transport"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
)

// testBackend embeds nopBackend and overrides specific methods via func fields.
type testBackend struct {
	nopBackend
	fnLocalVideos   func(context.Context) ([]domain.LocalVideo, error)
	fnVideoPosition func(context.Context, string) (int64, bool, error)
	fnSaveVideoPos  func(context.Context, string, int64) error
	fnHasLocalVideo func(context.Context, string) (domain.LocalVideo, bool, error)
	fnEvents        func(context.Context) (<-chan api.Event, error)
	fnAllChannels   func(context.Context) ([]domain.Channel, error)
	fnBlockChannel  func(context.Context, domain.Channel) error
	fnDeleteAllLoc  func(context.Context) (int, error)
	fnClearDl       func(context.Context) error
	fnExport        func(context.Context, portability.ExportOptions) (portability.Bundle, error)
	fnImportPreview func(context.Context, portability.Bundle, portability.ImportOptions) (portability.ImportPlan, error)
	fnImportApply   func(context.Context, portability.Bundle, portability.ImportOptions) (portability.ImportResult, error)
	fnListProfiles  func(context.Context) ([]string, error)
	fnGetProfile    func(context.Context, string) ([]byte, bool, error)
	fnSaveProfile   func(context.Context, string, []byte) error
	fnCheckAvail    func(context.Context) ([]config.ConfigIssue, error)
	fnSetChanState  func(context.Context, string, domain.SubscriptionState) error
	fnSaveChanVids  func(context.Context, string, []domain.Video) error
	fnChanLatestN   func(context.Context, string, string, int) ([]domain.Video, error)
	fnSaveSBSegs    func(context.Context, string, []domain.SBSegment) error
	fnAddToPlaylist func(context.Context, int64, string) error
	fnRemoveFromPl  func(context.Context, int64, string) error
}

func (b *testBackend) SetChannelState(ctx context.Context, channelID string, state domain.SubscriptionState) error {
	if b.fnSetChanState != nil {
		return b.fnSetChanState(ctx, channelID, state)
	}
	return b.nopBackend.SetChannelState(ctx, channelID, state)
}

func (b *testBackend) SaveChannelVideos(ctx context.Context, channelID string, videos []domain.Video) error {
	if b.fnSaveChanVids != nil {
		return b.fnSaveChanVids(ctx, channelID, videos)
	}
	return b.nopBackend.SaveChannelVideos(ctx, channelID, videos)
}

func (b *testBackend) ChannelLatestN(ctx context.Context, channelURL, channelID string, n int) ([]domain.Video, error) {
	if b.fnChanLatestN != nil {
		return b.fnChanLatestN(ctx, channelURL, channelID, n)
	}
	return b.nopBackend.ChannelLatestN(ctx, channelURL, channelID, n)
}

func (b *testBackend) SaveVideoSBSegments(ctx context.Context, videoID string, segs []domain.SBSegment) error {
	if b.fnSaveSBSegs != nil {
		return b.fnSaveSBSegs(ctx, videoID, segs)
	}
	return b.nopBackend.SaveVideoSBSegments(ctx, videoID, segs)
}

func (b *testBackend) AddToPlaylist(ctx context.Context, playlistID int64, videoID string) error {
	if b.fnAddToPlaylist != nil {
		return b.fnAddToPlaylist(ctx, playlistID, videoID)
	}
	return b.nopBackend.AddToPlaylist(ctx, playlistID, videoID)
}

func (b *testBackend) RemoveFromPlaylist(ctx context.Context, playlistID int64, videoID string) error {
	if b.fnRemoveFromPl != nil {
		return b.fnRemoveFromPl(ctx, playlistID, videoID)
	}
	return b.nopBackend.RemoveFromPlaylist(ctx, playlistID, videoID)
}

func (b *testBackend) CheckAvailability(ctx context.Context) ([]config.ConfigIssue, error) {
	if b.fnCheckAvail != nil {
		return b.fnCheckAvail(ctx)
	}
	return b.nopBackend.CheckAvailability(ctx)
}

func (b *testBackend) ListProfiles(ctx context.Context) ([]string, error) {
	if b.fnListProfiles != nil {
		return b.fnListProfiles(ctx)
	}
	return b.nopBackend.ListProfiles(ctx)
}

func (b *testBackend) GetProfile(ctx context.Context, name string) ([]byte, bool, error) {
	if b.fnGetProfile != nil {
		return b.fnGetProfile(ctx, name)
	}
	return b.nopBackend.GetProfile(ctx, name)
}

func (b *testBackend) SaveProfile(ctx context.Context, name string, data []byte) error {
	if b.fnSaveProfile != nil {
		return b.fnSaveProfile(ctx, name, data)
	}
	return b.nopBackend.SaveProfile(ctx, name, data)
}

func (b *testBackend) Export(ctx context.Context, opts portability.ExportOptions) (portability.Bundle, error) {
	if b.fnExport != nil {
		return b.fnExport(ctx, opts)
	}
	return b.nopBackend.Export(ctx, opts)
}

func (b *testBackend) ImportPreview(ctx context.Context, bundle portability.Bundle, opts portability.ImportOptions) (portability.ImportPlan, error) {
	if b.fnImportPreview != nil {
		return b.fnImportPreview(ctx, bundle, opts)
	}
	return b.nopBackend.ImportPreview(ctx, bundle, opts)
}

func (b *testBackend) ImportApply(ctx context.Context, bundle portability.Bundle, opts portability.ImportOptions) (portability.ImportResult, error) {
	if b.fnImportApply != nil {
		return b.fnImportApply(ctx, bundle, opts)
	}
	return b.nopBackend.ImportApply(ctx, bundle, opts)
}

func (b *testBackend) AllChannels(ctx context.Context) ([]domain.Channel, error) {
	if b.fnAllChannels != nil {
		return b.fnAllChannels(ctx)
	}
	return b.nopBackend.AllChannels(ctx)
}

func (b *testBackend) BlockChannel(ctx context.Context, ch domain.Channel) error {
	if b.fnBlockChannel != nil {
		return b.fnBlockChannel(ctx, ch)
	}
	return b.nopBackend.BlockChannel(ctx, ch)
}

func (b *testBackend) DeleteAllLocalFiles(ctx context.Context) (int, error) {
	if b.fnDeleteAllLoc != nil {
		return b.fnDeleteAllLoc(ctx)
	}
	return b.nopBackend.DeleteAllLocalFiles(ctx)
}

func (b *testBackend) ClearDownloads(ctx context.Context) error {
	if b.fnClearDl != nil {
		return b.fnClearDl(ctx)
	}
	return b.nopBackend.ClearDownloads(ctx)
}

var _ api.Backend = (*testBackend)(nil)

func (b *testBackend) LocalVideos(ctx context.Context) ([]domain.LocalVideo, error) {
	if b.fnLocalVideos != nil {
		return b.fnLocalVideos(ctx)
	}
	return b.nopBackend.LocalVideos(ctx)
}

func (b *testBackend) VideoPosition(ctx context.Context, id string) (int64, bool, error) {
	if b.fnVideoPosition != nil {
		return b.fnVideoPosition(ctx, id)
	}
	return b.nopBackend.VideoPosition(ctx, id)
}

func (b *testBackend) SaveVideoPosition(ctx context.Context, id string, ms int64) error {
	if b.fnSaveVideoPos != nil {
		return b.fnSaveVideoPos(ctx, id, ms)
	}
	return b.nopBackend.SaveVideoPosition(ctx, id, ms)
}

func (b *testBackend) HasLocalVideo(ctx context.Context, id string) (domain.LocalVideo, bool, error) {
	if b.fnHasLocalVideo != nil {
		return b.fnHasLocalVideo(ctx, id)
	}
	return b.nopBackend.HasLocalVideo(ctx, id)
}

func (b *testBackend) Events(ctx context.Context) (<-chan api.Event, error) {
	if b.fnEvents != nil {
		return b.fnEvents(ctx)
	}
	return b.nopBackend.Events(ctx)
}

// newRoundTripSrv mounts all Connect handlers and the media handler on an
// httptest.Server, wrapped in the same httpauth.Bearer middleware the
// production daemon wraps its whole mux in, and returns a Remote client wired
// to it. Wrapping with the real middleware (rather than mounting bare
// handlers) is what makes this a round-trip test of what ships (C-1) — a
// prior version omitted the wrapping entirely, so it stayed green while the
// production stack 401'd every ticket-authenticated /media/ request.
// api.Backend satisfies media.LocalVideoStore (HasLocalVideo signature matches).
func newRoundTripSrv(t *testing.T, b api.Backend, token string) (*httptest.Server, *api.Remote) {
	t.Helper()
	mux := http.NewServeMux()
	transport.Mount(mux, b, token)
	mux.Handle("/media/", media.Handler(b, token))
	srv := httptest.NewServer(httpauth.Bearer(token, mux))
	t.Cleanup(srv.Close)
	return srv, api.NewRemote(srv.URL, token, &http.Client{})
}

// ── tests ──────────────────────────────────────────────────────────────────────

func TestRoundTripLocalVideos(t *testing.T) {
	now := time.Now().UTC()
	want := domain.LocalVideo{
		ID:             "vid1",
		Title:          "Test Video",
		Channel:        "TestChan",
		Duration:       1234,
		ViewCount:      5678,
		UploadDate:     "2024-01-01",
		FilePath:       "/data/vid1.mp4",
		DownloadType:   "video",
		Status:         domain.VideoStatus("watched"),
		LastPositionMs: 42000,
		DownloadedAt:   now,
		LastPlayed:     now.Add(-time.Hour),
	}
	b := &testBackend{
		fnLocalVideos: func(_ context.Context) ([]domain.LocalVideo, error) {
			return []domain.LocalVideo{want}, nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	got, err := remote.LocalVideos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 video, got %d", len(got))
	}
	v := got[0]
	if v.ID != want.ID {
		t.Errorf("ID: want %q, got %q", want.ID, v.ID)
	}
	if v.Title != want.Title {
		t.Errorf("Title: want %q, got %q", want.Title, v.Title)
	}
	if v.Duration != want.Duration {
		t.Errorf("Duration: want %d, got %d", want.Duration, v.Duration)
	}
	if v.LastPositionMs != want.LastPositionMs {
		t.Errorf("LastPositionMs: want %d, got %d", want.LastPositionMs, v.LastPositionMs)
	}
	if v.Status != want.Status {
		t.Errorf("Status: want %q, got %q", want.Status, v.Status)
	}
	if !v.DownloadedAt.Equal(want.DownloadedAt) {
		t.Errorf("DownloadedAt: want %v, got %v", want.DownloadedAt, v.DownloadedAt)
	}
	if !v.LastPlayed.Equal(want.LastPlayed) {
		t.Errorf("LastPlayed: want %v, got %v", want.LastPlayed, v.LastPlayed)
	}
}

func TestRoundTripLocalVideosZeroTimestamps(t *testing.T) {
	want := domain.LocalVideo{ID: "vid2", Title: "no timestamps"}
	b := &testBackend{
		fnLocalVideos: func(_ context.Context) ([]domain.LocalVideo, error) {
			return []domain.LocalVideo{want}, nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	got, err := remote.LocalVideos(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 video, got %d", len(got))
	}
	if !got[0].DownloadedAt.IsZero() {
		t.Errorf("DownloadedAt: want zero time, got %v", got[0].DownloadedAt)
	}
	if !got[0].LastPlayed.IsZero() {
		t.Errorf("LastPlayed: want zero time, got %v", got[0].LastPlayed)
	}
}

func TestRoundTripVideoPosition(t *testing.T) {
	var savedID string
	var savedMS int64
	b := &testBackend{
		fnSaveVideoPos: func(_ context.Context, id string, ms int64) error {
			savedID = id
			savedMS = ms
			return nil
		},
		fnVideoPosition: func(_ context.Context, _ string) (int64, bool, error) {
			return 12345, true, nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.SaveVideoPosition(context.Background(), "v1", 99000); err != nil {
		t.Fatal(err)
	}
	if savedID != "v1" || savedMS != 99000 {
		t.Errorf("SaveVideoPosition: want (v1, 99000), got (%s, %d)", savedID, savedMS)
	}

	ms, found, err := remote.VideoPosition(context.Background(), "v1")
	if err != nil {
		t.Fatalf("VideoPosition: %v", err)
	}
	if !found || ms != 12345 {
		t.Errorf("VideoPosition: want (12345, true), got (%d, %v)", ms, found)
	}
}

func TestRoundTripResolveSourceFallback(t *testing.T) {
	// nopBackend.HasLocalVideo returns false → handler returns the fallback URL unchanged.
	_, remote := newRoundTripSrv(t, &testBackend{}, "")

	const fallback = "https://youtu.be/vid1"
	src, err := remote.ResolveSource(context.Background(), "vid1", fallback)
	if err != nil {
		t.Fatal(err)
	}
	if src.URI != fallback {
		t.Errorf("want fallback URI %q, got %q", fallback, src.URI)
	}
}

// TestRoundTripResolveSourceTicket verifies the full ticket flow:
// daemon mints a ticket → Remote prefixes baseURL → ticket grants access to
// /media/ even though the request carries no Authorization header. Because
// newRoundTripSrv wraps the mux in the real httpauth.Bearer middleware, this
// exercises the exact production stack the C-1 fix targets.
func TestRoundTripResolveSourceTicket(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "video-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, writeErr := f.WriteString("video content"); writeErr != nil {
		t.Fatal(writeErr)
	}
	f.Close()

	const token = "testtoken"
	b := &testBackend{
		fnHasLocalVideo: func(_ context.Context, id string) (domain.LocalVideo, bool, error) {
			return domain.LocalVideo{ID: id, FilePath: f.Name()}, true, nil
		},
	}
	srv, remote := newRoundTripSrv(t, b, token)

	src, err := remote.ResolveSource(context.Background(), "vid1", "https://youtu.be/vid1")
	if err != nil {
		t.Fatal(err)
	}
	// Remote must have prepended its baseURL to the relative /media/ URI.
	if !strings.HasPrefix(src.URI, srv.URL+"/media/vid1?t=") {
		t.Fatalf("unexpected URI: %s", src.URI)
	}
	// The ticket must grant access — GET the URI directly (no bearer header).
	resp, err := http.Get(src.URI) //nolint:noctx // test-only GET to check ticket auth
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("media endpoint with ticket: want 200, got %d", resp.StatusCode)
	}
}

// TestRoundTripBearerRequiredForNonMedia verifies the production auth wrapping
// is actually exercised: an unauthenticated client must be rejected by a
// non-media RPC when a token is configured.
func TestRoundTripBearerRequiredForNonMedia(t *testing.T) {
	srv, _ := newRoundTripSrv(t, &testBackend{}, "s3cr3t")

	unauth := api.NewRemote(srv.URL, "", &http.Client{})
	if _, err := unauth.LocalVideos(context.Background()); err == nil {
		t.Fatal("expected an error for an unauthenticated request, got nil")
	}
}

func TestRoundTripEvents(t *testing.T) {
	want := api.Event{
		Kind:    api.EventDownloadProgress,
		VideoID: "vid1",
		Detail:  "50%",
	}
	b := &testBackend{
		fnEvents: func(_ context.Context) (<-chan api.Event, error) {
			ch := make(chan api.Event, 1)
			ch <- want
			close(ch)
			return ch, nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := remote.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case got, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before receiving event")
		}
		if got.Kind != want.Kind || got.VideoID != want.VideoID || got.Detail != want.Detail {
			t.Errorf("event mismatch: want %+v, got %+v", want, got)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for event")
	}
}

// TestRoundTripAllChannels verifies the new AllChannels RPC carries the Phase
// 1/2 fields (subscription_state, blocked) across the wire — they were absent
// from the proto Channel message before Phase 3.
func TestRoundTripAllChannels(t *testing.T) {
	want := domain.Channel{
		ID:             "ch1",
		Name:           "Chan One",
		Alias:          "one",
		Tags:           []string{"news", "tech"},
		URL:            "https://youtube.com/ch1",
		State:          domain.SubLocal,
		Blocked:        false,
		LastActivityAt: 1785000000,
	}
	b := &testBackend{
		fnAllChannels: func(_ context.Context) ([]domain.Channel, error) {
			return []domain.Channel{want, {ID: "blk", Name: "Blocked", State: domain.SubNone, Blocked: true}}, nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	got, err := remote.AllChannels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 channels, got %d", len(got))
	}
	if got[0].State != domain.SubLocal {
		t.Errorf("State: want %q, got %q", domain.SubLocal, got[0].State)
	}
	if len(got[0].Tags) != 2 || got[0].Tags[0] != "news" {
		t.Errorf("Tags: want [news tech], got %v", got[0].Tags)
	}
	if got[0].LastActivityAt != 1785000000 {
		t.Errorf("LastActivityAt: want 1785000000, got %d", got[0].LastActivityAt)
	}
	if !got[1].Blocked || got[1].State != domain.SubNone {
		t.Errorf("blocked channel: want blocked+none, got blocked=%v state=%q", got[1].Blocked, got[1].State)
	}
}

// TestRoundTripExport verifies the Portability Export RPC carries a full bundle
// across the wire as opaque JSON bytes and decodes back into the typed Bundle,
// including the opt-in watch-data flag reaching the daemon.
func TestRoundTripExport(t *testing.T) {
	var gotOpts portability.ExportOptions
	want := portability.Bundle{
		SchemaVersion: portability.SchemaVersion,
		Channels: []portability.ChannelExport{
			{ChannelID: "ch1", Name: "One", Tags: []string{"news"}, SubscriptionState: "subscribed_yt"},
			{ChannelID: "blk", Name: "Blocked", SubscriptionState: "none", Blocked: true},
		},
		BlockedNames: []string{"Spammer"},
		Playlists:    []portability.PlaylistExport{{Name: "Favs", VideoIDs: []string{"v1"}}},
		Videos:       []portability.VideoExport{{ID: "v1", Title: "One"}},
		History:      []portability.HistoryExport{{VideoID: "v1", EventType: "playVideo", Timestamp: 123}},
	}
	b := &testBackend{
		fnExport: func(_ context.Context, opts portability.ExportOptions) (portability.Bundle, error) {
			gotOpts = opts
			return want, nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	got, err := remote.Export(context.Background(), portability.ExportOptions{IncludeWatchData: true})
	if err != nil {
		t.Fatal(err)
	}
	if !gotOpts.IncludeWatchData {
		t.Error("IncludeWatchData flag did not reach the daemon")
	}
	if got.SchemaVersion != want.SchemaVersion {
		t.Errorf("SchemaVersion: want %d, got %d", want.SchemaVersion, got.SchemaVersion)
	}
	if len(got.Channels) != 2 || got.Channels[1].ChannelID != "blk" || !got.Channels[1].Blocked {
		t.Errorf("Channels round-trip mismatch: %+v", got.Channels)
	}
	if len(got.BlockedNames) != 1 || got.BlockedNames[0] != "Spammer" {
		t.Errorf("BlockedNames mismatch: %v", got.BlockedNames)
	}
	if len(got.Playlists) != 1 || got.Playlists[0].VideoIDs[0] != "v1" {
		t.Errorf("Playlists mismatch: %+v", got.Playlists)
	}
	if len(got.History) != 1 || got.History[0].Timestamp != 123 {
		t.Errorf("History mismatch: %+v", got.History)
	}
}

// TestRoundTripImport verifies the ImportPreview and ImportApply RPCs carry the
// bundle + opt-in flags to the daemon and decode the JSON-bytes plan/result back.
func TestRoundTripImport(t *testing.T) {
	sent := portability.Bundle{
		SchemaVersion: portability.SchemaVersion,
		Channels:      []portability.ChannelExport{{ChannelID: "c1", SubscriptionState: "subscribed_yt"}},
	}
	var previewOpts, applyOpts portability.ImportOptions
	var gotBundle portability.Bundle
	b := &testBackend{
		fnImportPreview: func(_ context.Context, bundle portability.Bundle, opts portability.ImportOptions) (portability.ImportPlan, error) {
			gotBundle, previewOpts = bundle, opts
			return portability.ImportPlan{SchemaVersion: bundle.SchemaVersion, Compatible: true, NewChannels: 1}, nil
		},
		fnImportApply: func(_ context.Context, _ portability.Bundle, opts portability.ImportOptions) (portability.ImportResult, error) {
			applyOpts = opts
			return portability.ImportResult{ChannelsUpserted: 1}, nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	plan, err := remote.ImportPreview(context.Background(), sent, portability.ImportOptions{IncludeWatchData: true})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Compatible || plan.NewChannels != 1 {
		t.Errorf("plan round-trip mismatch: %+v", plan)
	}
	if len(gotBundle.Channels) != 1 || gotBundle.Channels[0].ChannelID != "c1" {
		t.Errorf("bundle did not reach daemon: %+v", gotBundle.Channels)
	}
	if !previewOpts.IncludeWatchData {
		t.Error("IncludeWatchData flag did not reach the daemon on preview")
	}

	res, err := remote.ImportApply(context.Background(), sent, portability.ImportOptions{ConvertYTToLocal: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.ChannelsUpserted != 1 {
		t.Errorf("result round-trip mismatch: %+v", res)
	}
	if !applyOpts.ConvertYTToLocal {
		t.Error("ConvertYTToLocal flag did not reach the daemon on apply")
	}
}

// TestRoundTripBlockChannelError confirms a backend error on the BlockChannel
// RPC propagates back to the Remote caller (e.g. domain.ErrChannelBlocked).
func TestRoundTripBlockChannelError(t *testing.T) {
	b := &testBackend{
		fnBlockChannel: func(_ context.Context, _ domain.Channel) error {
			return domain.ErrChannelBlocked
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	err := remote.BlockChannel(context.Background(), domain.Channel{ID: "ch1"})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("error should mention blocked, got %v", err)
	}
}

// TestRoundTripDeleteAllLocalFiles confirms the bulk-delete count crosses the
// wire (Phase 10).
func TestRoundTripDeleteAllLocalFiles(t *testing.T) {
	called := false
	b := &testBackend{
		fnDeleteAllLoc: func(_ context.Context) (int, error) {
			called = true
			return 7, nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	got, err := remote.DeleteAllLocalFiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("backend DeleteAllLocalFiles was not invoked")
	}
	if got != 7 {
		t.Errorf("deleted count: want 7, got %d", got)
	}
}

// TestRoundTripClearDownloads confirms the queue-only ClearDownloads reaches the
// backend over the wire and returns no error (Phase 10 — no more file paths).
func TestRoundTripClearDownloads(t *testing.T) {
	called := false
	b := &testBackend{
		fnClearDl: func(_ context.Context) error {
			called = true
			return nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.ClearDownloads(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("backend ClearDownloads was not invoked")
	}
}

// TestRoundTripProfiles verifies the Phase 18 ProfileService carries opaque
// profile bytes + names across the wire in both directions, including the
// GetProfile found=false "not on the daemon yet" case.
func TestRoundTripProfiles(t *testing.T) {
	var savedName string
	var savedData []byte
	blob := []byte(`{"theme":"gruvbox"}`)
	b := &testBackend{
		fnListProfiles: func(_ context.Context) ([]string, error) {
			return []string{"team", "personal"}, nil
		},
		fnGetProfile: func(_ context.Context, name string) ([]byte, bool, error) {
			if name == "team" {
				return blob, true, nil
			}
			return nil, false, nil
		},
		fnSaveProfile: func(_ context.Context, name string, data []byte) error {
			savedName, savedData = name, data
			return nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")
	ctx := context.Background()

	names, err := remote.ListProfiles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "team" {
		t.Errorf("ListProfiles: want [team personal], got %v", names)
	}

	data, found, err := remote.GetProfile(ctx, "team")
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(data, blob) {
		t.Errorf("GetProfile(team): want (%s, true), got (%s, %v)", blob, data, found)
	}

	if _, found, err := remote.GetProfile(ctx, "absent"); err != nil || found {
		t.Errorf("GetProfile(absent): want (_, false, nil), got found=%v err=%v", found, err)
	}

	if err := remote.SaveProfile(ctx, "team", blob); err != nil {
		t.Fatal(err)
	}
	if savedName != "team" || !bytes.Equal(savedData, blob) {
		t.Errorf("SaveProfile reached daemon as (%s, %s), want (team, %s)", savedName, savedData, blob)
	}
}

// TestRoundTripCheckAvailability verifies the Phase 20 HealthService carries the
// daemon-side availability probe's issues across the wire with severity
// preserved, and that the Remote adapter tags them as daemon-originated so a
// remote user knows the fault is server-side.
func TestRoundTripCheckAvailability(t *testing.T) {
	b := &testBackend{
		fnCheckAvail: func(_ context.Context) ([]config.ConfigIssue, error) {
			return []config.ConfigIssue{
				{Severity: config.SeverityError, Message: "yt-dlp not found on PATH"},
				{Severity: config.SeverityWarning, Message: "no cookie source configured"},
			}, nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	got, err := remote.CheckAvailability(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 issues, got %d", len(got))
	}
	if got[0].Severity != config.SeverityError || got[1].Severity != config.SeverityWarning {
		t.Errorf("severities not preserved across the wire: %+v", got)
	}
	if !strings.HasPrefix(got[0].Message, "daemon: ") {
		t.Errorf("issue not tagged as daemon-originated: %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, "yt-dlp not found on PATH") {
		t.Errorf("original message lost: %q", got[0].Message)
	}
}

// TestRoundTripCheckAvailabilityHealthy confirms a clean daemon environment
// crosses the wire as no issues (not an error).
func TestRoundTripCheckAvailabilityHealthy(t *testing.T) {
	_, remote := newRoundTripSrv(t, &testBackend{}, "")

	got, err := remote.CheckAvailability(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("want no issues from a healthy daemon, got %v", got)
	}
}

// TestRoundTripSetChannelState verifies the channelID + the typed
// SubscriptionState arg reach the daemon intact — the state is marshaled as a
// bare string on the wire (state: string(state)), so this guards that the enum
// value survives the string round-trip.
func TestRoundTripSetChannelState(t *testing.T) {
	var gotID string
	var gotState domain.SubscriptionState
	b := &testBackend{
		fnSetChanState: func(_ context.Context, channelID string, state domain.SubscriptionState) error {
			gotID, gotState = channelID, state
			return nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.SetChannelState(context.Background(), "ch1", domain.SubLocal); err != nil {
		t.Fatal(err)
	}
	if gotID != "ch1" {
		t.Errorf("channelID: want ch1, got %q", gotID)
	}
	if gotState != domain.SubLocal {
		t.Errorf("state: want %q, got %q", domain.SubLocal, gotState)
	}
}

// TestRoundTripSaveChannelVideos verifies both the channelID and the []Video
// payload cross the wire intact — the videos go through protoconv.VideosToProto
// on send, so this guards that per-field values (ID/Title/Duration/ViewCount)
// survive that conversion.
func TestRoundTripSaveChannelVideos(t *testing.T) {
	var gotID string
	var gotVids []domain.Video
	sent := []domain.Video{
		{ID: "v1", Title: "First", Channel: "Chan", ChannelID: "ch1", Duration: 601, ViewCount: 12345, UploadDate: "20240101", URL: "https://youtu.be/v1"},
		{ID: "v2", Title: "Second", Channel: "Chan", ChannelID: "ch1", Duration: 42, ViewCount: 7, UploadDate: "20240202", URL: "https://youtu.be/v2"},
	}
	b := &testBackend{
		fnSaveChanVids: func(_ context.Context, channelID string, videos []domain.Video) error {
			gotID, gotVids = channelID, videos
			return nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.SaveChannelVideos(context.Background(), "ch1", sent); err != nil {
		t.Fatal(err)
	}
	if gotID != "ch1" {
		t.Errorf("channelID: want ch1, got %q", gotID)
	}
	if len(gotVids) != 2 {
		t.Fatalf("want 2 videos, got %d", len(gotVids))
	}
	if gotVids[0].ID != "v1" || gotVids[0].Title != "First" || gotVids[0].Duration != 601 || gotVids[0].ViewCount != 12345 {
		t.Errorf("video[0] mismatch: %+v", gotVids[0])
	}
	if gotVids[1].ID != "v2" || gotVids[1].UploadDate != "20240202" {
		t.Errorf("video[1] mismatch: %+v", gotVids[1])
	}
}

// TestRoundTripChannelLatestN verifies the channelURL/channelID/n args reach the
// daemon and the []Video return value comes back intact (n is int32 on the wire).
func TestRoundTripChannelLatestN(t *testing.T) {
	var gotURL, gotID string
	var gotN int
	want := []domain.Video{
		{ID: "v1", Title: "Latest", ChannelID: "ch1", Duration: 300, ViewCount: 99},
	}
	b := &testBackend{
		fnChanLatestN: func(_ context.Context, channelURL, channelID string, n int) ([]domain.Video, error) {
			gotURL, gotID, gotN = channelURL, channelID, n
			return want, nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	got, err := remote.ChannelLatestN(context.Background(), "https://youtube.com/ch1", "ch1", 5)
	if err != nil {
		t.Fatal(err)
	}
	if gotURL != "https://youtube.com/ch1" || gotID != "ch1" || gotN != 5 {
		t.Errorf("args mismatch: url=%q id=%q n=%d", gotURL, gotID, gotN)
	}
	if len(got) != 1 || got[0].ID != "v1" || got[0].Duration != 300 || got[0].ViewCount != 99 {
		t.Errorf("returned video mismatch: %+v", got)
	}
}

// TestRoundTripSaveVideoSBSegments verifies the videoID + the []SBSegment
// payload (float Start/End timestamps) survive the wire — SBSegments were part
// of the C-1 protoconv data-loss finding, so this guards their fractional
// timestamps specifically.
func TestRoundTripSaveVideoSBSegments(t *testing.T) {
	var gotID string
	var gotSegs []domain.SBSegment
	sent := []domain.SBSegment{
		{Start: 12.5, End: 34.75},
		{Start: 120.0, End: 145.25},
	}
	b := &testBackend{
		fnSaveSBSegs: func(_ context.Context, videoID string, segs []domain.SBSegment) error {
			gotID, gotSegs = videoID, segs
			return nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.SaveVideoSBSegments(context.Background(), "vid1", sent); err != nil {
		t.Fatal(err)
	}
	if gotID != "vid1" {
		t.Errorf("videoID: want vid1, got %q", gotID)
	}
	if len(gotSegs) != 2 {
		t.Fatalf("want 2 segments, got %d", len(gotSegs))
	}
	if gotSegs[0].Start != 12.5 || gotSegs[0].End != 34.75 {
		t.Errorf("segment[0] timestamps lost: %+v", gotSegs[0])
	}
	if gotSegs[1].Start != 120.0 || gotSegs[1].End != 145.25 {
		t.Errorf("segment[1] timestamps lost: %+v", gotSegs[1])
	}
}

// TestRoundTripAddToPlaylist verifies the (playlistID int64, videoID) args reach
// the daemon intact — the int64 playlistID must not be truncated on the wire.
func TestRoundTripAddToPlaylist(t *testing.T) {
	var gotPlaylistID int64
	var gotVideoID string
	b := &testBackend{
		fnAddToPlaylist: func(_ context.Context, playlistID int64, videoID string) error {
			gotPlaylistID, gotVideoID = playlistID, videoID
			return nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.AddToPlaylist(context.Background(), 4200000000, "vid1"); err != nil {
		t.Fatal(err)
	}
	if gotPlaylistID != 4200000000 {
		t.Errorf("playlistID: want 4200000000, got %d", gotPlaylistID)
	}
	if gotVideoID != "vid1" {
		t.Errorf("videoID: want vid1, got %q", gotVideoID)
	}
}

// TestRoundTripRemoveFromPlaylist covers the other arg-carrying playlist-order
// mutation (there is no dedicated reorder RPC; add + remove are how ordering is
// edited). It guards the same (playlistID int64, videoID) pair on the way out.
func TestRoundTripRemoveFromPlaylist(t *testing.T) {
	var gotPlaylistID int64
	var gotVideoID string
	b := &testBackend{
		fnRemoveFromPl: func(_ context.Context, playlistID int64, videoID string) error {
			gotPlaylistID, gotVideoID = playlistID, videoID
			return nil
		},
	}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.RemoveFromPlaylist(context.Background(), 77, "vidX"); err != nil {
		t.Fatal(err)
	}
	if gotPlaylistID != 77 {
		t.Errorf("playlistID: want 77, got %d", gotPlaylistID)
	}
	if gotVideoID != "vidX" {
		t.Errorf("videoID: want vidX, got %q", gotVideoID)
	}
}
