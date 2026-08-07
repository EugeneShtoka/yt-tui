package config

import "strings"

// SubtitleLangsArg and SponsorBlockArg format DaemonConfig fields into the
// comma-joined argument strings the yt-dlp consumers (downloader, youtube)
// expect. They live on DaemonConfig — the single source both the api and
// downloader packages read — rather than being duplicated in each consumer.

// SubtitleLangsArg joins the configured subtitle languages into yt-dlp's
// comma-separated --sub-langs form (empty when none are configured).
func (c *DaemonConfig) SubtitleLangsArg() string {
	if len(c.SubtitleLangs) == 0 {
		return ""
	}
	return strings.Join(c.SubtitleLangs, ",")
}

// SponsorBlockArg joins the enabled SponsorBlock categories into yt-dlp's
// comma-separated form (empty when SponsorBlock is off or no categories are set).
func (c *DaemonConfig) SponsorBlockArg() string {
	if !c.SponsorBlock || len(c.SponsorBlockCats) == 0 {
		return ""
	}
	out := c.SponsorBlockCats[0]
	for _, cat := range c.SponsorBlockCats[1:] {
		out += "," + cat
	}
	return out
}
