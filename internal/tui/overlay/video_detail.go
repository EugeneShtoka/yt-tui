package overlay

import (
	"context"
	"fmt"
	"image"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/media"
	"github.com/EugeneShtoka/yt-tui/internal/sys"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/atotto/clipboard"
)

const panelW = 52

// OverlaySizeMsg is sent by Root to overlays during resize so they can
// compute terminal-absolute positions (e.g. for Kitty image placement).
type OverlaySizeMsg struct {
	ContentW int // terminal columns available left of the overlay panel
	ContentH int // terminal rows available for content
	KittyRow int // 1-indexed terminal row where the panel interior begins
}

// vdSubState is the sub-state of the VideoDetail overlay.
type vdSubState int

const (
	vdPanel    vdSubState = iota // detail side panel
	vdLinks                      // links modal over panel
	vdChapters                   // chapters modal over panel
)

// ── private messages ──────────────────────────────────────────────────────────

type vdDetailsMsg struct {
	details domain.VideoDetails
	err     error
}

// ── VideoDetail ───────────────────────────────────────────────────────────────

// VideoDetail is the video-detail side panel with nested links/chapters modals.
type VideoDetail struct {
	backend      api.Backend
	keys         keymap.KeyMap
	closeOnLinks bool // cfg.CloseOnLinkOpen

	video   *domain.VideoDetails
	loading bool
	spinner spinner.Model

	descLines []string
	descVS    int
	links     *[]domain.Link
	chapters  *[]domain.Chapter

	thumb         image.Image
	thumbB64      string
	thumbRendered string

	contentW int // terminal columns left of the panel (for Kitty col position)
	kittyRow int // 1-indexed terminal row where panel interior starts

	subState   vdSubState
	linkSel    int
	chapterSel int
	circular   bool
}

// NewVideoDetail creates a VideoDetail overlay that immediately starts loading
// details for the given video.
func NewVideoDetail(backend api.Backend, keys keymap.KeyMap, v domain.Video, closeOnLinks, circular bool) (VideoDetail, tea.Cmd) {
	sp := spinner.New()
	vd := VideoDetail{
		backend:      backend,
		keys:         keys,
		closeOnLinks: closeOnLinks,
		loading:      true,
		spinner:      sp,
		circular:     circular,
	}
	return vd, tea.Batch(vd.fetchCmd(v.URL), sp.Tick)
}

// ── overlay.Overlay interface ─────────────────────────────────────────────────

func (vd VideoDetail) InterceptsInput() bool { return false }
func (vd VideoDetail) WidthReduction() int   { return panelW }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (vd VideoDetail) Init() tea.Cmd  { return nil }
func (vd VideoDetail) View() tea.View { return tea.NewView("") } // rendering done via Render(behind,...)

func (vd VideoDetail) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case spinner.TickMsg:
		if vd.loading {
			var cmd tea.Cmd
			vd.spinner, cmd = vd.spinner.Update(m)
			return vd, cmd
		}

	case vdDetailsMsg:
		vd.loading = false
		if m.err != nil {
			return vd, tea.Batch(
				func() tea.Msg { return PopOverlayMsg{} },
				func() tea.Msg { return tuipkg.StatusMsg{Text: "video details: " + m.err.Error(), IsErr: true} },
			)
		}
		details := m.details
		vd.video = &details
		vd.descLines = wordWrap(details.Description, panelW-2)
		if len(details.Chapters) > 0 {
			chapters, _ := media.ProcessChapters(details.Chapters)
			vd.chapters = &chapters
		}
		// Save to cache.
		ctx := context.Background()
		_ = vd.backend.SaveVideoDetailsCache(ctx, details.ID, details.Description, details.ThumbnailURL, details.Subscribers)
		if vd.chapters != nil {
			_ = vd.backend.SaveVideoChapters(ctx, details.ID, *vd.chapters)
		}
		// Start thumbnail fetch.
		if details.ThumbnailURL != "" {
			return vd, LoadThumbnailCmd(details.ThumbnailURL)
		}

	case OverlaySizeMsg:
		vd.contentW = m.ContentW
		vd.kittyRow = m.KittyRow
		return vd, vd.kittyCmd()

	case ThumbnailLoadedMsg:
		vd.thumb = m.Img
		if m.Img != nil {
			if kittyCapable() {
				vd.thumbB64 = encodeThumbB64(m.Img)
			} else {
				_, thumbH := vd.thumbDimensions()
				vd.thumbRendered = renderThumbnailHalfBlock(m.Img, panelW-2, thumbH)
			}
		}
		return vd, vd.kittyCmd()

	case tea.KeyPressMsg:
		return vd.handleKey(m)
	}
	return vd, nil
}

