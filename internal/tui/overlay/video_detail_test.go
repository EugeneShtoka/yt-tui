package overlay

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
)

// TestVideoDetailRenderRectangular guards against the panel layout corruption
// where a rendered frame line ends up wider or narrower than the terminal (or
// gains an embedded newline), pushing the right border off its column and
// shifting subsequent rows. The composed frame must be a perfect rectangle:
// every line exactly `width` display columns, and exactly `height` lines.
func TestVideoDetailRenderRectangular(t *testing.T) {
	const width, height = 120, 40

	vd := VideoDetail{
		keys:     keymap.Build(config.KeyBindings{Close: "esc", Up: "k", Down: "j"}),
		subState: vdPanel,
	}
	details := domain.VideoDetails{
		Video: domain.Video{
			Title:      "Tan France Builds His Dream Home From Start to Finish | Architectural Digest → 🔥",
			Channel:    "Architectural Digest",
			ViewCount:  48800,
			Duration:   8262,
			UploadDate: "20260715",
			URL:        "https://www.youtube.com/watch?v=1CvWdg67luc",
		},
		Description: "Sit down and relax → ➤➤ 🔥🔥 then visit " +
			"https://www.example.com/some/really/long/path/that/overflows?q=1&x=2 for more, " +
			"and subscribe → http://bit.ly/abcdefghijklmnop 🔥 thanks for watching!",
		Subscribers: 1_000_000,
	}
	vd.video = &details
	vd.descLines = wordWrap(shortenURLs(details.Description, panelW-2), panelW-2)

	// "behind" simulates the tab table. Deliberately include lines WIDER than
	// the remaining width (width-panelW) and containing wide runes — the case
	// that made lipgloss .Width() word-wrap a single row into two lines and
	// desync the panel border.
	behindLines := make([]string, height)
	for i := range behindLines {
		behindLines[i] = strings.Repeat("Инфо ", 25) + "→🔥 overflow tail"
	}
	behind := strings.Join(behindLines, "\n")

	out, _ := vd.Render(behind, width, height)
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
