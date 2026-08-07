package app

import (
	"context"
	"errors"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/config"
)

// fakeProfileBackend serves one named profile, embedding the Nop for the rest.
type fakeProfileBackend struct {
	apitest.NopProfileBackend
	name  string
	data  []byte
	found bool
	err   error
	asked string // records the last name GetProfile was called with
}

func (f *fakeProfileBackend) GetProfile(_ context.Context, name string) ([]byte, bool, error) {
	f.asked = name
	if f.err != nil {
		return nil, false, f.err
	}
	if name == f.name {
		return f.data, f.found, nil
	}
	return nil, false, nil
}

// TestLoadProfileOnConnectApplies proves a found daemon profile overwrites the
// client's portable config in place while leaving machine-local fields alone.
func TestLoadProfileOnConnectApplies(t *testing.T) {
	blob, err := MarshalConfigProfile(sourceConfig())
	if err != nil {
		t.Fatalf("MarshalConfigProfile: %v", err)
	}
	be := &fakeProfileBackend{name: "team", data: blob, found: true}

	target := &config.Config{}
	target.Player = "mpv"               // machine-local — must survive
	target.DownloadDir = "/mnt/target"  // machine-local — must survive
	target.DaemonToken = "local-secret" // machine-local — must survive

	if err := LoadProfileOnConnect(context.Background(), be, target, "team"); err != nil {
		t.Fatalf("LoadProfileOnConnect: %v", err)
	}
	if be.asked != "team" {
		t.Errorf("GetProfile asked for %q, want team", be.asked)
	}
	if target.Theme != "gruvbox" || target.FeedMode != "mixed" || target.Keybindings.Quit != "Q" {
		t.Errorf("profile not applied: theme=%q feed=%q quit=%q", target.Theme, target.FeedMode, target.Keybindings.Quit)
	}
	if target.Player != "mpv" || target.DownloadDir != "/mnt/target" || target.DaemonToken != "local-secret" {
		t.Errorf("machine-local overwritten: player=%q dir=%q token=%q", target.Player, target.DownloadDir, target.DaemonToken)
	}
}

// TestLoadProfileOnConnectEmptyName is a no-op that never touches the backend.
func TestLoadProfileOnConnectEmptyName(t *testing.T) {
	be := &fakeProfileBackend{name: "team", data: []byte("{}"), found: true}
	target := &config.Config{}
	target.Theme = "keep"

	if err := LoadProfileOnConnect(context.Background(), be, target, ""); err != nil {
		t.Fatalf("LoadProfileOnConnect: %v", err)
	}
	if be.asked != "" {
		t.Errorf("empty name still queried the backend (asked %q)", be.asked)
	}
	if target.Theme != "keep" {
		t.Errorf("config mutated on empty name: theme=%q", target.Theme)
	}
}

// TestLoadProfileOnConnectMissing leaves the local config standing when the
// named profile isn't on the daemon yet.
func TestLoadProfileOnConnectMissing(t *testing.T) {
	be := &fakeProfileBackend{name: "team", found: false}
	target := &config.Config{}
	target.Theme = "keep"

	if err := LoadProfileOnConnect(context.Background(), be, target, "absent"); err != nil {
		t.Fatalf("LoadProfileOnConnect: %v", err)
	}
	if target.Theme != "keep" {
		t.Errorf("config mutated for a missing profile: theme=%q", target.Theme)
	}
}

// TestLoadProfileOnConnectError surfaces a genuine transport failure.
func TestLoadProfileOnConnectError(t *testing.T) {
	sentinel := errors.New("dial failed")
	be := &fakeProfileBackend{err: sentinel}
	if err := LoadProfileOnConnect(context.Background(), be, &config.Config{}, "team"); !errors.Is(err, sentinel) {
		t.Fatalf("LoadProfileOnConnect err = %v, want wrap of %v", err, sentinel)
	}
}

// TestApplyConfigProfileRejectsGarbage proves a corrupt blob errors instead of
// silently clobbering the config.
func TestApplyConfigProfileRejectsGarbage(t *testing.T) {
	target := &config.Config{}
	target.Theme = "keep"
	if err := ApplyConfigProfile(target, []byte("not json")); err == nil {
		t.Fatal("ApplyConfigProfile accepted invalid JSON")
	}
	if target.Theme != "keep" {
		t.Errorf("config mutated on decode failure: theme=%q", target.Theme)
	}
}
