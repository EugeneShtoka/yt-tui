package app

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/device/player"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/command"
	"github.com/EugeneShtoka/yt-tui/internal/tui/component"
	"github.com/EugeneShtoka/yt-tui/internal/tui/keymap"
	ovpkg "github.com/EugeneShtoka/yt-tui/internal/tui/overlay"
	"github.com/EugeneShtoka/yt-tui/internal/tui/playback"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/EugeneShtoka/yt-tui/internal/tui/videotable"
)

// selectionDebounceMsg is a root-internal debounce timer result.
type selectionDebounceMsg struct {
	token int
	video domain.Video
}

const selectionDebounceDelay = 150 * time.Millisecond

// Root is the top-level BubbleTea model.
// It owns focus, size, global key routing, and the tab/overlay stack.
type Root struct {
	ctx      context.Context // app-lifetime context; canceled on exit (H-1)
	backend  api.Backend
	media    api.MediaProvider // client-side thumbnail/transcript seam (may cache locally)
	cfg      *config.Config
	playback playback.Controller
	keys     keymap.KeyMap
	cmds     command.Registry

	width, height int

	tabBar    component.TabBar
	statusBar component.StatusBar

	tabs      []tuipkg.Tab
	activeIdx int

	overlays overlayStack

	// spinner is the single app-wide loading spinner. Root runs one tick loop
	// and forwards each frame to the active tab and top overlay; individual
	// tabs no longer drive their own spinner. spinnerRunning tracks whether a
	// tick is currently scheduled so syncSpinner only restarts it once (M-14).
	spinner        spinner.Model
	spinnerRunning bool

	tabChordActive bool
	// tabChordKeys maps a tab-chord second key to a panel index (config named
	// hotkeys + the positional 1..9 fallback). panelIdx maps a panel name to
	// its index for name-based navigation (the `:tab` command).
	tabChordKeys           map[string]int
	panelIdx               map[string]int
	selectionDebounceToken int
}

// New constructs the Root with the current tab set.
// pl may be nil if no player binary was found; play actions will show an error.
// issues are the non-fatal config/environment problems collected at startup;
// when non-empty they open a dismissible ConfigIssues overlay on the first
// frame so the user sees what was ignored (empty = clean start, no overlay).
func New(ctx context.Context, backend api.Backend, media api.MediaProvider, cfg *config.Config, pl player.Backend, issues []config.ConfigIssue) Root {
	keys := keymap.Build(cfg.Keybindings)

	// The tab bar is data-driven from the configured panel list; the default
	// layout reproduces the historical fixed tabs (see config.DefaultPanels).
	panels := effectivePanels(cfg.Panels)
	tabs := buildPanels(ctx, panels, backend, keys, cfg)

	titles := make([]string, len(tabs))
	for i, t := range tabs {
		titles[i] = t.Title()
	}

	var cmds command.Registry
	cmds.Register(globalCommands(ctx, backend, panelNames(panels))...)

	right := keys.Help.Help().Key + ": help  " + keys.Quit.Help().Key + ": quit"

	// The bubbles Line spinner ticks at 10 fps by default; slow it to a calm
	// 8 fps so the loading indicator reads as a spinner rather than a strobe.
	sp := spinner.New()
	sp.Spinner.FPS = time.Second / 8

	r := Root{
		ctx:          ctx,
		backend:      backend,
		media:        media,
		cfg:          cfg,
		playback:     playback.New(ctx, backend, pl),
		keys:         keys,
		cmds:         cmds,
		tabBar:       component.NewTabBar(titles),
		statusBar:    component.NewStatusBar(right),
		tabs:         tabs,
		tabChordKeys: buildTabChordKeys(cfg.Keybindings.TabKeys, panels),
		panelIdx:     panelIndexByName(panels),
		spinner:      sp,
	}
	if len(issues) > 0 {
		r.overlays = append(r.overlays, ovpkg.NewConfigIssues(issues, keys))
	}
	return r
}

// baseCtx returns the app-lifetime context threaded from main (canceled on
// exit, H-1). It falls back to r.baseCtx() only when unset — i.e. in
// tests that build Root as a literal without a ctx — so those never pass a nil
// context to the backend.
func (r Root) baseCtx() context.Context {
	if r.ctx != nil {
		return r.ctx
	}
	return context.Background()
}

// ── tea.Model ─────────────────────────────────────────────────────────────────

func (r Root) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(r.tabs)+1)
	for _, t := range r.tabs {
		cmds = append(cmds, t.Init())
	}
	cmds = append(cmds, r.spinner.Tick, pollTickCmd())
	return tea.Batch(cmds...)
}

// pollInterval is how often Root nudges the active tab to reload its DB-backed
// data, so videos a background crawl streams in appear without a manual refresh.
const pollInterval = 3 * time.Second

// pollTickCmd arms the next poll tick.
func pollTickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return tuipkg.PollTickMsg{} })
}

// Update is a pure dispatcher: each case delegates to a focused handler so the
// command bodies (play/enqueue/copy/…) live in named methods rather than inline.
// It wraps updateDispatch with syncSpinner so the spinner tick loop starts and
// stops based on whether the active tab or top overlay is actually loading,
// rather than running unconditionally for the process lifetime (M-14).
func (r Root) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	newModel, cmd := r.updateDispatch(msg)
	nr := newModel.(Root)
	nr, spinCmd := nr.syncSpinner()
	return nr, tea.Batch(cmd, spinCmd)
}

