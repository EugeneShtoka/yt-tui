package tui

import (
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// TabID is a typed identifier for each tab, used in navigation messages.
type TabID int

const (
	TabRecommended   TabID = iota
	TabSubscriptions
	TabChannels
	TabPlaylists
	TabSearch
	TabDownloading
	TabLocal
	TabHistory
	TabActivity
)

// ContextID identifies the UI context for key dispatch and sort filtering.
type ContextID int

const (
	CtxVideoList     ContextID = iota // rec, subs, channel drill-down, playlist vids
	CtxChannelList                    // subscriptions channel pane
	CtxTagList                        // channels tab: tag list
	CtxSearchVideo                    // search: video rows
	CtxSearchChannel                  // search: channel rows
	CtxPlaylistList                   // playlists top level
	CtxLocal                          // local tab
	CtxDownloading                    // downloading tab
	CtxHistoryVideo                   // history: video entry
	CtxHistorySearch                  // history: search entry
)

// Tab is a full-screen content area with tab-bar identity and keyboard metadata.
// Tabs are value types; Update returns the mutated copy.
type Tab interface {
	tea.Model
	ID() TabID
	Title() string
	ShortHelp() []key.Binding
	Context() ContextID
}

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

// CopyURLMsg requests Root to write a URL to the system clipboard.
type CopyURLMsg struct{ URL string }

// OpenOverlayMsg requests Root to open a named overlay over the current tab.
type OpenOverlayMsg struct {
	Kind    string // "video_detail", "links", "chapters", "add_to_playlist"
	VideoID string
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
