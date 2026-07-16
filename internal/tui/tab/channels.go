package tab

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/channels"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/nav"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── channel sort modes ────────────────────────────────────────────────────────

const (
	chSortDate     = 0
	chSortName     = 1
	chSortSubs     = 2
	chSortViews    = 3
	chSortVidName  = 4
	chSortDuration = 5
	chSortTags     = 6
)

// ── edit mode ─────────────────────────────────────────────────────────────────

const (
	chEditNone  = 0
	chEditAlias = 1
	chEditTags  = 2
)

// ── column widths ─────────────────────────────────────────────────────────────

const (
	colChName = 22
	colChSubs = 8
	colChTags = 14
)

// ── tab-private messages ──────────────────────────────────────────────────────

type chsLoadedMsg struct {
	chans     []domain.Channel
	latest    map[string]domain.Video
	subVideos []domain.Video
}

type chVideosCachedMsg struct {
	channelID string
	videos    []domain.Video
}

type chVideosFetchedMsg struct {
	channelID string
	videos    []domain.Video
}

// ── Channels ──────────────────────────────────────────────────────────────────

// Channels is the Channels tab: subscribed-channel list with flat and tag modes.
type Channels struct {
	backend            api.Backend
	keys               keymap.KeyMap
	circular           bool
	channelLatestCount int

	width, height int

	subs      channels.ChannelSet
	chLatest  map[string]domain.Video
	subVideos []domain.Video // all subscription videos for tag-mode filtering
	loading   bool
	sortMode  int
	spinner   spinner.Model

	// flat mode
	cursor        int
	vs            int
	pane          int // 0=channel list, 1=channel video pane
	chVideos      []domain.Video
	chVidsLoading bool
	chVidsRefresh bool
	vidCursor     int
	vidVS         int
	activeChID    string
	activeChURL   string

	// tags mode
	tagsMode     bool
	tagCursor    int
	tagVS        int
	tagSel       string
	tagVidCursor int
	tagVidVS     int

	// inline alias/tags edit
	editMode  int
	editInput textinput.Model
}

func NewChannels(backend api.Backend, keys keymap.KeyMap, circular bool, channelLatestCount int) Channels {
	return Channels{
		backend:            backend,
		keys:               keys,
		circular:           circular,
		channelLatestCount: channelLatestCount,
		sortMode:           chSortDate,
		spinner:            spinner.New(),
		editInput:          textinput.New(),
	}
}

// ── tui.Tab interface ─────────────────────────────────────────────────────────

func (t Channels) ID() tuipkg.TabID { return tuipkg.TabChannels }
func (t Channels) Title() string    { return "Channels" }
func (t Channels) ShortHelp() []key.Binding { return nil }

func (t Channels) InterceptsInput() bool { return t.editInput.Focused() }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (t Channels) Init() tea.Cmd {
	t.loading = true
	return tea.Batch(t.chsLoadCmd(), t.spinner.Tick)
}

func (t Channels) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height

	case spinner.TickMsg:
		if t.loading || t.chVidsLoading || t.chVidsRefresh {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(m)
			return t, cmd
		}

	case chsLoadedMsg:
		t.subs = channels.New(m.chans)
		t.chLatest = m.latest
		t.subVideos = m.subVideos
		t.loading = false

	case chVideosCachedMsg:
		if m.channelID == t.activeChID {
			t.chVideos = m.videos
			t.chVidsLoading = false
			t.chVidsRefresh = true
			return t, t.chVideosFetchCmd()
		}

	case chVideosFetchedMsg:
		if m.channelID == t.activeChID {
			t.chVideos = m.videos
			t.chVidsLoading = false
			t.chVidsRefresh = false
		}

	case tea.KeyMsg:
		if t.editMode != chEditNone {
			return t.handleEditInput(m)
		}
		return t.handleKey(m)
	}
	return t, nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (t Channels) View() string {
	headerText := "Channels"
	if t.loading {
		headerText += "  " + styles.Dim.Render(t.spinner.View()+" loading…")
	}
	if t.tagsMode {
		headerText += "  " + styles.Dim.Render("[tags]")
	}
	header := styles.SectionTitle.Render(headerText)
	headerH := lipgloss.Height(header)
	contentH := t.height - headerH

	if t.tagsMode {
		return t.viewTags(header, headerH, contentH)
	}
	return t.viewFlat(header, headerH, contentH)
}

