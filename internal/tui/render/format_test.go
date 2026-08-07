package render

import (
	"reflect"
	"testing"
)

// formatDuration is exercised directly (not via SetDurFmt) so the test never
// mutates the package-global ColDuration/activeDurFmt and stays order-independent.
func TestFormatDurationMatrix(t *testing.T) {
	// 3930s = 1h05m30s (totalM 65); 330s = 5m30s (h==0, exercises suppression).
	cases := []struct {
		f            DurFmt
		wantWithHour string // 3930s
		wantNoHour   string // 330s
	}{
		{DurFmtHHMMSS, "01:05:30", "00:05:30"},
		{DurFmthhmmss, "1:05:30", "5:30"},
		{DurFmtHHMM, "01:05", "00:05"},
		{DurFmthHmm, "1:05", "0:05"},
		{DurFmthhmm, "1:05", "5"},
		{DurFmtMMMSS, "065:30", "005:30"},
		{DurFmtmmmss, "65:30", "5:30"},
		{DurFmtMMM, "065", "005"},
		{DurFmtmMM, "65", "05"},
		{DurFmtmmm, "65", "5"},
	}
	for _, c := range cases {
		if got := formatDuration(3930, c.f); got != c.wantWithHour {
			t.Errorf("formatDuration(3930, %q) = %q, want %q", c.f, got, c.wantWithHour)
		}
		if got := formatDuration(330, c.f); got != c.wantNoHour {
			t.Errorf("formatDuration(330, %q) = %q, want %q", c.f, got, c.wantNoHour)
		}
	}
	// Unrecognized format falls back to the hh:mm branch.
	if got := formatDuration(3930, DurFmt("bogus")); got != "1:05" {
		t.Errorf("unknown format fallback = %q, want 1:05", got)
	}
}

func TestFormatDateMatrix(t *testing.T) {
	cases := []struct {
		f    DateFmt
		want string
	}{
		{DateFmtDMY, "21/07/2026"},
		{DateFmtMDY, "07/21/2026"},
		{DateFmtYMD, "2026-07-21"},
		{DateFmtDMYDash, "21-07-2026"},
		{DateFmt("bogus"), "21/07/2026"}, // fallback → DMY
	}
	for _, c := range cases {
		if got := formatDate("20260721", c.f); got != c.want {
			t.Errorf("formatDate(20260721, %q) = %q, want %q", c.f, got, c.want)
		}
	}
}

// Duration is the public wrapper: non-positive durations render blank.
func TestDurationWrapper(t *testing.T) {
	if got := Duration(0); got != "" {
		t.Errorf("Duration(0) = %q, want empty", got)
	}
	if got := Duration(-5); got != "" {
		t.Errorf("Duration(-5) = %q, want empty", got)
	}
	if got := Duration(330); got == "" {
		t.Errorf("Duration(330) unexpectedly blank")
	}
}

func TestViews(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, ""},
		{-1, ""},
		{1, "1"},
		{999, "999"},
		{1000, "1.0K"},
		{1_000_000, "1.0M"},
		{1_000_000_000, "1.0B"},
		{2_500_000_000, "2.5B"},
	}
	for _, c := range cases {
		if got := Views(c.n); got != c.want {
			t.Errorf("Views(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// Date passes non-8-length strings through untouched.
func TestDateWrapper(t *testing.T) {
	if got := Date("2026"); got != "2026" {
		t.Errorf("Date(short) = %q, want passthrough", got)
	}
	if got := Date("20260721"); got == "20260721" {
		t.Errorf("Date(valid) not formatted: %q", got)
	}
}

func TestWordWrap(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		width int
		want  []string
	}{
		{"empty width returns text whole", "hello", 0, []string{"hello"}},
		{"fits on one line", "a b c", 10, []string{"a b c"}},
		{"wraps at word boundary", "aa bb cc", 5, []string{"aa bb", "cc"}},
		{"CRLF normalized to line breaks", "a\r\nb", 10, []string{"a", "b"}},
		{"lone CR normalized", "a\rb", 10, []string{"a", "b"}},
		{"over-long token hard-broken", "abcdefgh", 3, []string{"abc", "def", "gh"}},
		{"CJK double-width hard break", "你好世界", 4, []string{"你好", "世界"}},
		{"blank paragraph preserved", "a\n\nb", 10, []string{"a", "", "b"}},
	}
	for _, c := range cases {
		if got := WordWrap(c.text, c.width); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: WordWrap(%q, %d) = %#v, want %#v", c.name, c.text, c.width, got, c.want)
		}
	}
}

func TestAbbreviateURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://www.example.com/very/long/path", "example.com/…"},
		{"http://example.com", "example.com/…"},
		{"https://sub.example.com/x?y=1", "sub.example.com/…"},
	}
	for _, c := range cases {
		if got := abbreviateURL(c.in); got != c.want {
			t.Errorf("abbreviateURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
