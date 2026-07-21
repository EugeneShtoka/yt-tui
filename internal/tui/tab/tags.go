package tab

import (
	"context"
	"fmt"
	"sort"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/channels"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
	etable "github.com/evertras/bubble-table/table"
)

// TagRow is the cell input type for the tag list table.
type TagRow struct {
	Tag   string
	Count int
}

func (r TagRow) GetTitle() string { return fmt.Sprintf("%s (%d)", r.Tag, r.Count) }

type tagsDataMsg struct {
	chans     []domain.Channel
	subVideos []domain.Video
}

func allTagsFrom(subs channels.ChannelSet) []string {
	seen := map[string]bool{}
	for _, ch := range subs.Channels() {
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

func channelsInTagFrom(subs channels.ChannelSet, tag string) []domain.Channel {
	var out []domain.Channel
	for _, ch := range subs.Channels() {
		for _, tg := range ch.Tags {
			if tg == tag {
				out = append(out, ch)
				break
			}
		}
	}
	return out
}

func tagVideosFrom(subs channels.ChannelSet, subVideos []domain.Video, tagSel string) []domain.Video {
	chans := channelsInTagFrom(subs, tagSel)
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
	for _, v := range subVideos {
		if idSet[v.ChannelID] {
			out = append(out, v)
		}
	}
	feed.SortVideos(out, feed.SortDate)
	return out
}

type Tags struct {
	backend  api.Backend
	keys     keymap.KeyMap
	circular bool

	width, height int

	subs      channels.ChannelSet
	subVideos []domain.Video
	loading   bool
	spinner   spinner.Model

	aux videotable.AuxData

	pane   int
	tagSel string

	// tag list table — uses TableNav
	tagNav  videotable.TableNav
	tagCols []videotable.ColumnDef[TagRow]

	// tag video table — uses TableNav
	tagVidNav  videotable.TableNav
	tagVidCols []videotable.ColumnDef[videotable.VideoData]
}

func NewTags(backend api.Backend, keys keymap.KeyMap, circular bool) Tags {
	tagCols := []videotable.ColumnDef[TagRow]{
		videotable.NumCol[TagRow](),
		videotable.BlankIndicatorCol[TagRow](),
		videotable.TitleFlexCol[TagRow](),
	}
	tagVidCols := []videotable.ColumnDef[videotable.VideoData]{
		videotable.NumCol[videotable.VideoData](), videotable.IndicatorCol[videotable.VideoData](), videotable.TitleFlexCol[videotable.VideoData](),
		videotable.ChannelCol[videotable.VideoData](), videotable.DurationCol[videotable.VideoData](), videotable.ViewsCol[videotable.VideoData](), videotable.DateCol[videotable.VideoData](),
	}
	return Tags{
		backend:    backend,
		keys:       keys,
		circular:   circular,
		spinner:    spinner.New(),
		tagNav:     videotable.NewTableNav(videotable.NewTable(tagCols), circular, 2),
		tagVidNav:  videotable.NewTableNav(videotable.NewVideoTable(tagVidCols), circular, 4),
		tagCols:    tagCols,
		tagVidCols: tagVidCols,
	}
}

func (t Tags) ID() tuipkg.TabID         { return tuipkg.TabTags }
func (t Tags) Title() string            { return "Tags" }
func (t Tags) InterceptsInput() bool    { return false }
func (t Tags) ShortHelp() []key.Binding { return []key.Binding{t.keys.DrillDown} }

func (t Tags) Init() tea.Cmd {
	t.loading = true
	return tea.Batch(t.tagsDataLoadCmd(), videotable.LoadAuxDataCmd(t.backend), t.spinner.Tick)
}

func (t Tags) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.tagNav.Resize(m.Width, m.Height)
		t.tagNav.SetRows(t.toTagRows())
		t.tagVidNav.Resize(m.Width, m.Height-2)
		t.tagVidNav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.tagVideosFor(t.tagSel), t.aux), t.tagVidCols))

	case spinner.TickMsg:
		if t.loading {
			var cmd tea.Cmd
			t.spinner, cmd = t.spinner.Update(m)
			return t, cmd
		}

	case tagsDataMsg:
		t.subs = channels.New(m.chans)
		t.subVideos = m.subVideos
		t.loading = false
		t.tagNav.SetRows(t.toTagRows())

	case videotable.AuxDataMsg:
		t.aux = m
		t.tagVidNav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(t.tagVideosFor(t.tagSel), t.aux), t.tagVidCols))

	case tuipkg.RefreshPositionsMsg:
		return t, videotable.LoadAuxDataCmd(t.backend)

	case tea.KeyPressMsg:
		return t.handleKey(m)
	}
	return t, nil
}

