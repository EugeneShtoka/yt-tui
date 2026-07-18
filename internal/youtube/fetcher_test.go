package youtube

import (
	"strings"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
)

// ── parseVideoLines ───────────────────────────────────────────────────────────

func TestParseVideoLinesKeepsValidVideo(t *testing.T) {
	fixture := `{"id":"vid1","title":"Real Video","channel":"Chan","channel_id":"UC1","duration":123.9,"view_count":1000,"upload_date":"20240101","webpage_url":"https://youtu.be/vid1"}`
	got, _, err := parseVideoLines(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d videos, want 1", len(got))
	}
	v := got[0]
	if v.ID != "vid1" || v.Title != "Real Video" || v.Channel != "Chan" || v.ChannelID != "UC1" {
		t.Errorf("field mapping wrong: %+v", v)
	}
	if v.Duration != 123 { // float64 → int truncation
		t.Errorf("duration = %d, want 123", v.Duration)
	}
	if v.ViewCount != 1000 {
		t.Errorf("viewCount = %d, want 1000", v.ViewCount)
	}
}

func TestParseVideoLinesFiltersBranches(t *testing.T) {
	lines := []string{
		`not json — ignored`,
		`{"id":"","title":"no id","view_count":10}`,                             // empty ID
		`{"id":"x","title":"","view_count":10}`,                                 // empty title
		`{"id":"tab","title":"A Channel","ie_key":"YoutubeTab","view_count":5}`, // channel tab
		`{"id":"pl","title":"A Playlist","_type":"playlist","view_count":5}`,    // playlist
		`{"id":"member","title":"Members only","view_count":0}`,                 // zero views (member-only)
		`{malformed json`, // unmarshal error
		`{"id":"good","title":"Good","view_count":42}`, // the only keeper
	}
	got, _, err := parseVideoLines(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 1 || got[0].ID != "good" {
		t.Fatalf("got %+v, want single video 'good'", got)
	}
}

func TestParseVideoLinesDerivesURLWhenMissing(t *testing.T) {
	fixture := `{"id":"abc","title":"T","view_count":1}`
	got, _, _ := parseVideoLines(strings.NewReader(fixture))
	if len(got) != 1 || got[0].URL != "https://www.youtube.com/watch?v=abc" {
		t.Errorf("derived URL wrong: %+v", got)
	}
}

// ── parseChannelLines ─────────────────────────────────────────────────────────

func TestParseChannelLines(t *testing.T) {
	lines := []string{
		`{"id":"UC1","title":"Channel One","channel_follower_count":500}`,
		`{"id":"","title":"no id"}`, // skipped
		`garbage`,
	}
	got, _, err := parseChannelLines(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d channels, want 1", len(got))
	}
	c := got[0]
	if c.ID != "UC1" || c.Name != "Channel One" || c.Subscribers != 500 {
		t.Errorf("channel mapping wrong: %+v", c)
	}
	if c.URL != "https://www.youtube.com/channel/UC1" {
		t.Errorf("derived channel URL wrong: %q", c.URL)
	}
}

// ── parseMixedLines ───────────────────────────────────────────────────────────

func TestParseMixedLinesRoutesEntries(t *testing.T) {
	lines := []string{
		`{"id":"UCchan","title":"A Channel","ie_key":"YoutubeTab"}`,
		`{"id":"vid","title":"A Video","view_count":9}`,
		`{"id":"pl","title":"A Playlist","_type":"playlist"}`,
		`{"id":"zero","title":"Zero views","view_count":0}`, // dropped: not a channel, 0 views
	}
	channels, videos, err := parseMixedLines(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(channels) != 2 {
		t.Errorf("got %d channels, want 2 (tab + playlist)", len(channels))
	}
	if len(videos) != 1 || videos[0].ID != "vid" {
		t.Errorf("got %+v videos, want single 'vid'", videos)
	}
}

// ── buildArgs (already pure) ──────────────────────────────────────────────────

func TestBuildArgs(t *testing.T) {
	base := &config.Config{}
	args := buildArgs(base, "https://x", 0)
	if args[len(args)-1] != "https://x" {
		t.Errorf("url must be last arg, got %v", args)
	}
	if containsArg(args, "--cookies-from-browser") {
		t.Errorf("no browser configured but --cookies-from-browser present")
	}
	if containsArg(args, "--playlist-end") {
		t.Errorf("limit=0 but --playlist-end present")
	}

	withBrowser := &config.Config{DaemonConfig: config.DaemonConfig{Browser: "firefox"}}
	args = buildArgs(withBrowser, "https://x", 50)
	if !argPairPresent(args, "--cookies-from-browser", "firefox") {
		t.Errorf("browser flag missing/wrong: %v", args)
	}
	if !argPairPresent(args, "--playlist-end", "50") {
		t.Errorf("playlist-end missing/wrong: %v", args)
	}
}

func containsArg(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func argPairPresent(args []string, flag, val string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == val {
			return true
		}
	}
	return false
}

// ── isRateLimited / retryDelay ────────────────────────────────────────────────

func TestIsRateLimited(t *testing.T) {
	limited := []string{
		"ERROR: HTTP Error 429: Too Many Requests",
		"you are being Rate-Limited",
		"hit a rate limit",
	}
	for _, s := range limited {
		if !isRateLimited(s) {
			t.Errorf("isRateLimited(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "ERROR: video unavailable", "network error"} {
		if isRateLimited(s) {
			t.Errorf("isRateLimited(%q) = true, want false", s)
		}
	}
}

func TestRetryDelayIsExponential(t *testing.T) {
	if got := retryDelay(0); got != 5*time.Second {
		t.Errorf("retryDelay(0) = %v, want 5s", got)
	}
	if got := retryDelay(1); got != 10*time.Second {
		t.Errorf("retryDelay(1) = %v, want 10s", got)
	}
	if got := retryDelay(2); got != 20*time.Second {
		t.Errorf("retryDelay(2) = %v, want 20s", got)
	}
}
