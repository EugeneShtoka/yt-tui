package youtube

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
)

// ytdlpEntry is the raw JSON from yt-dlp --flat-playlist --dump-json.
type ytdlpEntry struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	Uploader         string  `json:"uploader"`
	Channel          string  `json:"channel"`
	ChannelID        string  `json:"channel_id"`
	PlaylistChannel  string  `json:"playlist_channel"`
	PlaylistUploader string  `json:"playlist_uploader"`
	Duration         float64 `json:"duration"`
	ViewCount        int64   `json:"view_count"`
	UploadDate       string  `json:"upload_date"`
	WebpageURL           string  `json:"webpage_url"`
	URL                  string  `json:"url"`
	ChannelURL           string  `json:"channel_url"`
	IEKey                string  `json:"ie_key"`
	Type                 string  `json:"_type"`
	ChannelFollowerCount int64   `json:"channel_follower_count"`
}

func (e ytdlpEntry) toVideo() Video {
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
	return Video{
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

func (e ytdlpEntry) toChannel() Channel {
	u := e.URL
	if u == "" && e.ID != "" {
		u = "https://www.youtube.com/channel/" + e.ID
	}
	name := e.Title
	if name == "" {
		name = e.Channel
	}
	return Channel{ID: e.ID, Name: name, URL: u, Subscribers: e.ChannelFollowerCount}
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

func tryParseVideos(args []string) ([]Video, string, error) {
	cmd := exec.Command("yt-dlp", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", err
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Start(); err != nil {
		return nil, "", err
	}
	var videos []Video
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)
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
		if e.ViewCount == 0 {
			continue
		}
		videos = append(videos, e.toVideo())
	}
	_ = cmd.Wait()
	return videos, errBuf.String(), scanner.Err()
}

func runAndParseVideos(args []string) ([]Video, error) {
	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			d := retryDelay(attempt - 1)
			debug.Log("video fetch rate-limited, retry %d/%d after %v", attempt, maxRetries, d)
			time.Sleep(d)
		}
		videos, stderrStr, err := tryParseVideos(args)
		if stderrStr != "" {
			debug.Log("yt-dlp stderr: %s", strings.TrimSpace(stderrStr))
		}
		if !isRateLimited(stderrStr) || attempt >= maxRetries {
			return videos, err
		}
	}
	return nil, fmt.Errorf("yt-dlp: max retries exceeded (rate limited)")
}

func tryParseChannels(args []string) ([]Channel, string, error) {
	cmd := exec.Command("yt-dlp", args...)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	out, err := cmd.Output()
	stderrStr := errBuf.String()
	if err != nil {
		return nil, stderrStr, fmt.Errorf("yt-dlp: %w", err)
	}
	var channels []Channel
	for _, line := range strings.Split(string(out), "\n") {
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
		channels = append(channels, e.toChannel())
	}
	return channels, stderrStr, nil
}

func runAndParseChannels(args []string) ([]Channel, error) {
	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			d := retryDelay(attempt - 1)
			debug.Log("channel fetch rate-limited, retry %d/%d after %v", attempt, maxRetries, d)
			time.Sleep(d)
		}
		channels, stderrStr, err := tryParseChannels(args)
		if stderrStr != "" {
			debug.Log("yt-dlp stderr: %s", strings.TrimSpace(stderrStr))
		}
		if !isRateLimited(stderrStr) || attempt >= maxRetries {
			return channels, err
		}
	}
	return nil, fmt.Errorf("yt-dlp: max retries exceeded (rate limited)")
}

func applyStripEmojisVideos(vv []Video) []Video {
	for i := range vv {
		vv[i].Title = StripEmojis(vv[i].Title)
	}
	return vv
}

func applyStripEmojisChannels(cc []Channel) []Channel {
	for i := range cc {
		cc[i].Name = StripEmojis(cc[i].Name)
	}
	return cc
}

// YTPlaylist is a YouTube playlist (ID + title).
type YTPlaylist struct {
	ID    string
	Title string
}

// --- Bubbletea message types ---

type FetchResultMsg struct {
	Source string
	Videos []Video
	Err    error
}

