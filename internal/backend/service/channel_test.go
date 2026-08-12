package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

type fakeChannelRepo struct {
	subs       []domain.Channel
	subsErr    error
	saveErr    error
	cached     []domain.Video
	cachedErr  error
	saveVidErr error
	savedPages [][]domain.Video // each SaveChannelVideos call, in order
	// call trackers for the block/state transitions
	blocked        string
	unblocked      string
	deletedVideos  string
	stateChangedID string
	stateChangedTo domain.SubscriptionState
	stamped        []string
	fetchOffsets   []int64 // each SetChannelFetchOffset call, in order
}

func (f *fakeChannelRepo) GetSubscribedChannels(ctx context.Context) ([]domain.Channel, error) {
	return f.subs, f.subsErr
}
func (f *fakeChannelRepo) AllChannels(ctx context.Context) ([]domain.Channel, error) {
	return f.subs, f.subsErr
}
func (f *fakeChannelRepo) BlockedChannels(ctx context.Context) ([]domain.Channel, error) {
	return f.subs, f.subsErr
}
func (f *fakeChannelRepo) SaveSubscribedChannels(context.Context, []domain.Channel) error {
	return f.saveErr
}
func (f *fakeChannelRepo) AddSubscribedChannel(context.Context, domain.Channel) error { return nil }
func (f *fakeChannelRepo) RemoveSubscribedChannel(context.Context, string) error      { return nil }
func (f *fakeChannelRepo) SetChannelState(ctx context.Context, id string, state domain.SubscriptionState) error {
	f.stateChangedID, f.stateChangedTo = id, state
	return nil
}
func (f *fakeChannelRepo) BlockChannel(ctx context.Context, id string) error {
	f.blocked = id
	return nil
}
func (f *fakeChannelRepo) UnblockChannel(ctx context.Context, id string) error {
	f.unblocked = id
	return nil
}
func (f *fakeChannelRepo) DeleteChannelVideos(ctx context.Context, id string) error {
	f.deletedVideos = id
	return nil
}
func (f *fakeChannelRepo) GetChannelVideos(context.Context, string) ([]domain.Video, error) {
	return f.cached, f.cachedErr
}
func (f *fakeChannelRepo) SaveChannelVideos(ctx context.Context, _ string, videos []domain.Video) error {
	f.savedPages = append(f.savedPages, videos)
	return f.saveVidErr
}
func (f *fakeChannelRepo) TouchChannelVideosRefreshed(context.Context, string) error { return nil }
func (f *fakeChannelRepo) SetChannelFetchOffset(ctx context.Context, _ string, offset int64) error {
	f.fetchOffsets = append(f.fetchOffsets, offset)
	return nil
}
func (f *fakeChannelRepo) StampChannelActivity(ctx context.Context, ids ...string) error {
	f.stamped = append(f.stamped, ids...)
	return nil
}

type fakeChannelSource struct {
	subs   []domain.Channel
	videos []domain.Video
	err    error
}

func (f fakeChannelSource) SubscribedChannels(context.Context) ([]domain.Channel, error) {
	return f.subs, f.err
}
func (f fakeChannelSource) ChannelVideosStream(_ context.Context, _, _ string, onPage func([]domain.Video) error) ([]domain.Video, error) {
	if f.err == nil && onPage != nil && len(f.videos) > 0 {
		if err := onPage(f.videos); err != nil {
			return nil, err
		}
	}
	return f.videos, f.err
}
func (f fakeChannelSource) ChannelVideosPage(_ context.Context, _, _ string, start int) ([]domain.Video, int, bool, error) {
	if start > 1 {
		return nil, start, false, f.err
	}
	return f.videos, start + 1, false, f.err
}
func (f fakeChannelSource) ChannelLatestN(_ context.Context, _, _ string, _ int) ([]domain.Video, error) {
	return f.videos, f.err
}

// recordingSource records which fetch path each channel took, so the backfill
// test can assert full-pull vs latest-N routing per channel.
type recordingSource struct {
	full   []string
	latest []string
}

