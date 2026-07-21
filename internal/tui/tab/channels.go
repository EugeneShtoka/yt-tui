package tab

import (
	"context"
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/channels"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
	etable "github.com/evertras/bubble-table/table"
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

// ChannelRow is the cell input type for the channel list table.
type ChannelRow struct {
	Channel            domain.Channel
	Latest             domain.Video
	LatestPositionSecs int
}

func (r ChannelRow) GetTitle() string       { return r.Latest.Title }
func (r ChannelRow) GetChannelID() string   { return r.Channel.ID }
func (r ChannelRow) GetChannelName() string { return r.Channel.DisplayName() }
func (r ChannelRow) GetCount() int64        { return r.Channel.Subscribers }
func (r ChannelRow) GetTags() []string      { return r.Channel.Tags }
func (r ChannelRow) GetLatestVideo() videotable.VideoData {
	return videotable.VideoData{Video: r.Latest, LastPositionSecs: r.LatestPositionSecs}
}

type chsLoadedMsg struct {
	chans  []domain.Channel
	latest map[string]domain.Video
}
type chVideosCachedMsg struct {
	channelID string
	videos    []domain.Video
}
type chVideosFetchedMsg struct {
	channelID string
	videos    []domain.Video
}

type Channels struct {
	backend            api.Backend
	keys               keymap.KeyMap
	circular           bool
	channelLatestCount int

	width, height int

	subs      channels.ChannelSet
	chLatest  map[string]domain.Video
	sortedChs []domain.Channel // cached sorted slice, rebuilt on mutation
	loading   bool
	sortMode  int
	spinner   spinner.Model

	aux videotable.AuxData

	pane          int
	chVideos      []domain.Video
	chVidsLoading bool
	chVidsRefresh bool
	activeChID    string
	activeChURL   string

	sortChordActive bool

	editMode  int
	editInput textinput.Model

	// channel list table — uses TableNav
	chNav  videotable.TableNav
	chCols []videotable.ColumnDef[ChannelRow]

	// video-list table — uses TableNav
	chVidNav  videotable.TableNav
	chVidCols []videotable.ColumnDef[videotable.VideoData]
}

func NewChannels(backend api.Backend, keys keymap.KeyMap, circular bool, channelLatestCount int) Channels {
	chCols := []videotable.ColumnDef[ChannelRow]{
		videotable.NumCol[ChannelRow](),
		videotable.BlankIndicatorCol[ChannelRow](),
		videotable.ChNameCol[ChannelRow](),
		videotable.ChTagsCol[ChannelRow](),
		videotable.SubsCol[ChannelRow](),
		videotable.TitleFlexCol[ChannelRow](),
		videotable.ChLatestDurationCol[ChannelRow](),
		videotable.ChLatestViewsCol[ChannelRow](),
		videotable.ChLatestDateCol[ChannelRow](),
	}
	chVidCols := []videotable.ColumnDef[videotable.VideoData]{
		videotable.NumCol[videotable.VideoData](), videotable.IndicatorCol[videotable.VideoData](), videotable.TitleFlexCol[videotable.VideoData](),
		videotable.DurationCol[videotable.VideoData](), videotable.ViewsCol[videotable.VideoData](), videotable.DateCol[videotable.VideoData](),
	}
	return Channels{
		backend:            backend,
		keys:               keys,
		circular:           circular,
		channelLatestCount: channelLatestCount,
		sortMode:           chSortDate,
		spinner:            spinner.New(),
		editInput:          textinput.New(),
		chNav:              videotable.NewTableNav(videotable.NewTable(chCols), circular, 2),
		chVidNav:           videotable.NewTableNav(videotable.NewVideoTable(chVidCols), circular, 4),
		chCols:             chCols,
		chVidCols:          chVidCols,
	}
}

func (t Channels) ID() tuipkg.TabID { return tuipkg.TabChannels }
func (t Channels) Title() string    { return "Channels" }
func (t Channels) ShortHelp() []key.Binding {
	return []key.Binding{t.keys.DrillDown, t.keys.RenameChannel, t.keys.TagChannel, t.keys.Unsubscribe, t.keys.SortChord}
}
func (t Channels) InterceptsInput() bool { return t.editInput.Focused() }

func (t Channels) Init() tea.Cmd {
	t.loading = true
	return tea.Batch(t.chsLoadCmd(), videotable.LoadAuxDataCmd(t.backend), t.spinner.Tick)
}

// rebuildSorted recomputes and caches the sorted channel slice.
// Call whenever subs, chLatest, or sortMode changes.
func (t *Channels) rebuildSorted() {
	t.sortedChs = t.sortChannelSlice(t.subs.Channels())
}

func (t Channels) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.chNav.Resize(m.Width, m.Height)
		t.chNav.SetRows(t.toChannelRows(t.sortedChs))
		t.chVidNav.Resize(m.Width, m.Height-2)
		t.chVidNav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.chVideos, t.aux), t.chVidCols))

	case spinner.TickMsg:
		if t.loading || t.chVidsLoading || t.chVidsRefresh {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(m)
			return t, cmd
		}

	case chsLoadedMsg:
		t.subs = channels.New(m.chans)
		t.chLatest = m.latest
		t.loading = false
		t.rebuildSorted()
		t.chNav.SetRows(t.toChannelRows(t.sortedChs))

	case videotable.AuxDataMsg:
		t.aux = m
		t.chVidNav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.chVideos, t.aux), t.chVidCols))

	case tuipkg.RefreshPositionsMsg:
		return t, videotable.LoadAuxDataCmd(t.backend)

	case chVideosCachedMsg:
		if m.channelID == t.activeChID {
			t.chVideos = m.videos
			t.chVidsLoading = false
			t.chVidsRefresh = true
			t.chVidNav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.chVideos, t.aux), t.chVidCols))
			return t, t.chVideosFetchCmd()
		}

	case chVideosFetchedMsg:
		if m.channelID == t.activeChID {
			t.chVideos = m.videos
			t.chVidsLoading = false
			t.chVidsRefresh = false
			t.chVidNav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.chVideos, t.aux), t.chVidCols))
		}

	case tea.KeyPressMsg:
		if t.editMode != chEditNone {
			return t.handleEditInput(m)
		}
		return t.handleKey(m)
	}
	return t, nil
}

