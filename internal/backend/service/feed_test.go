package service

import (
	"context"
	"errors"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

type fakeFeedRepo struct {
	hidden     map[string]bool
	hiddenErr  error
	local      []domain.LocalVideo
	localErr   error
	subs       []domain.Channel
	subsErr    error
	saveErr    error
	savedName  string
	saveCalls  int
	blIDs      []string
	blNames    []string
	blErr      error
	resolved   []resolvedBlock // records ResolveBlockedName calls
	resolveErr error
}

type resolvedBlock struct{ name, id string }

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

func (f *fakeFeedRepo) Blocklist(ctx context.Context) ([]string, []string, error) {
	return f.blIDs, f.blNames, f.blErr
}

func (f *fakeFeedRepo) ResolveBlockedName(ctx context.Context, name, id string) error {
	f.resolved = append(f.resolved, resolvedBlock{name, id})
	return f.resolveErr
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

// The DB-derived blocklist filters matching videos out of the feed, and a
// name-only block matched by an ID-carrying video is resolved back to the DB.
func TestFeedServiceRecommendedBlocklist(t *testing.T) {
	repo := &fakeFeedRepo{
		hidden:  map[string]bool{},
		blIDs:   []string{"chBlockedID"},
		blNames: []string{"Bad Chan"},
	}
	src := fakeRecSource{videos: []domain.Video{
		{ID: "keep", ChannelID: "chOK"},
		{ID: "byID", ChannelID: "chBlockedID"},
		{ID: "byName", Channel: "Bad Chan", ChannelID: "chBadResolved"},
	}}
	got, err := NewFeedService(repo, src, &config.Config{}).Recommended(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "keep" {
		t.Fatalf("blocklist filter: got %+v, want only 'keep'", got)
	}
	if len(repo.resolved) != 1 || repo.resolved[0] != (resolvedBlock{"Bad Chan", "chBadResolved"}) {
		t.Fatalf("name-only block not resolved: got %+v", repo.resolved)
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