func (r Root) syncSpinner() (Root, tea.Cmd) {
	loading := r.activeTab().Loading() || len(r.overlays) > 0
	if loading && !r.spinnerRunning {
		r.spinnerRunning = true
		return r, r.spinner.Tick
	}
	return r, nil
}

func (r Root) updateDispatch(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		return r.handleResize(m.Width, m.Height)
	case tea.KeyPressMsg:
		return r.handleKey(m)
	case tuipkg.OpenOverlayMsg:
		return r.handleOpenOverlay(m)
	case ovpkg.PopOverlayMsg:
		return r.handlePopOverlay()
	case ovpkg.ApplyConfigProfileMsg:
		return r.handleApplyConfigProfile(m)
	}
	if handled, model, cmd := r.dispatchPlayback(msg); handled {
		return model, cmd
	}
	if handled, model, cmd := r.dispatchChannelAction(msg); handled {
		return model, cmd
	}
	if handled, model, cmd := r.dispatchCommandPalette(msg); handled {
		return model, cmd
	}
	return r.dispatchMisc(msg)
}

// dispatchPlayback routes playback- and position-tracking messages. It reports
// handled=false for any other message so updateDispatch can fall through.
func (r Root) dispatchPlayback(msg tea.Msg) (bool, tea.Model, tea.Cmd) {
	// The playback lifecycle (play/launch/started/position-tick) is owned by the
	// playback controller; it only produces commands, so Root delegates without
	// threading its model through (H-2).
	if cmd, ok := r.playback.Update(msg); ok {
		return true, r, cmd
	}
	switch m := msg.(type) {
	case tuipkg.RefreshPositionsMsg:
		model, cmd := r.handleRefreshPositions(m)
		return true, model, cmd
	case tuipkg.EnqueueMsg:
		model, cmd := r.handleEnqueue(m)
		return true, model, cmd
	case tuipkg.EnqueueSucceededMsg:
		model, cmd := r.handleEnqueueSucceeded(m)
		return true, model, cmd
	case tuipkg.CopyURLMsg:
		model, cmd := r.handleCopyURL(m)
		return true, model, cmd
	case tuipkg.CopyTextMsg:
		model, cmd := r.handleCopyText(m)
		return true, model, cmd
	}
	return false, r, nil
}

// dispatchChannelAction routes navigation and channel-management messages. It
// reports handled=false for any other message so updateDispatch can fall through.
func (r Root) dispatchChannelAction(msg tea.Msg) (bool, tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuipkg.NavigateMsg:
		model, cmd := r.handleNavigate(m)
		return true, model, cmd
	case tuipkg.NavigateToPanelMsg:
		model, cmd := r.handleNavigateToPanel(m)
		return true, model, cmd
	case tuipkg.HideChannelMsg:
		model, cmd := r.handleHideChannel(m)
		return true, model, cmd
	case tuipkg.UnsubscribeMsg:
		model, cmd := r.handleUnsubscribe(m)
		return true, model, cmd
	case tuipkg.UnsubscribeResultMsg:
		model, cmd := r.handleUnsubscribeResult(m)
		return true, model, cmd
	case tuipkg.BlockChannelMsg:
		model, cmd := r.handleBlockChannel(m)
		return true, model, cmd
	case tuipkg.BlockChannelResultMsg:
		model, cmd := r.handleBlockChannelResult(m)
		return true, model, cmd
	case tuipkg.NavigateToChannelMsg:
		model, cmd := r.handleNavigate(tuipkg.NavigateMsg{Tab: tuipkg.TabChannels})
		return true, model, cmd
	case tuipkg.NavigateToPlaylistMsg:
		model, cmd := r.handleNavigateToPlaylist(m)
		return true, model, cmd
	}
	return false, r, nil
}

// dispatchMisc routes the remaining status/spinner messages and falls back to
// the broadcast path (preserving updateDispatch's original default case).
func (r Root) dispatchMisc(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuipkg.PollTickMsg:
		// Re-arm unconditionally (so the loop survives tab switches) and forward
		// to the active tab only, keeping the per-tick cost to one reload.
		var tabCmd tea.Cmd
		r, tabCmd = r.updateActiveTab(msg)
		return r, tea.Batch(pollTickCmd(), tabCmd)
	case tuipkg.StatusMsg:
		return r.handleStatus(m)
	case tuipkg.StatusExpireMsg:
		return r.handleStatus(m)
	case selectionDebounceMsg:
		return r.handleSelectionDebounce(m)
	case spinner.TickMsg:
		return r.handleSpinnerTick(m)
	default:
		return r.handleBroadcast(msg)
	}
}

// ── command handlers (extracted from Update) ──────────────────────────────────

// handleRefreshPositions issues a single aux-data load; the resulting
// AuxDataMsg reaches every tab via the default broadcast path in
// handleBroadcast, since it isn't a TabAddressedMsg (M-10 — this used to
// broadcast the raw RefreshPositionsMsg and let each of 7 tabs issue its own
// redundant LoadAuxDataCmd).
func (r Root) handleRefreshPositions(m tuipkg.RefreshPositionsMsg) (Root, tea.Cmd) {
	return r, videotable.LoadAuxDataCmd(r.baseCtx(), r.backend)
}