func (t Channels) View() tea.View {
	headerText := "Channels"
	if t.loading {
		headerText += "  " + styles.Dim.Render(t.spinner.View()+" loading…")
	}
	header := styles.SectionTitle.Render(headerText)
	headerH := lipgloss.Height(header)
	contentH := t.height - headerH
	return tea.NewView(t.viewContent(header, contentH))
}

func (t Channels) viewContent(header string, _ int) string {
	if t.pane == 0 {
		var body string
		switch {
		case t.loading && t.subs.Len() == 0:
			body = t.spinner.View() + " Loading channels…"
		case t.subs.Len() == 0:
			body = styles.Dim.Render("No channels found.")
		default:
			body = t.chNav.View()
		}
		if t.editMode != chEditNone {
			body = t.appendEditInput(body)
		}
		parts := []string{header, body}
		if s := t.chNav.NumBufView(); s != "" {
			parts = append(parts, s)
		}
		return lipgloss.JoinVertical(lipgloss.Left, parts...)
	}

	chName := ""
	idx := t.chNav.Index()
	if idx < len(t.sortedChs) {
		chName = t.sortedChs[idx].DisplayName()
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
		body = t.chVidNav.View()
	}
	parts := []string{header, subHeader, body}
	if s := t.chVidNav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (t Channels) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys

	if t.sortChordActive {
		t.sortChordActive = false
		sk := keys.Sort
		changed := true
		switch {
		case key.Matches(msg, sk.Date):
			t.sortMode = chSortDate
		case key.Matches(msg, sk.Views):
			t.sortMode = chSortViews
		case key.Matches(msg, sk.Name):
			t.sortMode = chSortName
		case key.Matches(msg, sk.Duration):
			t.sortMode = chSortDuration
		case key.Matches(msg, sk.Subscribers):
			t.sortMode = chSortSubs
		case key.Matches(msg, sk.Tags):
			t.sortMode = chSortTags
		default:
			changed = false
		}
		if changed {
			t.rebuildSorted()
			t.chNav.SetRows(t.toChannelRows(t.sortedChs))
		}
		return t, nil
	}

	return t.handleKeyFlat(msg)
}

