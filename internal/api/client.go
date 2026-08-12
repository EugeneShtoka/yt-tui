package api

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
)

// EventKind identifies the type of a daemon→TUI notification.
type EventKind string

const (
	EventDownloadProgress EventKind = "download_progress"
	EventDownloadDone     EventKind = "download_done"
	EventDownloadError    EventKind = "download_error"
)

// Event is a daemon→TUI notification (download progress, feed refresh, new videos).
type Event struct {
	Kind    EventKind
	VideoID string
	Detail  string // human-readable status / error message
}

// DownloadStatus mirrors downloader.Status without importing that package from the TUI.
type DownloadStatus string

const (
	DownloadPending  DownloadStatus = "pending"
	DownloadActive   DownloadStatus = "active"
	DownloadComplete DownloadStatus = "complete"
	DownloadFailed   DownloadStatus = "failed"
)

// DownloadItem is a snapshot of one queue entry, safe to pass across the api boundary.
type DownloadItem struct {
	VideoID   string
	Title     string
	Channel   string
	Duration  int // seconds
	URL       string
	AudioOnly bool
	Status    DownloadStatus
	Progress  float64
	Speed     string
	ETA       string
	FilePath  string
	Err       error
}

// These accessors let DownloadItem satisfy the videotable row interface used by
// the Downloading tab.
func (di DownloadItem) GetBaseTitle() string { return di.Title }
func (di DownloadItem) IsAudio() bool        { return di.AudioOnly }

// GetChannelID always returns "" because a queue snapshot carries only the
// channel display name, not its ID; the Downloading tab never keys off it.
func (di DownloadItem) GetChannelID() string   { return "" }
func (di DownloadItem) GetChannelName() string { return di.Channel }

// GetDurationSecs returns the raw duration in seconds; the Downloading tab's
// column formats it via render.Duration so it honors the configured format,
// like every other duration in the UI.
func (di DownloadItem) GetDurationSecs() int { return di.Duration }

// PlayableSource is returned by ResolveSource. URI is either an absolute file
// path (co-located InProc with a downloaded file) or an http[s]:// URL.
type PlayableSource struct {
	URI string
}

// FeedBackend is the recommended-feed / feed-cache slice of Backend, mirroring
// backendv1connect.FeedServiceHandler.
type FeedBackend interface {
	Recommended(ctx context.Context) ([]domain.Video, error)
	GetFeedCache(ctx context.Context, feed string) ([]domain.Video, error)
	SaveFeedCache(ctx context.Context, feed string, videos []domain.Video) error
	PurgeFeedCacheMissingChannelID(ctx context.Context, feed string) error
	HideRecVideo(ctx context.Context, videoID string) error
	HiddenRecVideoIDs(ctx context.Context) (map[string]bool, error)
	WatchedVideoIDs(ctx context.Context) (map[string]bool, error)
	ClearRecommended(ctx context.Context) error
}

// ChannelBackend is the channel subscription/listing slice of Backend. It is
// large, but deliberately so: it maps 1:1 to a single RPC service
// (backendv1connect.ChannelServiceHandler), so the surface is the service
// contract rather than a candidate for further splitting (M-2).
type ChannelBackend interface {
	Search(ctx context.Context, query string) (channels []domain.Channel, videos []domain.Video, err error)
	ChannelVideos(ctx context.Context, channelURL, channelID string) ([]domain.Video, error)
	ChannelLatestN(ctx context.Context, channelURL, channelID string, n int) ([]domain.Video, error)
	SubscribedChannels(ctx context.Context) ([]domain.Channel, error)
	GetSubscribedChannels(ctx context.Context) ([]domain.Channel, error)
	// AllChannels returns every known channel row (subscribed, annotated-but-
	// unsubscribed, and blocked); BlockedChannels returns just the blocked ones.
	AllChannels(ctx context.Context) ([]domain.Channel, error)
	BlockedChannels(ctx context.Context) ([]domain.Channel, error)
	GetChannelVideos(ctx context.Context, channelID string) ([]domain.Video, error)
	GetAllChannelVideos(ctx context.Context, channelIDs []string) ([]domain.Video, error)
	GetChannelLatestAll(ctx context.Context) (map[string]domain.Video, error)
	ChannelHideStats(ctx context.Context, channelID string) (hidden, played int, err error)
	Subscribe(ctx context.Context, ch domain.Channel) error
	Unsubscribe(ctx context.Context, ch domain.Channel) error
	// BlockChannel is the guarded block transition (unsubscribe + set blocked);
	// UnblockChannel clears the flag, leaving the channel unsubscribed.
	// SetChannelState moves a channel between subscription states, rejecting a
	// subscribe on a blocked channel with domain.ErrChannelBlocked.
	BlockChannel(ctx context.Context, ch domain.Channel) error
	UnblockChannel(ctx context.Context, channelID string) error
	SetChannelState(ctx context.Context, channelID string, state domain.SubscriptionState) error
	AddSubscribedChannel(ctx context.Context, ch domain.Channel) error
	SaveSubscribedChannels(ctx context.Context, channels []domain.Channel) error
	RemoveSubscribedChannel(ctx context.Context, channelID string) error
	DeleteChannelVideos(ctx context.Context, channelID string) error
	SetChannelAlias(ctx context.Context, channelID, alias string) error
	SetChannelTags(ctx context.Context, channelID string, tags []string) error
	SaveChannelVideos(ctx context.Context, channelID string, videos []domain.Video) error
}

