package portability

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
)

// The portability package exists to pin a STABLE, self-describing on-disk JSON
// contract that is deliberately decoupled from internal/domain (see the package
// doc). A rename or dropped json tag silently breaks every previously-exported
// bundle. These tests are the guard: they fail loudly the moment the wire shape
// drifts, so a domain refactor can't change the export format by accident.

// fullBundle populates every section (including opt-in watch data and a config
// profile) so each field's json tag is exercised.
func fullBundle() Bundle {
	return Bundle{
		SchemaVersion: SchemaVersion,
		Channels: []ChannelExport{{
			ChannelID: "c1", Name: "Chan", URL: "https://youtube.com/c1",
			Alias: "alias", Tags: []string{"news", "go"},
			SubscriptionState: "subscribed_local", Blocked: true,
		}},
		BlockedNames: []string{"Spammer"},
		Playlists:    []PlaylistExport{{Name: "Favs", VideoIDs: []string{"v1", "v2"}}},
		YTPlaylists:  []YTPlaylistRef{{ID: "PL1", Title: "My PL"}},
		Videos: []VideoExport{{
			ID: "v1", Title: "One", Channel: "Chan", ChannelID: "c1",
			Duration: 60, ViewCount: 1000, UploadDate: "20260101", URL: "u",
		}},
		History: []HistoryExport{{
			VideoID: "v1", Title: "One", Channel: "Chan", ChannelID: "c1",
			Duration: 60, ViewCount: 1000, UploadDate: "20260101",
			EventType: "playVideo", Details: "d", Timestamp: 1700000000,
		}},
		Positions: []PositionExport{{VideoID: "v1", PositionMs: 5000}},
		Config:    json.RawMessage(`{"theme":"dark"}`),
	}
}

// TestBundleTopLevelKeyContract pins the exact set of top-level json keys a
// fully-populated bundle emits. Adding/renaming/removing a section must update
// this list deliberately.
func TestBundleTopLevelKeyContract(t *testing.T) {
	data, err := json.Marshal(fullBundle())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	want := []string{
		"schema_version", "channels", "blocked_names", "playlists",
		"yt_playlists", "videos", "history", "positions", "config",
	}
	got := make([]string, 0, len(m))
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("top-level keys drifted:\n got  %v\n want %v", got, want)
	}
}

// TestBundleNestedFieldContract spot-checks the contract-critical nested keys —
// the identifiers importers key off. Uses table-driven "json path" lookups.
func TestBundleNestedFieldContract(t *testing.T) {
	data, err := json.Marshal(fullBundle())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var b struct {
		Channels []map[string]json.RawMessage `json:"channels"`
		Playlist []map[string]json.RawMessage `json:"playlists"`
		Videos   []map[string]json.RawMessage `json:"videos"`
		Position []map[string]json.RawMessage `json:"positions"`
		History  []map[string]json.RawMessage `json:"history"`
	}
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cases := []struct {
		name string
		row  map[string]json.RawMessage
		keys []string
	}{
		{"channel", b.Channels[0], []string{"channel_id", "subscription_state", "tags", "blocked"}},
		{"playlist", b.Playlist[0], []string{"name", "video_ids"}},
		{"video", b.Videos[0], []string{"id", "channel_id", "view_count", "upload_date"}},
		{"position", b.Position[0], []string{"video_id", "position_ms"}},
		{"history", b.History[0], []string{"video_id", "event_type", "timestamp"}},
	}
	for _, c := range cases {
		for _, k := range c.keys {
			if _, ok := c.row[k]; !ok {
				t.Errorf("%s section missing json key %q (contract drift)", c.name, k)
			}
		}
	}
}

// TestBundleRoundTrip guarantees a marshal→unmarshal cycle is lossless, so a
// bundle written by one build reloads identically in another.
func TestBundleRoundTrip(t *testing.T) {
	orig := fullBundle()
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Bundle
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(orig, got) {
		t.Errorf("round-trip lost data:\n orig %+v\n got  %+v", orig, got)
	}
}

// TestSchemaVersionAlwaysSerialized locks in that schema_version carries no
// omitempty: even the zero version must appear on the wire, or an importer
// can't detect an old/incompatible bundle.
func TestSchemaVersionAlwaysSerialized(t *testing.T) {
	data, err := json.Marshal(Bundle{}) // zero value, SchemaVersion == 0
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["schema_version"]; !ok {
		t.Error("schema_version must always be present (no omitempty)")
	}
}

// TestBundleOmitsEmptyOptionalSections verifies every optional section is
// dropped when empty, so a minimal export stays minimal (and personal watch
// data never leaks into a bundle that didn't opt in).
func TestBundleOmitsEmptyOptionalSections(t *testing.T) {
	data, err := json.Marshal(Bundle{SchemaVersion: SchemaVersion})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(m) != 1 {
		t.Errorf("empty bundle should serialize only schema_version, got keys %v", keysOf(m))
	}
	for _, section := range []string{"channels", "history", "positions", "config", "playlists"} {
		if _, ok := m[section]; ok {
			t.Errorf("optional section %q must be omitted when empty", section)
		}
	}
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
