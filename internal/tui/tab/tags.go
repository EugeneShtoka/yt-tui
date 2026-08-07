package tab

import (
	"context"
	"fmt"
	"sort"
	"time"

	"charm.land/bubbles/v2/key"
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
	Tag                string
	Count              int
	Latest             domain.Video
	LatestPositionSecs int
}

func (r TagRow) GetTitle() string { return r.Latest.Title }
func (r TagRow) GetLatestVideo() videotable.VideoData {
	return videotable.VideoData{Video: r.Latest, LastPositionSecs: r.LatestPositionSecs}
}

type tagsDataMsg struct {
	tuipkg.TabTarget
	chans     []domain.Channel // full channel universe (subscribed + annotated + blocked)
	subVideos []domain.Video   // subscribed-channel videos
	recVideos []domain.Video   // recommended-feed cache (folds rec channels into the universe)
}

// allTags returns the sorted, deduplicated tag set across the given channels.
func allTags(chs []domain.Channel) []string {
	seen := map[string]bool{}
	for i := range chs {
		for _, tg := range chs[i].Tags {
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

// channelsInTag returns the channels (from the given slice) carrying tag.
func channelsInTag(chs []domain.Channel, tag string) []domain.Channel {
	var out []domain.Channel
	for i := range chs {
		for _, tg := range chs[i].Tags {
			if tg == tag {
				out = append(out, chs[i])
				break
			}
		}
	}
	return out
}

// tagVideos returns the videos (from pool) belonging to channels in chs that
// carry tagSel, newest first.
func tagVideos(chs []domain.Channel, pool []domain.Video, tagSel string) []domain.Video {
	inTag := channelsInTag(chs, tagSel)
	if len(inTag) == 0 {
		return nil
	}
	idSet := make(map[string]bool, len(inTag))
	for i := range inTag {
		if inTag[i].ID != "" {
			idSet[inTag[i].ID] = true
		}
	}
	var out []domain.Video
	for _, v := range pool {
		if idSet[v.ChannelID] {
			out = append(out, v)
		}
	}
	feed.SortVideos(out, feed.SortDate)
	return out
}

// tagsBackend is the narrow slice of the backend the Tags tab needs: channel
// listing, feed cache, and the shared aux data. Declared consumer-side (ISP).
type tagsBackend interface {
	api.ChannelBackend
	api.FeedBackend
	videotable.AuxBackend
}

type Tags struct {
	ctx      context.Context
	backend  tagsBackend
	keys     keymap.KeyMap
	circular bool

	width, height int

	subs         channels.ChannelSet // full channel universe (subscribed + rec-feed + annotated)
	subVideos    []domain.Video      // combined video pool (subscribed ∪ recommended)
	recFeedIDs   map[string]bool     // channel IDs currently in the recommended feed
	loading      bool
	spinnerFrame string

	mode   srcMode     // active source filter (recommended / subscribed / mixed / stale)
	stale  staleFilter // stale-tagged-channel partition config (hide + threshold)
	picker modePicker  // the mode picker (shared inline selector)

	aux videotable.AuxData

	pane          int
	tagSel        string
	sortedTagRows []TagRow // cached rows, rebuilt on mutation

	// tag list table — uses TableNav
	tagNav  videotable.TableNav
	tagCols []videotable.ColumnDef[TagRow]

	// tag video table — uses TableNav
	tagVidNav  videotable.TableNav
	tagVidCols []videotable.ColumnDef[videotable.VideoData]
}

// tagColumns is the full, natural-order column set for the Tags list. Extracted
// so the per-panel column selector and the tab.PanelColumnKeys catalog share one
// source of truth.
func tagColumns() []videotable.ColumnDef[TagRow] {
	return []videotable.ColumnDef[TagRow]{
		videotable.NumCol[TagRow](),
		videotable.BlankIndicatorCol[TagRow](),
		{
			Col:  etable.NewColumn(videotable.KeyTagLabel, "Tag", videotable.ColChName),
			Cell: func(item TagRow, _ int) any { return fmt.Sprintf("%s (%d)", item.Tag, item.Count) },
		},
		videotable.TitleFlexCol[TagRow](),
		videotable.ChLatestWatchedCol[TagRow](), videotable.ChLatestDurationCol[TagRow](),
		videotable.ChLatestViewsCol[TagRow](),
		videotable.ChLatestDateCol[TagRow](),
	}
}

// TagsOpts carries the Tags-tab configuration. Grouping the bool/int stale pair
// behind named fields prevents silent transposition at the call site.
type TagsOpts struct {
	Mode      string // default source-filter mode
	HideStale bool   // hide stale-tagged channels
	StaleDays int    // age threshold for "stale"
}

func NewTags(ctx context.Context, backend tagsBackend, keys keymap.KeyMap, circular bool, opts TagsOpts, wantCols ...string) Tags {
	mode := parseSrcMode(opts.Mode)
	if mode == srcBlocked { // Tags has no blocked mode; fall back to subscribed.
		mode = srcSubscribed
	}
	tagCols := videotable.SelectColumns(tagColumns(), wantCols)
	tagVidCols := []videotable.ColumnDef[videotable.VideoData]{
		videotable.NumCol[videotable.VideoData](), videotable.IndicatorCol[videotable.VideoData](), videotable.TitleFlexCol[videotable.VideoData](),
		videotable.ChannelCol[videotable.VideoData](), videotable.WatchedCol[videotable.VideoData](), videotable.DurationCol[videotable.VideoData](), videotable.ViewsCol[videotable.VideoData](), videotable.DateCol[videotable.VideoData](),
	}
	return Tags{
		ctx:        ctx,
		backend:    backend,
		keys:       keys,
		circular:   circular,
		mode:       mode,
		recFeedIDs: map[string]bool{},
		stale:      staleFilter{hide: opts.HideStale, days: opts.StaleDays},
		picker:     newModePicker("Mode", modeLabels(tagModes), circular),
		tagNav:     videotable.NewTableNav(tagCols, circular, 2),
		tagVidNav:  videotable.NewTableNav(tagVidCols, circular, 4),
		tagCols:    tagCols,
		tagVidCols: tagVidCols,
	}
}

func (t Tags) ID() tuipkg.TabID      { return tuipkg.TabTags }
func (t Tags) Title() string         { return "Tags" }
func (t Tags) InterceptsInput() bool { return t.picker.isOpen() }
func (t Tags) Loading() bool         { return t.loading }

// modeChannels returns the channels in the current mode's partition (the set the
// tag list is computed from).
func (t Tags) modeChannels() []domain.Channel {
	return selectChannels(t.subs.Channels(), t.mode, t.recFeedIDs, t.stale, time.Now())
}
func (t Tags) SelectedVideo() (domain.Video, bool) {
	if t.pane == 1 {
		vids := t.tagVideosFor(t.tagSel)
		idx := t.tagVidNav.Index()
		if idx >= 0 && idx < len(vids) {
			return vids[idx], true
		}
		return domain.Video{}, false
	}
	idx := t.tagNav.Index()
	if idx < len(t.sortedTagRows) {
		if v := t.sortedTagRows[idx].Latest; v.ID != "" {
			return v, true
		}
	}
	return domain.Video{}, false
}
func (t Tags) ShortHelp() []key.Binding {
	return []key.Binding{t.keys.DrillDown, t.keys.PanelMode}
}

func (t Tags) Init() tea.Cmd {
	t.loading = true
	return tea.Batch(t.tagsDataLoadCmd(), videotable.LoadAuxDataCmd(t.ctx, t.backend))
}

func (t Tags) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.ContentSizeMsg:
		t.width, t.height = m.Width, m.Height
		t.tagNav.Resize(m.Width, m.Height)
		t.rebuildTagRows()
		t.tagVidNav.Resize(m.Width, m.Height)
		videotable.SetVideoRows(&t.tagVidNav, t.tagVideosFor(t.tagSel), t.aux, t.tagVidCols)

	case tuipkg.SpinnerFrameMsg:
		t.spinnerFrame = m.Frame

	case tuipkg.PollTickMsg:
		// Reload tag/channel data so videos a background crawl streams in appear
		// under their tags without a manual refresh.
		return t, t.tagsDataLoadCmd()

	case tagsDataMsg:
		// Fold recommended-feed channels into the universe (state=none, in memory
		// only) so the recommended/mixed modes list their tags, and pool the
		// recommended videos in for tag drill-downs.
		t.recFeedIDs = channels.RecFeedIDs(m.recVideos)
		t.subs = channels.New(mergeUniverse(m.chans, m.recVideos))
		t.subVideos = feed.MergeVideos(m.subVideos, m.recVideos)
		t.loading = false
		t.rebuildTagRows()

	case videotable.AuxDataMsg:
		t.aux = m
		t.rebuildTagRows()
		videotable.SetVideoRows(&t.tagVidNav, t.tagVideosFor(t.tagSel), t.aux, t.tagVidCols)

	case tea.KeyPressMsg:
		return t.handleKey(m)
	}
	return t, nil
}

func (t Tags) View() tea.View {
	headerText := "Tags" + styles.Dim.Render(" · "+t.mode.label())
	if t.loading {
		headerText += "  " + styles.Dim.Render(t.spinnerFrame+" loading…")
	}
	header := styles.SectionTitle.Render(headerText)

	if t.pane == 1 {
		tagHeader := drillSubHeader(t.tagSel, t.width, "")
		parts := []string{header, tagHeader, t.tagVidNav.View()}
		if s := t.tagVidNav.NumBufView(); s != "" {
			parts = append(parts, s)
		}
		return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
	}

	var body string
	switch {
	case t.loading && t.subs.Len() == 0:
		body = t.spinnerFrame + " Loading tags…"
	case len(t.sortedTagRows) == 0:
		body = styles.Dim.Render(t.emptyTagsText())
	default:
		body = t.tagNav.View()
	}
	// The mode picker can be opened over any state (loading, empty, or list).
	if t.picker.isOpen() {
		body = t.picker.view(body, t.keys.Escape.Help().Key, t.width)
	}
	parts := []string{header, body}
	if s := t.tagNav.NumBufView(); s != "" {
		parts = append(parts, s)
	}
	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

// emptyTagsText is the "nothing here" message tailored to the active mode.
func (t Tags) emptyTagsText() string {
	switch t.mode {
	case srcRecommended:
		return "No tags on recommended channels yet. Tag rec-feed channels from the Channels tab."
	case srcMixed:
		return "No tagged channels."
	case srcStale:
		return "No stale tagged channels."
	default:
		return "No tags on subscribed channels."
	}
}

func (t Tags) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if t.picker.isOpen() {
		return t.handlePickerKey(msg)
	}
	if t.pane == 0 {
		return t.handleKeyList(msg)
	}
	return t.handleKeyVideos(msg)
}

// handleKeyList routes keys for pane 0 (the tag list).
func (t Tags) handleKeyList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	n := len(t.sortedTagRows)

	if t.tagNav.HandleNav(msg, keys, n) {
		return t, nil
	}

	idx := t.tagNav.Index()
	switch {
	case key.Matches(msg, keys.PanelMode):
		t.picker.openAt(modeIndex(tagModes, t.mode))
		return t, nil
	case key.Matches(msg, keys.DrillDown), key.Matches(msg, keys.Right):
		if idx < n {
			t.tagSel = t.sortedTagRows[idx].Tag
			vids := t.tagVideosFor(t.tagSel)
			videotable.SetVideoRows(&t.tagVidNav, vids, t.aux, t.tagVidCols)
			t.tagVidNav.GotoRow(0)
			t.pane = 1
		}
	default:
		if idx < n {
			if v := t.sortedTagRows[idx].Latest; v.ID != "" {
				if cmd, ok := HandleVideoAction(msg, v, keys); ok {
					return t, cmd
				}
			}
		}
	}
	return t, nil
}