func (r Root) handleEnqueue(m tuipkg.EnqueueMsg) (Root, tea.Cmd) {
	return r, r.actions().enqueue(r.baseCtx(), m.Video, m.AudioOnly)
}

func (r Root) handleEnqueueSucceeded(m tuipkg.EnqueueSucceededMsg) (Root, tea.Cmd) {
	label := "video"
	if m.AudioOnly {
		label = "audio"
	}
	return r, tea.Batch(
		func() tea.Msg {
			return tuipkg.StatusMsg{Text: "Queued " + label + ": " + render.Truncate(m.Title, 50)}
		},
		func() tea.Msg { return tuipkg.DownloadItemsChangedMsg{} },
	)
}

func (r Root) handleCopyURL(m tuipkg.CopyURLMsg) (Root, tea.Cmd) {
	return r, copyCmd(m.URL, "Copied: "+m.URL)
}

// handleCopyText writes arbitrary text (e.g. a full transcript) to the clipboard,
// reporting a label-based status rather than echoing the whole payload.
func (r Root) handleCopyText(m tuipkg.CopyTextMsg) (Root, tea.Cmd) {
	label := m.Label
	if label == "" {
		label = "text"
	}
	return r, copyCmd(m.Text, "Copied "+label+" to clipboard")
}

// handleExport opens the export-selection overlay, which fetches the full
// app-owned data bundle and lets the user pick which sections to include
// (Space toggles, Enter writes) before saving a timestamped JSON to the data dir.
//
// The portable config profile is marshaled here and passed in, because the
// backend never sees config — in remote mode the config being backed up is the
// client's, not the daemon's — so it rides along inproc and remote identically
// with no RPC. A marshal failure is surfaced (as an unavailable config section)
// rather than aborting the whole export.
func (r Root) handleExport() (Root, tea.Cmd) {
	profile, perr := MarshalConfigProfile(r.cfg)
	es, cmd := ovpkg.NewExportSelect(r.baseCtx(), r.backend, r.keys, r.cfg.DataDir, profile, perr, r.cfg.CircularNav)
	r.overlays = append(r.overlays, es)
	return r, cmd
}

// handleImport opens the import-preview overlay, which lists exported bundle
// files in the data dir, shows a dry-run diff of the selected one with the two
// opt-in toggles (YT→local conversion, include watch data), and applies it. The
// preview overlay is the confirm step — no separate throwaway confirm UI.
func (r Root) handleImport() (Root, tea.Cmd) {
	ip, cmd := ovpkg.NewImportPreview(r.baseCtx(), r.backend, r.keys, r.cfg.DataDir, r.cfg.CircularNav)
	r.overlays = append(r.overlays, ip)
	return r, cmd
}

// handleApplyConfigProfile decodes the bundle's portable config profile and
// overwrites the live config's portable fields wholesale (machine-local fields
// stay put), then normalizes and persists it. It runs on the main update loop
// (not a background command) because Root owns the config value. Keybindings and
// panels are read at startup, so a restart is needed for the new layout/keys to
// take full effect — the status line says so.
func (r Root) handleApplyConfigProfile(m ovpkg.ApplyConfigProfileMsg) (Root, tea.Cmd) {
	if err := ApplyConfigProfile(r.cfg, m.Config); err != nil {
		return r, func() tea.Msg {
			return tuipkg.StatusMsg{Text: "import config: " + err.Error(), IsErr: true}
		}
	}
	if err := r.cfg.Save(); err != nil {
		return r, func() tea.Msg {
			return tuipkg.StatusMsg{Text: "import config: save: " + err.Error(), IsErr: true}
		}
	}
	return r, func() tea.Msg {
		return tuipkg.StatusMsg{Text: "Config applied — restart to load new keybindings & panels"}
	}
}

func (r Root) handleNavigateToPlaylist(m tuipkg.NavigateToPlaylistMsg) (Root, tea.Cmd) {
	var navCmd tea.Cmd
	r, navCmd = r.handleNavigate(tuipkg.NavigateMsg{Tab: tuipkg.TabPlaylists})
	var fwdCmd tea.Cmd
	r, fwdCmd = r.updateActiveTab(m)
	return r, tea.Batch(navCmd, fwdCmd)
}

func (r Root) handleStatus(m tea.Msg) (Root, tea.Cmd) {
	sb, cmd := r.statusBar.Update(m)
	r.statusBar = sb.(component.StatusBar)
	return r, cmd
}

func (r Root) handleSelectionDebounce(m selectionDebounceMsg) (Root, tea.Cmd) {
	if m.token == r.selectionDebounceToken && len(r.overlays) > 0 {
		// Deliver to the info panel wherever it is in the stack — a modal may have
		// been opened over it since the debounce was scheduled.
		return r.updatePanelOverlay(tuipkg.VideoSelectedMsg{Video: m.video})
	}
	return r, nil
}

