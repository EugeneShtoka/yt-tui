package domain

// Link is a URL extracted from a video description, with optional label text
// that appeared before the URL on the same line.
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Chapter is a timed section from a video's chapter list with both original and
// SponsorBlock-adjusted timestamps. SponsorBlock chapters are excluded; chapters
// whose boundaries coincide with a SponsorBlock segment (±3 s) are also dropped.
type Chapter struct {
	Title         string  `json:"title"`
	OriginalStart float64 `json:"original_start"`
	OriginalEnd   float64 `json:"original_end"`
	AdjustedStart float64 `json:"adjusted_start"`
	AdjustedEnd   float64 `json:"adjusted_end"`
}

// SBSegment is a SponsorBlock time range in the original video timeline.
// Stored alongside chapters and used to convert local-file positions to original
// timeline positions (and back) for unified cross-mode resume.
type SBSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// CachedDetails holds the cached metadata for a video fetched from yt-dlp.
type CachedDetails struct {
	Description  string
	ThumbnailURL string
	Subscribers  int64
	Links        *[]Link      // nil = never parsed; &[]Link{} = parsed, none found
	Chapters     *[]Chapter   // nil = none available; populated from yt-dlp metadata
	SBSegments   *[]SBSegment // nil = none; SponsorBlock cut ranges in original timeline
}
