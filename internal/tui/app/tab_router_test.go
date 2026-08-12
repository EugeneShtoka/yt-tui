package app

import (
	"testing"

	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// idTab is a fakeTab with a configurable ID, so router tests can build a panel
// set with distinct TabIDs for indexOfID.
type idTab struct {
	fakeTab
	id tuipkg.TabID
}

func (t idTab) ID() tuipkg.TabID { return t.id }

func threeTabRouter() tabRouter {
	return tabRouter{
		tabs: []tuipkg.Tab{
			idTab{id: tuipkg.TabFeed},
			idTab{id: tuipkg.TabChannels},
			idTab{id: tuipkg.TabTags},
		},
		chordKeys: map[string]int{"1": 0, "t": 2},
		panelIdx:  map[string]int{"feed": 0, "channels": 1, "tags": 2},
	}
}

func TestTabRouterWrapIndex(t *testing.T) {
	tr := threeTabRouter()
	cases := []struct {
		from, dir, want int
	}{
		{0, +1, 1}, {0, -1, 2}, {2, +1, 0}, {1, -1, 0},
	}
	for _, c := range cases {
		tr.activeIdx = c.from
		if got := tr.wrapIndex(c.dir); got != c.want {
			t.Errorf("wrapIndex(from=%d,dir=%d) = %d, want %d", c.from, c.dir, got, c.want)
		}
	}
}

func TestTabRouterValid(t *testing.T) {
	tr := threeTabRouter()
	for i, want := range map[int]bool{-1: false, 0: true, 2: true, 3: false} {
		if got := tr.valid(i); got != want {
			t.Errorf("valid(%d) = %v, want %v", i, got, want)
		}
	}
}

func TestTabRouterIndexOfID(t *testing.T) {
	tr := threeTabRouter()
	if i, ok := tr.indexOfID(tuipkg.TabChannels); !ok || i != 1 {
		t.Errorf("indexOfID(Channels) = (%d,%v), want (1,true)", i, ok)
	}
	if _, ok := tr.indexOfID(tuipkg.TabSearch); ok {
		t.Error("indexOfID(Search) should be absent")
	}
}

func TestTabRouterPanelIndex(t *testing.T) {
	tr := threeTabRouter()
	if i, ok := tr.panelIndex("tags"); !ok || i != 2 {
		t.Errorf("panelIndex(tags) = (%d,%v), want (2,true)", i, ok)
	}
	if _, ok := tr.panelIndex("nope"); ok {
		t.Error("panelIndex(nope) should be absent")
	}
}

func TestTabRouterChord(t *testing.T) {
	tr := threeTabRouter()
	if tr.chordArmed() {
		t.Error("chord should start disarmed")
	}
	tr.armChord()
	if !tr.chordArmed() {
		t.Error("armChord should arm")
	}
	if i, ok := tr.resolveChord("t"); !ok || i != 2 {
		t.Errorf("resolveChord(t) = (%d,%v), want (2,true)", i, ok)
	}
	if tr.chordArmed() {
		t.Error("resolveChord must disarm")
	}
	// An armed chord on an unbound key disarms and returns not-ok.
	tr.armChord()
	if _, ok := tr.resolveChord("z"); ok {
		t.Error("resolveChord(z) should be not-ok")
	}
	if tr.chordArmed() {
		t.Error("resolveChord must disarm even on miss")
	}
}

func TestTabRouterActiveAndSetActive(t *testing.T) {
	tr := threeTabRouter()
	tr.setActive(2)
	if tr.active().ID() != tuipkg.TabTags {
		t.Errorf("active after setActive(2) = %v, want Tags", tr.active().ID())
	}
}
