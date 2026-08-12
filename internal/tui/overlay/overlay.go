package overlay

import (
	"encoding/json"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

// Overlay is a modal component drawn on top of the active tab's content.
// Implementations are value types; Update returns a mutated copy cast to Overlay.
type Overlay interface {
	tea.Model
	// Render places the overlay on top of behind and returns the composed view.
	// Any terminal side effect (e.g. the video-detail Kitty thumbnail) is emitted
	// separately via tea.Raw from the overlay's commands, never embedded here.
	Render(behind string, width, height int) string
	// InterceptsInput reports whether the overlay owns a focused text input.
	InterceptsInput() bool
	// WidthReduction is columns reserved on the right edge (0 for centered modals;
	// non-zero for the video-detail side panel).
	WidthReduction() int
	// HasFocus reports whether the overlay is currently capturing keyboard input.
	HasFocus() bool
	// ID is a stable, process-unique instance identifier. Root uses it to route an
	// overlay's async fetch results (tuipkg.OverlayAddressedMsg) back to it
	// regardless of stack position, so stacking a second overlay on top can't
	// steal the first's in-flight messages.
	ID() int64
}

// overlaySeq hands out process-unique overlay IDs. A plain monotonic counter is
// enough — IDs only need to be distinct among concurrently-live overlays, and
// int64 never wraps in a session.
var overlaySeq atomic.Int64

// identity is embedded in every Overlay implementation to supply ID(). Set it in
// the constructor via newIdentity(); it survives the value-copy that every
// Update returns, so the instance keeps one ID for its whole lifetime.
type identity struct{ id int64 }

// ID reports the overlay's process-unique instance identifier.
func (i identity) ID() int64 { return i.id }

// newIdentity allocates a fresh process-unique identity for a new overlay.
func newIdentity() identity { return identity{id: overlaySeq.Add(1)} }

// PopOverlayMsg is emitted by an overlay when it wants Root to close it.
type PopOverlayMsg struct{}

// ApplyConfigProfileMsg is emitted by the import overlay after a successful
// import when the user opted to apply the bundle's config profile. It carries
// the raw config JSON (from Bundle.Config) for Root to decode and apply onto the
// live config — Root owns the config, so the mutation happens on the main update
// loop rather than in a background overlay command.
type ApplyConfigProfileMsg struct {
	Config json.RawMessage
}

// FocusSwitchMsg is sent by Root to the top overlay to toggle its focus state.
type FocusSwitchMsg struct{}

// VideoClearMsg is sent by Root when the tab selection changes, so the overlay
// can immediately reset to a loading state before the debounce fetch fires.
type VideoClearMsg struct{}

// placeOverlayBox renders content inside a rounded bordered box and centers it
// over behind (via render.OverlayCenter), composing the two strings by
// overwriting matching character cells.
func placeOverlayBox(behind, content string, totalWidth, boxWidth int) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorAccent).
		Padding(1, 2).
		Width(boxWidth).
		Render(content)
	return render.OverlayCenter(behind, box, totalWidth)
}
