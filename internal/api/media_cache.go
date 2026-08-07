//nolint:wrapcheck // pass-through adapter; inner-provider errors are already contextual
package api

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/backend/thumbs"
	"github.com/EugeneShtoka/yt-tui/internal/backend/transcache"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
)

// cachingMedia is the client-side MediaProvider: an optional local cache (a
// store on *this* machine) in front of an inner provider (the backend — InProc
// in single-binary mode, Remote when connected to a daemon). It is what lets the
// TUI overlay ask for a thumbnail/transcript without ever owning a net/http
// dependency or the CDN URL contract — that all lives here, behind the seam.
//
// Two independent caches meet here:
//   - the local cache (localThumbs): reactive — it keeps every thumbnail you
//     actually view. It lives in its own directory so the daemon/enrichment
//     Retain sweep (which prunes the proactive set to the eligible newest-N)
//     never evicts what you viewed.
//   - the server cache: the daemon's own proactive, eligible-bounded store,
//     reachable only through inner in remote mode. Single-binary has no remote
//     server, so there is no server cache to route through — the machine fetches
//     locally.
//
// Thumbnail egress on a local miss is governed by backendServes: when the
// backend serves thumbnails, ask it (in remote mode this keeps the daemon as the
// single network boundary); when it does not, this machine fetches the CDN
// directly. Either way the bytes are written back to the local cache. Transcripts
// always route through inner — the client can't run yt-dlp — so the local layer
// only caches the display text the backend returns.
type cachingMedia struct {
	inner            MediaProvider
	localThumbs      *thumbs.Store     // client-local thumbnail cache; nil = disabled
	localTranscripts *transcache.Store // client-local transcript-text cache; nil = disabled
	backendServes    bool              // does the backend (inner) serve thumbnails?
}

// NewMediaProvider wraps inner with a client-side cache. localThumbs and
// localTranscripts may be nil (that cache off). backendServes reports whether the
// backend serves thumbnails; when false the client fetches the CDN directly on a
// thumbnail miss.
//
// When there is nothing to add — no local caches and the backend serves
// thumbnails — inner is returned unwrapped, so a remote client with no local
// cache talks to the daemon directly.
func NewMediaProvider(inner MediaProvider, localThumbs *thumbs.Store, localTranscripts *transcache.Store, backendServes bool) MediaProvider {
	if localThumbs == nil && localTranscripts == nil && backendServes {
		return inner
	}
	return &cachingMedia{inner: inner, localThumbs: localThumbs, localTranscripts: localTranscripts, backendServes: backendServes}
}

// GetThumbnail resolves a thumbnail read-through: the local cache first, then the
// backend (or a direct CDN fetch when the backend doesn't serve thumbnails),
// writing whatever it obtains back to the local cache so re-opening any video
// you have viewed is instant.
func (m *cachingMedia) GetThumbnail(ctx context.Context, videoID, fallbackURL string) ([]byte, bool, error) {
	if m.localThumbs != nil {
		if data, ok, err := m.localThumbs.Get(videoID); err == nil && ok {
			return data, true, nil // stored images are already cropped
		}
	}

	data, ok, err := m.fetchThumbnail(ctx, videoID, fallbackURL)
	if err != nil || !ok || len(data) == 0 {
		return nil, false, err
	}

	// Cache everything the user views. Put crops on write, so serve the stored
	// (clean) bytes back on success; a store failure is non-fatal — return what we
	// fetched.
	if m.localThumbs != nil {
		if perr := m.localThumbs.Put(videoID, data); perr != nil {
			debug.Log("cachingMedia: local store %s: %v", videoID, perr)
		} else if stored, sok, gerr := m.localThumbs.Get(videoID); gerr == nil && sok {
			return stored, true, nil
		}
	}
	return data, true, nil
}

// fetchThumbnail obtains bytes for a local-cache miss: through the backend when
// it serves thumbnails, otherwise a direct (store-independent, cropped) CDN
// fetch from this machine.
func (m *cachingMedia) fetchThumbnail(ctx context.Context, videoID, fallbackURL string) ([]byte, bool, error) {
	if m.backendServes {
		return m.inner.GetThumbnail(ctx, videoID, fallbackURL)
	}
	url := fallbackURL
	if url == "" {
		url = thumbs.URLFor(videoID)
	}
	data, err := thumbs.Fetch(ctx, url)
	if err != nil {
		// A direct fetch failure is a soft miss (blank thumbnail), not an error the
		// overlay should surface — mirror the backend's (nil,false,nil) miss contract.
		debug.Log("cachingMedia: direct fetch %s: %v", videoID, err)
		return nil, false, nil
	}
	return data, len(data) > 0, nil
}

// GetTranscript resolves a transcript read-through: the local text cache first,
// then the backend (always — the client can't run yt-dlp), writing the returned
// text back to the local cache. Egress never branches: unlike thumbnails, a
// transcript is only ever the backend's to fetch.
func (m *cachingMedia) GetTranscript(ctx context.Context, videoID, videoURL string) (string, bool, error) {
	if m.localTranscripts != nil {
		if text, ok := m.localTranscripts.Get(videoID); ok {
			return text, true, nil
		}
	}
	text, ok, err := m.inner.GetTranscript(ctx, videoID, videoURL)
	if err != nil || !ok || text == "" {
		return text, ok, err
	}
	if m.localTranscripts != nil {
		if perr := m.localTranscripts.Put(videoID, text); perr != nil {
			debug.Log("cachingMedia: local transcript store %s: %v", videoID, perr)
		}
	}
	return text, true, nil
}
