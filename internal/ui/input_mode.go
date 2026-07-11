package ui

// inputMode enumerates the mutually-exclusive text-input modes that capture all
// keystrokes ahead of normal navigation dispatch. Exactly one is active at a
// time (modeNormal = none); the compiler now enforces the exclusivity that was
// previously only maintained by hand across a set of independent booleans gated
// by if-ladder order in handleKey.
//
// Overlays (add / link / chapter / video-detail) and the help screen are
// deliberately NOT part of this enum: link and chapter overlays stack on top of
// the video-detail panel (opened from within it, restored on close by the order
// of handleKey's overlay ladder), so they are not mutually exclusive and stay as
// separate fields.
type inputMode int

const (
	modeNormal         inputMode = iota
	modeCommand                  // ":" command line
	modeLocalFilter              // "/" filter box (Local tab)
	modeSearchInput              // Search tab query box focused
	modeCreateType               // playlist-create type selector (local vs YouTube)
	modeCreatePlaylist           // playlist-create name entry
	modeChannelEdit              // channel alias/tags editing (subChEditKind holds which)
)

// enterMode / exitMode centralize mode transitions. Because mode is a single
// field, entering a mode implicitly clears any other — the invariant the old
// scattered `= true` / `= false` bool assignments maintained by hand.
func (m *Model) enterMode(mode inputMode) { m.mode = mode }
func (m *Model) exitMode()                { m.mode = modeNormal }
