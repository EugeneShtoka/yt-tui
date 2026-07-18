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
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	chSortDate     = 0
	chSortName     = 1
	chSortSubs     = 2
	chSortViews    = 3
	chSortVidName  = 4
	chSortDuration = 5
	chSortTags     = 6
)

const (
	chEditNone  = 0
	chEditAlias = 1
	chEditTags  = 2
)

const (
	colChName = 22
	colChSubs = 8
	colChTags = 14
)

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
type chAuxLoadedMsg struct {
	positions   map[string]int64
	watched     map[string]bool
	localStatus map[string]domain.VideoStatus
}

type Channels struct {
	backend            api.Backend
	keys               keymap.KeyMap
	circular           bool
	channelLatestCount int

	width, height int

	subs      channels.ChannelSet
	chLatest  map[string]domain.Video
	subVideos []domain.Video
	loading   bool
	sortMode  int
	spinner   spinner.Model

	positions   map[string]int64
	watched     map[string]bool
	localStatus map[string]domain.VideoStatus

	pane          int
	chVideos      []domain.Video
	chVidsLoading bool
	chVidsRefresh bool
	activeChID    string
	activeChURL   string

	tagsMode bool
	tagSel   string

	editMode  int
	editInput textinput.Model

	chTable     table.Model
	chVidTable  table.Model
	tagTable    table.Model
	tagVidTable table.Model
	numBuf      string
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
		chTable:            newTable(),
		chVidTable:         newTable(),
		tagTable:           newTable(),
		tagVidTable:        newTable(),
	}
}

func (t Channels) ID() tuipkg.TabID          { return tuipkg.TabChannels }
func (t Channels) Title() string             { return "Channels" }
func (t Channels) ShortHelp() []key.Binding { return nil }
func (t Channels) InterceptsInput() bool     { return t.editInput.Focused() }

func (t Channels) Init() tea.Cmd {
	t.loading = true
	return tea.Batch(t.chsLoadCmd(), t.chAuxLoadCmd(), t.spinner.Tick)
}

func (t Channels) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.chTable.SetColumns(t.chChannelColumns())
		t.chTable.SetHeight(t.height - 2)
		t.chTable.SetRows(t.toChannelRows(t.sortedChannels()))
		t.chVidTable.SetColumns(computeVideoColumns(t.width, false))
		t.chVidTable.SetHeight(t.height - 4)
		t.chVidTable.SetRows(toVideoRows(t.chVideos, t.positions, t.watched, t.localStatus, false))
		t.tagTable.SetColumns(t.chTagColumns())
		t.tagTable.SetHeight(t.height - 2)
		t.tagTable.SetRows(t.toTagRows())
		t.tagVidTable.SetColumns(computeVideoColumns(t.width, true))
		t.tagVidTable.SetHeight(t.height - 4)

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
		t.chTable.SetRows(t.toChannelRows(t.sortedChannels()))
		t.tagTable.SetRows(t.toTagRows())

	case chAuxLoadedMsg:
		t.positions = m.positions
		t.watched = m.watched
		t.localStatus = m.localStatus
		t.chVidTable.SetRows(toVideoRows(t.chVideos, t.positions, t.watched, t.localStatus, false))

	case tuipkg.RefreshPositionsMsg:
		return t, t.chAuxLoadCmd()

	case chVideosCachedMsg:
		if m.channelID == t.activeChID {
			t.chVideos = m.videos
			t.chVidsLoading = false
			t.chVidsRefresh = true
			t.chVidTable.SetRows(toVideoRows(t.chVideos, t.positions, t.watched, t.localStatus, false))
			return t, t.chVideosFetchCmd()
		}

	case chVideosFetchedMsg:
		if m.channelID == t.activeChID {
			t.chVideos = m.videos
			t.chVidsLoading = false
			t.chVidsRefresh = false
			t.chVidTable.SetRows(toVideoRows(t.chVideos, t.positions, t.watched, t.localStatus, false))
		}

	case tea.KeyMsg:
		if t.editMode != chEditNone {
			return t.handleEditInput(m)
		}
		return t.handleKey(m)
	}
	return t, nil
}

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
		return t.viewTags(header, contentH)
	}
	return t.viewFlat(header, contentH)
}