// kittyCmd returns tea.Raw(kittyImageSeq) when a Kitty image should be placed,
// or nil when conditions are not met.
func (vd VideoDetail) kittyCmd() tea.Cmd {
	if !kittyCapable() || vd.thumbB64 == "" || vd.contentW == 0 || vd.subState != vdPanel {
		return nil
	}
	thumbW, thumbH := vd.thumbDimensions()
	seq := kittyImageSeq(vd.thumbB64, vd.kittyRow, vd.contentW+2, thumbW, thumbH)
	return tea.Raw(seq)
}

// Render composes the side panel to the right of behind.
// Returns (composedView, kittySeq); kittySeq is non-empty only on Kitty terminals.
func (vd VideoDetail) Render(behind string, width, height int) (string, string) {
	_, thumbH := vd.thumbDimensions()
	panel := vd.renderPanel(panelW, height, thumbH)

	// Clamp each line of 'behind' to strictly fill the remaining width
	leftW := width - panelW
	if leftW < 0 {
		leftW = 0
	}

	behindLines := strings.Split(behind, "\n")
	for i, line := range behindLines {
		behindLines[i] = lipgloss.NewStyle().MaxWidth(leftW).Width(leftW).Render(line)
	}
	croppedBehind := strings.Join(behindLines, "\n")

	composed := lipgloss.JoinHorizontal(lipgloss.Top, croppedBehind, panel)
	// Modals stack on top of the composed view.
	switch vd.subState {
	case vdLinks:
		composed = vd.renderLinksModal(composed, width)
	case vdChapters:
		composed = vd.renderChaptersModal(composed, width)
	}

	// Kitty image placement is handled via tea.Raw commands in Update/kittyCmd,
	// not via embedded escape sequences in the rendered string (bubbletea v2's
	// cell renderer drops APC sequences from Content).
	return composed, ""
}

// ── key handling ──────────────────────────────────────────────────────────────

func (vd VideoDetail) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch vd.subState {
	case vdLinks:
		return vd.handleLinksKey(msg)
	case vdChapters:
		return vd.handleChaptersKey(msg)
	}
	return vd.handlePanelKey(msg)
}

func (vd VideoDetail) handlePanelKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := vd.keys
	switch {
	case key.Matches(msg, keys.Escape), key.Matches(msg, keys.Left), key.Matches(msg, keys.Quit):
		closeCmds := []tea.Cmd{func() tea.Msg { return PopOverlayMsg{} }}
		if kittyCapable() && vd.thumbB64 != "" {
			closeCmds = append(closeCmds, tea.Raw(kittyDeleteSeq()))
		}
		return vd, tea.Batch(closeCmds...)

	case key.Matches(msg, keys.Down):
		vd.descVS++
	case key.Matches(msg, keys.Up):
		if vd.descVS > 0 {
			vd.descVS--
		}
	case key.Matches(msg, keys.PageDown):
		vd.descVS += 10
	case key.Matches(msg, keys.PageUp):
		vd.descVS -= 10
		if vd.descVS < 0 {
			vd.descVS = 0
		}
	case key.Matches(msg, keys.GotoBottom):
		vd.descVS = len(vd.descLines)

	case key.Matches(msg, keys.OpenLinks):
		if vd.video != nil {
			if vd.links == nil {
				urls := media.ExtractLinks(vd.video.Description)
				vd.links = &urls
				_ = vd.backend.SaveVideoLinks(context.Background(), vd.video.ID, urls)
			}
			if len(*vd.links) == 0 {
				return vd, func() tea.Msg { return tuipkg.StatusMsg{Text: "no links in description"} }
			}
			vd.subState = vdLinks
			vd.linkSel = 0
		}

	case key.Matches(msg, keys.OpenChapters):
		if vd.chapters != nil && len(*vd.chapters) > 0 {
			vd.subState = vdChapters
			vd.chapterSel = 0
		} else {
			return vd, func() tea.Msg { return tuipkg.StatusMsg{Text: "no chapters available"} }
		}
	}
	return vd, nil
}

