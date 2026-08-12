package transport_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/backend/httpauth"
	"github.com/EugeneShtoka/yt-tui/internal/backend/transport"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// These are black-box round-trip tests over the real remote-mode boundary:
// Remote client → Connect over an httptest server → transport handler →
// protoconv → backend, and back. They are the coverage the in-process (InProc)
// tests can never provide, and they guard the data-loss regressions the audit
// flagged (dropped Language / SBSegments / chapters / links at the proto seam).

// richBackend serves canned rich domain values and records request inputs.
type richBackend struct {
	apitest.NopBackend
	gotVideoURL   string
	gotChannelURL string
	gotChannelID  string
	gotPlaylistID string
	gotHistoryLim int
	details       domain.VideoDetails
	cache         domain.CachedDetails
	cacheFound    bool
	recommended   []domain.Video
	channelVideos []domain.Video
	playlistVids  []domain.Video
	history       []domain.HistoryEntry
	localVideos   []domain.LocalVideo
	channels      []domain.Channel
}

func (b *richBackend) SubscribedChannels(_ context.Context) ([]domain.Channel, error) {
	return b.channels, nil
}

func (b *richBackend) VideoDetails(_ context.Context, url string) (domain.VideoDetails, error) {
	b.gotVideoURL = url
	return b.details, nil
}

func (b *richBackend) GetVideoDetailsCache(_ context.Context, _ string) (domain.CachedDetails, bool, error) {
	return b.cache, b.cacheFound, nil
}

func (b *richBackend) Recommended(_ context.Context) ([]domain.Video, error) {
	return b.recommended, nil
}

func (b *richBackend) ChannelVideos(_ context.Context, url, id string) ([]domain.Video, error) {
	b.gotChannelURL, b.gotChannelID = url, id
	return b.channelVideos, nil
}

func (b *richBackend) LocalPlaylistVideos(_ context.Context, id string) ([]domain.Video, error) {
	b.gotPlaylistID = id
	return b.playlistVids, nil
}

func (b *richBackend) History(_ context.Context, limit int) ([]domain.HistoryEntry, error) {
	b.gotHistoryLim = limit
	return b.history, nil
}

func (b *richBackend) LocalVideos(_ context.Context) ([]domain.LocalVideo, error) {
	return b.localVideos, nil
}

// newRemote mounts the real transport handlers (optionally behind the bearer
// middleware) on an httptest server and returns a Remote client wired to it.
func newRemote(t *testing.T, backend api.Backend, token string) *api.Remote {
	t.Helper()
	mux := http.NewServeMux()
	transport.Mount(mux, backend, token)
	srv := httptest.NewServer(httpauth.Bearer(token, mux))
	t.Cleanup(srv.Close)
	return api.NewRemote(srv.URL, token, srv.Client())
}

func sampleDetails() domain.VideoDetails {
	return domain.VideoDetails{
		Video: domain.Video{
			ID: "vid1", Title: "T", Channel: "C", ChannelID: "ch1",
			Duration: 610, ViewCount: 12345, UploadDate: "20260715", URL: "https://youtu.be/vid1",
		},
		Description:  "desc with https://example.com",
		ThumbnailURL: "https://img/thumb.jpg",
		Subscribers:  99000,
		Language:     "ru",
		Chapters: []domain.RawChapter{
			{Title: "Intro", StartTime: 0, EndTime: 30},
			{Title: "Body", StartTime: 30, EndTime: 600},
		},
		SBSegments: []domain.SBSegment{{Start: 100, End: 120}, {Start: 300, End: 315}},
	}
}

