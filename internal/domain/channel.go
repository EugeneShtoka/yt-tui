package domain

// Channel is a YouTube channel (subscribed or search result).
type Channel struct {
	ID          string
	Name        string
	Alias       string   // user-defined display name override
	Tags        []string // user-defined categories
	URL         string
	Subscribers int64
	IsLocal     bool // subscribed locally, not via YouTube API
}

func (ch Channel) DisplayName() string {
	if ch.Alias != "" {
		return ch.Alias
	}
	return ch.Name
}
