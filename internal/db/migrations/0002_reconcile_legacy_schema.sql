-- 0002_reconcile_legacy_schema — bring pre-runner databases up to the 0001 shape.
--
-- Databases created before this runner existed were stamped
-- PRAGMA user_version = 2 by hand, so migrate() skipped 0001 (and would skip
-- every migration up to 2), freezing their ad-hoc schema forever. That schema is
-- baseline-equivalent plus known drift, and one drift silently broke the
-- recommended feed: feed_cache carried a `position INTEGER NOT NULL` column that
-- SaveFeedCache — which writes only (feed, video_id) — can never populate, so
-- every feed-cache write failed the NOT NULL constraint and rolled back. The
-- feed refetched on every open and never persisted. migrate() detects those
-- databases (hasLegacySchema) and treats them as version 1 so this runs.
--
-- Every statement is valid against BOTH shapes, the legacy one and a fresh 0001
-- database, because the runner cannot branch on schema: each drifted table is
-- rebuilt selecting only the columns 0001 defines, so a legacy extra column is
-- dropped and a fresh database is rebuilt unchanged. Copies are strict (no
-- orphan filtering) — every FK here was already enforced in the legacy schema,
-- so a violation means deeper corruption and should fail loudly rather than
-- silently drop rows.

-- feed_cache: drop the unpopulatable `position` column. Rows copy in rowid
-- order, which is the ranking they were written in (see GetFeedCache).
CREATE TABLE feed_cache_new (
    feed TEXT NOT NULL,
    video_id TEXT NOT NULL,
    PRIMARY KEY (feed, video_id)
);
INSERT INTO feed_cache_new (feed, video_id)
    SELECT feed, video_id FROM feed_cache;
DROP TABLE feed_cache;
ALTER TABLE feed_cache_new RENAME TO feed_cache;
CREATE INDEX idx_feed_cache_feed ON feed_cache(feed);
CREATE INDEX idx_feed_cache_video ON feed_cache(video_id);

-- local_videos: drop the dead last_position_ms column — video_positions owns
-- resume positions since the schema consolidation.
CREATE TABLE local_videos_new (
    id TEXT PRIMARY KEY REFERENCES videos(id),
    file_path TEXT NOT NULL,
    download_type TEXT DEFAULT 'video',
    downloaded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    status TEXT DEFAULT 'new',
    last_played DATETIME,
    file_size INTEGER NOT NULL DEFAULT 0
);
INSERT INTO local_videos_new
    (id, file_path, download_type, downloaded_at, status, last_played, file_size)
    SELECT id, file_path, download_type, downloaded_at, status, last_played, file_size
    FROM local_videos;
DROP TABLE local_videos;
ALTER TABLE local_videos_new RENAME TO local_videos;

-- activity_log: playlist_local_id holds a synthetic "local:N" collection id — a
-- string since playlist ids went int64 -> string — so the legacy INTEGER
-- affinity would coerce numeric-looking ids. Rebuild it as TEXT, and restore the
-- timestamp index the legacy schema never had (history.go reads
-- ORDER BY timestamp DESC LIMIT ?).
CREATE TABLE activity_log_new (
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
INSERT INTO activity_log_new
    (id, type, is_local, channel_id, channel_name, playlist_id, playlist_local_id,
     playlist_name, video_id, video_title, timestamp)
    SELECT id, type, is_local, channel_id, channel_name, playlist_id, playlist_local_id,
           playlist_name, video_id, video_title, timestamp
    FROM activity_log;
DROP TABLE activity_log;
ALTER TABLE activity_log_new RENAME TO activity_log;
CREATE INDEX idx_activity_log_timestamp ON activity_log(timestamp DESC);

-- blocked_names: name-based channel blocking was removed; blocking is ID-only.
DROP TABLE IF EXISTS blocked_names;
