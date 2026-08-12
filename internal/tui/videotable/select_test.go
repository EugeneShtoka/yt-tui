package videotable

import (
	"reflect"
	"testing"
)

// fullCols is a representative column set exercising the shared factories.
func fullCols() []ColumnDef[VideoData] {
	return []ColumnDef[VideoData]{
		NumCol[VideoData](), IndicatorCol[VideoData](), TitleFlexCol[VideoData](),
		ChannelCol[VideoData](), DurationCol[VideoData](), ViewsCol[VideoData](), DateCol[VideoData](),
	}
}

func TestColumnKeys(t *testing.T) {
	got := ColumnKeys(fullCols())
	want := []string{KeyNum, KeyInd, KeyTitle, KeyChannel, KeyDuration, KeyCount, KeyDate}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ColumnKeys = %v, want %v", got, want)
	}
}

func TestSelectColumnsEmptyReturnsAll(t *testing.T) {
	all := fullCols()
	got := SelectColumns(all, nil)
	if !reflect.DeepEqual(ColumnKeys(got), ColumnKeys(all)) {
		t.Errorf("empty wantKeys should return all columns; got %v", ColumnKeys(got))
	}
	if got := SelectColumns(all, []string{}); !reflect.DeepEqual(ColumnKeys(got), ColumnKeys(all)) {
		t.Errorf("[] wantKeys should return all columns; got %v", ColumnKeys(got))
	}
}

func TestSelectColumnsFiltersAndReorders(t *testing.T) {
	got := SelectColumns(fullCols(), []string{KeyTitle, KeyNum, KeyDate})
	want := []string{KeyTitle, KeyNum, KeyDate}
	if !reflect.DeepEqual(ColumnKeys(got), want) {
		t.Errorf("SelectColumns = %v, want %v (filter + reorder)", ColumnKeys(got), want)
	}
}

func TestSelectColumnsSkipsUnknownKeys(t *testing.T) {
	got := SelectColumns(fullCols(), []string{KeyTitle, "ghost", KeyViewsUnknownProbe, KeyNum})
	want := []string{KeyTitle, KeyNum}
	if !reflect.DeepEqual(ColumnKeys(got), want) {
		t.Errorf("SelectColumns should skip unknown keys; got %v, want %v", ColumnKeys(got), want)
	}
}

// A selection made up entirely of unknown keys falls back to "show all" rather
// than rendering an empty table.
func TestSelectColumnsAllUnknownFallsBackToAll(t *testing.T) {
	all := fullCols()
	got := SelectColumns(all, []string{"ghost", "phantom"})
	if !reflect.DeepEqual(ColumnKeys(got), ColumnKeys(all)) {
		t.Errorf("all-unknown selection should fall back to all; got %v", ColumnKeys(got))
	}
}

// KeyViewsUnknownProbe is a key that never appears in fullCols, used to prove
// unknown keys are skipped. Declared here (not as a real column key) on purpose.
const KeyViewsUnknownProbe = "not-a-real-key"
