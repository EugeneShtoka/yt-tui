package tab

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

func daysAgoUnix(d int) int64 { return time.Now().Add(-time.Duration(d) * 24 * time.Hour).Unix() }

// staleFixtures: one stale tagged channel, one fresh tagged channel, one
// subscribed (exempt), one blocked.
func staleFixtures() []domain.Channel {
	return []domain.Channel{
		{ID: "stale", Name: "StaleChan", State: domain.SubNone, Tags: []string{"old"}, LastActivityAt: daysAgoUnix(40)},
		{ID: "fresh", Name: "FreshChan", State: domain.SubNone, Tags: []string{"new"}, LastActivityAt: daysAgoUnix(2)},
		{ID: "yt", Name: "YTChan", State: domain.SubYT, Tags: []string{"sub"}},
		{ID: "blk", Name: "BlkChan", State: domain.SubNone, Blocked: true, Tags: []string{"b"}},
	}
}

func chLoadedStale(view string, hideStale bool, chs []domain.Channel) Channels {
	ch := NewChannels(context.Background(), &fakeBackend{}, testKeys(), false, ChannelsOpts{LatestCount: 3, RefreshMinutes: 60, View: view, HideStale: hideStale, StaleDays: 30})
	m, _ := ch.Update(chsLoadedMsg{chans: chs, latest: map[string]domain.Video{}})
	return m.(Channels)
}

func TestChannelsStaleMode(t *testing.T) {
	// The stale mode surfaces exactly the stale tagged channel.
	got := sortedIDs(chLoadedStale("stale", false, staleFixtures()).sortedChs)
	if !reflect.DeepEqual(got, []string{"stale"}) {
		t.Fatalf("stale mode: got %v, want [stale]", got)
	}
}

func TestChannelsHideStaleExcludesFromMixed(t *testing.T) {
	// hide on: mixed drops the stale channel but keeps fresh/subscribed.
	got := sortedIDs(chLoadedStale("mixed", true, staleFixtures()).sortedChs)
	if !reflect.DeepEqual(got, []string{"fresh", "yt"}) {
		t.Fatalf("mixed with hide on: got %v, want [fresh yt]", got)
	}
	// hide off: the stale channel reappears in mixed.
	got = sortedIDs(chLoadedStale("mixed", false, staleFixtures()).sortedChs)
	if !reflect.DeepEqual(got, []string{"fresh", "stale", "yt"}) {
		t.Fatalf("mixed with hide off: got %v, want [fresh stale yt]", got)
	}
}

func TestTagsStaleMode(t *testing.T) {
	tg := NewTags(context.Background(), &fakeBackend{}, testKeys(), false, TagsOpts{Mode: "stale", StaleDays: 30})
	m, _ := tg.Update(tagsDataMsg{chans: staleFixtures()})
	tg = m.(Tags)
	// Only the stale channel's tag ("old") should appear in the stale mode.
	var tags []string
	for _, r := range tg.sortedTagRows {
		tags = append(tags, r.Tag)
	}
	if !reflect.DeepEqual(tags, []string{"old"}) {
		t.Fatalf("tags stale mode: got %v, want [old]", tags)
	}
}

func TestFeedStaleLoadSelectsStaleChannels(t *testing.T) {
	var askedIDs []string
	be := &fakeBackend{
		allChannels:  func(context.Context) ([]domain.Channel, error) { return staleFixtures(), nil },
		getFeedCache: func(context.Context, string) ([]domain.Video, error) { return nil, nil },
		getAllChannelVideos: func(_ context.Context, ids []string) ([]domain.Video, error) {
			askedIDs = ids
			out := make([]domain.Video, len(ids))
			for i, id := range ids {
				out[i] = domain.Video{ID: "v-" + id, ChannelID: id, Title: id}
			}
			return out, nil
		},
	}
	f := NewFeed(context.Background(), be, testKeys(), false, FeedOpts{Mode: "stale", StaleDays: 30})
	msg := runCmd(f.staleLoadCmd())
	loaded, ok := msg.(feedStaleLoadedMsg)
	if !ok {
		t.Fatalf("want feedStaleLoadedMsg, got %#v", msg)
	}
	// Only the stale tagged channel qualifies (subscribed/blocked/fresh excluded).
	if !reflect.DeepEqual(askedIDs, []string{"stale"}) {
		t.Fatalf("stale load asked for %v, want [stale]", askedIDs)
	}
	if len(loaded.videos) != 1 || loaded.videos[0].ChannelID != "stale" {
		t.Fatalf("stale videos = %+v, want one from channel 'stale'", loaded.videos)
	}
}

func TestFeedStaleChannelInFeedIsExcluded(t *testing.T) {
	var askedIDs []string
	be := &fakeBackend{
		allChannels: func(context.Context) ([]domain.Channel, error) { return staleFixtures(), nil },
		// The "stale" channel is currently in the recommended feed → active, not stale.
		getFeedCache: func(context.Context, string) ([]domain.Video, error) {
			return []domain.Video{{ID: "rv", ChannelID: "stale"}}, nil
		},
		getAllChannelVideos: func(_ context.Context, ids []string) ([]domain.Video, error) {
			askedIDs = ids
			return nil, nil
		},
	}
	f := NewFeed(context.Background(), be, testKeys(), false, FeedOpts{Mode: "stale", StaleDays: 30})
	runCmd(f.staleLoadCmd())
	if len(askedIDs) != 0 {
		t.Fatalf("in-feed stale channel should be excluded, asked for %v", askedIDs)
	}
}
