package transport_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// These round-trip tests exercise the *write* direction of the proto seam:
// Remote client → Connect over httptest → transport handler → protoconv →
// backend. They assert the backend receives the full domain value the client
// sent, guarding the inbound mirror of the data-loss class the read-side tests
// cover (dropped chapters / SB segments / links / video fields on decode).
// See docs/ARCH-REVIEW-2026-08-07.md, finding H-3.

// writeRec records the domain values handed to the backend by the write
// handlers, so a test can assert nothing was dropped on the way in.
type writeRec struct {
	apitest.NopBackend

	videoID   string
	channelID string

	chapters []domain.Chapter
	segments []domain.SBSegment
	links    []domain.Link
	videos   []domain.Video
	channels []domain.Channel
	tags     []string
	state    domain.SubscriptionState
	activity domain.ActivityEntry

	// UpsertVideo / SaveVideoDetailsCache scalar captures.
	upsert      domain.Video
	cacheDesc   string
	cacheThumb  string
	cacheSubs   int64
	forcedError error
}

func (b *writeRec) SaveVideoChapters(_ context.Context, id string, c []domain.Chapter) error {
	b.videoID, b.chapters = id, c
	return b.forcedError
}

func (b *writeRec) SaveVideoSBSegments(_ context.Context, id string, s []domain.SBSegment) error {
	b.videoID, b.segments = id, s
	return b.forcedError
}

func (b *writeRec) SaveVideoLinks(_ context.Context, id string, l []domain.Link) error {
	b.videoID, b.links = id, l
	return b.forcedError
}

