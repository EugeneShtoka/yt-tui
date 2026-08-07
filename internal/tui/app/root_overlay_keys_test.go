package app

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	ovpkg "github.com/EugeneShtoka/yt-tui/internal/tui/overlay"
)

// fakeOverlay is a minimal ovpkg.Overlay double that records every msg it
// receives so tests can assert routing without a real overlay's side effects.
type fakeOverlay struct {
	id         int64
	intercepts bool
	hasFocus   bool
	received   []tea.Msg
}

func (o fakeOverlay) ID() int64     { return o.id }
func (o fakeOverlay) Init() tea.Cmd { return nil }
func (o fakeOverlay) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	o.received = append(o.received, msg)
	return o, nil
}
func (o fakeOverlay) View() tea.View                        { return tea.NewView("") }
func (o fakeOverlay) Render(behind string, _, _ int) string { return behind }
func (o fakeOverlay) InterceptsInput() bool                 { return o.intercepts }
func (o fakeOverlay) WidthReduction() int                   { return 0 }
func (o fakeOverlay) HasFocus() bool                        { return o.hasFocus }

// fakeTab is a minimal tuipkg.Tab double for the same purpose.
type fakeTab struct {
	intercepts bool
	received   []tea.Msg
}

func (t fakeTab) Init() tea.Cmd { return nil }
func (t fakeTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	t.received = append(t.received, msg)
	return t, nil
}
func (t fakeTab) View() tea.View                      { return tea.NewView("") }
func (t fakeTab) ID() tuipkg.TabID                    { return tuipkg.TabFeed }
func (t fakeTab) Title() string                       { return "fake" }
func (t fakeTab) ShortHelp() []key.Binding            { return nil }
func (t fakeTab) InterceptsInput() bool               { return t.intercepts }
func (t fakeTab) SelectedVideo() (domain.Video, bool) { return domain.Video{}, false }
func (t fakeTab) Loading() bool                       { return false }

// fakeAddressedMsg is an overlay-private async result double: it satisfies
// tuipkg.OverlayAddressedMsg via the embedded target, so Root routes it by
// overlay ID rather than stack position.
type fakeAddressedMsg struct{ tuipkg.OverlayTarget }

// An overlay's async result must reach the exact instance that spawned it —
// matched by ID — even when another overlay is stacked on top, and must never
// leak to the top overlay. Regression test for the add-to-playlist-over-info-
// panel starvation: stacking a second overlay used to steal the panel's
// in-flight details fetch because delivery was keyed on stack position.
func TestHandleBroadcast_OverlayAddressedMsg_RoutesByIDNotStackPosition(t *testing.T) {
	bottom := fakeOverlay{id: 1}
	top := fakeOverlay{id: 2}
	r := Root{keys: testKeyMap(), tabs: []tuipkg.Tab{fakeTab{}}, overlays: []ovpkg.Overlay{bottom, top}}

	msg := fakeAddressedMsg{tuipkg.OverlayTarget{ID: 1}} // addressed to the bottom overlay
	r2, _ := r.handleBroadcast(msg)

	if got := r2.overlays[0].(fakeOverlay).received; len(got) != 1 || got[0] != tea.Msg(msg) {
		t.Fatalf("bottom overlay (the target) did not receive its addressed msg, got %#v", got)
	}
	if got := r2.overlays[1].(fakeOverlay).received; len(got) != 0 {
		t.Fatalf("top overlay received a msg addressed to another overlay: %#v", got)
	}
}

// A result addressed to an overlay that has since been popped is dropped, not
// misdelivered to a surviving overlay.
func TestHandleBroadcast_OverlayAddressedMsg_DropsWhenTargetGone(t *testing.T) {
	survivor := fakeOverlay{id: 2}
	r := Root{keys: testKeyMap(), tabs: []tuipkg.Tab{fakeTab{}}, overlays: []ovpkg.Overlay{survivor}}

	r2, cmd := r.handleBroadcast(fakeAddressedMsg{tuipkg.OverlayTarget{ID: 99}})

	if got := r2.overlays[0].(fakeOverlay).received; len(got) != 0 {
		t.Fatalf("surviving overlay received a msg addressed to a popped overlay: %#v", got)
	}
	if cmd != nil {
		t.Fatalf("expected nil cmd for a dropped addressed msg, got %#v", cmd)
	}
}

func testKeyMap() keymap.KeyMap {
	return keymap.Build(config.KeyBindings{
		Quit:          "q",
		Close:         "esc",
		TabChord:      "t",
		FocusSwitch:   "f",
		Help:          "?",
		CommandPrompt: ":",
	})
}

func newTestRoot(top ovpkg.Overlay, activeTab tuipkg.Tab) Root {
	return Root{
		keys:     testKeyMap(),
		tabs:     []tuipkg.Tab{activeTab},
		overlays: []ovpkg.Overlay{top},
	}
}

func newTestRootNoOverlay(activeTab tuipkg.Tab) Root {
	return Root{keys: testKeyMap(), tabs: []tuipkg.Tab{activeTab}}
}