// handleKeyVideos routes keys for pane 1 (the tag's video list).
func (t Tags) handleKeyVideos(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := t.keys
	vids := t.tagVideosFor(t.tagSel)
	n := len(vids)
	if handled, back := handleDrillBackKey(&t.tagVidNav, msg, keys, n); handled {
		if back {
			t.pane = 0
			t.tagVidNav.GotoRow(0)
		}
		return t, nil
	}

	idx := t.tagVidNav.Index()
	switch {
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

// handlePickerKey drives the mode picker: applying the chosen mode recomputes the
// tag list from that partition and returns to the tag list from the top.
func (t Tags) handlePickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if t.picker.handleKey(msg, t.keys) == pickerCommitted {
		t.mode = tagModes[t.picker.selection()]
		t.pane = 0
		t.rebuildTagRows()
		t.tagNav.GotoRow(0)
	}
	return t, nil
}

func (t Tags) tagVideosFor(tagSel string) []domain.Video {
	return tagVideos(t.modeChannels(), t.subVideos, tagSel)
}

// rebuildTagRows recomputes and caches the sorted tag slice for the active mode.
// Call whenever subs, subVideos, the mode, or aux changes.
func (t *Tags) rebuildTagRows() {
	modeChs := t.modeChannels()
	items := allTags(modeChs)
	rows := make([]TagRow, len(items))
	for i, tag := range items {
		vids := t.tagVideosFor(tag)
		var latest domain.Video
		if len(vids) > 0 {
			latest = vids[0]
		}
		rows[i] = TagRow{
			Tag:                tag,
			Count:              len(channelsInTag(modeChs, tag)),
			Latest:             latest,
			LatestPositionSecs: int(t.aux.Positions[latest.ID] / 1000),
		}
	}
	t.sortedTagRows = rows
	t.tagNav.SetRows(videotable.BuildRows(rows, t.tagCols))
}

func (t Tags) tagsDataLoadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := t.ctx
		// Load the full channel universe so the mode picker can slice it
		// (subscribed / recommended / mixed) client-side without refetching.
		chans, err := t.backend.AllChannels(ctx)
		if err != nil {
			return tuipkg.StatusMsg{Text: "tags: " + err.Error(), IsErr: true}
		}
		ids := make([]string, 0, len(chans))
		for i := range chans {
			if chans[i].IsSubscribed() {
				ids = append(ids, chans[i].ID)
			}
		}
		subVideos, _ := t.backend.GetAllChannelVideos(ctx, ids)
		feed.SortVideos(subVideos, feed.SortDate)
		// The recommended-feed cache backs the recommended/mixed modes (tags on
		// channels we don't subscribe to). Best-effort.
		recVideos, _ := t.backend.GetFeedCache(ctx, "recommended")
		return tagsDataMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabTags}, chans: chans, subVideos: subVideos, recVideos: recVideos}
	}
}
