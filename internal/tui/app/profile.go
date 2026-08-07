package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/config"
)

// configProfile is the portable subset of the config that travels in an export
// bundle (portability.Bundle.Config, carried as opaque JSON). It captures the
// client-side presentation config (keybindings, panel layout, display prefs)
// plus the portable daemon preferences (feed tuning, sponsorblock, subtitles).
//
// Machine-local fields are deliberately excluded so applying a profile never
// overwrites a box's local wiring: player binary/backend, download dir, cookies,
// browser, TLS/cert paths, daemon addr/token, artifact dirs, and download
// concurrency all stay put. Applying a profile is a full overwrite of exactly
// the fields below (see applyTo) — no field-level merge; mismatches are the
// user's to reconcile afterward.
type configProfile struct {
	// ── display / UI (ClientConfig) ──
	Theme                   string             `json:"theme,omitempty"`
	HintMode                string             `json:"hint_mode"`
	DurationFormat          string             `json:"duration_format"`
	TranscriptWidth         string             `json:"transcript_width"`
	CloseOnLinkOpen         bool               `json:"close_on_link_open"`
	CircularNav             bool               `json:"circular_nav"`
	FeedMode                string             `json:"feed_mode"`
	ChannelsView            string             `json:"channels_view"`
	TagsMode                string             `json:"tags_mode"`
	HideStaleTaggedChannels bool               `json:"hide_stale_tagged_channels"`
	StaleTaggedChannelDays  int                `json:"stale_tagged_channel_days"`
	Panels                  []config.Panel     `json:"panels,omitempty"`
	Keybindings             config.KeyBindings `json:"keybindings"`

	// ── portable daemon preferences (DaemonConfig) ──
	SponsorBlock               bool     `json:"sponsorblock"`
	SponsorBlockCats           []string `json:"sponsorblock_categories,omitempty"`
	AudioFormat                string   `json:"audio_format,omitempty"`
	RecommendedMaxAgeDays      int      `json:"recommended_max_age_days"`
	RecommendedMinDurationSecs int      `json:"recommended_min_duration_secs"`
	RecommendedMinViews        int      `json:"recommended_min_views"`
	RecommendedFetchCount      int      `json:"recommended_fetch_count"`
	RecommendedMaxPages        int      `json:"recommended_max_pages"`
	ChannelLatestCount         int      `json:"channel_latest_count"`
	ChannelRefreshMinutes      int      `json:"channel_refresh_minutes"`
	ChannelStrikes             int      `json:"channel_strikes"`
	EnrichmentDelaySeconds     int      `json:"enrichment_delay_seconds"`
	ThumbnailsPerChannel       int      `json:"thumbnails_per_channel"`
	StripEmojis                bool     `json:"strip_emojis"`
	Subtitles                  bool     `json:"subtitles"`
	SubtitleLangs              []string `json:"subtitle_langs,omitempty"`
	SaveTranscript             bool     `json:"save_transcript"`
}

// newConfigProfile snapshots the portable fields of cfg. Slices and maps are
// deep-copied so the profile can be mutated (or the config re-saved) without
// aliasing the live config.
func newConfigProfile(cfg *config.Config) configProfile {
	return configProfile{
		Theme:                   cfg.Theme,
		HintMode:                cfg.HintMode,
		DurationFormat:          cfg.DurationFormat,
		TranscriptWidth:         cfg.TranscriptWidth,
		CloseOnLinkOpen:         cfg.CloseOnLinkOpen,
		CircularNav:             cfg.CircularNav,
		FeedMode:                cfg.FeedMode,
		ChannelsView:            cfg.ChannelsView,
		TagsMode:                cfg.TagsMode,
		HideStaleTaggedChannels: cfg.HideStaleTaggedChannels,
		StaleTaggedChannelDays:  cfg.StaleTaggedChannelDays,
		Panels:                  clonePanels(cfg.Panels),
		Keybindings:             cloneKeybindings(cfg.Keybindings),

		SponsorBlock:               cfg.SponsorBlock,
		SponsorBlockCats:           cloneStrings(cfg.SponsorBlockCats),
		AudioFormat:                cfg.AudioFormat,
		RecommendedMaxAgeDays:      cfg.RecommendedMaxAgeDays,
		RecommendedMinDurationSecs: cfg.RecommendedMinDurationSecs,
		RecommendedMinViews:        cfg.RecommendedMinViews,
		RecommendedFetchCount:      cfg.RecommendedFetchCount,
		RecommendedMaxPages:        cfg.RecommendedMaxPages,
		ChannelLatestCount:         cfg.ChannelLatestCount,
		ChannelRefreshMinutes:      cfg.ChannelRefreshMinutes,
		ChannelStrikes:             cfg.ChannelStrikes,
		EnrichmentDelaySeconds:     cfg.EnrichmentDelaySeconds,
		ThumbnailsPerChannel:       cfg.ThumbnailsPerChannel,
		StripEmojis:                cfg.StripEmojis,
		Subtitles:                  cfg.Subtitles,
		SubtitleLangs:              cloneStrings(cfg.SubtitleLangs),
		SaveTranscript:             cfg.SaveTranscript,
	}
}

