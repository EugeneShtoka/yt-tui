package overlay

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain/media"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// ── key handling ──────────────────────────────────────────────────────────────

func (vd VideoDetail) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch vd.subState {
	case vdLinks:
		return vd.handleLinksKey(msg)
	case vdChapters:
		return vd.handleChaptersKey(msg)
	case vdTranscript:
		return vd.handleTranscriptKey(msg)
	}
	return vd.handlePanelKey(msg)
}

func (vd VideoDetail) handlePanelKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := vd.keys
	if vs, ok := scrollKey(vd.descVS, len(vd.descLines), msg, keys); ok {
		vd.descVS = vs
		return vd, nil
	}
	switch {
	case key.Matches(msg, keys.Escape), key.Matches(msg, keys.Left), key.Matches(msg, keys.Quit):
		if vd.fetchCancel != nil {
			vd.fetchCancel() // closing the panel kills any in-flight fetch
		}
		closeCmds := []tea.Cmd{func() tea.Msg { return PopOverlayMsg{} }}
		if kittyCapable() && vd.thumbB64 != "" {
			closeCmds = append(closeCmds, tea.Raw(kittyDeleteSeq()))
		}
		return vd, tea.Batch(closeCmds...)

	case key.Matches(msg, keys.OpenLinks):
		return vd.openLinks()

	case key.Matches(msg, keys.OpenChapters):
		return vd.openChapters()

	case key.Matches(msg, keys.OpenTranscript):
		return vd.openTranscript()

	case key.Matches(msg, keys.Refresh):
		if vd.focused && vd.video != nil {
			vd.fetchToken++
			vd = vd.renewFetchCtx()
			return vd, vd.refreshCmd()
		}
	}
	return vd, nil
}

// openModal opens the requested sub-view inside the panel, reusing the same
// paths as the focused-panel key handlers. Root routes here (via OpenModalMsg)
// when a sub-view action arrives while the info panel is already open, so it
// never stacks a duplicate overlay.
func (vd VideoDetail) openModal(view InitialView) (VideoDetail, tea.Cmd) {
	switch view {
	case InitialViewLinks:
		return vd.openLinks()
	case InitialViewChapters:
		return vd.openChapters()
	case InitialViewTranscript:
		return vd.openTranscript()
	}
	return vd, nil
}

func (vd VideoDetail) openLinks() (VideoDetail, tea.Cmd) {
	if vd.video == nil {
		return vd, nil
	}
	var saveCmd tea.Cmd
	if !vd.linksLoaded {
		urls := media.ExtractLinks(vd.video.Description)
		vd.links, vd.linksLoaded = urls, true
		saveCmd = saveLinksCmd(vd.ctx, vd.backend, vd.video.ID, urls)
	}
	if len(vd.links) == 0 {
		return vd, tea.Batch(saveCmd, func() tea.Msg { return tuipkg.StatusMsg{Text: "no links in description"} })
	}
	vd.subState = vdLinks
	vd.linkSel = 0
	return vd, saveCmd
}

func (vd VideoDetail) openChapters() (VideoDetail, tea.Cmd) {
	if vd.chaptersLoaded && len(vd.chapters) > 0 {
		vd.subState = vdChapters
		vd.chapterSel = 0
		return vd, nil
	}
	return vd, func() tea.Msg { return tuipkg.StatusMsg{Text: "no chapters available"} }
}

// openTranscript opens the transcript modal, loading the transcript on first use
// (a separate backend fetch) and reusing it thereafter. The transcript fetch is
// independent of the panel's details load — it only needs the video ID/URL,
// which fetchVideo carries from construction — so it works even while the panel
// is still loading its details (vd.video == nil).
func (vd VideoDetail) openTranscript() (VideoDetail, tea.Cmd) {
	if vd.transcriptLoaded {
		vd.subState = vdTranscript
		vd.transcriptVS = 0
		return vd, nil
	}
	v := vd.fetchVideo
	if vd.video != nil {
		v = vd.video.Video
	}
	if v.ID == "" {
		return vd, nil // no video identity yet — nothing to fetch
	}
	// Miss: fetch off the event loop and show a loading popup meanwhile (the
	// panel renders it while subState is still vdPanel).
	vd.transcriptLoading = true
	return vd, vd.transcriptLoadCmd(v)
}

