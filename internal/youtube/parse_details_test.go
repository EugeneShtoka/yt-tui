package youtube

import (
	"strings"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
)

// ── parseVideoDetails ─────────────────────────────────────────────────────────

func TestParseVideoDetailsFullObject(t *testing.T) {
	c := &Client{cfg: &config.Config{}}
	fixture := `{
		"id":"vid1","title":"Hello","channel":"Chan","channel_id":"UC1",
		"duration":123.9,"view_count":4200,"upload_date":"20240101",
		"webpage_url":"https://youtu.be/vid1","description":"desc",
		"thumbnail":"https://img/vid1.jpg","channel_follower_count":9000,
		"language":"en",
		"chapters":[{"title":"Intro","start_time":0,"end_time":10}],
		"sponsorblock_chapters":[{"start_time":30.5,"end_time":45}]
	}`
	d, err := c.parseVideoDetails([]byte(fixture))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if d.ID != "vid1" || d.Title != "Hello" || d.Channel != "Chan" || d.ChannelID != "UC1" {
		t.Errorf("core fields wrong: %+v", d.Video)
	}
	if d.Duration != 123 { // float64 → int truncation
		t.Errorf("duration = %d, want 123", d.Duration)
	}
	if d.ViewCount != 4200 || d.UploadDate != "20240101" || d.URL != "https://youtu.be/vid1" {
		t.Errorf("metadata wrong: %+v", d.Video)
	}
	if d.Description != "desc" || d.ThumbnailURL != "https://img/vid1.jpg" || d.Subscribers != 9000 {
		t.Errorf("detail fields wrong: desc=%q thumb=%q subs=%d", d.Description, d.ThumbnailURL, d.Subscribers)
	}
	if d.Language != "en" {
		t.Errorf("language = %q, want en", d.Language)
	}
	if len(d.Chapters) != 1 || d.Chapters[0].Title != "Intro" || d.Chapters[0].EndTime != 10 {
		t.Errorf("chapters mapped wrong: %+v", d.Chapters)
	}
	if len(d.SBSegments) != 1 || d.SBSegments[0].Start != 30.5 || d.SBSegments[0].End != 45 {
		t.Errorf("sponsorblock segments mapped wrong: %+v", d.SBSegments)
	}
}

func TestParseVideoDetailsDerivesURLFromID(t *testing.T) {
	c := &Client{cfg: &config.Config{}}
	d, err := c.parseVideoDetails([]byte(`{"id":"abc","title":"T"}`)) // no webpage_url
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if d.URL != "https://www.youtube.com/watch?v=abc" {
		t.Errorf("URL should be derived from id, got %q", d.URL)
	}
}

func TestParseVideoDetailsMalformedJSON(t *testing.T) {
	c := &Client{cfg: &config.Config{}}
	if _, err := c.parseVideoDetails([]byte(`{not json`)); err == nil {
		t.Error("expected an error for malformed JSON")
	}
}

func TestParseVideoDetailsStripsEmojisWhenConfigured(t *testing.T) {
	c := &Client{cfg: &config.Config{DaemonConfig: config.DaemonConfig{StripEmojis: true}}}
	d, err := c.parseVideoDetails([]byte(`{"id":"v","title":"Cats 🐱 rule"}`))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if strings.ContainsRune(d.Title, '🐱') {
		t.Errorf("emoji not stripped: %q", d.Title)
	}
}

// ── parsePlaylistLines ────────────────────────────────────────────────────────

func TestParsePlaylistLines(t *testing.T) {
	lines := strings.Join([]string{
		`not json — ignored`,
		`{"id":"","title":"no id","_type":"playlist"}`,          // empty id → dropped
		`{"id":"PL1","title":"Music","_type":"playlist"}`,       // playlist by _type
		`{"id":"PL2","title":"Tab List","ie_key":"YoutubeTab"}`, // playlist by ie_key
		`{"id":"PL3","_type":"playlist"}`,                       // no title → falls back to id
		`{"id":"vid","title":"A Video","view_count":5}`,         // not a playlist → dropped
	}, "\n")

	got, raw, err := parsePlaylistLines(strings.NewReader(lines))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if raw != 3 || len(got) != 3 {
		t.Fatalf("got %d playlists (raw %d), want 3", len(got), raw)
	}
	byID := map[string]string{}
	for _, p := range got {
		byID[p.ID] = p.Title
	}
	if byID["PL1"] != "Music" || byID["PL2"] != "Tab List" {
		t.Errorf("titles wrong: %v", byID)
	}
	if byID["PL3"] != "PL3" {
		t.Errorf("missing title should fall back to id, got %q", byID["PL3"])
	}
}
