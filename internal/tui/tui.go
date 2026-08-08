// Package tui defines the shared vocabulary of the terminal UI: the Tab
// interface every tab implements, the typed TabID identifiers, and the
// cross-tab messages (navigation, overlays, clipboard, poll ticks) that flow
// through the Bubble Tea event loop. Concrete tabs live in subpackages.
package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// TabID is a typed identifier for each tab, used in navigation messages.
type TabID int

const (
	TabFeed TabID = iota
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
	// SelectedVideo returns the video under the cursor, or false if none.
	SelectedVideo() (domain.Video, bool)
	// Loading reports whether the tab has a fetch in flight that needs the
	// shared spinner ticking. Root stops the tick loop once no tab or overlay
	// reports Loading, and restarts it when one does.
	Loading() bool
}

// OverlayKind identifies which overlay to open.
type OverlayKind int

const (
	OverlayVideoDetail OverlayKind = iota
	OverlayVideoDetailLinks
	OverlayVideoDetailChapters
	OverlayVideoDetailTranscript
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

// NavigateToPanelMsg requests Root to switch to the panel with the given name
// (the `:tab <name>` command). Unknown names are reported to the status bar.
// Distinct from NavigateMsg (which targets a tab type) so custom panels are
// reachable by their configured name, not just by type.
type NavigateToPanelMsg struct{ Name string }

// HideChannelMsg requests Root to hide a channel from recommendations.
type HideChannelMsg struct{ Channel domain.Channel }

// WatchLaterMsg requests Root to add a video to Watch Later. The backend decides
// the store (YouTube's "WL" playlist when authed, else a local "Watch Later"
// playlist), so the TUI stays out of YouTube communication.
type WatchLaterMsg struct{ Video domain.Video }

// StatusMsg updates the status bar with a transient message.
type StatusMsg struct {
	Text  string
	IsErr bool
}

// StatusExpireMsg clears the status bar's text if Gen still matches the most
// recent StatusMsg's generation, so an older expiry timer can't clobber a
// status set after it (M-14 — Render() used to derive expiry from a bare
// wall-clock read with no scheduled repaint to force it).
type StatusExpireMsg struct{ Gen int }

// LaunchLocalVideoMsg requests Root to play a downloaded local file.
type LaunchLocalVideoMsg struct{ Video domain.LocalVideo }

// EnqueueMsg requests Root to add a video to the download queue.
type EnqueueMsg struct {
	Video     domain.Video
	AudioOnly bool
}

// CopyURLMsg requests Root to write a URL to the system clipboard.
type CopyURLMsg struct{ URL string }

// CopyTextMsg requests Root to write arbitrary text to the system clipboard.
// Label names the copied content for the status message (e.g. "transcript").
type CopyTextMsg struct {
	Text  string
	Label string
}

// OpenOverlayMsg requests Root to open a named overlay over the current tab.
type OpenOverlayMsg struct {
	Kind  OverlayKind  // OverlayVideoDetail | OverlayAddToPlaylist
	Video domain.Video // the video the overlay concerns
}

// RunCommandMsg carries the raw text typed into the command palette. Root pops
// the palette, resolves the first word against the command registry (view-local
// commands shadow global ones), and dispatches the matched command with the
// remaining words as args; an unknown command surfaces a status-bar error.
type RunCommandMsg struct{ Input string }

// OpenCommandHelpMsg requests Root to open the command-listing overlay (`:help`).
type OpenCommandHelpMsg struct{}

// OpenConfirmMsg requests Root to open a yes/no confirmation overlay. OnConfirm
// is the message dispatched when the user confirms; canceling emits nothing.
// It gates irreversible palette actions (e.g. delete-all-local) behind an
// explicit yes.
type OpenConfirmMsg struct {
	Prompt    string
	OnConfirm tea.Msg
}

// ClearDownloadsMsg requests Root to dismiss the whole download queue
// (backend.ClearDownloads) — the `:clear-downloads` command. Queue-only: it
// touches neither files, the DB, nor history.
type ClearDownloadsMsg struct{}

// DeleteAllLocalFilesMsg requests Root to delete every downloaded file
// (backend.DeleteAllLocalFiles) — the confirmed `:delete-all-local` action.
type DeleteAllLocalFilesMsg struct{}

// LocalVideosChangedMsg tells the Local tab to reload its library snapshot.
// Root dispatches it after a bulk local-file mutation (e.g. delete-all-local).
type LocalVideosChangedMsg struct{}

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

// UnsubscribeResultMsg reports the outcome of an UnsubscribeMsg's backend call.
// It is broadcast to every tab so those that optimistically removed the
// channel (Channels, Feed) can restore it if Err != nil.
type UnsubscribeResultMsg struct {
	Channel domain.Channel
	Err     error
}

// BlockChannelMsg requests Root to block or unblock a channel via the backend.
// The emitting tab has already applied the transition optimistically to its
// local set. Block==true blocks (guarded unsubscribe + set blocked); Block==false
// unblocks. Channel carries the pre-transition value so a failed call can revert.
type BlockChannelMsg struct {
	Channel domain.Channel
	Block   bool
}

// BlockChannelResultMsg reports the outcome of a BlockChannelMsg's backend call.
// It is broadcast to every tab so those that optimistically applied the
// transition (Channels) can restore the original channel if Err != nil.
type BlockChannelResultMsg struct {
	Channel domain.Channel
	Block   bool
	Err     error
}

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

// PollTickMsg is delivered by Root to the active tab on a fixed interval so
// DB-backed views (channel drill-in, feed, tags, channel list) reload while a
// background crawl or enrichment pass streams new rows into the DB. Only the
// active tab receives it, so the cost is one lightweight reload per interval.
type PollTickMsg struct{}

// VideoSelectedMsg is emitted by Root (debounced) to overlays when the tab cursor moves.
type VideoSelectedMsg struct{ Video domain.Video }

// SpinnerFrameMsg carries the current shared-spinner frame from Root to the
// active tab (and top overlay) each animation tick. Root owns a single spinner
// and its tick loop; tabs render the supplied frame instead of each driving
// their own loop and broadcasting ticks to every tab.
type SpinnerFrameMsg struct{ Frame string }

// TabAddressedMsg is implemented by tab-private messages (background-load
// results) that must be delivered only to their owning tab rather than
// broadcast to every tab. Root routes such messages by matching TargetTab()
// against each tab's ID().
type TabAddressedMsg interface {
	TargetTab() TabID
}

// TabTarget is embedded in a tab's private message types to satisfy
// TabAddressedMsg. Construct the message with Tab set to the owning tab's ID,
// e.g. feedSubLoadedMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabFeed}}.
type TabTarget struct{ Tab TabID }

// TargetTab reports the tab a message is addressed to.
func (t TabTarget) TargetTab() TabID { return t.Tab }

// OverlayAddressedMsg is the overlay analog of TabAddressedMsg: it is
// implemented by overlay-private messages (background fetch results) that must
// be delivered to the overlay instance that spawned them — wherever it sits in
// the stack — not to whatever overlay happens to be on top. Root routes such a
// message by matching TargetOverlay() against each overlay's ID().
//
// This decouples the two concerns Root used to conflate: async result delivery
// (by originator identity, handled here) and input routing (by stack position,
// still top-only). Without it, stacking any overlay over another silently
// stole the lower overlay's in-flight fetch results.
type OverlayAddressedMsg interface {
	TargetOverlay() int64
}

// OverlayTarget is embedded in an overlay's private message types to satisfy
// OverlayAddressedMsg. Construct the message with ID set to the owning overlay's
// ID(), e.g. vdCacheMsg{OverlayTarget: tuipkg.OverlayTarget{ID: vd.ID()}}.
type OverlayTarget struct{ ID int64 }

// TargetOverlay reports the overlay a message is addressed to.
func (o OverlayTarget) TargetOverlay() int64 { return o.ID }
