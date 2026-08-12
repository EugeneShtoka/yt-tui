package config

import "testing"

func TestDefaultStaleSettings(t *testing.T) {
	cfg := defaultConfig()
	if cfg.HideStaleTaggedChannels {
		t.Error("HideStaleTaggedChannels should default off")
	}
	if cfg.StaleTaggedChannelDays != 30 {
		t.Errorf("StaleTaggedChannelDays default = %d, want 30", cfg.StaleTaggedChannelDays)
	}
}

func TestStaleDaysNonPositiveResetsToDefault(t *testing.T) {
	cfg := loadFromTOML(t, "stale_tagged_channel_days = 0\n")
	if cfg.StaleTaggedChannelDays != 30 {
		t.Errorf("non-positive days = %d, want reset to 30", cfg.StaleTaggedChannelDays)
	}
	cfg = loadFromTOML(t, "stale_tagged_channel_days = -5\n")
	if cfg.StaleTaggedChannelDays != 30 {
		t.Errorf("negative days = %d, want reset to 30", cfg.StaleTaggedChannelDays)
	}
	cfg = loadFromTOML(t, "stale_tagged_channel_days = 14\n")
	if cfg.StaleTaggedChannelDays != 14 {
		t.Errorf("valid days = %d, want 14 preserved", cfg.StaleTaggedChannelDays)
	}
}

func TestStaleIsAValidPanelMode(t *testing.T) {
	for _, typ := range []string{"feed", "channels", "tags"} {
		if !validPanelMode(typ, "stale") {
			t.Errorf("validPanelMode(%q, \"stale\") = false, want true", typ)
		}
	}
	// A "stale" default mode survives applyDerivedDefaults for each tab.
	cfg := loadFromTOML(t, "feed_mode = \"stale\"\nchannels_view = \"stale\"\ntags_mode = \"stale\"\n")
	if cfg.FeedMode != "stale" || cfg.ChannelsView != "stale" || cfg.TagsMode != "stale" {
		t.Errorf("stale mode not preserved: feed=%q channels=%q tags=%q", cfg.FeedMode, cfg.ChannelsView, cfg.TagsMode)
	}
}

func TestHideStaleParsesFromTOML(t *testing.T) {
	cfg := loadFromTOML(t, "hide_stale_tagged_channels = true\n")
	if !cfg.HideStaleTaggedChannels {
		t.Error("hide_stale_tagged_channels = true not parsed")
	}
}
