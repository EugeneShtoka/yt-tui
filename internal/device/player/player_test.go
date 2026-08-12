package player

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/EugeneShtoka/yt-tui/internal/config"
)

func TestBaseName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"mpv", "mpv"},
		{"/usr/bin/mpv", "mpv"},
		{"/usr/local/bin/vlc", "vlc"},
		{"cvlc", "cvlc"},
		{"/opt/ff/ffplay", "ffplay"},
		{"/usr/bin/mpv-wrapped", "mpv"}, // truncated at first '-'
		{"flatpak-mpv", "flatpak"},      // '-' split takes the prefix
		{"-leading", "-leading"},        // idx==0 → not truncated
	}
	for _, c := range cases {
		if got := baseName(c.in); got != c.want {
			t.Errorf("baseName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNewDriverSelectsByBaseName(t *testing.T) {
	cases := []struct {
		path   string
		expect any
	}{
		{"/usr/bin/mpv", &mpvDriver{}},
		{"/usr/bin/vlc", &vlcDriver{}},
		{"/usr/bin/cvlc", &vlcDriver{}},
		{"/usr/bin/ffplay", &genericDriver{}},
		{"/usr/bin/something-else", &genericDriver{}},
	}
	for _, c := range cases {
		got := newDriver(c.path)
		switch c.expect.(type) {
		case *mpvDriver:
			if _, ok := got.(*mpvDriver); !ok {
				t.Errorf("newDriver(%q) = %T, want *mpvDriver", c.path, got)
			}
		case *vlcDriver:
			if _, ok := got.(*vlcDriver); !ok {
				t.Errorf("newDriver(%q) = %T, want *vlcDriver", c.path, got)
			}
		case *genericDriver:
			if _, ok := got.(*genericDriver); !ok {
				t.Errorf("newDriver(%q) = %T, want *genericDriver", c.path, got)
			}
		}
	}
}

// TestResolvePlayerPrefersConfiguredPlayer: a configured player found on PATH is
// returned ahead of the built-in fallbacks.
func TestResolvePlayerPrefersConfiguredPlayer(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "myplayer")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // test fixture executable
		t.Fatalf("write fake player: %v", err)
	}
	t.Setenv("PATH", dir)

	got, err := resolvePlayer(&config.Config{ClientConfig: config.ClientConfig{Player: "myplayer"}})
	if err != nil {
		t.Fatalf("resolvePlayer: %v", err)
	}
	if got != bin {
		t.Errorf("resolvePlayer = %q, want %q", got, bin)
	}
}

// TestResolvePlayerErrorsWhenNoneFound: with an empty PATH and no configured
// player, resolution fails with a helpful error rather than returning "".
func TestResolvePlayerErrorsWhenNoneFound(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir → nothing resolvable
	if _, err := resolvePlayer(&config.Config{ClientConfig: config.ClientConfig{Player: ""}}); err == nil {
		t.Error("expected an error when no player is on PATH")
	}
}