func (r *recordingSource) SubscribedChannels(context.Context) ([]domain.Channel, error) {
	return nil, nil
}
func (r *recordingSource) ChannelVideosStream(_ context.Context, _, id string, _ func([]domain.Video) error) ([]domain.Video, error) {
	r.full = append(r.full, id)
	return nil, nil
}

// ChannelVideosPage is the path backfill's round-robin full crawl now takes. It
// returns one real video with more=false, so each full-crawl channel completes
// in a single page and is recorded once — the routing assertions stay page-count
// agnostic. (A non-empty page is required: an empty first page is now treated as
// a transient miss and deferred, not counted as a completed crawl.)
func (r *recordingSource) ChannelVideosPage(_ context.Context, _, id string, start int) ([]domain.Video, int, bool, error) {
	r.full = append(r.full, id)
	return []domain.Video{{ID: id + "-1"}}, start + 1, false, nil
}
func (r *recordingSource) ChannelLatestN(_ context.Context, _, id string, _ int) ([]domain.Video, error) {
	r.latest = append(r.latest, id)
	return nil, nil
}

// TestChannelServiceBackfillSubscribedRouting is the core of the empty-channel
// bug fix: the deep crawl is keyed on FullyCrawled (not VideosRefreshedAt), so a
// channel that was only ever drilled into (latest-N stamped VideosRefreshedAt but
// never fully crawled) still gets its full catalog. A fully-crawled+stale one
// gets latest-N, a fully-crawled+fresh one is skipped, and a URL-less row is
// ignored.
func TestChannelServiceBackfillSubscribedRouting(t *testing.T) {
	now := time.Now().Unix()
	staleTS := now - int64((2 * time.Hour).Seconds())
	repo := &fakeChannelRepo{subs: []domain.Channel{
		{ID: "empty", URL: "u/empty", VideosRefreshedAt: 0, FetchedVideos: 0},
		// Drilled-into: latest-N stamped VideosRefreshedAt, but never fully crawled.
		// The bug was this being skipped/latest-N'd; it must get a full crawl.
		{ID: "drilled", URL: "u/drilled", VideosRefreshedAt: now, FetchedVideos: 0},
		{ID: "stale", URL: "u/stale", VideosRefreshedAt: staleTS, FetchedVideos: domain.FetchOffsetComplete},
		{ID: "fresh", URL: "u/fresh", VideosRefreshedAt: now, FetchedVideos: domain.FetchOffsetComplete},
		{ID: "nourl", URL: "", VideosRefreshedAt: 0, FetchedVideos: 0},
	}}
	src := &recordingSource{}
	n, err := NewChannelService(repo, src).BackfillSubscribed(context.Background(), 5, time.Hour, 0)
	if err != nil {
		t.Fatalf("BackfillSubscribed: %v", err)
	}
	if n != 3 {
		t.Errorf("fetched = %d, want 3 (empty + drilled + stale)", n)
	}
	if len(src.full) != 2 || src.full[0] != "empty" || src.full[1] != "drilled" {
		t.Errorf("full pulls = %v, want [empty drilled]", src.full)
	}
	if len(src.latest) != 1 || src.latest[0] != "stale" {
		t.Errorf("latest-N pulls = %v, want [stale]", src.latest)
	}
}

// interleaveSource serves a fixed number of pages per channel id and records the
// (id, page) fetch order, so the round-robin backfill can be asserted to advance
// breadth-first (one page per channel per rotation) rather than draining each
// channel before the next. It uses a page width of 1 (start doubles as the page
// number), independent of the real yt-dlp pageSize.
type interleaveSource struct {
	pagesPer map[string]int // channel id -> total pages
	calls    []string       // "id#page" in call order
}

