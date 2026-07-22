package videotable

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	etable "github.com/evertras/bubble-table/table"
)

var testKeys = keymap.KeyMap{
	Up:         key.NewBinding(key.WithKeys("k")),
	Down:       key.NewBinding(key.WithKeys("j")),
	PageUp:     key.NewBinding(key.WithKeys("u")),
	PageDown:   key.NewBinding(key.WithKeys("d")),
	GotoPrefix: key.NewBinding(key.WithKeys("g")),
	GotoBottom: key.NewBinding(key.WithKeys("G")),
	GotoLine:   key.NewBinding(key.WithKeys("enter")),
}

func press(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: text}
}

func makeNav(rowCount int, circular bool, overhead int) TableNav {
	type testRow struct{}
	cols := []ColumnDef[testRow]{
		{Col: etable.NewColumn("k", "H", 10), Cell: func(_ testRow, _ int) any { return "" }},
	}
	tbl := NewTable(cols)
	rows := make([]etable.Row, rowCount)
	for i := range rows {
		rows[i] = etable.NewRow(etable.RowData{"k": ""})
	}
	tbl = tbl.WithRows(rows)
	nav := NewTableNav(tbl, circular, overhead)
	nav.Resize(80, 24)
	return nav
}

func TestTableNavDown(t *testing.T) {
	tests := []struct {
		name      string
		start     int
		rowCount  int
		circular  bool
		wantIndex int
		wantOk    bool
	}{
		{"down from 0", 0, 5, false, 1, true},
		{"down from middle", 2, 5, false, 3, true},
		{"down at last non-circular", 4, 5, false, 4, true},
		{"down at last circular wraps", 4, 5, true, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nav := makeNav(tc.rowCount, tc.circular, 2)
			nav.GotoRow(tc.start)
			ok := nav.HandleNav(press("j"), testKeys, tc.rowCount)
			if ok != tc.wantOk {
				t.Errorf("HandleNav returned %v, want %v", ok, tc.wantOk)
			}
			if got := nav.Index(); got != tc.wantIndex {
				t.Errorf("index = %d, want %d", got, tc.wantIndex)
			}
		})
	}
}

func TestTableNavUp(t *testing.T) {
	tests := []struct {
		name      string
		start     int
		rowCount  int
		circular  bool
		wantIndex int
		wantOk    bool
	}{
		{"up from 1", 1, 5, false, 0, true},
		{"up from middle", 3, 5, false, 2, true},
		{"up at 0 non-circular stays", 0, 5, false, 0, true},
		{"up at 0 circular wraps", 0, 5, true, 4, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nav := makeNav(tc.rowCount, tc.circular, 2)
			nav.GotoRow(tc.start)
			ok := nav.HandleNav(press("k"), testKeys, tc.rowCount)
			if ok != tc.wantOk {
				t.Errorf("HandleNav returned %v, want %v", ok, tc.wantOk)
			}
			if got := nav.Index(); got != tc.wantIndex {
				t.Errorf("index = %d, want %d", got, tc.wantIndex)
			}
		})
	}
}

func TestTableNavCircular(t *testing.T) {
	t.Run("down wraps to 0", func(t *testing.T) {
		nav := makeNav(3, true, 2)
		nav.GotoRow(2)
		nav.HandleNav(press("j"), testKeys, 3)
		if got := nav.Index(); got != 0 {
			t.Errorf("index = %d, want 0", got)
		}
	})

	t.Run("up wraps to last", func(t *testing.T) {
		nav := makeNav(3, true, 2)
		nav.GotoRow(0)
		nav.HandleNav(press("k"), testKeys, 3)
		if got := nav.Index(); got != 2 {
			t.Errorf("index = %d, want 2", got)
		}
	})
}

func TestTableNavEmpty(t *testing.T) {
	t.Run("down on empty no panic", func(t *testing.T) {
		nav := makeNav(0, false, 2)
		ok := nav.HandleNav(press("j"), testKeys, 0)
		if !ok {
			t.Error("HandleNav should return true for a recognized key even on empty list")
		}
		if got := nav.Index(); got != 0 {
			t.Errorf("index = %d, want 0", got)
		}
	})

	t.Run("up on empty no panic", func(t *testing.T) {
		nav := makeNav(0, false, 2)
		ok := nav.HandleNav(press("k"), testKeys, 0)
		if !ok {
			t.Error("HandleNav should return true for a recognized key even on empty list")
		}
	})

	t.Run("G on empty no panic", func(t *testing.T) {
		nav := makeNav(0, false, 2)
		ok := nav.HandleNav(press("G"), testKeys, 0)
		if !ok {
			t.Error("HandleNav should return true for G even on empty list")
		}
	})
}

