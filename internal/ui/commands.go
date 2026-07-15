package ui

import (
	"os"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	tea "github.com/charmbracelet/bubbletea"
)

// persistErrMsg surfaces a background persistence failure in the status bar.
// The fire-and-forget Cmds below return it on failure and nil on success; a nil
// tea.Msg is dropped by the runtime, so the success path produces no message.
type persistErrMsg struct{ err error }

// The Cmds below replace raw `go func(){ _ = m.db.… }()` launches inside Update.
// Running the persist inside a tea.Cmd keeps Update pure (the runtime schedules
// the side effect), avoids racing on the value-copied Model, and surfaces errors.

func saveYTPlaylistsCmd(db Store, pls []domain.YTPlaylist) tea.Cmd {
	return func() tea.Msg {
		if err := db.SaveYTPlaylists(pls); err != nil {
			return persistErrMsg{err}
		}
		return nil
	}
}

func saveYTPlaylistVideosCmd(db Store, playlistID string, vids []domain.Video) tea.Cmd {
	return func() tea.Msg {
		if err := db.SaveYTPlaylistVideos(playlistID, vids); err != nil {
			return persistErrMsg{err}
		}
		return nil
	}
}

func saveChannelVideosCmd(db Store, chID string, vids []domain.Video) tea.Cmd {
	return func() tea.Msg {
		if err := db.SaveChannelVideos(chID, vids); err != nil {
			return persistErrMsg{err}
		}
		return nil
	}
}

func deleteChannelVideosCmd(db Store, chID string) tea.Cmd {
	return func() tea.Msg {
		if err := db.DeleteChannelVideos(chID); err != nil {
			return persistErrMsg{err}
		}
		return nil
	}
}

func saveFeedCacheCmd(db Store, feed string, vids []domain.Video) tea.Cmd {
	return func() tea.Msg {
		if err := db.SaveFeedCache(feed, vids); err != nil {
			return persistErrMsg{err}
		}
		return nil
	}
}

// saveSubsAndFeedCmd persists the subscribed-channel list and the recommended
// feed cache in one Cmd, preserving the original ordering (channels then feed).
func saveSubsAndFeedCmd(db Store, channels []domain.Channel, videos []domain.Video) tea.Cmd {
	return func() tea.Msg {
		if err := db.SaveSubscribedChannels(channels); err != nil {
			return persistErrMsg{err}
		}
		if err := db.SaveFeedCache("recommended", videos); err != nil {
			return persistErrMsg{err}
		}
		return nil
	}
}

// deleteFilesCmd removes downloaded files off the UI goroutine. Per-file errors
// are ignored (as the original inline loop did) — a missing file is not worth a
// status-bar warning.
func deleteFilesCmd(paths []string) tea.Cmd {
	return func() tea.Msg {
		for _, p := range paths {
			_ = os.Remove(p)
		}
		return nil
	}
}

// addToPlaylistCmd / deletePlaylistCmd take the *YTClient as a param so the Cmd
// closes over the (shareable) client rather than the value-copied Model.
func addToPlaylistCmd(client *youtube.YTClient, playlistID, videoID string) tea.Cmd {
	return func() tea.Msg {
		if err := client.AddToPlaylist(playlistID, videoID); err != nil {
			return persistErrMsg{err}
		}
		return nil
	}
}

func deletePlaylistCmd(client *youtube.YTClient, playlistID string) tea.Cmd {
	return func() tea.Msg {
		if err := client.DeletePlaylist(playlistID); err != nil {
			return persistErrMsg{err}
		}
		return nil
	}
}
