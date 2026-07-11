package ui

import (
	"strings"
	"testing"
)

// Pure feed/media logic (merge, filter, sort, SponsorBlock math, link
// extraction) moved to internal/feed and internal/media in P5 item #2; their
// tests live alongside them. What remains here is ui-local: command completion
// and the viewport-scroll helpers.

// cmdCompletionsFor tests
func TestCmdCompletionsForEmpty(t *testing.T) {
	result := cmdCompletionsFor("")
	if len(result) == 0 {
		t.Errorf("empty prefix: got no completions")
	}
}

func TestCmdCompletionsForFirstWord(t *testing.T) {
	result := cmdCompletionsFor("t")
	// Should return commands starting with "t"
	found := false
	for _, c := range result {
		if strings.HasPrefix(c, "t") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("first word 't': no completions start with 't'")
	}
}

func TestCmdCompletionsForSecondWord(t *testing.T) {
	result := cmdCompletionsFor("tab ")
	// Should return second-word completions for "tab"
	for _, c := range result {
		if !strings.HasPrefix(c, "tab ") {
			t.Errorf("second word 'tab ': got completion not starting with 'tab ': %s", c)
		}
	}
}

// vsMove boundary cases
func TestVsMoveNZero(t *testing.T) {
	c, vs := vsMove(0, 0, 0, 1, 10, false)
	if c != 0 || vs != 0 {
		t.Errorf("vsMove n=0: got (%d,%d), want (0,0)", c, vs)
	}
}

func TestVsMoveClampHigh(t *testing.T) {
	c, _ := vsMove(8, 0, 10, 5, 10, false)
	if c != 9 {
		t.Errorf("vsMove clamp high: got %d, want 9", c)
	}
}

func TestVsMoveClampLow(t *testing.T) {
	c, _ := vsMove(1, 0, 10, -5, 10, false)
	if c != 0 {
		t.Errorf("vsMove clamp low: got %d, want 0", c)
	}
}

func TestVsMoveCircularWrapForward(t *testing.T) {
	c, _ := vsMove(9, 0, 10, 1, 10, true)
	if c != 0 {
		t.Errorf("vsMove circular wrap forward: got %d, want 0", c)
	}
}

func TestVsMoveCircularWrapBackward(t *testing.T) {
	c, _ := vsMove(0, 0, 10, -1, 10, true)
	if c != 9 {
		t.Errorf("vsMove circular wrap backward: got %d, want 9", c)
	}
}

func TestVsMoveViewportInvariantDown(t *testing.T) {
	// cursor at viewport bottom edge moves down — vs must shift to keep cursor visible
	height := 5
	c, vs := vsMove(4, 0, 20, 1, height, false)
	if c < vs || c >= vs+height {
		t.Errorf("vsMove viewport invariant (down): cursor=%d not in [%d,%d)", c, vs, vs+height)
	}
	if vs != 1 {
		t.Errorf("vsMove viewport invariant (down): vs=%d, want 1", vs)
	}
}

func TestVsMoveViewportInvariantUp(t *testing.T) {
	// cursor at vs, moves up one — vs must shift down to keep cursor visible
	height := 5
	c, vs := vsMove(5, 5, 20, -1, height, false)
	if c < vs || c >= vs+height {
		t.Errorf("vsMove viewport invariant (up): cursor=%d not in [%d,%d)", c, vs, vs+height)
	}
	if vs != 4 {
		t.Errorf("vsMove viewport invariant (up): vs=%d, want 4", vs)
	}
}

// vsPage boundary cases
func TestVsPageUpAtStart(t *testing.T) {
	c, vs := vsPage(0, 0, 20, -1, 5, false)
	if c != 0 || vs != 0 {
		t.Errorf("vsPage up at start: got (%d,%d), want (0,0)", c, vs)
	}
}

func TestVsPageDownAtEnd(t *testing.T) {
	// at last page, page down should not advance past last item
	c, vs := vsPage(15, 15, 20, 1, 5, false)
	if c >= 20 {
		t.Errorf("vsPage down at end: cursor=%d >= n=20", c)
	}
	if vs != 15 {
		t.Errorf("vsPage down at end: vs=%d, want 15", vs)
	}
}

func TestVsPageCircularWrapDown(t *testing.T) {
	// page down from last page wraps to first page (circular)
	_, vs := vsPage(16, 15, 20, 1, 5, true)
	if vs != 0 {
		t.Errorf("vsPage circular wrap down: vs=%d, want 0", vs)
	}
}

func TestVsPageCircularWrapUp(t *testing.T) {
	// page up from first page wraps to last page (circular)
	_, vs := vsPage(0, 0, 20, -1, 5, true)
	if vs != 15 {
		t.Errorf("vsPage circular wrap up: vs=%d, want 15", vs)
	}
}

// vsJump boundary cases
func TestVsJumpNegativeTarget(t *testing.T) {
	c, vs := vsJump(-5, 20, 5)
	if c != 0 || vs != 0 {
		t.Errorf("vsJump negative target: got (%d,%d), want (0,0)", c, vs)
	}
}

func TestVsJumpPastEnd(t *testing.T) {
	c, vs := vsJump(25, 20, 5)
	if c != 19 {
		t.Errorf("vsJump past end: cursor=%d, want 19", c)
	}
	if vs != 15 {
		t.Errorf("vsJump past end: vs=%d, want 15", vs)
	}
}

func TestVsJumpCenterInMiddle(t *testing.T) {
	// target in middle of large list — vs should center around it
	c, vs := vsJump(10, 20, 5)
	if c != 10 {
		t.Errorf("vsJump center: cursor=%d, want 10", c)
	}
	if vs != 8 {
		t.Errorf("vsJump center: vs=%d, want 8 (height/2=2 above target)", vs)
	}
}

func TestVsJumpNearTop(t *testing.T) {
	c, vs := vsJump(1, 20, 5)
	if c != 1 {
		t.Errorf("vsJump near top: cursor=%d, want 1", c)
	}
	if vs != 0 {
		t.Errorf("vsJump near top: vs=%d, want 0", vs)
	}
}

func TestVsJumpNearBottom(t *testing.T) {
	c, vs := vsJump(18, 20, 5)
	if c != 18 {
		t.Errorf("vsJump near bottom: cursor=%d, want 18", c)
	}
	if vs != 15 {
		t.Errorf("vsJump near bottom: vs=%d, want 15", vs)
	}
}
