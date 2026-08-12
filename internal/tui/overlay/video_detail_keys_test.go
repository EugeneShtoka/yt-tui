package overlay

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
)

// These tests characterize the VideoDetail panel↔modal state machine (open a
// modal from the panel, navigate, dismiss back) so the H-3 decomposition stays
// behavior-preserving. They avoid the DrillDown "open URL" path, which execs a
// real URL opener.

func vdStateKeys() keymap.KeyMap {
	return keymap.Build(config.KeyBindings{
		Close: "esc", Up: "k", Down: "j",
		GotoPrefix: "g", GotoBottom: "G",
		OpenLinks: "L", OpenChapters: "C", OpenTranscript: "T",
		DrillDown: "enter", Play: "p", PlayAudio: "a", CopyURL: "y",
	})
}

func vdKey(r rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: r, Text: string(r)} }
func vdEsc() tea.KeyPressMsg       { return tea.KeyPressMsg{Code: tea.KeyEscape} }
func vdEnter() tea.KeyPressMsg     { return tea.KeyPressMsg{Code: tea.KeyEnter} }

// panelVD builds a loaded panel overlay with links, chapters, and a transcript
// all preloaded, so each modal can be opened without triggering a fetch.
func panelVD(closeOnLinks bool) VideoDetail {
	return VideoDetail{
		keys:         vdStateKeys(),
		subState:     vdPanel,
		initialView:  InitialViewPanel,
		closeOnLinks: closeOnLinks,
		video:        &domain.VideoDetails{Video: domain.Video{ID: "abc", URL: "https://youtu.be/abc", Title: "T"}},

		links:       []domain.Link{{Label: "A", URL: "https://a.example"}, {Label: "B", URL: "https://b.example"}},
		linksLoaded: true,

		chapters:       []domain.Chapter{{Title: "Intro", OriginalStart: 0}, {Title: "Part 2", OriginalStart: 100}},
		chaptersLoaded: true,

		transcriptText:   "line one\nline two\nline three",
		transcriptLoaded: true,
	}
}

func asVD(t *testing.T, m tea.Model) VideoDetail {
	t.Helper()
	vd, ok := m.(VideoDetail)
	if !ok {
		t.Fatalf("Update returned %T, want VideoDetail", m)
	}
	return vd
}

// PanelFocused drives the list-border dim: it is true only for a focused side
// panel, and FocusSwitch toggles it (Root reads it to fade the list frame).
func TestVideoDetailPanelFocused(t *testing.T) {
	vd := panelVD(false)
	if vd.PanelFocused() {
		t.Fatal("side panel should start unfocused")
	}
	vd = asVD(t, mustModel(vd.Update(FocusSwitchMsg{})))
	if !vd.PanelFocused() {
		t.Fatal("PanelFocused should be true after FocusSwitch")
	}
	vd = asVD(t, mustModel(vd.Update(FocusSwitchMsg{})))
	if vd.PanelFocused() {
		t.Fatal("PanelFocused should toggle back to false")
	}

	// A standalone (non-panel) view is never a "focused panel" even if focused
	// were somehow set — FocusSwitch is a no-op outside vdPanel.
	std := panelVD(false)
	std.initialView = InitialViewTranscript
	std.subState = vdTranscript
	std = asVD(t, mustModel(std.Update(FocusSwitchMsg{})))
	if std.PanelFocused() {
		t.Fatal("non-panel view must not report PanelFocused")
	}
}

func mustModel(m tea.Model, _ tea.Cmd) tea.Model { return m }

// OpenModalMsg opens a sub-view inside an already-open (even unfocused) panel,
// so Root never stacks a second VideoDetail overlay. While the modal is open the
// panel captures input even though it was opened unfocused; Esc returns to the
// panel and releases input back to the list.
func TestOpenModalMsgOpensSubViewInUnfocusedPanel(t *testing.T) {
	vd := panelVD(false) // unfocused, transcript preloaded
	if vd.HasFocus() {
		t.Fatal("unfocused panel with no modal should not capture input")
	}
	vd = asVD(t, mustModel(vd.Update(OpenModalMsg{View: InitialViewTranscript})))
	if vd.subState != vdTranscript {
		t.Fatalf("subState = %d, want vdTranscript after OpenModalMsg", vd.subState)
	}
	if !vd.HasFocus() {
		t.Fatal("panel with an open modal must capture input even when unfocused")
	}
	vd = asVD(t, mustModel(vd.Update(vdEsc())))
	if vd.subState != vdPanel {
		t.Fatalf("subState = %d, want vdPanel after Esc", vd.subState)
	}
	if vd.HasFocus() {
		t.Fatal("closing the modal must release input capture back to the list")
	}
}

