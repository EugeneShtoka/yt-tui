package youtube

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	tea "github.com/charmbracelet/bubbletea"
)

// YTClient holds browser-extracted cookies and can make YouTube innertube API calls.
type YTClient struct {
	cookieHeader string
	sapisid      string
}

// NewYTClient extracts cookies from the configured browser via yt-dlp and builds a client.
func NewYTClient(cfg *config.Config) (*YTClient, error) {
	if cfg.Browser == "" {
		return nil, fmt.Errorf("no browser configured")
	}

	f, err := os.CreateTemp("", "yt-tui-cookies-*.txt")
	if err != nil {
		return nil, err
	}
	cookiePath := f.Name()
	f.Close()
	// Remove the empty file so yt-dlp creates it itself — passing an empty file
	// to --cookies causes "does not look like a Netscape format cookies file".
	os.Remove(cookiePath)
	defer os.Remove(cookiePath)

	// Use the channels feed — same URL the app already uses for subscriptions,
	// so it's known to work. --flat-playlist --playlist-end 1 fetches minimal
	// data; the important side-effect is yt-dlp writing the cookie jar to file.
	cmd := exec.Command("yt-dlp",
		"--cookies-from-browser", cfg.Browser,
		"--cookies", cookiePath,
		"--flat-playlist",
		"--playlist-end", "1",
		"--quiet",
		"--no-warnings",
		"https://www.youtube.com/feed/channels",
	)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("cookie extraction: %s", msg)
	}

	cookieHeader, sapisid, err := parseCookieFile(cookiePath)
	if err != nil {
		return nil, err
	}
	if sapisid == "" {
		return nil, fmt.Errorf("SAPISID not found; ensure browser is logged in to YouTube")
	}

	return &YTClient{cookieHeader: cookieHeader, sapisid: sapisid}, nil
}

func parseCookieFile(path string) (cookieHeader, sapisid string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}

	seen := make(map[string]bool)
	var pairs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		// yt-dlp prefixes HttpOnly cookie lines with "#HttpOnly_" — strip it.
		line = strings.TrimPrefix(line, "#HttpOnly_")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		domain := fields[0]
		// Only accept cookies scoped to youtube.com, not google.com etc.
		if !strings.HasSuffix(domain, "youtube.com") {
			continue
		}
		name := fields[5]
		value := fields[6]
		// Deduplicate: first occurrence wins (most specific domain first in yt-dlp output).
		if seen[name] {
			continue
		}
		seen[name] = true
		pairs = append(pairs, name+"="+value)
		// Prefer __Secure-3PAPISID for HTTPS innertube requests; fall back to SAPISID.
		if name == "__Secure-3PAPISID" {
			sapisid = value
		}
		if name == "SAPISID" && sapisid == "" {
			sapisid = value
		}
	}
	var names []string
	for _, p := range pairs {
		if i := strings.IndexByte(p, '='); i >= 0 {
			names = append(names, p[:i])
		}
	}
	debug.Log("parseCookieFile: cookies=%d names=%v sapisid_len=%d", len(pairs), names, len(sapisid))
	return strings.Join(pairs, "; "), sapisid, nil
}

func (c *YTClient) sapisidhash() string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	h := sha1.New()
	h.Write([]byte(ts + " " + c.sapisid + " https://www.youtube.com"))
	hash := ts + "_" + hex.EncodeToString(h.Sum(nil))
	debug.Log("sapisidhash: ts=%s sapisid_prefix=%q", ts, c.sapisid[:min(6, len(c.sapisid))])
	return hash
}

func (c *YTClient) post(endpoint string, body map[string]any) ([]byte, error) {
	payload := map[string]any{
		"context": map[string]any{
			"client": map[string]any{
				"clientName":    "WEB",
				"clientVersion": "2.20231219.04.00",
				"hl":            "en",
			},
		},
	}
	for k, v := range body {
		payload[k] = v
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST",
		"https://www.youtube.com/youtubei/v1/"+endpoint,
		bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "SAPISIDHASH "+c.sapisidhash())
	req.Header.Set("X-Origin", "https://www.youtube.com")
	req.Header.Set("X-Goog-AuthUser", "0")
	req.Header.Set("Referer", "https://www.youtube.com/")
	req.Header.Set("Cookie", c.cookieHeader)
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		preview := string(respData)
		if len(preview) > 120 {
			preview = preview[:120]
		}
		return nil, fmt.Errorf("YouTube API %d: %s", resp.StatusCode, preview)
	}
	return respData, nil
}

