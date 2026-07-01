package youtube

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/EugeneShtoka/yt-tui/internal/config"
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
	WebpageURL       string  `json:"webpage_url"`
	URL              string  `json:"url"`
	IEKey            string  `json:"ie_key"`
	Type             string  `json:"_type"`
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
	u := e.WebpageURL
	if u == "" && e.ID != "" {
		u = "https://www.youtube.com/watch?v=" + e.ID
	}
	return Video{
		ID:         e.ID,
		Title:      e.Title,
		Channel:    ch,
		ChannelID:  e.ChannelID,
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
	return Channel{ID: e.ID, Name: name, URL: u}
}

func buildArgs(cfg *config.Config, url string, limit int) []string {
	args := []string{
		"--flat-playlist",
		"--dump-json",
		"--no-warnings",
		"--quiet",
		"--extractor-args", "youtubetab:approximate_date",
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

func runAndParseVideos(args []string) ([]Video, error) {
	cmd := exec.Command("yt-dlp", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
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
		// Skip channel/playlist entries, only videos
		if e.IEKey == "YoutubeTab" || e.Type == "playlist" {
			continue
		}
		videos = append(videos, e.toVideo())
	}
	_ = cmd.Wait()
	return videos, scanner.Err()
}

func runAndParseChannels(args []string) ([]Channel, error) {
	cmd := exec.Command("yt-dlp", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp: %w", err)
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
	return channels, nil
}

// --- Bubbletea message types ---

type FetchResultMsg struct {
	Source string
	Videos []Video
	Err    error
}

type ChannelListMsg struct {
	Channels []Channel
	Err      error
}

type ChannelVideosMsg struct {
	ChannelID string
	Videos    []Video
	Err       error
}

type SearchResultMsg struct {
	Query  string
	Videos []Video
	Err    error
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
		return FetchResultMsg{Source: "recommended", Videos: videos, Err: err}
	}
}

func FetchSubscriptions(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		args := buildArgs(cfg, "https://www.youtube.com/feed/subscriptions", 100)
		videos, err := runAndParseVideos(args)
		return FetchResultMsg{Source: "subscriptions", Videos: videos, Err: err}
	}
}

func FetchSubscribedChannels(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		args := buildArgs(cfg, "https://www.youtube.com/feed/channels", 0)
		channels, err := runAndParseChannels(args)
		return ChannelListMsg{Channels: channels, Err: err}
	}
}

func FetchChannelVideos(cfg *config.Config, channelID string) tea.Cmd {
	return func() tea.Msg {
		url := "https://www.youtube.com/channel/" + channelID + "/videos"
		args := buildArgs(cfg, url, 50)
		videos, err := runAndParseVideos(args)
		return ChannelVideosMsg{ChannelID: channelID, Videos: videos, Err: err}
	}
}

func Search(cfg *config.Config, query string) tea.Cmd {
	return func() tea.Msg {
		url := "ytsearch25:" + query
		args := buildArgs(cfg, url, 25)
		videos, err := runAndParseVideos(args)
		return SearchResultMsg{Query: query, Videos: videos, Err: err}
	}
}