func (t Channels) viewFlat(header string, headerH, contentH int) string {
	if t.pane == 0 {
		var body string
		switch {
		case t.loading && t.subs.Len() == 0:
			body = t.spinner.View() + " Loading channels…"
		case t.subs.Len() == 0:
			body = styles.Dim.Render("No channels found.")
		default:
			body = t.renderChannelList(contentH)
		}
		if t.editMode != chEditNone {
			body = t.appendEditInput(body)
		}
		return lipgloss.JoinVertical(lipgloss.Left, header, body)
	}

	// video pane
	sorted := t.sortedChannels()
	chName := ""
	if t.cursor < len(sorted) {
		chName = sorted[t.cursor].DisplayName()
	}
	subHeaderText := "← " + chName
	if t.chVidsRefresh {
		subHeaderText += "  " + styles.Dim.Render(t.spinner.View()+" refreshing…")
	}
	subHeader := styles.SectionTitle.Render(subHeaderText)
	subH := lipgloss.Height(subHeader)
	videoH := t.height - headerH - subH

	var body string
	if t.chVidsLoading {
		body = t.spinner.View() + " Loading…"
	} else {
		ctx := VideoListCtx{Width: t.width, ShowChannel: false}
		body = renderVideoRows(ctx, t.chVideos, t.vidCursor, t.vidVS, videoH)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, subHeader, body)
}

func (t Channels) viewTags(header string, headerH, contentH int) string {
	if t.pane == 1 {
		tagHeader := styles.SectionTitle.Render("← " + tagDisplayName(t.tagSel))
		tagH := lipgloss.Height(tagHeader)
		vids := t.tagVideos()
		ctx := VideoListCtx{Width: t.width, ShowChannel: true}
		body := renderVideoRows(ctx, vids, t.tagVidCursor, t.tagVidVS, t.height-headerH-tagH)
		return lipgloss.JoinVertical(lipgloss.Left, header, tagHeader, body)
	}
	var body string
	if t.loading && t.subs.Len() == 0 {
		body = t.spinner.View() + " Loading channels…"
	} else {
		body = t.renderTagList(contentH)
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}

// ── key handling ──────────────────────────────────────────────────────────────

func (t Channels) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys

	if key.Matches(msg, keys.ToggleMode) {
		t.tagsMode = !t.tagsMode
		t.pane = 0
		t.tagCursor = 0
		t.tagVS = 0
		return t, nil
	}

	if t.tagsMode {
		return t.handleKeyTags(msg)
	}
	return t.handleKeyFlat(msg)
}

func (t Channels) handleKeyFlat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	pageH := t.pageHeight()

	// ── channel list pane ──────────────────────────────────────────────────────
	if t.pane == 0 {
		sorted := t.sortedChannels()
		n := len(sorted)
		switch {
		case key.Matches(msg, keys.Up):
			t.cursor, t.vs = nav.Move(t.cursor, t.vs, n, -1, pageH, t.circular)
		case key.Matches(msg, keys.Down):
			t.cursor, t.vs = nav.Move(t.cursor, t.vs, n, +1, pageH, t.circular)
		case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
			if t.cursor < n {
				ch := sorted[t.cursor]
				t.pane = 1
				t.vidCursor = 0
				t.vidVS = 0
				if ch.ID == t.activeChID && len(t.chVideos) > 0 {
					t.chVidsLoading = false
					t.chVidsRefresh = true
					return t, tea.Batch(t.chVideosFetchCmd(), t.spinner.Tick)
				}
				t.activeChID = ch.ID
				t.activeChURL = ch.URL
				t.chVideos = nil
				t.chVidsLoading = true
				t.chVidsRefresh = false
				return t, tea.Batch(t.chDrilldownCmd(ch), t.spinner.Tick)
			}
		case key.Matches(msg, keys.RenameChannel):
			if t.cursor < n {
				ch := sorted[t.cursor]
				t.editInput.SetValue(ch.Alias)
				t.editInput.Placeholder = "alias (empty to clear)…"
				t.editInput.Focus()
				t.editMode = chEditAlias
				return t, textinput.Blink
			}
		case key.Matches(msg, keys.TagChannel):
			if t.cursor < n {
				ch := sorted[t.cursor]
				t.editInput.SetValue(strings.Join(ch.Tags, ", "))
				t.editInput.Placeholder = "comma-separated tags…"
				t.editInput.Focus()
				t.editMode = chEditTags
				return t, textinput.Blink
			}
		case key.Matches(msg, keys.Unsubscribe):
			if t.cursor < n {
				ch := sorted[t.cursor]
				t.subs.Remove(ch)
				newN := t.subs.Len()
				if t.cursor >= newN {
					if newN > 0 {
						t.cursor = newN - 1
					} else {
						t.cursor, t.vs = 0, 0
					}
				}
				return t, func() tea.Msg { return tuipkg.UnsubscribeMsg{Channel: ch} }
			}
		}
		return t, nil
	}

	// ── channel video pane ─────────────────────────────────────────────────────
	n := len(t.chVideos)
	switch {
	case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
		t.pane = 0
	case key.Matches(msg, keys.Up):
		t.vidCursor, t.vidVS = nav.Move(t.vidCursor, t.vidVS, n, -1, pageH, t.circular)
	case key.Matches(msg, keys.Down):
		t.vidCursor, t.vidVS = nav.Move(t.vidCursor, t.vidVS, n, +1, pageH, t.circular)
	case key.Matches(msg, keys.PageUp):
		t.vidCursor, t.vidVS = nav.Page(t.vidCursor, t.vidVS, n, -1, pageH, t.circular)
	case key.Matches(msg, keys.PageDown):
		t.vidCursor, t.vidVS = nav.Page(t.vidCursor, t.vidVS, n, +1, pageH, t.circular)
	case key.Matches(msg, keys.GotoBottom):
		t.vidCursor, t.vidVS = nav.Jump(n-1, n, pageH)
	case key.Matches(msg, keys.Unsubscribe):
		sorted := t.sortedChannels()
		if t.cursor < len(sorted) {
			ch := sorted[t.cursor]
			t.subs.Remove(ch)
			t.pane = 0
			newN := t.subs.Len()
			if t.cursor >= newN && newN > 0 {
				t.cursor = newN - 1
			}
			return t, func() tea.Msg { return tuipkg.UnsubscribeMsg{Channel: ch} }
		}
	default:
		if v, ok := t.currentChVideo(); ok {
			return t.handleVideoAction(msg, v)
		}
	}
	return t, nil
}