func (s *interleaveSource) SubscribedChannels(context.Context) ([]domain.Channel, error) {
	return nil, nil
}
func (s *interleaveSource) ChannelVideosStream(context.Context, string, string, func([]domain.Video) error) ([]domain.Video, error) {
	return nil, nil
}
func (s *interleaveSource) ChannelLatestN(context.Context, string, string, int) ([]domain.Video, error) {
	return nil, nil
}
func (s *interleaveSource) ChannelVideosPage(_ context.Context, _, id string, start int) ([]domain.Video, int, bool, error) {
	s.calls = append(s.calls, fmt.Sprintf("%s#%d", id, start))
	return []domain.Video{{ID: fmt.Sprintf("%s-%d", id, start)}}, start + 1, start < s.pagesPer[id], nil
}

// TestChannelServiceBackfillRoundRobin is the core of the equal-fill change: a
// small channel and a big one are both never-fully-pulled, and the crawl must
// advance breadth-first — the small channel's only page arrives before the big
// channel's second page — so one huge channel can't starve the rest.
func TestChannelServiceBackfillRoundRobin(t *testing.T) {
	repo := &fakeChannelRepo{subs: []domain.Channel{
		{ID: "small", URL: "u/small", FetchedVideos: 0},
		{ID: "big", URL: "u/big", FetchedVideos: 0},
	}}
	src := &interleaveSource{pagesPer: map[string]int{"small": 1, "big": 3}}
	n, err := NewChannelService(repo, src).BackfillSubscribed(context.Background(), 5, time.Hour, 0)
	if err != nil {
		t.Fatalf("BackfillSubscribed: %v", err)
	}
	if n != 2 {
		t.Errorf("fetched = %d, want 2 (both channels fully crawled)", n)
	}
	want := []string{"small#1", "big#1", "big#2", "big#3"}
	if !reflect.DeepEqual(src.calls, want) {
		t.Errorf("fetch order = %v, want %v (breadth-first)", src.calls, want)
	}
}

// pagedResumeSource serves `pages` single-item pages for one channel and records
// the start offset each page was requested at, so resume-from-offset and the
// completion sentinel can be asserted.
type pagedResumeSource struct {
	pages  int
	starts []int
}

func (s *pagedResumeSource) SubscribedChannels(context.Context) ([]domain.Channel, error) {
	return nil, nil
}
func (s *pagedResumeSource) ChannelVideosStream(context.Context, string, string, func([]domain.Video) error) ([]domain.Video, error) {
	return nil, nil
}
func (s *pagedResumeSource) ChannelLatestN(context.Context, string, string, int) ([]domain.Video, error) {
	return nil, nil
}
func (s *pagedResumeSource) ChannelVideosPage(_ context.Context, _, _ string, start int) ([]domain.Video, int, bool, error) {
	s.starts = append(s.starts, start)
	return []domain.Video{{ID: fmt.Sprintf("v%d", start)}}, start + 1, start < s.pages, nil
}

// TestChannelServiceBackfillResumesFromOffset is the core of the resume change: a
// channel paused mid-crawl (FetchedVideos > 0) continues from its stored offset
// instead of restarting at the top, persists an advancing offset after each
// non-final page, and stamps the fully-crawled sentinel on completion.
func TestChannelServiceBackfillResumesFromOffset(t *testing.T) {
	repo := &fakeChannelRepo{subs: []domain.Channel{
		{ID: "paused", URL: "u/paused", FetchedVideos: 1}, // stopped after the first page
	}}
	src := &pagedResumeSource{pages: 3}
	if _, err := NewChannelService(repo, src).BackfillSubscribed(context.Background(), 5, time.Hour, 0); err != nil {
		t.Fatalf("BackfillSubscribed: %v", err)
	}
	// Resumes at offset+1 = 2 and pages through to the end — never re-fetches 1.
	if want := []int{2, 3}; !reflect.DeepEqual(src.starts, want) {
		t.Errorf("page starts = %v, want %v (resume, not restart)", src.starts, want)
	}
	// Non-final page persists its advancing offset (nextStart-1), completion sets
	// the sentinel — so the next run would skip straight to latest-N.
	if want := []int64{2, domain.FetchOffsetComplete}; !reflect.DeepEqual(repo.fetchOffsets, want) {
		t.Errorf("persisted offsets = %v, want %v", repo.fetchOffsets, want)
	}
}