func (vd VideoDetail) handleLinksKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := vd.keys
	links := *vd.links
	n := len(links)

	if newSel, consumed := vd.moveSelector(vd.linkSel, n, msg); consumed {
		vd.linkSel = newSel
		return vd, nil
	}
	switch {
	case key.Matches(msg, keys.Escape), key.Matches(msg, keys.Quit):
		vd.subState = vdPanel
	case key.Matches(msg, keys.DrillDown):
		if n > 0 {
			if err := sys.OpenURL(links[vd.linkSel].URL); err != nil {
				return vd, func() tea.Msg { return tuipkg.StatusMsg{Text: "open: " + err.Error(), IsErr: true} }
			}
			if vd.closeOnLinks {
				vd.subState = vdPanel
			}
		}
	case key.Matches(msg, keys.CopyURL):
		if n > 0 {
			u := links[vd.linkSel].URL
			if err := clipboard.WriteAll(u); err != nil {
				return vd, func() tea.Msg { return tuipkg.StatusMsg{Text: "clipboard: " + err.Error(), IsErr: true} }
			}
			return vd, func() tea.Msg { return tuipkg.StatusMsg{Text: "copied: " + u} }
		}
	}
	return vd, nil
}

func (vd VideoDetail) handleChaptersKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := vd.keys
	chapters := *vd.chapters
	n := len(chapters)

	if newSel, consumed := vd.moveSelector(vd.chapterSel, n, msg); consumed {
		vd.chapterSel = newSel
		return vd, nil
	}
	switch {
	case key.Matches(msg, keys.Escape), key.Matches(msg, keys.Quit):
		vd.subState = vdPanel
	case key.Matches(msg, keys.Play):
		if n > 0 && vd.video != nil {
			ch := chapters[vd.chapterSel]
			v := vd.video.Video
			v.URL = fmt.Sprintf("%s&t=%d", v.URL, int(ch.OriginalStart))
			return vd, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v} }
		}
	case key.Matches(msg, keys.PlayAudio):
		if n > 0 && vd.video != nil {
			ch := chapters[vd.chapterSel]
			v := vd.video.Video
			v.URL = fmt.Sprintf("%s&t=%d", v.URL, int(ch.OriginalStart))
			return vd, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v, AudioOnly: true} }
		}
	case key.Matches(msg, keys.CopyURL):
		if n > 0 && vd.video != nil {
			ch := chapters[vd.chapterSel]
			u := fmt.Sprintf("https://www.youtube.com/watch?v=%s&t=%d", vd.video.ID, int(ch.OriginalStart))
			if err := clipboard.WriteAll(u); err != nil {
				return vd, func() tea.Msg { return tuipkg.StatusMsg{Text: "clipboard: " + err.Error(), IsErr: true} }
			}
			return vd, func() tea.Msg { return tuipkg.StatusMsg{Text: "copied: " + u} }
		}
	}
	return vd, nil
}

// moveSelector handles Up/Down/GotoBottom for overlay lists.
func (vd VideoDetail) moveSelector(sel, n int, msg tea.KeyPressMsg) (newSel int, consumed bool) {
	keys := vd.keys
	switch {
	case key.Matches(msg, keys.Up):
		if sel > 0 {
			return sel - 1, true
		}
		if vd.circular && n > 0 {
			return n - 1, true
		}
		return sel, true
	case key.Matches(msg, keys.Down):
		if sel < n-1 {
			return sel + 1, true
		}
		if vd.circular {
			return 0, true
		}
		return sel, true
	case key.Matches(msg, keys.GotoBottom):
		if n > 0 {
			return n - 1, true
		}
		return sel, true

	}
	return sel, false
}

// ── rendering ─────────────────────────────────────────────────────────────────

