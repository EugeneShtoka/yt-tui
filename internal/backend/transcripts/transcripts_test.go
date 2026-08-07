package transcripts

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "transcripts"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

// write simulates yt-dlp producing a transcript file for id in the given lang.
func write(t *testing.T, s *Store, id, lang string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.dir, id+"."+lang+".srt"), []byte("1\n00:00\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", id, err)
	}
}

// writeSRT drops a one-cue .srt for id in lang whose text identifies the file.
func writeSRT(t *testing.T, s *Store, id, lang, text string) {
	t.Helper()
	srt := "1\n00:00:01,000 --> 00:00:03,000\n" + text + "\n"
	if err := os.WriteFile(filepath.Join(s.dir, id+"."+lang+".srt"), []byte(srt), 0o600); err != nil {
		t.Fatalf("writeSRT %s.%s: %v", id, lang, err)
	}
}

func TestSelectCuesLanguagePriority(t *testing.T) {
	cases := []struct {
		name       string
		langs      []string // downloaded sidecar languages
		original   string
		acceptable []string
		want       string // expected cue text (== the chosen language), "" = no match
	}{
		{"original wins when acceptable", []string{"en", "ru"}, "ru", []string{"en", "ru"}, "ru"},
		{"array order when original not acceptable", []string{"en", "ru"}, "ru", []string{"en"}, "en"},
		{"array order when original not downloaded", []string{"en"}, "ru", []string{"ru", "en"}, "en"},
		{"glob matches regional variant", []string{"en-US"}, "", []string{"en.*"}, "en-US"},
		{"no acceptable match", []string{"de"}, "de", []string{"en", "ru"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			for _, l := range tc.langs {
				writeSRT(t, s, "vid", l, l) // cue text == language code
			}
			cues, ok := s.SelectCues("vid", tc.original, tc.acceptable)
			if tc.want == "" {
				if ok {
					t.Fatalf("expected no match, got %v", cues)
				}
				return
			}
			if !ok || len(cues) != 1 || cues[0].Text != tc.want {
				t.Fatalf("SelectCues = %v (ok=%v), want cue text %q", cues, ok, tc.want)
			}
		})
	}
}

func TestHasDelete(t *testing.T) {
	s := newTestStore(t)
	const id = "vid_-123"
	if s.Has(id) {
		t.Fatal("Has before write")
	}
	write(t, s, id, "en")
	write(t, s, id, "es") // two languages for one video
	if !s.Has(id) {
		t.Fatal("Has after write")
	}
	if err := s.Delete(id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if s.Has(id) {
		t.Fatal("Has after delete — both language files should be gone")
	}
}

func TestRetain(t *testing.T) {
	s := newTestStore(t)
	write(t, s, "keep_1", "en")
	write(t, s, "keep_1", "fr")
	write(t, s, "drop_1", "en")
	removed, err := s.Retain(map[string]bool{"keep_1": true})
	if err != nil {
		t.Fatalf("Retain: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed=%d, want 1 (only drop_1.en.srt)", removed)
	}
	if !s.Has("keep_1") || s.Has("drop_1") {
		t.Fatal("Retain kept/dropped the wrong ids")
	}
}

func TestOutputTemplateAndUnsafeID(t *testing.T) {
	s := newTestStore(t)
	if got := s.OutputTemplate(); filepath.Base(got) != "%(id)s.%(ext)s" {
		t.Fatalf("OutputTemplate base = %q", filepath.Base(got))
	}
	if s.Has("../evil") || s.Has("a/b") {
		t.Fatal("unsafe ids must never match")
	}
}