// emptyPageSource simulates a transient soft-failure: yt-dlp returns an empty
// page with no error (the runner tolerates partial success). It always returns
// an empty page, so it stands in for a channel whose listing momentarily fails.
type emptyPageSource struct{ calls int }

func (s *emptyPageSource) SubscribedChannels(context.Context) ([]domain.Channel, error) {
	return nil, nil
}
func (s *emptyPageSource) ChannelVideosStream(context.Context, string, string, func([]domain.Video) error) ([]domain.Video, error) {
	return nil, nil
}
func (s *emptyPageSource) ChannelLatestN(context.Context, string, string, int) ([]domain.Video, error) {
	return nil, nil
}
func (s *emptyPageSource) ChannelVideosPage(_ context.Context, _, _ string, start int) ([]domain.Video, int, bool, error) {
	s.calls++
	return nil, start, false, nil
}

// TestChannelServiceBackfillDefersCompletionOnEmptyFirstPage is the regression
// guard for the "stuck at latest-N" bug: a never-crawled channel whose first
// page comes back empty (a soft yt-dlp failure, not a real empty catalog) must
// NOT be stamped fully-crawled — doing so froze it at the sentinel and lost its
// back-catalog forever. It is instead dropped from the rotation to retry next run.
func TestChannelServiceBackfillDefersCompletionOnEmptyFirstPage(t *testing.T) {
	repo := &fakeChannelRepo{subs: []domain.Channel{
		{ID: "c", URL: "u/c", FetchedVideos: 0}, // never crawled
	}}
	src := &emptyPageSource{}
	n, err := NewChannelService(repo, src).BackfillSubscribed(context.Background(), 5, time.Hour, 0)
	if err != nil {
		t.Fatalf("BackfillSubscribed: %v", err)
	}
	if n != 0 {
		t.Errorf("completed = %d, want 0 (an empty first page must not count as fully crawled)", n)
	}
	for _, off := range repo.fetchOffsets {
		if off == domain.FetchOffsetComplete {
			t.Fatalf("channel wrongly stamped fully-crawled (sentinel %d) on an empty first page", domain.FetchOffsetComplete)
		}
	}
	if len(repo.stamped) != 0 {
		t.Errorf("channel activity stamped %v; a deferred channel must not be stamped complete", repo.stamped)
	}
	if len(repo.savedPages) != 0 {
		t.Errorf("saved %d pages, want 0 (nothing to save)", len(repo.savedPages))
	}
	if src.calls != 1 {
		t.Errorf("page fetches = %d, want 1 (fetched once, then deferred)", src.calls)
	}
}

// TestChannelServiceBackfillCompletesEmptyResumeTail locks the other side of the
// rule: a channel already partly crawled (FetchedVideos > 0) that returns an
// empty page on resume is at its genuine tail, not a soft failure, so it does
// complete — the defer only applies to never-crawled channels.
func TestChannelServiceBackfillCompletesEmptyResumeTail(t *testing.T) {
	repo := &fakeChannelRepo{subs: []domain.Channel{
		{ID: "c", URL: "u/c", FetchedVideos: 5}, // resumed mid-crawl
	}}
	src := &emptyPageSource{}
	n, err := NewChannelService(repo, src).BackfillSubscribed(context.Background(), 5, time.Hour, 0)
	if err != nil {
		t.Fatalf("BackfillSubscribed: %v", err)
	}
	if n != 1 {
		t.Errorf("completed = %d, want 1 (a resumed channel's empty tail completes)", n)
	}
	if k := len(repo.fetchOffsets); k == 0 || repo.fetchOffsets[k-1] != domain.FetchOffsetComplete {
		t.Errorf("offsets = %v, want the fully-crawled sentinel (%d) stamped last", repo.fetchOffsets, domain.FetchOffsetComplete)
	}
}