// handleBroadcast is the default catch-all. Messages that implement
// tuipkg.TabAddressedMsg (background-load responses like feedSubLoadedMsg,
// chsLoadedMsg) are routed only to their owner tab; everything else is
// broadcast to every tab so untagged messages still reach whichever tab handles
// them. The top overlay always gets a chance to handle its own private messages
// (M-5).
func (r Root) handleBroadcast(msg tea.Msg) (Root, tea.Cmd) {
	// Overlay-addressed async results (OverlayAddressedMsg) route to the overlay
	// instance that spawned them — matched by ID(), wherever it sits in the stack
	// — never to whatever overlay is on top. This is what lets a background
	// overlay (e.g. the info panel with AddToPlaylist stacked over it) still
	// receive its own fetch results instead of having them stolen by the top
	// overlay. A message whose target has since been popped is simply dropped, and
	// such private results never reach the tabs.
	if am, ok := msg.(tuipkg.OverlayAddressedMsg); ok {
		for i, o := range r.overlays {
			if o.ID() == am.TargetOverlay() {
				updated, cmd := o.Update(msg)
				r.overlays[i] = updated.(ovpkg.Overlay)
				return r, cmd
			}
		}
		return r, nil
	}

	var c1 tea.Cmd
	if len(r.overlays) > 0 {
		r, c1 = r.updateTopOverlay(msg)
	}
	var bcmds []tea.Cmd
	if c1 != nil {
		bcmds = append(bcmds, c1)
	}

	if am, ok := msg.(tuipkg.TabAddressedMsg); ok {
		for i, t := range r.tabs {
			if t.ID() != am.TargetTab() {
				continue
			}
			updated, cmd := t.Update(msg)
			r.tabs[i] = updated.(tuipkg.Tab)
			if cmd != nil {
				bcmds = append(bcmds, cmd)
			}
			break
		}
		return r, tea.Batch(bcmds...)
	}

	for i, t := range r.tabs {
		updated, cmd := t.Update(msg)
		r.tabs[i] = updated.(tuipkg.Tab)
		if cmd != nil {
			bcmds = append(bcmds, cmd)
		}
	}
	return r, tea.Batch(bcmds...)
}

func (r Root) View() tea.View {
	tabBar := r.tabBar.Render()
	status := r.statusBar.Render()
	contentH := r.height - lipgloss.Height(tabBar) - lipgloss.Height(status)

	// Fade the active tab's table frame while the info panel holds focus, so the
	// focused panel (normal border) and the inactive list (dim border) form a
	// symmetric focus signal. Set before rendering the tab; the table reads it.
	styles.ListBorderDimmed = r.listBorderDimmed()

	content := r.activeTab().View().Content

	for _, o := range r.overlays {
		content = o.Render(content, r.width, contentH)
	}

	if actual := lipgloss.Height(content); actual < contentH {
		content += strings.Repeat("\n", contentH-actual)
	}

	// Final safety net: clamp every content line to exactly the terminal width so
	// no overlay/modal/tab can emit an over-wide line that wraps in the terminal
	// and shifts every row below it.
	contentLines := strings.Split(content, "\n")
	for i, l := range contentLines {
		contentLines[i] = render.ClampLine(l, r.width)
	}
	content = strings.Join(contentLines, "\n")

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, tabBar, content, status))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// ── message handlers ──────────────────────────────────────────────────────────

// handleSpinnerTick advances the single shared spinner and forwards its current
// frame to the active tab and top overlay so they can render loading animation
// without each running an independent tick loop. While something is still
// loading it returns the library's auto-rearm cmd, which is delayed by the
// spinner's FPS — that delay is what paces the animation. Once nothing is
// loading it drops the rearm so the loop stops instead of running forever
// (M-14); syncSpinner restarts it on demand.
func (r Root) handleSpinnerTick(m spinner.TickMsg) (Root, tea.Cmd) {
	var rearm tea.Cmd
	r.spinner, rearm = r.spinner.Update(m)

	frame := tuipkg.SpinnerFrameMsg{Frame: r.spinner.View()}
	if r.activeIdx >= 0 && r.activeIdx < len(r.tabs) {
		updated, _ := r.tabs[r.activeIdx].Update(frame)
		r.tabs[r.activeIdx] = updated.(tuipkg.Tab)
	}
	// Fan the frame to every overlay, not just the top one: a still-loading
	// overlay beneath a stacked modal (e.g. the info panel with AddToPlaylist over
	// it) must keep animating its spinner. It's still visible around the centered
	// modal, so a frozen spinner reads as a hang.
	for i := range r.overlays {
		ov, _ := r.overlays[i].Update(frame)
		r.overlays[i] = ov.(ovpkg.Overlay)
	}

	if r.activeTab().Loading() || len(r.overlays) > 0 {
		return r, rearm
	}
	r.spinnerRunning = false
	return r, nil
}

