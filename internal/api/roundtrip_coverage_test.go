//nolint:wrapcheck // test stub — delegates to the apitest fake; errors are irrelevant
package api_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// recBackend is a recording fake for the ~75 RPC methods not already covered by
// testBackend's fn* fields. It embeds nopBackend (zero-value everything), holds
// canned return values, and records the args each handler decoded off the wire.
// Only the methods a given test exercises are overridden; everything else falls
// through to the nop so recBackend still satisfies the full api.Backend contract.
type recBackend struct {
	nopBackend

	// canned returns
	retVideos       []domain.Video
	retChannels     []domain.Channel
	retVidMap       map[string]domain.Video
	retStrMap       map[string]bool
	retInt64Map     map[string]int64
	retDetails      domain.VideoDetails
	retCached       domain.CachedDetails
	retCachedOK     bool
	retThumb        []byte
	retThumbOK      bool
	retTranscript   string
	retTranscriptOK bool
	retLocalHas     domain.LocalVideo
	retLocalHasOK   bool
	retYTPlaylists  []domain.YTPlaylist
	retPlaylists    []domain.Playlist
	retPlVideoIDs   []string
	retNewPlID      string
	retNewYTPlID    string
	retHistory      []domain.HistoryEntry
	retActivity     []domain.ActivityEntry
	retDownloads    []api.DownloadItem
	retHidden       int
	retPlayed       int
	retSearchCh     []domain.Channel
	retSearchVid    []domain.Video

	// recorded args
	gotFeed          string
	gotVideoID       string
	gotChannelID     string
	gotChannelIDs    []string
	gotVideos        []domain.Video
	gotChapters      []domain.Chapter
	gotLinks         []domain.Link
	gotChannel       domain.Channel
	gotChannels      []domain.Channel
	gotActivity      domain.ActivityEntry
	gotYTPls         []domain.YTPlaylist
	gotLocal         domain.LocalVideo
	gotVideo         domain.Video
	gotAudioOnly     bool
	gotTags          []string
	gotAlias         string
	gotStatus        domain.VideoStatus
	gotPlaylistIDStr string
	// UpsertVideo scalar args
	gotUpDuration int
	gotUpViews    int64
	gotUpTitle    string
	// SaveVideoDetailsCache scalar args
	gotDesc     string
	gotThumbURL string
	gotSubs     int64
}

var _ api.Backend = (*recBackend)(nil)

// ── Feed ─────────────────────────────────────────────────────────────────────

func (b *recBackend) Recommended(context.Context) ([]domain.Video, error) {
	return b.retVideos, nil
}
func (b *recBackend) GetFeedCache(_ context.Context, feed string) ([]domain.Video, error) {
	b.gotFeed = feed
	return b.retVideos, nil
}
func (b *recBackend) SaveFeedCache(_ context.Context, feed string, videos []domain.Video) error {
	b.gotFeed, b.gotVideos = feed, videos
	return nil
}
func (b *recBackend) HiddenRecVideoIDs(context.Context) (map[string]bool, error) {
	return b.retStrMap, nil
}
func (b *recBackend) WatchedVideoIDs(context.Context) (map[string]bool, error) {
	return b.retStrMap, nil
}

// ── Channel ──────────────────────────────────────────────────────────────────

func (b *recBackend) Search(_ context.Context, _ string) ([]domain.Channel, []domain.Video, error) {
	return b.retSearchCh, b.retSearchVid, nil
}
func (b *recBackend) ChannelVideos(_ context.Context, _, channelID string) ([]domain.Video, error) {
	b.gotChannelID = channelID
	return b.retVideos, nil
}
func (b *recBackend) GetChannelVideos(_ context.Context, channelID string) ([]domain.Video, error) {
	b.gotChannelID = channelID
	return b.retVideos, nil
}
func (b *recBackend) GetAllChannelVideos(_ context.Context, ids []string) ([]domain.Video, error) {
	b.gotChannelIDs = ids
	return b.retVideos, nil
}
func (b *recBackend) GetChannelLatestAll(context.Context) (map[string]domain.Video, error) {
	return b.retVidMap, nil
}
func (b *recBackend) ChannelHideStats(_ context.Context, channelID string) (int, int, error) {
	b.gotChannelID = channelID
	return b.retHidden, b.retPlayed, nil
}
func (b *recBackend) SubscribedChannels(context.Context) ([]domain.Channel, error) {
	return b.retChannels, nil
}
func (b *recBackend) GetSubscribedChannels(context.Context) ([]domain.Channel, error) {
	return b.retChannels, nil
}
func (b *recBackend) BlockedChannels(context.Context) ([]domain.Channel, error) {
	return b.retChannels, nil
}
func (b *recBackend) Subscribe(_ context.Context, ch domain.Channel) error {
	b.gotChannel = ch
	return nil
}
func (b *recBackend) AddSubscribedChannel(_ context.Context, ch domain.Channel) error {
	b.gotChannel = ch
	return nil
}
func (b *recBackend) SaveSubscribedChannels(_ context.Context, channels []domain.Channel) error {
	b.gotChannels = channels
	return nil
}
func (b *recBackend) SetChannelTags(_ context.Context, channelID string, tags []string) error {
	b.gotChannelID, b.gotTags = channelID, tags
	return nil
}
func (b *recBackend) SetChannelAlias(_ context.Context, channelID, alias string) error {
	b.gotChannelID, b.gotAlias = channelID, alias
	return nil
}

// ── Video ────────────────────────────────────────────────────────────────────

func (b *recBackend) VideoDetails(_ context.Context, _ string) (domain.VideoDetails, error) {
	return b.retDetails, nil
}
func (b *recBackend) GetVideoDetailsCache(_ context.Context, videoID string) (domain.CachedDetails, bool, error) {
	b.gotVideoID = videoID
	return b.retCached, b.retCachedOK, nil
}
func (b *recBackend) GetThumbnail(_ context.Context, videoID, _ string) ([]byte, bool, error) {
	b.gotVideoID = videoID
	return b.retThumb, b.retThumbOK, nil
}
func (b *recBackend) GetTranscript(_ context.Context, videoID, _ string) (string, bool, error) {
	b.gotVideoID = videoID
	return b.retTranscript, b.retTranscriptOK, nil
}
func (b *recBackend) HasLocalVideo(_ context.Context, videoID string) (domain.LocalVideo, bool, error) {
	b.gotVideoID = videoID
	return b.retLocalHas, b.retLocalHasOK, nil
}
func (b *recBackend) AllVideoPositions(context.Context) (map[string]int64, error) {
	return b.retInt64Map, nil
}
func (b *recBackend) UpsertVideo(_ context.Context, id, title, _, channelID string, duration int, viewCount int64, _, _ string) error {
	b.gotVideoID, b.gotUpTitle, b.gotChannelID = id, title, channelID
	b.gotUpDuration, b.gotUpViews = duration, viewCount
	return nil
}
func (b *recBackend) SetVideoStatus(_ context.Context, id string, status domain.VideoStatus) error {
	b.gotVideoID, b.gotStatus = id, status
	return nil
}
func (b *recBackend) SaveVideoDetailsCache(_ context.Context, videoID, description, thumbnailURL string, subscribers int64) error {
	b.gotVideoID, b.gotDesc, b.gotThumbURL, b.gotSubs = videoID, description, thumbnailURL, subscribers
	return nil
}
func (b *recBackend) SaveVideoChapters(_ context.Context, videoID string, chapters []domain.Chapter) error {
	b.gotVideoID, b.gotChapters = videoID, chapters
	return nil
}
func (b *recBackend) SaveVideoLinks(_ context.Context, videoID string, links []domain.Link) error {
	b.gotVideoID, b.gotLinks = videoID, links
	return nil
}