func (t Channels) handleKeyTags(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	pageH := t.pageHeight()

	if t.pane == 0 {
		// tag list
		items := t.allTags()
		n := len(items)
		switch {
		case key.Matches(msg, keys.Up):
			t.tagCursor, t.tagVS = nav.Move(t.tagCursor, t.tagVS, n, -1, pageH, t.circular)
		case key.Matches(msg, keys.Down):
			t.tagCursor, t.tagVS = nav.Move(t.tagCursor, t.tagVS, n, +1, pageH, t.circular)
		case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
			if t.tagCursor < n {
				t.tagSel = items[t.tagCursor]
				t.tagVidCursor = 0
				t.tagVidVS = 0
				t.pane = 1
			}
		}
		return t, nil
	}

	// tag video list
	vids := t.tagVideos()
	n := len(vids)
	switch {
	case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
		t.pane = 0
		t.tagVidCursor = 0
		t.tagVidVS = 0
	case key.Matches(msg, keys.Up):
		t.tagVidCursor, t.tagVidVS = nav.Move(t.tagVidCursor, t.tagVidVS, n, -1, pageH, t.circular)
	case key.Matches(msg, keys.Down):
		t.tagVidCursor, t.tagVidVS = nav.Move(t.tagVidCursor, t.tagVidVS, n, +1, pageH, t.circular)
	case key.Matches(msg, keys.PageUp):
		t.tagVidCursor, t.tagVidVS = nav.Page(t.tagVidCursor, t.tagVidVS, n, -1, pageH, t.circular)
	case key.Matches(msg, keys.PageDown):
		t.tagVidCursor, t.tagVidVS = nav.Page(t.tagVidCursor, t.tagVidVS, n, +1, pageH, t.circular)
	case key.Matches(msg, keys.GotoBottom):
		t.tagVidCursor, t.tagVidVS = nav.Jump(n-1, n, pageH)
	default:
		if t.tagVidCursor < n {
			return t.handleVideoAction(msg, vids[t.tagVidCursor])
		}
	}
	return t, nil
}

