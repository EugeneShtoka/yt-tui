package channels

import (
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// IsStale reports whether ch is a "stale" tagged channel as of now: it carries
// at least one tag, is not subscribed, is not blocked, and has had no recorded
// activity within thresholdDays (see domain.Channel.LastActivityAt for what
// counts as activity). It is the counterpart to the auto-hide filter — the
// Channels/Tags panels hide exactly these from their other modes and surface
// them in the stale mode.
//
// Subscribed channels are deliberately exempt: the user follows them on purpose,
// so their tags should never be auto-hidden. A non-positive thresholdDays
// disables the check (nothing is ever stale).
func IsStale(ch domain.Channel, now time.Time, thresholdDays int) bool {
	if thresholdDays <= 0 {
		return false
	}
	if ch.IsSubscribed() || ch.Blocked || len(ch.Tags) == 0 {
		return false
	}
	cutoff := now.Add(-time.Duration(thresholdDays) * 24 * time.Hour).Unix()
	return ch.LastActivityAt < cutoff
}
