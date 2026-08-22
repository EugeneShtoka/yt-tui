package player

import (
	"slices"
	"testing"
	"time"
)

func TestNewDriverSelection(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		wantDBus string
	}{
		{"mpv absolute", "/usr/bin/mpv", "org.mpris.MediaPlayer2.mpv"},
		{"mpv bare", "mpv", "org.mpris.MediaPlayer2.mpv"},
		{"vlc", "/usr/bin/vlc", "org.mpris.MediaPlayer2.vlc"},
		{"cvlc", "cvlc", "org.mpris.MediaPlayer2.vlc"},
		{"generic ffplay", "/opt/bin/ffplay", "org.mpris.MediaPlayer2.ffplay"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newDriver(tt.path)
			if got := d.Path(); got != tt.path {
				t.Errorf("Path() = %q, want %q", got, tt.path)
			}
			if got := d.DBusName(); got != tt.wantDBus {
				t.Errorf("DBusName() = %q, want %q", got, tt.wantDBus)
			}
		})
	}
}

func TestMpvArgs(t *testing.T) {
	d := &mpvDriver{path: "mpv"}

	// Local file with a title and a resume position: force the title and seek.
	got := d.Args("/v/local.mp4", "My Title", 90*time.Second)
	want := []string{"--term-status-msg=", "--force-media-title=My Title", "--start=90", "/v/local.mp4"}
	if !slices.Equal(got, want) {
		t.Errorf("local Args = %v, want %v", got, want)
	}

	// HTTP source: no --force-media-title (yt-dlp sets it), no seek at 0.
	got = d.Args("https://y/v1", "My Title", 0)
	if slices.Contains(got, "--force-media-title=My Title") {
		t.Errorf("http Args must not force media title: %v", got)
	}
	if want := []string{"--term-status-msg=", "https://y/v1"}; !slices.Equal(got, want) {
		t.Errorf("http Args = %v, want %v", got, want)
	}

	// AudioArgs prepends --no-video.
	audio := d.AudioArgs("/v/local.mp4", "", 0)
	if len(audio) == 0 || audio[0] != "--no-video" {
		t.Errorf("AudioArgs must lead with --no-video: %v", audio)
	}
}

func TestVlcArgs(t *testing.T) {
	d := &vlcDriver{path: "vlc"}

	if got, want := d.Args("s", "", 30*time.Second), []string{"--start-time=30", "s"}; !slices.Equal(got, want) {
		t.Errorf("Args with resume = %v, want %v", got, want)
	}
	if got, want := d.Args("s", "", 0), []string{"s"}; !slices.Equal(got, want) {
		t.Errorf("Args without resume = %v, want %v", got, want)
	}
	if audio := d.AudioArgs("s", "", 0); len(audio) == 0 || audio[0] != "--novideo" {
		t.Errorf("AudioArgs must lead with --novideo: %v", audio)
	}
}

func TestGenericArgs(t *testing.T) {
	d := &genericDriver{path: "/opt/bin/ffplay"}

	if got, want := d.Args("src", "title", 42*time.Second), []string{"src"}; !slices.Equal(got, want) {
		t.Errorf("generic Args = %v, want just [source] %v", got, want)
	}
	// AudioArgs mirrors Args for a generic player (no audio-only flag known).
	if got, want := d.AudioArgs("src", "title", 0), d.Args("src", "title", 0); !slices.Equal(got, want) {
		t.Errorf("generic AudioArgs = %v, want %v", got, want)
	}
	if got, want := d.DBusName(), "org.mpris.MediaPlayer2.ffplay"; got != want {
		t.Errorf("DBusName() = %q, want %q", got, want)
	}
}
