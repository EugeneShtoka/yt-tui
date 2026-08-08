package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/backend/service"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
)

// memPort is a mutable in-memory PortabilityRepo + PortabilityWriter, so import
// merge policy and idempotence can be exercised end-to-end without a real DB.
type memPort struct {
	channels     map[string]domain.Channel
	blockedNames map[string]bool
	plID         map[string]int64
	plVids       map[int64]map[string]bool
	nextPlID     int64
	ytPlaylists  []domain.YTPlaylist
	videos       map[string]domain.Video
	history      []domain.HistoryEntry
	positions    map[string]int64
}

func newMemPort() *memPort {
	return &memPort{
		channels: map[string]domain.Channel{}, blockedNames: map[string]bool{},
		plID: map[string]int64{}, plVids: map[int64]map[string]bool{},
		videos:    map[string]domain.Video{},
		positions: map[string]int64{},
	}
}

// ── reads ──
func (m *memPort) AllChannels(ctx context.Context) ([]domain.Channel, error) {
	out := make([]domain.Channel, 0, len(m.channels))
	for id := range m.channels {
		out = append(out, m.channels[id])
	}
	return out, nil
}

func (m *memPort) Blocklist(ctx context.Context) ([]string, []string, error) {
	var ids, names []string
	for id := range m.channels {
		if m.channels[id].Blocked {
			ids = append(ids, id)
		}
	}
	for n := range m.blockedNames {
		names = append(names, n)
	}
	return ids, names, nil
}

func (m *memPort) Playlists(ctx context.Context) ([]domain.Playlist, error) {
	out := make([]domain.Playlist, 0, len(m.plID))
	for name, id := range m.plID {
		out = append(out, domain.Playlist{ID: id, Name: name})
	}
	return out, nil
}

func (m *memPort) PlaylistVideos(ctx context.Context, id int64) ([]domain.Video, error) {
	var out []domain.Video
	for vid := range m.plVids[id] {
		out = append(out, domain.Video{ID: vid})
	}
	return out, nil
}

func (m *memPort) GetYTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error) {
	return m.ytPlaylists, nil
}
func (m *memPort) History(context.Context, int) ([]domain.HistoryEntry, error) { return m.history, nil }
func (m *memPort) AllVideoPositions(ctx context.Context) (map[string]int64, error) {
	return m.positions, nil
}

// ── writes ──
func (m *memPort) UpsertChannel(ctx context.Context, ch domain.Channel) error {
	m.channels[ch.ID] = ch
	return nil
}
func (m *memPort) AddBlockedName(ctx context.Context, name string) error {
	m.blockedNames[name] = true
	return nil
}

func (m *memPort) CreatePlaylist(ctx context.Context, name string) (int64, error) {
	if id, ok := m.plID[name]; ok {
		return id, nil
	}
	m.nextPlID++
	m.plID[name] = m.nextPlID
	m.plVids[m.nextPlID] = map[string]bool{}
	return m.nextPlID, nil
}

func (m *memPort) AddToPlaylist(ctx context.Context, id int64, vid string) error {
	if m.plVids[id] == nil {
		m.plVids[id] = map[string]bool{}
	}
	m.plVids[id][vid] = true
	return nil
}

