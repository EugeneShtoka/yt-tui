package ui

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tea "github.com/charmbracelet/bubbletea"
)

func subCtx(videos []domain.Video, pageSize int) viewCtx {
	f := feed.New(videos)
	return viewCtx{keys: testListKeys(), pageSize: pageSize, circular: false, subFeed: &f}
}

func TestSubscriptionsDownMovesCursor(t *testing.T) {
	v := subscriptionsView{sort: vidSortDate}
	if in := v.update(tea.KeyMsg{Type: tea.KeyDown}, subCtx(sampleVideos(), 10)); in == nil {
		t.Fatal("update should always forward a subActionIntent")
	}
	if v.cursor != 1 {
		t.Errorf("Down: cursor=%d, want 1", v.cursor)
	}
}

func TestSubscriptionsActionIntentCarriesKey(t *testing.T) {
	v := subscriptionsView{}
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}
	in := v.update(msg, subCtx(sampleVideos(), 10))
	got, ok := in.(subActionIntent)
	if !ok {
		t.Fatalf("want subActionIntent, got %T", in)
	}
	if got.msg.String() != "p" {
		t.Errorf("intent carried msg=%q, want p", got.msg.String())
	}
	if v.cursor != 0 {
		t.Errorf("non-nav key moved cursor to %d", v.cursor)
	}
}

func TestSubscriptionsReclampAfterShrink(t *testing.T) {
	v := subscriptionsView{cursor: 2}
	v.reclamp(2, 10)
	if v.cursor != 1 {
		t.Errorf("reclamp: cursor=%d, want 1", v.cursor)
	}
}

func TestSubscriptionsContext(t *testing.T) {
	if got := (subscriptionsView{}).context(viewCtx{}); got != CtxVideoList {
		t.Errorf("context=%v, want CtxVideoList", got)
	}
}
