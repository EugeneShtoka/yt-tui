package overlay

import (
	"context"
	"image"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/media"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
)

const panelW = 52

// panelGap is the blank column spacing left between the tab content and the
// info side panel so the two don't visually touch.
const panelGap = 2

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
	vdPanel      vdSubState = iota // detail side panel
	vdLinks                        // links modal over panel
	vdChapters                     // chapters modal over panel
	vdTranscript                   // transcript modal over panel
)

// InitialView controls which sub-panel a VideoDetail overlay opens in after loading.
type InitialView int

const (
	InitialViewPanel InitialView = iota
	InitialViewLinks
	InitialViewChapters
	InitialViewTranscript
)

// ── private messages ──────────────────────────────────────────────────────────

// These async results carry OverlayTarget so Root delivers them to the exact
// VideoDetail instance that issued the fetch (see tuipkg.OverlayAddressedMsg),
// not to whatever overlay is on top. token still disambiguates fetch generations
// within that one instance (video changed / refresh); OverlayTarget disambiguates
// between instances.
type vdDetailsMsg struct {
	tuipkg.OverlayTarget
	details domain.VideoDetails
	err     error
	token   int
}

// vdCacheMsg carries the result of the local details-cache lookup that runs
// before any network fetch. On a hit the panel renders immediately from disk;
// on a miss the handler falls back to a full yt-dlp fetch.
type vdCacheMsg struct {
	tuipkg.OverlayTarget
	video   domain.Video
	details domain.CachedDetails
	ok      bool
	token   int
}

// vdTranscriptMsg carries the result of an async transcript fetch.
type vdTranscriptMsg struct {
	tuipkg.OverlayTarget
	text  string
	err   error
	token int
}

// ── VideoDetail ───────────────────────────────────────────────────────────────

// VideoDetail is the video-detail side panel with nested links/chapters modals.
type VideoDetail struct {
	identity
	ctx          context.Context
	backend      api.VideoBackend
	media        api.MediaProvider // thumbnail/transcript seam (client-side cache + egress)
	keys         keymap.KeyMap
	closeOnLinks bool // cfg.CloseOnLinkOpen

	video      *domain.VideoDetails
	fetchVideo domain.Video // the video currently being fetched / displayed
	focused    bool
	fetchToken int
	// fetchCtx/fetchCancel scope the in-flight yt-dlp fetch to the current video
	// generation: renewFetchCtx cancels the previous one when the video changes or
	// the overlay closes, so a superseded fetch is actually killed, not just
	// ignored via fetchToken (H-1 tail).
	fetchCtx     context.Context
	fetchCancel  context.CancelFunc
	loading      bool
	spinnerFrame string

	descLines []string
	descVS    int
	// links/chapters hold the extracted lists; the *Loaded flags distinguish
	// "not yet extracted" from "extracted but empty" without a pointer-to-slice
	// (which risked a nil deref on the modal key handlers). (L-3)
	links          []domain.Link
	linksLoaded    bool
	chapters       []domain.Chapter
	chaptersLoaded bool
	// transcript modal state: the full text (for copy) plus wrapped display lines
	// and a scroll offset. transcriptLoaded distinguishes "still fetching" from
	// "fetched, empty".
	transcriptText   string
	transcriptVS     int
	transcriptLoaded bool
	// transcriptLoading is true while an in-panel transcript fetch is in flight
	// (miss on first open): the panel shows a loading-spinner popup until the
	// fetch resolves into the transcript modal. The standalone transcript overlay
	// has its own initialView-driven loading box, so this is panel-only.
	transcriptLoading bool
	transcriptWidth   string // width spec for the transcript popup ("80" | "50%")

	thumb         image.Image
	thumbB64      string
	thumbRendered string

	contentW int // terminal columns left of the panel (for Kitty col position)
	contentH int // terminal content rows (viewport height for modal scroll clamps)
	kittyRow int // 1-indexed terminal row where panel interior starts

	subState    vdSubState
	initialView InitialView
	linkSel     int
	chapterSel  int
	circular    bool
}

