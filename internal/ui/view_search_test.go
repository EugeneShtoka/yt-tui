package ui

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	tea "github.com/charmbracelet/bubbletea"
)

func sampleChannels() []youtube.Channel {
	return []youtube.Channel{
		{ID: "c1", Name: "Chan One"},
		{ID: "c2", Name: "Chan Two"},
	}
}

func searchCtx(chSel *youtube.Channel, channels []youtube.Channel, videos, chVideos []youtube.Video) viewCtx {
	return viewCtx{
		keys:           testListKeys(),
		pageSize:       10,
		circular:       false,
		searchChSel:    chSel,
		searchChannels: channels,
		searchVideos:   videos,
		searchChVideos: chVideos,
	}
}

func TestSearchResultsDownMovesCursor(t *testing.T) {
	v := searchView{}
	// Every key forwards a searchActionIntent (the router runs the mode-specific
	// action after nav), so a non-nil intent is expected; the cursor move matters.
	in := v.update(tea.KeyMsg{Type: tea.KeyDown}, searchCtx(nil, sampleChannels(), sampleVideos(), nil))
	if in == nil {
		t.Fatal("update should always forward a searchActionIntent")
	}
	if v.cursor != 1 {
		t.Errorf("Down: cursor=%d, want 1", v.cursor)
	}
	// vs stays 0 while the cursor is still within the channel section.
	if v.vs != 0 {
		t.Errorf("Down over channels: vs=%d, want 0", v.vs)
	}
}

func TestSearchResultsContextSwitchesChannelToVideo(t *testing.T) {
	ctx := searchCtx(nil, sampleChannels(), sampleVideos(), nil)
	// Cursor on a channel row.
	if got := (searchView{cursor: 0}).context(ctx); got != CtxSearchChannel {
		t.Errorf("cursor 0: context=%v, want CtxSearchChannel", got)
	}
	// Cursor past the two channels lands on a video row.
	if got := (searchView{cursor: 2}).context(ctx); got != CtxSearchVideo {
		t.Errorf("cursor 2: context=%v, want CtxSearchVideo", got)
	}
}

func TestSearchDrillDownNavMovesVidCursor(t *testing.T) {
	sel := sampleChannels()[0]
	v := searchView{cursor: 1} // results cursor must be untouched in drill mode
	v.update(tea.KeyMsg{Type: tea.KeyDown}, searchCtx(&sel, sampleChannels(), sampleVideos(), sampleVideos()))
	if v.vidCursor != 1 {
		t.Errorf("drill Down: vidCursor=%d, want 1", v.vidCursor)
	}
	if v.cursor != 1 {
		t.Errorf("drill mode moved results cursor to %d, want 1 (unchanged)", v.cursor)
	}
}

func TestSearchDrillDownContextIsVideoList(t *testing.T) {
	sel := sampleChannels()[0]
	ctx := searchCtx(&sel, sampleChannels(), sampleVideos(), sampleVideos())
	if got := (searchView{cursor: 0}).context(ctx); got != CtxVideoList {
		t.Errorf("drill context=%v, want CtxVideoList", got)
	}
}

func TestSearchActionIntentCarriesKey(t *testing.T) {
	v := searchView{}
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}
	in := v.update(msg, searchCtx(nil, sampleChannels(), sampleVideos(), nil))
	got, ok := in.(searchActionIntent)
	if !ok {
		t.Fatalf("want searchActionIntent, got %T", in)
	}
	if got.msg.String() != "p" {
		t.Errorf("intent carried msg=%q, want p", got.msg.String())
	}
	if v.cursor != 0 {
		t.Errorf("non-nav key moved cursor to %d", v.cursor)
	}
}

func TestSearchJumpToLast(t *testing.T) {
	v := searchView{}
	nCh, nVid := len(sampleChannels()), len(sampleVideos())
	v.jumpToLast(nCh, nVid, 10)
	if want := nCh + nVid - 1; v.cursor != want {
		t.Errorf("jumpToLast: cursor=%d, want %d", v.cursor, want)
	}
}