func (r Root) handleResize(w, h int) (Root, tea.Cmd) {
	r.width, r.height = w, h
	r.tabBar = r.tabBar.WithWidth(w)
	r.statusBar = r.statusBar.WithWidth(w).WithHints(r.tabHints())

	tabBarH := lipgloss.Height(r.tabBar.Render())
	statusH := lipgloss.Height(r.statusBar.Render())
	contentH := h - tabBarH - statusH
	contentW := w - r.overlays.widthReduction()

	sizeMsg := tuipkg.ContentSizeMsg{Width: contentW, Height: contentH}
	var cmds []tea.Cmd
	for i, t := range r.tabs {
		updated, cmd := t.Update(sizeMsg)
		r.tabs[i] = updated.(tuipkg.Tab)
		cmds = append(cmds, cmd)
	}

	// Forward size info to overlays so they can compute Kitty image positions.
	ovMsg := ovpkg.OverlaySizeMsg{
		ContentW: contentW,
		ContentH: contentH,
		KittyRow: tabBarH + 2, // 1-indexed: past tab bar + top panel border
	}
	for i, o := range r.overlays {
		updated, cmd := o.Update(ovMsg)
		r.overlays[i] = updated.(ovpkg.Overlay)
		cmds = append(cmds, cmd)
	}

	return r, tea.Batch(cmds...)
}

func (r Root) handleOpenOverlay(m tuipkg.OpenOverlayMsg) (Root, tea.Cmd) {
	switch m.Kind {
	case tuipkg.OverlayVideoDetail:
		return r.openInfoPanel(m)
	case tuipkg.OverlayVideoDetailLinks, tuipkg.OverlayVideoDetailChapters, tuipkg.OverlayVideoDetailTranscript:
		return r.openVideoDetailModal(m)
	case tuipkg.OverlayAddToPlaylist:
		atp, cmd := ovpkg.NewAddToPlaylist(r.baseCtx(), r.backend, r.keys, m.Video, r.cfg.CircularNav)
		r.overlays = append(r.overlays, atp)
		return r, cmd
	}
	return r, nil
}

// openInfoPanel toggles the info side panel: pressing VideoInfo while the panel
// is already the top overlay closes it; otherwise it opens the panel and hands
// it its size via the resize that OverlaySizeMsg rides on.
func (r Root) openInfoPanel(m tuipkg.OpenOverlayMsg) (Root, tea.Cmd) {
	if _, ok := r.overlays.topPanel(); ok {
		return r.handlePopOverlay()
	}
	vd, cmd := ovpkg.NewVideoDetail(r.baseCtx(), r.backend, r.media, r.keys, m.Video, ovpkg.VideoDetailOpts{
		CloseOnLinks: r.cfg.CloseOnLinkOpen, Circular: r.cfg.CircularNav,
		InitialView: ovpkg.InitialViewPanel, TranscriptWidth: r.cfg.TranscriptWidth,
	})
	r.overlays = append(r.overlays, vd)
	var resizeCmd tea.Cmd
	r, resizeCmd = r.handleResize(r.width, r.height)
	return r, tea.Batch(cmd, resizeCmd)
}

// openVideoDetailModal opens a links/chapters/transcript view. When the info
// panel is already open it routes the sub-view into that panel (no duplicate
// side panel, shared loaded details); otherwise it stacks a standalone, centered
// modal that reserves no width and never resizes or closes the panel.
func (r Root) openVideoDetailModal(m tuipkg.OpenOverlayMsg) (Root, tea.Cmd) {
	initView := ovpkg.InitialViewLinks
	switch m.Kind {
	case tuipkg.OverlayVideoDetailChapters:
		initView = ovpkg.InitialViewChapters
	case tuipkg.OverlayVideoDetailTranscript:
		initView = ovpkg.InitialViewTranscript
	}
	if _, ok := r.overlays.topPanel(); ok {
		return r.updateTopOverlay(ovpkg.OpenModalMsg{View: initView})
	}
	vd, cmd := ovpkg.NewVideoDetail(r.baseCtx(), r.backend, r.media, r.keys, m.Video, ovpkg.VideoDetailOpts{
		CloseOnLinks: r.cfg.CloseOnLinkOpen, Circular: r.cfg.CircularNav,
		InitialView: initView, TranscriptWidth: r.cfg.TranscriptWidth,
	})
	r.overlays = append(r.overlays, vd)
	return r, cmd
}

// handleOpenHelp opens the keyboard-shortcut overlay, unless it is already the
// top overlay (pressing Help again while it is open is a no-op; the overlay's
// own key handler closes it).
func (r Root) handleOpenHelp() (Root, tea.Cmd) {
	if r.overlays.topIsHelp() {
		return r, nil
	}
	r.overlays = append(r.overlays, ovpkg.NewHelp(r.keys))
	return r, nil
}

func (r Root) handlePopOverlay() (Root, tea.Cmd) {
	if r.overlays.pop() {
		// The popped overlay reserved width (the info panel); re-layout so the tab
		// content reclaims it.
		return r.handleResize(r.width, r.height)
	}
	return r, nil
}

