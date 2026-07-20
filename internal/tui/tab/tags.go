package tab

import (
	"context"
	"fmt"
	"sort"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/channels"
	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	etable "github.com/evertras/bubble-table/table"
)

// Column key constants for tagTable rows.
const (
	colKeyTagNum   = "tagnum"
	colKeyTagInd   = "tagind"
	colKeyTagLabel = "taglabel"
)

// TagRow is the cell input type for the tag list table.
type TagRow struct {
	Tag   string
	Count int
}

type tagsDataMsg struct {
	chans     []domain.Channel
	subVideos []domain.Video
}

func tagListColumns() []videotable.ColumnDef[TagRow] {
	return []videotable.ColumnDef[TagRow]{
		{
			Col:  etable.NewColumn(colKeyTagNum, ralign("#", render.ColNum), render.ColNum),
			Cell: func(r TagRow, i int) any { return fmt.Sprintf("%4d", i+1) },
		},
		{
			Col:  etable.NewColumn(colKeyTagInd, " ", colIndicator),
			Cell: func(r TagRow, _ int) any { return "  " },
		},
		{
			Col: etable.NewFlexColumn(colKeyTagLabel, "Tag", 1),
			Cell: func(r TagRow, _ int) any {
				return fmt.Sprintf("%s (%d)", tagDisplayName(r.Tag), r.Count)
			},
		},
	}
}

func tagDisplayName(tag string) string { return tag }

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

	pane          int
	tagSel        string
	gotoTopActive bool
	numBuf        string

	tagTable   etable.Model
	tagVidNav  videotable.TableNav
	tagCols    []videotable.ColumnDef[TagRow]
	tagVidCols []videotable.VideoColumnDef
}

func NewTags(backend api.Backend, keys keymap.KeyMap, circular bool) Tags {
	tagCols := tagListColumns()
	tagVidCols := []videotable.VideoColumnDef{
		videotable.Num, videotable.Indicator, videotable.Title,
		videotable.Channel, videotable.DurationCol(), videotable.Views, videotable.Date,
	}
	return Tags{
		backend:    backend,
		keys:       keys,
		circular:   circular,
		spinner:    spinner.New(),
		tagTable:   videotable.NewTable(tagCols),
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
		t.tagTable = t.tagTable.WithTargetWidth(m.Width).WithTargetHeight(m.Height - 2)
		t.tagTable = t.tagTable.WithRows(t.toTagRows())
		t.tagVidNav.Resize(m.Width, m.Height-2)
		t.tagVidNav.SetRows(videotable.BuildVideoRows(t.tagVideosFor(t.tagSel), t.tagVidCols, t.aux.RenderCtx(nil)))

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
		t.tagTable = t.tagTable.WithRows(t.toTagRows())

	case videotable.AuxDataMsg:
		t.aux = m
		t.tagVidNav.SetRows(videotable.BuildVideoRows(t.tagVideosFor(t.tagSel), t.tagVidCols, t.aux.RenderCtx(nil)))

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
	headerH := lipgloss.Height(header)
	contentH := t.height - headerH

	if t.pane == 1 {
		tagHeader := styles.SectionTitle.Render("← " + tagDisplayName(t.tagSel))
		parts := []string{header, tagHeader, t.tagVidNav.View()}
		if s := t.tagVidNav.NumBufView(); s != "" {
			parts = append(parts, s)
		}
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
	}

	_ = contentH
	var body string
	if t.loading && t.subs.Len() == 0 {
		body = t.spinner.View() + " Loading tags…"
	} else {
		body = t.tagTable.View()
	}
	parts := []string{header, body}
	if s := gotoLineView(t.numBuf); s != "" {
		parts = append(parts, s)
	}
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

func (t Tags) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys

	if consumed, doTop := handleGotoPrefix(&t.gotoTopActive, t.keys, msg); consumed {
		if doTop {
			t.numBuf = ""
			if t.pane == 1 {
				t.tagVidNav.GotoRow(0)
			} else {
				t.tagTable = t.tagTable.WithHighlightedRow(0)
			}
		}
		return t, nil
	}

	if checkGotoNum(&t.numBuf, msg) {
		return t, nil
	}
	numBuf := t.numBuf
	t.numBuf = ""

	if key.Matches(msg, keys.GotoLine) {
		if t.pane == 1 {
			n := len(t.tagVideosFor(t.tagSel))
			if numBuf != "" {
				if row := gotoRowIndex(numBuf); row >= 0 {
					t.tagVidNav.GotoRow(row)
				}
			} else if n > 0 {
				t.tagVidNav.GotoRow(n - 1)
			}
		} else {
			n := len(allTagsFrom(t.subs))
			if numBuf != "" {
				if row := gotoRowIndex(numBuf); row >= 0 {
					t.tagTable = t.tagTable.WithHighlightedRow(row)
				}
			} else if n > 0 {
				t.tagTable = t.tagTable.WithHighlightedRow(n - 1)
			}
		}
		return t, nil
	}

	if t.pane == 0 {
		return t.handleKeyList(msg, numBuf)
	}
	return t.handleKeyVids(msg, numBuf)
}

func (t Tags) handleKeyList(msg tea.KeyPressMsg, numBuf string) (tea.Model, tea.Cmd) {
	keys := t.keys
	items := allTagsFrom(t.subs)
	n := len(items)
	idx := t.tagTable.GetHighlightedRowIndex()
	switch {
	case key.Matches(msg, keys.Up):
		if idx > 0 {
			t.tagTable = t.tagTable.WithHighlightedRow(idx - 1)
		} else if t.circular && n > 0 {
			t.tagTable = t.tagTable.WithHighlightedRow(n - 1)
		}
	case key.Matches(msg, keys.Down):
		if idx < n-1 {
			t.tagTable = t.tagTable.WithHighlightedRow(idx + 1)
		} else if t.circular && n > 0 {
			t.tagTable = t.tagTable.WithHighlightedRow(0)
		}
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		if idx < n {
			t.tagSel = items[idx]
			vids := t.tagVideosFor(t.tagSel)
			t.tagVidNav.SetRows(videotable.BuildVideoRows(vids, t.tagVidCols, t.aux.RenderCtx(nil)))
			t.tagVidNav.GotoRow(0)
			t.pane = 1
		}
	}
	_ = numBuf
	return t, nil
}

func (t Tags) handleKeyVids(msg tea.KeyPressMsg, numBuf string) (tea.Model, tea.Cmd) {
	keys := t.keys
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
	_ = numBuf
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