// VideoBackend is the per-video detail/position/lifecycle slice of Backend.
// Like ChannelBackend it maps 1:1 to a single RPC service
// (backendv1connect.VideoServiceHandler), so its size is the service contract,
// not neglected segregation (M-2).
type VideoBackend interface {
	VideoDetails(ctx context.Context, videoURL string) (domain.VideoDetails, error)
	GetVideoDetailsCache(ctx context.Context, videoID string) (domain.CachedDetails, bool, error)
	// HasLocalVideo, VideoPosition: a false result with a nil error means
	// legitimately absent (no download / no saved position). A non-nil error
	// means the lookup itself failed (DB error, or in remote mode a transport
	// failure) and must not be treated as "absent" by the caller. (H-8)
	HasLocalVideo(ctx context.Context, videoID string) (domain.LocalVideo, bool, error)
	VideoPosition(ctx context.Context, videoID string) (int64, bool, error)
	AllVideoPositions(ctx context.Context) (map[string]int64, error)
	UpsertVideo(ctx context.Context, id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error
	SetVideoStatus(ctx context.Context, id string, status domain.VideoStatus) error
	SaveVideoPosition(ctx context.Context, videoID string, ms int64) error
	DeleteVideoPosition(ctx context.Context, videoID string) error
	UpdateLastPosition(ctx context.Context, id string, ms int64) error
	SaveVideoDetailsCache(ctx context.Context, videoID, description, thumbnailURL string, subscribers int64) error
	SaveVideoChapters(ctx context.Context, videoID string, chapters []domain.Chapter) error
	SaveVideoSBSegments(ctx context.Context, videoID string, segs []domain.SBSegment) error
	SaveVideoLinks(ctx context.Context, videoID string, links []domain.Link) error
	ClearVideoDetailsCache(ctx context.Context) error
	// DeleteVideoCompletely removes every trace of a video (local file+row,
	// history, saved position) as a single server-side operation rather than
	// a client-side saga of three separate calls. (M-23)
	DeleteVideoCompletely(ctx context.Context, videoID string) error

	// ResolveSource returns a playable URI: local file path or HTTP URL.
	// videoID is used to check for a locally-downloaded file; fallbackURL is the
	// YouTube URL used when no local file exists so the player can handle streaming.
	// In remote mode the daemon returns an authenticated /media/{id} URL for downloads.
	ResolveSource(ctx context.Context, videoID, fallbackURL string) (PlayableSource, error)

	// GetThumbnail returns the thumbnail image bytes for a video. The daemon
	// serves them from its local cache, fetching on a miss (and persisting when
	// the video is thumbnail-eligible), so a remote client never reaches the CDN
	// itself. A false result with nil error means no image could be obtained;
	// callers fall back to their own fetch. fallbackURL is the preferred image
	// URL (empty uses the predictable CDN default).
	GetThumbnail(ctx context.Context, videoID, fallbackURL string) ([]byte, bool, error)

	// GetTranscript returns a video's transcript as display-ready plain text. The
	// daemon serves it from the transcript store, fetching (and persisting) on a
	// miss. A false result with nil error means no transcript could be obtained.
	// videoURL is used for the on-miss fetch.
	GetTranscript(ctx context.Context, videoID, videoURL string) (string, bool, error)

	// EligibleThumbnailIDs returns the set of video IDs whose thumbnails the
	// backend caches (recommended feed ∪ newest-N per subscribed channel), bounded
	// by the daemon's own thumbnails_per_channel. A client uses it to pre-warm its
	// local cache with exactly the set the daemon holds.
	EligibleThumbnailIDs(ctx context.Context) (map[string]bool, error)
}

// MediaProvider is the narrow client-side seam for the two renderable per-video
// artifacts — the thumbnail image and the transcript text. The TUI overlay
// depends on this instead of the full VideoBackend, so the only media surface it
// touches is these two methods, and a client-side caching wrapper (see
// NewMediaProvider) can sit in front of the daemon without dragging the rest of
// the backend through it. Both InProc and Remote already implement these methods
// (they are part of VideoBackend), so they satisfy MediaProvider directly.
//
// Egress differs per artifact and is derived, not configured: a transcript is
// always fetched by the daemon (the client may have no yt-dlp/cookies), whereas
// a thumbnail is fetched by the client directly only when the daemon's thumbnail
// cache is off — otherwise it routes through the daemon like everything else.
type MediaProvider interface {
	GetThumbnail(ctx context.Context, videoID, fallbackURL string) ([]byte, bool, error)
	GetTranscript(ctx context.Context, videoID, videoURL string) (string, bool, error)
}

// LibraryBackend is the downloaded-file bookkeeping slice of Backend,
// mirroring backendv1connect.LibraryServiceHandler.
type LibraryBackend interface {
	LocalVideos(ctx context.Context) ([]domain.LocalVideo, error)
	HasLocalVideo(ctx context.Context, videoID string) (domain.LocalVideo, bool, error)
	AddLocalVideo(ctx context.Context, v domain.LocalVideo) error
	DeleteLocalVideo(ctx context.Context, id string) error
	// DeleteAllLocalFiles is the bulk "reclaim disk space" action: it deletes
	// every downloaded file + row, writing a delete history event for each
	// (looping DeleteLocalVideo). Returns the number of local videos removed.
	DeleteAllLocalFiles(ctx context.Context) (int, error)
}

// LocalPlaylistBackend is the local (SQLite) playlist slice: user-created
// playlists and their membership.
type LocalPlaylistBackend interface {
	LocalPlaylists(ctx context.Context) ([]domain.Playlist, error)
	LocalPlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error)
	PlaylistVideoIDs(ctx context.Context, playlistID string) ([]string, error)
	CreatePlaylist(ctx context.Context, name string) (string, error)
	DeletePlaylist(ctx context.Context, id string) error
	AddToPlaylist(ctx context.Context, playlistID string, videoID string) error
	RemoveFromPlaylist(ctx context.Context, playlistID string, videoID string) error
}