// ── Library ──────────────────────────────────────────────────────────────────

func (b *recBackend) AddLocalVideo(_ context.Context, v domain.LocalVideo) error {
	b.gotLocal = v
	return nil
}

// ── Playlist ─────────────────────────────────────────────────────────────────

func (b *recBackend) LocalPlaylists(context.Context) ([]domain.Playlist, error) {
	return b.retPlaylists, nil
}
func (b *recBackend) LocalPlaylistVideos(_ context.Context, _ string) ([]domain.Video, error) {
	return b.retVideos, nil
}
func (b *recBackend) PlaylistVideoIDs(_ context.Context, _ string) ([]string, error) {
	return b.retPlVideoIDs, nil
}
func (b *recBackend) CreatePlaylist(_ context.Context, _ string) (string, error) {
	return b.retNewPlID, nil
}
func (b *recBackend) CreateYTPlaylist(_ context.Context, _ string) (string, error) {
	return b.retNewYTPlID, nil
}
func (b *recBackend) YTPlaylists(context.Context) ([]domain.YTPlaylist, error) {
	return b.retYTPlaylists, nil
}
func (b *recBackend) GetYTPlaylists(context.Context) ([]domain.YTPlaylist, error) {
	return b.retYTPlaylists, nil
}
func (b *recBackend) YTPlaylistVideos(_ context.Context, playlistID string) ([]domain.Video, error) {
	b.gotPlaylistIDStr = playlistID
	return b.retVideos, nil
}
func (b *recBackend) GetYTPlaylistVideos(_ context.Context, playlistID string) ([]domain.Video, error) {
	b.gotPlaylistIDStr = playlistID
	return b.retVideos, nil
}
func (b *recBackend) SaveYTPlaylists(_ context.Context, playlists []domain.YTPlaylist) error {
	b.gotYTPls = playlists
	return nil
}
func (b *recBackend) SaveYTPlaylistVideos(_ context.Context, playlistID string, videos []domain.Video) error {
	b.gotPlaylistIDStr, b.gotVideos = playlistID, videos
	return nil
}

// ── History ──────────────────────────────────────────────────────────────────

func (b *recBackend) History(_ context.Context, _ int) ([]domain.HistoryEntry, error) {
	return b.retHistory, nil
}
func (b *recBackend) HistoryVideos(_ context.Context, _ int) ([]domain.HistoryEntry, error) {
	return b.retHistory, nil
}
func (b *recBackend) VideoHistory(_ context.Context, videoID string) ([]domain.HistoryEntry, error) {
	b.gotVideoID = videoID
	return b.retHistory, nil
}
func (b *recBackend) ActivityLog(_ context.Context, _ int) ([]domain.ActivityEntry, error) {
	return b.retActivity, nil
}
func (b *recBackend) LogActivity(_ context.Context, e domain.ActivityEntry) error {
	b.gotActivity = e
	return nil
}

// ── Download ─────────────────────────────────────────────────────────────────

func (b *recBackend) DownloadItems(context.Context) ([]api.DownloadItem, error) {
	return b.retDownloads, nil
}
func (b *recBackend) Enqueue(_ context.Context, video domain.Video, audioOnly bool) error {
	b.gotVideo, b.gotAudioOnly = video, audioOnly
	return nil
}

// ── Tier 1: DEEP field-level round-trips ─────────────────────────────────────

// videoFields asserts a domain.Video survived protoconv (both directions) with
// every scalar field intact. int Duration is int32 on the wire; ViewCount is
// int64 — both must not truncate.
func assertVideo(t *testing.T, got, want domain.Video) {
	t.Helper()
	if got.ID != want.ID || got.Title != want.Title || got.Channel != want.Channel ||
		got.ChannelID != want.ChannelID || got.Duration != want.Duration ||
		got.ViewCount != want.ViewCount || got.UploadDate != want.UploadDate || got.URL != want.URL {
		t.Errorf("video mismatch:\n want %+v\n  got %+v", want, got)
	}
}

func sampleVideos() []domain.Video {
	return []domain.Video{
		{ID: "v1", Title: "First", Channel: "Chan", ChannelID: "ch1", Duration: 601, ViewCount: 12_345_678_901, UploadDate: "20240101", URL: "https://youtu.be/v1"},
		{ID: "v2", Title: "Second", Channel: "Chan", ChannelID: "ch1", Duration: 42, ViewCount: 7, UploadDate: "20240202", URL: "https://youtu.be/v2"},
	}
}

// TestRoundTripRecommended guards the Recommended read carrying a full []Video
// (incl. a >2^32 ViewCount) through protoconv.VideosToProto/ProtoToVideos.
func TestRoundTripRecommended(t *testing.T) {
	want := sampleVideos()
	_, remote := newRoundTripSrv(t, &recBackend{retVideos: want}, "")
	got, err := remote.Recommended(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 videos, got %d", len(got))
	}
	assertVideo(t, got[0], want[0])
	assertVideo(t, got[1], want[1])
}

