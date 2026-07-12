package ui

import "testing"

func TestOverlayStackPushPopTop(t *testing.T) {
	var m Model
	if m.topOverlay() != overlayNone {
		t.Fatalf("empty stack top = %d, want overlayNone", m.topOverlay())
	}
	m.pushOverlay(overlayVideoDetail)
	m.pushOverlay(overlayLinks)
	if m.topOverlay() != overlayLinks {
		t.Errorf("top after pushing links = %d, want overlayLinks", m.topOverlay())
	}
	// Popping links reveals the video-detail base — the whole point of the stack.
	m.popOverlay()
	if m.topOverlay() != overlayVideoDetail {
		t.Errorf("top after popping links = %d, want overlayVideoDetail", m.topOverlay())
	}
	m.popOverlay()
	if m.topOverlay() != overlayNone || len(m.overlays) != 0 {
		t.Errorf("stack not empty after final pop: %v", m.overlays)
	}
	// Popping an empty stack is a no-op.
	m.popOverlay()
	if len(m.overlays) != 0 {
		t.Errorf("pop on empty stack changed it: %v", m.overlays)
	}
}

func TestOverlayPushIdempotentAtTop(t *testing.T) {
	var m Model
	m.pushOverlay(overlayVideoDetail)
	m.pushOverlay(overlayVideoDetail) // re-open the frontmost overlay: no-op
	if len(m.overlays) != 1 {
		t.Errorf("idempotent push added a duplicate: %v", m.overlays)
	}
}

func TestOverlayHasMembership(t *testing.T) {
	var m Model
	m.pushOverlay(overlayVideoDetail)
	m.pushOverlay(overlayChapters)
	if !m.hasOverlay(overlayVideoDetail) || !m.hasOverlay(overlayChapters) {
		t.Errorf("hasOverlay missed a stacked overlay: %v", m.overlays)
	}
	if m.hasOverlay(overlayLinks) || m.hasOverlay(overlayAdd) {
		t.Errorf("hasOverlay reported an absent overlay: %v", m.overlays)
	}
}

func TestCloseOverlaysFrom(t *testing.T) {
	var m Model
	m.pushOverlay(overlayVideoDetail)
	m.pushOverlay(overlayLinks)
	// Tearing down video-detail also removes anything stacked above it.
	m.closeOverlaysFrom(overlayVideoDetail)
	if len(m.overlays) != 0 {
		t.Errorf("closeOverlaysFrom(videoDetail) left: %v", m.overlays)
	}

	// No-op when the target isn't present.
	m.pushOverlay(overlayAdd)
	m.closeOverlaysFrom(overlayVideoDetail)
	if !m.hasOverlay(overlayAdd) {
		t.Errorf("closeOverlaysFrom removed unrelated overlay: %v", m.overlays)
	}
}

// closeVideoDetail must clear the whole video-detail sub-stack, matching the
// old vidDetailOverlay=false + chapterOverlay=false teardown.
func TestCloseVideoDetailClearsStack(t *testing.T) {
	m := Model{overlays: []overlayKind{overlayVideoDetail, overlayChapters}}
	nm := m.closeVideoDetail()
	if len(nm.overlays) != 0 {
		t.Errorf("closeVideoDetail left overlays: %v", nm.overlays)
	}
	if nm.vidDetailVideo != nil {
		t.Errorf("closeVideoDetail did not reset vidDetailVideo")
	}
}
