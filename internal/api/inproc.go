//nolint:wrapcheck // pass-through adapter; errors from backend/db/yt are already contextual
package api

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/backend/enrich"
	"github.com/EugeneShtoka/yt-tui/internal/backend/profiles"
	"github.com/EugeneShtoka/yt-tui/internal/backend/service"
	"github.com/EugeneShtoka/yt-tui/internal/backend/thumbs"
	"github.com/EugeneShtoka/yt-tui/internal/backend/transcripts"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	"golang.org/x/sync/singleflight"
)

// InProc implements Backend by calling services and db/youtube/downloader directly.
// It is a thin adapter — all business logic lives in the service layer. Its
// methods are split across inproc_{feed,channel,video,library,playlist,
// history,download}.go, one file per Backend role interface (H-7).
type InProc struct {
	db           *db.DB
	yt           *youtube.Client
	ytAPI        atomic.Pointer[youtube.YTClient] // set by InitYTClient; used for YT playlist mutations
	cfg          *config.Config
	dl           *downloader.Downloader
	thumbs       *thumbs.Store      // on-disk thumbnail cache; nil if the dir couldn't be created
	transcripts  *transcripts.Store // on-disk transcript store; nil if the dir couldn't be created
	profiles     *profiles.Store    // daemon-stored named config profiles; nil if the dir couldn't be created
	feed         *service.FeedService
	ch           *service.ChannelService
	port         *service.PortabilityService
	enrichDone   chan struct{}  // closed when the StartBackgroundEnrichment goroutine exits; nil until started
	transcriptWG sync.WaitGroup // tracks fire-and-forget transcript-note builds so shutdown can drain them
	bgWG         sync.WaitGroup // tracks other detached maintenance goroutines (e.g. thumbnail recrop) so shutdown can drain them
	// transcriptSF collapses concurrent transcript-note builds for the same video
	// into one yt-dlp fetch. Opening a fresh video otherwise fires the interactive
	// GetTranscript build and the background maybeSaveTranscript build at once —
	// a duplicate-request burst YouTube rate-limits (429), which then stalls behind
	// the fetch retry. Keyed by video ID.
	transcriptSF singleflight.Group
}

// NewInProc creates an InProc Backend, wiring services to their adapters.
func NewInProc(database *db.DB, yt *youtube.Client, dl *downloader.Downloader, cfg *config.Config) *InProc {
	store, err := thumbs.NewStore(cfg.ThumbnailsPath())
	if err != nil {
		debug.Log("NewInProc: thumbnail cache disabled: %v", err)
		store = nil
	}
	trStore, err := transcripts.NewStore(cfg.TranscriptsPath())
	if err != nil {
		debug.Log("NewInProc: transcript store disabled: %v", err)
		trStore = nil
	} else if merr := trStore.EnableMarkdown(cfg.TranscriptMarkdownPath()); merr != nil {
		debug.Log("NewInProc: transcript markdown disabled: %v", merr)
	}
	profStore, err := profiles.NewStore(cfg.ProfilesPath())
	if err != nil {
		debug.Log("NewInProc: profile store disabled: %v", err)
		profStore = nil
	}
	return &InProc{
		db:          database,
		yt:          yt,
		cfg:         cfg,
		dl:          dl,
		thumbs:      store,
		transcripts: trStore,
		profiles:    profStore,
		feed:        service.NewFeedService(database, yt, cfg),
		ch:          service.NewChannelService(database, yt),
		port:        service.NewPortabilityService(database, database),
	}
}

