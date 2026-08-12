package thumbs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "thumbnails"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

func TestPutGetHasDelete(t *testing.T) {
	s := newTestStore(t)
	const id = "abc123_-XYZ"

	if _, ok, _ := s.Get(id); ok {
		t.Fatal("expected miss before Put")
	}
	if s.Has(id) {
		t.Fatal("Has should be false before Put")
	}
	want := []byte("\xff\xd8\xff jpeg-ish bytes")
	if err := s.Put(id, want); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !s.Has(id) {
		t.Fatal("Has should be true after Put")
	}
	got, ok, err := s.Get(id)
	if err != nil || !ok {
		t.Fatalf("Get after Put: ok=%v err=%v", ok, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Get mismatch: %q != %q", got, want)
	}
	if err := s.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if s.Has(id) {
		t.Fatal("Has should be false after Delete")
	}
	// Deleting a missing file is not an error.
	if err := s.Delete(id); err != nil {
		t.Fatalf("Delete missing: %v", err)
	}
}

func TestUnsafeIDRejected(t *testing.T) {
	s := newTestStore(t)
	for _, bad := range []string{"", "../escape", "a/b", "with space", "dot.dot"} {
		if err := s.Put(bad, []byte("x")); err == nil {
			t.Errorf("Put(%q) should fail", bad)
		}
		if _, ok, _ := s.Get(bad); ok {
			t.Errorf("Get(%q) should miss", bad)
		}
	}
	// No stray files should have been created outside the id scheme.
	entries, _ := os.ReadDir(s.dir)
	for _, e := range entries {
		t.Errorf("unexpected file created: %s", e.Name())
	}
}

func TestRetain(t *testing.T) {
	s := newTestStore(t)
	for _, id := range []string{"keep_1", "keep_2", "drop_1", "drop_2"} {
		if err := s.Put(id, []byte("img")); err != nil {
			t.Fatalf("Put %s: %v", id, err)
		}
	}
	removed, err := s.Retain(map[string]bool{"keep_1": true, "keep_2": true})
	if err != nil {
		t.Fatalf("Retain: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed=%d, want 2", removed)
	}
	if !s.Has("keep_1") || !s.Has("keep_2") {
		t.Fatal("kept ids were deleted")
	}
	if s.Has("drop_1") || s.Has("drop_2") {
		t.Fatal("dropped ids survived Retain")
	}
}

func TestURLFor(t *testing.T) {
	if got := URLFor("dQw4w9WgXcQ"); got != "https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg" {
		t.Fatalf("URLFor: %s", got)
	}
}