func (vd VideoDetail) renderPanel(panelW, panelH, thumbH int) string {
	innerW := panelW - 2
	accent := lipgloss.NewStyle().Foreground(styles.ColorAccent)
	norm := func(s string) string { return styles.Normal.Width(innerW).Render(s) }

	innerH := panelH - 2
	const footerH = 2
	contentRows := innerH - footerH

	var lines []string
	needsScroll := false

	if vd.loading {
		lines = append(lines, norm(vd.spinner.View()+" Loading…"))
	} else if vd.video != nil {
		v := vd.video

		switch {
		case kittyCapable():
			for i := 0; i < thumbH; i++ {
				lines = append(lines, norm(""))
			}
		case vd.thumbRendered != "":
			lines = append(lines, strings.Split(vd.thumbRendered, "\n")...)
		default:
			placeholder := strings.Repeat("░", innerW)
			for i := 0; i < thumbH; i++ {
				lines = append(lines, norm(placeholder))
			}
		}

		lines = append(lines, norm(""))
		for i, tl := range wordWrap(v.Title, innerW) {
			if i >= 3 {
				break
			}
			lines = append(lines, norm(tl))
		}

		lbl := styles.Dim
		meta := func(k, val string) string {
			return styles.Normal.Width(innerW).Render(lbl.Render(k) + val)
		}
		lines = append(lines, norm(""), meta("Channel  ", render.Truncate(v.Channel, innerW-9)))
		if v.Subscribers > 0 {
			lines = append(lines, meta("Subs     ", render.Views(v.Subscribers)))
		}
		lines = append(lines,
			meta("Views    ", v.ViewsStr()),
			meta("Duration ", v.DurationStr()),
			meta("Date     ", v.DateStr()),
			styles.Help.Width(innerW).Render(""),
			styles.Help.Width(innerW).Render(render.Truncate(v.URL, innerW)),
		)

		if v.Description != "" {
			lines = append(lines, styles.ColHeader.Width(innerW).Render(""), styles.ColHeader.Width(innerW).Render("Description"))
			available := contentRows - len(lines)
			if available > 0 {
				descLines := vd.descLines
				needsScroll = len(descLines) > available
				maxVS := len(descLines) - 1
				if maxVS < 0 {
					maxVS = 0
				}
				vs := vd.descVS
				if vs > maxVS {
					vs = maxVS
				}
				visible := descLines[vs:]
				if len(visible) > available {
					visible = visible[:available]
				}
				for _, dl := range visible {
					lines = append(lines, norm(dl))
				}
			}
		}
	}

	for len(lines) < contentRows {
		lines = append(lines, norm(""))
	}
	lines = lines[:contentRows]

	closeKey := vd.keys.Escape.Help().Key
	closeHint := closeKey + ": close"
	var footerText string
	if needsScroll {
		scrollHint := vd.keys.Down.Help().Key + "/" + vd.keys.Up.Help().Key + ": scroll"
		space := innerW - lipgloss.Width(scrollHint) - lipgloss.Width(closeHint)
		if space < 1 {
			space = 1
		}
		footerText = scrollHint + strings.Repeat(" ", space) + closeHint
	} else {
		space := innerW - lipgloss.Width(closeHint)
		if space < 1 {
			space = 1
		}
		footerText = strings.Repeat(" ", space) + closeHint
	}
	lines = append(lines, styles.Help.Width(innerW).Render(""), styles.Help.Width(innerW).Render(footerText))

	title := " Video Details "
	top := accent.Render("╭─" + title + strings.Repeat("─", innerW-len(title)-1) + "╮")
	bot := accent.Render("╰" + strings.Repeat("─", innerW) + "╯")
	rows := make([]string, 0, panelH)
	rows = append(rows, top)
	for _, l := range lines {
		rows = append(rows, accent.Render("│")+l+accent.Render("│"))
	}
	rows = append(rows, bot)
	return strings.Join(rows, "\n")
}

func (vd VideoDetail) renderLinksModal(behind string, width int) string {
	if vd.links == nil {
		return behind
	}
	links := *vd.links
	const innerW = 56
	lines := []string{styles.Bold.Render("Links in description"), ""}
	for i, lnk := range links {
		num := fmt.Sprintf("%2d. ", i+1)
		text := lnk.Label
		if text == "" {
			text = lnk.URL
		}
		text = render.Truncate(text, innerW-len(num)-2)
		row := num + text
		if i == vd.linkSel {
			lines = append(lines, styles.Selected.Render("▶ "+row))
		} else {
			lines = append(lines, "  "+row)
		}
	}
	if len(links) > 0 {
		lines = append(lines, "", styles.Help.Render(render.Truncate(links[vd.linkSel].URL, innerW)))
	}
	actionHint := "enter: open  y: copy"
	closeHint := vd.keys.Escape.Help().Key + ": close"
	space := innerW - lipgloss.Width(actionHint) - lipgloss.Width(closeHint)
	if space < 1 {
		space = 1
	}
	lines = append(lines, "", styles.Help.Render(actionHint+strings.Repeat(" ", space)+closeHint))
	return placeOverlayBox(behind, strings.Join(lines, "\n"), width, innerW+6)
}