// A same-video selection that arrives while the panel is still fetching (e.g. the
// 150ms debounce scheduled when a sub-view is opened during initial load) must
// not bump fetchToken — doing so cancels the in-flight details/transcript fetch
// and leaves the panel stuck loading.
func TestVideoSelectedNoopWhileFetchingSameVideo(t *testing.T) {
	v := domain.Video{ID: "abc", URL: "https://youtu.be/abc"}
	vd := VideoDetail{
		keys: vdStateKeys(), subState: vdPanel, initialView: InitialViewPanel,
		loading: true, fetchVideo: v, fetchToken: 3, // video not yet loaded
	}
	got, cmd := vd.Update(tuipkg.VideoSelectedMsg{Video: v})
	vd = asVD(t, got)
	if vd.fetchToken != 3 {
		t.Fatalf("fetchToken = %d, want 3 (a same-video selection during load must not restart the fetch)", vd.fetchToken)
	}
	if cmd != nil {
		t.Fatalf("expected no reload command for the already-fetching video, got %#v", cmd())
	}
}

// blockingTranscriptBackend blocks GetTranscript until the caller's ctx ends,
// standing in for a cold fetch that outlives the client's patience.
type blockingTranscriptBackend struct{ apitest.NopBackend }

func (b blockingTranscriptBackend) GetTranscript(ctx context.Context, _, _ string) (string, bool, error) {
	<-ctx.Done()
	return "", false, nil // caller inspects ctx.Err() itself for the timeout branch
}

