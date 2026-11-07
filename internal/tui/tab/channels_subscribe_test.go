package tab

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// selectChannel moves the list cursor onto the channel with id.
func selectChannel(t *testing.T, c Channels, id string) Channels {
	t.Helper()
	for i := range c.sortedChs {
		if c.sortedChs[i].ID == id {
			c.listNav.GotoRow(i)
			return c
		}
	}
	t.Fatalf("channel %q not in view %v", id, sortedIDs(c.sortedChs))
	return c
}

func TestChannelsSubscribeMarksOptimisticallyAndEmits(t *testing.T) {
	c := selectChannel(t, chLoaded("mixed", mixedChannels()), "none")

	m, cmd := c.Update(tea.KeyPressMsg{Text: "S"})
	got := m.(Channels)

	if ch, _ := got.subs.ByID("none"); ch.State != domain.SubYT {
		t.Errorf("optimistic subscribe: state %q, want %q", ch.State, domain.SubYT)
	}
	sm, ok := runCmd(cmd).(tuipkg.SubscribeMsg)
	if !ok || sm.Channel.ID != "none" || sm.Channel.State != domain.SubYT {
		t.Fatalf("want SubscribeMsg{none, subscribed_yt}, got %#v", runCmd(cmd))
	}
}

func TestChannelsSubscribeAlreadySubscribedIsNoop(t *testing.T) {
	c := selectChannel(t, chLoaded("mixed", mixedChannels()), "yt")

	_, cmd := c.Update(tea.KeyPressMsg{Text: "S"})
	if st, ok := runCmd(cmd).(tuipkg.StatusMsg); !ok || !st.IsErr {
		t.Fatalf("want error status for an already-subscribed channel, got %#v", runCmd(cmd))
	}
}

func TestChannelsSubscribeResultRevertsOwnMarkOnError(t *testing.T) {
	c := selectChannel(t, chLoaded("mixed", mixedChannels()), "none")
	m, cmd := c.Update(tea.KeyPressMsg{Text: "S"})
	sm := runCmd(cmd).(tuipkg.SubscribeMsg)

	m, _ = m.(Channels).Update(tuipkg.SubscribeResultMsg{Channel: sm.Channel, Err: errors.New("boom")})
	got := m.(Channels)
	if ch, _ := got.subs.ByID("none"); ch.State != domain.SubNone {
		t.Errorf("failed subscribe should revert to none, got %q", ch.State)
	}
}

// A failed subscribe fired from a video row elsewhere must not demote a channel
// the Channels tab already has as subscribed.
func TestChannelsSubscribeResultErrorLeavesForeignChannelAlone(t *testing.T) {
	c := chLoaded("mixed", mixedChannels())
	m, _ := c.Update(tuipkg.SubscribeResultMsg{Channel: domain.Channel{ID: "yt", State: domain.SubYT}, Err: errors.New("boom")})
	got := m.(Channels)
	if ch, _ := got.subs.ByID("yt"); ch.State != domain.SubYT {
		t.Errorf("foreign failure demoted a subscribed channel to %q", ch.State)
	}
}

// A successful subscribe from a video row (Feed, Search…) shows up in Channels.
func TestChannelsSubscribeResultSuccessFromElsewhere(t *testing.T) {
	c := chLoaded("subscribed", mixedChannels())
	m, _ := c.Update(tuipkg.SubscribeResultMsg{Channel: domain.Channel{ID: "none", Name: "NoneChan", State: domain.SubYT}})
	got := m.(Channels)
	if ch, _ := got.subs.ByID("none"); ch.State != domain.SubYT || len(ch.Tags) != 1 {
		t.Errorf("known channel should become subscribed keeping its tags, got %+v", ch)
	}

	m, _ = got.Update(tuipkg.SubscribeResultMsg{Channel: domain.Channel{ID: "new", Name: "New", State: domain.SubYT}})
	got = m.(Channels)
	if _, ok := got.subs.ByID("new"); !ok {
		t.Error("unknown channel should be added on successful subscribe")
	}
}

func TestHandleVideoActionSubscribe(t *testing.T) {
	keys := testKeys()
	cmd, ok := HandleVideoAction(tea.KeyPressMsg{Text: "S"}, domain.Video{ChannelID: "c1", Channel: "Chan"}, keys)
	if !ok {
		t.Fatal("Subscribe key not handled")
	}
	sm, isSub := runCmd(cmd).(tuipkg.SubscribeMsg)
	if !isSub || sm.Channel.ID != "c1" || sm.Channel.Name != "Chan" {
		t.Fatalf("want SubscribeMsg{c1}, got %#v", runCmd(cmd))
	}

	cmd, _ = HandleVideoAction(tea.KeyPressMsg{Text: "S"}, domain.Video{Title: "x"}, keys)
	if st, isSt := runCmd(cmd).(tuipkg.StatusMsg); !isSt || !st.IsErr {
		t.Fatalf("no channel id: want error status, got %#v", runCmd(cmd))
	}
}