// StartBackgroundEnrichment launches the background pass that syncs
// subscriptions, backfills/refreshes channel videos, fetches full details
// (correcting approximate upload dates) and caches thumbnails for subscribed and
// recommended videos. It returns immediately; the pass runs in its own goroutine,
// then repeats every RefreshMinutes, until ctx is canceled. Call once per
// process, after construction. Pair with WaitEnrichment at shutdown.
func (p *InProc) StartBackgroundEnrichment(ctx context.Context) {
	// One-time migration: re-crop thumbnails cached before cropping moved to store
	// time, so the (now crop-free) render path shows clean images. Marker-guarded,
	// so it's a cheap no-op after the first sweep.
	if p.thumbs != nil {
		// Tracked on bgWG (not enrichDone) so it can run concurrently with the
		// enrichment pass without delaying it, while still being drained by
		// WaitEnrichment at shutdown (L-2).
		p.bgWG.Add(1)
		go func() {
			defer p.bgWG.Done()
			if n, err := p.thumbs.Recrop(); err != nil {
				debug.Log("enrich: recrop: %v", err)
			} else if n > 0 {
				debug.Log("enrich: recropped %d cached thumbnails", n)
			}
		}()
	}
	e := enrich.New(p.db, p.yt, p.thumbs, p.transcripts, enrich.Params{
		DelaySeconds:         p.cfg.EnrichmentDelaySeconds,
		ThumbnailsPerChannel: p.cfg.ThumbnailsPerChannel,
		SaveTranscript:       p.cfg.SaveTranscript,
		SubtitleLangs:        p.cfg.SubtitleLangs,
		SponsorBlockCats:     p.cfg.SponsorBlockArg(),
	})
	// Backfill then enrich, in one goroutine and in that order: the backfill
	// populates channel_videos for channels never opened (so the enricher, which
	// only upgrades existing rows, has them to work on). Backfill runs even when
	// enrichment is disabled (e == nil) — an empty Feed is the bug we're fixing.
	// enrichDone lets shutdown join this goroutine (WaitEnrichment) so no
	// background DB write races database.Close().
	//
	// After the initial pass the goroutine repeats it every RefreshMinutes
	// (the same knob that gates staleness): each tick re-syncs subscriptions and
	// re-runs the stale-refresh, so a channel not updated within the window has its
	// latest videos pulled without the user reopening the app. Both passes are
	// resumable/idempotent, so a tick that fires while nothing is stale is cheap.
	p.enrichDone = make(chan struct{})
	go func() {
		defer close(p.enrichDone)
		pass := func() {
			p.backfillSubscribed(ctx)
			if e != nil {
				e.Run(ctx)
			}
		}
		pass()
		// Snapshot under the config lock: this runs on the background goroutine,
		// concurrently with a possible mid-session profile import (H-1).
		interval := time.Duration(p.cfg.DaemonSnapshot().RefreshMinutes) * time.Minute
		if interval <= 0 {
			return // no cadence configured; the one-shot pass above is all we do
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pass()
			}
		}
	}()
}

// WaitEnrichment blocks until every background goroutine this InProc owns has
// exited: the StartBackgroundEnrichment pass (if started), any in-flight
// fire-and-forget transcript-note builds spawned by VideoDetails, and the
// one-time thumbnail recrop maintenance sweep. Callers cancel
// the enrichment context first; this then waits out any live writer so a
// subsequent database.Close() / transcript-store write cannot race it (the
// single-binary shutdown ordering, H-1). Returns immediately when nothing runs.
func (p *InProc) WaitEnrichment() {
	if p.enrichDone != nil {
		<-p.enrichDone
	}
	p.transcriptWG.Wait()
	p.bgWG.Wait()
}

// backfillPace is the sleep between per-channel backfill fetches when no
// enrichment delay is configured — the yt-dlp channel-listing call is the same
// one drill-in makes, so a modest fixed gap is enough to stay polite.
const backfillPace = 2 * time.Second

// backfillSubscribed first pulls the live subscription list from the YT account
// (adding channels newly subscribed on YouTube), then fetches videos for
// subscribed channels that have never been opened (full pull) or have gone stale
// (latest N), so the Feed reflects the account without a manual drill-in. Gated on
// YouTubeEnabled — pointless (and noisy) when YouTube access is off. Paced by
// EnrichmentDelaySeconds, falling back to backfillPace.
func (p *InProc) backfillSubscribed(ctx context.Context) {
	// Snapshot once under the config lock: this runs on the background
	// enrichment goroutine, concurrently with a possible mid-session profile
	// import that overwrites these fields (H-1).
	cfg := p.cfg.DaemonSnapshot()
	if !cfg.YouTubeEnabled {
		return
	}
	// Sync the subscription list from the account before backfilling videos, so a
	// fresh/empty DB seeds itself and new subscriptions appear automatically.
	// Best-effort: a fetch failure (no cookies, no network) leaves the cached list
	// intact and backfill proceeds with whatever is already in the DB.
	if _, err := p.ch.SubscribedChannels(ctx); err != nil {
		debug.Log("enrich: sync subscriptions: %v", err)
	}
	delay := time.Duration(cfg.EnrichmentDelaySeconds) * time.Second
	if delay <= 0 {
		delay = backfillPace
	}
	stale := time.Duration(cfg.RefreshMinutes) * time.Minute
	n, err := p.ch.BackfillSubscribed(ctx, cfg.ChannelLatestCount, stale, delay)
	if err != nil {
		debug.Log("enrich: backfill: %v", err)
	} else if n > 0 {
		debug.Log("enrich: backfilled %d channel(s)", n)
	}
	p.syncWatchLater(ctx)
}

// syncWatchLater refreshes the cached YouTube "WL" playlist so the local Watch
// Later view tracks additions/removals made on YouTube (including on other
// devices). Best-effort and part of the backfill cycle: a fetch failure (no
// cookies, no network, WL unavailable) leaves the cache intact. A genuinely
// empty WL legitimately clears the cache; a fetch error never does.
func (p *InProc) syncWatchLater(ctx context.Context) {
	vids, err := p.yt.PlaylistVideos(ctx, domain.WatchLaterYTID)
	if err != nil {
		debug.Log("enrich: sync watch later: %v", err)
		return
	}
	if err := p.db.SaveYTPlaylistVideos(ctx, domain.WatchLaterYTID, vids); err != nil {
		debug.Log("enrich: save watch later: %v", err)
	}
}
