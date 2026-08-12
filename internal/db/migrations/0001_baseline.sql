-- 0001_baseline — the initial yt-tui SQLite schema.
--
-- Migrations are numbered NNNN_description.sql and applied in order by migrate()
-- (see migrate.go): each file whose number is greater than the database's
-- PRAGMA user_version runs once, inside a transaction, and then stamps
-- user_version to its own number. A fresh database starts at user_version 0 and
-- ends this migration at 1.
--
-- Because each migration runs exactly once against a known state, statements are
-- plain CREATE (no IF NOT EXISTS): a double-apply is a bug and should fail loudly
-- rather than be silently masked.

CREATE TABLE videos (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    channel TEXT,
    channel_id TEXT,
    duration INTEGER DEFAULT 0,
    view_count INTEGER DEFAULT 0,
    upload_date TEXT,
    url TEXT,
    added_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE local_videos (
    id TEXT PRIMARY KEY REFERENCES videos(id),
    file_path TEXT NOT NULL,
    download_type TEXT DEFAULT 'video',
    downloaded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    status TEXT DEFAULT 'new',
    last_played DATETIME,
    file_size INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    video_id TEXT REFERENCES videos(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    details TEXT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- collections unifies local playlists and cached YouTube playlists into one
-- named-set-of-videos entity. kind discriminates the two: 'local' ids are
-- synthetic "local:N" strings, 'yt' ids are the YouTube playlist id (incl.
-- "WL"). A partial unique index enforces name uniqueness for local rows only;
-- YT titles may collide (their id is the real key).
CREATE TABLE collections (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL,
    name       TEXT NOT NULL,
    synced     INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- collection_videos is the shared membership junction for both playlist kinds.
-- No position column: membership order is the insertion order (rowid) —
-- SaveYTPlaylistVideos writes in sequence and local adds append — and the UI
-- re-sorts, so it's redundant. channel_videos/feed_cache stay separate (order
-- derives from upload_date / a feed ranking).
CREATE TABLE collection_videos (
    collection_id TEXT NOT NULL,
    video_id TEXT NOT NULL REFERENCES videos(id),
    added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (collection_id, video_id)
);

-- No position column: a feed's order is the insertion order (the source's
-- ranking, written in sequence by SaveFeedCache) and the UI re-sorts anyway,
-- so reads order by rowid. Same rationale channel_videos never stored order.
CREATE TABLE feed_cache (
    feed TEXT NOT NULL,
    video_id TEXT NOT NULL,
    PRIMARY KEY (feed, video_id)
);

CREATE TABLE hidden_rec_videos (
    video_id TEXT PRIMARY KEY,
    hidden_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE subscribed_channels (
    channel_id          TEXT PRIMARY KEY,
    name                TEXT NOT NULL DEFAULT '',
    url                 TEXT NOT NULL DEFAULT '',
    subscribers         INTEGER NOT NULL DEFAULT 0,
    alias               TEXT NOT NULL DEFAULT '',
    tags                TEXT NOT NULL DEFAULT '',
    is_local            INTEGER NOT NULL DEFAULT 0,
    subscription_state  TEXT NOT NULL DEFAULT 'none',
    blocked             INTEGER NOT NULL DEFAULT 0,
    videos_refreshed_at INTEGER NOT NULL DEFAULT 0,
    fetched_videos      INTEGER NOT NULL DEFAULT 0,
    last_activity_at    INTEGER NOT NULL DEFAULT 0,
    updated_at          DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE channel_videos (
    channel_id TEXT NOT NULL,
    video_id   TEXT NOT NULL REFERENCES videos(id),
    fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (channel_id, video_id)
);

-- video_details_cache is a key/value enrichment side-store keyed by video_id,
-- deliberately WITHOUT a FK to videos: it is read as a point lookup that
-- decorates a domain.Video the caller already holds (never joined), and it
-- legitimately holds rows for videos not in `videos` (e.g. viewed search
-- results) and lacks rows for videos that are — a hit/miss cache, not a
-- relation. Invalidation is coarse (whole-table clear on column-set change, via
-- the meta fingerprint); the two paths that delete `videos` rows clean it
-- explicitly.
CREATE TABLE video_details_cache (
    video_id      TEXT PRIMARY KEY,
    description   TEXT NOT NULL DEFAULT '',
    thumbnail_url TEXT NOT NULL DEFAULT '',
    subscribers   INTEGER NOT NULL DEFAULT 0,
    links         TEXT,
    chapters      TEXT,
    sb_segments   TEXT,
    fetched_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE activity_log (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    type             TEXT NOT NULL,
    is_local         INTEGER NOT NULL DEFAULT 0,
    channel_id       TEXT,
    channel_name     TEXT,
    playlist_id      TEXT,
    playlist_local_id TEXT,
    playlist_name    TEXT,
    video_id         TEXT,
    video_title      TEXT,
    timestamp        DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE video_positions (
    video_id    TEXT PRIMARY KEY REFERENCES videos(id) ON DELETE CASCADE,
    position_ms INTEGER NOT NULL DEFAULT 0,
    updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes.
CREATE UNIQUE INDEX idx_collections_local_name
    ON collections(name) WHERE kind='local';
CREATE INDEX idx_feed_cache_feed ON feed_cache(feed);
CREATE INDEX idx_history_timestamp ON history(timestamp DESC);
CREATE INDEX idx_history_video ON history(video_id);
CREATE INDEX idx_videos_upload_date ON videos(upload_date DESC);
CREATE INDEX idx_channel_videos_video ON channel_videos(video_id);
CREATE INDEX idx_collection_videos_video ON collection_videos(video_id);
CREATE INDEX idx_feed_cache_video ON feed_cache(video_id);
-- activity_log is read `ORDER BY timestamp DESC LIMIT ?` (history.go); index it.
CREATE INDEX idx_activity_log_timestamp ON activity_log(timestamp DESC);
