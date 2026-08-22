package app

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/playback"
)

func panelTestBackend() apitest.NopBackend { return apitest.NopBackend{} }

// TestBuildPanelsDefaultLayout: the default panel list reproduces the historical
// tab order, one tab per panel, with the expected tab IDs.
func TestBuildPanelsDefaultLayout(t *testing.T) {
	keys := keymap.Build(config.KeyBindings{})
	cfg := &config.Config{}
	tabs := buildPanels(context.Background(), config.DefaultPanels, panelTestBackend(), keys, cfg)
	want := []tuipkg.TabID{
		tuipkg.TabFeed, tuipkg.TabChannels, tuipkg.TabTags, tuipkg.TabPlaylists,
		tuipkg.TabSearch, tuipkg.TabDownloading, tuipkg.TabLocal, tuipkg.TabHistory, tuipkg.TabActivity,
	}
	if len(tabs) != len(want) {
		t.Fatalf("buildPanels(DefaultPanels) = %d tabs, want %d", len(tabs), len(want))
	}
	for i, id := range want {
		if tabs[i].ID() != id {
			t.Errorf("tab[%d].ID() = %v, want %v", i, tabs[i].ID(), id)
		}
	}
}

func TestBuildPanelUnknownType(t *testing.T) {
	keys := keymap.Build(config.KeyBindings{})
	if _, ok := buildPanel(context.Background(), config.Panel{Name: "x", Type: "nope"}, panelTestBackend(), keys, &config.Config{}); ok {
		t.Error("buildPanel accepted an unknown type")
	}
}

// TestBuildPanelThreadsMode: a panel Mode is threaded into the constructed tab.
// The mode no longer shows in the title, so we observe it via the Feed tab's
// mode-gated ShortHelp — mixed shows both HideVideo (rec) and Unsubscribe (sub);
// recommended shows only HideVideo, so their binding counts differ.
func TestBuildPanelThreadsMode(t *testing.T) {
	keys := keymap.Build(config.KeyBindings{})
	cfg := &config.Config{}
	mixed, _ := buildPanel(context.Background(), config.Panel{Name: "f", Type: "feed", Mode: "mixed"}, panelTestBackend(), keys, cfg)
	rec, _ := buildPanel(context.Background(), config.Panel{Name: "f", Type: "feed", Mode: "recommended"}, panelTestBackend(), keys, cfg)
	if len(mixed.ShortHelp()) == len(rec.ShortHelp()) {
		t.Errorf("panel Mode not threaded: mixed and recommended have equal ShortHelp (%d)", len(rec.ShortHelp()))
	}
}

func TestModeOr(t *testing.T) {
	if got := modeOr("mixed", "recommended"); got != "mixed" {
		t.Errorf("modeOr(mixed, recommended) = %q, want mixed", got)
	}
	if got := modeOr("", "recommended"); got != "recommended" {
		t.Errorf("modeOr(\"\", recommended) = %q, want recommended", got)
	}
}

func TestPanelIndexByNameFirstWins(t *testing.T) {
	panels := []config.Panel{
		{Name: "a", Type: "feed"},
		{Name: "b", Type: "local"},
		{Name: "a", Type: "history"}, // duplicate name
	}
	idx := panelIndexByName(panels)
	if idx["a"] != 0 {
		t.Errorf("duplicate name did not keep first index: got %d", idx["a"])
	}
	if idx["b"] != 1 {
		t.Errorf("idx[b] = %d, want 1", idx["b"])
	}
}

// TestBuildTabChordKeysPositionalAndNamed: positional 1..9 are always present,
// configured named hotkeys map to the right index, and a named binding wins
// over a positional digit on collision.
func TestBuildTabChordKeysPositionalAndNamed(t *testing.T) {
	panels := []config.Panel{
		{Name: "feed", Type: "feed"},
		{Name: "local", Type: "local"},
		{Name: "history", Type: "history"},
	}
	tabKeys := map[string]string{
		"l": "local",   // named → index 1
		"2": "history", // digit hotkey overrides the positional "2" (index 1) → index 2
	}
	m := buildTabChordKeys(tabKeys, panels)
	if m["1"] != 0 || m["3"] != 2 {
		t.Errorf("positional fallback wrong: %v", m)
	}
	if m["l"] != 1 {
		t.Errorf("named hotkey l -> local: got %d, want 1", m["l"])
	}
	if m["2"] != 2 {
		t.Errorf("named binding did not win over positional digit: m[2] = %d, want 2", m["2"])
	}
}