// dismissModal closes the currently-shown modal: it returns to the side panel
// when this overlay is the info panel, or pops the whole overlay when it is a
// standalone links/chapters modal (which has no panel to fall back to).
func (vd VideoDetail) dismissModal() (VideoDetail, tea.Cmd) {
	if vd.initialView != InitialViewPanel {
		if vd.fetchCancel != nil {
			vd.fetchCancel() // popping a standalone modal kills its in-flight fetch
		}
		return vd, func() tea.Msg { return PopOverlayMsg{} }
	}
	vd.subState = vdPanel
	// Returning to the panel must re-place the Kitty thumbnail: a modal's frame
	// redraw clears the image, and if it was loaded while a modal was up (e.g.
	// opening the transcript immediately after the panel) kittyCmd skipped the
	// placement because subState wasn't vdPanel. This is the single path back.
	return vd, vd.kittyAfterFrameCmd()
}

func (vd VideoDetail) handleLinksKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := vd.keys
	links := vd.links
	n := len(links)

	if newSel, consumed := vd.moveSelector(vd.linkSel, n, msg); consumed {
		vd.linkSel = newSel
		return vd, nil
	}
	switch {
	case key.Matches(msg, keys.Escape), key.Matches(msg, keys.Quit):
		return vd.dismissModal()
	case key.Matches(msg, keys.DrillDown):
		if n > 0 {
			openCmd := openURLCmd(links[vd.linkSel].URL)
			if vd.closeOnLinks {
				var dismissCmd tea.Cmd
				vd, dismissCmd = vd.dismissModal()
				return vd, tea.Batch(openCmd, dismissCmd)
			}
			return vd, openCmd
		}
	case key.Matches(msg, keys.CopyURL):
		if n > 0 {
			u := links[vd.linkSel].URL
			return vd, func() tea.Msg { return tuipkg.CopyURLMsg{URL: u} }
		}
	}
	return vd, nil
}

func (vd VideoDetail) handleChaptersKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := vd.keys
	chapters := vd.chapters
	n := len(chapters)

	if newSel, consumed := vd.moveSelector(vd.chapterSel, n, msg); consumed {
		vd.chapterSel = newSel
		return vd, nil
	}
	switch {
	case key.Matches(msg, keys.Escape), key.Matches(msg, keys.Quit):
		return vd.dismissModal()
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
			return vd, func() tea.Msg { return tuipkg.CopyURLMsg{URL: u} }
		}
	}
	return vd, nil
}

// handleTranscriptKey scrolls the transcript modal, jumps between chapter
// headers ([ / ]), and copies the full text.
func (vd VideoDetail) handleTranscriptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	keys := vd.keys
	if key.Matches(msg, keys.Escape) || key.Matches(msg, keys.Quit) {
		return vd.dismissModal()
	}
	if vs, ok := scrollKey(vd.transcriptVS, len(vd.transcriptWrapped()), msg, keys); ok {
		vd.transcriptVS = vs
		return vd, nil
	}
	switch {
	case key.Matches(msg, keys.NextChapter):
		for _, r := range transcriptHeaderRows(vd.transcriptWrapped()) {
			if r > vd.transcriptVS {
				vd.transcriptVS = r
				break
			}
		}
	case key.Matches(msg, keys.PrevChapter):
		target := 0
		for _, r := range transcriptHeaderRows(vd.transcriptWrapped()) {
			if r >= vd.transcriptVS {
				break
			}
			target = r
		}
		vd.transcriptVS = target
	case key.Matches(msg, keys.CopyTranscript):
		text := vd.transcriptText
		if text == "" {
			return vd, nil
		}
		return vd, func() tea.Msg { return tuipkg.CopyTextMsg{Text: text, Label: "transcript"} }
	}
	return vd, nil
}

// moveSelector handles Up/Down/GotoBottom for overlay lists.
func (vd VideoDetail) moveSelector(sel, n int, msg tea.KeyPressMsg) (newSel int, consumed bool) {
	return moveVertical(sel, n, msg, vd.keys, vd.circular, true)
}
