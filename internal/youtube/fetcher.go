package youtube

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// pageSize is the number of items fetched per yt-dlp call for paginated requests.
const pageSize = 200

// Client wraps config for plain (non-tea) YouTube fetch operations.
type Client struct {
	cfg *config.Config
}

// NewClient creates a new Client.
func NewClient(cfg *config.Config) *Client {
	return &Client{cfg: cfg}
}

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

func isRateLimited(s string) bool {
	sl := strings.ToLower(s)
	return strings.Contains(sl, "http error 429") ||
		strings.Contains(sl, "too many requests") ||
		strings.Contains(sl, "rate-limited") ||
		strings.Contains(sl, "rate limit")
}

func retryDelay(attempt int) time.Duration {
	return time.Duration(1<<uint(attempt)) * 5 * time.Second
}

// buildArgs builds yt-dlp arguments with an optional upper limit (0 = no limit).
// Used for requests that intentionally cap results (recommended feed, search, channel-latest).
func buildArgs(cfg *config.Config, url string, limit int) []string {
	args := []string{
		"--flat-playlist",
		"--dump-json",
		"--no-warnings",
		"--quiet",
		"--extractor-args", "youtubetab:approximate_date",
		"--sleep-requests", "1",
	}
	if cfg.Browser != "" {
		args = append(args, "--cookies-from-browser", cfg.Browser)
	}
	if limit > 0 {
		args = append(args, "--playlist-end", fmt.Sprintf("%d", limit))
	}
	args = append(args, url)
	return args
}

// buildArgsPage builds yt-dlp arguments for one page of a paginated fetch.
// start is 1-indexed; each page covers [start, start+pageSize-1].
func buildArgsPage(cfg *config.Config, u string, start int) []string {
	end := start + pageSize - 1
	args := []string{
		"--flat-playlist",
		"--dump-json",
		"--no-warnings",
		"--quiet",
		"--extractor-args", "youtubetab:approximate_date",
		"--sleep-requests", "1",
	}
	if cfg.Browser != "" {
		args = append(args, "--cookies-from-browser", cfg.Browser)
	}
	args = append(args,
		"--playlist-start", fmt.Sprintf("%d", start),
		"--playlist-end", fmt.Sprintf("%d", end),
		u)
	return args
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

// parseMixedLines scans yt-dlp output that interleaves channels and videos (search).
func parseMixedLines(r io.Reader) (channels []domain.Channel, videos []domain.Video, err error) {
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
				channels = append(channels, entry.toChannel())
			}
		} else if entry.Title != "" && entry.ViewCount != 0 {
			videos = append(videos, entry.toVideo())
		}
	}
	if err := scanner.Err(); err != nil {
		return channels, videos, fmt.Errorf("parseMixedLines: %w", err)
	}
	return channels, videos, nil
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

func tryParseVideos(args []string) ([]domain.Video, int, string, error) {
	cmd := exec.CommandContext(context.Background(), "yt-dlp", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, "", fmt.Errorf("tryParseVideos stdout: %w", err)
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		return nil, 0, "", fmt.Errorf("tryParseVideos start: %w", err)
	}
	videos, raw, scanErr := parseVideoLines(stdout)
	_ = cmd.Wait()
	return videos, raw, errBuf.String(), scanErr
}

func runAndParseVideos(args []string) ([]domain.Video, int, error) {
	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			d := retryDelay(attempt - 1)
			debug.Log("video fetch rate-limited, retry %d/%d after %v", attempt, maxRetries, d)
			time.Sleep(d)
		}
		videos, raw, stderrStr, err := tryParseVideos(args)
		if stderrStr != "" {
			debug.Log("yt-dlp stderr: %s", strings.TrimSpace(stderrStr))
		}
		if !isRateLimited(stderrStr) || attempt >= maxRetries {
			return videos, raw, err
		}
	}
	return nil, 0, fmt.Errorf("yt-dlp: max retries exceeded (rate limited)")
}