// VideoDetailOpts carries the overlay's presentation flags. Grouping them behind
// named fields keeps the constructor readable and prevents silent transposition
// of the two adjacent bools (CloseOnLinks, Circular) at the call site.
type VideoDetailOpts struct {
	CloseOnLinks    bool        // close the overlay after opening a link
	Circular        bool        // wrap navigation at list ends
	InitialView     InitialView // which sub-panel opens after loading
	TranscriptWidth string      // transcript-modal width spec
}

// NewVideoDetail creates a VideoDetail overlay that immediately starts loading
// details for the given video. opts.InitialView controls which sub-panel opens
// after loading.
func NewVideoDetail(ctx context.Context, backend api.VideoBackend, media api.MediaProvider, keys keymap.KeyMap, v domain.Video, opts VideoDetailOpts) (VideoDetail, tea.Cmd) {
	vd := VideoDetail{
		identity:        newIdentity(),
		ctx:             ctx,
		backend:         backend,
		media:           media,
		keys:            keys,
		closeOnLinks:    opts.CloseOnLinks,
		loading:         true,
		circular:        opts.Circular,
		initialView:     opts.InitialView,
		fetchVideo:      v,
		transcriptWidth: opts.TranscriptWidth,
	}
	vd = vd.renewFetchCtx()
	// The transcript modal has its own async source (a separate backend fetch),
	// independent of the video-details cache the panel/links/chapters share.
	if opts.InitialView == InitialViewTranscript {
		return vd, vd.transcriptLoadCmd(v)
	}
	return vd, vd.cacheCmd(v)
}

// renewFetchCtx cancels the previous video generation's fetch context (killing a
// superseded yt-dlp call) and installs a fresh one derived from the app-lifetime
// ctx. Callers must use the returned VideoDetail.
func (vd VideoDetail) renewFetchCtx() VideoDetail {
	if vd.fetchCancel != nil {
		vd.fetchCancel()
	}
	base := vd.ctx
	if base == nil {
		base = context.Background()
	}
	vd.fetchCtx, vd.fetchCancel = context.WithCancel(base)
	return vd
}

// fetchContext is the context every fetch command runs under: the current
// per-generation fetchCtx, falling back to the app ctx (or Background) when a
// fetch runs before one was installed (e.g. a bare test literal).
func (vd VideoDetail) fetchContext() context.Context {
	switch {
	case vd.fetchCtx != nil:
		return vd.fetchCtx
	case vd.ctx != nil:
		return vd.ctx
	default:
		return context.Background()
	}
}

// ── overlay.Overlay interface ─────────────────────────────────────────────────

func (vd VideoDetail) InterceptsInput() bool { return false }
func (vd VideoDetail) WidthReduction() int {
	if vd.initialView != InitialViewPanel {
		return 0 // direct modal mode — no side panel, so nothing is reserved
	}
	return panelW + panelGap
}

// IsPanel reports whether this overlay is the info side panel (as opposed to a
// standalone links/chapters modal). Root uses it to toggle the panel without
// closing a modal.
func (vd VideoDetail) IsPanel() bool { return vd.initialView == InitialViewPanel }

// OpenModalMsg asks an already-open info panel to open one of its modal
// sub-views (links/chapters/transcript) in place. Root sends it instead of
// stacking a second VideoDetail overlay over the panel: two stacked panels
// would share the same untagged async fetch messages (handleBroadcast delivers
// only to the top overlay), so the panel's details result would land on the
// modal and vice-versa.
type OpenModalMsg struct{ View InitialView }

// HasFocus reports whether the overlay captures keyboard input. Standalone
// links/chapters modals always capture input; the side panel captures input
// only when explicitly focused.
func (vd VideoDetail) HasFocus() bool {
	// An open modal sub-view (links/chapters/transcript) always captures input so
	// it stays interactive even when the panel was opened unfocused (the list held
	// focus); closing the modal drops back to vdPanel and returns control.
	return vd.focused || vd.initialView != InitialViewPanel || vd.subState != vdPanel
}

