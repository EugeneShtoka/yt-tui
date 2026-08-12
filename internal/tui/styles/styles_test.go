package styles

import "testing"

func TestDarkenHex(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		factor float64
		want   string
	}{
		{"half", "#7a7ab8", 0.5, "#3d3d5c"},
		{"zero is black", "#ffffff", 0, "#000000"},
		{"identity", "#123456", 1, "#123456"},
		{"uppercase input", "#7A7AB8", 0.5, "#3d3d5c"},
		{"malformed returns unchanged", "blue", 0.5, "blue"},
		{"short hex returns unchanged", "#fff", 0.5, "#fff"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := darkenHex(c.in, c.factor); got != c.want {
				t.Errorf("darkenHex(%q, %v) = %q, want %q", c.in, c.factor, got, c.want)
			}
		})
	}
}