func (b *writeRec) UpsertVideo(_ context.Context, id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error {
	b.upsert = domain.Video{
		ID: id, Title: title, Channel: channel, ChannelID: channelID,
		Duration: duration, ViewCount: viewCount, UploadDate: uploadDate, URL: url,
	}
	return b.forcedError
}

func (b *writeRec) SaveVideoDetailsCache(_ context.Context, id, description, thumbnailURL string, subscribers int64) error {
	b.videoID, b.cacheDesc, b.cacheThumb, b.cacheSubs = id, description, thumbnailURL, subscribers
	return b.forcedError
}

func (b *writeRec) SaveChannelVideos(_ context.Context, id string, v []domain.Video) error {
	b.channelID, b.videos = id, v
	return b.forcedError
}

func (b *writeRec) SaveSubscribedChannels(_ context.Context, c []domain.Channel) error {
	b.channels = c
	return b.forcedError
}

func (b *writeRec) SetChannelTags(_ context.Context, id string, tags []string) error {
	b.channelID, b.tags = id, tags
	return b.forcedError
}

func (b *writeRec) SetChannelState(_ context.Context, id string, s domain.SubscriptionState) error {
	b.channelID, b.state = id, s
	return b.forcedError
}

func (b *writeRec) LogActivity(_ context.Context, e domain.ActivityEntry) error {
	b.activity = e
	return b.forcedError
}

func TestRoundTripSaveVideoChapters(t *testing.T) {
	want := []domain.Chapter{
		{Title: "Intro", OriginalStart: 0, OriginalEnd: 30, AdjustedStart: 0, AdjustedEnd: 28},
		{Title: "Body", OriginalStart: 30, OriginalEnd: 600, AdjustedStart: 28, AdjustedEnd: 590},
	}
	be := &writeRec{}
	r := newRemote(t, be, "")

	if err := r.SaveVideoChapters(context.Background(), "vid1", want); err != nil {
		t.Fatalf("SaveVideoChapters: %v", err)
	}
	if be.videoID != "vid1" {
		t.Errorf("videoID not forwarded: got %q", be.videoID)
	}
	if !reflect.DeepEqual(be.chapters, want) {
		t.Errorf("chapters not preserved on decode:\n got  %+v\n want %+v", be.chapters, want)
	}
}

func TestRoundTripSaveVideoSBSegments(t *testing.T) {
	want := []domain.SBSegment{{Start: 100, End: 120}, {Start: 300.5, End: 315.25}}
	be := &writeRec{}
	r := newRemote(t, be, "")

	if err := r.SaveVideoSBSegments(context.Background(), "vid1", want); err != nil {
		t.Fatalf("SaveVideoSBSegments: %v", err)
	}
	if be.videoID != "vid1" || !reflect.DeepEqual(be.segments, want) {
		t.Errorf("SB segments not preserved: id=%q got %+v want %+v", be.videoID, be.segments, want)
	}
}

func TestRoundTripSaveVideoLinks(t *testing.T) {
	want := []domain.Link{
		{Label: "Homepage", URL: "https://example.com"},
		{Label: "", URL: "https://bare.example"},
	}
	be := &writeRec{}
	r := newRemote(t, be, "")

	if err := r.SaveVideoLinks(context.Background(), "vid1", want); err != nil {
		t.Fatalf("SaveVideoLinks: %v", err)
	}
	if be.videoID != "vid1" || !reflect.DeepEqual(be.links, want) {
		t.Errorf("links not preserved: id=%q got %+v want %+v", be.videoID, be.links, want)
	}
}

func TestRoundTripUpsertVideo(t *testing.T) {
	want := domain.Video{
		ID: "v9", Title: "Title", Channel: "Chan", ChannelID: "c9",
		Duration: 617, ViewCount: 987654, UploadDate: "20260715", URL: "https://youtu.be/v9",
	}
	be := &writeRec{}
	r := newRemote(t, be, "")

	if err := r.UpsertVideo(context.Background(), want.ID, want.Title, want.Channel, want.ChannelID, want.Duration, want.ViewCount, want.UploadDate, want.URL); err != nil {
		t.Fatalf("UpsertVideo: %v", err)
	}
	if !reflect.DeepEqual(be.upsert, want) {
		t.Errorf("UpsertVideo args not preserved:\n got  %+v\n want %+v", be.upsert, want)
	}
}

func TestRoundTripSaveVideoDetailsCache(t *testing.T) {
	be := &writeRec{}
	r := newRemote(t, be, "")

	if err := r.SaveVideoDetailsCache(context.Background(), "vid1", "a description", "https://img/t.jpg", 4200); err != nil {
		t.Fatalf("SaveVideoDetailsCache: %v", err)
	}
	if be.videoID != "vid1" || be.cacheDesc != "a description" || be.cacheThumb != "https://img/t.jpg" || be.cacheSubs != 4200 {
		t.Errorf("cache args not preserved: id=%q desc=%q thumb=%q subs=%d",
			be.videoID, be.cacheDesc, be.cacheThumb, be.cacheSubs)
	}
}

func TestRoundTripSaveChannelVideos(t *testing.T) {
	// Video is all-primitive and uses the same protoconv pair as the read path,
	// so full equality is the right assertion here.
	want := []domain.Video{
		{ID: "a", Title: "A", Channel: "C", ChannelID: "c1", Duration: 100, ViewCount: 9, UploadDate: "20260101", URL: "https://a"},
		{ID: "b", Title: "B", Channel: "C2", ChannelID: "c2", Duration: 5, ViewCount: 0, UploadDate: "20250102", URL: "https://b"},
	}
	be := &writeRec{}
	r := newRemote(t, be, "")

	if err := r.SaveChannelVideos(context.Background(), "ch1", want); err != nil {
		t.Fatalf("SaveChannelVideos: %v", err)
	}
	if be.channelID != "ch1" {
		t.Errorf("channelID not forwarded: got %q", be.channelID)
	}
	if !reflect.DeepEqual(be.videos, want) {
		t.Errorf("channel videos not preserved:\n got  %+v\n want %+v", be.videos, want)
	}
}

func TestRoundTripSetChannelTags(t *testing.T) {
	want := []string{"tech", "go", "tui"}
	be := &writeRec{}
	r := newRemote(t, be, "")

	if err := r.SetChannelTags(context.Background(), "ch1", want); err != nil {
		t.Fatalf("SetChannelTags: %v", err)
	}
	if be.channelID != "ch1" || !reflect.DeepEqual(be.tags, want) {
		t.Errorf("tags not preserved: id=%q got %v want %v", be.channelID, be.tags, want)
	}
}

func TestRoundTripSetChannelState(t *testing.T) {
	be := &writeRec{}
	r := newRemote(t, be, "")

	if err := r.SetChannelState(context.Background(), "ch1", domain.SubLocal); err != nil {
		t.Fatalf("SetChannelState: %v", err)
	}
	if be.channelID != "ch1" || be.state != domain.SubLocal {
		t.Errorf("state not preserved: id=%q state=%q", be.channelID, be.state)
	}
}

func TestRoundTripSaveSubscribedChannels(t *testing.T) {
	// Channel carries deprecated/derived fields (IsLocal), so assert the durable
	// ones rather than full equality — mirrors TestRoundTripSubscribedChannels.
	want := domain.Channel{
		ID: "ch1", Name: "Chan", Alias: "MyAlias", Tags: []string{"tech", "go"},
		URL: "https://c", Subscribers: 4200, State: domain.SubYT, Blocked: false,
		VideosRefreshedAt: 1_700_000_000,
	}
	be := &writeRec{}
	r := newRemote(t, be, "")

	if err := r.SaveSubscribedChannels(context.Background(), []domain.Channel{want}); err != nil {
		t.Fatalf("SaveSubscribedChannels: %v", err)
	}
	if len(be.channels) != 1 {
		t.Fatalf("got %d channels, want 1", len(be.channels))
	}
	c := be.channels[0]
	if c.ID != want.ID || c.Name != want.Name || c.Alias != want.Alias ||
		c.URL != want.URL || c.Subscribers != want.Subscribers || c.State != want.State ||
		c.Blocked != want.Blocked || c.VideosRefreshedAt != want.VideosRefreshedAt {
		t.Errorf("channel fields not preserved on decode: %+v", c)
	}
	if !reflect.DeepEqual(c.Tags, want.Tags) {
		t.Errorf("channel tags = %v, want %v", c.Tags, want.Tags)
	}
}

func TestRoundTripLogActivity(t *testing.T) {
	// ActivityEntry carries a time.Time (DeepEqual-fragile across the seam), so
	// assert the primitive fields that must survive.
	want := domain.ActivityEntry{
		Type: "add_to_playlist", IsLocal: true, ChannelID: "c1", ChannelName: "Chan",
		PlaylistID: "PL1", PlaylistLocalID: "local:7", PlaylistName: "Watch", VideoID: "v1", VideoTitle: "VT",
	}
	be := &writeRec{}
	r := newRemote(t, be, "")

	if err := r.LogActivity(context.Background(), want); err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	g := be.activity
	if g.Type != want.Type || g.IsLocal != want.IsLocal || g.ChannelID != want.ChannelID ||
		g.ChannelName != want.ChannelName || g.PlaylistID != want.PlaylistID ||
		g.PlaylistLocalID != want.PlaylistLocalID || g.PlaylistName != want.PlaylistName ||
		g.VideoID != want.VideoID || g.VideoTitle != want.VideoTitle {
		t.Errorf("activity fields not preserved: %+v", g)
	}
}

// TestRoundTripBackendErrorPropagates confirms a backend error surfaces to the
// Remote client as a non-nil error through the rpcErr → Connect path.
func TestRoundTripBackendErrorPropagates(t *testing.T) {
	be := &writeRec{forcedError: errors.New("backend boom")}
	r := newRemote(t, be, "")

	err := r.SaveVideoLinks(context.Background(), "vid1", []domain.Link{{URL: "https://x"}})
	if err == nil {
		t.Fatal("expected error to propagate from backend, got nil")
	}
}