func (c *YTClient) editPlaylist(playlistID string, action map[string]any) error {
	_, err := c.post("browse/edit_playlist", map[string]any{
		"playlistId": playlistID,
		"actions":    []map[string]any{action},
	})
	return err
}

func (c *YTClient) AddToWatchLater(videoID string) error {
	return c.editPlaylist("WL", map[string]any{
		"action":       "ACTION_ADD_VIDEO",
		"addedVideoId": videoID,
	})
}

func (c *YTClient) RemoveFromWatchLater(videoID string) error {
	return c.editPlaylist("WL", map[string]any{
		"action":         "ACTION_REMOVE_VIDEO_BY_VIDEO_ID",
		"removedVideoId": videoID,
	})
}

func (c *YTClient) AddToPlaylist(playlistID, videoID string) error {
	return c.editPlaylist(playlistID, map[string]any{
		"action":       "ACTION_ADD_VIDEO",
		"addedVideoId": videoID,
	})
}

func (c *YTClient) RemoveFromPlaylist(playlistID, videoID string) error {
	return c.editPlaylist(playlistID, map[string]any{
		"action":         "ACTION_REMOVE_VIDEO_BY_VIDEO_ID",
		"removedVideoId": videoID,
	})
}

func (c *YTClient) CreatePlaylist(title string) (string, error) {
	resp, err := c.post("playlist/create", map[string]any{
		"title":         title,
		"privacyStatus": "PRIVATE",
	})
	if err != nil {
		return "", err
	}
	var result struct {
		PlaylistID string `json:"playlistId"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", err
	}
	if result.PlaylistID == "" {
		return "", fmt.Errorf("empty playlist ID in response")
	}
	return result.PlaylistID, nil
}

func (c *YTClient) DeletePlaylist(playlistID string) error {
	_, err := c.post("playlist/delete", map[string]any{
		"playlistId": playlistID,
	})
	return err
}

func (c *YTClient) Subscribe(channelID string) error {
	data, err := c.post("subscription/subscribe", map[string]any{
		"channelIds": []string{channelID},
	})
	if err != nil {
		return err
	}
	debug.Log("subscribe response: %s", string(data))
	var result struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		ResponseContext *struct {
			MainAppWebResponseContext *struct {
				LoggedOut bool `json:"loggedOut"`
			} `json:"mainAppWebResponseContext"`
		} `json:"responseContext"`
	}
	if json.Unmarshal(data, &result) == nil {
		if result.Error != nil {
			return fmt.Errorf("%d: %s", result.Error.Code, result.Error.Message)
		}
		if result.ResponseContext != nil &&
			result.ResponseContext.MainAppWebResponseContext != nil &&
			result.ResponseContext.MainAppWebResponseContext.LoggedOut {
			return fmt.Errorf("not logged in — session may have expired")
		}
	}
	return nil
}

func (c *YTClient) Unsubscribe(channelID string) error {
	_, err := c.post("subscription/unsubscribe", map[string]any{
		"externalChannelId": channelID,
	})
	return err
}

// --- Tea commands for operations that return results ---

type SubscribeMsg struct {
	ChannelID   string
	ChannelName string
	Err         error
}

type UnsubscribeMsg struct {
	ChannelID   string
	ChannelName string
	Err         error
}

type CreatePlaylistMsg struct {
	Name string
	ID   string
	Err  error
}

func SubscribeToChannel(client *YTClient, channelID, channelName string) tea.Cmd {
	return func() tea.Msg {
		err := client.Subscribe(channelID)
		return SubscribeMsg{ChannelID: channelID, ChannelName: channelName, Err: err}
	}
}

func UnsubscribeFromChannel(client *YTClient, channelID, channelName string) tea.Cmd {
	return func() tea.Msg {
		err := client.Unsubscribe(channelID)
		return UnsubscribeMsg{ChannelID: channelID, ChannelName: channelName, Err: err}
	}
}

func CreateYTPlaylist(client *YTClient, name string) tea.Cmd {
	return func() tea.Msg {
		id, err := client.CreatePlaylist(name)
		return CreatePlaylistMsg{Name: name, ID: id, Err: err}
	}
}

type RemoveYTPlaylistVideoMsg struct {
	Err error
}

func RemoveYTPlaylistVideo(client *YTClient, playlistID, videoID string) tea.Cmd {
	return func() tea.Msg {
		return RemoveYTPlaylistVideoMsg{Err: client.RemoveFromPlaylist(playlistID, videoID)}
	}
}