func lastMsg(msgs []tea.Msg) tea.Msg {
	if len(msgs) == 0 {
		return nil
	}
	return msgs[len(msgs)-1]
}

func quitMsg(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// When the top overlay owns a focused text input (e.g. the add-to-playlist
// create-name field), every key must reach it verbatim — global chords must
// not steal characters the user is trying to type. Regression test for H-3.
func TestHandleKey_OverlayInterceptsInput_RoutesRawKeyToOverlay(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{"quit-letter", tea.KeyPressMsg{Text: "q"}},
		{"tabchord-letter", tea.KeyPressMsg{Text: "t"}},
		{"focusswitch-letter", tea.KeyPressMsg{Text: "f"}},
		{"tab-code", tea.KeyPressMsg{Code: tea.KeyTab}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			top := fakeOverlay{intercepts: true, hasFocus: true}
			r := newTestRoot(top, fakeTab{})

			r2, cmd := r.handleKey(tc.msg)

			if quitMsg(cmd) {
				t.Fatalf("quit while overlay intercepts input")
			}
			if r2.tabChordActive {
				t.Fatalf("tab chord armed while overlay intercepts input")
			}
			got := r2.overlays[0].(fakeOverlay).received
			if len(got) != 1 || got[0] != tea.Msg(tc.msg) {
				t.Fatalf("overlay did not receive raw key verbatim, got %#v", got)
			}
		})
	}
}

// When the overlay itself has no focus (e.g. an unfocused info side panel)
// but the tab beneath has a focused text input (e.g. Search's query box),
// keys must reach the tab, not global chords or the overlay.
func TestHandleKey_TabInterceptsInputUnderUnfocusedOverlay_RoutesToTab(t *testing.T) {
	top := fakeOverlay{intercepts: false, hasFocus: false}
	activeTab := fakeTab{intercepts: true}
	r := newTestRoot(top, activeTab)

	r2, cmd := r.handleKey(tea.KeyPressMsg{Text: "q"})

	if quitMsg(cmd) {
		t.Fatalf("quit while active tab intercepts input")
	}
	got := r2.tabs[0].(fakeTab).received
	if len(got) != 1 {
		t.Fatalf("active tab did not receive the key, got %#v", got)
	}
}

// With an overlay open that does NOT intercept input or hold focus (plain
// list-mode browsing), Quit must actually quit the app — previously it was
// unconditionally routed to the overlay, which only popped it (H-3 bug b).
func TestHandleKey_QuitClosesAppEvenWithOverlayOpen(t *testing.T) {
	top := fakeOverlay{intercepts: false, hasFocus: false}
	r := newTestRoot(top, fakeTab{})

	_, cmd := r.handleKey(tea.KeyPressMsg{Text: "q"})

	if !quitMsg(cmd) {
		t.Fatalf("expected Quit to quit the app with a non-intercepting overlay open")
	}
}

// Same as above but for an overlay that holds focus without intercepting
// input (e.g. a list-mode modal you're navigating with j/k) — Quit should
// still quit the whole app rather than merely popping the overlay.
func TestHandleKey_QuitClosesAppEvenWithFocusedOverlayOpen(t *testing.T) {
	top := fakeOverlay{intercepts: false, hasFocus: true}
	r := newTestRoot(top, fakeTab{})

	_, cmd := r.handleKey(tea.KeyPressMsg{Text: "q"})

	if !quitMsg(cmd) {
		t.Fatalf("expected Quit to quit the app with a focused, non-intercepting overlay open")
	}
}

// Escape must remain the universal overlay-close key regardless of focus.
func TestHandleKey_EscapeAlwaysRoutesToOverlay(t *testing.T) {
	top := fakeOverlay{intercepts: false, hasFocus: false}
	r := newTestRoot(top, fakeTab{})
	msg := tea.KeyPressMsg{Code: tea.KeyEscape}

	r2, _ := r.handleKey(msg)

	got := r2.overlays[0].(fakeOverlay).received
	if len(got) != 1 || got[0] != tea.Msg(msg) {
		t.Fatalf("overlay did not receive Escape, got %#v", got)
	}
}

// ── M-6 characterization: remaining global-chord precedence branches ──────────

// Help opens over a non-help overlay (and, being global, takes precedence over
// forwarding the key to the overlay/tab).
func TestHandleKey_HelpOpensOverOverlay(t *testing.T) {
	r := newTestRoot(fakeOverlay{}, fakeTab{})
	r2, _ := r.handleKey(tea.KeyPressMsg{Code: '?', Text: "?"})
	if len(r2.overlays) != 2 {
		t.Fatalf("overlays = %d, want 2 (help opened)", len(r2.overlays))
	}
	if _, ok := r2.overlays[len(r2.overlays)-1].(ovpkg.Help); !ok {
		t.Errorf("top overlay is %T, want ovpkg.Help", r2.overlays[len(r2.overlays)-1])
	}
}

