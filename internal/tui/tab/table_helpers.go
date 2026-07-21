package tab

import (
	"strconv"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// ── transitional nav helpers (removed as tabs migrate to TableNav) ────────────

func handleGotoPrefix(active *bool, keys keymap.KeyMap, msg tea.KeyPressMsg) (consumed, doGotoTop bool) {
	if key.Matches(msg, keys.GotoPrefix) {
		if *active {
			*active = false
			return true, true
		}
		*active = true
		return true, false
	}
	*active = false
	return false, false
}

func checkGotoNum(buf *string, msg tea.KeyPressMsg) bool {
	if len(msg.Text) == 1 {
		if r := rune(msg.Text[0]); r >= '0' && r <= '9' {
			*buf += string(r)
			return true
		}
	}
	return false
}

func gotoRowIndex(numBuf string) int {
	if n, err := strconv.Atoi(numBuf); err == nil && n > 0 {
		return n - 1
	}
	return -1
}

func gotoLineView(numBuf string) string {
	if numBuf == "" {
		return ""
	}
	return styles.Bold.Render(":" + numBuf + "▌")
}

// HandleVideoAction dispatches the 8 universal pure-message video actions:
// Play, PlayAudio, Download, DownloadAudio, CopyURL, VideoInfo, AddList, HideChannel.
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
		return func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "video_detail", Video: v} }, true
	case key.Matches(msg, keys.AddList):
		return func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "add_to_playlist", Video: v} }, true
	case key.Matches(msg, keys.HideChannel):
		ch := domain.Channel{ID: v.ChannelID, Name: v.Channel}
		return func() tea.Msg { return tuipkg.HideChannelMsg{Channel: ch} }, true
	}
	return nil, false
}
