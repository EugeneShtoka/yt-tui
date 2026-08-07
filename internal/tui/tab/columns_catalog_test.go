package tab

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

// Every buildable panel type offers a non-empty, well-formed column set, and
// each set matches what its constructor actually renders (same builder source).
func TestPanelColumnKeysCoversEveryPanelType(t *testing.T) {
	for _, p := range config.DefaultPanels {
		keys := PanelColumnKeys(p.Type)
		if len(keys) == 0 {
			t.Errorf("panel type %q offers no columns", p.Type)
		}
		// The number index column is a structural default-on column present on
		// every panel's primary list.
		if keys[0] != videotable.KeyNum {
			t.Errorf("panel type %q: first column = %q, want %q", p.Type, keys[0], videotable.KeyNum)
		}
	}
}

func TestPanelColumnKeysUnknownTypeIsNil(t *testing.T) {
	if got := PanelColumnKeys("nosuch"); got != nil {
		t.Errorf("unknown type should offer nil columns, got %v", got)
	}
}

// The feed offers exactly its rendered set (guards catalog/constructor drift).
func TestPanelColumnKeysFeedMatchesBuilder(t *testing.T) {
	got := PanelColumnKeys("feed")
	want := videotable.ColumnKeys(feedColumns())
	if len(got) != len(want) {
		t.Fatalf("feed catalog = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("feed catalog[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
