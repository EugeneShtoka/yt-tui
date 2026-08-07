package overlay

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
)

// TestFetchCtxCanceledOnVideoSwitch verifies that selecting a different video
// cancels the previous video's fetch context — so a superseded yt-dlp call is
// actually killed, not merely ignored via fetchToken (H-1 tail).
func TestFetchCtxCanceledOnVideoSwitch(t *testing.T) {
	b := &fakeDetailBackend{cacheOK: false}
	keys := keymap.Build(config.KeyBindings{Close: "esc"})
	v1 := domain.Video{ID: "v1", URL: "https://youtu.be/v1", Title: "one"}

	vd, _ := NewVideoDetail(context.Background(), b, b, keys, v1, VideoDetailOpts{InitialView: InitialViewPanel, TranscriptWidth: "50%"})
	first := vd.fetchCtx
	if first == nil || first.Err() != nil {
		t.Fatal("NewVideoDetail should install a live fetch context")
	}

	got, _ := vd.Update(tuipkg.VideoSelectedMsg{Video: domain.Video{ID: "v2", URL: "https://youtu.be/v2", Title: "two"}})
	vd2 := got.(VideoDetail)

	if first.Err() == nil {
		t.Error("selecting a new video should cancel the previous fetch context")
	}
	if vd2.fetchCtx == nil || vd2.fetchCtx.Err() != nil {
		t.Error("the new video's fetch context should be live")
	}
}

// TestVideoClearCancelsInFlightFetch verifies a tab-switch clear invalidates the
// in-flight fetch (bumps the token and cancels the ctx), so a late result for the
// previous tab's video can't land on the panel after the switch.
func TestVideoClearCancelsInFlightFetch(t *testing.T) {
	b := &fakeDetailBackend{cacheOK: false}
	keys := keymap.Build(config.KeyBindings{Close: "esc"})
	v := domain.Video{ID: "v1", URL: "https://youtu.be/v1", Title: "one"}

	vd, _ := NewVideoDetail(context.Background(), b, b, keys, v, VideoDetailOpts{InitialView: InitialViewPanel, TranscriptWidth: "50%"})
	first := vd.fetchCtx
	tok := vd.fetchToken

	cleared, _ := vd.Update(VideoClearMsg{})
	vd2 := cleared.(VideoDetail)

	if first.Err() == nil {
		t.Error("clear should cancel the in-flight fetch context")
	}
	if vd2.fetchToken == tok {
		t.Error("clear should bump fetchToken to stale-out the in-flight fetch")
	}
	if vd2.fetchCtx == nil || vd2.fetchCtx.Err() != nil {
		t.Error("clear should install a fresh live fetch context")
	}
}

// TestVideoClearThenReselectSameVideoRefetches guards the "switch away and back"
// case: after a clear, reselecting the same video (the one the panel was showing)
// must re-fetch instead of being swallowed by the fetchVideo no-op guard, which
// used to leave the panel blank on return.
func TestVideoClearThenReselectSameVideoRefetches(t *testing.T) {
	b := &fakeDetailBackend{cacheOK: false}
	keys := keymap.Build(config.KeyBindings{Close: "esc"})
	v := domain.Video{ID: "v1", URL: "https://youtu.be/v1", Title: "one"}

	vd, _ := NewVideoDetail(context.Background(), b, b, keys, v, VideoDetailOpts{InitialView: InitialViewPanel, TranscriptWidth: "50%"})
	cleared, _ := vd.Update(VideoClearMsg{})
	vd = cleared.(VideoDetail)
	if vd.fetchVideo.ID != "" {
		t.Fatalf("clear should forget fetchVideo, got %q", vd.fetchVideo.ID)
	}
	if vd.video != nil {
		t.Fatal("clear should drop the shown video")
	}

	got, cmd := vd.Update(tuipkg.VideoSelectedMsg{Video: v})
	vd2 := got.(VideoDetail)
	if cmd == nil {
		t.Fatal("reselecting the cleared video should trigger a fetch, got nil cmd")
	}
	if !vd2.loading {
		t.Error("reselecting the cleared video should set loading=true")
	}
	if vd2.fetchVideo.ID != "v1" {
		t.Errorf("fetchVideo should be set to the reselected video, got %q", vd2.fetchVideo.ID)
	}
}

// TestFetchCtxCanceledOnClose verifies closing the panel cancels the in-flight
// fetch context.
func TestFetchCtxCanceledOnClose(t *testing.T) {
	b := &fakeDetailBackend{cacheOK: false}
	keys := keymap.Build(config.KeyBindings{Close: "esc"})
	v := domain.Video{ID: "v1", URL: "https://youtu.be/v1", Title: "one"}

	vd, _ := NewVideoDetail(context.Background(), b, b, keys, v, VideoDetailOpts{InitialView: InitialViewPanel, TranscriptWidth: "50%"})
	ctx := vd.fetchCtx

	if _, cmd := vd.handlePanelKey(tea.KeyPressMsg{Code: tea.KeyEscape}); cmd == nil {
		t.Fatal("Esc should return a close command")
	}
	if ctx.Err() == nil {
		t.Error("closing the panel should cancel the in-flight fetch context")
	}
}