func (t Channels) handleKeyFlat(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys

	if t.pane == 0 {
		n := len(t.sortedChs)

		if t.chNav.HandleNav(msg, keys, n) {
			return t, nil
		}

		idx := t.chNav.Index()
		switch {
		case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
			if idx < n {
				ch := t.sortedChs[idx]
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
				t.chVidNav.SetRows(nil)
				t.chVidNav.GotoRow(0)
				return t, tea.Batch(t.chDrilldownCmd(ch), t.spinner.Tick)
			}
		case key.Matches(msg, keys.RenameChannel):
			if idx < n {
				ch := t.sortedChs[idx]
				t.editInput.SetValue(ch.DisplayName())
				t.editInput.Placeholder = "alias (empty to clear)…"
				t.editInput.Focus()
				t.editMode = chEditAlias
				return t, textinput.Blink
			}
		case key.Matches(msg, keys.TagChannel):
			if idx < n {
				ch := t.sortedChs[idx]
				t.editInput.SetValue(strings.Join(ch.Tags, ", "))
				t.editInput.Placeholder = "comma-separated tags…"
				t.editInput.Focus()
				t.editMode = chEditTags
				return t, textinput.Blink
			}
		case key.Matches(msg, keys.Unsubscribe):
			if idx < n {
				ch := t.sortedChs[idx]
				t.subs.Remove(ch)
				t.rebuildSorted()
				t.chNav.SetRows(t.toChannelRows(t.sortedChs))
				return t, func() tea.Msg { return tuipkg.UnsubscribeMsg{Channel: ch} }
			}
		case key.Matches(msg, keys.SortChord):
			t.sortChordActive = true
		}
		return t, nil
	}

	// pane 1: channel video list
	n := len(t.chVideos)
	numBufBefore := t.chVidNav.NumBufView() != ""
	if t.chVidNav.HandleNav(msg, keys, n) {
		return t, nil
	}

	idx := t.chVidNav.Index()
	switch {
	case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
		if numBufBefore {
			return t, nil
		}
		t.pane = 0
	case key.Matches(msg, keys.GotoBottom):
		if n > 0 {
			t.chVidNav.GotoRow(n - 1)
		}
	case key.Matches(msg, keys.Unsubscribe):
		chIdx := t.chNav.Index()
		if chIdx < len(t.sortedChs) {
			ch := t.sortedChs[chIdx]
			t.subs.Remove(ch)
			t.pane = 0
			t.rebuildSorted()
			t.chNav.SetRows(t.toChannelRows(t.sortedChs))
			return t, func() tea.Msg { return tuipkg.UnsubscribeMsg{Channel: ch} }
		}
	default:
		if v, ok := t.chVidAt(idx); ok {
			if cmd, ok := HandleVideoAction(msg, v, keys); ok {
				return t, cmd
			}
		}
	}
	return t, nil
}

func (t Channels) handleEditInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	switch {
	case key.Matches(msg, keys.Escape):
		t.editMode = chEditNone
		t.editInput.Blur()
		return t, nil
	case key.Matches(msg, keys.DrillDown):
		val := strings.TrimSpace(t.editInput.Value())
		idx := t.chNav.Index()
		if idx < len(t.sortedChs) {
			ch := t.sortedChs[idx]
			if t.editMode == chEditAlias {
				t.subs.SetAlias(ch.ID, val)
				t.rebuildSorted()
				t.chNav.SetRows(t.toChannelRows(t.sortedChs))
				t.editMode = chEditNone
				t.editInput.Blur()
				if val == "" {
					return t, t.chSetAliasCmd(ch.ID, val, "Alias cleared")
				}
				return t, t.chSetAliasCmd(ch.ID, val, "Alias set: "+val)
			}
			tags := parseTags(val)
			t.subs.SetTags(ch.ID, tags)
			t.rebuildSorted()
			t.chNav.SetRows(t.toChannelRows(t.sortedChs))
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

func (t Channels) chVidAt(idx int) (domain.Video, bool) {
	if idx >= 0 && idx < len(t.chVideos) {
		return t.chVideos[idx], true
	}
	return domain.Video{}, false
}

func (t Channels) toChannelRows(sorted []domain.Channel) []etable.Row {
	rows := make([]ChannelRow, len(sorted))
	for i, ch := range sorted {
		latest := t.chLatest[ch.ID]
		rows[i] = ChannelRow{
			Channel:            ch,
			Latest:             latest,
			LatestPositionSecs: int(t.aux.Positions[latest.ID] / 1000),
		}
	}
	return videotable.BuildRows(rows, t.chCols)
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
		return chsLoadedMsg{chans: chans, latest: latest}
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
	return func() tea.Msg {
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