// WatchLaterBackend is the Watch Later slice. The implementation chooses the
// store: YouTube's "WL" playlist when the YT client is initialized, otherwise a
// reserved local "Watch Later" playlist (domain.WatchLaterPlaylistName). Add
// carries the full video so the local path can persist metadata; remove needs
// only the id.
type WatchLaterBackend interface {
	AddToWatchLater(ctx context.Context, video domain.Video) error
	RemoveFromWatchLater(ctx context.Context, videoID string) error
}

// YTPlaylistBackend is the remote YouTube-playlist slice: fetching, caching, and
// mutating the user's real YouTube playlists via the innertube client.
type YTPlaylistBackend interface {
	YTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error)
	YTPlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error)
	GetYTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error)
	GetYTPlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error)
	// SyncYTPlaylists is the throttled background refresh: it returns the cached
	// list when it was synced within refresh_minutes, otherwise fetches live and
	// persists. The decision is made backend-side (single-binary or daemon).
	SyncYTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error)
	SaveYTPlaylists(ctx context.Context, playlists []domain.YTPlaylist) error
	SaveYTPlaylistVideos(ctx context.Context, playlistID string, videos []domain.Video) error
	InitYTClient(ctx context.Context) error
	CreateYTPlaylist(ctx context.Context, name string) (id string, err error)
	DeleteYTPlaylist(ctx context.Context, playlistID string) error
	AddToYTPlaylist(ctx context.Context, playlistID, videoID string) error
	RemoveFromYTPlaylist(ctx context.Context, playlistID, videoID string) error
}

// PlaylistBackend composes the three orthogonal playlist slices (local, Watch
// Later, and YouTube). They share no state and the TUI consumes them
// independently, so a consumer can now depend on just the slice it uses (M-2).
// The aggregate is retained because it maps 1:1 to a single RPC service
// (backendv1connect.PlaylistServiceHandler) at the transport boundary.
type PlaylistBackend interface {
	LocalPlaylistBackend
	WatchLaterBackend
	YTPlaylistBackend
}

