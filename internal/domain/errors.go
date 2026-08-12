package domain

import "errors"

// ErrYTNotInitialized is returned by YouTube API mutations (subscribe,
// playlist operations) when InitYTClient has not yet succeeded for this
// session's browser-cookie auth.
var ErrYTNotInitialized = errors.New("youtube api not initialized")

// ErrChannelBlocked is returned when a subscription transition is attempted on a
// blocked channel. Per the block invariant (blocked == true ⟹ State == SubNone),
// a channel must be unblocked before it can be (re)subscribed.
var ErrChannelBlocked = errors.New("channel is blocked; unblock before subscribing")

// ErrProfilesUnavailable is returned when a config-profile save is attempted but
// the daemon's profile store could not be initialized (e.g. the profiles
// directory wasn't creatable). Reads degrade to an empty set instead.
var ErrProfilesUnavailable = errors.New("config profile store unavailable")
