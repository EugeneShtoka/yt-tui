package db

import (
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

// SaveChannelVideos upserts all videos for a channel and links them.
func (d *DB) SaveChannelVideos(channelID string, videos []youtube.Video) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, v := range videos {
		if _, err := tx.Exec(`
			INSERT INTO videos (id, title, channel, channel_id, duration, view_count, upload_date, url)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				title=excluded.title, channel=excluded.channel,
				channel_id=COALESCE(NULLIF(excluded.channel_id,''), channel_id),
				duration=excluded.duration, view_count=excluded.view_count,
				upload_date=excluded.upload_date, url=excluded.url
		`, v.ID, v.Title, v.Channel, v.ChannelID, v.Duration, v.ViewCount, v.UploadDate, v.URL); err != nil {
			return err
		}
		if _, err := tx.Exec(`
			INSERT OR IGNORE INTO channel_videos (channel_id, video_id) VALUES (?, ?)
		`, channelID, v.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetChannelVideos returns persisted videos for a channel, newest first.
func (d *DB) GetChannelVideos(channelID string) ([]youtube.Video, error) {
	rows, err := d.sql.Query(`
		SELECT v.id, v.title, COALESCE(v.channel,''), COALESCE(v.channel_id,''),
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM channel_videos cv
		JOIN videos v ON v.id = cv.video_id
		WHERE cv.channel_id = ?
		ORDER BY v.upload_date DESC
	`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []youtube.Video
	for rows.Next() {
		var v youtube.Video
		if err := rows.Scan(&v.ID, &v.Title, &v.Channel, &v.ChannelID,
			&v.Duration, &v.ViewCount, &v.UploadDate, &v.URL); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

// SaveSubscribedChannels persists the full channel list, preserving alias and tags for existing channels.
// Only non-local (YT-subscribed) channels can be removed; local subscriptions are preserved.
func (d *DB) SaveSubscribedChannels(channels []youtube.Channel) error {
	if len(channels) == 0 {
		return nil
	}
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Remove YT-managed channels that are no longer subscribed (preserve is_local=1 entries).
	ph := make([]string, len(channels))
	ids := make([]interface{}, len(channels))
	for i, ch := range channels {
		ph[i] = "?"
		ids[i] = ch.ID
	}
	if _, err := tx.Exec(
		`DELETE FROM subscribed_channels WHERE is_local=0 AND channel_id NOT IN (`+strings.Join(ph, ",")+`)`,
		ids...,
	); err != nil {
		return err
	}
	// Upsert — alias and tags are intentionally excluded from the UPDATE SET so they are preserved.
	for _, ch := range channels {
		if ch.ID == "" {
			continue
		}
		if _, err := tx.Exec(`
			INSERT INTO subscribed_channels (channel_id, name, url, subscribers, is_local)
			VALUES (?, ?, ?, ?, 0)
			ON CONFLICT(channel_id) DO UPDATE SET
				name=excluded.name, url=excluded.url,
				subscribers=excluded.subscribers,
				is_local=0,
				updated_at=CURRENT_TIMESTAMP
		`, ch.ID, ch.Name, ch.URL, ch.Subscribers); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RemoveSubscribedChannel removes a single channel from the local subscriptions DB.
func (d *DB) RemoveSubscribedChannel(channelID string) error {
	_, err := d.sql.Exec(`DELETE FROM subscribed_channels WHERE channel_id=?`, channelID)
	return err
}

// DeleteChannelVideos removes all channel_videos rows for a given channel.
func (d *DB) DeleteChannelVideos(channelID string) error {
	_, err := d.sql.Exec(`DELETE FROM channel_videos WHERE channel_id=?`, channelID)
	return err
}

// GetSubscribedChannels returns the persisted channel list including any user-set alias and tags.
func (d *DB) GetSubscribedChannels() ([]youtube.Channel, error) {
	rows, err := d.sql.Query(`
		SELECT channel_id, name, url, subscribers,
		       COALESCE(alias,''), COALESCE(tags,''), COALESCE(is_local,0)
		FROM subscribed_channels ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []youtube.Channel
	for rows.Next() {
		var ch youtube.Channel
		var tagsStr string
		var isLocal int
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.URL, &ch.Subscribers, &ch.Alias, &tagsStr, &isLocal); err != nil {
			return nil, err
		}
		if tagsStr != "" {
			ch.Tags = strings.Split(tagsStr, ",")
		}
		ch.IsLocal = isLocal == 1
		out = append(out, ch)
	}
	return out, rows.Err()
}

// AddSubscribedChannel upserts a single channel, preserving any existing alias and tags.
func (d *DB) AddSubscribedChannel(ch youtube.Channel) error {
	isLocal := 0
	if ch.IsLocal {
		isLocal = 1
	}
	_, err := d.sql.Exec(`
		INSERT INTO subscribed_channels (channel_id, name, url, subscribers, is_local)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(channel_id) DO UPDATE SET
			name=excluded.name, url=excluded.url,
			subscribers=excluded.subscribers,
			is_local=excluded.is_local,
			updated_at=CURRENT_TIMESTAMP
	`, ch.ID, ch.Name, ch.URL, ch.Subscribers, isLocal)
	return err
}

// SetChannelAlias sets or clears the display-name alias for a subscribed channel.
func (d *DB) SetChannelAlias(channelID, alias string) error {
	_, err := d.sql.Exec(`UPDATE subscribed_channels SET alias=? WHERE channel_id=?`, alias, channelID)
	return err
}

// SetChannelTags replaces the tag list for a subscribed channel.
func (d *DB) SetChannelTags(channelID string, tags []string) error {
	_, err := d.sql.Exec(`UPDATE subscribed_channels SET tags=? WHERE channel_id=?`,
		strings.Join(tags, ","), channelID)
	return err
}

// GetAllChannelVideos returns all videos for the given channel IDs, newest first.
func (d *DB) GetAllChannelVideos(channelIDs []string) ([]youtube.Video, error) {
	if len(channelIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(channelIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(channelIDs))
	for i, id := range channelIDs {
		args[i] = id
	}
	rows, err := d.sql.Query(`
		SELECT v.id, v.title, COALESCE(v.channel,''), cv.channel_id,
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM channel_videos cv
		JOIN videos v ON v.id = cv.video_id
		WHERE cv.channel_id IN (`+placeholders+`)
		ORDER BY v.upload_date DESC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []youtube.Video
	for rows.Next() {
		var v youtube.Video
		if err := rows.Scan(&v.ID, &v.Title, &v.Channel, &v.ChannelID,
			&v.Duration, &v.ViewCount, &v.UploadDate, &v.URL); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetChannelLatestAll returns the most recent video per channel derived from channel_videos.
func (d *DB) GetChannelLatestAll() (map[string]youtube.Video, error) {
	rows, err := d.sql.Query(`
		WITH latest AS (
			SELECT cv.channel_id, MAX(v.upload_date) AS max_date
			FROM channel_videos cv
			JOIN videos v ON v.id = cv.video_id
			GROUP BY cv.channel_id
		)
		SELECT l.channel_id, v.id, v.title, COALESCE(v.channel,''), COALESCE(v.channel_id,''),
		       COALESCE(v.duration,0), COALESCE(v.view_count,0),
		       COALESCE(v.upload_date,''), COALESCE(v.url,'')
		FROM latest l
		JOIN channel_videos cv ON cv.channel_id = l.channel_id
		JOIN videos v ON v.id = cv.video_id AND v.upload_date = l.max_date
		GROUP BY l.channel_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]youtube.Video)
	for rows.Next() {
		var chID string
		var v youtube.Video
		if err := rows.Scan(&chID, &v.ID, &v.Title, &v.Channel, &v.ChannelID,
			&v.Duration, &v.ViewCount, &v.UploadDate, &v.URL); err != nil {
			return nil, err
		}
		out[chID] = v
	}
	return out, rows.Err()
}

// ChannelHideStats returns count of hidden videos and played videos for a channel.
func (d *DB) ChannelHideStats(channelID string) (hidden, played int, err error) {
	err = d.sql.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM hidden_rec_videos hrv
			 JOIN videos v ON v.id = hrv.video_id
			 WHERE v.channel_id = ?) AS hidden_count,
			(SELECT COUNT(*) FROM history h
			 JOIN videos v ON v.id = h.video_id
			 WHERE v.channel_id = ? AND h.event_type IN ('playVideo','playAudio','streamVideo','streamAudio')) AS play_count
	`, channelID, channelID).Scan(&hidden, &played)
	return hidden, played, err
}
