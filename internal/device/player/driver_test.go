package player

import (
	"slices"
	"strings"
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
			d := newDriver(tt.path, CookieSource{})
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
	// A remote source additionally asks yt-dlp to verify the stream; with no
	// cookie source configured that is the only option added.
	if want := []string{"--term-status-msg=", "--ytdl-raw-options-append=check-formats=", "https://y/v1"}; !slices.Equal(got, want) {
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

// TestMpvCookieArgs: mpv resolves YouTube through yt-dlp, which YouTube now
// refuses to serve anonymously, so a remote source has to carry the configured
// cookie source. A local file must not — it never goes through yt-dlp.
func TestMpvCookieArgs(t *testing.T) {
	tests := []struct {
		name    string
		cookies CookieSource
		source  string
		want    string // expected --ytdl-raw-options-append value, "" for none
	}{
		{"browser jar on a URL", CookieSource{Browser: "vivaldi+gnomekeyring"},
			"https://y/v1", "--ytdl-raw-options-append=cookies-from-browser=vivaldi+gnomekeyring"},
		{"cookie file on a URL", CookieSource{File: "/home/u/cookies.txt"},
			"https://y/v1", "--ytdl-raw-options-append=cookies=/home/u/cookies.txt"},
		{"file wins over browser", CookieSource{File: "/c.txt", Browser: "firefox"},
			"https://y/v1", "--ytdl-raw-options-append=cookies=/c.txt"},
		{"no source configured", CookieSource{}, "https://y/v1", ""},
		{"local file never gets cookies", CookieSource{Browser: "firefox"}, "/v/local.mp4", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &mpvDriver{path: "mpv", cookies: tt.cookies}
			for _, got := range [][]string{
				d.Args(tt.source, "T", 0),
				d.AudioArgs(tt.source, "T", 0), // audio-only playback needs them too
			} {
				if tt.want == "" {
					for _, a := range got {
						if strings.Contains(a, "cookies") {
							t.Errorf("unexpected cookie arg %q in %v", a, got)
						}
					}
					continue
				}
				if !slices.Contains(got, tt.want) {
					t.Errorf("args %v omit %q", got, tt.want)
				}
			}
		})
	}
}

// TestMpvCookiesNotCommaJoined: the value is passed with -append precisely so mpv
// cannot split a path containing a comma into two broken options.
func TestMpvCookiesNotCommaJoined(t *testing.T) {
	d := &mpvDriver{path: "mpv", cookies: CookieSource{File: "/od,d/cookies.txt"}}
	got := d.Args("https://y/v1", "", 0)
	if !slices.Contains(got, "--ytdl-raw-options-append=cookies=/od,d/cookies.txt") {
		t.Errorf("comma-bearing path not passed intact: %v", got)
	}
	for _, a := range got {
		if strings.HasPrefix(a, "--ytdl-raw-options=") {
			t.Errorf("must not use the comma-splitting form: %q", a)
		}
	}
}

// TestCookieSourceOption pins the precedence CookieSource shares with
// youtube.cookieArgs, so playback authenticates as the same user as every other
// yt-dlp call.
func TestCookieSourceOption(t *testing.T) {
	cases := []struct {
		src  CookieSource
		want string
	}{
		{CookieSource{}, ""},
		{CookieSource{Browser: "firefox"}, "cookies-from-browser=firefox"},
		{CookieSource{File: "/c.txt"}, "cookies=/c.txt"},
		{CookieSource{File: "/c.txt", Browser: "firefox"}, "cookies=/c.txt"},
	}
	for _, c := range cases {
		if got := c.src.ytdlpOption(); got != c.want {
			t.Errorf("%+v.ytdlpOption() = %q, want %q", c.src, got, c.want)
		}
	}
}

// TestMpvChecksFormats: a remote source must ask yt-dlp to verify the stream
// before the player gets it — YouTube hands out URLs that answer 403 forever, and
// without this a dead one reaches mpv and playback simply fails. A local file has
// nothing to verify.
func TestMpvChecksFormats(t *testing.T) {
	const want = "--ytdl-raw-options-append=check-formats="
	d := &mpvDriver{path: "mpv", cookies: CookieSource{Browser: "firefox"}}

	for _, got := range [][]string{
		d.Args("https://y/v1", "T", 0),
		d.AudioArgs("https://y/v1", "T", 0),
		d.Args("https://y/v1", "T", 30*time.Second), // also when resuming
	} {
		if !slices.Contains(got, want) {
			t.Errorf("remote args %v omit %q", got, want)
		}
	}
	for _, got := range [][]string{
		d.Args("/v/local.mp4", "T", 0),
		d.AudioArgs("/v/local.mp4", "T", 0),
	} {
		if slices.Contains(got, want) {
			t.Errorf("local args must not check formats: %v", got)
		}
	}
}

// TestMpvSourceStaysLast: mpv takes the source positionally, so every injected
// option has to land before it or mpv parses the URL as an option's value.
func TestMpvSourceStaysLast(t *testing.T) {
	d := &mpvDriver{path: "mpv", cookies: CookieSource{Browser: "firefox"}}
	for _, args := range [][]string{
		d.Args("https://y/v1", "T", 42*time.Second),
		d.AudioArgs("https://y/v1", "T", 0),
		d.Args("/v/local.mp4", "T", 5*time.Second),
	} {
		if last := args[len(args)-1]; last != "https://y/v1" && last != "/v/local.mp4" {
			t.Errorf("source is not the final argument: %v", args)
		}
	}
}