func (t Channels) viewFlat(header string, contentH int) string {
	_ = contentH
	if t.pane == 0 {
		var body string
		switch {
		case t.loading && t.subs.Len() == 0:
			body = t.spinner.View() + " Loading channels…"
		case t.subs.Len() == 0:
			body = styles.Dim.Render("No channels found.")
		default:
			body = t.chTable.View()
		}
		if t.editMode != chEditNone {
			body = t.appendEditInput(body)
		}
		parts := []string{header, body}
		if t.numBuf != "" {
			parts = append(parts, gotoLineView(t.numBuf))
		}
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}

	sorted := t.sortedChannels()
	chName := ""
	if t.chTable.Cursor() < len(sorted) {
		chName = sorted[t.chTable.Cursor()].DisplayName()
	}
	subHeaderText := "← " + chName
	if t.chVidsRefresh {
		subHeaderText += "  " + styles.Dim.Render(t.spinner.View()+" refreshing…")
	}
	subHeader := styles.SectionTitle.Render(subHeaderText)
	var body string
	if t.chVidsLoading {
		body = t.spinner.View() + " Loading…"
	} else {
		body = t.chVidTable.View()
	}
	parts := []string{header, subHeader, body}
	if t.numBuf != "" {
		parts = append(parts, gotoLineView(t.numBuf))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t Channels) viewTags(header string, contentH int) string {
	_ = contentH
	if t.pane == 1 {
		tagHeader := styles.SectionTitle.Render("← " + tagDisplayName(t.tagSel))
		parts := []string{header, tagHeader, t.tagVidTable.View()}
		if t.numBuf != "" {
			parts = append(parts, gotoLineView(t.numBuf))
		}
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}
	var body string
	if t.loading && t.subs.Len() == 0 {
		body = t.spinner.View() + " Loading channels…"
	} else {
		body = t.tagTable.View()
	}
	parts := []string{header, body}
	if t.numBuf != "" {
		parts = append(parts, gotoLineView(t.numBuf))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t Channels) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keys := t.keys

	if key.Matches(msg, keys.ToggleMode) {
		t.tagsMode = !t.tagsMode
		t.pane = 0
		t.tagTable.GotoTop()
		t.numBuf = ""
		return t, nil
	}

	if checkGotoNum(&t.numBuf, msg) {
		return t, nil
	}
	numBuf := t.numBuf
	t.numBuf = ""

	// GotoLine dispatched to whichever table is currently active
	if key.Matches(msg, keys.GotoLine) {
		var tbl *table.Model
		switch {
		case t.tagsMode && t.pane == 1:
			tbl = &t.tagVidTable
		case t.tagsMode:
			tbl = &t.tagTable
		case t.pane == 1:
			tbl = &t.chVidTable
		default:
			tbl = &t.chTable
		}
		if numBuf != "" {
			applyGoto(numBuf, tbl)
		} else {
			tbl.GotoBottom()
		}
		return t, nil
	}

	if t.tagsMode {
		return t.handleKeyTags(msg, numBuf)
	}
	return t.handleKeyFlat(msg, numBuf)
}

func (t Channels) handleKeyFlat(msg tea.KeyMsg, numBuf string) (tea.Model, tea.Cmd) {
	keys := t.keys

	if t.pane == 0 {
		sorted := t.sortedChannels()
		n := len(sorted)
		switch {
		case key.Matches(msg, keys.Up):
			if t.circular && n > 0 && t.chTable.Cursor() == 0 {
				t.chTable.GotoBottom()
			} else {
				t.chTable.MoveUp(1)
			}
		case key.Matches(msg, keys.Down):
			if t.circular && n > 0 && t.chTable.Cursor() == n-1 {
				t.chTable.GotoTop()
			} else {
				t.chTable.MoveDown(1)
			}
		case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
			if t.chTable.Cursor() < n {
				ch := sorted[t.chTable.Cursor()]
				t.pane = 1
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
				t.chVidTable.GotoTop()
				t.chVidTable.SetRows(nil)
				return t, tea.Batch(t.chDrilldownCmd(ch), t.spinner.Tick)
			}
		case key.Matches(msg, keys.RenameChannel):
			if t.chTable.Cursor() < n {
				ch := sorted[t.chTable.Cursor()]
				t.editInput.SetValue(ch.Alias)
				t.editInput.Placeholder = "alias (empty to clear)…"
				t.editInput.Focus()
				t.editMode = chEditAlias
				return t, textinput.Blink
			}
		case key.Matches(msg, keys.TagChannel):
			if t.chTable.Cursor() < n {
				ch := sorted[t.chTable.Cursor()]
				t.editInput.SetValue(strings.Join(ch.Tags, ", "))
				t.editInput.Placeholder = "comma-separated tags…"
				t.editInput.Focus()
				t.editMode = chEditTags
				return t, textinput.Blink
			}
		case key.Matches(msg, keys.Unsubscribe):
			if t.chTable.Cursor() < n {
				ch := sorted[t.chTable.Cursor()]
				t.subs.Remove(ch)
				t.chTable.SetRows(t.toChannelRows(t.sortedChannels()))
				return t, func() tea.Msg { return tuipkg.UnsubscribeMsg{Channel: ch} }
			}
		}
		_ = numBuf
		return t, nil
	}

	n := len(t.chVideos)
	switch {
	case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
		if numBuf != "" {
			return t, nil
		}
		t.pane = 0
	case key.Matches(msg, keys.Up):
		if t.circular && n > 0 && t.chVidTable.Cursor() == 0 {
			t.chVidTable.GotoBottom()
		} else {
			t.chVidTable.MoveUp(1)
		}
	case key.Matches(msg, keys.Down):
		if t.circular && n > 0 && t.chVidTable.Cursor() == n-1 {
			t.chVidTable.GotoTop()
		} else {
			t.chVidTable.MoveDown(1)
		}
	case key.Matches(msg, keys.PageUp):
		t.chVidTable.MoveUp(t.chVidTable.Height())
	case key.Matches(msg, keys.PageDown):
		t.chVidTable.MoveDown(t.chVidTable.Height())
	case key.Matches(msg, keys.GotoBottom):
		t.chVidTable.GotoBottom()
	case key.Matches(msg, keys.Unsubscribe):
		sorted := t.sortedChannels()
		if t.chTable.Cursor() < len(sorted) {
			ch := sorted[t.chTable.Cursor()]
			t.subs.Remove(ch)
			t.pane = 0
			t.chTable.SetRows(t.toChannelRows(t.sortedChannels()))
			return t, func() tea.Msg { return tuipkg.UnsubscribeMsg{Channel: ch} }
		}
	default:
		if v, ok := t.currentChVideo(); ok {
			return t.handleVideoAction(msg, v)
		}
	}
	return t, nil
}

func (t Channels) handleKeyTags(msg tea.KeyMsg, numBuf string) (tea.Model, tea.Cmd) {
	keys := t.keys

	if t.pane == 0 {
		items := t.allTags()
		n := len(items)
		switch {
		case key.Matches(msg, keys.Up):
			if t.circular && n > 0 && t.tagTable.Cursor() == 0 {
				t.tagTable.GotoBottom()
			} else {
				t.tagTable.MoveUp(1)
			}
		case key.Matches(msg, keys.Down):
			if t.circular && n > 0 && t.tagTable.Cursor() == n-1 {
				t.tagTable.GotoTop()
			} else {
				t.tagTable.MoveDown(1)
			}
		case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
			if t.tagTable.Cursor() < n {
				t.tagSel = items[t.tagTable.Cursor()]
				vids := t.tagVideos()
				t.tagVidTable.SetRows(toVideoRows(vids, t.positions, t.watched, t.localStatus, true))
				t.tagVidTable.GotoTop()
				t.pane = 1
			}
		}
		_ = numBuf
		return t, nil
	}

	vids := t.tagVideos()
	n := len(vids)
	switch {
	case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
		if numBuf != "" {
			return t, nil
		}
		t.pane = 0
		t.tagVidTable.GotoTop()
	case key.Matches(msg, keys.Up):
		if t.circular && n > 0 && t.tagVidTable.Cursor() == 0 {
			t.tagVidTable.GotoBottom()
		} else {
			t.tagVidTable.MoveUp(1)
		}
	case key.Matches(msg, keys.Down):
		if t.circular && n > 0 && t.tagVidTable.Cursor() == n-1 {
			t.tagVidTable.GotoTop()
		} else {
			t.tagVidTable.MoveDown(1)
		}
	case key.Matches(msg, keys.PageUp):
		t.tagVidTable.MoveUp(t.tagVidTable.Height())
	case key.Matches(msg, keys.PageDown):
		t.tagVidTable.MoveDown(t.tagVidTable.Height())
	case key.Matches(msg, keys.GotoBottom):
		t.tagVidTable.GotoBottom()
	default:
		if t.tagVidTable.Cursor() < n {
			return t.handleVideoAction(msg, vids[t.tagVidTable.Cursor()])
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
		if t.chTable.Cursor() < len(sorted) {
			ch := sorted[t.chTable.Cursor()]
			if t.editMode == chEditAlias {
				t.subs.SetAlias(ch.ID, val)
				t.chTable.SetRows(t.toChannelRows(t.sortedChannels()))
				t.editMode = chEditNone
				t.editInput.Blur()
				if val == "" {
					return t, t.chSetAliasCmd(ch.ID, val, "Alias cleared")
				}
				return t, t.chSetAliasCmd(ch.ID, val, "Alias set: "+val)
			}
			tags := parseTags(val)
			t.subs.SetTags(ch.ID, tags)
			t.chTable.SetRows(t.toChannelRows(t.sortedChannels()))
			t.editMode = chEditNone
			t.editInput.Blur()
			return t, t.chSetTagsCmd(ch.ID, tags)
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
			return strings.ToLower(t.chLatest[out[i].ID].Title) < strings.ToLower(t.chLatest[out[j].ID].Title)
		})
	case chSortDuration:
		sort.SliceStable(out, func(i, j int) bool {
			return t.chLatest[out[i].ID].Duration > t.chLatest[out[j].ID].Duration
		})
	case chSortTags:
		sort.SliceStable(out, func(i, j int) bool {
			ti, tj := chFirstTag(out[i].Tags), chFirstTag(out[j].Tags)
			if ti != tj {
				return ti < tj
			}
			return strings.ToLower(out[i].DisplayName()) < strings.ToLower(out[j].DisplayName())
		})
	}
	return out
}

func (t Channels) channelsInTag(tag string) []domain.Channel {
	var out []domain.Channel
	for _, ch := range t.subs.Channels() {
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
	c := t.chVidTable.Cursor()
	if c >= 0 && c < len(t.chVideos) {
		return t.chVideos[c], true
	}
	return domain.Video{}, false
}

func (t Channels) chChannelColumns() []table.Column {
	titleW := t.width - render.ColNum - colIndicator - colChName - colChTags - colChSubs - render.ColDuration - render.ColViews - render.ColDate
	if titleW < 10 {
		titleW = 10
	}
	return []table.Column{
		{Title: "#", Width: render.ColNum},
		{Title: " ", Width: colIndicator},
		{Title: "Channel", Width: colChName},
		{Title: "Tags", Width: colChTags},
		{Title: "Subs", Width: colChSubs},
		{Title: "Latest Video", Width: titleW},
		{Title: "Duration", Width: render.ColDuration},
		{Title: "Views", Width: render.ColViews},
		{Title: "Date", Width: render.ColDate},
	}
}

func (t Channels) chTagColumns() []table.Column {
	labelW := t.width - render.ColNum - colIndicator
	if labelW < 10 {
		labelW = 10
	}
	return []table.Column{
		{Title: "#", Width: render.ColNum},
		{Title: " ", Width: colIndicator},
		{Title: "Tag", Width: labelW},
	}
}

func (t Channels) toChannelRows(sorted []domain.Channel) []table.Row {
	rows := make([]table.Row, len(sorted))
	for i, ch := range sorted {
		latest := t.chLatest[ch.ID]
		rows[i] = table.Row{
			rowNum(i), "  ",
			ch.DisplayName(),
			strings.Join(ch.Tags, ", "),
			render.Views(ch.Subscribers),
			latest.Title,
			latest.DurationStr(),
			latest.ViewsStr(),
			latest.DateStr(),
		}
	}
	return rows
}

func (t Channels) toTagRows() []table.Row {
	items := t.allTags()
	rows := make([]table.Row, len(items))
	for i, tag := range items {
		count := len(t.channelsInTag(tag))
		rows[i] = table.Row{rowNum(i), "  ", fmt.Sprintf("%s (%d)", tagDisplayName(tag), count)}
	}
	return rows
}

func (t Channels) appendEditInput(body string) string {
	label := "Alias: "
	if t.editMode == chEditTags {
		label = "Tags (comma-separated): "
	}
	return body + "\n" + styles.Bold.Render(label) + t.editInput.View()
}

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

func (t Channels) chAuxLoadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		positions, _ := t.backend.AllVideoPositions(ctx)
		watched, _ := t.backend.WatchedVideoIDs(ctx)
		localVids, _ := t.backend.LocalVideos(ctx)
		localStatus := make(map[string]domain.VideoStatus, len(localVids))
		for i := range localVids {
			localStatus[localVids[i].ID] = localVids[i].Status
		}
		return chAuxLoadedMsg{positions: positions, watched: watched, localStatus: localStatus}
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
		_ = mode
		if err := t.backend.SetChannelAlias(context.Background(), channelID, alias); err != nil {
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
