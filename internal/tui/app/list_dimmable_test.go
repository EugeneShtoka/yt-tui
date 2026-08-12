package app

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/tui/tab"
)

// The video-list tabs must satisfy listDimmable so syncListBorderDimmed's type
// assertion actually matches them at runtime. This is a compile-time guard: an
// interface with an unexported method can only be satisfied within its own
// package, so if WithListBorderDimmed were ever unexported again the assertion
// would silently stop matching (the border would never dim) — here it fails to
// compile instead.
var (
	_ listDimmable = tab.Feed{}
	_ listDimmable = tab.Local{}
	_ listDimmable = tab.Channels{}
	_ listDimmable = tab.Tags{}
	_ listDimmable = tab.Playlists{}
	_ listDimmable = tab.Search{}
)

// TestListDimmableReturnsTab confirms the setter returns the same concrete tab
// type (value semantics preserved) so Root can store it back in its tab slice.
func TestListDimmableReturnsTab(t *testing.T) {
	if _, ok := (tab.Feed{}).WithListBorderDimmed(true).(tab.Feed); !ok {
		t.Error("Feed.WithListBorderDimmed must return a tab.Feed")
	}
}