func tryParseChannels(args []string) ([]domain.Channel, int, string, error) {
	cmd := exec.CommandContext(context.Background(), "yt-dlp", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, "", fmt.Errorf("tryParseChannels stdout: %w", err)
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		return nil, 0, "", fmt.Errorf("tryParseChannels start: %w", err)
	}
	channels, raw, scanErr := parseChannelLines(stdout)
	_ = cmd.Wait()
	return channels, raw, errBuf.String(), scanErr
}

func runAndParseChannels(args []string) ([]domain.Channel, int, error) {
	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			d := retryDelay(attempt - 1)
			debug.Log("channel fetch rate-limited, retry %d/%d after %v", attempt, maxRetries, d)
			time.Sleep(d)
		}
		channels, raw, stderrStr, err := tryParseChannels(args)
		if stderrStr != "" {
			debug.Log("yt-dlp stderr: %s", strings.TrimSpace(stderrStr))
		}
		if !isRateLimited(stderrStr) || attempt >= maxRetries {
			return channels, raw, err
		}
	}
	return nil, 0, fmt.Errorf("yt-dlp: max retries exceeded (rate limited)")
}

func tryParsePlaylists(args []string) ([]domain.YTPlaylist, int, string, error) {
	cmd := exec.CommandContext(context.Background(), "yt-dlp", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, 0, "", fmt.Errorf("tryParsePlaylists stdout: %w", err)
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		return nil, 0, "", fmt.Errorf("tryParsePlaylists start: %w", err)
	}
	playlists, raw, scanErr := parsePlaylistLines(stdout)
	_ = cmd.Wait()
	return playlists, raw, errBuf.String(), scanErr
}

func runAndParsePlaylists(args []string) ([]domain.YTPlaylist, int, error) {
	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			d := retryDelay(attempt - 1)
			debug.Log("playlist fetch rate-limited, retry %d/%d after %v", attempt, maxRetries, d)
			time.Sleep(d)
		}
		playlists, raw, stderrStr, err := tryParsePlaylists(args)
		if stderrStr != "" {
			debug.Log("yt-dlp stderr: %s", strings.TrimSpace(stderrStr))
		}
		if !isRateLimited(stderrStr) || attempt >= maxRetries {
			return playlists, raw, err
		}
	}
	return nil, 0, fmt.Errorf("yt-dlp: max retries exceeded (rate limited)")
}

func tryParseMixed(args []string) (channels []domain.Channel, videos []domain.Video, stderrStr string, err error) {
	cmd := exec.CommandContext(context.Background(), "yt-dlp", args...)
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		return nil, nil, "", fmt.Errorf("tryParseMixed stdout: %w", e)
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if e := cmd.Start(); e != nil {
		return nil, nil, "", fmt.Errorf("tryParseMixed start: %w", e)
	}
	channels, videos, scanErr := parseMixedLines(stdout)
	_ = cmd.Wait()
	return channels, videos, errBuf.String(), scanErr
}

func runAndParseMixed(args []string) (channels []domain.Channel, videos []domain.Video, err error) {
	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			d := retryDelay(attempt - 1)
			debug.Log("mixed fetch rate-limited, retry %d/%d after %v", attempt, maxRetries, d)
			time.Sleep(d)
		}
		ch, vid, stderrStr, e := tryParseMixed(args)
		if stderrStr != "" {
			debug.Log("yt-dlp stderr: %s", strings.TrimSpace(stderrStr))
		}
		if !isRateLimited(stderrStr) || attempt >= maxRetries {
			return ch, vid, e
		}
	}
	return nil, nil, fmt.Errorf("yt-dlp: max retries exceeded (rate limited)")
}

func applyStripEmojisVideos(vv []domain.Video) []domain.Video {
	for i := range vv {
		vv[i].Title = StripEmojis(vv[i].Title)
	}
	return vv
}