func (t Channels) handleVideoAction(msg tea.KeyMsg, v domain.Video) (tea.Model, tea.Cmd) {
	keys := t.keys
	switch {
	case key.Matches(msg, keys.Play):
		return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v} }
	case key.Matches(msg, keys.PlayAudio):
		return t, func() tea.Msg { return tuipkg.PlayVideoMsg{Video: v, AudioOnly: true} }
	case key.Matches(msg, keys.Download):
		return t, func() tea.Msg { return tuipkg.EnqueueMsg{Video: v} }
	case key.Matches(msg, keys.DownloadAudio):
		return t, func() tea.Msg { return tuipkg.EnqueueMsg{Video: v, AudioOnly: true} }
	case key.Matches(msg, keys.CopyURL):
		return t, func() tea.Msg { return tuipkg.CopyURLMsg{URL: v.URL} }
	case key.Matches(msg, keys.VideoInfo):
		return t, func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "video_detail", Video: v} }
	case key.Matches(msg, keys.AddList):
		return t, func() tea.Msg { return tuipkg.OpenOverlayMsg{Kind: "add_to_playlist", Video: v} }
	}
	return t, nil
}

func (t Channels) handleEditInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	switch {
	case key.Matches(msg, keys.Escape):
		t.editMode = chEditNone
		t.editInput.Blur()
		return t, nil
	case key.Matches(msg, keys.DrillDown):
		val := strings.TrimSpace(t.editInput.Value())
		sorted := t.sortedChannels()
		if t.cursor < len(sorted) {
			ch := sorted[t.cursor]
			if t.editMode == chEditAlias {
				t.subs.SetAlias(ch.ID, val)
				if val == "" {
					return t, t.chSetAliasCmd(ch.ID, val, "Alias cleared")
				}
				return t, t.chSetAliasCmd(ch.ID, val, "Alias set: "+val)
			} else {
				tags := parseTags(val)
				t.subs.SetTags(ch.ID, tags)
				return t, t.chSetTagsCmd(ch.ID, tags)
			}
		}
		t.editMode = chEditNone
		t.editInput.Blur()
		return t, nil
	default:
		var cmd tea.Cmd
		t.editInput, cmd = t.editInput.Update(msg)
		return t, cmd
	}
}

// ── data helpers ──────────────────────────────────────────────────────────────

func (t Channels) sortedChannels() []domain.Channel {
	return t.sortChannelSlice(t.subs.Channels())
}

func (t Channels) sortChannelSlice(chs []domain.Channel) []domain.Channel {
	out := make([]domain.Channel, len(chs))
	copy(out, chs)
	switch t.sortMode {
	case chSortDate:
		sort.SliceStable(out, func(i, j int) bool {
			return t.chLatest[out[i].ID].UploadDate > t.chLatest[out[j].ID].UploadDate
		})
	case chSortName:
		sort.SliceStable(out, func(i, j int) bool {
			return strings.ToLower(out[i].DisplayName()) < strings.ToLower(out[j].DisplayName())
		})
	case chSortSubs:
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Subscribers > out[j].Subscribers
		})
	case chSortViews:
		sort.SliceStable(out, func(i, j int) bool {
			return t.chLatest[out[i].ID].ViewCount > t.chLatest[out[j].ID].ViewCount
		})
	case chSortVidName:
		sort.SliceStable(out, func(i, j int) bool {
			ti := strings.ToLower(t.chLatest[out[i].ID].Title)
			tj := strings.ToLower(t.chLatest[out[j].ID].Title)
			return ti < tj
		})
	case chSortDuration:
		sort.SliceStable(out, func(i, j int) bool {
			return t.chLatest[out[i].ID].Duration > t.chLatest[out[j].ID].Duration
		})
	case chSortTags:
		sort.SliceStable(out, func(i, j int) bool {
			ti := chFirstTag(out[i].Tags)
			tj := chFirstTag(out[j].Tags)
			if ti != tj {
				return ti < tj
			}
			return strings.ToLower(out[i].DisplayName()) < strings.ToLower(out[j].DisplayName())
		})
	}
	return out
}

func (t Channels) channelsInTag(tag string) []domain.Channel {
	all := t.subs.Channels()
	var out []domain.Channel
	for _, ch := range all {
		for _, tg := range ch.Tags {
			if tg == tag {
				out = append(out, ch)
				break
			}
		}
	}
	return out
}

func (t Channels) tagVideos() []domain.Video {
	chans := t.channelsInTag(t.tagSel)
	if len(chans) == 0 {
		return nil
	}
	idSet := make(map[string]bool, len(chans))
	for _, ch := range chans {
		if ch.ID != "" {
			idSet[ch.ID] = true
		}
	}
	var out []domain.Video
	for _, v := range t.subVideos {
		if idSet[v.ChannelID] {
			out = append(out, v)
		}
	}
	feed.SortVideos(out, feed.SortDate)
	return out
}

