// Package channels owns the subscribed-channel list together with its lookup
// index, keeping the two in sync. Held by value on the Model and mutated
// through pointer methods (same pattern as feed.Feed and library.Library).
package channels

import (
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// ChannelSet owns the subscribed-channel slice together with a membership
// index (channel ID → true, "name:lowercaseName" → true). All mutations go
// through its methods so the slice and index never diverge.
type ChannelSet struct {
	channels []domain.Channel
	index    map[string]bool
	// nameRefs counts how many channels currently share each "name:..." index
	// key, since IDs are unique but two distinct YouTube channels can share a
	// display name. Without this, removing one same-named channel deleted the
	// shared key outright and delisted the other from name-based lookups
	// (e.g. feed.FilterSubscribed's fallback match).
	nameRefs map[string]int
}

// New builds a ChannelSet from an initial slice.
func New(channels []domain.Channel) ChannelSet {
	var s ChannelSet
	s.rebuild(channels)
	return s
}

// rebuild replaces the slice and reconstructs the index from scratch.
func (s *ChannelSet) rebuild(channels []domain.Channel) {
	s.channels = channels
	s.index = make(map[string]bool, len(channels)*2)
	s.nameRefs = make(map[string]int, len(channels))
	for i := range channels {
		s.addToIndex(channels[i])
	}
}

func (s *ChannelSet) addToIndex(ch domain.Channel) {
	if ch.ID != "" {
		s.index[ch.ID] = true
	}
	if ch.Name != "" {
		key := "name:" + strings.ToLower(ch.Name)
		s.nameRefs[key]++
		s.index[key] = true
	}
}

func (s *ChannelSet) removeFromIndex(id, name string) {
	delete(s.index, id)
	if name == "" {
		return
	}
	key := "name:" + strings.ToLower(name)
	if s.nameRefs[key] > 1 {
		s.nameRefs[key]--
		return
	}
	delete(s.nameRefs, key)
	delete(s.index, key)
}

// ── Reads ─────────────────────────────────────────────────────────────────────

func (s *ChannelSet) Channels() []domain.Channel { return s.channels }
func (s *ChannelSet) Len() int                   { return len(s.channels) }

// Index returns the membership map for read-only use (e.g. feed.FilterSubscribed).
// Callers must not mutate the returned map.
func (s *ChannelSet) Index() map[string]bool { return s.index }

// ByID returns the channel with the given ID, or (zero, false) if not found.
func (s *ChannelSet) ByID(id string) (domain.Channel, bool) {
	for i := range s.channels {
		if s.channels[i].ID == id {
			return s.channels[i], true
		}
	}
	return domain.Channel{}, false
}

// ── Mutations ─────────────────────────────────────────────────────────────────

// Subscribe appends ch if its ID is not already present. Returns false if duplicate.
func (s *ChannelSet) Subscribe(ch domain.Channel) bool {
	if s.index[ch.ID] {
		return false
	}
	s.channels = append(s.channels, ch)
	s.addToIndex(ch)
	return true
}

// Remove drops the channel from the set and clears its index entries.
func (s *ChannelSet) Remove(ch domain.Channel) {
	out := make([]domain.Channel, 0, len(s.channels))
	for i := range s.channels {
		if s.channels[i].ID != ch.ID {
			out = append(out, s.channels[i])
		}
	}
	s.channels = out
	s.removeFromIndex(ch.ID, ch.Name)
}

// Unsubscribe finds the channel by ID, removes it, and returns it.
// Returns (zero, false) if not found.
func (s *ChannelSet) Unsubscribe(id string) (domain.Channel, bool) {
	ch, found := s.ByID(id)
	if !found {
		return domain.Channel{}, false
	}
	s.Remove(ch)
	return ch, true
}

// SetAlias updates the alias of the channel with the given ID in place.
func (s *ChannelSet) SetAlias(id, alias string) {
	for i := range s.channels {
		if s.channels[i].ID == id {
			s.channels[i].Alias = alias
			return
		}
	}
}

// SetTags updates the tags of the channel with the given ID in place.
func (s *ChannelSet) SetTags(id string, tags []string) {
	for i := range s.channels {
		if s.channels[i].ID == id {
			s.channels[i].Tags = tags
			return
		}
	}
}

// Set replaces the channel sharing ch's ID in place, or appends it when absent,
// keeping the membership index in sync. It is the general "upsert one row"
// mutation used by state transitions (block/unblock) and their optimistic
// rollback, where the whole channel value (state, blocked, annotations) changes
// at once rather than a single field.
func (s *ChannelSet) Set(ch domain.Channel) {
	for i := range s.channels {
		if s.channels[i].ID == ch.ID {
			old := s.channels[i]
			s.channels[i] = ch
			// Re-key the index only when the identity keys actually change; the
			// common case (state/blocked/annotation edit) leaves them untouched.
			if old.Name != ch.Name {
				s.removeFromIndex(old.ID, old.Name)
				s.addToIndex(ch)
			}
			return
		}
	}
	s.channels = append(s.channels, ch)
	s.addToIndex(ch)
}

// Sync merges a fresh YT-fetched channel list with an existing set:
// local-only channels (present in existing but absent from ytChannels) are
// preserved, and alias/tag fields are carried over from existing entries.
func Sync(existing, ytChannels []domain.Channel) []domain.Channel {
	ytIDs := make(map[string]bool, len(ytChannels))
	for i := range ytChannels {
		ytIDs[ytChannels[i].ID] = true
	}
	var localOnly []domain.Channel
	for i := range existing {
		if !ytIDs[existing[i].ID] {
			localOnly = append(localOnly, existing[i])
		}
	}
	ytChannels = append(ytChannels, localOnly...)
	existingMap := make(map[string]domain.Channel, len(existing))
	for i := range existing {
		existingMap[existing[i].ID] = existing[i]
	}
	for i := range ytChannels {
		if old, ok := existingMap[ytChannels[i].ID]; ok {
			ytChannels[i].Alias = old.Alias
			ytChannels[i].Tags = old.Tags
		}
	}
	return ytChannels
}

// SyncFromYT merges a fresh YT-fetched channel list into the set.
// Returns true if membership changed.
func (s *ChannelSet) SyncFromYT(ytChannels []domain.Channel) bool {
	merged := Sync(s.channels, ytChannels)
	if !membershipChanged(s.channels, merged) {
		return false
	}
	s.rebuild(merged)
	return true
}

func membershipChanged(a, b []domain.Channel) bool {
	if len(a) != len(b) {
		return true
	}
	ids := make(map[string]bool, len(a))
	for i := range a {
		ids[a[i].ID] = true
	}
	for i := range b {
		if !ids[b[i].ID] {
			return true
		}
	}
	return false
}
