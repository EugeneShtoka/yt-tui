package ui

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tea "github.com/charmbracelet/bubbletea"
)

func recCtx(videos []domain.Video, pageSize int) viewCtx {
	f := feed.NewStarting(videos)
	return viewCtx{keys: testListKeys(), pageSize: pageSize, circular: false, recFeed: &f}
}

func sampleVideos() []domain.Video {
	return []domain.Video{
		{ID: "a", Title: "Alpha"},
		{ID: "b", Title: "Beta"},
		{ID: "c", Title: "Gamma"},
	}
}

func TestRecommendedDownMovesCursor(t *testing.T) {
	v := recommendedView{sort: vidSortViews}
	// Every key forwards a recActionIntent (matching the original handler, which
	// always ran handleVideoAction after its nav switch), so the intent being
	// non-nil is expected; what matters here is the cursor moved.
	if in := v.update(tea.KeyMsg{Type: tea.KeyDown}, recCtx(sampleVideos(), 10)); in == nil {
		t.Fatal("update should always forward a recActionIntent")
	}
	if v.cursor != 1 {
		t.Errorf("Down: cursor=%d, want 1", v.cursor)
	}
}

func TestRecommendedActionIntentCarriesKey(t *testing.T) {
	v := recommendedView{}
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}
	in := v.update(msg, recCtx(sampleVideos(), 10))
	got, ok := in.(recActionIntent)
	if !ok {
		t.Fatalf("want recActionIntent, got %T", in)
	}
	if got.msg.String() != "p" {
		t.Errorf("intent carried msg=%q, want p", got.msg.String())
	}
	// A non-nav key must not move the cursor.
	if v.cursor != 0 {
		t.Errorf("non-nav key moved cursor to %d", v.cursor)
	}
}

func TestRecommendedReclampAfterShrink(t *testing.T) {
	v := recommendedView{cursor: 2}
	v.reclamp(2, 10)
	if v.cursor != 1 {
		t.Errorf("reclamp: cursor=%d, want 1", v.cursor)
	}
}

func TestRecommendedContext(t *testing.T) {
	if got := (recommendedView{}).context(viewCtx{}); got != CtxVideoList {
		t.Errorf("context=%v, want CtxVideoList", got)
	}
}