func TestTableNavPageDown(t *testing.T) {
	// overhead=2, height=24 → pageH=22
	tests := []struct {
		name      string
		start     int
		rowCount  int
		wantIndex int
	}{
		{"page down clamps to last when not enough rows", 0, 20, 19},
		{"page down advances by pageH when rows available", 0, 30, 22},
		{"page down from middle clamps", 10, 25, 24},
		{"page down from middle advances", 0, 50, 22},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nav := makeNav(tc.rowCount, false, 2)
			nav.GotoRow(tc.start)
			ok := nav.HandleNav(press("d"), testKeys, tc.rowCount)
			if !ok {
				t.Error("HandleNav should return true for PageDown")
			}
			if got := nav.Index(); got != tc.wantIndex {
				t.Errorf("index = %d, want %d", got, tc.wantIndex)
			}
		})
	}
}

func TestTableNavPageUp(t *testing.T) {
	// overhead=2, height=24 → pageH=22
	tests := []struct {
		name      string
		start     int
		rowCount  int
		wantIndex int
	}{
		{"page up from bottom clamps to 0 when not enough rows", 19, 20, 0},
		{"page up from middle clamps to 0", 10, 30, 0},
		{"page up from far enough advances by pageH", 30, 50, 8},
		{"page up already at 0 stays", 0, 20, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nav := makeNav(tc.rowCount, false, 2)
			nav.GotoRow(tc.start)
			ok := nav.HandleNav(press("u"), testKeys, tc.rowCount)
			if !ok {
				t.Error("HandleNav should return true for PageUp")
			}
			if got := nav.Index(); got != tc.wantIndex {
				t.Errorf("index = %d, want %d", got, tc.wantIndex)
			}
		})
	}
}

func TestTableNavGotoTop(t *testing.T) {
	t.Run("first g sets gotoTopActive returns true index unchanged", func(t *testing.T) {
		nav := makeNav(10, false, 2)
		nav.GotoRow(5)
		ok := nav.HandleNav(press("g"), testKeys, 10)
		if !ok {
			t.Error("first g should return true")
		}
		if got := nav.Index(); got != 5 {
			t.Errorf("index = %d, want 5 (unchanged after first g)", got)
		}
		if !nav.gotoTopActive {
			t.Error("gotoTopActive should be true after first g")
		}
	})

	t.Run("second g goes to row 0", func(t *testing.T) {
		nav := makeNav(10, false, 2)
		nav.GotoRow(5)
		nav.HandleNav(press("g"), testKeys, 10)
		ok := nav.HandleNav(press("g"), testKeys, 10)
		if !ok {
			t.Error("second g should return true")
		}
		if got := nav.Index(); got != 0 {
			t.Errorf("index = %d, want 0", got)
		}
		if nav.gotoTopActive {
			t.Error("gotoTopActive should be false after gg")
		}
	})

	t.Run("g then other key resets gotoTopActive and processes new key", func(t *testing.T) {
		nav := makeNav(10, false, 2)
		nav.GotoRow(5)
		nav.HandleNav(press("g"), testKeys, 10)
		// Press "j" (Down) — should reset gotoTopActive and move down
		nav.HandleNav(press("j"), testKeys, 10)
		if nav.gotoTopActive {
			t.Error("gotoTopActive should be reset after non-g key")
		}
		if got := nav.Index(); got != 6 {
			t.Errorf("index = %d, want 6 (moved down from 5)", got)
		}
	})

	t.Run("g then unrecognized key resets gotoTopActive", func(t *testing.T) {
		nav := makeNav(10, false, 2)
		nav.GotoRow(3)
		nav.HandleNav(press("g"), testKeys, 10)
		ok := nav.HandleNav(press("x"), testKeys, 10)
		if ok {
			t.Error("unrecognized key should return false")
		}
		if nav.gotoTopActive {
			t.Error("gotoTopActive should be reset after unrecognized key")
		}
		if got := nav.Index(); got != 3 {
			t.Errorf("index = %d, want 3 (unchanged)", got)
		}
	})
}

func TestTableNavGotoBottom(t *testing.T) {
	tests := []struct {
		name      string
		start     int
		rowCount  int
		wantIndex int
	}{
		{"G from row 0 goes to last", 0, 5, 4},
		{"G from middle goes to last", 2, 5, 4},
		{"G already at last stays", 4, 5, 4},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nav := makeNav(tc.rowCount, false, 2)
			nav.GotoRow(tc.start)
			ok := nav.HandleNav(press("G"), testKeys, tc.rowCount)
			if !ok {
				t.Error("G should return true")
			}
			if got := nav.Index(); got != tc.wantIndex {
				t.Errorf("index = %d, want %d", got, tc.wantIndex)
			}
		})
	}

	t.Run("G on empty list no-op", func(t *testing.T) {
		nav := makeNav(0, false, 2)
		ok := nav.HandleNav(press("G"), testKeys, 0)
		if !ok {
			t.Error("G should return true even on empty list")
		}
	})
}