// applyTo overwrites cfg's portable fields from the profile, leaving every
// machine-local field untouched. Slices and maps are deep-copied so cfg never
// shares backing storage with the (transient) profile. Callers should follow
// with cfg.Normalize() to re-validate before saving.
func (p configProfile) applyTo(cfg *config.Config) {
	cfg.Theme = p.Theme
	cfg.HintMode = p.HintMode
	cfg.DurationFormat = p.DurationFormat
	cfg.TranscriptWidth = p.TranscriptWidth
	cfg.CloseOnLinkOpen = p.CloseOnLinkOpen
	cfg.CircularNav = p.CircularNav
	cfg.FeedMode = p.FeedMode
	cfg.ChannelsView = p.ChannelsView
	cfg.TagsMode = p.TagsMode
	cfg.HideStaleTaggedChannels = p.HideStaleTaggedChannels
	cfg.StaleTaggedChannelDays = p.StaleTaggedChannelDays
	cfg.Panels = clonePanels(p.Panels)
	cfg.Keybindings = cloneKeybindings(p.Keybindings)

	cfg.SponsorBlock = p.SponsorBlock
	cfg.SponsorBlockCats = cloneStrings(p.SponsorBlockCats)
	cfg.AudioFormat = p.AudioFormat
	cfg.RecommendedMaxAgeDays = p.RecommendedMaxAgeDays
	cfg.RecommendedMinDurationSecs = p.RecommendedMinDurationSecs
	cfg.RecommendedMinViews = p.RecommendedMinViews
	cfg.RecommendedFetchCount = p.RecommendedFetchCount
	cfg.RecommendedMaxPages = p.RecommendedMaxPages
	cfg.ChannelLatestCount = p.ChannelLatestCount
	cfg.ChannelRefreshMinutes = p.ChannelRefreshMinutes
	cfg.ChannelStrikes = p.ChannelStrikes
	cfg.EnrichmentDelaySeconds = p.EnrichmentDelaySeconds
	cfg.ThumbnailsPerChannel = p.ThumbnailsPerChannel
	cfg.StripEmojis = p.StripEmojis
	cfg.Subtitles = p.Subtitles
	cfg.SubtitleLangs = cloneStrings(p.SubtitleLangs)
	cfg.SaveTranscript = p.SaveTranscript
}

// MarshalConfigProfile snapshots cfg's portable fields as a profile blob — the
// opaque JSON that both the export bundle (Bundle.Config) and a daemon-stored
// profile carry. Keeping this the single encoder keeps the on-the-wire profile
// format single-sourced here in the client.
func MarshalConfigProfile(cfg *config.Config) ([]byte, error) {
	data, err := json.Marshal(newConfigProfile(cfg))
	if err != nil {
		return nil, fmt.Errorf("encode config profile: %w", err)
	}
	return data, nil
}

// ApplyConfigProfile decodes a profile blob and overwrites cfg's portable
// fields wholesale (machine-local fields stay put), then re-normalizes. It does
// not persist — callers Save() when they want the change durable. It is the
// single decoder counterpart to MarshalConfigProfile, shared by bundle-import
// and daemon-profile-on-connect so both paths stay in lockstep.
func ApplyConfigProfile(cfg *config.Config, data []byte) error {
	var p configProfile
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("decode config profile: %w", err)
	}
	p.applyTo(cfg)
	cfg.Normalize()
	return nil
}

// LoadProfileOnConnect fetches the named daemon profile and applies it over cfg
// in place. It runs before the TUI starts so keybindings and panels are read
// from the profile's values (no restart needed, unlike a mid-session import).
//
// An empty name or a not-yet-existing profile is a silent no-op — the local
// config stands — so pointing several clients at a daemon profile that isn't
// saved yet is harmless. Applying is in-memory only: the daemon stays the
// source of truth, so each connect re-applies rather than rewriting the
// client's on-disk config. Only a genuine transport/decode failure errors.
func LoadProfileOnConnect(ctx context.Context, backend api.ProfileBackend, cfg *config.Config, name string) error {
	if name == "" {
		return nil
	}
	data, found, err := backend.GetProfile(ctx, name)
	if err != nil {
		return fmt.Errorf("load profile %q: %w", name, err)
	}
	if !found {
		return nil
	}
	return ApplyConfigProfile(cfg, data)
}

func cloneStrings(s []string) []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s...)
}

func clonePanels(p []config.Panel) []config.Panel {
	if p == nil {
		return nil
	}
	return append([]config.Panel(nil), p...)
}

// cloneKeybindings copies a KeyBindings value, deep-copying its TabKeys map (the
// only reference field — the nested key structs are value types copied by the
// struct assignment).
func cloneKeybindings(kb config.KeyBindings) config.KeyBindings {
	if kb.TabKeys != nil {
		tk := make(map[string]string, len(kb.TabKeys))
		for k, v := range kb.TabKeys {
			tk[k] = v
		}
		kb.TabKeys = tk
	}
	return kb
}