func (t Channels) allTags() []string {
	seen := map[string]bool{}
	for _, ch := range t.subs.Channels() {
		for _, tg := range ch.Tags {
			if tg != "" {
				seen[tg] = true
			}
		}
	}
	tags := make([]string, 0, len(seen))
	for tg := range seen {
		tags = append(tags, tg)
	}
	sort.Strings(tags)
	return tags
}

func (t Channels) currentChVideo() (domain.Video, bool) {
	if t.vidCursor >= 0 && t.vidCursor < len(t.chVideos) {
		return t.chVideos[t.vidCursor], true
	}
	return domain.Video{}, false
}

func (t Channels) pageHeight() int {
	h := t.height - 4 // header + sub-header + col-header
	if h < 1 {
		h = 1
	}
	return h
}

// ── rendering ─────────────────────────────────────────────────────────────────

func (t Channels) renderChannelList(height int) string {
	sorted := t.sortedChannels()
	if len(sorted) == 0 {
		return ""
	}
	titleW := t.width - render.ColNum - 1 - 2 - colChName - 1 - colChTags - 1 - colChSubs - 1 - render.ColDuration - 1 - render.ColViews - 1 - render.ColDate
	if titleW < 10 {
		titleW = 10
	}

	colHdr := strings.Repeat(" ", render.ColNum) + " " + "  " +
		styles.ColHeader.Width(colChName).Render("Channel") + " " +
		styles.ColHeader.Width(colChTags).Render("Tags") + " " +
		styles.ColHeader.Width(colChSubs).Render("Subs") + " " +
		styles.ColHeader.Width(titleW).Render("Latest Video") + " " +
		styles.ColHeader.Width(render.ColDuration).Render("Duration") + " " +
		styles.ColHeader.Width(render.ColViews).Render("Views") + " " +
		styles.ColHeader.Width(render.ColDate).Render("Date")

	start, end := nav.Window(t.vs, len(sorted), height-1)
	rows := []string{colHdr}
	for i := start; i < end; i++ {
		rows = append(rows, t.renderChannelRow(sorted[i], i == t.cursor, i+1, titleW))
	}
	return strings.Join(rows, "\n")
}

func (t Channels) renderChannelRow(ch domain.Channel, selected bool, num, titleW int) string {
	latest := t.chLatest[ch.ID]

	chName := render.Truncate(ch.DisplayName(), colChName-2)
	tagsStr := render.Truncate(strings.Join(ch.Tags, ", "), colChTags)
	subs := render.Views(ch.Subscribers)
	vidTitle := render.Truncate(latest.Title, titleW)
	dur := latest.DurationStr()
	views := latest.ViewsStr()
	date := latest.DateStr()

	sep := " "
	numStyle := styles.RowNum
	chStyle := styles.Normal.Width(colChName)
	tagsStyle := styles.Dim.Width(colChTags)
	subsStyle := styles.Duration.Width(colChSubs)
	titleStyle := styles.Normal.Width(titleW)
	durStyle := styles.Duration.Width(render.ColDuration)
	viewsStyle := styles.Duration.Width(render.ColViews)
	dateStyle := styles.Channel.Width(render.ColDate)
	indicator := "  "

	if selected {
		indicator = styles.Selected.Render("▶ ")
		numStyle = numStyle.Background(styles.ColorBgSelect)
		sep = lipgloss.NewStyle().Background(styles.ColorBgSelect).Render(" ")
		chStyle = chStyle.Background(styles.ColorBgSelect)
		tagsStyle = tagsStyle.Background(styles.ColorBgSelect)
		subsStyle = subsStyle.Background(styles.ColorBgSelect)
		titleStyle = styles.Selected.Width(titleW)
		durStyle = durStyle.Background(styles.ColorBgSelect)
		viewsStyle = viewsStyle.Background(styles.ColorBgSelect)
		dateStyle = dateStyle.Background(styles.ColorBgSelect)
	}

	numStr := numStyle.Render(fmt.Sprintf("%*d ", render.ColNum, num))
	return numStr + indicator +
		chStyle.Render(chName) + sep +
		tagsStyle.Render(tagsStr) + sep +
		subsStyle.Render(subs) + sep +
		titleStyle.Render(vidTitle) + sep +
		durStyle.Render(dur) + sep +
		viewsStyle.Render(views) + sep +
		dateStyle.Render(date)
}

