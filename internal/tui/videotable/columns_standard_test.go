package videotable

import (
	"slices"
	"testing"
)

// TestStandardVideoColumnKeys locks the two shared video column sets so a future
// edit to one call site (or the helper) can't silently drift the layout. The
// 7-col set omits Channel; the with-Channel set is identical plus a Channel
// column inserted right after the title.
func TestStandardVideoColumnKeys(t *testing.T) {
	wantStd := []string{KeyNum, KeyInd, KeyTitle, KeyWatched, KeyDuration, KeyCount, KeyDate}
	if got := ColumnKeys(StandardVideoColumns()); !slices.Equal(got, wantStd) {
		t.Errorf("StandardVideoColumns keys = %v, want %v", got, wantStd)
	}

	wantCh := []string{KeyNum, KeyInd, KeyTitle, KeyChannel, KeyWatched, KeyDuration, KeyCount, KeyDate}
	if got := ColumnKeys(StandardVideoColumnsWithChannel()); !slices.Equal(got, wantCh) {
		t.Errorf("StandardVideoColumnsWithChannel keys = %v, want %v", got, wantCh)
	}

	// With-Channel must be exactly Standard + KeyChannel after KeyTitle.
	std := ColumnKeys(StandardVideoColumns())
	titleIdx := slices.Index(std, KeyTitle)
	withCh := slices.Insert(slices.Clone(std), titleIdx+1, KeyChannel)
	if got := ColumnKeys(StandardVideoColumnsWithChannel()); !slices.Equal(got, withCh) {
		t.Errorf("with-Channel set drifted from Standard+Channel: got %v, derived %v", got, withCh)
	}
}