type ChannelListMsg struct {
	Channels   []Channel
	Err        error
	Background bool // true = startup silent fetch; don't update subChannels/subChLoaded/status
}

type ChannelVideosMsg struct {
	Source    string // "search" or "subscriptions"
	ChannelID string
	Videos    []Video
	Err       error
}

type SearchResultMsg struct {
	Query    string
	Channels []Channel
	Videos   []Video
	Err      error
}

type YTPlaylistsMsg struct {
	Playlists  []YTPlaylist
	Err        error
	Background bool
}

type PlaylistVideosMsg struct {
	PlaylistID string
	Videos     []Video
	Err        error
}

type VideoDetailsMsg struct {
	Details VideoDetails
	Err     error
}

// --- Fetch commands ---

func FetchRecommended(cfg *config.Config) tea.Cmd {
	limit := cfg.RecommendedFetchCount
	if limit <= 0 {
		limit = 150
	}
	return func() tea.Msg {
		args := buildArgs(cfg, "https://www.youtube.com/feed/recommended", limit)
		videos, err := runAndParseVideos(args)
		if cfg.StripEmojis {
			videos = applyStripEmojisVideos(videos)
		}
		return FetchResultMsg{Source: "recommended", Videos: videos, Err: err}
	}
}

func FetchSubscribedChannels(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		args := buildArgs(cfg, "https://www.youtube.com/feed/channels", 0)
		channels, err := runAndParseChannels(args)
		if cfg.StripEmojis {
			channels = applyStripEmojisChannels(channels)
		}
		return ChannelListMsg{Channels: channels, Err: err}
	}
}

// FetchSubscribedChannelsBackground fetches subscribed channels silently at
// startup to populate the filter without touching subscriptions UI state.
func FetchSubscribedChannelsBackground(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		args := buildArgs(cfg, "https://www.youtube.com/feed/channels", 0)
		channels, err := runAndParseChannels(args)
		if cfg.StripEmojis {
			channels = applyStripEmojisChannels(channels)
		}
		return ChannelListMsg{Channels: channels, Err: err, Background: true}
	}
}

func FetchChannelVideos(cfg *config.Config, channelURL, channelID, source string) tea.Cmd {
	return func() tea.Msg {
		vidURL := channelURL
		if vidURL == "" {
			vidURL = "https://www.youtube.com/channel/" + channelID
		}
		if !strings.HasSuffix(vidURL, "/videos") {
			vidURL += "/videos"
		}
		args := buildArgs(cfg, vidURL, 0)
		videos, err := runAndParseVideos(args)
		if cfg.StripEmojis {
			videos = applyStripEmojisVideos(videos)
		}
		return ChannelVideosMsg{Source: source, ChannelID: channelID, Videos: videos, Err: err}
	}
}

// FetchChannelLatest fetches the cfg.ChannelLatestCount most recent videos for a channel.
// Used for background population of the channel list without a full fetch.
func FetchChannelLatest(cfg *config.Config, channelURL, channelID string) tea.Cmd {
	return FetchChannelLatestN(cfg, channelURL, channelID, cfg.ChannelLatestCount)
}

// FetchChannelLatestN fetches at most n recent videos for a channel silently in background.
func FetchChannelLatestN(cfg *config.Config, channelURL, channelID string, n int) tea.Cmd {
	return func() tea.Msg {
		vidURL := channelURL
		if vidURL == "" {
			vidURL = "https://www.youtube.com/channel/" + channelID
		}
		if !strings.HasSuffix(vidURL, "/videos") {
			vidURL += "/videos"
		}
		args := buildArgs(cfg, vidURL, n)
		videos, err := runAndParseVideos(args)
		if cfg.StripEmojis {
			videos = applyStripEmojisVideos(videos)
		}
		return ChannelVideosMsg{Source: "ch-background", ChannelID: channelID, Videos: videos, Err: err}
	}
}

func tryParseMixed(args []string) (channels []Channel, videos []Video, stderrStr string, err error) {
	cmd := exec.Command("yt-dlp", args...)
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		return nil, nil, "", e
	}
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if e := cmd.Start(); e != nil {
		return nil, nil, "", e
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 2*1024*1024), 2*1024*1024)
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
	_ = cmd.Wait()
	return channels, videos, errBuf.String(), scanner.Err()
}