func (t Channels) renderTagList(height int) string {
	items := t.allTags()
	labelW := t.width - render.ColNum - 3

	colHdr := strings.Repeat(" ", render.ColNum) + " " + "  " +
		styles.ColHeader.Width(labelW).Render("Tag")

	start, end := nav.Window(t.tagVS, len(items), height-1)
	rows := []string{colHdr}
	for i := start; i < end; i++ {
		tag := items[i]
		count := len(t.channelsInTag(tag))
		label := fmt.Sprintf("%s (%d)", tagDisplayName(tag), count)
		selected := i == t.tagCursor

		numStyle := styles.RowNum
		rowStyle := styles.Normal.Width(labelW)
		indicator := "  "

		if selected {
			indicator = styles.Selected.Render("▶ ")
			numStyle = numStyle.Background(styles.ColorBgSelect)
			rowStyle = styles.Selected.Width(labelW)
		}

		numStr := numStyle.Render(fmt.Sprintf("%*d ", render.ColNum, i+1))
		rows = append(rows, numStr+indicator+rowStyle.Render(label))
	}
	return strings.Join(rows, "\n")
}

func (t Channels) appendEditInput(body string) string {
	label := "Alias: "
	if t.editMode == chEditTags {
		label = "Tags (comma-separated): "
	}
	return body + "\n" + styles.Bold.Render(label) + t.editInput.View()
}

// ── background commands ───────────────────────────────────────────────────────

func (t Channels) chsLoadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		chans, err := t.backend.GetSubscribedChannels(ctx)
		if err != nil {
			return tuipkg.StatusMsg{Text: "channels: " + err.Error(), IsErr: true}
		}
		latest, err := t.backend.GetChannelLatestAll(ctx)
		if err != nil {
			latest = make(map[string]domain.Video)
		}
		ids := make([]string, len(chans))
		for i, ch := range chans {
			ids[i] = ch.ID
		}
		subVideos, _ := t.backend.GetAllChannelVideos(ctx, ids)
		feed.SortVideos(subVideos, feed.SortDate)
		return chsLoadedMsg{chans: chans, latest: latest, subVideos: subVideos}
	}
}

func (t Channels) chDrilldownCmd(ch domain.Channel) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		cached, err := t.backend.GetChannelVideos(ctx, ch.ID)
		if err == nil && len(cached) > 0 {
			return chVideosCachedMsg{channelID: ch.ID, videos: cached}
		}
		videos, err := t.backend.ChannelVideos(ctx, ch.URL, ch.ID)
		if err != nil {
			return tuipkg.StatusMsg{Text: "load videos: " + err.Error(), IsErr: true}
		}
		return chVideosFetchedMsg{channelID: ch.ID, videos: videos}
	}
}

func (t Channels) chVideosFetchCmd() tea.Cmd {
	chID, chURL, n := t.activeChID, t.activeChURL, t.channelLatestCount
	return func() tea.Msg {
		ctx := context.Background()
		var videos []domain.Video
		var err error
		if n > 0 {
			videos, err = t.backend.ChannelLatestN(ctx, chURL, chID, n)
		} else {
			videos, err = t.backend.ChannelVideos(ctx, chURL, chID)
		}
		if err != nil {
			return tuipkg.StatusMsg{Text: "refresh: " + err.Error(), IsErr: true}
		}
		return chVideosFetchedMsg{channelID: chID, videos: videos}
	}
}

func (t Channels) chSetAliasCmd(channelID, alias, status string) tea.Cmd {
	mode := t.editMode
	return func() tea.Msg {
		_ = mode // capture to avoid closure-related confusion
		ctx := context.Background()
		if err := t.backend.SetChannelAlias(ctx, channelID, alias); err != nil {
			return tuipkg.StatusMsg{Text: "alias: " + err.Error(), IsErr: true}
		}
		return tuipkg.StatusMsg{Text: status}
	}
}

func (t Channels) chSetTagsCmd(channelID string, tags []string) tea.Cmd {
	return func() tea.Msg {
		if err := t.backend.SetChannelTags(context.Background(), channelID, tags); err != nil {
			return tuipkg.StatusMsg{Text: "tags: " + err.Error(), IsErr: true}
		}
		return tuipkg.StatusMsg{Text: "Tags updated"}
	}
}

// ── pure helpers ──────────────────────────────────────────────────────────────

func tagDisplayName(tag string) string { return tag }

func chFirstTag(tags []string) string {
	if len(tags) == 0 {
		return "\xff"
	}
	return strings.ToLower(tags[0])
}

func parseTags(val string) []string {
	parts := strings.Split(val, ",")
	var tags []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}
