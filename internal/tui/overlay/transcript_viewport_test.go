package overlay

import "testing"

// transcriptViewportRows must track contentH (the modal height) and only fall to
// its floor when the height is unknown. The 3-row floor is exactly the symptom of
// the "popup is 3 rows tall" bug: it appears when a standalone modal is opened
// without its size seeded (contentH == 0), so Root must seed it on open.
func TestTranscriptViewportRowsTracksContentH(t *testing.T) {
	var vd VideoDetail // contentH == 0 → floor
	if got := vd.transcriptViewportRows(); got != 3 {
		t.Errorf("contentH=0 → %d rows, want floor 3", got)
	}
	vd.contentH = 40
	if got, want := vd.transcriptViewportRows(), 40-modalChromeRows; got != want {
		t.Errorf("contentH=40 → %d rows, want %d", got, want)
	}
}
