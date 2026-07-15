package channels

import (
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

func newSet(channels []domain.Channel) ChannelSet { return New(channels) }

func TestAddDedup(t *testing.T) {
	s := newSet([]domain.Channel{{ID: "ch1", Name: "Alpha"}})
	added := s.Subscribe(domain.Channel{ID: "ch1", Name: "Alpha"})
	if added {
		t.Error("Subscribe: expected false for duplicate ID")
	}
	if s.Len() != 1 {
		t.Errorf("Subscribe: len=%d, want 1", s.Len())
	}
}

func TestAddNew(t *testing.T) {
	s := newSet(nil)
	s.Subscribe(domain.Channel{ID: "ch1", Name: "Alpha"})
	if s.Len() != 1 {
		t.Errorf("Subscribe: len=%d, want 1", s.Len())
	}
	if !s.Index()["ch1"] || !s.Index()["name:alpha"] {
		t.Error("Subscribe: index not updated")
	}
}

func TestRemove(t *testing.T) {
	s := newSet([]domain.Channel{{ID: "ch1", Name: "Alpha"}, {ID: "ch2", Name: "Beta"}})
	s.Remove(domain.Channel{ID: "ch1", Name: "Alpha"})
	if s.Len() != 1 || s.Channels()[0].ID != "ch2" {
		t.Errorf("Remove: got %+v, want [ch2]", s.Channels())
	}
	if s.Index()["ch1"] || s.Index()["name:alpha"] {
		t.Error("Remove: index not cleaned up")
	}
}

func TestSetAlias(t *testing.T) {
	s := newSet([]domain.Channel{{ID: "ch1"}})
	s.SetAlias("ch1", "my alias")
	ch, ok := s.ByID("ch1")
	if !ok || ch.Alias != "my alias" {
		t.Errorf("SetAlias: got %q, want %q", ch.Alias, "my alias")
	}
}

func TestSetTags(t *testing.T) {
	s := newSet([]domain.Channel{{ID: "ch1"}})
	s.SetTags("ch1", []string{"tech", "news"})
	ch, _ := s.ByID("ch1")
	if len(ch.Tags) != 2 || ch.Tags[0] != "tech" {
		t.Errorf("SetTags: got %v", ch.Tags)
	}
}

func TestSyncFromYTMembershipChanged(t *testing.T) {
	s := newSet([]domain.Channel{{ID: "ch1", Name: "Alpha", Alias: "kept"}})
	changed := s.SyncFromYT([]domain.Channel{{ID: "ch1", Name: "Alpha"}, {ID: "ch2", Name: "Beta"}})
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
	s := newSet([]domain.Channel{{ID: "ch1"}})
	changed := s.SyncFromYT([]domain.Channel{{ID: "ch1"}})
	if changed {
		t.Error("SyncFromYT: expected changed=false")
	}
}

func TestSyncFromYTPreservesLocalOnly(t *testing.T) {
	s := newSet([]domain.Channel{{ID: "ch1"}, {ID: "local1"}})
	s.SyncFromYT([]domain.Channel{{ID: "ch1"}})
	if _, ok := s.ByID("local1"); !ok {
		t.Error("SyncFromYT: local-only channel was dropped")
	}
}

func TestUnsubscribeLocal(t *testing.T) {
	ch := domain.Channel{ID: "ch1", Name: "Alpha", IsLocal: true}
	s := newSet([]domain.Channel{ch})
	got, ok := s.Unsubscribe("ch1")
	if !ok {
		t.Fatal("Unsubscribe: expected ok=true")
	}
	if !got.IsLocal {
		t.Error("Unsubscribe: expected IsLocal=true on returned channel")
	}
	if _, found := s.ByID("ch1"); found {
		t.Error("Unsubscribe: channel still in set after removal")
	}
}

func TestUnsubscribeRemote(t *testing.T) {
	s := newSet([]domain.Channel{{ID: "ch1", Name: "Alpha"}})
	got, ok := s.Unsubscribe("ch1")
	if !ok {
		t.Fatal("Unsubscribe: expected ok=true for remote channel")
	}
	if got.IsLocal {
		t.Error("Unsubscribe: expected IsLocal=false for remote channel")
	}
	if _, found := s.ByID("ch1"); found {
		t.Error("Unsubscribe: channel still in set after removal")
	}
}

func TestUnsubscribeNotFound(t *testing.T) {
	s := newSet([]domain.Channel{{ID: "ch1"}})
	_, ok := s.Unsubscribe("missing")
	if ok {
		t.Error("Unsubscribe: expected ok=false for non-existent channel")
	}
	if s.Len() != 1 {
		t.Error("Unsubscribe: set mutated despite not-found")
	}
}