// PanelFocused reports whether this is the side info panel and it currently holds
// focus. Root uses it to fade the underlying list's frame (styles.ListBorderDimmed)
// so the active panel reads as focused.
func (vd VideoDetail) PanelFocused() bool { return vd.IsPanel() && vd.focused }

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (vd VideoDetail) Init() tea.Cmd  { return nil }
func (vd VideoDetail) View() tea.View { return tea.NewView("") } // rendering done via Render(behind,...)

func (vd VideoDetail) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {

	case tuipkg.SpinnerFrameMsg:
		vd.spinnerFrame = m.Frame
		return vd, nil

	case vdDetailsMsg:
		return vd.handleDetailsMsg(m)

	case vdCacheMsg:
		return vd.handleCacheMsg(m)

	case vdTranscriptMsg:
		return vd.handleTranscriptMsg(m)

	case OpenModalMsg:
		return vd.openModal(m.View)

	case FocusSwitchMsg:
		if vd.subState == vdPanel {
			vd.focused = !vd.focused
		}
		return vd, vd.kittyAfterFrameCmd()

	case VideoClearMsg:
		return vd.handleVideoClear()

	case kittyRefreshMsg:
		return vd, vd.kittyCmd()

	case tuipkg.VideoSelectedMsg:
		// No-op for the video already shown or already being fetched. The
		// fetchVideo guard matters during the initial load: a stray same-video
		// selection (e.g. the 150ms debounce scheduled when a sub-view is opened)
		// would otherwise bump fetchToken and cancel the in-flight details/
		// transcript fetch, leaving the panel stuck loading.
		if (vd.video != nil && vd.video.ID == m.Video.ID) || vd.fetchVideo.ID == m.Video.ID {
			return vd, nil
		}
		vd.loading = true
		vd.fetchToken++
		vd.fetchVideo = m.Video
		vd = vd.resetTranscript() // the old video's transcript no longer applies
		vd = vd.renewFetchCtx()   // cancel the previous video's in-flight fetch
		return vd, vd.cacheCmd(m.Video)

	case OverlaySizeMsg:
		vd.contentW = m.ContentW
		vd.contentH = m.ContentH
		vd.kittyRow = m.KittyRow
		return vd, vd.kittyAfterFrameCmd()

	case ThumbnailLoadedMsg:
		return vd.handleThumbnailLoaded(m)

	case tea.KeyPressMsg:
		return vd.handleKey(m)
	}
	return vd, nil
}

// handleDetailsMsg applies a completed network fetch: it stores the details,
// re-wraps the description, extracts chapters, and schedules the best-effort
// cache writes plus post-load steps.
func (vd VideoDetail) handleDetailsMsg(m vdDetailsMsg) (tea.Model, tea.Cmd) {
	if m.token != vd.fetchToken {
		return vd, nil // stale fetch — video changed while fetching
	}
	firstLoad := vd.video == nil
	vd.loading = false
	if m.err != nil {
		if firstLoad {
			return vd, tea.Batch(
				func() tea.Msg { return PopOverlayMsg{} },
				func() tea.Msg { return tuipkg.StatusMsg{Text: "video details: " + m.err.Error(), IsErr: true} },
			)
		}
		return vd, func() tea.Msg { return tuipkg.StatusMsg{Text: "video details: " + m.err.Error(), IsErr: true} }
	}
	details := m.details
	var chapters []domain.Chapter
	chaptersLoaded := false
	if len(details.Chapters) > 0 {
		chapters, _ = media.ProcessChapters(details.Chapters)
		chaptersLoaded = true
	}
	// Cache writes are best-effort and non-authoritative, but they're still
	// disk/network I/O and must not run inline in Update — wrap them in commands.
	cmds := []tea.Cmd{saveDetailsCacheCmd(vd.ctx, vd.backend, details)}
	if chaptersLoaded {
		cmds = append(cmds, saveChaptersCmd(vd.ctx, vd.backend, details.ID, chapters))
	}
	// A network load carries a fresh description, so links are left unloaded to be
	// re-extracted lazily on the next openLinks().
	return vd.applyDetails(details, chapters, chaptersLoaded, nil, false, cmds)
}

