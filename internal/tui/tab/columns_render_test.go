package tab

import (
	"context"
	"strings"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

// With no column selection every offered column renders (the show-all default).
func TestLocalRendersAllColumnsByDefault(t *testing.T) {
	const width, height = 120, 24
	lc := NewLocal(context.Background(), &fakeBackend{}, testKeys(), false, "")
	lc, _ = updateLocal(lc, sized(width, height))
	lc, _ = updateLocal(lc, localLoadedMsg{videos: []domain.LocalVideo{{ID: "v1", Title: "Hello", Channel: "Chan"}}})
	content := lc.View().Content
	for _, header := range []string{"Title", "Channel", "Duration", "Views", "Size", "Date"} {
		if !strings.Contains(content, header) {
			t.Errorf("default Local view missing %q header:\n%s", header, content)
		}
	}
}

// A configured subset both hides the omitted columns and keeps the chosen ones.
func TestLocalHonorsConfiguredColumns(t *testing.T) {
	const width, height = 120, 24
	lc := NewLocal(context.Background(), &fakeBackend{}, testKeys(), false, "", videotable.KeyNum, videotable.KeyTitle, videotable.KeyDate)
	lc, _ = updateLocal(lc, sized(width, height))
	lc, _ = updateLocal(lc, localLoadedMsg{videos: []domain.LocalVideo{{ID: "v1", Title: "Hello", Channel: "Chan"}}})
	content := lc.View().Content
	if !strings.Contains(content, "Title") || !strings.Contains(content, "Date") {
		t.Errorf("configured columns missing Title/Date:\n%s", content)
	}
	for _, hidden := range []string{"Channel", "Views", "Size", "Duration"} {
		if strings.Contains(content, hidden) {
			t.Errorf("hidden column %q still rendered:\n%s", hidden, content)
		}
	}
}
