package videotable

// HasTitle is implemented by row types whose primary display column is a plain text label.
type HasTitle interface {
	GetTitle() string
}

// HasAudioTitle is implemented by row types that can represent either audio or video content.
// The ♪ suffix is rendered by the column factory — not by the type itself.
type HasAudioTitle interface {
	GetBaseTitle() string
	IsAudio() bool
}

// HasChannelInfo is implemented by row types that have an associated channel.
// ChannelCol uses GetChannelID to look up aliases; falls back to GetChannelName.
type HasChannelInfo interface {
	GetChannelID() string
	GetChannelName() string
}

// HasCount is implemented by row types with a large integer count (views, subscribers).
type HasCount interface {
	GetCount() int64
}

// HasDate is implemented by row types with a display date.
// GetRawDate returns a date string in YYYYMMDD format; DateCol formats it via render.Date.
// For time.Time fields, format as "20060102" before returning.
type HasDate interface {
	GetRawDate() string
}

// HasDuration is implemented by row types with a playable duration.
// GetLastPositionSecs returns 0 when no playback position is known.
// DurationCol renders "pos/total" when position > 0, otherwise "total".
type HasDuration interface {
	GetDurationSecs() int
	GetLastPositionSecs() int
}

// HasIndicator is implemented by row types that show a status bullet (●/○).
type HasIndicator interface {
	GetIndicator() string
}

// HasLabel is implemented by row types with a Warning-styled fixed-width type label.
type HasLabel interface {
	GetLabel() string
}
