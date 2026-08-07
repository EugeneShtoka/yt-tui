package feed

import (
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// FilterByMinDuration removes videos shorter than minSecs seconds.
// Videos with Duration == 0 (unknown) are kept. Pass minSecs <= 0 to skip.
func FilterByMinDuration(videos []domain.Video, minSecs int) []domain.Video {
	if minSecs <= 0 {
		return videos
	}
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		if v.Duration == 0 || v.Duration >= minSecs {
			out = append(out, v)
		}
	}
	return out
}

// FilterByMinViews removes videos with fewer than minViews views.
// Videos with ViewCount == 0 (unknown) are kept. Pass minViews <= 0 to skip.
func FilterByMinViews(videos []domain.Video, minViews int) []domain.Video {
	if minViews <= 0 {
		return videos
	}
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		if v.ViewCount == 0 || v.ViewCount >= int64(minViews) {
			out = append(out, v)
		}
	}
	return out
}

// FilterByAge removes videos whose upload date is older than maxDays.
// Videos with no date are kept.
func FilterByAge(videos []domain.Video, maxDays int) []domain.Video {
	if maxDays <= 0 {
		return videos
	}
	cutoff := time.Now().AddDate(0, 0, -maxDays)
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		if len(v.UploadDate) != 8 {
			out = append(out, v)
			continue
		}
		t, err := time.Parse("20060102", v.UploadDate)
		if err != nil || !t.Before(cutoff) {
			out = append(out, v)
		}
	}
	return out
}

// FilterDownloaded removes videos that are already in the local library.
func FilterDownloaded(videos []domain.Video, local map[string]domain.LocalVideo) []domain.Video {
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		if _, ok := local[v.ID]; !ok {
			out = append(out, v)
		}
	}
	return out
}

// FilterHidden removes videos the user has explicitly hidden from recommended.
func FilterHidden(videos []domain.Video, hidden map[string]bool) []domain.Video {
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		if !hidden[v.ID] {
			out = append(out, v)
		}
	}
	return out
}

// Blocklist is the set of blocked channels used to filter the recommended feed.
// It is a pure projection derived by the service layer from the DB (blocked
// channel rows + the blocked_names side table), keeping this package free of any
// storage dependency. IDs are matched exactly; Names cover legacy name-only
// blocks whose channel ID isn't known yet and are matched case-insensitively.
type Blocklist struct {
	IDs   map[string]bool   // blocked channel IDs
	Names map[string]string // lowercased channel name → original stored name
}

// NewBlocklist builds a Blocklist from raw ID and name slices, indexing names
// case-insensitively while retaining the original spelling for enrichment.
func NewBlocklist(ids, names []string) Blocklist {
	bl := Blocklist{
		IDs:   make(map[string]bool, len(ids)),
		Names: make(map[string]string, len(names)),
	}
	for _, id := range ids {
		if id != "" {
			bl.IDs[id] = true
		}
	}
	for _, n := range names {
		if n != "" {
			bl.Names[strings.ToLower(n)] = n
		}
	}
	return bl
}

// Match reports whether the video's channel is blocked. It matches by ID first
// (exact), then by name (case-insensitive). When the match is name-only it
// returns the original stored name so the caller can resolve it to an ID.
func (b Blocklist) Match(v domain.Video) (blockedName string, matched bool) {
	if v.ChannelID != "" && b.IDs[v.ChannelID] {
		return "", true
	}
	if v.Channel != "" {
		if name, ok := b.Names[strings.ToLower(v.Channel)]; ok {
			return name, true
		}
	}
	return "", false
}

// BlacklistEnrichment records that the name-only block for Name was matched by a
// video carrying ChannelID, so the caller can upgrade it to an ID-keyed block.
// Returning these instead of writing storage keeps FilterBlacklisted pure and
// testable, leaving persistence to the service layer.
type BlacklistEnrichment struct {
	Name      string
	ChannelID string
}

// FilterBlacklisted removes videos whose channel is blocked. It is pure: it
// returns the surviving videos plus any name-only blocks it could enrich with a
// channel ID (deduplicated by name), leaving persistence to the caller.
func FilterBlacklisted(videos []domain.Video, bl Blocklist) ([]domain.Video, []BlacklistEnrichment) {
	out := make([]domain.Video, 0, len(videos))
	var enrich []BlacklistEnrichment
	enriched := make(map[string]bool)
	for _, v := range videos {
		if name, matched := bl.Match(v); matched {
			if name != "" && v.ChannelID != "" && !enriched[name] {
				enrich = append(enrich, BlacklistEnrichment{Name: name, ChannelID: v.ChannelID})
				enriched[name] = true
			}
			continue
		}
		out = append(out, v)
	}
	return out, enrich
}

// FilterSubscribed removes videos whose channel the user is already subscribed
// to (matched by channel ID or, failing that, by lowercased channel name).
func FilterSubscribed(videos []domain.Video, subscribed map[string]bool) []domain.Video {
	if len(subscribed) == 0 {
		return videos
	}
	out := make([]domain.Video, 0, len(videos))
	for _, v := range videos {
		if subscribed[v.ChannelID] {
			continue
		}
		if v.Channel != "" && subscribed["name:"+strings.ToLower(v.Channel)] {
			continue
		}
		out = append(out, v)
	}
	return out
}
