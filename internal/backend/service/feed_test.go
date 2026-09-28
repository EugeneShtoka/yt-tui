package service

import (
	"context"
	"errors"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

type fakeFeedRepo struct {
	hidden    map[string]bool
	hiddenErr error
	local     []domain.LocalVideo
	localErr  error
	subs      []domain.Channel
	subsErr   error
	saveErr   error
	savedName string
	saveCalls int
	blIDs     []string
	blErr     error
	cached    []domain.Video
	cachedErr error
}

func (f *fakeFeedRepo) HiddenRecVideoIDs(ctx context.Context) (map[string]bool, error) {
	return f.hidden, f.hiddenErr
}
func (f *fakeFeedRepo) LocalVideos(ctx context.Context) ([]domain.LocalVideo, error) {
	return f.local, f.localErr
}
func (f *fakeFeedRepo) GetSubscribedChannels(ctx context.Context) ([]domain.Channel, error) {
	return f.subs, f.subsErr
}

func (f *fakeFeedRepo) SaveFeedCache(ctx context.Context, name string, _ []domain.Video) error {
	f.savedName = name
	f.saveCalls++
	return f.saveErr
}

func (f *fakeFeedRepo) GetFeedCache(ctx context.Context, _ string) ([]domain.Video, error) {
	return f.cached, f.cachedErr
}

func (f *fakeFeedRepo) Blocklist(ctx context.Context) ([]string, error) {
	return f.blIDs, f.blErr
}

type fakeRecSource struct {
	videos []domain.Video
	err    error
}

func (f fakeRecSource) Recommended(context.Context) ([]domain.Video, error) { return f.videos, f.err }

// A read error anywhere in the filter-pipeline inputs must surface, not yield a
// silently-wrong feed (H-5).
func TestFeedServiceRecommendedPropagatesRepoErrors(t *testing.T) {
	sentinel := errors.New("db down")
	tests := []struct {
		name string
		repo *fakeFeedRepo
	}{
		{"hidden ids error", &fakeFeedRepo{hiddenErr: sentinel}},
		{"local videos error", &fakeFeedRepo{localErr: sentinel}},
		{"subscribed channels error", &fakeFeedRepo{subsErr: sentinel}},
		{"blocklist error", &fakeFeedRepo{blErr: sentinel}},
	}
	src := fakeRecSource{videos: []domain.Video{{ID: "a"}}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewFeedService(tc.repo, src, &config.Config{})
			_, err := s.Recommended(context.Background())
			if !errors.Is(err, sentinel) {
				t.Fatalf("want sentinel error, got %v", err)
			}
		})
	}
}

func TestFeedServiceRecommendedSourceError(t *testing.T) {
	s := NewFeedService(&fakeFeedRepo{}, fakeRecSource{err: errors.New("yt fail")}, &config.Config{})
	if _, err := s.Recommended(context.Background()); err == nil {
		t.Fatal("want source error, got nil")
	}
}

func TestFeedServiceRecommendedHappyPath(t *testing.T) {
	repo := &fakeFeedRepo{hidden: map[string]bool{}}
	src := fakeRecSource{videos: []domain.Video{{ID: "a", ChannelID: "c1"}}}
	got, err := NewFeedService(repo, src, &config.Config{}).Recommended(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 video, got %d", len(got))
	}
	if repo.savedName != "recommended" {
		t.Fatalf("feed cache not saved, savedName=%q", repo.savedName)
	}
}

// The DB-derived blocklist filters videos whose channel ID is blocked. Blocking
// is channel-ID-only, so a video that merely shares a blocked channel's name (but
// carries a different ID) survives.
func TestFeedServiceRecommendedBlocklist(t *testing.T) {
	repo := &fakeFeedRepo{
		hidden: map[string]bool{},
		blIDs:  []string{"chBlockedID"},
	}
	src := fakeRecSource{videos: []domain.Video{
		{ID: "keep", ChannelID: "chOK"},
		{ID: "byID", ChannelID: "chBlockedID"},
		{ID: "sameName", Channel: "Blocked Chan", ChannelID: "chOtherID"},
	}}
	got, err := NewFeedService(repo, src, &config.Config{}).Recommended(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].ID != "keep" || got[1].ID != "sameName" {
		t.Fatalf("blocklist filter: got %+v, want 'keep' and 'sameName'", got)
	}
}

// A cache-write failure is best-effort: the filtered list is still valid, so the
// call must succeed (H-5 keeps read errors fatal but cache writes non-fatal).
func TestFeedServiceRecommendedCacheSaveErrorIsNonFatal(t *testing.T) {
	repo := &fakeFeedRepo{hidden: map[string]bool{}, saveErr: errors.New("disk full")}
	src := fakeRecSource{videos: []domain.Video{{ID: "a"}}}
	got, err := NewFeedService(repo, src, &config.Config{}).Recommended(context.Background())
	if err != nil {
		t.Fatalf("cache-save error should be non-fatal, got %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 video, got %d", len(got))
	}
}

// The cached feed is a snapshot of a past fetch, so a block or a hide made after
// it was written must still be honored on the cold-start read — otherwise the
// suppressed rows come straight back on every restart.
func TestFeedServiceFeedCacheAppliesSuppressionFilters(t *testing.T) {
	repo := &fakeFeedRepo{
		cached: []domain.Video{
			{ID: "keep", ChannelID: "cOK"},
			{ID: "blocked", ChannelID: "cBad"},
			{ID: "hiddenVid", ChannelID: "cOK"},
		},
		blIDs:  []string{"cBad"},
		hidden: map[string]bool{"hiddenVid": true},
	}
	s := NewFeedService(repo, fakeRecSource{}, &config.Config{})
	got, err := s.FeedCache(context.Background(), "recommended")
	if err != nil {
		t.Fatalf("FeedCache: %v", err)
	}
	if len(got) != 1 || got[0].ID != "keep" {
		t.Fatalf("want only the unsuppressed video, got %#v", got)
	}
}

// A read error in either suppression source must surface rather than yield a
// feed that silently shows blocked channels.
func TestFeedServiceFeedCachePropagatesRepoErrors(t *testing.T) {
	sentinel := errors.New("db down")
	cached := []domain.Video{{ID: "v1", ChannelID: "c1"}}
	tests := []struct {
		name string
		repo *fakeFeedRepo
	}{
		{"cache read error", &fakeFeedRepo{cachedErr: sentinel}},
		{"blocklist error", &fakeFeedRepo{cached: cached, blErr: sentinel}},
		{"hidden ids error", &fakeFeedRepo{cached: cached, hiddenErr: sentinel}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewFeedService(tc.repo, fakeRecSource{}, &config.Config{})
			if _, err := s.FeedCache(context.Background(), "recommended"); !errors.Is(err, sentinel) {
				t.Fatalf("want sentinel, got %v", err)
			}
		})
	}
}
