package domain

import "testing"

// ResumeOffset / FullyCrawled encode the backfill resume sentinel
// (FetchOffsetComplete = -1). These branches are load-bearing for the crawler
// and churned recently, so pin the boundary rows.
func TestChannelResumeOffset(t *testing.T) {
	cases := []struct {
		name    string
		fetched int64
		want    int64
	}{
		{"never crawled", 0, 0},
		{"paused mid-crawl", 42, 42},
		{"complete sentinel", FetchOffsetComplete, 0},
		{"any negative treated as complete", -7, 0},
	}
	for _, c := range cases {
		ch := Channel{FetchedVideos: c.fetched}
		if got := ch.ResumeOffset(); got != c.want {
			t.Errorf("%s: ResumeOffset(%d) = %d, want %d", c.name, c.fetched, got, c.want)
		}
	}
}

func TestChannelFullyCrawled(t *testing.T) {
	cases := []struct {
		fetched int64
		want    bool
	}{
		{FetchOffsetComplete, true},
		{0, false},
		{100, false},
	}
	for _, c := range cases {
		if got := (Channel{FetchedVideos: c.fetched}).FullyCrawled(); got != c.want {
			t.Errorf("FullyCrawled(%d) = %v, want %v", c.fetched, got, c.want)
		}
	}
}

// SubState derives from the legacy IsLocal flag only when State is unset — the
// empty-State fallback returns SubYT (not SubNone), which IsSubscribed relies on.
func TestChannelSubState(t *testing.T) {
	cases := []struct {
		name    string
		state   SubscriptionState
		isLocal bool
		want    SubscriptionState
	}{
		{"explicit state wins over legacy flag", SubYT, true, SubYT},
		{"explicit none", SubNone, false, SubNone},
		{"empty state + local → local fallback", "", true, SubLocal},
		{"empty state + not local → yt fallback", "", false, SubYT},
	}
	for _, c := range cases {
		ch := Channel{State: c.state, IsLocal: c.isLocal}
		if got := ch.SubState(); got != c.want {
			t.Errorf("%s: SubState() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestChannelIsSubscribed(t *testing.T) {
	cases := []struct {
		name string
		ch   Channel
		want bool
	}{
		{"yt subscribed", Channel{State: SubYT}, true},
		{"local subscribed", Channel{State: SubLocal}, true},
		{"explicit none", Channel{State: SubNone}, false},
		{"legacy local flag", Channel{IsLocal: true}, true},
		{"bare channel falls back to yt", Channel{}, true},
	}
	for _, c := range cases {
		if got := c.ch.IsSubscribed(); got != c.want {
			t.Errorf("%s: IsSubscribed() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestChannelDisplayName(t *testing.T) {
	if got := (Channel{Name: "Real", Alias: "Nick"}).DisplayName(); got != "Nick" {
		t.Errorf("alias should win: got %q", got)
	}
	if got := (Channel{Name: "Real"}).DisplayName(); got != "Real" {
		t.Errorf("name should be used when no alias: got %q", got)
	}
}

func TestHistoryIsAudio(t *testing.T) {
	cases := []struct {
		event string
		want  bool
	}{
		{"streamAudio", true},
		{"download audio", true},
		{"playAudio", true},
		{"playVideo", false},
		{"streamVideo", false},
		{"", false},
	}
	for _, c := range cases {
		if got := (HistoryEntry{EventType: c.event}).IsAudio(); got != c.want {
			t.Errorf("IsAudio(%q) = %v, want %v", c.event, got, c.want)
		}
	}
}

func TestActivityGetActivityDetail(t *testing.T) {
	cases := []struct {
		name string
		e    ActivityEntry
		want string
	}{
		{"subscribe local", ActivityEntry{Type: "subscribe", IsLocal: true, ChannelName: "Chan"}, "Chan (local)"},
		{"subscribe remote", ActivityEntry{Type: "subscribe", ChannelName: "Chan"}, "Chan (remote)"},
		{"create playlist", ActivityEntry{Type: "create_playlist", IsLocal: true, PlaylistName: "Favs"}, "Favs (local)"},
		{"add to playlist", ActivityEntry{Type: "add_to_playlist", VideoTitle: "Vid", PlaylistName: "Favs"}, "Vid → Favs (remote)"},
		{"unknown type falls through", ActivityEntry{Type: "mystery"}, "mystery"},
	}
	for _, c := range cases {
		if got := c.e.GetActivityDetail(); got != c.want {
			t.Errorf("%s: GetActivityDetail() = %q, want %q", c.name, got, c.want)
		}
	}
}