func (r Root) handleKey(msg tea.KeyPressMsg) (Root, tea.Cmd) {
	// Chord completion is universal — resolve before re-matching TabChord.
	if r.tabChordActive {
		r.tabChordActive = false
		if idx, ok := r.tabChordKeys[msg.String()]; ok {
			return r.handleTabNavigateIndex(idx)
		}
		return r, nil
	}

	if len(r.overlays) > 0 {
		return r.handleKeyWithOverlay(msg)
	}

	if r.activeTab().InterceptsInput() {
		return r.updateActiveTab(msg)
	}
	switch {
	case key.Matches(msg, r.keys.Quit):
		return r, tea.Quit
	case key.Matches(msg, r.keys.Help):
		return r.handleOpenHelp()
	case key.Matches(msg, r.keys.Export):
		return r.handleExport()
	case key.Matches(msg, r.keys.Import):
		return r.handleImport()
	case key.Matches(msg, r.keys.CommandPrompt):
		return r.handleOpenCommandBar()
	case key.Matches(msg, r.keys.Tab):
		return r.handleTabCycle(+1)
	case key.Matches(msg, r.keys.ShiftTab):
		return r.handleTabCycle(-1)
	case key.Matches(msg, r.keys.TabChord):
		r.tabChordActive = true
		return r, nil
	}
	return r.updateActiveTab(msg)
}

// handleKeyWithOverlay resolves a key press while at least one overlay is open.
// Extracted verbatim from handleKey; preserves the exact match order and
// short-circuit semantics.
func (r Root) handleKeyWithOverlay(msg tea.KeyPressMsg) (Root, tea.Cmd) {
	top := r.overlays[len(r.overlays)-1]

	// A focused text input (in the overlay, or in the tab beneath an
	// unfocused overlay) must receive every key verbatim — global chords
	// below would otherwise steal characters instead of inserting them.
	if top.InterceptsInput() {
		return r.updateTopOverlay(msg)
	}
	if !top.HasFocus() && r.activeTab().InterceptsInput() {
		return r.forwardKeyToActiveTab(msg)
	}

	// Help is global but special-cased first: when help is already on top, fall
	// through so its own key handler closes it — pressing Help toggles rather
	// than stacks.
	if _, helpOnTop := top.(ovpkg.Help); !helpOnTop && key.Matches(msg, r.keys.Help) {
		return r.handleOpenHelp()
	}

	// The remaining global chords, in precedence order. A focused text overlay
	// was already handled by the InterceptsInput short-circuit above, so e.g.
	// `:` there types a literal colon rather than opening the palette.
	for _, c := range r.overlayChords() {
		if key.Matches(msg, c.binding) {
			return c.handle(r, msg)
		}
	}

	// No chord matched: a focused overlay consumes the key; otherwise it falls
	// through to the tab beneath.
	if top.HasFocus() {
		return r.updateTopOverlay(msg)
	}
	return r.forwardKeyToActiveTab(msg)
}

// overlayChord is one global key chord active while an overlay is open, paired
// with its handler. handle receives the current Root and key.
type overlayChord struct {
	binding key.Binding
	handle  func(Root, tea.KeyPressMsg) (Root, tea.Cmd)
}

// overlayChords is the ordered global-chord table handleKeyWithOverlay iterates:
// the sequence is the precedence spec. (Help is handled before this table
// because of its toggle-on-top exception.)
func (r Root) overlayChords() []overlayChord {
	return []overlayChord{
		{r.keys.CommandPrompt, func(r Root, _ tea.KeyPressMsg) (Root, tea.Cmd) { return r.handleOpenCommandBar() }},
		{r.keys.FocusSwitch, func(r Root, _ tea.KeyPressMsg) (Root, tea.Cmd) { return r.updateTopOverlay(ovpkg.FocusSwitchMsg{}) }},
		{r.keys.Tab, func(r Root, _ tea.KeyPressMsg) (Root, tea.Cmd) { return r.handleTabCycle(+1) }},
		{r.keys.ShiftTab, func(r Root, _ tea.KeyPressMsg) (Root, tea.Cmd) { return r.handleTabCycle(-1) }},
		{r.keys.TabChord, func(r Root, _ tea.KeyPressMsg) (Root, tea.Cmd) { r.tabChordActive = true; return r, nil }},
		{r.keys.Escape, func(r Root, msg tea.KeyPressMsg) (Root, tea.Cmd) { return r.updateTopOverlay(msg) }},
		{r.keys.Quit, func(r Root, _ tea.KeyPressMsg) (Root, tea.Cmd) { return r, tea.Quit }},
	}
}

func (r Root) handleTabCycle(dir int) (Root, tea.Cmd) {
	if len(r.overlays) == 0 {
		return r.cycleTab(dir)
	}
	// Switch the tab first (the reselect debounce reads the NEW tab's selection),
	// then reconcile overlays: close stacked modals, keep/close the info panel.
	var cycleCmd tea.Cmd
	r, cycleCmd = r.cycleTab(dir)
	var reconcileCmd tea.Cmd
	r, reconcileCmd = r.reconcileOverlaysAfterTabSwitch()
	return r, tea.Batch(cycleCmd, reconcileCmd)
}

// handleTabNavigateIndex switches to the panel at index idx (tab-chord / `:tab`
// navigation) and reconciles any open info overlay via finishTabSwitch.
func (r Root) handleTabNavigateIndex(idx int) (Root, tea.Cmd) {
	var navCmd tea.Cmd
	r, navCmd = r.handleNavigateIndex(idx)
	return r.finishTabSwitch(navCmd)
}

