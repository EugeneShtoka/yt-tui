package transport_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/api/backend/v1/backendv1connect"
)

// This is an anti-drift guard (modeled on app/command_audit_test.go): every RPC
// verb on every Connect service must be either exercised by a black-box
// round-trip test (roundTripped) or consciously exempt with a reason
// (exemptVerbs). Adding a new verb fails the build until it is classified, so
// the remote-mode proto<->domain boundary can't silently lose coverage.

func serviceHandlerTypes() map[string]reflect.Type {
	return map[string]reflect.Type{
		"Feed":        reflect.TypeOf((*backendv1connect.FeedServiceHandler)(nil)).Elem(),
		"Channel":     reflect.TypeOf((*backendv1connect.ChannelServiceHandler)(nil)).Elem(),
		"Video":       reflect.TypeOf((*backendv1connect.VideoServiceHandler)(nil)).Elem(),
		"Library":     reflect.TypeOf((*backendv1connect.LibraryServiceHandler)(nil)).Elem(),
		"Playlist":    reflect.TypeOf((*backendv1connect.PlaylistServiceHandler)(nil)).Elem(),
		"History":     reflect.TypeOf((*backendv1connect.HistoryServiceHandler)(nil)).Elem(),
		"Portability": reflect.TypeOf((*backendv1connect.PortabilityServiceHandler)(nil)).Elem(),
		"Profile":     reflect.TypeOf((*backendv1connect.ProfileServiceHandler)(nil)).Elem(),
		"Health":      reflect.TypeOf((*backendv1connect.HealthServiceHandler)(nil)).Elem(),
		"Download":    reflect.TypeOf((*backendv1connect.DownloadServiceHandler)(nil)).Elem(),
	}
}

// roundTripped are the verbs exercised by the round-trip tests in
// roundtrip_test.go — the data-carrying ones where a dropped field would be
// silent (keyed by bare method name).
var roundTripped = map[string]bool{
	"VideoDetails":         true,
	"GetVideoDetailsCache": true,
	"Recommended":          true,
	"ChannelVideos":        true,
	"LocalPlaylistVideos":  true,
	"History":              true,
	"LocalVideos":          true,
	"SubscribedChannels":   true,
}

const (
	exMutation    = "mutation/command — no rich domain payload to drop (scalar/bool/count/error only)"
	exVideoList   = "returns []Video via the same mapper a round-tripped verb exercises"
	exChannelList = "returns []Channel via ChannelToProto, round-tripped via SubscribedChannels"
	exHistoryList = "returns []HistoryEntry via the same mapper History round-trips"
	exScalars     = "returns scalars/IDs/opaque bytes — no structured domain fields to drop"
	exStream      = "server stream — not a request/response mapping"
	exBundle      = "portability bundle — covered by the portability JSON-contract test"
	exTimestamped = "timestamp-bearing record; low data-loss risk, round-trip not added"
)

