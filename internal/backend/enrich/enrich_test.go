package enrich

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/backend/transcripts"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// noWait is a sleep seam that never blocks but still honors cancellation.
func noWait(ctx context.Context, _ time.Duration) bool { return ctx.Err() == nil }

// fakeCat is an in-memory Catalog recording the writes the enricher makes.
type fakeCat struct {
	rec, sub    []domain.VideoRef
	eligible    map[string]bool   // returned by ThumbnailEligibleIDs
	dates       map[string]string // videoID -> upload_date written
	detailSaves []string          // videoIDs SaveVideoDetailsCache was called for
}

func newFakeCat() *fakeCat { return &fakeCat{dates: map[string]string{}} }

func (f *fakeCat) RecommendedVideosWithoutDetails(ctx context.Context) ([]domain.VideoRef, error) {
	return f.rec, nil
}
func (f *fakeCat) SubscribedVideosWithoutDetails(context.Context, int) ([]domain.VideoRef, error) {
	return f.sub, nil
}
func (f *fakeCat) ThumbnailEligibleIDs(context.Context, int) (map[string]bool, error) {
	return f.eligible, nil
}
func (f *fakeCat) UpdateVideoUploadDate(ctx context.Context, id, date string) error {
	f.dates[id] = date
	return nil
}
func (f *fakeCat) SaveVideoChapters(context.Context, string, []domain.Chapter) error     { return nil }
func (f *fakeCat) SaveVideoSBSegments(context.Context, string, []domain.SBSegment) error { return nil }
func (f *fakeCat) SaveVideoLinks(context.Context, string, []domain.Link) error           { return nil }
func (f *fakeCat) SaveVideoDetailsCache(ctx context.Context, id, _, _ string, _ int64) error {
	f.detailSaves = append(f.detailSaves, id)
	return nil
}

// fakeFetcher returns canned details and records the URL fetch order.
type fakeFetcher struct {
	byURL       map[string]domain.VideoDetails
	order       []string
	transcripts []string // watch URLs VideoDetailsWithTranscript was called with
	srtDir      string   // when set, each unified call writes a canned .srt there
}

func (f *fakeFetcher) VideoDetails(_ context.Context, url string) (domain.VideoDetails, error) {
	f.order = append(f.order, url)
	return f.byURL[url], nil
}

func (f *fakeFetcher) VideoDetailsWithTranscript(_ context.Context, url, _, _, _ string) (domain.VideoDetails, error) {
	f.transcripts = append(f.transcripts, url)
	// Simulate yt-dlp's side effect: drop a .srt sidecar so a note can be built.
	if f.srtDir != "" {
		id := url[strings.LastIndex(url, "=")+1:]
		srt := "1\n00:00:00,000 --> 00:00:01,000\nhello world\n"
		_ = os.WriteFile(filepath.Join(f.srtDir, id+".en.srt"), []byte(srt), 0o600)
	}
	return f.byURL[url], nil
}

func detailsWithDate(id, date string) domain.VideoDetails {
	return domain.VideoDetails{Video: domain.Video{ID: id, UploadDate: date}, Description: "d"}
}

func TestNewDisabledWhenBothOff(t *testing.T) {
	if e := New(newFakeCat(), &fakeFetcher{}, nil, nil, Params{DelaySeconds: 0, ThumbnailsPerChannel: 0}); e != nil {
		t.Fatal("New should return nil when nothing is enabled")
	}
	// Thumbnails need a store; count>0 with a nil store is still "off".
	if e := New(newFakeCat(), &fakeFetcher{}, nil, nil, Params{DelaySeconds: 0, ThumbnailsPerChannel: 30}); e != nil {
		t.Fatal("New should return nil when thumbnails requested but no store")
	}
}

func TestRunDetailsOrderAndBackfill(t *testing.T) {
	cat := newFakeCat()
	cat.rec = []domain.VideoRef{{ID: "r1", URL: "u-r1"}}
	cat.sub = []domain.VideoRef{{ID: "s1", URL: "u-s1"}, {ID: "s2", URL: ""}} // s2 has no URL → skipped
	yt := &fakeFetcher{byURL: map[string]domain.VideoDetails{
		"u-r1": detailsWithDate("r1", "20250214"),
		"u-s1": detailsWithDate("s1", "20240101"),
	}}
	e := New(cat, yt, nil, nil, Params{DelaySeconds: 1})
	e.sleep = noWait

	e.Run(context.Background())

	// Recommended is fetched before subscribed; s2 (empty URL) is skipped.
	if len(yt.order) != 2 || yt.order[0] != "u-r1" || yt.order[1] != "u-s1" {
		t.Fatalf("fetch order = %v, want [u-r1 u-s1]", yt.order)
	}
	if cat.dates["r1"] != "20250214" || cat.dates["s1"] != "20240101" {
		t.Fatalf("upload dates not backfilled: %v", cat.dates)
	}
	if len(cat.detailSaves) != 2 {
		t.Fatalf("details saved for %v, want 2", cat.detailSaves)
	}
}

func TestRunTranscriptsBuildsNotes(t *testing.T) {
	cat := newFakeCat()
	cat.eligible = map[string]bool{"a": true, "b": true}
	srtDir := t.TempDir()
	yt := &fakeFetcher{
		srtDir: srtDir,
		byURL: map[string]domain.VideoDetails{
			watchURL("a"): detailsWithDate("a", "20250101"),
			watchURL("b"): detailsWithDate("b", "20250202"),
		},
	}
	store, err := transcripts.NewStore(srtDir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := store.EnableMarkdown(t.TempDir()); err != nil {
		t.Fatalf("EnableMarkdown: %v", err)
	}
	e := New(cat, yt, nil, store, Params{DelaySeconds: 1, SaveTranscript: true, ThumbnailsPerChannel: 30, SubtitleLangs: []string{"en.*"}})
	if e == nil {
		t.Fatal("New should enable when SaveTranscript + markdown store + delay are set")
	}
	e.sleep = noWait

	e.Run(context.Background())

	// One unified fetch per eligible id (store starts empty).
	if len(yt.transcripts) != 2 {
		t.Fatalf("unified fetch called %d times, want 2 (%v)", len(yt.transcripts), yt.transcripts)
	}
	for _, u := range yt.transcripts {
		if u != watchURL("a") && u != watchURL("b") {
			t.Fatalf("unexpected transcript URL %q", u)
		}
	}
	// A note is built for each, and its metadata was applied to the DB (so the
	// details pass would skip these videos).
	for _, id := range []string{"a", "b"} {
		if !store.HasMarkdown(id) {
			t.Fatalf("note not built for %q", id)
		}
	}
	if len(cat.detailSaves) != 2 {
		t.Fatalf("details applied for %v, want 2", cat.detailSaves)
	}
}

func TestRunStopsOnCancelledContext(t *testing.T) {
	cat := newFakeCat()
	cat.rec = []domain.VideoRef{{ID: "r1", URL: "u-r1"}}
	yt := &fakeFetcher{byURL: map[string]domain.VideoDetails{"u-r1": detailsWithDate("r1", "20250101")}}
	e := New(cat, yt, nil, nil, Params{DelaySeconds: 1})
	e.sleep = noWait

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled
	e.Run(ctx)

	if len(yt.order) != 0 {
		t.Fatalf("no fetches expected on canceled ctx, got %v", yt.order)
	}
}