func runAndParseMixed(args []string) (channels []Channel, videos []Video, err error) {
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

func Search(cfg *config.Config, query string) tea.Cmd {
	return func() tea.Msg {
		// Run channel search concurrently with video search.
		// ytsearch25: only returns videos; channels need the YouTube channel filter.
		type chResult struct {
			channels []Channel
			err      error
		}
		chCh := make(chan chResult, 1)
		go func() {
			chURL := "https://www.youtube.com/results?search_query=" +
				url.QueryEscape(query) + "&sp=EgIQAg%3D%3D"
			args := buildArgs(cfg, chURL, 10)
			channels, _, err := runAndParseMixed(args)
			if cfg.StripEmojis {
				channels = applyStripEmojisChannels(channels)
			}
			chCh <- chResult{channels, err}
		}()

		vidArgs := buildArgs(cfg, "ytsearch25:"+query, 25)
		_, videos, err := runAndParseMixed(vidArgs)
		if cfg.StripEmojis {
			videos = applyStripEmojisVideos(videos)
		}

		cr := <-chCh
		if err == nil && cr.err != nil {
			err = cr.err
		}
		return SearchResultMsg{Query: query, Channels: cr.channels, Videos: videos, Err: err}
	}
}

func FetchYTPlaylists(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		args := buildArgs(cfg, "https://www.youtube.com/feed/playlists", 0)
		playlists, err := runAndParsePlaylists(args)
		return YTPlaylistsMsg{Playlists: playlists, Err: err}
	}
}

func FetchYTPlaylistsBackground(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		args := buildArgs(cfg, "https://www.youtube.com/feed/playlists", 0)
		playlists, err := runAndParsePlaylists(args)
		return YTPlaylistsMsg{Playlists: playlists, Err: err, Background: true}
	}
}

func FetchPlaylistVideos(cfg *config.Config, playlistID string) tea.Cmd {
	return func() tea.Msg {
		url := "https://www.youtube.com/playlist?list=" + playlistID
		args := buildArgs(cfg, url, 0)
		videos, err := runAndParseVideos(args)
		if cfg.StripEmojis {
			videos = applyStripEmojisVideos(videos)
		}
		return PlaylistVideosMsg{PlaylistID: playlistID, Videos: videos, Err: err}
	}
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

func FetchVideoDetails(cfg *config.Config, videoURL string) tea.Cmd {
	return func() tea.Msg {
		args := []string{"--dump-json", "--no-warnings", "--quiet"}
		if cfg.Browser != "" {
			args = append(args, "--cookies-from-browser", cfg.Browser)
		}
		args = append(args, videoURL)

		cmd := exec.Command("yt-dlp", args...)
		out, err := cmd.Output()
		if err != nil {
			return VideoDetailsMsg{Err: fmt.Errorf("yt-dlp: %w", err)}
		}
		var e ytdlpDetailEntry
		if err := json.Unmarshal(out, &e); err != nil {
			return VideoDetailsMsg{Err: fmt.Errorf("parse: %w", err)}
		}
		u := e.WebpageURL
		if u == "" && e.ID != "" {
			u = "https://www.youtube.com/watch?v=" + e.ID
		}
		title := e.Title
		if cfg.StripEmojis {
			title = StripEmojis(title)
		}
		chapters := make([]Chapter, len(e.Chapters))
		for i, c := range e.Chapters {
			chapters[i] = Chapter{Title: c.Title, StartTime: c.StartTime, EndTime: c.EndTime}
		}
		return VideoDetailsMsg{Details: VideoDetails{
			Video: Video{
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
		}}
	}
}

func runAndParsePlaylists(args []string) ([]YTPlaylist, error) {
	cmd := exec.Command("yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp: %w", err)
	}
	var playlists []YTPlaylist
	for _, line := range strings.Split(string(out), "\n") {
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
			title := e.Title
			if title == "" {
				title = e.ID
			}
			playlists = append(playlists, YTPlaylist{ID: e.ID, Title: title})
		}
	}
	return playlists, nil
}
