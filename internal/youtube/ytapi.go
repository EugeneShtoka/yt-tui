package youtube

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/debug"
)

// This file is the domain API surface of the YouTube innertube client: the
// playlist and subscription verbs. They are built on the auth/transport core
// (YTClient.post) in ytauth.go (M-3). These methods are called from the backend
// service layer; the TUI reaches them through the backend interface, so this
// package has no Bubble Tea dependency.

func (c *YTClient) editPlaylist(ctx context.Context, playlistID string, action map[string]any) error {
	_, err := c.post(ctx, "browse/edit_playlist", map[string]any{
		"playlistId": playlistID,
		"actions":    []map[string]any{action},
	})
	if err != nil {
		return fmt.Errorf("editPlaylist %s: %w", playlistID, err)
	}
	return nil
}

func (c *YTClient) AddToWatchLater(ctx context.Context, videoID string) error {
	return c.editPlaylist(ctx, "WL", map[string]any{
		"action":       "ACTION_ADD_VIDEO",
		"addedVideoId": videoID,
	})
}

func (c *YTClient) RemoveFromWatchLater(ctx context.Context, videoID string) error {
	return c.editPlaylist(ctx, "WL", map[string]any{
		"action":         "ACTION_REMOVE_VIDEO_BY_VIDEO_ID",
		"removedVideoId": videoID,
	})
}

func (c *YTClient) AddToPlaylist(ctx context.Context, playlistID, videoID string) error {
	return c.editPlaylist(ctx, playlistID, map[string]any{
		"action":       "ACTION_ADD_VIDEO",
		"addedVideoId": videoID,
	})
}

func (c *YTClient) RemoveFromPlaylist(ctx context.Context, playlistID, videoID string) error {
	return c.editPlaylist(ctx, playlistID, map[string]any{
		"action":         "ACTION_REMOVE_VIDEO_BY_VIDEO_ID",
		"removedVideoId": videoID,
	})
}

func (c *YTClient) CreatePlaylist(ctx context.Context, title string) (string, error) {
	resp, err := c.post(ctx, "playlist/create", map[string]any{
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
		return "", fmt.Errorf("CreatePlaylist unmarshal: %w", err)
	}
	if result.PlaylistID == "" {
		return "", fmt.Errorf("empty playlist ID in response")
	}
	return result.PlaylistID, nil
}

func (c *YTClient) DeletePlaylist(ctx context.Context, playlistID string) error {
	_, err := c.post(ctx, "playlist/delete", map[string]any{
		"playlistId": playlistID,
	})
	if err != nil {
		return fmt.Errorf("DeletePlaylist %s: %w", playlistID, err)
	}
	return nil
}

func (c *YTClient) Subscribe(ctx context.Context, channelID string) error {
	data, err := c.post(ctx, "subscription/subscribe", map[string]any{
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

func (c *YTClient) Unsubscribe(ctx context.Context, channelID string) error {
	_, err := c.post(ctx, "subscription/unsubscribe", map[string]any{
		"externalChannelId": channelID,
	})
	if err != nil {
		return fmt.Errorf("Unsubscribe %s: %w", channelID, err)
	}
	return nil
}
