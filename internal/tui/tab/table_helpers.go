package tab

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

// HandleVideoAction dispatches the 10 universal pure-message video actions:
// Play, PlayAudio, Download, DownloadAudio, CopyURL, VideoInfo, OpenLinks, OpenChapters, AddList, HideChannel.
// Returns (cmd, true) if handled; (nil, false) if the key did not match.
// Tabs call this after navigation; unmatched keys fall through to tab-specific handling.
func HandleVideoAction(msg tea.KeyPressMsg, v domain.Video, keys keymap.KeyMap) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, keys.Play):
		return func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v} }, true
	case key.Matches(msg, keys.PlayAudio):
		return func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v, AudioOnly: true} }, true
	case key.Matches(msg, keys.Download):
		return func() tea.Msg { return tuipkg.EnqueueMsg{Video: v} }, true
	case key.Matches(msg, keys.DownloadAudio):
		return func() tea.Msg { return tuipkg.EnqueueMsg{Video: v, AudioOnly: true} }, true
	case key.Matches(msg, keys.CopyURL):
		return func() tea.Msg { return tuipkg.CopyURLMsg{URL: v.URL} }, true
	case key.Matches(msg, keys.VideoInfo):
		return func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: tuipkg.OverlayVideoDetail, Video: v} }, true
	case key.Matches(msg, keys.OpenLinks):
		return func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: tuipkg.OverlayVideoDetailLinks, Video: v} }, true
	case key.Matches(msg, keys.OpenChapters):
		return func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: tuipkg.OverlayVideoDetailChapters, Video: v} }, true
	case key.Matches(msg, keys.OpenTranscript):
		return func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: tuipkg.OverlayVideoDetailTranscript, Video: v} }, true
	case key.Matches(msg, keys.AddList):
		return func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: tuipkg.OverlayAddToPlaylist, Video: v} }, true
	case key.Matches(msg, keys.HideChannel):
		ch := domain.Channel{ID: v.ChannelID, Name: v.Channel}
		return func() tea.Msg { return tuipkg.HideChannelMsg{Channel: ch} }, true
	}
	return nil, false
}

// videoActionAt returns the universal video-action cmd for row idx of videos, or
// ok=false when idx is out of range or msg isn't a video action. Consolidates
// the "highlighted row → HandleVideoAction" fallback the video panes repeat.
func videoActionAt(videos []domain.Video, idx int, msg tea.KeyPressMsg, keys keymap.KeyMap) (tea.Cmd, bool) {
	if idx < 0 || idx >= len(videos) {
		return nil, false
	}
	return HandleVideoAction(msg, videos[idx], keys)
}

// handleDrillBackKey consolidates the numBuf-guard-then-exit pattern shared
// by every drill-down video pane (Channels, Tags, Playlists, Search, History):
// Up/Down/PageUp/PageDown/goto-line are handled by nav itself; Left/Escape
// clears a pending goto-line buffer on its first press instead of leaving it
// stuck until an unrelated nav key clears it as a side effect (M-18 — only
// Search did this before), and only backs out of the pane once the buffer is
// already empty. Returns handled=true if the key was consumed; back=true
// means the caller should pop out of the drill pane.
func handleDrillBackKey(nav *videotable.TableNav, msg tea.KeyPressMsg, keys keymap.KeyMap, n int) (handled, back bool) {
	numBufBefore := nav.NumBufView() != ""
	if nav.HandleNav(msg, keys, n) {
		return true, false
	}
	if key.Matches(msg, keys.Left) || key.Matches(msg, keys.Escape) {
		if numBufBefore {
			nav.ClearNumBuf()
			return true, false
		}
		return true, true
	}
	return false, false
}

// drillSubHeader renders the "← <name>" subheader shared by every drill-down
// pane, truncated to width and with an optional dim status suffix (e.g. a
// refreshing spinner). Consolidates 5 near-identical one-off builds (M-18).
func drillSubHeader(name string, width int, suffix string) string {
	text := "← " + render.Truncate(name, width-4)
	if suffix != "" {
		text += "  " + styles.Dim.Render(suffix)
	}
	return styles.SectionTitle.Render(text)
}