func applyStripEmojisChannels(cc []domain.Channel) []domain.Channel {
	for i := range cc {
		cc[i].Name = StripEmojis(cc[i].Name)
	}
	return cc
}

// Recommended fetches the recommended feed videos (intentionally capped by config).
func (c *Client) Recommended() ([]domain.Video, error) {
	limit := c.cfg.RecommendedFetchCount
	if limit <= 0 {
		limit = 150
	}
	args := buildArgs(c.cfg, "https://www.youtube.com/feed/recommended", limit)
	videos, _, err := runAndParseVideos(args)
	if c.cfg.StripEmojis {
		videos = applyStripEmojisVideos(videos)
	}
	return videos, err
}

// SubscribedChannels fetches all subscribed channels, paginated.
func (c *Client) SubscribedChannels() ([]domain.Channel, error) {
	u := "https://www.youtube.com/feed/channels"
	var all []domain.Channel
	for start := 1; ; start += pageSize {
		args := buildArgsPage(c.cfg, u, start)
		page, raw, err := runAndParseChannels(args)
		if err != nil {
			return all, err
		}
		if c.cfg.StripEmojis {
			page = applyStripEmojisChannels(page)
		}
		all = append(all, page...)
		if raw < pageSize {
			break
		}
	}
	return all, nil
}

// ChannelVideos fetches all videos for a channel, paginated.
func (c *Client) ChannelVideos(channelURL, channelID string) ([]domain.Video, error) {
	vidURL := channelURL
	if vidURL == "" {
		vidURL = "https://www.youtube.com/channel/" + channelID
	}
	if !strings.HasSuffix(vidURL, "/videos") {
		vidURL += "/videos"
	}
	var all []domain.Video
	for start := 1; ; start += pageSize {
		args := buildArgsPage(c.cfg, vidURL, start)
		page, raw, err := runAndParseVideos(args)
		if err != nil {
			return all, err
		}
		if c.cfg.StripEmojis {
			page = applyStripEmojisVideos(page)
		}
		all = append(all, page...)
		if raw < pageSize {
			break
		}
	}
	return all, nil
}

// ChannelLatest fetches the cfg.ChannelLatestCount most recent videos for a channel.
func (c *Client) ChannelLatest(channelURL, channelID string) ([]domain.Video, error) {
	return c.ChannelLatestN(channelURL, channelID, c.cfg.ChannelLatestCount)
}

// ChannelLatestN fetches at most n recent videos for a channel (intentionally capped).
func (c *Client) ChannelLatestN(channelURL, channelID string, n int) ([]domain.Video, error) {
	vidURL := channelURL
	if vidURL == "" {
		vidURL = "https://www.youtube.com/channel/" + channelID
	}
	if !strings.HasSuffix(vidURL, "/videos") {
		vidURL += "/videos"
	}
	args := buildArgs(c.cfg, vidURL, n)
	videos, _, err := runAndParseVideos(args)
	if c.cfg.StripEmojis {
		videos = applyStripEmojisVideos(videos)
	}
	return videos, err
}

// Search searches YouTube for the given query (intentionally capped results).
func (c *Client) Search(query string) (channels []domain.Channel, videos []domain.Video, err error) {
	type chResult struct {
		channels []domain.Channel
		err      error
	}
	chCh := make(chan chResult, 1)
	go func() {
		chURL := "https://www.youtube.com/results?search_query=" +
			url.QueryEscape(query) + "&sp=EgIQAg%3D%3D"
		args := buildArgs(c.cfg, chURL, 10)
		chs, _, chErr := runAndParseMixed(args)
		if c.cfg.StripEmojis {
			chs = applyStripEmojisChannels(chs)
		}
		chCh <- chResult{chs, chErr}
	}()

	vidArgs := buildArgs(c.cfg, "ytsearch25:"+query, 25)
	_, videos, err = runAndParseMixed(vidArgs)
	if c.cfg.StripEmojis {
		videos = applyStripEmojisVideos(videos)
	}

	cr := <-chCh
	if err == nil && cr.err != nil {
		err = cr.err
	}
	return cr.channels, videos, err
}

