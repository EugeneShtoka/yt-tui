package overlay

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
)

// fakeTranscriptBackend serves a canned transcript.
type fakeTranscriptBackend struct {
	apitest.NopBackend
	text string
	ok   bool
}

func (b *fakeTranscriptBackend) GetTranscript(context.Context, string, string) (string, bool, error) {
	return b.text, b.ok, nil
}

func transcriptKeys() keymap.KeyMap {
	return keymap.Build(config.KeyBindings{Close: "esc", Quit: "q", Up: "k", Down: "j", GotoBottom: "G", OpenTranscript: "e", CopyTranscript: "y", NextChapter: "]", PrevChapter: "["})
}

// TestTranscriptModalRendersAndCopies drives the standalone transcript overlay
// end to end: load → render the text + copy hint → 'c' emits a CopyTextMsg.
func TestTranscriptModalRendersAndCopies(t *testing.T) {
	const width, height = 100, 30
	b := &fakeTranscriptBackend{text: "first caption line\nsecond caption line\n", ok: true}
	v := domain.Video{ID: "abc", URL: "https://youtu.be/abc", Title: "T"}

	vd, cmd := NewVideoDetail(context.Background(), b, b, transcriptKeys(), v, VideoDetailOpts{InitialView: InitialViewTranscript, TranscriptWidth: "50%"})
	if cmd == nil {
		t.Fatal("NewVideoDetail returned no command")
	}
	msg, ok := cmd().(vdTranscriptMsg)
	if !ok {
		t.Fatalf("transcript modal must load via vdTranscriptMsg, got %T", cmd())
	}

	model, _ := vd.Update(msg)
	got := model.(VideoDetail)
	if got.subState != vdTranscript {
		t.Fatalf("subState = %d, want vdTranscript", got.subState)
	}

	out := got.Render(rectangularBehind(width, height), width, height)
	if !strings.Contains(out, "Transcript") ||
		!strings.Contains(out, "first caption line") ||
		!strings.Contains(out, "second caption line") {
		t.Fatalf("transcript modal missing expected content:\n%s", out)
	}
	if !strings.Contains(out, "y: copy all") {
		t.Errorf("transcript modal missing copy hint:\n%s", out)
	}
	// No rendered line may exceed the terminal width (ClampLine invariant).
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w > width {
			t.Errorf("line %d width = %d, want <= %d", i, w, width)
		}
	}

	// The copy-transcript key copies the full transcript text to the clipboard.
	model2, ccmd := got.handleTranscriptKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	_ = model2
	if ccmd == nil {
		t.Fatal("copy key produced no command")
	}
	cp, ok := ccmd().(tuipkg.CopyTextMsg)
	if !ok {
		t.Fatalf("copy key must emit CopyTextMsg, got %T", ccmd())
	}
	if cp.Text != b.text {
		t.Errorf("copied text = %q, want full transcript %q", cp.Text, b.text)
	}
}

// TestTranscriptChaptersRenderAndNavigate verifies chapter headers render bold
// with the "## <timestamp>" marker stripped, and that ] / [ jump the scroll
// position between chapter headers.
func TestTranscriptChaptersRenderAndNavigate(t *testing.T) {
	const width, height = 100, 30
	body := "## 0:00 First Chapter\nalpha\nbeta\n## 1:40 Second Chapter\ngamma\ndelta\n"
	b := &fakeTranscriptBackend{text: body, ok: true}
	v := domain.Video{ID: "abc", URL: "https://youtu.be/abc", Title: "T"}

	vd, cmd := NewVideoDetail(context.Background(), b, b, transcriptKeys(), v, VideoDetailOpts{InitialView: InitialViewTranscript, TranscriptWidth: "50%"})
	model, _ := vd.Update(cmd().(vdTranscriptMsg))
	// Establish the popup width so transcriptWrapped agrees on line indices.
	model, _ = model.(VideoDetail).Update(OverlaySizeMsg{ContentW: width})
	got := model.(VideoDetail)

	out := got.Render(rectangularBehind(width, height), width, height)
	if !strings.Contains(out, "First Chapter") || !strings.Contains(out, "Second Chapter") {
		t.Fatalf("chapter titles missing from render:\n%s", out)
	}
	if strings.Contains(out, "## ") {
		t.Errorf("chapter marker '## ' should be stripped in the overlay:\n%s", out)
	}
	if strings.Contains(out, "1:40") {
		t.Errorf("chapter timestamp should be stripped from the header:\n%s", out)
	}
	if !strings.Contains(out, "]: chapters") {
		t.Errorf("chapter navigation hint missing:\n%s", out)
	}

	// ] jumps to the next chapter header (wrapped line index 3), [ back to the first.
	next, _ := got.handleTranscriptKey(tea.KeyPressMsg{Code: ']', Text: "]"})
	if vs := next.(VideoDetail).transcriptVS; vs != 3 {
		t.Errorf("] should land on second chapter (row 3), got vs=%d", vs)
	}
	prev, _ := next.(VideoDetail).handleTranscriptKey(tea.KeyPressMsg{Code: '[', Text: "["})
	if vs := prev.(VideoDetail).transcriptVS; vs != 0 {
		t.Errorf("[ should land back on first chapter (row 0), got vs=%d", vs)
	}
}

// TestTranscriptMissingPops verifies a standalone modal with no transcript
// available closes itself with an error status.
func TestTranscriptMissingPops(t *testing.T) {
	b := &fakeTranscriptBackend{ok: false}
	v := domain.Video{ID: "x", URL: "u"}
	vd, cmd := NewVideoDetail(context.Background(), b, b, transcriptKeys(), v, VideoDetailOpts{InitialView: InitialViewTranscript, TranscriptWidth: "50%"})
	msg := cmd().(vdTranscriptMsg)
	if msg.err == nil {
		t.Fatal("expected an error when no transcript is available")
	}
	_, batch := vd.Update(msg)
	if batch == nil {
		t.Fatal("expected a command batch (pop + status) on missing transcript")
	}
}
