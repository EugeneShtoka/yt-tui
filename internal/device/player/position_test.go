package player

import "testing"

func TestParsePositionUs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		value  any
		wantUs int64
		wantOk bool
	}{
		{"valid microseconds", int64(1_500_000), 1_500_000, true},
		{"zero", int64(0), 0, true},
		{"negative rejected", int64(-1), 0, false},
		{"wrong type uint32", uint32(1000), 0, false},
		{"wrong type string", "1000", 0, false},
		{"wrong type float64", float64(1000), 0, false},
		{"nil value", nil, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotUs, gotOk := parsePositionUs(tt.value)
			if gotUs != tt.wantUs || gotOk != tt.wantOk {
				t.Errorf("parsePositionUs(%v) = (%d, %v), want (%d, %v)", tt.value, gotUs, gotOk, tt.wantUs, tt.wantOk)
			}
		})
	}
}
