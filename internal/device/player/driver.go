package player

import (
	"strings"
	"time"
)

// Driver abstracts all player-specific CLI arguments.
type Driver interface {
	Path() string
	Args(source, title string, startAt time.Duration) []string
	AudioArgs(source, title string, startAt time.Duration) []string
	DBusName() string
}

// CookieSource tells a driver where the player's URL resolver should read YouTube
// cookies from. Playback resolves YouTube URLs through yt-dlp just like every
// other yt-dlp call in the app, so it needs the same cookie source: YouTube now
// refuses anonymous playback extraction outright ("Sign in to confirm you're not
// a bot"), which made playback the one broken path while listing, metadata and
// downloads — all of which already pass cookies — kept working.
//
// A cookie file takes precedence over a browser jar, mirroring the precedence in
// youtube.cookieArgs so both paths authenticate as the same user.
type CookieSource struct {
	File    string // explicit Netscape cookie file
	Browser string // browser cookie jar spec, e.g. "vivaldi+gnomekeyring"
}

// ytdlpOption renders the source as one yt-dlp option in mpv's
// --ytdl-raw-options "key=value" form, or "" when no source is configured.
func (c CookieSource) ytdlpOption() string {
	switch {
	case c.File != "":
		return "cookies=" + c.File
	case c.Browser != "":
		return "cookies-from-browser=" + c.Browser
	default:
		return ""
	}
}

// isRemote reports whether a source is a URL the player has to resolve over the
// network rather than a path it can open directly. Only remote sources go through
// yt-dlp, so only they need cookies — and only they get their title from it.
func isRemote(source string) bool { return strings.HasPrefix(source, "http") }