// TestRoundTripChannelVideoReads covers the four channel-video read RPCs that
// return []Video: the videoID/channelID arg must reach the daemon and the
// returned videos must survive the wire.
func TestRoundTripChannelVideoReads(t *testing.T) {
	want := sampleVideos()
	ctx := context.Background()

	t.Run("ChannelVideos", func(t *testing.T) {
		b := &recBackend{retVideos: want}
		_, remote := newRoundTripSrv(t, b, "")
		got, err := remote.ChannelVideos(ctx, "https://youtube.com/ch1", "ch1")
		if err != nil {
			t.Fatal(err)
		}
		if b.gotChannelID != "ch1" {
			t.Errorf("channelID: want ch1, got %q", b.gotChannelID)
		}
		if len(got) != 2 {
			t.Fatalf("want 2, got %d", len(got))
		}
		assertVideo(t, got[0], want[0])
	})

	t.Run("GetChannelVideos", func(t *testing.T) {
		b := &recBackend{retVideos: want}
		_, remote := newRoundTripSrv(t, b, "")
		got, err := remote.GetChannelVideos(ctx, "ch1")
		if err != nil {
			t.Fatal(err)
		}
		if b.gotChannelID != "ch1" {
			t.Errorf("channelID: want ch1, got %q", b.gotChannelID)
		}
		assertVideo(t, got[0], want[0])
	})

	t.Run("GetAllChannelVideos", func(t *testing.T) {
		b := &recBackend{retVideos: want}
		_, remote := newRoundTripSrv(t, b, "")
		got, err := remote.GetAllChannelVideos(ctx, []string{"ch1", "ch2"})
		if err != nil {
			t.Fatal(err)
		}
		if len(b.gotChannelIDs) != 2 || b.gotChannelIDs[1] != "ch2" {
			t.Errorf("channelIDs: want [ch1 ch2], got %v", b.gotChannelIDs)
		}
		assertVideo(t, got[1], want[1])
	})

	t.Run("GetChannelLatestAll", func(t *testing.T) {
		b := &recBackend{retVidMap: map[string]domain.Video{"ch1": want[0]}}
		_, remote := newRoundTripSrv(t, b, "")
		got, err := remote.GetChannelLatestAll(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Fatalf("want 1 entry, got %d", len(got))
		}
		assertVideo(t, got["ch1"], want[0])
	})
}

// TestRoundTripGetFeedCache guards the feed arg reaching the daemon and the
// cached []Video coming back intact.
func TestRoundTripGetFeedCache(t *testing.T) {
	want := sampleVideos()
	b := &recBackend{retVideos: want}
	_, remote := newRoundTripSrv(t, b, "")
	got, err := remote.GetFeedCache(context.Background(), "recommended")
	if err != nil {
		t.Fatal(err)
	}
	if b.gotFeed != "recommended" {
		t.Errorf("feed: want recommended, got %q", b.gotFeed)
	}
	assertVideo(t, got[0], want[0])
}

// TestRoundTripVideoDetails is the C-1 guard. protoconv once silently dropped
// VideoDetails.Language and .SBSegments; this asserts BOTH survive the wire, plus
// the embedded Video, the raw Chapters (with fractional float timestamps), and
// the fractional SBSegment Start/End — the exact fields the C-1 data-loss bug lost.
func TestRoundTripVideoDetails(t *testing.T) {
	want := domain.VideoDetails{
		Video:        domain.Video{ID: "v1", Title: "Deep", Channel: "Chan", ChannelID: "ch1", Duration: 300, ViewCount: 999, UploadDate: "20240101", URL: "https://youtu.be/v1"},
		Description:  "a description",
		ThumbnailURL: "https://img/thumb.jpg",
		Subscribers:  4200,
		Language:     "ru", // C-1: was dropped
		Chapters: []domain.RawChapter{
			{Title: "Intro", StartTime: 0.0, EndTime: 12.5},
			{Title: "Main", StartTime: 12.5, EndTime: 305.75},
		},
		SBSegments: []domain.SBSegment{ // C-1: was dropped
			{Start: 30.25, End: 45.5},
		},
	}
	b := &recBackend{retDetails: want}
	_, remote := newRoundTripSrv(t, b, "")

	got, err := remote.VideoDetails(context.Background(), "https://youtu.be/v1")
	if err != nil {
		t.Fatal(err)
	}
	assertVideo(t, got.Video, want.Video)
	if got.Description != want.Description || got.ThumbnailURL != want.ThumbnailURL || got.Subscribers != want.Subscribers {
		t.Errorf("metadata mismatch: %+v", got)
	}
	if got.Language != "ru" {
		t.Errorf("Language (C-1): want ru, got %q", got.Language)
	}
	if len(got.Chapters) != 2 {
		t.Fatalf("Chapters: want 2, got %d", len(got.Chapters))
	}
	if got.Chapters[1].StartTime != 12.5 || got.Chapters[1].EndTime != 305.75 {
		t.Errorf("Chapter fractional timestamps lost: %+v", got.Chapters[1])
	}
	if len(got.SBSegments) != 1 {
		t.Fatalf("SBSegments (C-1): want 1, got %d", len(got.SBSegments))
	}
	if got.SBSegments[0].Start != 30.25 || got.SBSegments[0].End != 45.5 {
		t.Errorf("SBSegment fractional timestamps lost: %+v", got.SBSegments[0])
	}
}

// assertCachedPopulated is the populated-pointers case of GetVideoDetailsCache,
// extracted to keep the parent test under the funlen limit. It asserts each
// non-nil *[]T pointer survives the wire with its fractional timestamps.
func assertCachedPopulated(t *testing.T) {
	t.Helper()
	links := []domain.Link{{Label: "site", URL: "https://x"}}
	chapters := []domain.Chapter{{Title: "C1", OriginalStart: 1.5, OriginalEnd: 2.75, AdjustedStart: 1.0, AdjustedEnd: 2.25}}
	segs := []domain.SBSegment{{Start: 5.5, End: 9.25}}
	want := domain.CachedDetails{
		Description:  "desc",
		ThumbnailURL: "https://t",
		Subscribers:  77,
		Links:        &links,
		Chapters:     &chapters,
		SBSegments:   &segs,
	}
	b := &recBackend{retCached: want, retCachedOK: true}
	_, remote := newRoundTripSrv(t, b, "")

	got, found, err := remote.GetVideoDetailsCache(context.Background(), "v1")
	if err != nil {
		t.Fatal(err)
	}
	if b.gotVideoID != "v1" {
		t.Errorf("videoID: want v1, got %q", b.gotVideoID)
	}
	if !found {
		t.Fatal("found: want true")
	}
	if got.Description != "desc" || got.Subscribers != 77 {
		t.Errorf("scalar mismatch: %+v", got)
	}
	if got.Links == nil || len(*got.Links) != 1 || (*got.Links)[0].URL != "https://x" {
		t.Errorf("Links pointer lost: %+v", got.Links)
	}
	if got.Chapters == nil || (*got.Chapters)[0].OriginalEnd != 2.75 || (*got.Chapters)[0].AdjustedStart != 1.0 {
		t.Errorf("Chapters pointer/timestamps lost: %+v", got.Chapters)
	}
	if got.SBSegments == nil || (*got.SBSegments)[0].Start != 5.5 || (*got.SBSegments)[0].End != 9.25 {
		t.Errorf("SBSegments pointer/timestamps lost: %+v", got.SBSegments)
	}
}

