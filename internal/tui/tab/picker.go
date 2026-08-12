package tab

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
)

// pickerOutcome is the result of feeding one key to an open modePicker.
type pickerOutcome int

const (
	pickerOngoing   pickerOutcome = iota // navigated or unrelated key; picker stays open
	pickerCommitted                      // Enter: caller should apply selection()
	pickerCanceled                       // Esc: caller should discard
)

// modePicker is a small inline selector shared by panels that switch a source
// filter/mode via the PanelMode key (the Channels view, the Feed mode). It owns
// the open flag, the highlighted index, and the up/down/enter/esc handling; the
// host panel supplies the title + option labels and maps the committed index to
// its own enum. Extracted from the Phase-4 Channels picker so new panels reuse
// one widget instead of re-implementing the pattern (DRY).
//
// It is held by value on the tab and mutated through pointer methods on the
// addressable field, mirroring how Feed/sortState persist across Bubble Tea's
// value-copy of the model.
type modePicker struct {
	title    string
	options  []string
	circular bool
	open     bool
	sel      int
}

// newModePicker builds a closed picker titled title over the given option labels.
func newModePicker(title string, options []string, circular bool) modePicker {
	return modePicker{title: title, options: options, circular: circular}
}

func (p modePicker) isOpen() bool   { return p.open }
func (p modePicker) selection() int { return p.sel }

// openAt shows the picker with sel highlighted (typically the active mode).
func (p *modePicker) openAt(sel int) {
	p.open = true
	p.sel = sel
}

// handleKey drives an open picker: Up/Down move (wrapping when circular), Enter
// commits, Esc cancels. It closes itself on commit/cancel and reports whether the
// caller should apply, discard, or keep waiting on the selection.
func (p *modePicker) handleKey(msg tea.KeyPressMsg, keys keymap.KeyMap) pickerOutcome {
	switch {
	case key.Matches(msg, keys.Up):
		if p.sel > 0 {
			p.sel--
		} else if p.circular {
			p.sel = len(p.options) - 1
		}
	case key.Matches(msg, keys.Down):
		if p.sel < len(p.options)-1 {
			p.sel++
		} else if p.circular {
			p.sel = 0
		}
	case key.Matches(msg, keys.DrillDown):
		p.open = false
		return pickerCommitted
	case key.Matches(msg, keys.Escape):
		p.open = false
		return pickerCanceled
	}
	return pickerOngoing
}

// view renders the picker as a centered popup box composited over behind,
// highlighting the current option and showing the move/select/cancel hint
// (escKey is the panel's close binding). width is the content width used to
// center the box. Drawing on top of behind — rather than appending beneath it —
// keeps the picker visible even when a full-height table fills the panel.
func (p modePicker) view(behind, escKey string, width int) string {
	lines := []string{styles.Bold.Render(p.title + ":"), ""}
	for i, label := range p.options {
		if i == p.sel {
			lines = append(lines, styles.Selected.Render("▶ "+label))
		} else {
			lines = append(lines, "  "+label)
		}
	}
	lines = append(lines, "", styles.Help.Render("j/k: move  enter: select  "+escKey+": cancel"))
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.ColorAccent).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	return render.OverlayCenter(behind, box, width)
}
