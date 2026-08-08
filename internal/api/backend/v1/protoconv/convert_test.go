package protoconv

import (
	"reflect"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ptr is a tiny helper for building the nullable *[]T fields on CachedDetails.
func ptr[T any](v T) *T { return &v }

// The round-trip tests below convert a domain value to proto and back, asserting
// the result is identical. They are the guard the C-1 audit finding asked for:
// any converter that forgets a field (as VideoDetailsToProto did for Language and
// SBSegments) fails here instead of silently dropping data in remote mode.
//
// Values are chosen to populate every field that survives the wire. Fields that
// are intentionally not on the proto (e.g. Channel.FetchedVideos, Channel.IsLocal
// which is normalized into State) are left at their zero/normalized form so the
// comparison stays meaningful rather than asserting a lossy mapping is lossless.

func TestVideoRoundTrip(t *testing.T) {
	cases := []domain.Video{
		{},
		{
			ID: "vid1", Title: "Hello", Channel: "Chan", ChannelID: "c1",
			Duration: 3661, ViewCount: 1234567, UploadDate: "20260102", URL: "https://y/vid1",
		},
	}
	for _, in := range cases {
		if got := ProtoToVideo(VideoToProto(in)); !reflect.DeepEqual(got, in) {
			t.Errorf("Video round-trip mismatch:\n got  %+v\n want %+v", got, in)
		}
	}
}

func TestVideoDetailsRoundTrip(t *testing.T) {
	cases := []domain.VideoDetails{
		{},
		{
			Video: domain.Video{
				ID: "vid1", Title: "T", Channel: "C", ChannelID: "c1",
				Duration: 120, ViewCount: 9, UploadDate: "20260101", URL: "u",
			},
			Description:  "desc",
			ThumbnailURL: "thumb",
			Subscribers:  42,
			Chapters: []domain.RawChapter{
				{Title: "Intro", StartTime: 0, EndTime: 30},
				{Title: "Body", StartTime: 30, EndTime: 90},
			},
			Language: "ru", // C-1: must survive the wire
			SBSegments: []domain.SBSegment{ // C-1: must survive the wire
				{Start: 10.5, End: 20.25},
				{Start: 100, End: 110},
			},
		},
	}
	for _, in := range cases {
		if got := ProtoToVideoDetails(VideoDetailsToProto(in)); !reflect.DeepEqual(got, in) {
			t.Errorf("VideoDetails round-trip mismatch:\n got  %+v\n want %+v", got, in)
		}
	}
}

func TestCachedDetailsRoundTrip(t *testing.T) {
	cases := []domain.CachedDetails{
		{Description: "d", ThumbnailURL: "t", Subscribers: 7}, // all *_parsed false → nil slices
		{
			Description:  "d",
			ThumbnailURL: "t",
			Subscribers:  7,
			Links:        ptr([]domain.Link{{Label: "L", URL: "u"}}),
			Chapters:     ptr([]domain.Chapter{{Title: "C", OriginalStart: 1, OriginalEnd: 2, AdjustedStart: 3, AdjustedEnd: 4}}),
			SBSegments:   ptr([]domain.SBSegment{{Start: 5, End: 6}}),
		},
		{ // parsed-but-empty must stay non-nil after the trip
			Links:      ptr([]domain.Link{}),
			Chapters:   ptr([]domain.Chapter{}),
			SBSegments: ptr([]domain.SBSegment{}),
		},
	}
	for _, in := range cases {
		if got := ProtoToCachedDetails(CachedDetailsToProto(in)); !reflect.DeepEqual(got, in) {
			t.Errorf("CachedDetails round-trip mismatch:\n got  %+v\n want %+v", got, in)
		}
	}
}

func TestLocalVideoRoundTrip(t *testing.T) {
	ts := time.Unix(1_700_000_000, 0).UTC()
	cases := []domain.LocalVideo{
		{},
		{
			ID: "v", Title: "T", Channel: "C", Duration: 60, ViewCount: 3,
			UploadDate: "20260101", FilePath: "/tmp/v.mp4", DownloadType: "video",
			Status: domain.VideoStatus("downloaded"), LastPositionMs: 1234,
			DownloadedAt: ts, LastPlayed: ts,
		},
	}
	for _, in := range cases {
		if got := ProtoToLocalVideo(LocalVideoToProto(in)); !reflect.DeepEqual(got, in) {
			t.Errorf("LocalVideo round-trip mismatch:\n got  %+v\n want %+v", got, in)
		}
	}
}

func TestChannelRoundTrip(t *testing.T) {
	// State is set explicitly so SubState() normalization is a no-op; IsLocal and
	// FetchedVideos are not on the wire, so leave them at their round-trippable form.
	cases := []domain.Channel{
		{ID: "c", Name: "N", State: domain.SubYT},
		{
			ID: "c2", Name: "N2", Alias: "A", Tags: []string{"x", "y"}, URL: "u",
			Subscribers: 100, State: domain.SubLocal, VideosRefreshedAt: 999, LastActivityAt: 42,
		},
		{ID: "c3", Name: "N3", State: domain.SubNone, Blocked: true},
	}
	for _, in := range cases {
		if got := ProtoToChannel(ChannelToProto(in)); !reflect.DeepEqual(got, in) {
			t.Errorf("Channel round-trip mismatch:\n got  %+v\n want %+v", got, in)
		}
	}
}

func TestHistoryEntryRoundTrip(t *testing.T) {
	ts := time.Unix(1_700_000_000, 0).UTC()
	in := domain.HistoryEntry{
		ID: 7, VideoID: "v", Title: "T", Channel: "C", ChannelID: "c1",
		Duration: 90, ViewCount: 5, UploadDate: "20260101",
		EventType: "watched", Details: "resume", Timestamp: ts,
	}
	if got := ProtoToHistoryEntry(HistoryEntryToProto(in)); !reflect.DeepEqual(got, in) {
		t.Errorf("HistoryEntry round-trip mismatch:\n got  %+v\n want %+v", got, in)
	}
}

func TestActivityEntryRoundTrip(t *testing.T) {
	ts := time.Unix(1_700_000_000, 0).UTC()
	in := domain.ActivityEntry{
		ID: 3, Type: "upload", IsLocal: true, ChannelID: "c1", ChannelName: "C",
		PlaylistID: "p1", PlaylistLocalID: 9, PlaylistName: "P",
		VideoID: "v1", VideoTitle: "V", Timestamp: ts,
	}
	if got := ProtoToActivityEntry(ActivityEntryToProto(in)); !reflect.DeepEqual(got, in) {
		t.Errorf("ActivityEntry round-trip mismatch:\n got  %+v\n want %+v", got, in)
	}
}

func TestPlaylistRoundTrip(t *testing.T) {
	ts := time.Unix(1_700_000_000, 0).UTC()
	in := domain.Playlist{ID: 5, Name: "P", CreatedAt: ts}
	if got := ProtoToPlaylist(PlaylistToProto(in)); !reflect.DeepEqual(got, in) {
		t.Errorf("Playlist round-trip mismatch:\n got  %+v\n want %+v", got, in)
	}
}
