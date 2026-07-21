package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// TabID is a typed identifier for each tab, used in navigation messages.
type TabID int

const (
	TabRecommended TabID = iota
	TabSubscriptions
	TabChannels
	TabTags
	TabPlaylists
	TabSearch
	TabDownloading
	TabLocal
	TabHistory
	TabActivity
)

// Tab is a full-screen content area with tab-bar identity and keyboard metadata.
// Tabs are value types; Update returns the mutated copy.
type Tab interface {
	tea.Model
	ID() TabID
	Title() string
	ShortHelp() []key.Binding
	// InterceptsInput returns true when the tab has a text input focused and
	// Root should bypass global key bindings (quit, tab-switch, etc.).
	InterceptsInput() bool
}

// OverlayKind identifies which overlay to open.
type OverlayKind int

const (
	OverlayVideoDetail OverlayKind = iota
	OverlayAddToPlaylist
)

// ── Cross-root messages ───────────────────────────────────────────────────────
// Tabs emit these as tea.Cmd results; Root handles them.

// ContentSizeMsg tells a tab how much content area it has after the chrome is reserved.
type ContentSizeMsg struct{ Width, Height int }

// PlayVideoMsg requests Root to start playback.
type PlayVideoMsg struct {
	Video     domain.Video
	AudioOnly bool
}

// NavigateMsg requests Root to switch to a tab, optionally pre-seeding state.
type NavigateMsg struct {
	Tab   TabID
	Query string // pre-filled search query when Tab == TabSearch
}

// HideChannelMsg requests Root to hide a channel from recommendations.
type HideChannelMsg struct{ Channel domain.Channel }

// StatusMsg updates the status bar with a transient message.
type StatusMsg struct {
	Text  string
	IsErr bool
}

// LaunchLocalVideoMsg requests Root to play a downloaded local file.
type LaunchLocalVideoMsg struct{ Video domain.LocalVideo }

// EnqueueMsg requests Root to add a video to the download queue.
type EnqueueMsg struct {
	Video     domain.Video
	AudioOnly bool
}

// CopyURLMsg requests Root to write a URL to the system clipboard.
type CopyURLMsg struct{ URL string }

// OpenOverlayMsg requests Root to open a named overlay over the current tab.
type OpenOverlayMsg struct {
	Kind  OverlayKind  // OverlayVideoDetail | OverlayAddToPlaylist
	Video domain.Video // the video the overlay concerns
}

// NavigateToChannelMsg requests Root to open the Channels tab scrolled to a channel.
type NavigateToChannelMsg struct {
	ChannelID   string
	ChannelName string
}

// NavigateToPlaylistMsg requests Root to open the Playlists tab scrolled to a playlist.
type NavigateToPlaylistMsg struct {
	PlaylistID      string // YT playlist ID (empty for local)
	PlaylistLocalID int64  // local playlist DB ID (0 for YT)
	PlaylistName    string
}

// UnsubscribeMsg requests Root to unsubscribe from a channel via the backend.
// The emitting tab has already removed the channel from its local feed.
type UnsubscribeMsg struct{ Channel domain.Channel }

// SearchActivateMsg tells the Search tab to prefill its query and execute a search.
// Root dispatches this when NavigateMsg.Query is non-empty.
type SearchActivateMsg struct{ Query string }

// SearchFocusInputMsg tells the Search tab to focus its text input.
// Root dispatches this when navigating to Search with no pre-filled query.
type SearchFocusInputMsg struct{}

// HistoryChangedMsg tells the History tab to reload its entries.
// Emitted after a search or any event that adds a history record.
type HistoryChangedMsg struct{}

// EnqueueSucceededMsg is an internal root→root message produced after a successful
// backend.Enqueue call, carrying enough info to build the status text and notify
// the Downloading tab.
type EnqueueSucceededMsg struct {
	Title     string
	AudioOnly bool
}

// DownloadItemsChangedMsg tells the Downloading tab to refresh its queue snapshot.
// Root dispatches this after a successful Enqueue call.
type DownloadItemsChangedMsg struct{}

// RefreshPositionsMsg tells all tabs to reload playback positions and watched
// status from the DB. Root dispatches this when the player exits.
type RefreshPositionsMsg struct{}