func TestTableNavGotoLine(t *testing.T) {
	t.Run("digits then Enter goes to 1-based line", func(t *testing.T) {
		nav := makeNav(20, false, 2)
		nav.HandleNav(press("1"), testKeys, 20)
		nav.HandleNav(press("2"), testKeys, 20)
		ok := nav.HandleNav(press("enter"), testKeys, 20)
		if !ok {
			t.Error("Enter should return true")
		}
		if got := nav.Index(); got != 11 {
			t.Errorf("index = %d, want 11 (line 12, 0-based)", got)
		}
	})

	t.Run("single digit then Enter", func(t *testing.T) {
		nav := makeNav(10, false, 2)
		nav.HandleNav(press("5"), testKeys, 10)
		nav.HandleNav(press("enter"), testKeys, 10)
		if got := nav.Index(); got != 4 {
			t.Errorf("index = %d, want 4", got)
		}
	})

	t.Run("line number 0 is ignored", func(t *testing.T) {
		nav := makeNav(10, false, 2)
		nav.GotoRow(3)
		nav.HandleNav(press("0"), testKeys, 10)
		nav.HandleNav(press("enter"), testKeys, 10)
		// numBuf="0", lineNum=0, lineNum > 0 check fails → no move
		if got := nav.Index(); got != 3 {
			t.Errorf("index = %d, want 3 (line 0 ignored)", got)
		}
	})

	t.Run("Enter with empty numBuf goes to last row", func(t *testing.T) {
		nav := makeNav(10, false, 2)
		nav.GotoRow(0)
		ok := nav.HandleNav(press("enter"), testKeys, 10)
		if !ok {
			t.Error("Enter should return true")
		}
		if got := nav.Index(); got != 9 {
			t.Errorf("index = %d, want 9 (last row)", got)
		}
	})

	t.Run("Enter with empty numBuf on empty list no-op", func(t *testing.T) {
		nav := makeNav(0, false, 2)
		ok := nav.HandleNav(press("enter"), testKeys, 0)
		if !ok {
			t.Error("Enter should return true")
		}
	})

	t.Run("digits then G ignores numBuf and goes to bottom", func(t *testing.T) {
		nav := makeNav(10, false, 2)
		nav.HandleNav(press("1"), testKeys, 10)
		nav.HandleNav(press("G"), testKeys, 10)
		if got := nav.Index(); got != 9 {
			t.Errorf("index = %d, want 9 (G ignores numBuf)", got)
		}
	})

	t.Run("line number exceeds rowCount clamps to last", func(t *testing.T) {
		nav := makeNav(5, false, 2)
		nav.HandleNav(press("9"), testKeys, 5)
		nav.HandleNav(press("9"), testKeys, 5)
		nav.HandleNav(press("enter"), testKeys, 5)
		// lineNum=99, WithHighlightedRow clamps internally
		// bubble-table clamps to rowCount-1
		if got := nav.Index(); got > 4 {
			t.Errorf("index = %d, want <= 4", got)
		}
	})
}

func TestTableNavNumBufRestore(t *testing.T) {
	t.Run("digits then unrecognized key restores numBuf", func(t *testing.T) {
		nav := makeNav(10, false, 2)
		nav.HandleNav(press("5"), testKeys, 10)
		ok := nav.HandleNav(press("x"), testKeys, 10)
		if ok {
			t.Error("unrecognized key should return false")
		}
		if nav.numBuf != "5" {
			t.Errorf("numBuf = %q, want %q", nav.numBuf, "5")
		}
	})

	t.Run("multi-digit then unrecognized key restores full numBuf", func(t *testing.T) {
		nav := makeNav(10, false, 2)
		nav.HandleNav(press("4"), testKeys, 10)
		nav.HandleNav(press("2"), testKeys, 10)
		ok := nav.HandleNav(press("x"), testKeys, 10)
		if ok {
			t.Error("unrecognized key should return false")
		}
		if nav.numBuf != "42" {
			t.Errorf("numBuf = %q, want %q", nav.numBuf, "42")
		}
	})

	t.Run("digits then navigation key clears numBuf", func(t *testing.T) {
		nav := makeNav(10, false, 2)
		nav.HandleNav(press("3"), testKeys, 10)
		nav.HandleNav(press("j"), testKeys, 10) // Down — recognized but not goto
		if nav.numBuf != "" {
			t.Errorf("numBuf = %q, want empty after navigation key", nav.numBuf)
		}
	})
}

func TestTableNavReturnValues(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		wantOk bool
	}{
		{"down consumed", "j", true},
		{"up consumed", "k", true},
		{"page down consumed", "d", true},
		{"page up consumed", "u", true},
		{"G consumed", "G", true},
		{"g consumed", "g", true},
		{"enter consumed", "enter", true},
		{"digit consumed", "5", true},
		{"unrecognized x not consumed", "x", false},
		{"unrecognized z not consumed", "z", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nav := makeNav(5, false, 2)
			ok := nav.HandleNav(press(tc.key), testKeys, 5)
			if ok != tc.wantOk {
				t.Errorf("HandleNav(%q) = %v, want %v", tc.key, ok, tc.wantOk)
			}
		})
	}
}