func TestRoundTripVideoDetailsPreservesRichFields(t *testing.T) {
	want := sampleDetails()
	be := &richBackend{details: want}
	r := newRemote(t, be, "")

	got, err := r.VideoDetails(context.Background(), "https://youtu.be/vid1")
	if err != nil {
		t.Fatalf("VideoDetails: %v", err)
	}
	if be.gotVideoURL != "https://youtu.be/vid1" {
		t.Errorf("request videoURL not forwarded: got %q", be.gotVideoURL)
	}
	// Full equality catches ANY dropped field — the point of the guard is that
	// Language, SBSegments and Chapters survive the proto seam.
	if !reflect.DeepEqual(got, want) {
		t.Errorf("VideoDetails round-trip mismatch:\n got  %+v\n want %+v", got, want)
	}
}

func TestRoundTripCachedDetailsPreservesPointerFields(t *testing.T) {
	links := []domain.Link{{Label: "Homepage", URL: "https://l.example"}}
	chaps := []domain.Chapter{{Title: "c", OriginalStart: 0, OriginalEnd: 10, AdjustedStart: 0, AdjustedEnd: 9}}
	sb := []domain.SBSegment{{Start: 1, End: 2}}
	want := domain.CachedDetails{
		Description: "d", ThumbnailURL: "t", Subscribers: 5,
		Links: &links, Chapters: &chaps, SBSegments: &sb,
	}
	be := &richBackend{cache: want, cacheFound: true}
	r := newRemote(t, be, "")

	got, found, err := r.GetVideoDetailsCache(context.Background(), "vid1")
	if err != nil {
		t.Fatalf("GetVideoDetailsCache: %v", err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CachedDetails round-trip mismatch:\n got  %+v\n want %+v", got, want)
	}
}

func TestRoundTripCacheMissReturnsNotFound(t *testing.T) {
	be := &richBackend{cacheFound: false}
	r := newRemote(t, be, "")

	_, found, err := r.GetVideoDetailsCache(context.Background(), "vid1")
	if err != nil {
		t.Fatalf("GetVideoDetailsCache: %v", err)
	}
	if found {
		t.Error("found = true for a cache miss, want false")
	}
}

func TestRoundTripVideoListsPreserveFields(t *testing.T) {
	vids := []domain.Video{
		{ID: "a", Title: "A", Channel: "C", ChannelID: "c1", Duration: 100, ViewCount: 9, UploadDate: "20260101", URL: "https://a"},
		{ID: "b", Title: "B", Channel: "C2", ChannelID: "c2", Duration: 5, ViewCount: 0, UploadDate: "20250102", URL: "https://b"},
	}
	be := &richBackend{recommended: vids, channelVideos: vids}
	r := newRemote(t, be, "")

	rec, err := r.Recommended(context.Background())
	if err != nil {
		t.Fatalf("Recommended: %v", err)
	}
	if !reflect.DeepEqual(rec, vids) {
		t.Errorf("Recommended round-trip mismatch:\n got  %+v\n want %+v", rec, vids)
	}

	chVids, err := r.ChannelVideos(context.Background(), "https://chan", "ch1")
	if err != nil {
		t.Fatalf("ChannelVideos: %v", err)
	}
	if be.gotChannelURL != "https://chan" || be.gotChannelID != "ch1" {
		t.Errorf("ChannelVideos args not forwarded: url=%q id=%q", be.gotChannelURL, be.gotChannelID)
	}
	if !reflect.DeepEqual(chVids, vids) {
		t.Errorf("ChannelVideos round-trip mismatch:\n got  %+v\n want %+v", chVids, vids)
	}
}

func TestRoundTripLocalPlaylistVideos(t *testing.T) {
	vids := []domain.Video{{ID: "p1", Title: "PV", Channel: "C", ChannelID: "c1", Duration: 42, ViewCount: 7, UploadDate: "20260201", URL: "https://p1"}}
	be := &richBackend{playlistVids: vids}
	r := newRemote(t, be, "")

	got, err := r.LocalPlaylistVideos(context.Background(), "local:7")
	if err != nil {
		t.Fatalf("LocalPlaylistVideos: %v", err)
	}
	if be.gotPlaylistID != "local:7" {
		t.Errorf("playlistID not forwarded: got %q", be.gotPlaylistID)
	}
	if !reflect.DeepEqual(got, vids) {
		t.Errorf("playlist videos round-trip mismatch:\n got  %+v\n want %+v", got, vids)
	}
}

func TestRoundTripHistory(t *testing.T) {
	// HistoryEntry carries a time.Time (DeepEqual-fragile across the proto seam),
	// so assert the primitive fields that must survive.
	be := &richBackend{history: []domain.HistoryEntry{
		{ID: 1, VideoID: "h1", Title: "HT", Channel: "C", ChannelID: "c1", Duration: 10, ViewCount: 3, UploadDate: "20260101", EventType: "streamVideo", Details: "d"},
	}}
	r := newRemote(t, be, "")

	got, err := r.History(context.Background(), 50)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if be.gotHistoryLim != 50 {
		t.Errorf("limit not forwarded: got %d", be.gotHistoryLim)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	e := got[0]
	if e.VideoID != "h1" || e.EventType != "streamVideo" || e.Title != "HT" || e.Duration != 10 {
		t.Errorf("history entry fields not preserved: %+v", e)
	}
}

func TestRoundTripLocalVideos(t *testing.T) {
	// LocalVideo carries time.Time fields; assert the durable primitives.
	be := &richBackend{localVideos: []domain.LocalVideo{
		{ID: "l1", Title: "LT", Channel: "C", Duration: 20, ViewCount: 4, UploadDate: "20260101", FilePath: "/v/l1.mp4", FileSize: 1234, DownloadType: "video", LastPositionMs: 5000},
	}}
	r := newRemote(t, be, "")

	got, err := r.LocalVideos(context.Background())
	if err != nil {
		t.Fatalf("LocalVideos: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d local videos, want 1", len(got))
	}
	v := got[0]
	if v.ID != "l1" || v.Title != "LT" || v.FilePath != "/v/l1.mp4" || v.FileSize != 1234 || v.LastPositionMs != 5000 {
		t.Errorf("local video fields not preserved: %+v", v)
	}
}

func TestRoundTripSubscribedChannelsPreserveRichFields(t *testing.T) {
	want := domain.Channel{
		ID: "ch1", Name: "Chan", Alias: "MyAlias", Tags: []string{"tech", "go"},
		URL: "https://c", Subscribers: 4200, Blocked: false, VideosRefreshedAt: 1_700_000_000,
	}
	be := &richBackend{channels: []domain.Channel{want}}
	r := newRemote(t, be, "")

	got, err := r.SubscribedChannels(context.Background())
	if err != nil {
		t.Fatalf("SubscribedChannels: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d channels, want 1", len(got))
	}
	c := got[0]
	if c.ID != want.ID || c.Name != want.Name || c.Alias != want.Alias ||
		c.Subscribers != want.Subscribers || c.Blocked != want.Blocked ||
		c.VideosRefreshedAt != want.VideosRefreshedAt {
		t.Errorf("channel rich fields not preserved: %+v", c)
	}
	if !reflect.DeepEqual(c.Tags, want.Tags) {
		t.Errorf("channel tags = %v, want %v", c.Tags, want.Tags)
	}
}

// TestRoundTripBearerAuth exercises the production auth middleware on the real
// transport path: a matching token succeeds, a wrong token is rejected.
func TestRoundTripBearerAuth(t *testing.T) {
	be := &richBackend{details: sampleDetails()}

	ok := newRemote(t, be, "s3cret")
	if _, err := ok.VideoDetails(context.Background(), "u"); err != nil {
		t.Errorf("matching token rejected: %v", err)
	}

	mux := http.NewServeMux()
	transport.Mount(mux, be, "s3cret")
	srv := httptest.NewServer(httpauth.Bearer("s3cret", mux))
	t.Cleanup(srv.Close)
	bad := api.NewRemote(srv.URL, "wrong-token", srv.Client())
	if _, err := bad.VideoDetails(context.Background(), "u"); err == nil {
		t.Error("wrong token accepted, want an auth error")
	}
}