// The command palette opens over a non-text overlay.
func TestHandleKey_CommandPromptOpensOverOverlay(t *testing.T) {
	r := newTestRoot(fakeOverlay{}, fakeTab{})
	r2, _ := r.handleKey(tea.KeyPressMsg{Code: ':', Text: ":"})
	if len(r2.overlays) != 2 {
		t.Fatalf("overlays = %d, want 2 (palette opened)", len(r2.overlays))
	}
	if _, ok := r2.overlays[len(r2.overlays)-1].(ovpkg.CommandBar); !ok {
		t.Errorf("top overlay is %T, want ovpkg.CommandBar", r2.overlays[len(r2.overlays)-1])
	}
}

// FocusSwitch is delivered to the top overlay as a FocusSwitchMsg.
func TestHandleKey_FocusSwitchRoutesToTopOverlay(t *testing.T) {
	r := newTestRoot(fakeOverlay{hasFocus: true}, fakeTab{})
	r2, _ := r.handleKey(tea.KeyPressMsg{Code: 'f', Text: "f"})
	got := lastMsg(r2.overlays[0].(fakeOverlay).received)
	if _, ok := got.(ovpkg.FocusSwitchMsg); !ok {
		t.Errorf("top overlay received %#v, want FocusSwitchMsg", got)
	}
}

// TabChord arms the chord (does not act immediately) with an overlay open.
func TestHandleKey_TabChordArmsWithOverlay(t *testing.T) {
	r := newTestRoot(fakeOverlay{}, fakeTab{})
	r2, cmd := r.handleKey(tea.KeyPressMsg{Code: 't', Text: "t"})
	if !r2.tabChordActive {
		t.Error("tab chord not armed")
	}
	if cmd != nil {
		t.Errorf("arming the chord should issue no cmd, got %#v", cmd())
	}
}

// A focused overlay that neither intercepts input nor matches a global chord
// receives the key.
func TestHandleKey_FocusedOverlayReceivesUnboundKey(t *testing.T) {
	r := newTestRoot(fakeOverlay{hasFocus: true}, fakeTab{})
	msg := tea.KeyPressMsg{Code: 'z', Text: "z"}
	r2, _ := r.handleKey(msg)
	if got := lastMsg(r2.overlays[0].(fakeOverlay).received); got != tea.Msg(msg) {
		t.Errorf("focused overlay received %#v, want the raw key", got)
	}
}

// An unfocused, non-intercepting overlay forwards unbound keys to the tab.
func TestHandleKey_UnfocusedOverlayForwardsUnboundKeyToTab(t *testing.T) {
	r := newTestRoot(fakeOverlay{}, fakeTab{})
	msg := tea.KeyPressMsg{Code: 'z', Text: "z"}
	r2, _ := r.handleKey(msg)
	if got := lastMsg(r2.tabs[0].(fakeTab).received); got != tea.Msg(msg) {
		t.Errorf("tab received %#v, want the forwarded key", got)
	}
}

// ── no-overlay context ────────────────────────────────────────────────────────

func TestHandleKey_NoOverlay_QuitQuits(t *testing.T) {
	r := newTestRootNoOverlay(fakeTab{})
	if _, cmd := r.handleKey(tea.KeyPressMsg{Code: 'q', Text: "q"}); !quitMsg(cmd) {
		t.Error("Quit should quit with no overlay open")
	}
}

func TestHandleKey_NoOverlay_HelpOpens(t *testing.T) {
	r := newTestRootNoOverlay(fakeTab{})
	r2, _ := r.handleKey(tea.KeyPressMsg{Code: '?', Text: "?"})
	if len(r2.overlays) != 1 {
		t.Fatalf("overlays = %d, want 1 (help opened)", len(r2.overlays))
	}
	if _, ok := r2.overlays[0].(ovpkg.Help); !ok {
		t.Errorf("overlay is %T, want ovpkg.Help", r2.overlays[0])
	}
}

func TestHandleKey_NoOverlay_TabChordArms(t *testing.T) {
	r := newTestRootNoOverlay(fakeTab{})
	r2, _ := r.handleKey(tea.KeyPressMsg{Code: 't', Text: "t"})
	if !r2.tabChordActive {
		t.Error("tab chord not armed with no overlay")
	}
}

func TestHandleKey_NoOverlay_UnboundKeyGoesToTab(t *testing.T) {
	r := newTestRootNoOverlay(fakeTab{})
	msg := tea.KeyPressMsg{Code: 'z', Text: "z"}
	r2, _ := r.handleKey(msg)
	if got := lastMsg(r2.tabs[0].(fakeTab).received); got != tea.Msg(msg) {
		t.Errorf("tab received %#v, want the unbound key", got)
	}
}

// An armed chord consumes the next key: an unmapped second key disarms without
// navigating.
func TestHandleKey_ArmedChordDisarmsOnUnmappedKey(t *testing.T) {
	r := newTestRootNoOverlay(fakeTab{})
	r.tabChordActive = true
	r.tabChordKeys = map[string]int{"1": 0}
	r2, cmd := r.handleKey(tea.KeyPressMsg{Code: 'z', Text: "z"})
	if r2.tabChordActive {
		t.Error("armed chord should disarm after consuming a key")
	}
	if cmd != nil {
		t.Errorf("unmapped chord key should do nothing, got %#v", cmd())
	}
}
