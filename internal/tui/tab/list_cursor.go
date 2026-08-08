package tab

// listCursor is a plain cursor + viewport-start pair for a scrollable list of n
// items shown pageH rows at a time. It is the bespoke scroll/cursor math the
// Search tab's recent-query list used to hand-roll inline (M-3), pulled into one
// named, testable unit distinct from videotable.TableNav (which is a bordered
// multi-column table widget, not a plain text list). The item count n is passed
// in on each call so the cursor owns no data — only the position.
type listCursor struct {
	cursor   int
	vs       int // viewport start (index of the first visible row)
	circular bool
}

// move advances the cursor by delta (wrapping when circular, else clamping) and
// keeps it visible within a pageH-high window.
func (c *listCursor) move(delta, n, pageH int) {
	if n <= 0 {
		return
	}
	pos := c.cursor + delta
	if c.circular {
		pos = ((pos % n) + n) % n
	} else {
		if pos < 0 {
			pos = 0
		}
		if pos >= n {
			pos = n - 1
		}
	}
	c.syncViewport(pos, n, pageH)
}

// page scrolls by a full page in direction (-1 up, +1 down), keeping the cursor
// at the same relative row within the viewport where possible.
func (c *listCursor) page(direction, n, pageH int) {
	if n <= 0 {
		return
	}
	relPos := c.cursor - c.vs
	newVS := c.vs + direction*pageH
	if newVS < 0 {
		newVS = 0
	}
	if newVS+pageH > n {
		newVS = n - pageH
		if newVS < 0 {
			newVS = 0
		}
	}
	pos := newVS + relPos
	if pos < 0 {
		pos = 0
	}
	if pos >= n {
		pos = n - 1
	}
	c.vs = newVS
	c.cursor = pos
}

// jumpTo moves the cursor to idx, scrolling it into view.
func (c *listCursor) jumpTo(idx, n, pageH int) {
	if n <= 0 {
		return
	}
	c.syncViewport(idx, n, pageH)
}

// syncViewport sets the cursor to pos (clamped to [0,n)) and adjusts vs so pos
// stays visible in a pageH-high window.
func (c *listCursor) syncViewport(pos, n, pageH int) {
	if pos < 0 {
		pos = 0
	}
	if pos >= n {
		pos = n - 1
	}
	if pageH > 0 {
		if pos < c.vs {
			c.vs = pos
		}
		if pos >= c.vs+pageH {
			c.vs = pos - pageH + 1
		}
		if c.vs < 0 {
			c.vs = 0
		}
	}
	c.cursor = pos
}

// window returns the [start,end) slice of item indices visible in a pageH-high
// viewport over n items.
func (c *listCursor) window(n, pageH int) (start, end int) {
	if n == 0 || pageH <= 0 {
		return 0, 0
	}
	if pageH >= n {
		return 0, n
	}
	start = c.vs
	end = start + pageH
	if end > n {
		end = n
		start = end - pageH
		if start < 0 {
			start = 0
		}
	}
	return start, end
}
