package tab

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// assertNoLineOverflows guards the ClampLine invariant: no rendered frame line
// may exceed the content width, or lipgloss re-wraps it and corrupts the layout.
func assertNoLineOverflows(t *testing.T, content string, width int) {
	t.Helper()
	for i, l := range strings.Split(content, "\n") {
		if w := ansi.StringWidth(l); w > width {
			t.Errorf("line %d width = %d > %d: %q", i, w, width, l)
		}
	}
}

// A pathological title (very long + wide runes + emoji) must not push any table
// row past the frame width once the tab is sized.
func TestFeedViewClampsWideRows(t *testing.T) {
	const width, height = 80, 24
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{Mode: "recommended", StaleDays: 30})
	f, _ = updateFeed(f, sized(width, height))
	f, _ = updateFeed(f, feedRecCacheMsg{videos: []domain.Video{{
		ID:      "v1",
		Title:   strings.Repeat("Очень длинное название видео 🔥 ", 8),
		Channel: strings.Repeat("Канал ", 20),
	}}})
	assertNoLineOverflows(t, f.View().Content, width)
}

func TestLocalViewClampsWideRows(t *testing.T) {
	const width, height = 80, 24
	lc := NewLocal(context.Background(), &fakeBackend{}, testKeys(), false, "")
	lc, _ = updateLocal(lc, sized(width, height))
	lc, _ = updateLocal(lc, localLoadedMsg{videos: []domain.LocalVideo{{
		ID:      "v1",
		Title:   strings.Repeat("A ridiculously long local title 🔥 ", 6),
		Channel: strings.Repeat("Channel ", 15),
	}}})
	assertNoLineOverflows(t, lc.View().Content, width)
}

// The empty/loading states are also frames and must not overflow.
func TestFeedEmptyViewFits(t *testing.T) {
	const width = 80
	f := NewFeed(context.Background(), &fakeBackend{}, testKeys(), false, FeedOpts{Mode: "recommended", StaleDays: 30})
	f, _ = updateFeed(f, tuipkg.ContentSizeMsg{Width: width, Height: 24})
	assertNoLineOverflows(t, f.View().Content, width)
}