// A cold transcript fetch that outruns transcriptFetchTimeout must not hang the
// popup: the command resolves with a retry hint instead of blocking forever.
func TestTranscriptLoadTimesOutInsteadOfHanging(t *testing.T) {
	orig := transcriptFetchTimeout
	transcriptFetchTimeout = 20 * time.Millisecond
	defer func() { transcriptFetchTimeout = orig }()

	vd := VideoDetail{
		keys:       transcriptKeys(), // binds OpenTranscript to the e key
		media:      blockingTranscriptBackend{},
		fetchVideo: domain.Video{ID: "abc", URL: "https://youtu.be/abc"},
	}
	vd = vd.renewFetchCtx()

	cmd := vd.transcriptLoadCmd(vd.fetchVideo)
	done := make(chan vdTranscriptMsg, 1)
	go func() { done <- cmd().(vdTranscriptMsg) }()

	select {
	case m := <-done:
		if m.err == nil || !strings.Contains(m.err.Error(), "still fetching") {
			t.Fatalf("want a 'still fetching' timeout error, got %v", m.err)
		}
		if !strings.Contains(m.err.Error(), "e") {
			t.Errorf("timeout error should mention the retry key, got %q", m.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("transcriptLoadCmd hung past the timeout")
	}
}

// A first-open transcript (cache miss) shows a loading-spinner popup while the
// fetch is in flight, then opens the modal once it resolves.
func TestTranscriptLoadingPopupShownUntilResolved(t *testing.T) {
	vd := panelVD(false)
	vd.transcriptLoaded = false // force a miss
	vd.transcriptText = ""

	vd = asVD(t, mustModel(vd.Update(OpenModalMsg{View: InitialViewTranscript})))
	if !vd.transcriptLoading {
		t.Fatal("transcriptLoading should be true after opening a not-yet-loaded transcript")
	}
	if vd.subState != vdPanel {
		t.Fatalf("subState = %d, want vdPanel while the fetch is in flight", vd.subState)
	}
	vd.spinnerFrame = "|"
	if out := vd.Render("behind", 120, 30); !strings.Contains(out, "Loading transcript…") {
		t.Fatalf("expected the loading popup in the render, got:\n%s", out)
	}

	vd = asVD(t, mustModel(vd.Update(vdTranscriptMsg{text: "hello world", token: vd.fetchToken})))
	if vd.transcriptLoading {
		t.Fatal("transcriptLoading should clear once the transcript resolves")
	}
	if vd.subState != vdTranscript {
		t.Fatalf("subState = %d, want vdTranscript after the fetch resolves", vd.subState)
	}
}

// Triggering the transcript while the panel is still loading its details
// (vd.video == nil) must still start the fetch and show the loading popup — the
// transcript is an independent fetch keyed off fetchVideo, not the details.
func TestTranscriptWorksWhileDetailsStillLoading(t *testing.T) {
	v := domain.Video{ID: "abc", URL: "https://youtu.be/abc"}
	vd := VideoDetail{
		keys: vdStateKeys(), subState: vdPanel, initialView: InitialViewPanel,
		loading: true, fetchVideo: v, // details in flight: video not set yet
	}
	got, cmd := vd.Update(OpenModalMsg{View: InitialViewTranscript})
	vd = asVD(t, got)
	if !vd.transcriptLoading {
		t.Fatal("transcriptLoading should be true even while details are still loading")
	}
	if cmd == nil {
		t.Fatal("expected a transcript-load command to be issued while details load")
	}
	vd.spinnerFrame = "|"
	if out := vd.Render("behind", 120, 30); !strings.Contains(out, "Loading transcript…") {
		t.Fatalf("expected the loading popup while details load, got:\n%s", out)
	}
}

// Switching to a different video must drop the previous video's transcript and
// any in-flight loading popup, so a superseded fetch can't leak across videos.
func TestVideoChangeResetsTranscriptState(t *testing.T) {
	vd := panelVD(false)
	vd.transcriptLoading = true
	vd.transcriptLoaded = true
	vd.transcriptText = "old transcript"

	other := domain.Video{ID: "zzz", URL: "https://youtu.be/zzz"}
	vd = asVD(t, mustModel(vd.Update(tuipkg.VideoSelectedMsg{Video: other})))
	if vd.transcriptLoading || vd.transcriptLoaded || vd.transcriptText != "" {
		t.Fatalf("transcript state not reset on video change: loading=%v loaded=%v text=%q",
			vd.transcriptLoading, vd.transcriptLoaded, vd.transcriptText)
	}
}

func TestPanelOpensLinksAndDismissesBackToPanel(t *testing.T) {
	vd := panelVD(false)
	m, _ := vd.Update(vdKey('L'))
	if got := asVD(t, m).subState; got != vdLinks {
		t.Fatalf("after OpenLinks subState = %d, want vdLinks", got)
	}
	m, _ = asVD(t, m).Update(vdEsc())
	if got := asVD(t, m).subState; got != vdPanel {
		t.Errorf("after Esc subState = %d, want vdPanel (return to panel, not pop)", got)
	}
}

func TestPanelOpenLinksWithNoLinksStaysPanelWithStatus(t *testing.T) {
	vd := panelVD(false)
	vd.links, vd.linksLoaded = nil, true // loaded but empty
	m, cmd := vd.Update(vdKey('L'))
	if got := asVD(t, m).subState; got != vdPanel {
		t.Errorf("subState = %d, want vdPanel (no links → stay on panel)", got)
	}
	if sm, ok := cmd().(tuipkg.StatusMsg); !ok || !strings.Contains(sm.Text, "no links") {
		t.Errorf("want a 'no links' StatusMsg, got %#v", cmd())
	}
}

func TestLinksSelectionMovesAndCopyEmitsURL(t *testing.T) {
	vd := panelVD(false)
	vd.subState = vdLinks
	m, _ := vd.Update(vdKey('j')) // Down
	if got := asVD(t, m).linkSel; got != 1 {
		t.Fatalf("after Down linkSel = %d, want 1", got)
	}
	m, cmd := asVD(t, m).Update(vdKey('y')) // CopyURL of the selected link
	if got := asVD(t, m).subState; got != vdLinks {
		t.Errorf("CopyURL should not change subState, got %d", got)
	}
	cm, ok := cmd().(tuipkg.CopyURLMsg)
	if !ok || cm.URL != "https://b.example" {
		t.Errorf("want CopyURLMsg for the 2nd link, got %#v", cmd())
	}
}

func TestLinksDrillDownClosesOnlyWhenCloseOnLinks(t *testing.T) {
	// closeOnLinks=false: DrillDown keeps the modal open (URL opens in background).
	vd := panelVD(false)
	vd.subState = vdLinks
	m, _ := vd.Update(vdEnter())
	if got := asVD(t, m).subState; got != vdLinks {
		t.Errorf("closeOnLinks=false: subState = %d, want vdLinks (stay open)", got)
	}
	// closeOnLinks=true: DrillDown returns to the panel.
	vd2 := panelVD(true)
	vd2.subState = vdLinks
	m2, _ := vd2.Update(vdEnter())
	if got := asVD(t, m2).subState; got != vdPanel {
		t.Errorf("closeOnLinks=true: subState = %d, want vdPanel (dismiss on open)", got)
	}
}

func TestChaptersOpenSeekPlayAndDismiss(t *testing.T) {
	vd := panelVD(false)
	m, _ := vd.Update(vdKey('C'))
	if got := asVD(t, m).subState; got != vdChapters {
		t.Fatalf("after OpenChapters subState = %d, want vdChapters", got)
	}
	m, _ = asVD(t, m).Update(vdKey('j')) // select 2nd chapter (start=100)
	m, cmd := asVD(t, m).Update(vdKey('p'))
	pm, ok := cmd().(tuipkg.PlayVideoMsg)
	if !ok {
		t.Fatalf("Play should emit PlayVideoMsg, got %#v", cmd())
	}
	if !strings.Contains(pm.Video.URL, "&t=100") {
		t.Errorf("play URL should seek to chapter start: %q", pm.Video.URL)
	}
	m, _ = asVD(t, m).Update(vdEsc())
	if got := asVD(t, m).subState; got != vdPanel {
		t.Errorf("Esc should return to panel, got %d", got)
	}
}

func TestChaptersOpenWithNoneStaysPanel(t *testing.T) {
	vd := panelVD(false)
	vd.chapters, vd.chaptersLoaded = nil, false
	m, cmd := vd.Update(vdKey('C'))
	if got := asVD(t, m).subState; got != vdPanel {
		t.Errorf("no chapters → subState = %d, want vdPanel", got)
	}
	if sm, ok := cmd().(tuipkg.StatusMsg); !ok || !strings.Contains(sm.Text, "no chapters") {
		t.Errorf("want a 'no chapters' StatusMsg, got %#v", cmd())
	}
}

func TestTranscriptOpenScrollsAndDismisses(t *testing.T) {
	vd := panelVD(false)
	vd.transcriptWidth = "80"                    // absolute width → short lines don't wrap
	vd.contentH = 11                             // viewportRows = 11 - modalChromeRows(8) = 3
	vd.transcriptText = "l1\nl2\nl3\nl4\nl5\nl6" // 6 lines → maxVS = 6 - 3 = 3
	m, _ := vd.Update(vdKey('T'))
	if got := asVD(t, m).subState; got != vdTranscript {
		t.Fatalf("after OpenTranscript subState = %d, want vdTranscript", got)
	}
	m, _ = asVD(t, m).Update(vdKey('j')) // Down
	if got := asVD(t, m).transcriptVS; got != 1 {
		t.Errorf("after Down transcriptVS = %d, want 1", got)
	}
	// Down can't scroll past the last page (regression: it used to run away, which
	// left j/k dead after G).
	for i := 0; i < 10; i++ {
		m, _ = asVD(t, m).Update(vdKey('j'))
	}
	if got := asVD(t, m).transcriptVS; got != 3 {
		t.Errorf("Down past end transcriptVS = %d, want clamp at 3", got)
	}
	m, _ = asVD(t, m).Update(vdKey('g')) // gg → top
	if got := asVD(t, m).transcriptVS; got != 0 {
		t.Errorf("after gg transcriptVS = %d, want 0", got)
	}
	m, _ = asVD(t, m).Update(vdKey('G')) // G → bottom
	if got := asVD(t, m).transcriptVS; got != 3 {
		t.Errorf("after G transcriptVS = %d, want 3 (maxVS)", got)
	}
	m, _ = asVD(t, m).Update(vdEsc())
	if got := asVD(t, m).subState; got != vdPanel {
		t.Errorf("Esc should return to panel, got %d", got)
	}
}