// exemptVerbs are verbs deliberately not round-trip-tested, each with a reason.
var exemptVerbs = map[string]string{
	// Channel
	"AddSubscribedChannel":    exMutation,
	"AllChannels":             exChannelList,
	"BlockChannel":            exMutation,
	"BlockedChannels":         exChannelList,
	"ChannelHideStats":        exScalars,
	"ChannelLatestN":          exVideoList,
	"DeleteChannelVideos":     exMutation,
	"GetAllChannelVideos":     exVideoList,
	"GetChannelLatestAll":     exVideoList,
	"GetChannelVideos":        exVideoList,
	"GetSubscribedChannels":   exChannelList,
	"RemoveSubscribedChannel": exMutation,
	"SaveChannelVideos":       exMutation,
	"SaveSubscribedChannels":  exMutation,
	"Search":                  exChannelList,
	"SetChannelAlias":         exMutation,
	"SetChannelState":         exMutation,
	"SetChannelTags":          exMutation,
	"Subscribe":               exMutation,
	"UnblockChannel":          exMutation,
	"Unsubscribe":             exMutation,
	// Download
	"CancelDownload": exMutation,
	"ClearDownloads": exMutation,
	"DownloadItems":  exScalars,
	"Enqueue":        exMutation,
	"Events":         exStream,
	// Feed
	"ClearRecommended": exMutation,
	"GetFeedCache":     exVideoList,
	"HiddenVideoIDs":   exScalars,
	"HideVideo":        exMutation,
	"PurgeFeedCache":   exMutation,
	"SaveFeedCache":    exMutation,
	"WatchedVideoIDs":  exScalars,
	// Health
	"CheckAvailability": exScalars,
	// History
	"ActivityLog":         exTimestamped,
	"AddHistory":          exMutation,
	"ClearHistory":        exMutation,
	"DeleteSearchHistory": exMutation,
	"DeleteVideoHistory":  exMutation,
	"HistoryVideos":       exHistoryList,
	"LogActivity":         exMutation,
	"SearchQueries":       exScalars,
	"VideoHistory":        exHistoryList,
	// Library
	"AddLocalVideo":       exMutation,
	"DeleteAllLocalFiles": exScalars,
	"DeleteLocalVideo":    exMutation,
	"HasLocalVideo":       "returns a single LocalVideo via the same mapper LocalVideos round-trips",
	// Playlist
	"AddToPlaylist":        exMutation,
	"AddToWatchLater":      exMutation,
	"AddToYTPlaylist":      exMutation,
	"CreatePlaylist":       exMutation,
	"CreateYTPlaylist":     exMutation,
	"DeletePlaylist":       exMutation,
	"DeleteYTPlaylist":     exMutation,
	"GetYTPlaylistVideos":  exVideoList,
	"GetYTPlaylists":       exScalars,
	"InitYTClient":         exMutation,
	"LocalPlaylists":       exTimestamped,
	"PlaylistVideoIDs":     exScalars,
	"RemoveFromPlaylist":   exMutation,
	"RemoveFromWatchLater": exMutation,
	"RemoveFromYTPlaylist": exMutation,
	"SaveYTPlaylistVideos": exMutation,
	"SaveYTPlaylists":      exMutation,
	"YTPlaylistVideos":     exVideoList,
	"YTPlaylists":          exScalars,
	// Portability
	"Export":        exBundle,
	"ImportApply":   exBundle,
	"ImportPreview": exBundle,
	// Profile
	"GetProfile":   exScalars,
	"ListProfiles": exScalars,
	"SaveProfile":  exMutation,
	// Video
	"AllVideoPositions":      exScalars,
	"ClearVideoDetailsCache": exMutation,
	"DeleteVideoCompletely":  exMutation,
	"DeleteVideoPosition":    exMutation,
	"GetThumbnail":           exScalars,
	"GetTranscript":          exScalars,
	"EligibleThumbnailIDs":   exScalars,
	"Capabilities":           exScalars,
	"ResolveSource":          exScalars,
	"SaveVideoChapters":      exMutation,
	"SaveVideoDetailsCache":  exMutation,
	"SaveVideoLinks":         exMutation,
	"SaveVideoPosition":      exMutation,
	"SaveVideoSBSegments":    exMutation,
	"SetVideoStatus":         exMutation,
	"UpdateLastPosition":     exMutation,
	"UpsertVideo":            exMutation,
	"VideoPosition":          exScalars,
}

func TestEveryTransportVerbClassified(t *testing.T) {
	var unclassified []string
	for svc, typ := range serviceHandlerTypes() {
		for i := 0; i < typ.NumMethod(); i++ {
			name := typ.Method(i).Name
			if roundTripped[name] {
				continue
			}
			if _, ok := exemptVerbs[name]; ok {
				continue
			}
			unclassified = append(unclassified, svc+"."+name)
		}
	}
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Fatalf("unclassified transport verb(s) %v — add a round-trip test (roundTripped) "+
			"or exempt with a reason (exemptVerbs)", unclassified)
	}
}

func TestNoStaleVerbClassifications(t *testing.T) {
	exists := map[string]bool{}
	for _, typ := range serviceHandlerTypes() {
		for i := 0; i < typ.NumMethod(); i++ {
			exists[typ.Method(i).Name] = true
		}
	}
	for name := range roundTripped {
		if !exists[name] {
			t.Errorf("roundTripped references %q, not an RPC verb", name)
		}
	}
	for name := range exemptVerbs {
		if !exists[name] {
			t.Errorf("exemptVerbs references %q, not an RPC verb", name)
		}
	}
}