// handleCacheMsg applies the local details-cache lookup: a miss falls back to a
// network fetch, a hit renders the panel immediately from disk.
func (vd VideoDetail) handleCacheMsg(m vdCacheMsg) (tea.Model, tea.Cmd) {
	if m.token != vd.fetchToken {
		return vd, nil // stale — video changed while reading the cache
	}
	if !m.ok {
		// Cache miss: fall back to a full yt-dlp fetch, which will also
		// populate the cache so the next open is instant. Keep loading=true
		// so the spinner stays up until the network fetch returns.
		return vd, vd.fetchCmd(m.video)
	}
	details := domain.VideoDetails{
		Video:        m.video,
		Description:  m.details.Description,
		ThumbnailURL: m.details.ThumbnailURL,
		Subscribers:  m.details.Subscribers,
	}
	// Cache stores already-processed (SponsorBlock-adjusted) chapters and parsed
	// links; reuse them directly. Absent links stay unloaded so openLinks extracts
	// them lazily from the description.
	var chapters []domain.Chapter
	chaptersLoaded := false
	if m.details.Chapters != nil && len(*m.details.Chapters) > 0 {
		chapters, chaptersLoaded = *m.details.Chapters, true
	}
	var links []domain.Link
	linksLoaded := false
	if m.details.Links != nil {
		links, linksLoaded = *m.details.Links, true
	}
	return vd.applyDetails(details, chapters, chaptersLoaded, links, linksLoaded, nil)
}

// applyDetails is the shared tail of the network (handleDetailsMsg) and cache
// (handleCacheMsg) load paths: it records the loaded video, re-wraps the
// description, stores the resolved chapters/links (links only when a modal isn't
// already showing them), then runs applyPostLoad and batches any extra
// (cache-write) commands the caller supplies. firstLoad and the previous
// thumbnail URL are derived from the pre-load state, so it must run before
// vd.video is otherwise mutated.
func (vd VideoDetail) applyDetails(details domain.VideoDetails, chapters []domain.Chapter, chaptersLoaded bool, links []domain.Link, linksLoaded bool, cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	firstLoad := vd.video == nil
	vd.loading = false
	prevThumbURL := ""
	if vd.video != nil {
		prevThumbURL = vd.video.ThumbnailURL
	}
	vd.video = &details
	vd.descLines = render.WordWrap(render.ShortenURLs(details.Description, panelW-2), panelW-2)
	vd.chapters, vd.chaptersLoaded = chapters, chaptersLoaded
	if vd.subState != vdLinks {
		vd.links, vd.linksLoaded = links, linksLoaded
	}
	vd, cmds = vd.applyPostLoad(firstLoad, prevThumbURL, cmds)
	return vd, tea.Batch(cmds...)
}

// handleVideoClear tears down the current video and any placed Kitty image, and
// invalidates the in-flight fetch generation. It fires on a tab switch or an
// under-panel selection change, so the previously-shown video no longer applies
// and we must:
//   - bump fetchToken + renew the fetch ctx, so a fetch still in flight for the
//     old selection is canceled and its late result is dropped instead of landing
//     on the now-wrong tab (else the panel shows the previous tab's video); and
//   - forget fetchVideo, so the upcoming debounced VideoSelectedMsg re-fetches
//     even when it names that same video — the fetchVideo guard would otherwise
//     treat it as a redundant reselect and leave the panel blank on return.
func (vd VideoDetail) handleVideoClear() (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	if kittyCapable() && vd.thumbB64 != "" {
		cmds = append(cmds, tea.Raw(kittyDeleteSeq()))
	}
	vd.fetchToken++
	vd = vd.renewFetchCtx()
	vd.loading = false
	vd.video = nil
	vd.fetchVideo = domain.Video{}
	vd.thumb = nil
	vd.thumbB64 = ""
	vd.thumbRendered = ""
	vd = vd.resetTranscript()
	return vd, tea.Batch(cmds...)
}

