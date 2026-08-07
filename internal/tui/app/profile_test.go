package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
)

// sourceConfig is a fully-populated config whose portable fields all differ
// from a freshly-defaulted config, so a round-trip can prove each one is
// carried. Machine-local fields are set to recognizable values so we can prove
// they are NOT carried.
func sourceConfig() *config.Config {
	return &config.Config{
		ClientConfig: config.ClientConfig{
			// portable — display / UI
			Theme:                   "gruvbox",
			HintMode:                "minimal",
			DurationFormat:          "mm:ss",
			TranscriptWidth:         "80",
			CircularNav:             true,
			FeedMode:                "mixed",
			ChannelsView:            "blocked",
			TagsMode:                "recommended",
			HideStaleTaggedChannels: true,
			StaleTaggedChannelDays:  45,
			Panels:                  []config.Panel{{Name: "feed", Type: "feed", Mode: "mixed", Sort: "date"}},
			Keybindings: config.KeyBindings{
				Download: "z", Quit: "Q",
				SortKeys: config.SortKeys{Date: "1"},
				TabKeys:  map[string]string{"f": "feed"},
			},
			// machine-local — must NOT be carried
			Player:        "vlc",
			PlayerBackend: "custom",
			DaemonAddr:    "https://daemon:9999",
			DaemonToken:   "sekret",
			TLSCACert:     "/etc/ca.pem",
		},
		DaemonConfig: config.DaemonConfig{
			// portable — daemon preferences
			SponsorBlock:               true,
			SponsorBlockCats:           []string{"sponsor", "intro"},
			AudioFormat:                "opus",
			RecommendedMaxAgeDays:      14,
			RecommendedMinDurationSecs: 60,
			RecommendedMinViews:        1000,
			RecommendedFetchCount:      200,
			RecommendedMaxPages:        5,
			ChannelLatestCount:         7,
			ChannelRefreshMinutes:      120,
			ChannelStrikes:             4,
			EnrichmentDelaySeconds:     3,
			ThumbnailsPerChannel:       10,
			StripEmojis:                true,
			Subtitles:                  true,
			SubtitleLangs:              []string{"en", "ru"},
			SaveTranscript:             true,
			// machine-local — must NOT be carried
			DownloadDir:   "/mnt/src/videos",
			Browser:       "firefox",
			CookiesFile:   "/home/src/cookies.txt",
			MaxDownloads:  9,
			TLSCert:       "/etc/src.crt",
			TLSKey:        "/etc/src.key",
			Token:         "daemon-token",
			ThumbnailsDir: "/src/thumbs",
		},
	}
}

// TestConfigProfileRoundTrip proves a full overwrite: every portable field is
// carried from source → profile → JSON → applied onto a target, while the
// target's machine-local fields survive untouched.
func TestConfigProfileRoundTrip(t *testing.T) {
	src := sourceConfig()

	// Marshal/unmarshal to simulate the bundle round-trip.
	raw, err := json.Marshal(newConfigProfile(src))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got configProfile
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Target has its own, distinct machine-local wiring that must be preserved.
	target := &config.Config{}
	target.Player = "mpv"
	target.DownloadDir = "/mnt/target/videos"
	target.DaemonAddr = "http://localhost:8080"
	target.TLSCACert = "/target/ca.pem"
	target.MaxDownloads = 2

	got.applyTo(target)

	// Portable fields now equal the source.
	if target.Theme != "gruvbox" || target.HintMode != "minimal" || target.DurationFormat != "mm:ss" {
		t.Errorf("display prefs not applied: %+v", target.ClientConfig)
	}
	if target.FeedMode != "mixed" || target.ChannelsView != "blocked" || target.TagsMode != "recommended" {
		t.Errorf("mode prefs not applied: feed=%q chans=%q tags=%q", target.FeedMode, target.ChannelsView, target.TagsMode)
	}
	if !target.CircularNav || target.CloseOnLinkOpen {
		t.Errorf("bool prefs not applied: circular=%v link=%v", target.CircularNav, target.CloseOnLinkOpen)
	}
	if !target.HideStaleTaggedChannels || target.StaleTaggedChannelDays != 45 {
		t.Errorf("stale prefs not applied")
	}
	if len(target.Panels) != 1 || target.Panels[0].Mode != "mixed" || target.Panels[0].Sort != "date" {
		t.Errorf("panels not applied: %+v", target.Panels)
	}
	if target.Keybindings.Download != "z" || target.Keybindings.Quit != "Q" ||
		target.Keybindings.SortKeys.Date != "1" || target.Keybindings.TabKeys["f"] != "feed" {
		t.Errorf("keybindings not applied: %+v", target.Keybindings)
	}
	if !target.SponsorBlock || target.AudioFormat != "opus" || len(target.SponsorBlockCats) != 2 {
		t.Errorf("sponsorblock prefs not applied")
	}
	if target.RecommendedMaxAgeDays != 14 || target.RecommendedFetchCount != 200 || target.ChannelStrikes != 4 {
		t.Errorf("recommended/channel tuning not applied")
	}
	if len(target.SubtitleLangs) != 2 || !target.SaveTranscript || !target.StripEmojis {
		t.Errorf("subtitle/transcript prefs not applied")
	}

	// Machine-local fields on the target are untouched.
	if target.Player != "mpv" || target.DownloadDir != "/mnt/target/videos" ||
		target.DaemonAddr != "http://localhost:8080" || target.TLSCACert != "/target/ca.pem" ||
		target.MaxDownloads != 2 {
		t.Errorf("machine-local fields were overwritten: player=%q dir=%q addr=%q ca=%q max=%d",
			target.Player, target.DownloadDir, target.DaemonAddr, target.TLSCACert, target.MaxDownloads)
	}
}

// TestConfigProfileExcludesMachineLocal proves the serialized profile carries no
// machine-local values, so a shared bundle never leaks a box's local wiring
// (paths, tokens, addresses).
func TestConfigProfileExcludesMachineLocal(t *testing.T) {
	raw, err := json.Marshal(newConfigProfile(sourceConfig()))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	blob := string(raw)
	for _, leaked := range []string{
		"vlc", "https://daemon:9999", "sekret", "/etc/ca.pem", "/mnt/src/videos",
		"firefox", "/home/src/cookies.txt", "/etc/src.crt", "/etc/src.key",
		"daemon-token", "/src/thumbs",
	} {
		if strings.Contains(blob, leaked) {
			t.Errorf("machine-local value %q leaked into profile JSON: %s", leaked, blob)
		}
	}
}

// TestConfigProfileSlicesCopied proves applyTo does not alias the profile's
// backing slices/maps into the config, so a later mutation of one can't corrupt
// the other.
func TestConfigProfileSlicesCopied(t *testing.T) {
	p := newConfigProfile(sourceConfig())
	target := &config.Config{}
	p.applyTo(target)

	// Mutating the config's slice must not reach back into the profile.
	target.SubtitleLangs[0] = "de"
	if p.SubtitleLangs[0] == "de" {
		t.Error("SubtitleLangs aliased between profile and config")
	}
	target.Panels[0].Name = "changed"
	if p.Panels[0].Name == "changed" {
		t.Error("Panels aliased between profile and config")
	}
	target.Keybindings.TabKeys["x"] = "y"
	if _, ok := p.Keybindings.TabKeys["x"]; ok {
		t.Error("TabKeys aliased between profile and config")
	}
}
