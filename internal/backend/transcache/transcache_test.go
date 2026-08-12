package transcache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreGetPutRoundTrip(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, ok := s.Get("v1"); ok {
		t.Error("empty store should miss")
	}
	if err := s.Put("v1", "hello\nworld"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if text, ok := s.Get("v1"); !ok || text != "hello\nworld" {
		t.Errorf("Get = (%q,%v), want (\"hello\\nworld\",true)", text, ok)
	}
	if err := s.Put("../evil", "x"); err == nil {
		t.Error("Put with an unsafe id must error")
	}
	if _, ok := s.Get("../evil"); ok {
		t.Error("Get with an unsafe id must miss")
	}
}

func TestRetainNewestKeepsNewestByMtime(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	// Write five entries and stamp increasing mtimes so ordering is deterministic:
	// "a" oldest … "e" newest.
	ids := []string{"a", "b", "c", "d", "e"}
	for i, id := range ids {
		if perr := s.Put(id, id); perr != nil {
			t.Fatalf("Put %s: %v", id, perr)
		}
		mt := time.Unix(1_000+int64(i)*10, 0)
		if cerr := os.Chtimes(filepath.Join(dir, id+".txt"), mt, mt); cerr != nil {
			t.Fatalf("Chtimes %s: %v", id, cerr)
		}
	}

	removed, err := s.RetainNewest(2)
	if err != nil {
		t.Fatalf("RetainNewest: %v", err)
	}
	if removed != 3 {
		t.Errorf("removed %d, want 3", removed)
	}
	for _, id := range []string{"a", "b", "c"} {
		if _, ok := s.Get(id); ok {
			t.Errorf("%q should have been evicted", id)
		}
	}
	for _, id := range []string{"d", "e"} {
		if _, ok := s.Get(id); !ok {
			t.Errorf("%q should have been kept", id)
		}
	}

	// max <= 0 keeps everything.
	if n, _ := s.RetainNewest(0); n != 0 {
		t.Errorf("RetainNewest(0) removed %d, want 0", n)
	}
}
