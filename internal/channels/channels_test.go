package channels

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

type mockDB struct {
	removed       []string
	deletedVideos []string
}

func (m *mockDB) RemoveSubscribedChannel(id string) error {
	m.removed = append(m.removed, id)
	return nil
}

func (m *mockDB) DeleteChannelVideos(id string) error {
	m.deletedVideos = append(m.deletedVideos, id)
	return nil
}

func newSet(channels []youtube.Channel) ChannelSet { return New(channels, nil, nil) }

func TestAddDedup(t *testing.T) {
	s := newSet([]youtube.Channel{{ID: "ch1", Name: "Alpha"}})
	added := s.Subscribe(youtube.Channel{ID: "ch1", Name: "Alpha"})
	if added {
		t.Error("Subscribe: expected false for duplicate ID")
	}
	if s.Len() != 1 {
		t.Errorf("Subscribe: len=%d, want 1", s.Len())
	}
}

func TestAddNew(t *testing.T) {
	s := newSet(nil)
	s.Subscribe(youtube.Channel{ID: "ch1", Name: "Alpha"})
	if s.Len() != 1 {
		t.Errorf("Subscribe: len=%d, want 1", s.Len())
	}
	if !s.Index()["ch1"] || !s.Index()["name:alpha"] {
		t.Error("Subscribe: index not updated")
	}
}

func TestRemove(t *testing.T) {
	s := newSet([]youtube.Channel{{ID: "ch1", Name: "Alpha"}, {ID: "ch2", Name: "Beta"}})
	s.Remove("ch1", "Alpha")
	if s.Len() != 1 || s.Channels()[0].ID != "ch2" {
		t.Errorf("Remove: got %+v, want [ch2]", s.Channels())
	}
	if s.Index()["ch1"] || s.Index()["name:alpha"] {
		t.Error("Remove: index not cleaned up")
	}
}

func TestSetAlias(t *testing.T) {
	s := newSet([]youtube.Channel{{ID: "ch1"}})
	s.SetAlias("ch1", "my alias")
	ch, ok := s.ByID("ch1")
	if !ok || ch.Alias != "my alias" {
		t.Errorf("SetAlias: got %q, want %q", ch.Alias, "my alias")
	}
}

func TestSetTags(t *testing.T) {
	s := newSet([]youtube.Channel{{ID: "ch1"}})
	s.SetTags("ch1", []string{"tech", "news"})
	ch, _ := s.ByID("ch1")
	if len(ch.Tags) != 2 || ch.Tags[0] != "tech" {
		t.Errorf("SetTags: got %v", ch.Tags)
	}
}

func TestSyncFromYTMembershipChanged(t *testing.T) {
	s := newSet([]youtube.Channel{{ID: "ch1", Name: "Alpha", Alias: "kept"}})
	changed := s.SyncFromYT([]youtube.Channel{{ID: "ch1", Name: "Alpha"}, {ID: "ch2", Name: "Beta"}})
	if !changed {
		t.Error("SyncFromYT: expected changed=true")
	}
	if s.Len() != 2 {
		t.Errorf("SyncFromYT: len=%d, want 2", s.Len())
	}
	ch, _ := s.ByID("ch1")
	if ch.Alias != "kept" {
		t.Errorf("SyncFromYT: alias not preserved, got %q", ch.Alias)
	}
}

func TestSyncFromYTNoChange(t *testing.T) {
	s := newSet([]youtube.Channel{{ID: "ch1"}})
	changed := s.SyncFromYT([]youtube.Channel{{ID: "ch1"}})
	if changed {
		t.Error("SyncFromYT: expected changed=false")
	}
}

func TestSyncFromYTPreservesLocalOnly(t *testing.T) {
	s := newSet([]youtube.Channel{{ID: "ch1"}, {ID: "local1"}})
	s.SyncFromYT([]youtube.Channel{{ID: "ch1"}})
	if _, ok := s.ByID("local1"); !ok {
		t.Error("SyncFromYT: local-only channel was dropped")
	}
}

func TestUnsubscribeLocalRemovesAndCallsDB(t *testing.T) {
	db := &mockDB{}
	s := New([]youtube.Channel{{ID: "ch1", Name: "Alpha", IsLocal: true}}, db, nil)
	cmd, ok := s.Unsubscribe("ch1", "Alpha")
	if !ok {
		t.Fatal("Unsubscribe: expected ok=true for local channel")
	}
	if _, found := s.ByID("ch1"); found {
		t.Error("Unsubscribe: channel still in set after local unsubscribe")
	}
	if cmd == nil {
		t.Fatal("Unsubscribe: expected non-nil cmd for local channel")
	}
	cmd() // execute to trigger DB calls
	if len(db.removed) != 1 || db.removed[0] != "ch1" {
		t.Errorf("Unsubscribe: RemoveSubscribedChannel not called, got %v", db.removed)
	}
	if len(db.deletedVideos) != 1 || db.deletedVideos[0] != "ch1" {
		t.Errorf("Unsubscribe: DeleteChannelVideos not called, got %v", db.deletedVideos)
	}
}

func TestUnsubscribeRemoteNoClientBlocked(t *testing.T) {
	s := New([]youtube.Channel{{ID: "ch1", Name: "Alpha"}}, nil, nil)
	cmd, ok := s.Unsubscribe("ch1", "Alpha")
	if ok {
		t.Error("Unsubscribe: expected ok=false when ytClient is nil")
	}
	if cmd != nil {
		t.Error("Unsubscribe: expected nil cmd when ytClient is nil")
	}
	if _, found := s.ByID("ch1"); !found {
		t.Error("Unsubscribe: channel should remain in set when blocked")
	}
}

func TestUnsubscribeRemoteWithClientRemoves(t *testing.T) {
	s := New([]youtube.Channel{{ID: "ch1", Name: "Alpha"}}, nil, &youtube.YTClient{})
	cmd, ok := s.Unsubscribe("ch1", "Alpha")
	if !ok {
		t.Fatal("Unsubscribe: expected ok=true when ytClient is set")
	}
	if cmd == nil {
		t.Error("Unsubscribe: expected non-nil cmd for remote channel")
	}
	if _, found := s.ByID("ch1"); found {
		t.Error("Unsubscribe: channel should be removed optimistically")
	}
}
