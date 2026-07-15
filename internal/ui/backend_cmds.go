package ui

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

// ── Msg types (moved from youtube package) ────────────────────────────────────

type fetchRecommendedMsg struct {
	videos []domain.Video
	err    error
}

type fetchChannelListMsg struct {
	channels   []domain.Channel
	err        error
	background bool
}

type fetchChannelVideosMsg struct {
	source    string // "search", "subscriptions", or "ch-background"
	channelID string
	videos    []domain.Video
	err       error
}

type fetchSearchResultMsg struct {
	query    string
	channels []domain.Channel
	videos   []domain.Video
	err      error
}

type fetchYTPlaylistsMsg struct {
	playlists  []domain.YTPlaylist
	err        error
	background bool
}

type fetchPlaylistVideosMsg struct {
	playlistID string
	videos     []domain.Video
	err        error
}

type fetchVideoDetailsMsg struct {
	details domain.VideoDetails
	err     error
}

// ── tea.Cmd adapters ──────────────────────────────────────────────────────────

func cmdFetchRecommended(b api.Backend) tea.Cmd {
	return func() tea.Msg {
		v, err := b.Recommended(context.Background())
		return fetchRecommendedMsg{videos: v, err: err}
	}
}

func cmdFetchSubscribedChannels(b api.Backend) tea.Cmd {
	return func() tea.Msg {
		ch, err := b.SubscribedChannels(context.Background())
		return fetchChannelListMsg{channels: ch, err: err, background: false}
	}
}

func cmdFetchSubscribedChannelsBackground(b api.Backend) tea.Cmd {
	return func() tea.Msg {
		ch, err := b.SubscribedChannels(context.Background())
		return fetchChannelListMsg{channels: ch, err: err, background: true}
	}
}

func cmdFetchChannelVideos(b api.Backend, channelURL, channelID, source string) tea.Cmd {
	return func() tea.Msg {
		vids, err := b.ChannelVideos(context.Background(), channelURL, channelID)
		return fetchChannelVideosMsg{source: source, channelID: channelID, videos: vids, err: err}
	}
}

func cmdFetchChannelLatestN(b api.Backend, channelURL, channelID string, n int) tea.Cmd {
	return func() tea.Msg {
		vids, err := b.ChannelLatestN(context.Background(), channelURL, channelID, n)
		return fetchChannelVideosMsg{source: "ch-background", channelID: channelID, videos: vids, err: err}
	}
}

func cmdSearch(b api.Backend, query string) tea.Cmd {
	return func() tea.Msg {
		channels, videos, err := b.Search(context.Background(), query)
		return fetchSearchResultMsg{query: query, channels: channels, videos: videos, err: err}
	}
}

func cmdFetchYTPlaylists(b api.Backend) tea.Cmd {
	return func() tea.Msg {
		pls, err := b.YTPlaylists(context.Background())
		return fetchYTPlaylistsMsg{playlists: pls, err: err, background: false}
	}
}

func cmdFetchYTPlaylistsBackground(b api.Backend) tea.Cmd {
	return func() tea.Msg {
		pls, err := b.YTPlaylists(context.Background())
		return fetchYTPlaylistsMsg{playlists: pls, err: err, background: true}
	}
}

func cmdFetchPlaylistVideos(b api.Backend, playlistID string) tea.Cmd {
	return func() tea.Msg {
		vids, err := b.YTPlaylistVideos(context.Background(), playlistID)
		return fetchPlaylistVideosMsg{playlistID: playlistID, videos: vids, err: err}
	}
}

func cmdFetchVideoDetails(b api.Backend, videoURL string) tea.Cmd {
	return func() tea.Msg {
		details, err := b.VideoDetails(context.Background(), videoURL)
		return fetchVideoDetailsMsg{details: details, err: err}
	}
}
