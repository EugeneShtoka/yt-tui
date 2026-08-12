package tab

import (
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/channels"
)

// mergeUniverse returns the channel universe backing the Channels/Tags panels:
// every stored channel plus synthesized state=none rows for recommended-feed
// channels not already stored. Built on a fresh slice so it never aliases the
// caller's backing array.
func mergeUniverse(stored []domain.Channel, recVideos []domain.Video) []domain.Channel {
	synth := channels.SynthesizeRec(stored, recVideos)
	out := make([]domain.Channel, 0, len(stored)+len(synth))
	out = append(out, stored...)
	out = append(out, synth...)
	return out
}

// srcMode is the source-filter partition shared by the Channels and Tags panels
// (the counterpart to the Feed panel's feedMode). It selects which slice of the
// channel universe — subscribed, recommended-feed, their union, or blocked — a
// panel shows. Chosen from the PanelMode picker.
//
// The universe folds the recommended-feed channels in via
// channels.SynthesizeRec, and inRecFeed marks the channels currently seen in the
// recommended feed (see channels.RecFeedIDs); the predicate uses it so the
// recommended/mixed modes reflect the live feed rather than every row ever
// stored. Blocked channels are excluded from recommended/mixed by design —
// blocked is its own filter (Channels only).
type srcMode int

const (
	srcRecommended srcMode = iota // rec-feed channels not (yet) subscribed — the tagging surface
	srcSubscribed                 // subscription_state != none
	srcMixed                      // every non-blocked known channel (subscribed ∪ rec-feed ∪ annotated)
	srcBlocked                    // blocked channels only (Channels panel only)
	srcStale                      // stale tagged channels only (the auto-hide counterpart)
)

// channelModes is the Channels panel's picker option list. The srcStale entry is
// the counterpart to the auto-hide filter: it surfaces exactly the tagged,
// unsubscribed channels the other modes drop when hiding is on.
var channelModes = []srcMode{srcRecommended, srcSubscribed, srcMixed, srcBlocked, srcStale}

// tagModes is the Tags panel's picker option list. Tags partition on the same
// recommended/subscribed/mixed/stale axis as Channels but have no blocked mode
// (tags live on channels regardless of subscription; a blocked-tags view is
// meaningless).
var tagModes = []srcMode{srcRecommended, srcSubscribed, srcMixed, srcStale}

// label is the picker option + header label for the mode.
func (m srcMode) label() string {
	switch m {
	case srcRecommended:
		return "Recommended"
	case srcMixed:
		return "Mixed"
	case srcBlocked:
		return "Blocked"
	case srcStale:
		return "Stale"
	default:
		return "Subscribed"
	}
}

// includes reports whether ch belongs in this mode's partition. inRecFeed is
// whether the channel currently appears in the recommended feed. Blocked
// channels are excluded from every mode except srcBlocked.
func (m srcMode) includes(ch domain.Channel, inRecFeed bool) bool {
	switch m {
	case srcBlocked:
		return ch.Blocked
	case srcRecommended:
		return inRecFeed && !ch.IsSubscribed() && !ch.Blocked
	case srcSubscribed:
		return ch.IsSubscribed() && !ch.Blocked
	default: // srcMixed
		return !ch.Blocked
	}
}

// parseSrcMode maps a config string to a srcMode, defaulting to subscribed. The
// legacy "all" value (the pre-Phase-13 Channels view) maps to mixed, its closest
// equivalent.
func parseSrcMode(s string) srcMode {
	switch s {
	case "recommended":
		return srcRecommended
	case "mixed", "all":
		return srcMixed
	case "blocked":
		return srcBlocked
	case "stale":
		return srcStale
	default:
		return srcSubscribed
	}
}

// staleFilter carries the stale-tagged-channel partition config for a panel. It
// is orthogonal to the srcMode source axis: when Hide is on, the non-stale modes
// exclude stale channels; the srcStale mode always shows exactly the stale set
// (regardless of Hide). Days is the stale threshold (see channels.IsStale).
type staleFilter struct {
	hide bool
	days int
}

// isStale reports whether ch is a stale tagged channel as of now. A channel
// currently in the recommended feed is treated as active (never stale) even if
// its stored timestamp hasn't caught up — being in the feed IS activity.
func (f staleFilter) isStale(ch domain.Channel, inRecFeed bool, now time.Time) bool {
	if inRecFeed {
		return false
	}
	return channels.IsStale(ch, now, f.days)
}

// selectChannels returns the channel universe filtered by the source mode and
// the stale partition, as of now. In srcStale it returns exactly the stale
// tagged channels; otherwise it returns the mode's partition, excluding stale
// channels when the filter's Hide is on. recFeedIDs reports rec-feed membership
// per channel. Shared by the Channels and Tags panels (DRY).
func selectChannels(universe []domain.Channel, mode srcMode, recFeedIDs map[string]bool, sf staleFilter, now time.Time) []domain.Channel {
	out := make([]domain.Channel, 0, len(universe))
	for i := range universe {
		ch := universe[i]
		inRec := recFeedIDs[ch.ID]
		stale := sf.isStale(ch, inRec, now)
		if mode == srcStale {
			if stale {
				out = append(out, ch)
			}
			continue
		}
		if !mode.includes(ch, inRec) {
			continue
		}
		if sf.hide && stale {
			continue
		}
		out = append(out, ch)
	}
	return out
}

// modeLabels turns a mode option list into the picker's label slice.
func modeLabels(modes []srcMode) []string {
	labels := make([]string, len(modes))
	for i, m := range modes {
		labels[i] = m.label()
	}
	return labels
}

// modeIndex returns the position of mode within modes (0 if absent), used to open
// the picker highlighted on the active mode.
func modeIndex(modes []srcMode, mode srcMode) int {
	for i, m := range modes {
		if m == mode {
			return i
		}
	}
	return 0
}
