package app

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
)

// paletteCommands maps a user-facing Backend action to the palette command that
// exposes it. Keys stay as fast paths; the palette is the discoverable surface.
// The Phase-21 audit: every backend action either has a command here or is an
// explicitly-exempted internal operation below — a new Backend method that is
// neither fails TestEveryBackendMethodIsClassified, forcing a conscious
// register-or-exempt decision.
var paletteCommands = map[string]string{
	"Enqueue":             "download",
	"ClearDownloads":      "clear-downloads",
	"DeleteAllLocalFiles": "delete-all-local",
}

// exemptActions are Backend methods that are NOT global palette actions:
// internal reads, background sync/cache writes, and per-item operations that
// live on a dedicated key within a view (block, subscribe, delete-one, …)
// rather than as a global command. Membership is the "conscious decision".
var exemptActions = map[string]bool{}

func init() {
	for _, name := range []string{
		// FeedBackend — reads + background feed/cache maintenance + per-item hide.
		"Recommended", "GetFeedCache", "SaveFeedCache", "PurgeFeedCacheMissingChannelID",
		"HideRecVideo", "HiddenRecVideoIDs", "WatchedVideoIDs", "ClearRecommended",
		// ChannelBackend — search/reads + per-channel actions with dedicated keys.
		"Search", "ChannelVideos", "ChannelLatestN", "SubscribedChannels",
		"GetSubscribedChannels", "AllChannels", "BlockedChannels", "GetChannelVideos",
		"GetAllChannelVideos", "GetChannelLatestAll", "ChannelHideStats", "Subscribe",
		"Unsubscribe", "BlockChannel", "UnblockChannel", "SetChannelState",
		"AddSubscribedChannel", "SaveSubscribedChannels", "RemoveSubscribedChannel",
		"DeleteChannelVideos", "SetChannelAlias", "SetChannelTags", "SaveChannelVideos",
		// VideoBackend — per-video reads/writes/lifecycle, all view-local.
		"VideoDetails", "GetVideoDetailsCache", "HasLocalVideo", "VideoPosition",
		"AllVideoPositions", "UpsertVideo", "SetVideoStatus", "SaveVideoPosition",
		"DeleteVideoPosition", "UpdateLastPosition", "SaveVideoDetailsCache",
		"SaveVideoChapters", "SaveVideoSBSegments", "SaveVideoLinks",
		"ClearVideoDetailsCache", "DeleteVideoCompletely", "ResolveSource",
		"GetThumbnail", "GetTranscript", "EligibleThumbnailIDs",
		// LibraryBackend — per-file bookkeeping (bulk delete is a command above).
		"LocalVideos", "AddLocalVideo", "DeleteLocalVideo",
		// PlaylistBackend — playlist CRUD, all reached from the Playlists view.
		"LocalPlaylists", "LocalPlaylistVideos", "PlaylistVideoIDs", "CreatePlaylist",
		"DeletePlaylist", "AddToPlaylist", "RemoveFromPlaylist",
		"AddToWatchLater", "RemoveFromWatchLater",
		"YTPlaylists", "YTPlaylistVideos",
		"GetYTPlaylists", "GetYTPlaylistVideos", "SaveYTPlaylists", "SaveYTPlaylistVideos",
		"InitYTClient", "CreateYTPlaylist", "DeleteYTPlaylist", "AddToYTPlaylist",
		"RemoveFromYTPlaylist",
		// HistoryBackend — history reads + writes, reached from the History view.
		"History", "HistoryVideos", "VideoHistory", "ActivityLog", "SearchQueries",
		"AddHistory", "LogActivity", "DeleteVideoHistory", "DeleteSearchHistory",
		"ClearHistory",
		// DownloadBackend — per-item queue ops + the event stream.
		"CancelDownload", "DownloadItems", "Events",
		// PortabilityBackend — Export/Import have dedicated keys (E / I) + overlays.
		"Export", "ImportPreview", "ImportApply",
		// ProfileBackend — connect-time config plumbing, no interactive surface.
		"ListProfiles", "GetProfile", "SaveProfile",
		// StatusBackend — startup diagnostics, surfaced automatically.
		"CheckAvailability", "Capabilities",
	} {
		exemptActions[name] = true
	}
}

// TestEveryBackendMethodIsClassified asserts every method on the composed
// api.Backend interface is either exposed via a palette command or explicitly
// exempt. A new backend method that is neither fails here on purpose.
func TestEveryBackendMethodIsClassified(t *testing.T) {
	iface := reflect.TypeOf((*api.Backend)(nil)).Elem()
	var unclassified []string
	for i := 0; i < iface.NumMethod(); i++ {
		name := iface.Method(i).Name
		_, backed := paletteCommands[name]
		if !backed && !exemptActions[name] {
			unclassified = append(unclassified, name)
		}
	}
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Fatalf("unclassified Backend method(s) %v — add a palette command to "+
			"paletteCommands or exempt them in exemptActions (Phase-21 audit)", unclassified)
	}
}

// TestPaletteCommandsExist asserts every command named in paletteCommands is
// actually registered, so the audit map can't drift from the registry.
func TestPaletteCommandsExist(t *testing.T) {
	cmds := globalCommands(context.Background(), apitest.NopBackend{}, nil)
	registered := make(map[string]bool, len(cmds))
	for _, c := range cmds {
		registered[c.Name] = true
	}
	for method, cmdName := range paletteCommands {
		if !registered[cmdName] {
			t.Errorf("paletteCommands[%q] = %q but no such command is registered", method, cmdName)
		}
	}
}

// TestNoStaleAuditEntries guards the other direction: every classified name
// must still exist on the interface, so removing a Backend method surfaces the
// stale audit entry instead of silently rotting.
func TestNoStaleAuditEntries(t *testing.T) {
	iface := reflect.TypeOf((*api.Backend)(nil)).Elem()
	exists := make(map[string]bool, iface.NumMethod())
	for i := 0; i < iface.NumMethod(); i++ {
		exists[iface.Method(i).Name] = true
	}
	for name := range paletteCommands {
		if !exists[name] {
			t.Errorf("paletteCommands references %q, not a Backend method", name)
		}
	}
	for name := range exemptActions {
		if !exists[name] {
			t.Errorf("exemptActions references %q, not a Backend method", name)
		}
	}
}
