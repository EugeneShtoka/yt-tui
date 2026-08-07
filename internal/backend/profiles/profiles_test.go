package profiles

import (
	"bytes"
	"path/filepath"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "profiles"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

func TestSaveGetRoundTrip(t *testing.T) {
	s := newStore(t)
	want := []byte(`{"theme":"gruvbox"}`)
	if err := s.Save("work", want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, found, err := s.Get("work")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("Get: found = false, want true")
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Get: data = %q, want %q", got, want)
	}
}

func TestGetMissingIsNotFound(t *testing.T) {
	s := newStore(t)
	got, found, err := s.Get("nope")
	if err != nil {
		t.Fatalf("Get: unexpected error %v", err)
	}
	if found {
		t.Fatalf("Get: found = true for missing profile (data %q)", got)
	}
}

func TestSaveOverwrites(t *testing.T) {
	s := newStore(t)
	if err := s.Save("p", []byte("v1")); err != nil {
		t.Fatalf("Save v1: %v", err)
	}
	if err := s.Save("p", []byte("v2")); err != nil {
		t.Fatalf("Save v2: %v", err)
	}
	got, _, err := s.Get("p")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "v2" {
		t.Fatalf("Get after overwrite = %q, want v2", got)
	}
}

func TestListSortedAndComplete(t *testing.T) {
	s := newStore(t)
	for _, n := range []string{"zeta", "alpha", "mid"} {
		if err := s.Save(n, []byte("{}")); err != nil {
			t.Fatalf("Save %s: %v", n, err)
		}
	}
	names, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"alpha", "mid", "zeta"}
	if len(names) != len(want) {
		t.Fatalf("List = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("List = %v, want %v", names, want)
		}
	}
}

func TestListEmpty(t *testing.T) {
	s := newStore(t)
	names, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("List on empty store = %v, want none", names)
	}
}

func TestInvalidNamesRejected(t *testing.T) {
	s := newStore(t)
	for _, name := range []string{"", ".", "..", "a/b", "a\\b", "../evil", "with/slash"} {
		if err := s.Save(name, []byte("{}")); err == nil {
			t.Errorf("Save(%q): expected error, got nil", name)
		}
		if _, _, err := s.Get(name); err == nil {
			t.Errorf("Get(%q): expected error, got nil", name)
		}
	}
}

// A traversal-shaped name must never let a write escape the profiles dir.
func TestSaveDoesNotEscapeDir(t *testing.T) {
	s := newStore(t)
	if err := s.Save("../escape", []byte("x")); err == nil {
		t.Fatal("Save(../escape): expected rejection, got nil")
	}
	// The parent dir must not have gained an escape.json sibling.
	if matches, _ := filepath.Glob(filepath.Join(s.dir, "..", "*.json")); len(matches) != 0 {
		t.Fatalf("write escaped the profiles dir: %v", matches)
	}
}
