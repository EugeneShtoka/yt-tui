package db

import (
	"context"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

func TestChannelVideosRefreshedRoundTrip(t *testing.T) {
	d := newTestDB(t)
	if err := d.AddSubscribedChannel(context.Background(), domain.Channel{ID: "UC1", Name: "C", URL: "u"}); err != nil {
		t.Fatal(err)
	}

	// Never refreshed → 0.
	chans, err := d.GetSubscribedChannels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(chans) != 1 || chans[0].VideosRefreshedAt != 0 {
		t.Fatalf("want zero VideosRefreshedAt initially, got %d", chans[0].VideosRefreshedAt)
	}

	before := time.Now().Add(-time.Second).Unix()
	if touchErr := d.TouchChannelVideosRefreshed(context.Background(), "UC1"); touchErr != nil {
		t.Fatal(touchErr)
	}
	chans, err = d.GetSubscribedChannels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := chans[0].VideosRefreshedAt; got < before {
		t.Errorf("VideosRefreshedAt = %d, want >= %d", got, before)
	}
}
