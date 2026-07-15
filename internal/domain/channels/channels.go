// Package channels owns the subscribed-channel list together with its lookup
// index, keeping the two in sync. Held by value on the Model and mutated
// through pointer methods (same pattern as feed.Feed and library.Library).
package channels

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

// DB is the narrow persistence interface required for unsubscribe operations.
type DB interface {
	RemoveSubscribedChannel(channelID string) error
	DeleteChannelVideos(channelID string) error
}

// ChannelSet owns the subscribed-channel slice together with a membership
// index (channel ID → true, "name:lowercaseName" → true). All mutations go
// through its methods so the slice and index never diverge.
type ChannelSet struct {
	channels []domain.Channel
	index    map[string]bool
	db       DB
	ytClient *youtube.YTClient
}

// New builds a ChannelSet from an initial slice. db and ytClient are injected
// for use by Unsubscribe; ytClient may be nil until browser cookies are ready.
func New(channels []domain.Channel, db DB, ytClient *youtube.YTClient) ChannelSet {
	var s ChannelSet
	s.db = db
	s.ytClient = ytClient
	s.rebuild(channels)
	return s
}

// SetYTClient updates the YouTube client after it becomes available.
func (s *ChannelSet) SetYTClient(client *youtube.YTClient) { s.ytClient = client }

// rebuild replaces the slice and reconstructs the index from scratch.
func (s *ChannelSet) rebuild(channels []domain.Channel) {
	s.channels = channels
	s.index = make(map[string]bool, len(channels)*2)
	for _, ch := range channels {
		s.addToIndex(ch)
	}
}

func (s *ChannelSet) addToIndex(ch domain.Channel) {
	if ch.ID != "" {
		s.index[ch.ID] = true
	}
	if ch.Name != "" {
		s.index["name:"+strings.ToLower(ch.Name)] = true
	}
}

func (s *ChannelSet) removeFromIndex(id, name string) {
	delete(s.index, id)
	if name != "" {
		delete(s.index, "name:"+strings.ToLower(name))
	}
}

// ── Reads ─────────────────────────────────────────────────────────────────────

func (s *ChannelSet) Channels() []domain.Channel { return s.channels }
func (s *ChannelSet) Len() int                   { return len(s.channels) }

// Index returns the membership map for read-only use (e.g. feed.FilterSubscribed).
// Callers must not mutate the returned map.
func (s *ChannelSet) Index() map[string]bool { return s.index }

// ByID returns the channel with the given ID, or (zero, false) if not found.
func (s *ChannelSet) ByID(id string) (domain.Channel, bool) {
	for _, ch := range s.channels {
		if ch.ID == id {
			return ch, true
		}
	}
	return domain.Channel{}, false
}

func (s *ChannelSet) isLocal(id string) bool {
	ch, ok := s.ByID(id)
	return ok && ch.IsLocal
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

// Remove drops the channel with the given ID and clears its index entries.
func (s *ChannelSet) Remove(id, name string) {
	out := make([]domain.Channel, 0, len(s.channels))
	for _, ch := range s.channels {
		if ch.ID != id {
			out = append(out, ch)
		}
	}
	s.channels = out
	s.removeFromIndex(id, name)
}

// Unsubscribe removes the channel from the set and returns the appropriate
// backend command (local DB removal or YouTube API call). Returns (nil, false)
// if the channel cannot be unsubscribed — remote channel with no YouTube client.
func (s *ChannelSet) Unsubscribe(id, name string) (tea.Cmd, bool) {
	local := s.isLocal(id)
	if !local && s.ytClient == nil {
		return nil, false
	}
	s.Remove(id, name)
	if local {
		return localUnsubscribeCmd(s.db, id), true
	}
	return youtube.UnsubscribeFromChannel(s.ytClient, id, name), true
}

func localUnsubscribeCmd(db DB, channelID string) tea.Cmd {
	return func() tea.Msg {
		_ = db.RemoveSubscribedChannel(channelID)
		_ = db.DeleteChannelVideos(channelID)
		return nil
	}
}

// SetAlias updates the alias of the channel with the given ID in place.
func (s *ChannelSet) SetAlias(id, alias string) {
	for i, ch := range s.channels {
		if ch.ID == id {
			s.channels[i].Alias = alias
			return
		}
	}
}

// SetTags updates the tags of the channel with the given ID in place.
func (s *ChannelSet) SetTags(id string, tags []string) {
	for i, ch := range s.channels {
		if ch.ID == id {
			s.channels[i].Tags = tags
			return
		}
	}
}

// SyncFromYT merges a fresh YT-fetched channel list into the set.
// Local-only channels are preserved. Alias and tag fields are carried over
// from the current set when membership changes. Returns true if the set
// membership changed (channels added or removed).
func (s *ChannelSet) SyncFromYT(ytChannels []domain.Channel) bool {
	ytIDs := make(map[string]bool, len(ytChannels))
	for _, ch := range ytChannels {
		ytIDs[ch.ID] = true
	}
	var localOnly []domain.Channel
	for _, ch := range s.channels {
		if !ytIDs[ch.ID] {
			localOnly = append(localOnly, ch)
		}
	}
	merged := append(ytChannels, localOnly...)
	if !membershipChanged(s.channels, merged) {
		return false
	}
	existing := make(map[string]domain.Channel, len(s.channels))
	for _, ch := range s.channels {
		existing[ch.ID] = ch
	}
	for i, ch := range merged {
		if old, ok := existing[ch.ID]; ok {
			merged[i].Alias = old.Alias
			merged[i].Tags = old.Tags
		}
	}
	s.rebuild(merged)
	return true
}

func membershipChanged(a, b []domain.Channel) bool {
	if len(a) != len(b) {
		return true
	}
	ids := make(map[string]bool, len(a))
	for _, ch := range a {
		ids[ch.ID] = true
	}
	for _, ch := range b {
		if !ids[ch.ID] {
			return true
		}
	}
	return false
}
