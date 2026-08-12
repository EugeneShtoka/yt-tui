package tab

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// searchWithVideos runs a query that returns only videos (no channels), so the
// video pane is focused, and returns the Search tab ready for key assertions.
func searchWithVideos(t *testing.T, vids ...domain.Video) Search {
	t.Helper()
	fb := &fakeBackend{
		search: func(_ context.Context, _ string) ([]domain.Channel, []domain.Video, error) {
			return nil, vids, nil
		},
	}
	s := NewSearch(context.Background(), fb, testKeys(), false)
	s, _ = updateSearch(s, sized(80, 24))
	s, cmd := updateSearch(s, tuipkg.SearchActivateMsg{Query: "go"})
	res, ok := runCmd(cmd).(srchResultMsg)
	if !ok {
		t.Fatalf("want srchResultMsg from query, got %T", runCmd(cmd))
	}
	s, _ = updateSearch(s, res)
	if !s.onVideos {
		t.Fatal("video pane should be focused when the result has no channels")
	}
	return s
}

// In the video-results pane, DrillDown plays the highlighted video.
func TestSearchVideoResultPlayHighlighted(t *testing.T) {
	s := searchWithVideos(t, domain.Video{ID: "sv1"}, domain.Video{ID: "sv2"})

	_, cmd := updateSearch(s, tea.KeyPressMsg{Code: tea.KeyEnter})

	play, ok := runCmd(cmd).(tuipkg.PlayVideoMsg)
	if !ok {
		t.Fatalf("DrillDown should emit PlayVideoMsg, got %#v", runCmd(cmd))
	}
	if play.Video.ID != "sv1" {
		t.Errorf("played video = %q, want sv1", play.Video.ID)
	}
}

// Navigation moves the cursor within the video-results pane, so DrillDown then
// plays the newly-highlighted video (exercises HandleNav + srchCurrentVideo).
func TestSearchVideoResultNavigateThenPlay(t *testing.T) {
	s := searchWithVideos(t, domain.Video{ID: "sv1"}, domain.Video{ID: "sv2"})

	s, _ = updateSearch(s, tea.KeyPressMsg{Text: "j"}) // Down
	_, cmd := updateSearch(s, tea.KeyPressMsg{Code: tea.KeyEnter})

	play, ok := runCmd(cmd).(tuipkg.PlayVideoMsg)
	if !ok {
		t.Fatalf("DrillDown should emit PlayVideoMsg, got %#v", runCmd(cmd))
	}
	if play.Video.ID != "sv2" {
		t.Errorf("played video after Down = %q, want sv2", play.Video.ID)
	}
}