func (t Tags) View() tea.View {
	headerText := "Tags"
	if t.loading {
		headerText += "  " + styles.Dim.Render(t.spinner.View()+" loading…")
	}
	header := styles.SectionTitle.Render(headerText)

	if t.pane == 1 {
		tagHeader := styles.SectionTitle.Render("← " + t.tagSel)
		parts := []string{header, tagHeader, t.tagVidNav.View()}
		if s := t.tagVidNav.NumBufView(); s != "" {
			parts = append(parts, s)
		}
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
	}

	var body string
	if t.loading && t.subs.Len() == 0 {
		body = t.spinner.View() + " Loading tags…"
	} else {
		body = t.tagNav.View()
	}
	parts := []string{header, body}
	if s := t.tagNav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

func (t Tags) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys

	if t.pane == 0 {
		items := allTagsFrom(t.subs)
		n := len(items)

		if t.tagNav.HandleNav(msg, keys, n) {
			return t, nil
		}

		idx := t.tagNav.Index()
		switch {
		case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
			if idx < n {
				t.tagSel = items[idx]
				vids := t.tagVideosFor(t.tagSel)
				t.tagVidNav.SetRows(videotable.BuildVideoRows(videotable.EnrichAll(vids, t.aux), t.tagVidCols))
				t.tagVidNav.GotoRow(0)
				t.pane = 1
			}
		}
		return t, nil
	}

	// pane 1: tag video list
	vids := t.tagVideosFor(t.tagSel)
	n := len(vids)
	numBufBefore := t.tagVidNav.NumBufView() != ""
	if t.tagVidNav.HandleNav(msg, keys, n) {
		return t, nil
	}

	idx := t.tagVidNav.Index()
	switch {
	case key.Matches(msg, keys.Left), key.Matches(msg, keys.Escape):
		if numBufBefore {
			return t, nil
		}
		t.pane = 0
		t.tagVidNav.GotoRow(0)
	case key.Matches(msg, keys.GotoBottom):
		if n > 0 {
			t.tagVidNav.GotoRow(n - 1)
		}
	default:
		if idx < n {
			if cmd, ok := HandleVideoAction(msg, vids[idx], keys); ok {
				return t, cmd
			}
		}
	}
	return t, nil
}

func (t Tags) tagVideosFor(tagSel string) []domain.Video {
	return tagVideosFrom(t.subs, t.subVideos, tagSel)
}

func (t Tags) toTagRows() []etable.Row {
	items := allTagsFrom(t.subs)
	rows := make([]TagRow, len(items))
	for i, tag := range items {
		rows[i] = TagRow{Tag: tag, Count: len(channelsInTagFrom(t.subs, tag))}
	}
	return videotable.BuildRows(rows, t.tagCols)
}

func (t Tags) tagsDataLoadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		chans, err := t.backend.GetSubscribedChannels(ctx)
		if err != nil {
			return tuipkg.StatusMsg{Text: "tags: " + err.Error(), IsErr: true}
		}
		ids := make([]string, len(chans))
		for i, ch := range chans {
			ids[i] = ch.ID
		}
		subVideos, _ := t.backend.GetAllChannelVideos(ctx, ids)
		feed.SortVideos(subVideos, feed.SortDate)
		return tagsDataMsg{chans: chans, subVideos: subVideos}
	}
}
