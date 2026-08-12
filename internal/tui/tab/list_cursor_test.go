package tab

import "testing"

func TestListCursorMoveClampAndWrap(t *testing.T) {
	t.Parallel()
	// Non-circular: clamps at both ends.
	c := listCursor{}
	c.move(-1, 3, 10) // already at 0, can't go below
	if c.cursor != 0 {
		t.Errorf("clamp low: cursor=%d, want 0", c.cursor)
	}
	c.move(+5, 3, 10) // past the end → clamp to n-1
	if c.cursor != 2 {
		t.Errorf("clamp high: cursor=%d, want 2", c.cursor)
	}

	// Circular: wraps around both ends.
	cc := listCursor{circular: true}
	cc.move(-1, 3, 10) // 0 → wraps to 2
	if cc.cursor != 2 {
		t.Errorf("wrap low: cursor=%d, want 2", cc.cursor)
	}
	cc.move(+1, 3, 10) // 2 → wraps to 0
	if cc.cursor != 0 {
		t.Errorf("wrap high: cursor=%d, want 0", cc.cursor)
	}
}

func TestListCursorMoveEmptyIsNoOp(t *testing.T) {
	t.Parallel()
	c := listCursor{cursor: 0}
	c.move(+1, 0, 10)
	if c.cursor != 0 || c.vs != 0 {
		t.Errorf("empty list move must be a no-op, got cursor=%d vs=%d", c.cursor, c.vs)
	}
}

func TestListCursorSyncViewportScrolls(t *testing.T) {
	t.Parallel()
	c := listCursor{}
	// Jump to index 9 in a 10-item list with a 3-row window: viewport must scroll
	// so the cursor is the last visible row (vs = 9-3+1 = 7).
	c.syncViewport(9, 10, 3)
	if c.cursor != 9 || c.vs != 7 {
		t.Errorf("syncViewport(9,10,3) = cursor %d vs %d, want 9, 7", c.cursor, c.vs)
	}
	// Jumping back above the window pulls vs down to the cursor.
	c.syncViewport(2, 10, 3)
	if c.cursor != 2 || c.vs != 2 {
		t.Errorf("syncViewport(2,10,3) = cursor %d vs %d, want 2, 2", c.cursor, c.vs)
	}
}

func TestListCursorWindow(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		vs, n, pageH int
		wantS, wantE int
	}{
		{"empty", 0, 0, 5, 0, 0},
		{"fits fully", 0, 3, 5, 0, 3},
		{"scrolled mid", 4, 20, 5, 4, 9},
		{"clamped at end", 18, 20, 5, 15, 20},
		{"zero pageH", 3, 20, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := listCursor{vs: tt.vs}
			s, e := c.window(tt.n, tt.pageH)
			if s != tt.wantS || e != tt.wantE {
				t.Errorf("window(%d,%d) with vs=%d = (%d,%d), want (%d,%d)", tt.n, tt.pageH, tt.vs, s, e, tt.wantS, tt.wantE)
			}
		})
	}
}

func TestListCursorPageScrolls(t *testing.T) {
	t.Parallel()
	c := listCursor{}
	c.page(+1, 20, 5) // page down one window
	if c.vs == 0 {
		t.Errorf("page down should advance the viewport, vs=%d", c.vs)
	}
	prev := c.vs
	c.page(-1, 20, 5) // page back up
	if c.vs >= prev {
		t.Errorf("page up should move the viewport back, vs=%d (was %d)", c.vs, prev)
	}
}
