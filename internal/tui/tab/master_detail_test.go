package tab

import "testing"

func TestMasterDetailPaneTransitions(t *testing.T) {
	t.Parallel()
	var m masterDetail

	if m.inDetail() {
		t.Fatal("a zero-value masterDetail must start in the master (list) pane")
	}
	m.drillIn()
	if !m.inDetail() {
		t.Error("drillIn should enter the detail pane")
	}
	m.drillOut()
	if m.inDetail() {
		t.Error("drillOut should return to the master (list) pane")
	}
}

func TestMasterDetailResizeRecordsDimensions(t *testing.T) {
	t.Parallel()
	var m masterDetail
	m.resize(120, 40)
	if m.width != 120 || m.height != 40 {
		t.Errorf("resize recorded (%d,%d), want (120,40)", m.width, m.height)
	}
}