// finishTabSwitch reconciles any open overlays after a chord/name tab switch,
// mirroring handleTabCycle: stacked modals close and the info panel stays open,
// re-tracking the new tab. A no-op when nothing is open.
func (r Root) finishTabSwitch(navCmd tea.Cmd) (Root, tea.Cmd) {
	var reconcileCmd tea.Cmd
	r, reconcileCmd = r.reconcileOverlaysAfterTabSwitch()
	return r, tea.Batch(navCmd, reconcileCmd)
}

// handleNavigate switches to the first panel whose tab has the given TabID,
// used by internal cross-tab navigation (NavigateToChannel/Playlist, feed
// actions). User-facing navigation goes by panel index/name instead.
func (r Root) handleNavigate(m tuipkg.NavigateMsg) (Root, tea.Cmd) {
	for i, t := range r.tabs {
		if t.ID() == m.Tab {
			return r.applyNavigate(i, m.Query)
		}
	}
	return r, nil
}

// handleNavigateIndex switches to the panel at index idx.
func (r Root) handleNavigateIndex(idx int) (Root, tea.Cmd) {
	return r.applyNavigate(idx, "")
}

// handleNavigateToPanel resolves a panel name (from the `:tab` command) to its
// index and switches to it, reporting an unknown name to the status bar.
func (r Root) handleNavigateToPanel(m tuipkg.NavigateToPanelMsg) (Root, tea.Cmd) {
	idx, ok := r.panelIdx[m.Name]
	if !ok {
		name := m.Name
		return r, func() tea.Msg { return tuipkg.StatusMsg{Text: "unknown tab: " + name, IsErr: true} }
	}
	return r.handleTabNavigateIndex(idx)
}

// applyNavigate activates the panel at index i, syncs the tab bar + status
// hints, and drives the Search tab's prefill/focus behavior.
func (r Root) applyNavigate(i int, query string) (Root, tea.Cmd) {
	if i < 0 || i >= len(r.tabs) {
		return r, nil
	}
	r.activeIdx = i
	r.tabBar = r.tabBar.WithActive(i)
	r.statusBar = r.statusBar.WithHints(r.tabHints())
	var refreshCmd tea.Cmd
	r, refreshCmd = r.refreshOnOpen()
	if r.activeTab().ID() == tuipkg.TabSearch {
		if query != "" {
			q := query
			return r, tea.Batch(refreshCmd, func() tea.Msg { return tuipkg.SearchActivateMsg{Query: q} })
		}
		return r, tea.Batch(refreshCmd, func() tea.Msg { return tuipkg.SearchFocusInputMsg{} })
	}
	return r, refreshCmd
}

func (r Root) handleHideChannel(m tuipkg.HideChannelMsg) (Root, tea.Cmd) {
	return r, r.actions().hideChannel(r.baseCtx(), m.Channel)
}

func (r Root) handleUnsubscribe(m tuipkg.UnsubscribeMsg) (Root, tea.Cmd) {
	return r, r.actions().unsubscribe(r.baseCtx(), m.Channel)
}

// handleUnsubscribeResult broadcasts the result to every tab, so those that
// optimistically removed the channel (Channels, Subscriptions) can restore it
// on failure, and shows the outcome in the status bar.
func (r Root) handleUnsubscribeResult(m tuipkg.UnsubscribeResultMsg) (Root, tea.Cmd) {
	r, bcastCmd := r.handleBroadcast(m)
	statusCmd := func() tea.Msg {
		if m.Err != nil {
			return tuipkg.StatusMsg{Text: "unsubscribe: " + m.Err.Error(), IsErr: true}
		}
		return tuipkg.StatusMsg{Text: "Unsubscribed: " + m.Channel.Name}
	}
	return r, tea.Batch(bcastCmd, statusCmd)
}

// handleBlockChannel runs the guarded block/unblock transition on the backend
// and reports the outcome. The tab has already applied it optimistically.
func (r Root) handleBlockChannel(m tuipkg.BlockChannelMsg) (Root, tea.Cmd) {
	return r, r.actions().blockChannel(r.baseCtx(), m.Channel, m.Block)
}

