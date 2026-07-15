package nav

// Move moves cursor by delta with optional circular wrapping, adjusting viewport start.
func Move(cursor, vs, n, delta, height int, circular bool) (newCursor, newVS int) {
	if n <= 0 {
		return 0, 0
	}
	c := cursor + delta
	if circular {
		c = ((c % n) + n) % n
	} else {
		if c < 0 {
			c = 0
		}
		if c >= n {
			c = n - 1
		}
	}
	if c < vs {
		vs = c
	}
	if c >= vs+height {
		vs = c - height + 1
	}
	if vs < 0 {
		vs = 0
	}
	return c, vs
}

// Page advances one full page in direction (+1 down, -1 up), preserving relative position.
func Page(cursor, vs, n, direction, height int, circular bool) (newCursor, newVS int) {
	relPos := cursor - vs
	newVS = vs + direction*height
	if newVS < 0 {
		if circular && n > 0 {
			newVS = max(0, n-height)
		} else {
			newVS = 0
		}
	}
	if newVS+height > n {
		if circular {
			newVS = 0
		} else {
			newVS = n - height
			if newVS < 0 {
				newVS = 0
			}
		}
	}
	newCursor = newVS + relPos
	if newCursor < 0 {
		newCursor = 0
	}
	if newCursor >= n {
		newCursor = n - 1
	}
	return newCursor, newVS
}

// Jump moves to a target index, centering it in the viewport.
func Jump(target, n, height int) (newCursor, newVS int) {
	c := target
	if c < 0 {
		c = 0
	}
	if c >= n {
		c = n - 1
	}
	vs := c - height/2
	if vs < 0 {
		vs = 0
	}
	if vs+height > n {
		vs = n - height
		if vs < 0 {
			vs = 0
		}
	}
	return c, vs
}

// Window returns [start, end) for the visible slice anchored at vs.
func Window(vs, n, height int) (start, end int) {
	if n == 0 || height <= 0 {
		return 0, 0
	}
	if height >= n {
		return 0, n
	}
	start = vs
	end = start + height
	if end > n {
		end = n
		start = end - height
		if start < 0 {
			start = 0
		}
	}
	return start, end
}
