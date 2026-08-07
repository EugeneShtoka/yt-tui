package overlay

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"charm.land/bubbles/v2/textinput"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
)

func atpKeys() keymap.KeyMap {
	return keymap.Build(config.KeyBindings{Close: "esc", Up: "k", Down: "j", DrillDown: "enter"})
}

// assertRectangular guards the same invariant as the video-detail panel test:
// the composed frame must be exactly `width` columns on every line and exactly
// `height` lines, or the overlay border desyncs and shifts every row below it.
func assertRectangular(t *testing.T, out string, width, height int) {
	t.Helper()
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			t.Errorf("line %d width = %d, want %d: %q", i, w, width, l)
		}
	}
	if len(lines) != height {
		t.Errorf("composed line count = %d, want %d (embedded newline / wrap corruption)", len(lines), height)
	}
}

// rectangularBehind returns a height×width block of exactly-width lines to stand
// in for the tab table the overlay is drawn over.
func rectangularBehind(width, height int) string {
	rows := make([]string, height)
	for i := range rows {
		rows[i] = strings.Repeat("x", width)
	}
	return strings.Join(rows, "\n")
}

func TestAddToPlaylistRenderRectangularList(t *testing.T) {
	const width, height = 100, 30
	atp := AddToPlaylist{
		keys:     atpKeys(),
		input:    textinput.New(),
		ytLoaded: true,
		sel:      1,
		ytPlaylists: []domain.YTPlaylist{
			{ID: "PL1", Title: "Watch Later"},
			// A deliberately over-long, wide-rune title to stress truncation.
			{ID: "PL2", Title: "Мои любимые видео про архитектуру и дизайн интерьера 🔥🔥🔥 overflow tail"},
		},
	}
	out := atp.Render(rectangularBehind(width, height), width, height)
	assertRectangular(t, out, width, height)
}

func TestAddToPlaylistRenderRectangularCreateMode(t *testing.T) {
	const width, height = 100, 30
	ti := textinput.New()
	ti.SetValue("New Playlist Name")
	atp := AddToPlaylist{
		keys:       atpKeys(),
		input:      ti,
		createMode: true,
		createYT:   true,
	}
	out := atp.Render(rectangularBehind(width, height), width, height)
	assertRectangular(t, out, width, height)
}
