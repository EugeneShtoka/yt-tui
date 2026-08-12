package service

import (
	"context"
	"fmt"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// backfillRepo is the narrow persistence slice the scheduler needs — a subset
// of ChannelRepo. Keeping it separate documents (and limits) exactly what the
// crawler touches, so the scheduler's dependencies don't creep with the wider
// CRUD port (M-2).
type backfillRepo interface {
	GetSubscribedChannels(ctx context.Context) ([]domain.Channel, error)
	SaveChannelVideos(ctx context.Context, channelID string, videos []domain.Video) error
	SetChannelFetchOffset(ctx context.Context, channelID string, offset int64) error
	TouchChannelVideosRefreshed(ctx context.Context, channelID string) error
	StampChannelActivity(ctx context.Context, channelIDs ...string) error
}

// backfillSource fetches back-catalog pages — the only ChannelSource method the
// scheduler uses.
type backfillSource interface {
	ChannelVideosPage(ctx context.Context, channelURL, channelID string, start int) ([]domain.Video, int, bool, error)
}

// latestNFetch refreshes a channel's newest N videos (merge + persist + stamp).
// The scheduler delegates the stale-refresh path to it rather than duplicating
// ChannelService.ChannelLatestN's merge logic — a drill-in and a stale refresh
// must behave identically.
type latestNFetch func(ctx context.Context, channelURL, channelID string, n int) ([]domain.Video, error)

// backfillScheduler owns the round-robin back-catalog crawl and the stale
// latest-N refresh: the startup counterpart to a drill-in fetch. Extracted from
// ChannelService (M-2) so the scheduling algorithm — resume cursors, breadth-first
// rotation, pacing — evolves independently of subscription CRUD.
type backfillScheduler struct {
	repo    backfillRepo
	source  backfillSource
	latestN latestNFetch
}

func newBackfillScheduler(repo backfillRepo, source backfillSource, latestN latestNFetch) *backfillScheduler {
	return &backfillScheduler{repo: repo, source: source, latestN: latestN}
}

// Run populates the video cache for subscribed channels that the user hasn't
// opened yet, the startup counterpart to the drill-in fetch: seeding or
// subscribing a channel registers it but never fetches its videos, so without
// this a freshly-added channel stays empty in the Feed until manually visited.
//
// A channel whose back-catalog isn't fully crawled yet (!FullyCrawled) gets a
// deep crawl that resumes from its stored offset — a never-crawled channel from
// the top, a paused one from where the last run stopped. An already-fully-crawled
// channel that has gone stale (VideosRefreshedAt older than staleAfter) gets only
// the latest latestN, mirroring the drill-in auto-refresh. Fresh channels are
// skipped. staleAfter <= 0 treats every fully-crawled channel as stale.
//
// Best-effort and paced: each per-channel error is logged and the sweep
// continues, and delay is slept between fetches to avoid hammering yt-dlp.
// Honors ctx cancellation. Returns the number of channels fetched.
func (b *backfillScheduler) Run(ctx context.Context, latestN int, staleAfter, delay time.Duration) (int, error) {
	subs, err := b.repo.GetSubscribedChannels(ctx)
	if err != nil {
		return 0, fmt.Errorf("BackfillSubscribed: %w", err)
	}
	now := time.Now().Unix()

	// Partition subscriptions by what they need. A not-yet-fully-crawled channel
	// needs a deep back-catalog crawl (resumed from its offset) — even if a
	// drill-in latest-N already stamped videos_refreshed_at. An already-crawled
	// channel that has gone stale needs only the latest N; a fresh one is left be;
	// a URL-less row can't be fetched.
	var fullCrawl, staleRefresh []*domain.Channel
	for i := range subs {
		ch := &subs[i]
		if ch.URL == "" {
			continue
		}
		switch {
		case !ch.FullyCrawled():
			fullCrawl = append(fullCrawl, ch)
		case staleAfter <= 0 || now-ch.VideosRefreshedAt >= int64(staleAfter.Seconds()):
			staleRefresh = append(staleRefresh, ch)
		}
	}

	fetched := 0
	// Stale latest-N refreshes first: cheap single calls that keep already-loaded
	// channels current before the potentially long full crawls begin.
	for _, ch := range staleRefresh {
		select {
		case <-ctx.Done():
			return fetched, nil
		default:
		}
		if _, ferr := b.latestN(ctx, ch.URL, ch.ID, latestN); ferr != nil {
			debug.Log("ChannelService.BackfillSubscribed: latest-N %s: %v", ch.ID, ferr)
			continue
		}
		fetched++
		if !sleepPaced(ctx, delay) {
			return fetched, nil
		}
	}
	// Then round-robin the full crawls one page per channel per rotation, so a
	// single huge channel can't head-of-line-block the rest.
	return fetched + b.roundRobinFullCrawl(ctx, fullCrawl, delay), nil
}

// roundRobinFullCrawl crawls each channel's back-catalog breadth-first — one page
// per channel per rotation — so every channel receives its newest page before any
// channel receives its second, and no single large channel starves the others.
// Each channel resumes from its stored offset, and that offset is persisted after
// every saved page so an interrupted sweep continues where it left off next run
// rather than restarting from the top. A channel leaves the rotation once fully
// crawled (a short final page → stamped complete) or on a page error (best-effort,
// logged; its offset is preserved for retry). Honors ctx cancellation and paces
// yt-dlp calls by delay. Returns the number of channels fully crawled.
func (b *backfillScheduler) roundRobinFullCrawl(ctx context.Context, channels []*domain.Channel, delay time.Duration) int {
	type cursor struct {
		ch      *domain.Channel
		start   int
		initial int // start this run began at; distinguishes a real tail from a soft miss
	}
	active := make([]*cursor, len(channels))
	for i, ch := range channels {
		s := int(ch.ResumeOffset()) + 1
		active[i] = &cursor{ch: ch, start: s, initial: s}
	}
	completed := 0
	for len(active) > 0 {
		var next []*cursor
		for _, cur := range active {
			if ctx.Err() != nil {
				return completed
			}
			page, nextStart, more, err := b.source.ChannelVideosPage(ctx, cur.ch.URL, cur.ch.ID, cur.start)
			if err != nil {
				debug.Log("ChannelService.BackfillSubscribed: %s @%d: %v", cur.ch.ID, cur.start, err)
				continue // drop from rotation; resumes from its saved offset next run
			}
			if len(page) > 0 {
				if serr := b.repo.SaveChannelVideos(ctx, cur.ch.ID, page); serr != nil {
					// A save failure must not advance the offset, or the page is skipped
					// for good. Keep this cursor's offset and retry the same page next run.
					debug.Log("ChannelService.BackfillSubscribed: save %s: %v", cur.ch.ID, serr)
					_ = sleepPaced(ctx, delay)
					continue
				}
			}
			switch {
			case more:
				// Persist the resume cursor (list positions consumed) before moving
				// on, so a crash between rotations doesn't lose this page's progress.
				if serr := b.repo.SetChannelFetchOffset(ctx, cur.ch.ID, int64(nextStart-1)); serr != nil {
					debug.Log("ChannelService.BackfillSubscribed: offset %s: %v", cur.ch.ID, serr)
				}
				cur.start = nextStart
				next = append(next, cur)
			case len(page) > 0 || cur.start > cur.initial || cur.ch.ResumeOffset() > 0:
				// Genuine end of catalog: a short non-empty page, an empty page reached
				// after advancing this run, or an empty page on a channel already partly
				// crawled. Stamp complete so future runs only latest-N it.
				b.stampCrawlComplete(ctx, cur.ch.ID)
				completed++
			default:
				// Empty page on the very first fetch of a never-crawled channel. yt-dlp
				// is run in partial-success mode, so a soft failure (rate-limit,
				// cold cookies, region hiccup) returns an empty page with no error —
				// indistinguishable here from a truly empty catalog. Treating it as
				// "fully crawled" would freeze the channel at the sentinel and lose its
				// back-catalog forever (the "stuck at latest-N" bug). Do NOT stamp it:
				// drop from the rotation and retry next run. A genuinely empty channel
				// simply retries — cheap and paced, and far better than silent data loss.
				debug.Log("ChannelService.BackfillSubscribed: %s empty first page, deferring completion", cur.ch.ID)
			}
			if !sleepPaced(ctx, delay) {
				return completed
			}
		}
		active = next
	}
	return completed
}

// stampCrawlComplete records a finished deep crawl: videos refreshed + fetch
// offset set to the "fully crawled" sentinel + activity, mirroring ChannelVideos'
// completion stamps so backfill only latest-N's the channel from now on. Errors
// are logged, not fatal.
func (b *backfillScheduler) stampCrawlComplete(ctx context.Context, channelID string) {
	if err := b.repo.TouchChannelVideosRefreshed(ctx, channelID); err != nil {
		debug.Log("ChannelService.BackfillSubscribed: touch %s: %v", channelID, err)
	}
	if err := b.repo.SetChannelFetchOffset(ctx, channelID, domain.FetchOffsetComplete); err != nil {
		debug.Log("ChannelService.BackfillSubscribed: complete %s: %v", channelID, err)
	}
	if err := b.repo.StampChannelActivity(ctx, channelID); err != nil {
		debug.Log("ChannelService.BackfillSubscribed: stamp %s: %v", channelID, err)
	}
}

// sleepPaced sleeps for delay unless ctx is canceled first. It returns false if
// ctx was canceled (the caller should stop), true otherwise. A non-positive
// delay only checks cancellation.
func sleepPaced(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(delay):
		return true
	}
}