// TestRoundTripGetVideoDetailsCache exercises the pointer-field semantics of
// CachedDetails: Links/Chapters/SBSegments are *[]T where nil ("never parsed")
// must be distinguishable from &[]T{} / populated. The wire carries a
// *Parsed bool per field, so this guards that nil-vs-populated survives — plus
// the fractional Chapter/SBSegment timestamps and the found=true flag.
func TestRoundTripGetVideoDetailsCache(t *testing.T) {
	ctx := context.Background()

	t.Run("populated", func(t *testing.T) { assertCachedPopulated(t) })

	t.Run("nil pointers stay nil", func(t *testing.T) {
		// The *Parsed=false wire flags must round-trip nil pointers as nil, NOT
		// as empty slices — nil means "never parsed" and drives a re-parse.
		want := domain.CachedDetails{Description: "unparsed"}
		b := &recBackend{retCached: want, retCachedOK: true}
		_, remote := newRoundTripSrv(t, b, "")

		got, found, err := remote.GetVideoDetailsCache(ctx, "v2")
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatal("found: want true")
		}
		if got.Links != nil {
			t.Errorf("Links: want nil, got %+v", got.Links)
		}
		if got.Chapters != nil {
			t.Errorf("Chapters: want nil, got %+v", got.Chapters)
		}
		if got.SBSegments != nil {
			t.Errorf("SBSegments: want nil, got %+v", got.SBSegments)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, remote := newRoundTripSrv(t, &recBackend{}, "")
		_, found, err := remote.GetVideoDetailsCache(ctx, "absent")
		if err != nil {
			t.Fatal(err)
		}
		if found {
			t.Error("found: want false for absent cache")
		}
	})
}

// TestRoundTripGetThumbnail guards the videoID arg reaching the daemon and the
// raw image bytes + found flag returning intact (bytes must not be corrupted).
func TestRoundTripGetThumbnail(t *testing.T) {
	blob := []byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0xff, 0x7f}
	b := &recBackend{retThumb: blob, retThumbOK: true}
	_, remote := newRoundTripSrv(t, b, "")

	got, found, err := remote.GetThumbnail(context.Background(), "v1", "https://fallback")
	if err != nil {
		t.Fatal(err)
	}
	if b.gotVideoID != "v1" {
		t.Errorf("videoID: want v1, got %q", b.gotVideoID)
	}
	if !found {
		t.Error("found: want true")
	}
	if !bytes.Equal(got, blob) {
		t.Errorf("thumbnail bytes corrupted: want %v, got %v", blob, got)
	}
}

// TestRoundTripGetTranscript guards the videoID arg + the transcript text/found
// flag surviving the wire.
func TestRoundTripGetTranscript(t *testing.T) {
	b := &recBackend{retTranscript: "hello\nworld", retTranscriptOK: true}
	_, remote := newRoundTripSrv(t, b, "")

	txt, found, err := remote.GetTranscript(context.Background(), "v1", "https://youtu.be/v1")
	if err != nil {
		t.Fatal(err)
	}
	if b.gotVideoID != "v1" {
		t.Errorf("videoID: want v1, got %q", b.gotVideoID)
	}
	if !found || txt != "hello\nworld" {
		t.Errorf("transcript mismatch: found=%v txt=%q", found, txt)
	}
}

// TestRoundTripHasLocalVideo guards the videoID arg and the LocalVideo+found
// return. A false found must be reported honestly (not swallowed) — see the
// H-8 comment in remote_library.go.
func TestRoundTripHasLocalVideo(t *testing.T) {
	now := time.Now().UTC()
	want := domain.LocalVideo{ID: "v1", Title: "Local", FilePath: "/data/v1.mp4", Status: domain.StatusWatched, LastPositionMs: 5000, DownloadedAt: now}
	b := &recBackend{retLocalHas: want, retLocalHasOK: true}
	_, remote := newRoundTripSrv(t, b, "")

	got, found, err := remote.HasLocalVideo(context.Background(), "v1")
	if err != nil {
		t.Fatal(err)
	}
	if b.gotVideoID != "v1" {
		t.Errorf("videoID: want v1, got %q", b.gotVideoID)
	}
	if !found {
		t.Fatal("found: want true")
	}
	if got.ID != want.ID || got.FilePath != want.FilePath || got.Status != want.Status || got.LastPositionMs != want.LastPositionMs {
		t.Errorf("LocalVideo mismatch: %+v", got)
	}
	if !got.DownloadedAt.Equal(now) {
		t.Errorf("DownloadedAt: want %v, got %v", now, got.DownloadedAt)
	}
}

// TestRoundTripChannelReads covers the three channel-list read RPCs that return
// []Channel through protoconv.ChannelsToProto — asserting the State enum, Tags,
// and Blocked flag survive.
func TestRoundTripChannelReads(t *testing.T) {
	want := []domain.Channel{
		{ID: "ch1", Name: "One", Alias: "one", Tags: []string{"news"}, URL: "https://y/ch1", State: domain.SubYT, LastActivityAt: 100},
		{ID: "ch2", Name: "Two", State: domain.SubLocal, Tags: []string{"a", "b"}},
	}
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		call func(*api.Remote) ([]domain.Channel, error)
	}{
		{"SubscribedChannels", func(r *api.Remote) ([]domain.Channel, error) { return r.SubscribedChannels(ctx) }},
		{"GetSubscribedChannels", func(r *api.Remote) ([]domain.Channel, error) { return r.GetSubscribedChannels(ctx) }},
		{"BlockedChannels", func(r *api.Remote) ([]domain.Channel, error) { return r.BlockedChannels(ctx) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, remote := newRoundTripSrv(t, &recBackend{retChannels: want}, "")
			got, err := tc.call(remote)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 {
				t.Fatalf("want 2, got %d", len(got))
			}
			if got[0].State != domain.SubYT || got[1].State != domain.SubLocal {
				t.Errorf("State enum lost: %q %q", got[0].State, got[1].State)
			}
			if len(got[0].Tags) != 1 || got[0].Tags[0] != "news" {
				t.Errorf("Tags lost: %v", got[0].Tags)
			}
			if got[0].LastActivityAt != 100 {
				t.Errorf("LastActivityAt lost: %d", got[0].LastActivityAt)
			}
		})
	}
}

// TestRoundTripSearch guards the two-channel-and-video split return of Search.
func TestRoundTripSearch(t *testing.T) {
	b := &recBackend{
		retSearchCh:  []domain.Channel{{ID: "ch1", Name: "One", State: domain.SubYT}},
		retSearchVid: sampleVideos(),
	}
	_, remote := newRoundTripSrv(t, b, "")

	chs, vids, err := remote.Search(context.Background(), "query")
	if err != nil {
		t.Fatal(err)
	}
	if len(chs) != 1 || chs[0].ID != "ch1" {
		t.Errorf("channels mismatch: %+v", chs)
	}
	if len(vids) != 2 {
		t.Fatalf("want 2 videos, got %d", len(vids))
	}
	assertVideo(t, vids[0], sampleVideos()[0])
}

