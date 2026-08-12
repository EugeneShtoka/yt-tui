package overlay

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

// Help is a scrollable, read-only overlay listing every configured key binding,
// grouped by area. It is opened by the Help key and closed with Escape, Quit, or
// Help again.
type Help struct {
	identity
	keys keymap.KeyMap
	vs   int // first visible row (scroll offset)
}

// NewHelp builds the help overlay from the active key map.
func NewHelp(keys keymap.KeyMap) Help { return Help{identity: newIdentity(), keys: keys} }

// ── overlay.Overlay interface ─────────────────────────────────────────────────

func (h Help) InterceptsInput() bool { return false }
func (h Help) WidthReduction() int   { return 0 }
func (h Help) HasFocus() bool        { return true }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (h Help) Init() tea.Cmd  { return nil }
func (h Help) View() tea.View { return tea.NewView("") } // rendering done via Render(behind,...)

func (h Help) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return h, nil
	}
	switch {
	case key.Matches(km, h.keys.Escape), key.Matches(km, h.keys.Quit), key.Matches(km, h.keys.Help):
		return h, func() tea.Msg { return PopOverlayMsg{} }
	case key.Matches(km, h.keys.Down):
		h.vs++
	case key.Matches(km, h.keys.Up):
		if h.vs > 0 {
			h.vs--
		}
	case key.Matches(km, h.keys.PageDown):
		h.vs += 10
	case key.Matches(km, h.keys.PageUp):
		h.vs -= 10
		if h.vs < 0 {
			h.vs = 0
		}
	}
	return h, nil
}

// helpSection is a titled group of key bindings shown in the overlay.
type helpSection struct {
	title string
	binds []key.Binding
}

func (h Help) sections() []helpSection {
	k := h.keys
	return []helpSection{
		{"General", []key.Binding{k.Help, k.Quit, k.Escape, k.Filter, k.Refresh, k.ForceRefresh}},
		{"Navigation", []key.Binding{k.Up, k.Down, k.Left, k.Right, k.PageUp, k.PageDown, k.GotoPrefix, k.GotoBottom, k.GotoLine, k.DrillDown}},
		{"Tabs", []key.Binding{k.Tab, k.ShiftTab, k.TabChord, k.FocusSwitch}},
		{"Video actions", []key.Binding{k.Play, k.PlayAudio, k.Download, k.DownloadAudio, k.CopyURL, k.VideoInfo, k.OpenLinks, k.OpenChapters, k.OpenTranscript, k.CopyTranscript, k.AddList, k.WatchLater, k.Delete, k.HideVideo}},
		{"Channels & tags", []key.Binding{k.Subscribe, k.Unsubscribe, k.Block, k.HideChannel, k.RenameChannel, k.TagChannel}},
		{"View & data", []key.Binding{k.PanelMode, k.ToggleMode, k.NewList, k.Export, k.Import, k.CommandPrompt}},
		{"Sort  (press sort key, then…)", []key.Binding{k.Sort.Date, k.Sort.Views, k.Sort.Name, k.Sort.Channel, k.Sort.Duration, k.Sort.Subscribers, k.Sort.Tags, k.Sort.Size}},
	}
}

// contentLines flattens the sections into rendered "key  description" rows,
// skipping bindings that have no key configured.
func (h Help) contentLines(innerW int) []string {
	const keyCol = 12
	var lines []string
	for si, sec := range h.sections() {
		if si > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, styles.Bold.Render(sec.title))
		for _, b := range sec.binds {
			hk := b.Help()
			if hk.Key == "" {
				continue
			}
			keyStr := hk.Key
			if lipgloss.Width(keyStr) > keyCol-1 {
				keyStr = render.Truncate(keyStr, keyCol-1)
			}
			pad := keyCol - lipgloss.Width(keyStr)
			row := styles.Help.Render(keyStr) + strings.Repeat(" ", pad) + render.ClampLine(hk.Desc, innerW-keyCol)
			lines = append(lines, row)
		}
	}
	return lines
}

func (h Help) Render(behind string, width, height int) string {
	boxW, innerW := render.ModalBox(width, 6, 72)

	lines := h.contentLines(innerW)

	maxRows := height - 8 // borders, padding, title, footer
	if maxRows < 3 {
		maxRows = 3
	}
	needsScroll := len(lines) > maxRows
	maxVS := len(lines) - maxRows
	if maxVS < 0 {
		maxVS = 0
	}
	vs := h.vs
	if vs > maxVS {
		vs = maxVS
	}

	out := []string{styles.Bold.Render("Keyboard shortcuts"), ""}
	visible := lines[vs:]
	if len(visible) > maxRows {
		visible = visible[:maxRows]
	}
	out = append(out, visible...)

	closeHint := h.keys.Escape.Help().Key + ": close"
	var left string
	if needsScroll {
		left = h.keys.Down.Help().Key + "/" + h.keys.Up.Help().Key + ": scroll"
	}
	out = append(out, "", styles.Help.Render(render.JustifyEnds(left, closeHint, innerW)))

	return placeOverlayBox(behind, strings.Join(out, "\n"), width, boxW)
}