// emptyTailSource serves fullPages non-empty pages (more=true) then an empty
// page (more=false), so the empty page arrives after real progress this run.
type emptyTailSource struct {
	fullPages int
	starts    []int
}

func (s *emptyTailSource) SubscribedChannels(context.Context) ([]domain.Channel, error) {
	return nil, nil
}
func (s *emptyTailSource) ChannelVideosStream(context.Context, string, string, func([]domain.Video) error) ([]domain.Video, error) {
	return nil, nil
}
func (s *emptyTailSource) ChannelLatestN(context.Context, string, string, int) ([]domain.Video, error) {
	return nil, nil
}
func (s *emptyTailSource) ChannelVideosPage(_ context.Context, _, _ string, start int) ([]domain.Video, int, bool, error) {
	s.starts = append(s.starts, start)
	if start <= s.fullPages {
		return []domain.Video{{ID: fmt.Sprintf("v%d", start)}}, start + 1, true, nil
	}
	return nil, start, false, nil
}

// TestChannelServiceBackfillCompletesOnEmptyTailAfterProgress confirms that an
// empty page reached after advancing this run (a fresh channel that paged
// through its whole catalog) is a genuine end and completes — the defer must not
// over-correct and strand fully-crawled channels.
func TestChannelServiceBackfillCompletesOnEmptyTailAfterProgress(t *testing.T) {
	repo := &fakeChannelRepo{subs: []domain.Channel{
		{ID: "c", URL: "u/c", FetchedVideos: 0}, // fresh
	}}
	src := &emptyTailSource{fullPages: 2}
	n, err := NewChannelService(repo, src).BackfillSubscribed(context.Background(), 5, time.Hour, 0)
	if err != nil {
		t.Fatalf("BackfillSubscribed: %v", err)
	}
	if n != 1 {
		t.Errorf("completed = %d, want 1 (empty tail after progress completes)", n)
	}
	if len(repo.savedPages) != 2 {
		t.Errorf("saved %d pages, want 2 (the two real pages)", len(repo.savedPages))
	}
	if want := []int{1, 2, 3}; !reflect.DeepEqual(src.starts, want) {
		t.Errorf("page starts = %v, want %v", src.starts, want)
	}
	if k := len(repo.fetchOffsets); k == 0 || repo.fetchOffsets[k-1] != domain.FetchOffsetComplete {
		t.Errorf("offsets = %v, want the fully-crawled sentinel (%d) stamped last", repo.fetchOffsets, domain.FetchOffsetComplete)
	}
}

// TestChannelServiceBackfillSubscribedPropagatesListError confirms a repo failure
// listing subscriptions aborts the sweep with a wrapped error.
func TestChannelServiceBackfillSubscribedPropagatesListError(t *testing.T) {
	sentinel := errors.New("db down")
	repo := &fakeChannelRepo{subsErr: sentinel}
	_, err := NewChannelService(repo, &recordingSource{}).BackfillSubscribed(context.Background(), 5, time.Hour, 0)
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel error, got %v", err)
	}
}

func TestChannelServiceSubscribedChannelsPropagatesRepoError(t *testing.T) {
	sentinel := errors.New("db down")
	repo := &fakeChannelRepo{subsErr: sentinel}
	src := fakeChannelSource{subs: []domain.Channel{{ID: "c1"}}}
	_, err := NewChannelService(repo, src).SubscribedChannels(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel error, got %v", err)
	}
}

func TestChannelServiceSubscribedChannelsSaveErrorIsNonFatal(t *testing.T) {
	repo := &fakeChannelRepo{saveErr: errors.New("disk full")}
	src := fakeChannelSource{subs: []domain.Channel{{ID: "c1"}}}
	got, err := NewChannelService(repo, src).SubscribedChannels(context.Background())
	if err != nil {
		t.Fatalf("save error should be non-fatal, got %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 channel, got %d", len(got))
	}
}