// TestRoundTripChannelHideStats guards the (hidden, played) int pair — both are
// int32 on the wire and must not swap or truncate.
func TestRoundTripChannelHideStats(t *testing.T) {
	b := &recBackend{retHidden: 12, retPlayed: 34}
	_, remote := newRoundTripSrv(t, b, "")

	hidden, played, err := remote.ChannelHideStats(context.Background(), "ch1")
	if err != nil {
		t.Fatal(err)
	}
	if b.gotChannelID != "ch1" {
		t.Errorf("channelID: want ch1, got %q", b.gotChannelID)
	}
	if hidden != 12 || played != 34 {
		t.Errorf("stats mismatch: hidden=%d played=%d", hidden, played)
	}
}

// TestRoundTripYTPlaylistReads covers the YT playlist read RPCs (list + videos).
func TestRoundTripYTPlaylistReads(t *testing.T) {
	pls := []domain.YTPlaylist{{ID: "PL1", Title: "Favs"}, {ID: "PL2", Title: "Watch"}}
	vids := sampleVideos()
	ctx := context.Background()

	t.Run("YTPlaylists", func(t *testing.T) {
		_, remote := newRoundTripSrv(t, &recBackend{retYTPlaylists: pls}, "")
		got, err := remote.YTPlaylists(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[1].ID != "PL2" || got[1].Title != "Watch" {
			t.Errorf("YTPlaylists mismatch: %+v", got)
		}
	})

	t.Run("GetYTPlaylists", func(t *testing.T) {
		_, remote := newRoundTripSrv(t, &recBackend{retYTPlaylists: pls}, "")
		got, err := remote.GetYTPlaylists(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Title != "Favs" {
			t.Errorf("GetYTPlaylists mismatch: %+v", got)
		}
	})

	t.Run("YTPlaylistVideos", func(t *testing.T) {
		b := &recBackend{retVideos: vids}
		_, remote := newRoundTripSrv(t, b, "")
		got, err := remote.YTPlaylistVideos(ctx, "PL1")
		if err != nil {
			t.Fatal(err)
		}
		if b.gotPlaylistIDStr != "PL1" {
			t.Errorf("playlistID: want PL1, got %q", b.gotPlaylistIDStr)
		}
		assertVideo(t, got[0], vids[0])
	})

	t.Run("GetYTPlaylistVideos", func(t *testing.T) {
		b := &recBackend{retVideos: vids}
		_, remote := newRoundTripSrv(t, b, "")
		got, err := remote.GetYTPlaylistVideos(ctx, "PL2")
		if err != nil {
			t.Fatal(err)
		}
		if b.gotPlaylistIDStr != "PL2" {
			t.Errorf("playlistID: want PL2, got %q", b.gotPlaylistIDStr)
		}
		assertVideo(t, got[1], vids[1])
	})
}

// TestRoundTripLocalPlaylists covers the local-playlist reads: the []Playlist
// (with CreatedAt time), LocalPlaylistVideos, and PlaylistVideoIDs.
func TestRoundTripLocalPlaylists(t *testing.T) {
	now := time.Now().UTC()
	ctx := context.Background()

	t.Run("LocalPlaylists", func(t *testing.T) {
		want := []domain.Playlist{{ID: "local:7", Name: "Favs", CreatedAt: now}}
		_, remote := newRoundTripSrv(t, &recBackend{retPlaylists: want}, "")
		got, err := remote.LocalPlaylists(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "local:7" || got[0].Name != "Favs" {
			t.Fatalf("playlist mismatch: %+v", got)
		}
		if !got[0].CreatedAt.Equal(now) {
			t.Errorf("CreatedAt: want %v, got %v", now, got[0].CreatedAt)
		}
	})

	t.Run("LocalPlaylistVideos", func(t *testing.T) {
		want := sampleVideos()
		_, remote := newRoundTripSrv(t, &recBackend{retVideos: want}, "")
		got, err := remote.LocalPlaylistVideos(ctx, "local:7")
		if err != nil {
			t.Fatal(err)
		}
		assertVideo(t, got[0], want[0])
	})

	t.Run("PlaylistVideoIDs", func(t *testing.T) {
		_, remote := newRoundTripSrv(t, &recBackend{retPlVideoIDs: []string{"a", "b", "c"}}, "")
		got, err := remote.PlaylistVideoIDs(ctx, "local:7")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 || got[2] != "c" {
			t.Errorf("ids mismatch: %v", got)
		}
	})
}

// TestRoundTripCreatePlaylists guards the returned IDs of the two create RPCs:
// a local int64 id and a YT string id.
func TestRoundTripCreatePlaylists(t *testing.T) {
	ctx := context.Background()

	t.Run("CreatePlaylist", func(t *testing.T) {
		_, remote := newRoundTripSrv(t, &recBackend{retNewPlID: "local:4200000000"}, "")
		id, err := remote.CreatePlaylist(ctx, "New")
		if err != nil {
			t.Fatal(err)
		}
		if id != "local:4200000000" {
			t.Errorf("id: want local:4200000000, got %q", id)
		}
	})

	t.Run("CreateYTPlaylist", func(t *testing.T) {
		_, remote := newRoundTripSrv(t, &recBackend{retNewYTPlID: "PLnew"}, "")
		id, err := remote.CreateYTPlaylist(ctx, "New")
		if err != nil {
			t.Fatal(err)
		}
		if id != "PLnew" {
			t.Errorf("id: want PLnew, got %q", id)
		}
	})
}

// TestRoundTripHistoryReads covers the three HistoryEntry read RPCs, asserting
// scalar fields + the Timestamp time.Time survive protoconv.
func TestRoundTripHistoryReads(t *testing.T) {
	now := time.Now().UTC()
	want := []domain.HistoryEntry{{
		ID: 1, VideoID: "v1", Title: "Watched", Channel: "Chan", ChannelID: "ch1",
		Duration: 300, ViewCount: 999, UploadDate: "20240101", EventType: "playVideo",
		Details: "note", Timestamp: now,
	}}
	ctx := context.Background()

	t.Run("History", func(t *testing.T) {
		_, remote := newRoundTripSrv(t, &recBackend{retHistory: want}, "")
		got, err := remote.History(ctx, 10)
		if err != nil {
			t.Fatal(err)
		}
		assertHistory(t, got, now)
	})
	t.Run("HistoryVideos", func(t *testing.T) {
		_, remote := newRoundTripSrv(t, &recBackend{retHistory: want}, "")
		got, err := remote.HistoryVideos(ctx, 10)
		if err != nil {
			t.Fatal(err)
		}
		assertHistory(t, got, now)
	})
	t.Run("VideoHistory", func(t *testing.T) {
		b := &recBackend{retHistory: want}
		_, remote := newRoundTripSrv(t, b, "")
		got, err := remote.VideoHistory(ctx, "v1")
		if err != nil {
			t.Fatal(err)
		}
		if b.gotVideoID != "v1" {
			t.Errorf("videoID: want v1, got %q", b.gotVideoID)
		}
		assertHistory(t, got, now)
	})
}

func assertHistory(t *testing.T, got []domain.HistoryEntry, wantTS time.Time) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	e := got[0]
	if e.ID != 1 || e.VideoID != "v1" || e.Title != "Watched" || e.ChannelID != "ch1" ||
		e.Duration != 300 || e.ViewCount != 999 || e.EventType != "playVideo" || e.Details != "note" {
		t.Errorf("history entry mismatch: %+v", e)
	}
	if !e.Timestamp.Equal(wantTS) {
		t.Errorf("Timestamp: want %v, got %v", wantTS, e.Timestamp)
	}
}

// TestRoundTripActivityLog guards the ActivityEntry read carrying every field
// (both playlist IDs — string YT id and int64 local id — plus the Timestamp).
func TestRoundTripActivityLog(t *testing.T) {
	now := time.Now().UTC()
	want := domain.ActivityEntry{
		ID: 5, Type: "add_to_playlist", IsLocal: true, ChannelID: "ch1", ChannelName: "Chan",
		PlaylistID: "PL1", PlaylistLocalID: "local:42", PlaylistName: "Favs", VideoID: "v1", VideoTitle: "Vid", Timestamp: now,
	}
	_, remote := newRoundTripSrv(t, &recBackend{retActivity: []domain.ActivityEntry{want}}, "")

	got, err := remote.ActivityLog(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	e := got[0]
	if e.ID != 5 || e.Type != "add_to_playlist" || !e.IsLocal || e.ChannelID != "ch1" ||
		e.PlaylistID != "PL1" || e.PlaylistLocalID != "local:42" || e.PlaylistName != "Favs" || e.VideoTitle != "Vid" {
		t.Errorf("activity entry mismatch: %+v", e)
	}
	if !e.Timestamp.Equal(now) {
		t.Errorf("Timestamp: want %v, got %v", now, e.Timestamp)
	}
}

// TestRoundTripDownloadItems guards the DownloadItem read: the float Progress,
// the Status enum, and — the H-9 fix — the Err string being reconstructed into
// an error client-side.
func TestRoundTripDownloadItems(t *testing.T) {
	want := []api.DownloadItem{
		{VideoID: "v1", Title: "One", Channel: "Chan", Duration: 300, URL: "https://y/v1", AudioOnly: true, Progress: 0.42, Speed: "1MB/s", ETA: "10s", FilePath: "/data/v1.mp4"},
	}
	b := &recBackend{retDownloads: want}
	_, remote := newRoundTripSrv(t, b, "")

	got, err := remote.DownloadItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	d := got[0]
	if d.VideoID != "v1" || d.Title != "One" || d.Duration != 300 || !d.AudioOnly || d.FilePath != "/data/v1.mp4" {
		t.Errorf("download item mismatch: %+v", d)
	}
	if d.Progress != 0.42 {
		t.Errorf("Progress float lost: %v", d.Progress)
	}
}

// TestRoundTripAllVideoPositions guards the map[string]int64 return (int64 ms
// positions must not truncate).
func TestRoundTripAllVideoPositions(t *testing.T) {
	want := map[string]int64{"v1": 12_000_000_000, "v2": 5}
	_, remote := newRoundTripSrv(t, &recBackend{retInt64Map: want}, "")

	got, err := remote.AllVideoPositions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["v1"] != 12_000_000_000 || got["v2"] != 5 {
		t.Errorf("positions map mismatch: %v", got)
	}
}

// TestRoundTripHiddenAndWatchedIDs guards the map[string]bool returns.
func TestRoundTripHiddenAndWatchedIDs(t *testing.T) {
	want := map[string]bool{"v1": true, "v2": true}
	ctx := context.Background()

	t.Run("HiddenRecVideoIDs", func(t *testing.T) {
		_, remote := newRoundTripSrv(t, &recBackend{retStrMap: want}, "")
		got, err := remote.HiddenRecVideoIDs(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || !got["v1"] {
			t.Errorf("hidden ids mismatch: %v", got)
		}
	})
	t.Run("WatchedVideoIDs", func(t *testing.T) {
		_, remote := newRoundTripSrv(t, &recBackend{retStrMap: want}, "")
		got, err := remote.WatchedVideoIDs(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || !got["v2"] {
			t.Errorf("watched ids mismatch: %v", got)
		}
	})
}

// ── Tier 1 writes: recording args ─────────────────────────────────────────────

// TestRoundTripSaveFeedCache guards the feed name + []Video reaching the daemon.
func TestRoundTripSaveFeedCache(t *testing.T) {
	sent := sampleVideos()
	b := &recBackend{}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.SaveFeedCache(context.Background(), "recommended", sent); err != nil {
		t.Fatal(err)
	}
	if b.gotFeed != "recommended" {
		t.Errorf("feed: want recommended, got %q", b.gotFeed)
	}
	if len(b.gotVideos) != 2 {
		t.Fatalf("want 2 videos, got %d", len(b.gotVideos))
	}
	assertVideo(t, b.gotVideos[0], sent[0])
}

// TestRoundTripSaveVideoChapters guards the videoID + []Chapter reaching the
// daemon with fractional Original/Adjusted timestamps intact.
func TestRoundTripSaveVideoChapters(t *testing.T) {
	sent := []domain.Chapter{{Title: "Intro", OriginalStart: 0.0, OriginalEnd: 12.5, AdjustedStart: 0.0, AdjustedEnd: 10.25}}
	b := &recBackend{}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.SaveVideoChapters(context.Background(), "v1", sent); err != nil {
		t.Fatal(err)
	}
	if b.gotVideoID != "v1" {
		t.Errorf("videoID: want v1, got %q", b.gotVideoID)
	}
	if len(b.gotChapters) != 1 {
		t.Fatalf("want 1 chapter, got %d", len(b.gotChapters))
	}
	c := b.gotChapters[0]
	if c.Title != "Intro" || c.OriginalEnd != 12.5 || c.AdjustedEnd != 10.25 {
		t.Errorf("chapter fractional timestamps lost: %+v", c)
	}
}

// TestRoundTripSaveVideoLinks guards the videoID + []Link (label+url) reaching
// the daemon.
func TestRoundTripSaveVideoLinks(t *testing.T) {
	sent := []domain.Link{{Label: "Twitter", URL: "https://x.com/a"}, {Label: "", URL: "https://y.com"}}
	b := &recBackend{}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.SaveVideoLinks(context.Background(), "v1", sent); err != nil {
		t.Fatal(err)
	}
	if b.gotVideoID != "v1" {
		t.Errorf("videoID: want v1, got %q", b.gotVideoID)
	}
	if len(b.gotLinks) != 2 || b.gotLinks[0].Label != "Twitter" || b.gotLinks[1].URL != "https://y.com" {
		t.Errorf("links mismatch: %+v", b.gotLinks)
	}
}

// TestRoundTripSaveVideoDetailsCache guards the videoID + description/thumbURL +
// int64 subscribers scalar args reaching the daemon.
func TestRoundTripSaveVideoDetailsCache(t *testing.T) {
	b := &recBackend{}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.SaveVideoDetailsCache(context.Background(), "v1", "desc text", "https://t.jpg", 9_000_000_000); err != nil {
		t.Fatal(err)
	}
	if b.gotVideoID != "v1" || b.gotDesc != "desc text" || b.gotThumbURL != "https://t.jpg" || b.gotSubs != 9_000_000_000 {
		t.Errorf("args mismatch: id=%q desc=%q thumb=%q subs=%d", b.gotVideoID, b.gotDesc, b.gotThumbURL, b.gotSubs)
	}
}

// TestRoundTripSaveYTPlaylists guards the []YTPlaylist reaching the daemon.
func TestRoundTripSaveYTPlaylists(t *testing.T) {
	sent := []domain.YTPlaylist{{ID: "PL1", Title: "Favs"}, {ID: "PL2", Title: "Later"}}
	b := &recBackend{}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.SaveYTPlaylists(context.Background(), sent); err != nil {
		t.Fatal(err)
	}
	if len(b.gotYTPls) != 2 || b.gotYTPls[1].ID != "PL2" || b.gotYTPls[1].Title != "Later" {
		t.Errorf("yt playlists mismatch: %+v", b.gotYTPls)
	}
}

// TestRoundTripSaveYTPlaylistVideos guards the playlistID + []Video reaching the
// daemon.
func TestRoundTripSaveYTPlaylistVideos(t *testing.T) {
	sent := sampleVideos()
	b := &recBackend{}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.SaveYTPlaylistVideos(context.Background(), "PL1", sent); err != nil {
		t.Fatal(err)
	}
	if b.gotPlaylistIDStr != "PL1" {
		t.Errorf("playlistID: want PL1, got %q", b.gotPlaylistIDStr)
	}
	if len(b.gotVideos) != 2 {
		t.Fatalf("want 2 videos, got %d", len(b.gotVideos))
	}
	assertVideo(t, b.gotVideos[1], sent[1])
}

// TestRoundTripChannelWrites covers the three channel-payload writes (Subscribe,
// AddSubscribedChannel) that ship a full Channel through protoconv, plus
// SaveSubscribedChannels which ships a slice — asserting the State enum + Tags
// reach the daemon.
func TestRoundTripChannelWrites(t *testing.T) {
	ch := domain.Channel{ID: "ch1", Name: "One", Alias: "one", Tags: []string{"news"}, URL: "https://y/ch1", State: domain.SubLocal}
	ctx := context.Background()

	t.Run("Subscribe", func(t *testing.T) {
		b := &recBackend{}
		_, remote := newRoundTripSrv(t, b, "")
		if err := remote.Subscribe(ctx, ch); err != nil {
			t.Fatal(err)
		}
		if b.gotChannel.ID != "ch1" || b.gotChannel.State != domain.SubLocal || len(b.gotChannel.Tags) != 1 {
			t.Errorf("subscribe channel mismatch: %+v", b.gotChannel)
		}
	})

	t.Run("AddSubscribedChannel", func(t *testing.T) {
		b := &recBackend{}
		_, remote := newRoundTripSrv(t, b, "")
		if err := remote.AddSubscribedChannel(ctx, ch); err != nil {
			t.Fatal(err)
		}
		if b.gotChannel.ID != "ch1" || b.gotChannel.Alias != "one" {
			t.Errorf("add channel mismatch: %+v", b.gotChannel)
		}
	})

	t.Run("SaveSubscribedChannels", func(t *testing.T) {
		b := &recBackend{}
		_, remote := newRoundTripSrv(t, b, "")
		if err := remote.SaveSubscribedChannels(ctx, []domain.Channel{ch, {ID: "ch2", State: domain.SubYT}}); err != nil {
			t.Fatal(err)
		}
		if len(b.gotChannels) != 2 || b.gotChannels[1].ID != "ch2" || b.gotChannels[1].State != domain.SubYT {
			t.Errorf("save channels mismatch: %+v", b.gotChannels)
		}
	})
}

// TestRoundTripAddLocalVideo guards the full LocalVideo (incl. timestamps)
// reaching the daemon through protoconv.LocalVideoToProto.
func TestRoundTripAddLocalVideo(t *testing.T) {
	now := time.Now().UTC()
	sent := domain.LocalVideo{ID: "v1", Title: "Local", Channel: "Chan", Duration: 300, ViewCount: 99, FilePath: "/data/v1.mp4", DownloadType: "audio", Status: domain.StatusStarted, LastPositionMs: 3000, DownloadedAt: now}
	b := &recBackend{}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.AddLocalVideo(context.Background(), sent); err != nil {
		t.Fatal(err)
	}
	g := b.gotLocal
	if g.ID != "v1" || g.DownloadType != "audio" || g.Status != domain.StatusStarted || g.LastPositionMs != 3000 {
		t.Errorf("local video mismatch: %+v", g)
	}
	if !g.DownloadedAt.Equal(now) {
		t.Errorf("DownloadedAt: want %v, got %v", now, g.DownloadedAt)
	}
}

// TestRoundTripUpsertVideo guards the nine scalar args (esp. int32 duration and
// int64 viewCount) reaching the daemon un-swapped.
func TestRoundTripUpsertVideo(t *testing.T) {
	b := &recBackend{}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.UpsertVideo(context.Background(), "v1", "Title", "Chan", "ch1", 300, 12_345_678_901, "20240101", "https://y/v1"); err != nil {
		t.Fatal(err)
	}
	if b.gotVideoID != "v1" || b.gotUpTitle != "Title" || b.gotChannelID != "ch1" {
		t.Errorf("string args mismatch: id=%q title=%q ch=%q", b.gotVideoID, b.gotUpTitle, b.gotChannelID)
	}
	if b.gotUpDuration != 300 || b.gotUpViews != 12_345_678_901 {
		t.Errorf("numeric args mismatch: duration=%d views=%d", b.gotUpDuration, b.gotUpViews)
	}
}

// TestRoundTripLogActivity guards a full ActivityEntry reaching the daemon.
func TestRoundTripLogActivity(t *testing.T) {
	now := time.Now().UTC()
	sent := domain.ActivityEntry{Type: "subscribe", IsLocal: true, ChannelID: "ch1", ChannelName: "Chan", PlaylistLocalID: "local:9", Timestamp: now}
	b := &recBackend{}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.LogActivity(context.Background(), sent); err != nil {
		t.Fatal(err)
	}
	if b.gotActivity.Type != "subscribe" || !b.gotActivity.IsLocal || b.gotActivity.ChannelID != "ch1" || b.gotActivity.PlaylistLocalID != "local:9" {
		t.Errorf("activity mismatch: %+v", b.gotActivity)
	}
	if !b.gotActivity.Timestamp.Equal(now) {
		t.Errorf("Timestamp: want %v, got %v", now, b.gotActivity.Timestamp)
	}
}

// TestRoundTripSetChannelTags guards the channelID + []string tags reaching the daemon.
func TestRoundTripSetChannelTags(t *testing.T) {
	b := &recBackend{}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.SetChannelTags(context.Background(), "ch1", []string{"news", "tech"}); err != nil {
		t.Fatal(err)
	}
	if b.gotChannelID != "ch1" {
		t.Errorf("channelID: want ch1, got %q", b.gotChannelID)
	}
	if len(b.gotTags) != 2 || b.gotTags[1] != "tech" {
		t.Errorf("tags mismatch: %v", b.gotTags)
	}
}

// TestRoundTripSetChannelAlias guards the channelID + alias reaching the daemon.
func TestRoundTripSetChannelAlias(t *testing.T) {
	b := &recBackend{}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.SetChannelAlias(context.Background(), "ch1", "My Alias"); err != nil {
		t.Fatal(err)
	}
	if b.gotChannelID != "ch1" || b.gotAlias != "My Alias" {
		t.Errorf("args mismatch: id=%q alias=%q", b.gotChannelID, b.gotAlias)
	}
}

// TestRoundTripSetVideoStatus guards the id + typed VideoStatus enum (string on
// the wire) reaching the daemon.
func TestRoundTripSetVideoStatus(t *testing.T) {
	b := &recBackend{}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.SetVideoStatus(context.Background(), "v1", domain.StatusWatched); err != nil {
		t.Fatal(err)
	}
	if b.gotVideoID != "v1" || b.gotStatus != domain.StatusWatched {
		t.Errorf("args mismatch: id=%q status=%q", b.gotVideoID, b.gotStatus)
	}
}

// TestRoundTripEnqueue guards the full Video + audioOnly flag reaching the daemon.
func TestRoundTripEnqueue(t *testing.T) {
	sent := domain.Video{ID: "v1", Title: "Q", ChannelID: "ch1", Duration: 300, ViewCount: 99, URL: "https://y/v1"}
	b := &recBackend{}
	_, remote := newRoundTripSrv(t, b, "")

	if err := remote.Enqueue(context.Background(), sent, true); err != nil {
		t.Fatal(err)
	}
	assertVideo(t, b.gotVideo, sent)
	if !b.gotAudioOnly {
		t.Error("audioOnly flag did not reach the daemon")
	}
}

// ── Tier 2: SMOKE round-trips ─────────────────────────────────────────────────

// TestRoundTripSmoke exercises the handler decode→call→encode path for the
// scalar-only pass-through RPCs whose payloads carry no domain structs. Each
// runs against a plain nop backend and only asserts err == nil — enough to catch
// a panic, a marshaling break, or a mis-wired handler route for coverage.
func TestRoundTripSmoke(t *testing.T) {
	ctx := context.Background()
	_, remote := newRoundTripSrv(t, &testBackend{}, "")

	cases := []struct {
		name string
		call func() error
	}{
		{"AddHistory", func() error { return remote.AddHistory(ctx, "v1", "playVideo", "d") }},
		{"CancelDownload", func() error { return remote.CancelDownload(ctx, "v1") }},
		{"Enqueue", func() error { return remote.Enqueue(ctx, domain.Video{ID: "v1"}, false) }},
		{"ClearHistory", func() error { return remote.ClearHistory(ctx) }},
		{"ClearRecommended", func() error { return remote.ClearRecommended(ctx) }},
		{"ClearVideoDetailsCache", func() error { return remote.ClearVideoDetailsCache(ctx) }},
		{"DeleteChannelVideos", func() error { return remote.DeleteChannelVideos(ctx, "ch1") }},
		{"DeleteLocalVideo", func() error { return remote.DeleteLocalVideo(ctx, "v1") }},
		{"DeletePlaylist", func() error { return remote.DeletePlaylist(ctx, "local:7") }},
		{"DeleteSearchHistory", func() error { return remote.DeleteSearchHistory(ctx, "q") }},
		{"DeleteVideoCompletely", func() error { return remote.DeleteVideoCompletely(ctx, "v1") }},
		{"DeleteVideoHistory", func() error { return remote.DeleteVideoHistory(ctx, "v1") }},
		{"DeleteVideoPosition", func() error { return remote.DeleteVideoPosition(ctx, "v1") }},
		{"DeleteYTPlaylist", func() error { return remote.DeleteYTPlaylist(ctx, "PL1") }},
		{"HideRecVideo", func() error { return remote.HideRecVideo(ctx, "v1") }},
		{"InitYTClient", func() error { return remote.InitYTClient(ctx) }},
		{"PurgeFeedCacheMissingChannelID", func() error { return remote.PurgeFeedCacheMissingChannelID(ctx, "recommended") }},
		{"RemoveFromYTPlaylist", func() error { return remote.RemoveFromYTPlaylist(ctx, "PL1", "v1") }},
		{"RemoveSubscribedChannel", func() error { return remote.RemoveSubscribedChannel(ctx, "ch1") }},
		{"Unsubscribe", func() error { return remote.Unsubscribe(ctx, domain.Channel{ID: "ch1"}) }},
		{"UnblockChannel", func() error { return remote.UnblockChannel(ctx, "ch1") }},
		{"UpdateLastPosition", func() error { return remote.UpdateLastPosition(ctx, "v1", 5000) }},
		{"AddToYTPlaylist", func() error { return remote.AddToYTPlaylist(ctx, "PL1", "v1") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err != nil {
				t.Errorf("%s: unexpected error: %v", tc.name, err)
			}
		})
	}

	// SearchQueries returns a slice; smoke it separately.
	t.Run("SearchQueries", func(t *testing.T) {
		if _, err := remote.SearchQueries(ctx); err != nil {
			t.Errorf("SearchQueries: %v", err)
		}
	})
}
