package thumbs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRetainNewest verifies the client-side newest-N cap: the most-recently
// modified images are kept and the rest evicted. Bytes are arbitrary (non-JPEG
// passes through Put's crop untouched).
func TestRetainNewest(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	ids := []string{"a", "b", "c", "d"}
	for i, id := range ids {
		if perr := s.Put(id, []byte("img-"+id)); perr != nil {
			t.Fatalf("Put %s: %v", id, perr)
		}
		mt := time.Unix(1_000+int64(i)*10, 0) // "a" oldest … "d" newest
		if cerr := os.Chtimes(filepath.Join(dir, id+".jpg"), mt, mt); cerr != nil {
			t.Fatalf("Chtimes %s: %v", id, cerr)
		}
	}

	removed, err := s.RetainNewest(2)
	if err != nil {
		t.Fatalf("RetainNewest: %v", err)
	}
	if removed != 2 {
		t.Errorf("removed %d, want 2", removed)
	}
	if s.Has("a") || s.Has("b") {
		t.Error("oldest two thumbnails should have been evicted")
	}
	if !s.Has("c") || !s.Has("d") {
		t.Error("newest two thumbnails should have been kept")
	}
	if n, _ := s.RetainNewest(0); n != 0 {
		t.Errorf("RetainNewest(0) removed %d, want 0", n)
	}
}
