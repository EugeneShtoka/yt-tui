// Package playback owns the video-playback lifecycle for the TUI: resolving a
// source, launching the player, recording history, and persisting resume
// position. It is a focused controller extracted from Root (H-2). Because it
// depends only on a narrow Backend plus the player device — not on the Bubble
// Tea Root — the same logic can be driven headlessly (e.g. a background
// position tracker that outlives the TUI).
package playback

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/device/player"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
)

// History event types — stored in the DB; must match the schema strings.
const (
	EvtStreamVideo = "streamVideo"
	EvtStreamAudio = "streamAudio"
	EvtPlayVideo   = "playVideo"
	EvtPlayAudio   = "playAudio"
)

// savePositionInterval is how often an active session's position is persisted.
const savePositionInterval = 5 * time.Second

// Backend is the narrow slice of the app backend the playback controller needs:
// source resolution, resume-position read/write, and history. Declared
// consumer-side (ISP); api.Backend satisfies it.
type Backend interface {
	ResolveSource(ctx context.Context, videoID, fallbackURL string) (api.PlayableSource, error)
	VideoPosition(ctx context.Context, videoID string) (int64, bool, error)
	SaveVideoPosition(ctx context.Context, videoID string, ms int64) error
	AddHistory(ctx context.Context, videoID, eventType, extra string) error
}

// StartedMsg signals the player process launched. The controller responds by
// scheduling the wait + position-tick commands; Root also reacts to it (status
// line + a history-changed refresh), so it is exported.
type StartedMsg struct {
	VideoID string
	Sess    *player.Session
	Text    string
}

// savePositionTickMsg drives periodic position saves for an active session, on
// the event loop rather than a detached goroutine.
type savePositionTickMsg struct {
	id   string
	sess *player.Session
}

// Controller owns the playback lifecycle. Construct with New. It holds no
// mutable state, so its methods are value receivers and it is safe to copy.
type Controller struct {
	ctx     context.Context
	backend Backend
	player  player.Backend // may be nil when no player binary was found
}

// New returns a playback Controller. ctx is the app-lifetime context threaded
// into every backend call (H-1). pl may be nil, in which case play attempts
// surface an error status instead of launching.
func New(ctx context.Context, backend Backend, pl player.Backend) Controller {
	return Controller{ctx: ctx, backend: backend, player: pl}
}

// Update handles the playback-related messages and reports whether it consumed
// the message. It never mutates caller state — it only produces commands — so
// Root can delegate without threading its model through.
func (c Controller) Update(msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case tuipkg.PlayVideoMsg:
		return c.handlePlayVideo(m), true
	case tuipkg.LaunchLocalVideoMsg:
		return c.handleLaunchLocal(m), true
	case StartedMsg:
		return c.handleStarted(m), true
	case savePositionTickMsg:
		return c.handleSavePositionTick(m), true
	}
	return nil, false
}

func (c Controller) handlePlayVideo(m tuipkg.PlayVideoMsg) tea.Cmd {
	evt := EvtStreamVideo
	if m.AudioOnly {
		evt = EvtStreamAudio
	}
	return c.playCmd(m.Video.ID, m.Video.URL, m.Video.Title, m.AudioOnly, evt)
}

func (c Controller) handleLaunchLocal(m tuipkg.LaunchLocalVideoMsg) tea.Cmd {
	lv := m.Video
	// For local videos, pass empty fallbackURL — InProc returns the file path,
	// Remote returns the daemon's /media/{id} URL.
	return c.playCmd(lv.ID, "", lv.Title, false, EvtPlayVideo)
}

func (c Controller) handleStarted(m StartedMsg) tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return tuipkg.StatusMsg{Text: m.Text} },
		func() tea.Msg { return tuipkg.HistoryChangedMsg{} },
		c.waitCmd(m.VideoID, m.Sess),
		c.savePositionTickCmd(m.VideoID, m.Sess),
	)
}

func (c Controller) handleSavePositionTick(m savePositionTickMsg) tea.Cmd {
	select {
	case <-m.sess.Done():
		return nil // session ended; waitCmd handles the final save
	default:
	}
	id, sess := m.id, m.sess
	saveCmd := func() tea.Msg {
		if p, _ := sess.Position(); p > 0 {
			_ = c.backend.SaveVideoPosition(c.ctx, id, p.Milliseconds())
		}
		return nil
	}
	return tea.Batch(saveCmd, c.savePositionTickCmd(id, sess))
}

// savePositionTickCmd schedules the next position-save tick for a session.
func (c Controller) savePositionTickCmd(id string, sess *player.Session) tea.Cmd {
	return tea.Tick(savePositionInterval, func(_ time.Time) tea.Msg {
		return savePositionTickMsg{id: id, sess: sess}
	})
}

// PlayCmd is the entry point for starting playback: it resolves the video's
// source, looks up its resume position, launches the player, records history,
// and returns a StartedMsg (or an error StatusMsg). Periodic saves are driven on
// the event loop via savePositionTickMsg, so no detached goroutine writes
// positions off-loop.
func (c Controller) playCmd(id, fallbackURL, title string, audioOnly bool, eventType string) tea.Cmd {
	return func() tea.Msg {
		if c.player == nil {
			return tuipkg.StatusMsg{Text: "no video player found — install mpv or vlc", IsErr: true}
		}
		src, resolveErr := c.backend.ResolveSource(c.ctx, id, fallbackURL)
		if resolveErr != nil {
			return tuipkg.StatusMsg{Text: "resolve source: " + resolveErr.Error(), IsErr: true}
		}
		posMs, _, posErr := c.backend.VideoPosition(c.ctx, id)
		if posErr != nil {
			// Resume-position lookup failed (DB or, in remote mode, transport
			// error) — fall back to starting from 0 rather than blocking playback,
			// but keep the failure observable. (H-8)
			debug.Log("playCmd: VideoPosition(%s): %v", id, posErr)
		}
		pos := time.Duration(posMs) * time.Millisecond
		var sess *player.Session
		var launchErr error
		if audioOnly {
			sess, launchErr = c.player.LaunchAudio(id, src.URI, title, pos)
		} else {
			sess, launchErr = c.player.Launch(id, src.URI, title, pos)
		}
		if launchErr != nil {
			return tuipkg.StatusMsg{Text: "player: " + launchErr.Error(), IsErr: true}
		}
		_ = c.backend.AddHistory(c.ctx, id, eventType, "")
		return StartedMsg{VideoID: id, Sess: sess, Text: "Playing: " + render.Truncate(title, 60)}
	}
}

// waitCmd blocks until the player process exits, saves the final position, then
// triggers a UI refresh so tabs show the updated playback progress.
func (c Controller) waitCmd(id string, sess *player.Session) tea.Cmd {
	return func() tea.Msg {
		<-sess.Done()
		if p, _ := sess.Position(); p > 0 {
			_ = c.backend.SaveVideoPosition(c.ctx, id, p.Milliseconds())
		}
		return tuipkg.RefreshPositionsMsg{}
	}
}
