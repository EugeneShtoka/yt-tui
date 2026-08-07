// Package library owns the local (downloaded) video collection.
package library

import (
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
)

// Library owns the downloaded-video slice together with its by-ID lookup index,
// keeping the two in sync. Several tabs reload the library after a DB change
// (download-complete, delete from Local/Downloading/History, clear downloads,
// tab refresh); routing them all through Set fixes a latent bug where some sites
// reassigned the slice but forgot to rebuild the index, leaving stale
// "is downloaded" lookups (used by the recommended filter, the play paths, and
// the video-list "downloaded" badge).
//
// Held by value on the Model and mutated through pointer methods, so changes
// persist across Bubble Tea's value-copy of the Model (like feed.Feed).
type Library struct {
	videos []domain.LocalVideo
	byID   map[string]domain.LocalVideo
}

// New builds a Library from an initial slice, indexing it.
func New(videos []domain.LocalVideo) Library {
	var l Library
	l.Set(videos)
	return l
}

// Set replaces the collection and rebuilds the by-ID index atomically. This is
// the single reload path every DB-mutating site funnels through.
func (l *Library) Set(videos []domain.LocalVideo) {
	l.videos = videos
	l.byID = make(map[string]domain.LocalVideo, len(videos))
	for i := range videos {
		l.byID[videos[i].ID] = videos[i]
	}
}

// Clear empties the library.
func (l *Library) Clear() { l.Set(nil) }

func (l *Library) Videos() []domain.LocalVideo { return l.videos }
func (l *Library) Len() int                    { return len(l.videos) }

// ByID returns the local video with the given ID, if it is downloaded.
func (l *Library) ByID(id string) (domain.LocalVideo, bool) {
	v, ok := l.byID[id]
	return v, ok
}

// Has reports whether a video ID is in the library.
func (l *Library) Has(id string) bool {
	_, ok := l.byID[id]
	return ok
}

// IDs returns the by-ID index for read-only use (e.g. feed.FilterDownloaded,
// which takes the map). Callers must not mutate the returned map.
func (l *Library) IDs() map[string]domain.LocalVideo { return l.byID }

// Sort orders the collection in place by the given mode.
func (l *Library) Sort(mode int) { feed.SortLocalVideos(l.videos, mode) }