// handleBlockChannelResult broadcasts the result so the Channels tab can revert
// its optimistic transition on failure, and shows the outcome in the status bar.
func (r Root) handleBlockChannelResult(m tuipkg.BlockChannelResultMsg) (Root, tea.Cmd) {
	r, bcastCmd := r.handleBroadcast(m)
	verb := "block"
	if !m.Block {
		verb = "unblock"
	}
	statusCmd := func() tea.Msg {
		if m.Err != nil {
			return tuipkg.StatusMsg{Text: verb + ": " + m.Err.Error(), IsErr: true}
		}
		past := "Blocked: "
		if !m.Block {
			past = "Unblocked: "
		}
		return tuipkg.StatusMsg{Text: past + m.Channel.DisplayName()}
	}
	return r, tea.Batch(bcastCmd, statusCmd)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (r Root) activeTab() tuipkg.Tab { return r.tabs[r.activeIdx] }

// actions returns the backend-mutation command factories bound to Root's backend
// (H-2). Derived on demand rather than stored so a Root literal built in tests
// without construction wiring still routes through the same code path.
func (r Root) actions() backendActions { return backendActions{backend: r.backend} }

// listBorderDimmed reports whether the active tab's table frame should render
// dimmed — true exactly when a side info panel is open and holds focus, so the
// list beneath reads as inactive.
func (r Root) listBorderDimmed() bool {
	for _, o := range r.overlays {
		if vd, ok := o.(ovpkg.VideoDetail); ok && vd.PanelFocused() {
			return true
		}
	}
	return false
}

// forwardKeyToActiveTab sends msg to the active tab while an overlay that
// doesn't own the key is open, clearing the overlay if the selected video
// changes as a result (e.g. list navigation under an unfocused info panel).
func (r Root) forwardKeyToActiveTab(msg tea.Msg) (Root, tea.Cmd) {
	prevID := ""
	if v, ok := r.activeTab().SelectedVideo(); ok {
		prevID = v.ID
	}
	var tabCmd tea.Cmd
	r, tabCmd = r.updateActiveTab(msg)
	var clearCmd tea.Cmd
	if v, ok := r.activeTab().SelectedVideo(); ok && v.ID != prevID {
		r, clearCmd = r.updatePanelOverlay(ovpkg.VideoClearMsg{})
	}
	var debounceCmd tea.Cmd
	r, debounceCmd = r.withSelectionDebounce()
	return r, tea.Batch(tabCmd, clearCmd, debounceCmd)
}

func (r Root) updateActiveTab(msg tea.Msg) (Root, tea.Cmd) {
	updated, cmd := r.tabs[r.activeIdx].Update(msg)
	r.tabs[r.activeIdx] = updated.(tuipkg.Tab)
	r.statusBar = r.statusBar.WithHints(r.tabHints())
	return r, cmd
}

func (r Root) tabHints() string {
	hints := r.activeTab().ShortHelp()
	parts := make([]string, 0, len(hints))
	for _, b := range hints {
		h := b.Help()
		if h.Key != "" && h.Desc != "" {
			parts = append(parts, h.Key+": "+h.Desc)
		}
	}
	return strings.Join(parts, "  ")
}

// updateTopOverlay / updatePanelOverlay are thin Root delegations to the overlay
// stack, kept so their many callers keep the (Root, tea.Cmd) shape; the stack
// mechanics live on overlayStack (H-2).
func (r Root) updateTopOverlay(msg tea.Msg) (Root, tea.Cmd) {
	return r, r.overlays.updateTop(msg)
}

func (r Root) updatePanelOverlay(msg tea.Msg) (Root, tea.Cmd) {
	return r, r.overlays.updatePanel(msg)
}

// reconcileOverlaysAfterTabSwitch runs after the active tab changes with overlays
// open. Every stacked modal belongs to the tab/video you just left (e.g. an
// Add-to-Playlist for a now-off-screen video), so leaving it floating over a
// different tab is confusing — close them all. The info side panel is the
// exception: it follows the active tab, so it stays open and re-tracks the new
// selection (VideoClearMsg + debounced reselect).
func (r Root) reconcileOverlaysAfterTabSwitch() (Root, tea.Cmd) {
	if len(r.overlays) == 0 {
		return r, nil
	}
	// Cull every non-panel overlay (centered modals reserve no width, so dropping
	// them needs no re-layout); the info panel, which does reserve width, stays.
	if !r.overlays.cullNonPanel() {
		return r, nil
	}
	// The info side panel stays open across tab switches and re-tracks the new
	// tab's selection: VideoClearMsg resets it to a clean loading state, then the
	// debounce reselects the new tab's current video (or nothing, on an empty tab).
	var clearCmd tea.Cmd
	r, clearCmd = r.updatePanelOverlay(ovpkg.VideoClearMsg{})
	var debounceCmd tea.Cmd
	r, debounceCmd = r.withSelectionDebounce()
	return r, tea.Batch(clearCmd, debounceCmd)
}

func (r Root) cycleTab(dir int) (Root, tea.Cmd) {
	n := len(r.tabs)
	r.activeIdx = ((r.activeIdx+dir)%n + n) % n
	r.tabBar = r.tabBar.WithActive(r.activeIdx)
	r.statusBar = r.statusBar.WithHints(r.tabHints())
	var refreshCmd tea.Cmd
	r, refreshCmd = r.refreshOnOpen()
	if r.activeTab().ID() == tuipkg.TabSearch {
		return r, tea.Batch(refreshCmd, func() tea.Msg { return tuipkg.SearchFocusInputMsg{} })
	}
	return r, refreshCmd
}

// refreshOnOpen nudges the just-activated tab to reload its DB-backed data
// immediately by forwarding a PollTickMsg, so switching to a tab shows fresh
// rows (streamed in by a background crawl/enrichment) without waiting up to one
// pollInterval for the next tick. It reuses each tab's existing lightweight
// PollTickMsg reload path, so it never flips the loading spinner.
func (r Root) refreshOnOpen() (Root, tea.Cmd) {
	return r.updateActiveTab(tuipkg.PollTickMsg{})
}

func (r Root) withSelectionDebounce() (Root, tea.Cmd) {
	v, ok := r.activeTab().SelectedVideo()
	if !ok {
		return r, nil
	}
	r.selectionDebounceToken++
	token := r.selectionDebounceToken
	cmd := tea.Tick(selectionDebounceDelay, func(_ time.Time) tea.Msg {
		return selectionDebounceMsg{token: token, video: v}
	})
	return r, cmd
}