func (vd VideoDetail) renderChaptersModal(behind string, width int) string {
	if vd.chapters == nil {
		return behind
	}
	chapters := *vd.chapters
	const innerW = 58
	lines := []string{styles.Bold.Render("Chapters"), ""}
	for i, ch := range chapters {
		ts := fmtChapterTime(ch.OriginalStart)
		label := fmt.Sprintf("%-7s  %s", ts, render.Truncate(ch.Title, innerW-11))
		if i == vd.chapterSel {
			lines = append(lines, styles.Selected.Render("▶ "+label))
		} else {
			lines = append(lines, "  "+label)
		}
	}
	playKey := vd.keys.Play.Help().Key
	audioKey := vd.keys.PlayAudio.Help().Key
	copyKey := vd.keys.CopyURL.Help().Key
	closeKey := vd.keys.Escape.Help().Key
	actionHint := fmt.Sprintf("%s: stream  %s: audio  %s: copy url", playKey, audioKey, copyKey)
	closeHint := closeKey + ": close"
	space := innerW - lipgloss.Width(actionHint) - lipgloss.Width(closeHint)
	if space < 1 {
		space = 1
	}
	lines = append(lines, "", styles.Help.Render(actionHint+strings.Repeat(" ", space)+closeHint))
	return placeOverlayBox(behind, strings.Join(lines, "\n"), width, innerW+6)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (vd VideoDetail) thumbDimensions() (w, h int) {
	thumbW := panelW - 2
	thumbH := (thumbW*9 + 15) / 16 / 2
	if thumbH < 1 {
		thumbH = 1
	}
	if vd.thumb != nil {
		b := vd.thumb.Bounds()
		iw := b.Max.X - b.Min.X
		ih := b.Max.Y - b.Min.Y
		if iw > 0 && ih > 0 {
			if h := (thumbW*ih + iw - 1) / iw / 2; h >= 1 {
				thumbH = h
			}
		}
	}
	return thumbW, thumbH
}

func (vd VideoDetail) fetchCmd(videoURL string) tea.Cmd {
	return func() tea.Msg {
		details, err := vd.backend.VideoDetails(context.Background(), videoURL)
		return vdDetailsMsg{details: details, err: err}
	}
}

func fmtChapterTime(secs float64) string {
	s := int(secs)
	h, min, sec := s/3600, (s%3600)/60, s%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, min, sec)
	}
	return fmt.Sprintf("%d:%02d", min, sec)
}

// wordWrap splits text into lines of at most width visible characters,
// breaking at word boundaries. Long tokens are hard-broken. Newlines preserved.
func wordWrap(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var result []string
	for _, para := range strings.Split(text, "\n") {
		if lipgloss.Width(para) <= width {
			result = append(result, para)
			continue
		}
		words := strings.Fields(para)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		cur := ""
		for _, w := range words {
			if lipgloss.Width(w) > width {
				if cur != "" {
					result = append(result, cur)
					cur = ""
				}
				runes := []rune(w)
				for len(runes) > 0 {
					taken, col := 0, 0
					for taken < len(runes) {
						cw := runeWidth(runes[taken])
						if col+cw > width {
							break
						}
						col += cw
						taken++
					}
					if taken == 0 {
						taken = 1
					}
					result = append(result, string(runes[:taken]))
					runes = runes[taken:]
				}
				continue
			}
			candidate := w
			if cur != "" {
				candidate = cur + " " + w
			}
			if lipgloss.Width(candidate) <= width {
				cur = candidate
			} else {
				result = append(result, cur)
				cur = w
			}
		}
		if cur != "" {
			result = append(result, cur)
		}
	}
	return result
}

func runeWidth(r rune) int {
	// Fast path: ASCII is always 1 wide.
	if r < 128 {
		return 1
	}
	return lipgloss.Width(string(r))
}