func TestChannelServiceChannelVideosPropagatesCacheReadError(t *testing.T) {
	sentinel := errors.New("db down")
	repo := &fakeChannelRepo{cachedErr: sentinel}
	src := fakeChannelSource{videos: []domain.Video{{ID: "v1"}}}
	_, err := NewChannelService(repo, src).ChannelVideos(context.Background(), "url", "c1")
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel error, got %v", err)
	}
}

func TestChannelServiceChannelVideosSourceError(t *testing.T) {
	repo := &fakeChannelRepo{}
	src := fakeChannelSource{err: errors.New("yt fail")}
	if _, err := NewChannelService(repo, src).ChannelVideos(context.Background(), "url", "c1"); err == nil {
		t.Fatal("want source error, got nil")
	}
}

type fakeYTAPIClient struct{ unsubbed string }

func (fakeYTAPIClient) Subscribe(context.Context, string) error { return nil }
func (c *fakeYTAPIClient) Unsubscribe(_ context.Context, id string) error {
	c.unsubbed = id
	return nil
}

// TestChannelServiceBlockYTUnsubscribes covers the guarded block on a
// YT-subscribed channel with the API available: it unsubscribes upstream, flags
// the row blocked, and clears the cached videos.
func TestChannelServiceBlockYTUnsubscribes(t *testing.T) {
	repo := &fakeChannelRepo{}
	svc := NewChannelService(repo, fakeChannelSource{})
	yt := &fakeYTAPIClient{}
	svc.SetYTAPI(yt)

	if err := svc.Block(context.Background(), domain.Channel{ID: "c1", State: domain.SubYT}); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if yt.unsubbed != "c1" {
		t.Errorf("yt unsubscribe id = %q, want c1", yt.unsubbed)
	}
	if repo.blocked != "c1" {
		t.Errorf("repo.blocked = %q, want c1", repo.blocked)
	}
	if repo.deletedVideos != "c1" {
		t.Errorf("repo.deletedVideos = %q, want c1", repo.deletedVideos)
	}
}

// TestChannelServiceBlockWithoutYTAPI confirms blocking works offline: a
// YT-subscribed channel is still blocked locally when the API isn't initialized
// (no hard failure), so filtering stays authoritative in the DB.
func TestChannelServiceBlockWithoutYTAPI(t *testing.T) {
	repo := &fakeChannelRepo{}
	svc := NewChannelService(repo, fakeChannelSource{})

	if err := svc.Block(context.Background(), domain.Channel{ID: "c1", State: domain.SubYT}); err != nil {
		t.Fatalf("Block without YT api should not fail, got %v", err)
	}
	if repo.blocked != "c1" {
		t.Errorf("repo.blocked = %q, want c1", repo.blocked)
	}
}

// TestChannelServiceBlockLocalSkipsYT confirms a local sub is blocked without
// any YouTube API call.
func TestChannelServiceBlockLocalSkipsYT(t *testing.T) {
	repo := &fakeChannelRepo{}
	svc := NewChannelService(repo, fakeChannelSource{})
	yt := &fakeYTAPIClient{}
	svc.SetYTAPI(yt)

	if err := svc.Block(context.Background(), domain.Channel{ID: "c1", State: domain.SubLocal}); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if yt.unsubbed != "" {
		t.Errorf("yt unsubscribe should not be called for a local sub, got %q", yt.unsubbed)
	}
	if repo.blocked != "c1" {
		t.Errorf("repo.blocked = %q, want c1", repo.blocked)
	}
}

// TestChannelServiceUnblock confirms unblock delegates to the repo.
func TestChannelServiceUnblock(t *testing.T) {
	repo := &fakeChannelRepo{}
	svc := NewChannelService(repo, fakeChannelSource{})
	if err := svc.Unblock(context.Background(), "c1"); err != nil {
		t.Fatalf("Unblock: %v", err)
	}
	if repo.unblocked != "c1" {
		t.Errorf("repo.unblocked = %q, want c1", repo.unblocked)
	}
}

