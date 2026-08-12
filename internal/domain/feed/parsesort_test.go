package feed

import "testing"

func TestParseSortName(t *testing.T) {
	cases := []struct {
		name     string
		wantMode int
		wantOK   bool
	}{
		{"views", SortViews, true},
		{"date", SortDate, true},
		{"name", SortName, true},
		{"none", SortNone, true},
		{"channel", SortChannel, true},
		{"duration", SortDuration, true},
		{"subscribers", SortSubscribers, true},
		{"tags", SortTags, true},
		{"size", SortSize, true},
		{"", 0, false},
		{"bogus", 0, false},
		{"Date", 0, false}, // case-sensitive by design
	}
	for _, c := range cases {
		mode, ok := ParseSortName(c.name)
		if ok != c.wantOK || (ok && mode != c.wantMode) {
			t.Errorf("ParseSortName(%q) = (%d, %v), want (%d, %v)", c.name, mode, ok, c.wantMode, c.wantOK)
		}
	}
}