func (m *memPort) UpsertVideo(ctx context.Context, id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error {
	m.videos[id] = domain.Video{ID: id, Title: title, Channel: channel, ChannelID: channelID, Duration: duration, ViewCount: viewCount, UploadDate: uploadDate, URL: url}
	return nil
}

func (m *memPort) SaveYTPlaylists(ctx context.Context, pls []domain.YTPlaylist) error {
	m.ytPlaylists = pls
	return nil
}

func (m *memPort) AddHistoryEvent(ctx context.Context, videoID, eventType, details string, ts time.Time) error {
	m.history = append(m.history, domain.HistoryEntry{VideoID: videoID, EventType: eventType, Details: details, Timestamp: ts})
	return nil
}
func (m *memPort) SaveVideoPosition(ctx context.Context, videoID string, ms int64) error {
	m.positions[videoID] = ms
	return nil
}

func newImportSvc(m *memPort) *service.PortabilityService {
	return service.NewPortabilityService(m, m)
}

func apply(t *testing.T, svc *service.PortabilityService, b portability.Bundle, opts portability.ImportOptions) portability.ImportResult {
	t.Helper()
	res, err := svc.ImportApply(context.Background(), b, opts)
	if err != nil {
		t.Fatalf("ImportApply: %v", err)
	}
	return res
}

// TestImportApplyChannelMerge covers the channel merge policy: tags union, alias
// incoming-wins (non-empty), and YT→none default (annotations kept).
func TestImportApplyChannelMerge(t *testing.T) {
	m := newMemPort()
	m.channels["c1"] = domain.Channel{ID: "c1", Name: "Old", Alias: "old-alias", Tags: []string{"news"}, State: domain.SubLocal}
	svc := newImportSvc(m)

	b := portability.Bundle{
		SchemaVersion: portability.SchemaVersion,
		Channels: []portability.ChannelExport{
			{ChannelID: "c1", Name: "New", Alias: "new-alias", Tags: []string{"news", "tech"}, SubscriptionState: "subscribed_yt"},
			{ChannelID: "c2", Name: "Fresh", Tags: []string{"go"}, SubscriptionState: "subscribed_local"},
		},
	}
	res := apply(t, svc, b, portability.ImportOptions{}) // ConvertYTToLocal off
	if res.ChannelsUpserted != 2 {
		t.Fatalf("ChannelsUpserted: want 2, got %d", res.ChannelsUpserted)
	}
	c1 := m.channels["c1"]
	if c1.Alias != "new-alias" {
		t.Errorf("alias should be incoming-wins, got %q", c1.Alias)
	}
	if len(c1.Tags) != 2 || c1.Tags[0] != "news" || c1.Tags[1] != "tech" {
		t.Errorf("tags should union preserving order, got %v", c1.Tags)
	}
	if c1.State != domain.SubNone {
		t.Errorf("YT sub without conversion should drop to none, got %q", c1.State)
	}
	if c2 := m.channels["c2"]; c2.State != domain.SubLocal {
		t.Errorf("local sub should import as local, got %q", c2.State)
	}
}

func TestImportApplyYTToLocalConversion(t *testing.T) {
	m := newMemPort()
	svc := newImportSvc(m)
	b := portability.Bundle{
		SchemaVersion: portability.SchemaVersion,
		Channels:      []portability.ChannelExport{{ChannelID: "c1", SubscriptionState: "subscribed_yt"}},
	}
	apply(t, svc, b, portability.ImportOptions{ConvertYTToLocal: true})
	if got := m.channels["c1"].State; got != domain.SubLocal {
		t.Errorf("YT→local conversion: want local, got %q", got)
	}
}

func TestImportApplyBlockInvariant(t *testing.T) {
	m := newMemPort()
	// existing subscribed channel; incoming blocks it.
	m.channels["c1"] = domain.Channel{ID: "c1", State: domain.SubYT}
	svc := newImportSvc(m)
	b := portability.Bundle{
		SchemaVersion: portability.SchemaVersion,
		Channels:      []portability.ChannelExport{{ChannelID: "c1", SubscriptionState: "subscribed_yt", Blocked: true}},
		BlockedNames:  []string{"Spammer"},
	}
	res := apply(t, svc, b, portability.ImportOptions{ConvertYTToLocal: true})
	if c := m.channels["c1"]; !c.Blocked || c.State != domain.SubNone {
		t.Errorf("blocked channel must be none: blocked=%v state=%q", c.Blocked, c.State)
	}
	if res.BlockedNames != 1 || !m.blockedNames["Spammer"] {
		t.Errorf("blocked name not imported: %+v", m.blockedNames)
	}
}

func TestImportApplyPlaylistMergeByName(t *testing.T) {
	m := newMemPort()
	// existing "Favs" already holds v1.
	id, _ := m.CreatePlaylist(context.Background(), "Favs")
	m.AddToPlaylist(context.Background(), id, "v1") //nolint:errcheck // in-memory
	svc := newImportSvc(m)

	b := portability.Bundle{
		SchemaVersion: portability.SchemaVersion,
		Videos:        []portability.VideoExport{{ID: "v1", Title: "One"}, {ID: "v2", Title: "Two"}},
		Playlists:     []portability.PlaylistExport{{Name: "Favs", VideoIDs: []string{"v1", "v2"}}},
	}
	res := apply(t, svc, b, portability.ImportOptions{})
	if res.PlaylistAdds != 1 { // v1 already present, only v2 added
		t.Errorf("PlaylistAdds: want 1, got %d", res.PlaylistAdds)
	}
	if !m.plVids[id]["v2"] {
		t.Errorf("v2 not added to Favs")
	}
	if res.VideosUpserted != 2 {
		t.Errorf("VideosUpserted: want 2, got %d", res.VideosUpserted)
	}
}

func TestImportApplyWatchDataGating(t *testing.T) {
	m := newMemPort()
	svc := newImportSvc(m)
	b := portability.Bundle{
		SchemaVersion: portability.SchemaVersion,
		Videos:        []portability.VideoExport{{ID: "v1", Title: "One"}},
		History:       []portability.HistoryExport{{VideoID: "v1", EventType: "playVideo", Timestamp: 1000}},
		Positions:     []portability.PositionExport{{VideoID: "v1", PositionMs: 5000}},
	}
	// flag off: watch data ignored.
	if res := apply(t, svc, b, portability.ImportOptions{}); res.HistoryAdded != 0 || res.PositionsSet != 0 {
		t.Fatalf("watch data applied with flag off: %+v", res)
	}
	// flag on: history + position land.
	res := apply(t, svc, b, portability.ImportOptions{IncludeWatchData: true})
	if res.HistoryAdded != 1 || res.PositionsSet != 1 {
		t.Fatalf("watch data not applied: %+v", res)
	}
	if m.positions["v1"] != 5000 {
		t.Errorf("position not saved: %d", m.positions["v1"])
	}
}

func TestImportApplyPositionMaxPolicy(t *testing.T) {
	m := newMemPort()
	m.videos["v1"] = domain.Video{ID: "v1"}
	m.positions["v1"] = 9000 // already further along than the bundle
	svc := newImportSvc(m)
	b := portability.Bundle{
		SchemaVersion: portability.SchemaVersion,
		Videos:        []portability.VideoExport{{ID: "v1"}},
		Positions:     []portability.PositionExport{{VideoID: "v1", PositionMs: 5000}},
	}
	res := apply(t, svc, b, portability.ImportOptions{IncludeWatchData: true})
	if res.PositionsSet != 0 || m.positions["v1"] != 9000 {
		t.Errorf("max policy violated: set=%d pos=%d", res.PositionsSet, m.positions["v1"])
	}
}

// TestImportApplyIdempotent applies the same bundle twice; the second run must
// be a no-op (all-zero result) across every section.
func TestImportApplyIdempotent(t *testing.T) {
	m := newMemPort()
	svc := newImportSvc(m)
	b := portability.Bundle{
		SchemaVersion: portability.SchemaVersion,
		Channels:      []portability.ChannelExport{{ChannelID: "c1", SubscriptionState: "subscribed_local", Tags: []string{"go"}}},
		BlockedNames:  []string{"Spammer"},
		Videos:        []portability.VideoExport{{ID: "v1", Title: "One"}},
		Playlists:     []portability.PlaylistExport{{Name: "Favs", VideoIDs: []string{"v1"}}},
		YTPlaylists:   []portability.YTPlaylistRef{{ID: "PL1", Title: "My PL"}},
		History:       []portability.HistoryExport{{VideoID: "v1", EventType: "playVideo", Timestamp: 1000}},
		Positions:     []portability.PositionExport{{VideoID: "v1", PositionMs: 5000}},
	}
	opts := portability.ImportOptions{IncludeWatchData: true}
	apply(t, svc, b, opts)
	chans, hist, pos := len(m.channels), len(m.history), len(m.positions)

	// Second apply: the dedup'd sections must add nothing, and DB state must be
	// unchanged (channel/video rows are re-upserted with identical values).
	second := apply(t, svc, b, opts)
	if second.BlockedNames != 0 || second.PlaylistAdds != 0 ||
		second.YTPlaylists != 0 || second.HistoryAdded != 0 || second.PositionsSet != 0 {
		t.Fatalf("second import added new rows: %+v", second)
	}
	if len(m.channels) != chans || len(m.history) != hist || len(m.positions) != pos {
		t.Fatalf("state changed on re-import: channels %d→%d history %d→%d positions %d→%d",
			chans, len(m.channels), hist, len(m.history), pos, len(m.positions))
	}
}

func TestImportPreviewCounts(t *testing.T) {
	m := newMemPort()
	m.channels["c1"] = domain.Channel{ID: "c1", State: domain.SubYT} // existing → updated
	id, _ := m.CreatePlaylist(context.Background(), "Favs")
	m.AddToPlaylist(context.Background(), id, "v1") //nolint:errcheck // in-memory
	svc := newImportSvc(m)

	b := portability.Bundle{
		SchemaVersion: portability.SchemaVersion,
		Channels: []portability.ChannelExport{
			{ChannelID: "c1", SubscriptionState: "subscribed_local"}, // updated
			{ChannelID: "c2", SubscriptionState: "none", Blocked: true},
		},
		BlockedNames: []string{"Spammer"},
		Videos:       []portability.VideoExport{{ID: "v1"}, {ID: "v2"}},
		Playlists: []portability.PlaylistExport{
			{Name: "Favs", VideoIDs: []string{"v1", "v2"}}, // merged, +1
			{Name: "New", VideoIDs: []string{"v2"}},        // new, +1
		},
		YTPlaylists: []portability.YTPlaylistRef{{ID: "PL1", Title: "t"}},
		History:     []portability.HistoryExport{{VideoID: "v1", EventType: "playVideo", Timestamp: 1}},
		Positions:   []portability.PositionExport{{VideoID: "v1", PositionMs: 100}},
	}
	plan, err := svc.ImportPreview(context.Background(), b, portability.ImportOptions{IncludeWatchData: true})
	if err != nil {
		t.Fatalf("ImportPreview: %v", err)
	}
	assertEq(t, "NewChannels", plan.NewChannels, 1)
	assertEq(t, "UpdatedChannels", plan.UpdatedChannels, 1)
	assertEq(t, "BlockedChannels", plan.BlockedChannels, 1)
	assertEq(t, "NewBlockedNames", plan.NewBlockedNames, 1)
	assertEq(t, "NewPlaylists", plan.NewPlaylists, 1)
	assertEq(t, "MergedPlaylists", plan.MergedPlaylists, 1)
	assertEq(t, "PlaylistAdds", plan.PlaylistAdds, 2) // Favs+v2, New+v2
	assertEq(t, "Videos", plan.Videos, 2)
	assertEq(t, "NewYTPlaylists", plan.NewYTPlaylists, 1)
	assertEq(t, "NewHistory", plan.NewHistory, 1)
	assertEq(t, "NewPositions", plan.NewPositions, 1)
	if !plan.HasWatchData || !plan.Compatible {
		t.Errorf("HasWatchData=%v Compatible=%v", plan.HasWatchData, plan.Compatible)
	}
}

func TestImportIncompatibleSchema(t *testing.T) {
	svc := newImportSvc(newMemPort())
	b := portability.Bundle{SchemaVersion: portability.SchemaVersion + 1}
	plan, err := svc.ImportPreview(context.Background(), b, portability.ImportOptions{})
	if err != nil {
		t.Fatalf("ImportPreview: %v", err)
	}
	if plan.Compatible {
		t.Error("incompatible schema reported as compatible")
	}
	if _, err := svc.ImportApply(context.Background(), b, portability.ImportOptions{}); err == nil {
		t.Error("ImportApply should reject an incompatible schema")
	}
}

func assertEq(t *testing.T, field string, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("%s: want %d, got %d", field, want, got)
	}
}
