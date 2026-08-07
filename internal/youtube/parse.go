package youtube

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ytdlpEntry is the raw JSON from yt-dlp --flat-playlist --dump-json.
type ytdlpEntry struct {
	ID                   string  `json:"id"`
	Title                string  `json:"title"`
	Uploader             string  `json:"uploader"`
	Channel              string  `json:"channel"`
	ChannelID            string  `json:"channel_id"`
	PlaylistChannel      string  `json:"playlist_channel"`
	PlaylistUploader     string  `json:"playlist_uploader"`
	Duration             float64 `json:"duration"`
	ViewCount            int64   `json:"view_count"`
	UploadDate           string  `json:"upload_date"`
	WebpageURL           string  `json:"webpage_url"`
	URL                  string  `json:"url"`
	ChannelURL           string  `json:"channel_url"`
	IEKey                string  `json:"ie_key"`
	Type                 string  `json:"_type"`
	ChannelFollowerCount int64   `json:"channel_follower_count"`
}

func (e ytdlpEntry) toVideo() domain.Video {
	ch := e.Channel
	if ch == "" {
		ch = e.PlaylistChannel
	}
	if ch == "" {
		ch = e.Uploader
	}
	if ch == "" {
		ch = e.PlaylistUploader
	}
	chID := e.ChannelID
	if chID == "" {
		if parts := strings.SplitN(e.ChannelURL, "/channel/", 2); len(parts) == 2 {
			chID = strings.SplitN(parts[1], "/", 2)[0]
		}
	}
	u := e.WebpageURL
	if u == "" && e.ID != "" {
		u = "https://www.youtube.com/watch?v=" + e.ID
	}
	return domain.Video{
		ID:         e.ID,
		Title:      e.Title,
		Channel:    ch,
		ChannelID:  chID,
		Duration:   int(e.Duration),
		ViewCount:  e.ViewCount,
		UploadDate: e.UploadDate,
		URL:        u,
	}
}

func (e ytdlpEntry) toChannel() domain.Channel {
	u := e.URL
	if u == "" && e.ID != "" {
		u = "https://www.youtube.com/channel/" + e.ID
	}
	name := e.Title
	if name == "" {
		name = e.Channel
	}
	return domain.Channel{ID: e.ID, Name: name, URL: u, Subscribers: e.ChannelFollowerCount}
}

// newLineScanner returns a bufio.Scanner sized for yt-dlp's long JSON lines.
func newLineScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)
	return scanner
}

// parseVideoLines scans yt-dlp --dump-json output, keeping only real videos.
// Returns (videos, rawCount, err) where rawCount is the number of valid entries
// seen before the ViewCount==0 (member-only) filter, used for pagination decisions.
func parseVideoLines(r io.Reader) ([]domain.Video, int, error) {
	var videos []domain.Video
	raw := 0
	scanner := newLineScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var e ytdlpEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if e.ID == "" || e.Title == "" {
			continue
		}
		if e.IEKey == "YoutubeTab" || e.Type == "playlist" {
			continue
		}
		raw++ // count before member-only filter
		if e.ViewCount == 0 {
			continue
		}
		videos = append(videos, e.toVideo())
	}
	if err := scanner.Err(); err != nil {
		return videos, raw, fmt.Errorf("parseVideoLines: %w", err)
	}
	return videos, raw, nil
}

// parseChannelLines scans yt-dlp output for channel entries.
// Returns (channels, rawCount, err) for pagination decisions.
func parseChannelLines(r io.Reader) ([]domain.Channel, int, error) {
	var channels []domain.Channel
	raw := 0
	scanner := newLineScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var e ytdlpEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if e.ID == "" {
			continue
		}
		raw++
		channels = append(channels, e.toChannel())
	}
	if err := scanner.Err(); err != nil {
		return channels, raw, fmt.Errorf("parseChannelLines: %w", err)
	}
	return channels, raw, nil
}

// mixedResult bundles the two result slices from a search fetch so it can flow
// through the generic runWithRetry pipeline as a single value.
type mixedResult struct {
	channels []domain.Channel
	videos   []domain.Video
}

// parseMixedLines scans yt-dlp output that interleaves channels and videos (search).
func parseMixedLines(r io.Reader) (mixedResult, int, error) {
	var res mixedResult
	scanner := newLineScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var entry ytdlpEntry
		if json.Unmarshal([]byte(line), &entry) != nil || entry.ID == "" {
			continue
		}
		if entry.IEKey == "YoutubeTab" || entry.Type == "playlist" {
			if entry.Title != "" {
				res.channels = append(res.channels, entry.toChannel())
			}
		} else if entry.Title != "" && entry.ViewCount != 0 {
			res.videos = append(res.videos, entry.toVideo())
		}
	}
	if err := scanner.Err(); err != nil {
		return res, len(res.channels) + len(res.videos), fmt.Errorf("parseMixedLines: %w", err)
	}
	return res, len(res.channels) + len(res.videos), nil
}

// parsePlaylistLines scans yt-dlp output for playlist entries.
// Returns (playlists, rawCount, err) for pagination decisions.
func parsePlaylistLines(r io.Reader) ([]domain.YTPlaylist, int, error) {
	var playlists []domain.YTPlaylist
	raw := 0
	scanner := newLineScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var e ytdlpEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if e.ID == "" {
			continue
		}
		if e.Type == "playlist" || e.IEKey == "YoutubeTab" {
			raw++
			title := e.Title
			if title == "" {
				title = e.ID
			}
			playlists = append(playlists, domain.YTPlaylist{ID: e.ID, Title: title})
		}
	}
	if err := scanner.Err(); err != nil {
		return playlists, raw, fmt.Errorf("parsePlaylistLines: %w", err)
	}
	return playlists, raw, nil
}
