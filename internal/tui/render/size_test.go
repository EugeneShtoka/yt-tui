package render

import "testing"

func TestSize(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{0, ""},        // unknown → blank, not "0"
		{-1, ""},       // defensive: negatives render blank
		{512, "512B"},  // sub-KiB
		{1536, "1.5K"}, // 1.5 KiB
		{5 * 1 << 20, "5.0M"},
		{3 * 1 << 30, "3.0G"},
	}
	for _, c := range cases {
		if got := Size(c.bytes); got != c.want {
			t.Errorf("Size(%d) = %q, want %q", c.bytes, got, c.want)
		}
	}
}