// YTPlaylists fetches all user playlists, paginated.
func (c *Client) YTPlaylists() ([]domain.YTPlaylist, error) {
	u := "https://www.youtube.com/feed/playlists"
	var all []domain.YTPlaylist
	for start := 1; ; start += pageSize {
		args := buildArgsPage(c.cfg, u, start)
		page, raw, err := runAndParsePlaylists(args)
		if err != nil {
			return all, err
		}
		all = append(all, page...)
		if raw < pageSize {
			break
		}
	}
	return all, nil
}

// PlaylistVideos fetches all videos for a YouTube playlist, paginated.
func (c *Client) PlaylistVideos(playlistID string) ([]domain.Video, error) {
	u := "https://www.youtube.com/playlist?list=" + playlistID
	var all []domain.Video
	for start := 1; ; start += pageSize {
		args := buildArgsPage(c.cfg, u, start)
		page, raw, err := runAndParseVideos(args)
		if err != nil {
			return all, err
		}
		if c.cfg.StripEmojis {
			page = applyStripEmojisVideos(page)
		}
		all = append(all, page...)
		if raw < pageSize {
			break
		}
	}
	return all, nil
}

type ytdlpDetailChapter struct {
	Title     string  `json:"title"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

type ytdlpDetailEntry struct {
	ID                   string               `json:"id"`
	Title                string               `json:"title"`
	Channel              string               `json:"channel"`
	ChannelID            string               `json:"channel_id"`
	Duration             float64              `json:"duration"`
	ViewCount            int64                `json:"view_count"`
	UploadDate           string               `json:"upload_date"`
	WebpageURL           string               `json:"webpage_url"`
	Description          string               `json:"description"`
	Thumbnail            string               `json:"thumbnail"`
	ChannelFollowerCount int64                `json:"channel_follower_count"`
	Chapters             []ytdlpDetailChapter `json:"chapters"`
}

// VideoDetails fetches detailed info for a single video URL.
func (c *Client) VideoDetails(videoURL string) (domain.VideoDetails, error) {
	args := []string{"--dump-json", "--no-warnings", "--quiet"}
	if c.cfg.Browser != "" {
		args = append(args, "--cookies-from-browser", c.cfg.Browser)
	}
	args = append(args, videoURL)

	cmd := exec.CommandContext(context.Background(), "yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		return domain.VideoDetails{}, fmt.Errorf("yt-dlp: %w", err)
	}
	var e ytdlpDetailEntry
	if err := json.Unmarshal(out, &e); err != nil {
		return domain.VideoDetails{}, fmt.Errorf("parse: %w", err)
	}
	u := e.WebpageURL
	if u == "" && e.ID != "" {
		u = "https://www.youtube.com/watch?v=" + e.ID
	}
	title := e.Title
	if c.cfg.StripEmojis {
		title = StripEmojis(title)
	}
	chapters := make([]domain.RawChapter, len(e.Chapters))
	for i, ch := range e.Chapters {
		chapters[i] = domain.RawChapter{Title: ch.Title, StartTime: ch.StartTime, EndTime: ch.EndTime}
	}
	return domain.VideoDetails{
		Video: domain.Video{
			ID:         e.ID,
			Title:      title,
			Channel:    e.Channel,
			ChannelID:  e.ChannelID,
			Duration:   int(e.Duration),
			ViewCount:  e.ViewCount,
			UploadDate: e.UploadDate,
			URL:        u,
		},
		Description:  e.Description,
		ThumbnailURL: e.Thumbnail,
		Subscribers:  e.ChannelFollowerCount,
		Chapters:     chapters,
	}, nil
}
