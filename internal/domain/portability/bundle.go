// Package portability defines the versioned, self-describing export/import
// bundle for all app-owned data. The bundle is a stable JSON contract that is
// deliberately decoupled from the internal domain structs (which may be
// refactored freely): every section uses its own export DTO with explicit
// snake_case json tags, so a rename in internal/domain never silently changes
// the on-disk format. Bump SchemaVersion on any breaking change to the shape.
package portability

import "encoding/json"

// SchemaVersion is the current bundle format version. Importers compare it
// against their supported version to detect (in)compatibility.
const SchemaVersion = 1

// Bundle is the top-level export artifact. Optional sections are omitted when
// empty. Personal watch data (History/Positions) is only populated when
// ExportOptions.IncludeWatchData is set; everything else is always exported.
type Bundle struct {
	SchemaVersion int              `json:"schema_version"`
	Channels      []ChannelExport  `json:"channels,omitempty"`
	BlockedNames  []string         `json:"blocked_names,omitempty"`
	Playlists     []PlaylistExport `json:"playlists,omitempty"`
	WatchLater    []WatchLaterRef  `json:"watch_later,omitempty"`
	YTPlaylists   []YTPlaylistRef  `json:"yt_playlists,omitempty"`
	// Videos is the deduplicated metadata for every video referenced by
	// Playlists (by id), so the lists rehydrate on import without re-fetching.
	Videos []VideoExport `json:"videos,omitempty"`

	// ── opt-in personal watch data ──
	History   []HistoryExport  `json:"history,omitempty"`
	Positions []PositionExport `json:"positions,omitempty"`

	// Config is the portable client config profile (keybindings, panel layout,
	// display prefs, and portable daemon preferences). It is carried as opaque
	// JSON so the bundle stays decoupled from the config package (a foundation
	// package the domain must not import) — the same single-sourced-in-Go
	// rationale by which the whole bundle crosses the wire as JSON bytes. Only
	// the client marshals/unmarshals it; the backend never inspects config.
	// Empty when the bundle carries no config profile.
	Config json.RawMessage `json:"config,omitempty"`
}

// ChannelExport is one row of the channels table: enough to restore
// annotations (alias/tags), subscription state, and block status. The block
// invariant (blocked ⟹ state 'none') is preserved by the source data.
type ChannelExport struct {
	ChannelID         string   `json:"channel_id"`
	Name              string   `json:"name,omitempty"`
	URL               string   `json:"url,omitempty"`
	Alias             string   `json:"alias,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	SubscriptionState string   `json:"subscription_state"`
	Blocked           bool     `json:"blocked,omitempty"`
}

// PlaylistExport is a local playlist as a name plus an ordered list of video
// ids. Ids resolve against the Videos section. Merge-by-name on import means
// the autoincrement id is intentionally not exported.
type PlaylistExport struct {
	Name     string   `json:"name"`
	VideoIDs []string `json:"video_ids,omitempty"`
}

// WatchLaterRef mirrors the watch_later row. It carries its own denormalized
// title/channel/url because watch-later entries are not guaranteed to have a
// row in the shared videos table.
type WatchLaterRef struct {
	VideoID string `json:"video_id"`
	Title   string `json:"title,omitempty"`
	Channel string `json:"channel,omitempty"`
	URL     string `json:"url,omitempty"`
}

// YTPlaylistRef is a YouTube playlist reference (id + title only). Contents are
// re-fetched from YouTube on import, never carried in the bundle.
type YTPlaylistRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// VideoExport is the shared video-metadata row referenced by Playlists.
type VideoExport struct {
	ID         string `json:"id"`
	Title      string `json:"title,omitempty"`
	Channel    string `json:"channel,omitempty"`
	ChannelID  string `json:"channel_id,omitempty"`
	Duration   int    `json:"duration,omitempty"`
	ViewCount  int64  `json:"view_count,omitempty"`
	UploadDate string `json:"upload_date,omitempty"`
	URL        string `json:"url,omitempty"`
}

// HistoryExport is one play/stream/search/delete event. Timestamp is unix
// seconds (UTC-independent) rather than a formatted time. Video metadata is
// inline since search/delete events may have no shared videos row.
type HistoryExport struct {
	VideoID    string `json:"video_id,omitempty"`
	Title      string `json:"title,omitempty"`
	Channel    string `json:"channel,omitempty"`
	ChannelID  string `json:"channel_id,omitempty"`
	Duration   int    `json:"duration,omitempty"`
	ViewCount  int64  `json:"view_count,omitempty"`
	UploadDate string `json:"upload_date,omitempty"`
	EventType  string `json:"event_type"`
	Details    string `json:"details,omitempty"`
	Timestamp  int64  `json:"timestamp"`
}

// PositionExport is a saved playback resume position (video id → milliseconds).
type PositionExport struct {
	VideoID    string `json:"video_id"`
	PositionMs int64  `json:"position_ms"`
}

// ExportOptions gates the optional bundle sections. IncludeWatchData toggles
// personal watch data (history + resume positions); default off per the locked
// export decision.
type ExportOptions struct {
	IncludeWatchData bool `json:"include_watch_data"`
}
