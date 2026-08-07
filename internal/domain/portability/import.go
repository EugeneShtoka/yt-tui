package portability

// ImportOptions gates how a bundle is applied. Both toggles mirror the export
// side of the locked decisions:
//   - ConvertYTToLocal: a YouTube subscription (subscription_state
//     'subscribed_yt') isn't portable across YouTube accounts. When set, such
//     channels import as *local* subscriptions ('subscribed_local'); when unset
//     they drop to 'none' (annotations are still kept). Decision #1.
//   - IncludeWatchData: merge personal watch data (history + resume positions);
//     default off. Decision #2 (and only meaningful if the bundle actually
//     carries watch data — i.e. it was exported with IncludeWatchData set).
type ImportOptions struct {
	ConvertYTToLocal bool `json:"convert_yt_to_local"`
	IncludeWatchData bool `json:"include_watch_data"`
	// ApplyConfig overwrites the local portable config from the bundle's Config
	// profile (a full overwrite of the portable section; machine-local fields —
	// player, download dir, TLS/cert paths, daemon addr — are never touched).
	// Client-side only: the backend ignores it (config lives on the client), so
	// it needs no wire field. Default off — importing never clobbers a working
	// config unless the user opts in. Only meaningful when the bundle actually
	// carries a config profile (Bundle.Config non-empty).
	ApplyConfig bool `json:"apply_config"`
}

// ImportPlan is the dry-run diff produced by ImportPreview: a non-mutating
// summary of what ImportApply would write against the *current* database, so a
// UI can show it before the user commits. Counts are computed by diffing the
// bundle against existing rows; they are informational and need not be exact
// for correctness (apply is idempotent regardless).
type ImportPlan struct {
	// SchemaVersion is the bundle's declared version; Compatible reports whether
	// this build can apply it (currently: equal to the supported SchemaVersion).
	SchemaVersion int  `json:"schema_version"`
	Compatible    bool `json:"compatible"`

	NewChannels     int `json:"new_channels"`     // channel_ids absent locally
	UpdatedChannels int `json:"updated_channels"` // channel_ids already present
	BlockedChannels int `json:"blocked_channels"` // incoming rows with blocked=1
	NewBlockedNames int `json:"new_blocked_names"`

	NewPlaylists    int `json:"new_playlists"`    // names absent locally
	MergedPlaylists int `json:"merged_playlists"` // names already present
	PlaylistAdds    int `json:"playlist_adds"`    // video refs not yet in their playlist
	Videos          int `json:"videos"`           // shared video-metadata rows to upsert
	NewWatchLater   int `json:"new_watch_later"`  // watch-later ids absent locally
	NewYTPlaylists  int `json:"new_yt_playlists"` // YT playlist refs absent locally

	// Watch data — populated only when the bundle carries it AND the caller opts
	// in via ImportOptions.IncludeWatchData. HasWatchData reflects that combined
	// gate so a UI can show/grey the toggle accordingly.
	HasWatchData bool `json:"has_watch_data"`
	NewHistory   int  `json:"new_history"`   // events not already logged (dedup by id+type+ts)
	NewPositions int  `json:"new_positions"` // positions that would advance (max policy)
}

// ImportResult reports what ImportApply actually wrote. Because apply is
// idempotent, re-importing the same bundle yields an all-zero result.
type ImportResult struct {
	ChannelsUpserted int `json:"channels_upserted"`
	BlockedNames     int `json:"blocked_names"`
	PlaylistsTouched int `json:"playlists_touched"`
	PlaylistAdds     int `json:"playlist_adds"`
	VideosUpserted   int `json:"videos_upserted"`
	WatchLaterAdded  int `json:"watch_later_added"`
	YTPlaylists      int `json:"yt_playlists"`
	HistoryAdded     int `json:"history_added"`
	PositionsSet     int `json:"positions_set"`
}