// TestChannelServiceSetChannelState confirms the state transition delegates to
// the repo (which owns the block-invariant guard).
func TestChannelServiceSetChannelState(t *testing.T) {
	repo := &fakeChannelRepo{}
	svc := NewChannelService(repo, fakeChannelSource{})
	if err := svc.SetChannelState(context.Background(), "c1", domain.SubLocal); err != nil {
		t.Fatalf("SetChannelState: %v", err)
	}
	if repo.stateChangedID != "c1" || repo.stateChangedTo != domain.SubLocal {
		t.Errorf("state change = {%q, %q}, want {c1, subscribed_local}", repo.stateChangedID, repo.stateChangedTo)
	}
}

// TestChannelServiceSetYTAPIConcurrentWithSubscribe guards H-6: SetYTAPI (called
// once by InitYTClient, on whatever goroutine handles that RPC) used to write a
// plain struct field read lock-free by Subscribe/Unsubscribe on other daemon
// goroutines — a real data race under -race. ytAPI is now an atomic.Pointer.
func TestChannelServiceSetYTAPIConcurrentWithSubscribe(t *testing.T) {
	repo := &fakeChannelRepo{}
	src := fakeChannelSource{}
	svc := NewChannelService(repo, src)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			svc.SetYTAPI(&fakeYTAPIClient{})
		}
	}()
	go func() {
		defer wg.Done()
		ch := domain.Channel{ID: "c1"}
		for i := 0; i < 100; i++ {
			_ = svc.Subscribe(context.Background(), ch)
			_ = svc.Unsubscribe(context.Background(), ch)
		}
	}()
	wg.Wait()
}

// pagedSource emits videos in fixed-size pages through onPage, simulating a
// paginated crawl, so the incremental-persistence behavior can be asserted.
type pagedSource struct{ pages [][]domain.Video }

func (pagedSource) SubscribedChannels(context.Context) ([]domain.Channel, error) { return nil, nil }
func (pagedSource) ChannelLatestN(_ context.Context, _, _ string, _ int) ([]domain.Video, error) {
	return nil, nil
}
func (p pagedSource) ChannelVideosPage(_ context.Context, _, _ string, start int) ([]domain.Video, int, bool, error) {
	if start < 1 || start > len(p.pages) {
		return nil, start + 1, false, nil
	}
	return p.pages[start-1], start + 1, start < len(p.pages), nil
}
func (p pagedSource) ChannelVideosStream(_ context.Context, _, _ string, onPage func([]domain.Video) error) ([]domain.Video, error) {
	var all []domain.Video
	for _, pg := range p.pages {
		all = append(all, pg...)
		if onPage != nil {
			if err := onPage(pg); err != nil {
				return all, err
			}
		}
	}
	return all, nil
}

// TestChannelServiceChannelVideosPersistsEachPage asserts a crawl saves each page
// as it arrives (so a drilled-in list sees videos before the full pull finishes),
// then re-persists the merged result — i.e. more saves than pages, page-by-page.
func TestChannelServiceChannelVideosPersistsEachPage(t *testing.T) {
	src := pagedSource{pages: [][]domain.Video{
		{{ID: "a"}, {ID: "b"}},
		{{ID: "c"}},
	}}
	repo := &fakeChannelRepo{}
	got, err := NewChannelService(repo, src).ChannelVideos(context.Background(), "url", "c1")
	if err != nil {
		t.Fatalf("ChannelVideos: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("returned %d videos, want 3", len(got))
	}
	// One save per page (incremental) plus a final authoritative merged save.
	if len(repo.savedPages) != 3 {
		t.Fatalf("SaveChannelVideos called %d times, want 3 (2 pages + final)", len(repo.savedPages))
	}
	if len(repo.savedPages[0]) != 2 || len(repo.savedPages[1]) != 1 {
		t.Errorf("incremental saves were not page-sized: got %d then %d, want 2 then 1",
			len(repo.savedPages[0]), len(repo.savedPages[1]))
	}
	if len(repo.savedPages[2]) != 3 {
		t.Errorf("final merged save had %d videos, want 3", len(repo.savedPages[2]))
	}
}
