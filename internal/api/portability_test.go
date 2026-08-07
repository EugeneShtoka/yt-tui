package api_test

import (
	"context"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
	"github.com/EugeneShtoka/yt-tui/internal/procexec"
)

// TestInProcExportAgainstRealDB seeds a real SQLite DB through the public write
// API, then exports via the composed InProc backend — proving the whole inproc
// path (InProc.Export → PortabilityService → db) and that db.DB satisfies the
// export port with its live SQL.
func TestInProcExportAgainstRealDB(t *testing.T) {
	p, database := newInProc(t, procexec.OS{})

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(database.AddSubscribedChannel(context.Background(), domain.Channel{ID: "ch1", Name: "Chan One", URL: "u1", State: domain.SubYT}))
	must(database.SetChannelTags(context.Background(), "ch1", []string{"news", "tech"}))
	must(database.AddBlockedName(context.Background(), "Spammer"))
	must(database.UpsertVideo(context.Background(), "v1", "Vid One", "Chan One", "ch1", 60, 100, "20240101", "vurl"))
	plID, plErr := database.CreatePlaylist(context.Background(), "Favorites")
	must(plErr)
	must(database.AddToPlaylist(context.Background(), plID, "v1"))
	must(database.AddWatchLater(context.Background(), "wl1", "Later", "Chan", "wlurl"))
	must(database.AddHistory(context.Background(), "v1", "playVideo", ""))
	must(database.SaveVideoPosition(context.Background(), "v1", 42000))

	b, err := p.Export(context.Background(), portability.ExportOptions{IncludeWatchData: true})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if b.SchemaVersion != portability.SchemaVersion {
		t.Errorf("SchemaVersion: want %d, got %d", portability.SchemaVersion, b.SchemaVersion)
	}
	var ch1 portability.ChannelExport
	for _, c := range b.Channels {
		if c.ChannelID == "ch1" {
			ch1 = c
		}
	}
	if ch1.SubscriptionState != string(domain.SubYT) || len(ch1.Tags) != 2 {
		t.Errorf("ch1 export mismatch: %+v", ch1)
	}
	if len(b.BlockedNames) != 1 || b.BlockedNames[0] != "Spammer" {
		t.Errorf("BlockedNames: want [Spammer], got %v", b.BlockedNames)
	}
	if len(b.Playlists) != 1 || b.Playlists[0].Name != "Favorites" ||
		len(b.Playlists[0].VideoIDs) != 1 || b.Playlists[0].VideoIDs[0] != "v1" {
		t.Errorf("Playlists mismatch: %+v", b.Playlists)
	}
	if len(b.Videos) != 1 || b.Videos[0].ID != "v1" || b.Videos[0].Duration != 60 {
		t.Errorf("Videos mismatch: %+v", b.Videos)
	}
	if len(b.WatchLater) != 1 || b.WatchLater[0].VideoID != "wl1" {
		t.Errorf("WatchLater mismatch: %+v", b.WatchLater)
	}
	if len(b.History) != 1 || b.History[0].EventType != "playVideo" {
		t.Errorf("History mismatch: %+v", b.History)
	}
	if len(b.Positions) != 1 || b.Positions[0].VideoID != "v1" || b.Positions[0].PositionMs != 42000 {
		t.Errorf("Positions mismatch: %+v", b.Positions)
	}
}

// TestInProcImportRoundTripAgainstRealDB exports from one real SQLite-backed
// InProc and imports the bundle into a fresh one, proving the whole inproc
// import path (InProc.ImportPreview/ImportApply → PortabilityService → db) and
// that *db.DB satisfies the write port. It also checks YT→local conversion,
// watch-data merge, and idempotence against live SQL (FKs enforced).
func seedPortabilityDB(t *testing.T, d *db.DB) {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(d.AddSubscribedChannel(context.Background(), domain.Channel{ID: "ch1", Name: "Chan One", URL: "u1", State: domain.SubYT}))
	must(d.SetChannelTags(context.Background(), "ch1", []string{"news", "tech"}))
	must(d.AddBlockedName(context.Background(), "Spammer"))
	must(d.UpsertVideo(context.Background(), "v1", "Vid One", "Chan One", "ch1", 60, 100, "20240101", "vurl"))
	plID, plErr := d.CreatePlaylist(context.Background(), "Favorites")
	must(plErr)
	must(d.AddToPlaylist(context.Background(), plID, "v1"))
	must(d.AddWatchLater(context.Background(), "wl1", "Later", "Chan", "wlurl"))
	must(d.AddHistory(context.Background(), "v1", "playVideo", ""))
	must(d.SaveVideoPosition(context.Background(), "v1", 42000))
}

func TestInProcImportRoundTripAgainstRealDB(t *testing.T) {
	src, srcDB := newInProc(t, procexec.OS{})
	seedPortabilityDB(t, srcDB)

	bundle, err := src.Export(context.Background(), portability.ExportOptions{IncludeWatchData: true})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Fresh destination backend/DB.
	dst, dstDB := newInProc(t, procexec.OS{})
	opts := portability.ImportOptions{ConvertYTToLocal: true, IncludeWatchData: true}

	plan, err := dst.ImportPreview(context.Background(), bundle, opts)
	if err != nil {
		t.Fatalf("ImportPreview: %v", err)
	}
	if !plan.Compatible || plan.NewChannels != 1 || plan.NewPlaylists != 1 || plan.NewHistory != 1 {
		t.Fatalf("preview mismatch: %+v", plan)
	}

	res, err := dst.ImportApply(context.Background(), bundle, opts)
	if err != nil {
		t.Fatalf("ImportApply: %v", err)
	}
	if res.ChannelsUpserted != 1 || res.PlaylistAdds != 1 || res.WatchLaterAdded != 1 ||
		res.HistoryAdded != 1 || res.PositionsSet != 1 {
		t.Fatalf("apply result mismatch: %+v", res)
	}

	// The YT subscription converted to a local one in the destination.
	chans, err := dstDB.AllChannels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(chans) != 1 || chans[0].ID != "ch1" || chans[0].State != domain.SubLocal {
		t.Fatalf("channel not converted to local: %+v", chans)
	}
	// Watch data landed.
	if got, _, _ := dstDB.VideoPosition(context.Background(), "v1"); got != 42000 {
		t.Errorf("position not imported: %d", got)
	}

	// Idempotence: a second apply adds nothing.
	second, err := dst.ImportApply(context.Background(), bundle, opts)
	if err != nil {
		t.Fatalf("second ImportApply: %v", err)
	}
	if second.PlaylistAdds != 0 || second.WatchLaterAdded != 0 || second.HistoryAdded != 0 || second.PositionsSet != 0 {
		t.Fatalf("second apply not idempotent: %+v", second)
	}
}