// HistoryBackend is the play/search-history slice of Backend, mirroring
// backendv1connect.HistoryServiceHandler.
type HistoryBackend interface {
	History(ctx context.Context, limit int) ([]domain.HistoryEntry, error)
	HistoryVideos(ctx context.Context, limit int) ([]domain.HistoryEntry, error)
	VideoHistory(ctx context.Context, videoID string) ([]domain.HistoryEntry, error)
	ActivityLog(ctx context.Context, limit int) ([]domain.ActivityEntry, error)
	SearchQueries(ctx context.Context) ([]string, error)
	AddHistory(ctx context.Context, videoID, eventType, details string) error
	LogActivity(ctx context.Context, e domain.ActivityEntry) error
	DeleteVideoHistory(ctx context.Context, videoID string) error
	DeleteSearchHistory(ctx context.Context, query string) error
	ClearHistory(ctx context.Context) error
}

// DownloadBackend is the download-queue slice of Backend, mirroring
// backendv1connect.DownloadServiceHandler.
type DownloadBackend interface {
	Enqueue(ctx context.Context, video domain.Video, audioOnly bool) error
	CancelDownload(ctx context.Context, videoID string) error
	DownloadItems(ctx context.Context) ([]DownloadItem, error)
	// ClearDownloads dismisses every queue item (cancel-if-active), the bulk
	// form of CancelDownload. It touches only the transient queue — never
	// files, the DB, or history.
	ClearDownloads(ctx context.Context) error
	Events(ctx context.Context) (<-chan Event, error)
}

// PortabilityBackend is the export/import slice of Backend, mirroring
// backendv1connect.PortabilityServiceHandler. Export bundles a full backup;
// ImportPreview computes a non-mutating diff of what ImportApply would write,
// and ImportApply merges the bundle back in (idempotent).
type PortabilityBackend interface {
	Export(ctx context.Context, opts portability.ExportOptions) (portability.Bundle, error)
	ImportPreview(ctx context.Context, bundle portability.Bundle, opts portability.ImportOptions) (portability.ImportPlan, error)
	ImportApply(ctx context.Context, bundle portability.Bundle, opts portability.ImportOptions) (portability.ImportResult, error)
}

// ProfileBackend is the daemon-stored named config profiles slice of Backend,
// mirroring backendv1connect.ProfileServiceHandler. A profile's bytes are the
// client's portable config profile as opaque JSON (the same blob the export
// bundle carries): the daemon persists it by name and never parses its schema,
// so the profile format stays single-sourced in the client. Profiles are
// global on the daemon; a client applies one on connect via cfg.LoadProfile.
type ProfileBackend interface {
	ListProfiles(ctx context.Context) ([]string, error)
	// GetProfile returns a named profile's bytes. A false result with a nil
	// error means the profile simply doesn't exist (mirrors HasLocalVideo /
	// VideoPosition semantics); a non-nil error means the lookup itself failed.
	GetProfile(ctx context.Context, name string) (data []byte, found bool, err error)
	SaveProfile(ctx context.Context, name string, data []byte) error
}

// StatusBackend surfaces cheap, strictly-local health diagnostics. It never
// does network I/O. In single-binary (InProc) mode it probes the real
// environment (yt-dlp on PATH, cookie source resolves); in remote mode the
// probe belongs daemon-side (a planned health RPC), so the Remote adapter
// returns no issues for now rather than probing the client host.
type StatusBackend interface {
	// CheckAvailability reports non-fatal environment problems that would keep
	// YouTube features from working (empty when all is well). The client shows
	// them in the startup issue overlay alongside config-validation issues.
	CheckAvailability(ctx context.Context) ([]config.ConfigIssue, error)
	// Capabilities reports daemon-side feature switches the client needs to decide
	// its own behavior (e.g. whether the daemon serves thumbnails, which sets the
	// client's thumbnail egress). Cheap; issued once on connect.
	Capabilities(ctx context.Context) (Capabilities, error)
}

// Capabilities carries the daemon-side feature switches a client reads on
// connect. It is deliberately small and additive — new switches append fields.
type Capabilities struct {
	// ThumbnailsEnabled is true when the daemon serves thumbnails (its store is
	// up). When false, a client with a local cache fetches the CDN itself.
	ThumbnailsEnabled bool
}

// Backend is the full contract between the TUI and the data layer, composed
// of the role interfaces above so consumers (transport handlers, TUI tabs)
// can depend on just the slice they use instead of all ~80 methods (H-7).
// InProc implements it by calling db/youtube/downloader directly; Remote
// dials a yt-tuid daemon.
type Backend interface {
	FeedBackend
	ChannelBackend
	VideoBackend
	LibraryBackend
	PlaylistBackend
	HistoryBackend
	DownloadBackend
	PortabilityBackend
	ProfileBackend
	StatusBackend
}
