package domain

// SubscriptionState captures how a channel is subscribed. It replaces the older
// is_local boolean so a single channels table can hold subscribed, unsubscribed
// (but annotated), and blocked channels alike.
type SubscriptionState string

const (
	// SubNone means the channel is known but not subscribed (may still carry
	// alias/tags, e.g. a channel seen only in the recommended feed).
	SubNone SubscriptionState = "none"
	// SubYT means subscribed via the YouTube account.
	SubYT SubscriptionState = "subscribed_yt"
	// SubLocal means subscribed only locally (never pushed to YouTube).
	SubLocal SubscriptionState = "subscribed_local"
)

// FetchOffsetComplete is the sentinel Channel.FetchedVideos value meaning the
// channel's entire back-catalog has been crawled: no deep crawl remains, only
// latest-N refreshes. Negative so it can never collide with a real list offset.
const FetchOffsetComplete int64 = -1

// Channel is a YouTube channel (subscribed, blocked, or a bare search/feed result).
type Channel struct {
	ID          string
	Name        string
	Alias       string   // user-defined display name override
	Tags        []string // user-defined categories
	URL         string
	Subscribers int64
	IsLocal     bool // DEPRECATED: use State; retained until the is_local column is dropped
	// State is the subscription state (none / yt / local). Authoritative going
	// forward; IsLocal is kept in sync for callers not yet migrated.
	State SubscriptionState
	// Blocked reports whether the channel is on the blocklist. Invariant:
	// Blocked == true implies State == SubNone.
	Blocked bool
	// VideosRefreshedAt is the unix-seconds time this channel's videos were last
	// fetched from the source (0 = never). Kept as an int (not time.Time) so the
	// struct stays small enough for by-value range loops. Both the full pull and
	// the latest-N refresh advance it, so it means "last touched", not "fully
	// crawled" — use FetchedVideos/FullyCrawled for the latter.
	VideosRefreshedAt int64
	// FetchedVideos tracks the progress of the deep back-catalog crawl so it can
	// resume across runs instead of restarting from the top:
	//   0                    = never crawled (start from the newest video)
	//   N > 0                = paused mid-crawl; resume at list offset N
	//   FetchOffsetComplete  = entire catalog crawled (deep crawl done; only
	//                          latest-N refreshes from now on)
	// The exact video count isn't kept once complete — the authoritative count is
	// COUNT(*) on channel_videos — so this field's whole job is "where to resume
	// and am I done". Use FullyCrawled/ResumeOffset rather than the raw value.
	FetchedVideos int64
	// LastActivityAt is the unix-seconds time this channel last showed activity
	// (0 = never): drilled into, seen in a recommended-feed refresh, or a video
	// of it played/streamed/downloaded. Drives the stale-tagged-channel filter
	// (see channels.IsStale). Kept as an int for the same by-value-loop reason.
	LastActivityAt int64
}

// SubState returns the channel's subscription state, deriving it from the legacy
// IsLocal flag when State is unset so callers that only populate IsLocal keep working.
func (ch Channel) SubState() SubscriptionState {
	if ch.State != "" {
		return ch.State
	}
	if ch.IsLocal {
		return SubLocal
	}
	return SubYT
}

// IsSubscribed reports whether the channel is subscribed (locally or via YouTube).
func (ch Channel) IsSubscribed() bool { return ch.SubState() != SubNone }

func (ch Channel) DisplayName() string {
	if ch.Alias != "" {
		return ch.Alias
	}
	return ch.Name
}

func (ch Channel) GetTitle() string { return ch.DisplayName() }

// FullyCrawled reports whether the channel's entire back-catalog has been pulled
// (deep crawl complete). Once true, backfill only latest-N refreshes it.
func (ch Channel) FullyCrawled() bool { return ch.FetchedVideos == FetchOffsetComplete }

// ResumeOffset is the list offset a deep crawl should resume from: 0 for a
// never-crawled or already-complete channel (a complete one isn't deep-crawled),
// else the paused mid-crawl offset. Keeps the FetchedVideos sentinel out of
// caller arithmetic.
func (ch Channel) ResumeOffset() int64 {
	if ch.FetchedVideos < 0 {
		return 0
	}
	return ch.FetchedVideos
}