// resetTranscript clears cached transcript state when the shown video changes,
// so a superseded fetch can't leave a stale transcript or loading popup behind.
func (vd VideoDetail) resetTranscript() VideoDetail {
	vd.transcriptText = ""
	vd.transcriptVS = 0
	vd.transcriptLoaded = false
	vd.transcriptLoading = false
	return vd
}

// handleThumbnailLoaded stores the fetched thumbnail and encodes/renders it for
// the active terminal backend.
func (vd VideoDetail) handleThumbnailLoaded(m ThumbnailLoadedMsg) (tea.Model, tea.Cmd) {
	vd.thumb = m.Img
	if m.Img != nil {
		if kittyCapable() {
			vd.thumbB64 = encodeThumbB64(m.Img)
		} else {
			_, thumbH := vd.thumbDimensions()
			vd.thumbRendered = renderThumbnailHalfBlock(m.Img, panelW-2, thumbH)
		}
	}
	return vd, vd.kittyAfterFrameCmd()
}

// ── helpers ───────────────────────────────────────────────────────────────────

// defaultThumbAspect{W,H} is the 16:9 fallback aspect used to size the panel
// thumbnail before the real image dimensions are known.
const (
	defaultThumbAspectW = 16
	defaultThumbAspectH = 9
)

// halfBlockRows returns how many terminal rows a width-w image occupies when
// drawn with half-block glyphs at aspect num:den — ceil(w*num/den) pixel-rows,
// two per terminal row.
func halfBlockRows(w, num, den int) int {
	if den <= 0 {
		return 1
	}
	return (w*num + den - 1) / den / 2
}

func (vd VideoDetail) thumbDimensions() (w, h int) {
	thumbW := panelW - 2
	thumbH := halfBlockRows(thumbW, defaultThumbAspectH, defaultThumbAspectW)
	if thumbH < 1 {
		thumbH = 1
	}
	if vd.thumb != nil {
		b := vd.thumb.Bounds()
		iw := b.Max.X - b.Min.X
		ih := b.Max.Y - b.Min.Y
		if iw > 0 && ih > 0 {
			if h := halfBlockRows(thumbW, ih, iw); h >= 1 {
				thumbH = h
			}
		}
	}
	return thumbW, thumbH
}

// applyPostLoad runs the steps shared by a cache hit and a network fetch once
// vd.video is populated: schedule the thumbnail load when the URL changed, and
// — on the first load only — open the requested initial modal (links/chapters)
// or pop the overlay when the requested view isn't available.
func (vd VideoDetail) applyPostLoad(firstLoad bool, prevThumbURL string, cmds []tea.Cmd) (VideoDetail, []tea.Cmd) {
	if vd.video.ThumbnailURL != "" && vd.initialView == InitialViewPanel && vd.video.ThumbnailURL != prevThumbURL {
		cmds = append(cmds, loadThumbnailCmd(vd.ctx, vd.media, vd.ID(), vd.video.ID, vd.video.ThumbnailURL))
	}
	if firstLoad {
		var initCmd tea.Cmd
		switch vd.initialView {
		case InitialViewChapters:
			vd, initCmd = vd.openChapters()
		case InitialViewLinks:
			vd, initCmd = vd.openLinks()
		}
		if vd.initialView != InitialViewPanel && vd.subState == vdPanel {
			cmds = append(cmds, func() tea.Msg { return PopOverlayMsg{} })
		}
		cmds = append(cmds, initCmd)
	}
	return vd, cmds
}