func TestBuildTabChordKeysCapsAtNine(t *testing.T) {
	panels := make([]config.Panel, 12)
	for i := range panels {
		panels[i] = config.Panel{Name: string(rune('a' + i)), Type: "feed"}
	}
	m := buildTabChordKeys(nil, panels)
	if _, ok := m["9"]; !ok {
		t.Error("positional 9 missing")
	}
	if _, ok := m["10"]; ok {
		t.Error("positional went past 9")
	}
}

func TestEffectivePanelsDropsUnknownAndFallsBack(t *testing.T) {
	got := effectivePanels([]config.Panel{{Name: "f", Type: "feed"}, {Name: "x", Type: "bad"}})
	if len(got) != 1 || got[0].Type != "feed" {
		t.Errorf("effectivePanels did not drop unknown: %v", got)
	}
	empty := effectivePanels([]config.Panel{{Name: "x", Type: "bad"}})
	if len(empty) != len(config.DefaultPanels) {
		t.Errorf("all-invalid did not fall back to DefaultPanels: got %d", len(empty))
	}
}

// newPanelRoot builds a Root through the real constructor with the given panels
// and tab-key map, so navigation tests exercise the wired chord/name maps.
func newPanelRoot(panels []config.Panel, tabKeys map[string]string) Root {
	cfg := &config.Config{}
	cfg.Panels = panels
	cfg.Keybindings = config.KeyBindings{TabChord: "t", Quit: "q", Close: "esc", TabKeys: tabKeys}
	return New(context.Background(), panelTestBackend(), panelTestBackend(), cfg, nil, nil, playback.YtdlpInfo{})
}

func TestTabCommandNavigatesByName(t *testing.T) {
	r := newPanelRoot(config.DefaultPanels, nil) // name nav uses panelIdx, not tabKeys
	model, _ := r.Update(tuipkg.NavigateToPanelMsg{Name: "history"})
	got := model.(Root)
	if got.activeTab().ID() != tuipkg.TabHistory {
		t.Errorf(":tab history activated %v, want TabHistory", got.activeTab().ID())
	}
}

func TestTabCommandUnknownNameReportsError(t *testing.T) {
	r := newPanelRoot(config.DefaultPanels, nil)
	_, cmd := r.handleNavigateToPanel(tuipkg.NavigateToPanelMsg{Name: "ghost"})
	msg, ok := runCmd(cmd).(tuipkg.StatusMsg)
	if !ok || !msg.IsErr {
		t.Errorf(":tab ghost did not report an error status: %#v", runCmd(cmd))
	}
}

func TestChordNavigatesByNamedHotkey(t *testing.T) {
	r := newPanelRoot(config.DefaultPanels, map[string]string{"l": "local"})
	r, _ = r.handleKey(tea.KeyPressMsg{Text: "t"}) // arm TabChord
	if !r.router.chordActive {
		t.Fatal("TabChord did not arm")
	}
	r, _ = r.handleKey(tea.KeyPressMsg{Text: "l"}) // complete → local
	if r.activeTab().ID() != tuipkg.TabLocal {
		t.Errorf("chord t-l activated %v, want TabLocal", r.activeTab().ID())
	}
}

func TestChordNavigatesByPositional(t *testing.T) {
	r := newPanelRoot(config.DefaultPanels, nil)
	r, _ = r.handleKey(tea.KeyPressMsg{Text: "t"}) // arm
	r, _ = r.handleKey(tea.KeyPressMsg{Text: "3"}) // 3rd panel → tags
	if r.activeTab().ID() != tuipkg.TabTags {
		t.Errorf("chord t-3 activated %v, want TabTags (3rd panel)", r.activeTab().ID())
	}
}

// TestDataDrivenTabBarRenders verifies the tab bar is built from the panel list
// and every frame line stays within the terminal width (ClampLine invariant).
func TestDataDrivenTabBarRenders(t *testing.T) {
	r := newPanelRoot(config.DefaultPanels, nil)
	const w, h = 120, 40
	model, _ := r.Update(tea.WindowSizeMsg{Width: w, Height: h})
	r = model.(Root)
	frame := r.View().Content
	for i, line := range strings.Split(frame, "\n") {
		if got := lipgloss.Width(line); got > w {
			t.Fatalf("frame line %d width %d exceeds terminal width %d", i, got, w)
		}
	}
	bar := strings.Split(frame, "\n")[0]
	for _, want := range []string{"Feed", "Channels", "Local", "History"} {
		if !strings.Contains(bar, want) {
			t.Errorf("tab bar %q missing panel label %q", bar, want)
		}
	}
}
