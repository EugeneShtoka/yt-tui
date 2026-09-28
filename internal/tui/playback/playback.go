// Package playback owns the video-playback lifecycle for the TUI: resolving a
// source, launching the player, recording history, and persisting resume
// position. It is a focused controller extracted from Root (H-2). Because it
// depends only on a narrow Backend plus the player device — not on the Bubble
// Tea Root — the same logic can be driven headlessly (e.g. a background
// position tracker that outlives the TUI).
package playback

import (
	"context"
	"fmt"
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

// maxPlayAttempts caps how many times a single play request is launched. YouTube
// hands out dead stream URLs about half the time (see failureSignature.retry), so
// one retry already turns a coin flip into a ~3-in-4 chance and two into ~7-in-8;
// past that the waiting costs more than the extra odds are worth.
const maxPlayAttempts = 3

// playRequest is everything needed to launch a video, carried through the session
// so a launch that died on a transient failure can be run again from the top. The
// retry has to start at the top: the point is to make yt-dlp extract a fresh
// stream URL, which only happens on a new player launch.
type playRequest struct {
	id        string
	url       string // fallback URL, used when the backend cannot resolve one
	title     string
	audioOnly bool
	eventType string
	attempt   int // 1 for the first launch
}

// streamed reports whether the request plays over the network. Only streams are
// retried: a local file that will not open is broken in a way a second launch
// cannot mend, and re-launching it would just stall the status line.
func (r playRequest) streamed() bool {
	return r.eventType == EvtStreamVideo || r.eventType == EvtStreamAudio
}

// shouldRetry decides whether a finished session has earned another launch.
func (r playRequest) shouldRetry(res player.Result) bool {
	return r.streamed() && r.attempt < maxPlayAttempts && retryable(res)
}

// withAttempts annotates a final failure with how many launches it consumed.
// The retries are silent, so without this the advice to try again reads as
// something the user has not had a chance to do, and one line of failure hides
// that three launches were already spent on it.
func (r playRequest) withAttempts(diag string) string {
	if diag == "" || r.attempt <= 1 {
		return diag
	}
	return diag + fmt.Sprintf(" (%d attempts)", r.attempt)
}

// StartedMsg signals the player process launched. The controller responds by
// scheduling the wait + position-tick commands; Root also reacts to it (status
// line + a history-changed refresh), so it is exported.
type StartedMsg struct {
	VideoID string
	Sess    *player.Session
	Text    string

	req playRequest // unexported: retry bookkeeping is the controller's business
}

// savePositionTickMsg drives periodic position saves for an active session, on
// the event loop rather than a detached goroutine.
type savePositionTickMsg struct {
	id   string
	sess *player.Session
}

// endedMsg reports a finished session back onto the event loop, carrying the
// diagnosis of a launch that never played (empty for an ordinary end), so the
// position refresh and the failure report are handled together. retry is set
// instead of diag when the failure is worth another launch, so the user sees one
// outcome rather than an error per attempt.
type endedMsg struct {
	diag  string
	retry *playRequest
}

// Controller owns the playback lifecycle. Construct with New. It holds no
// mutable state, so its methods are value receivers and it is safe to copy.
type Controller struct {
	ctx     context.Context //nolint:containedctx // app-lifetime context from main (H-1); Update takes none
	backend Backend
	player  player.Backend // may be nil when no player binary was found
	ytdlp   YtdlpInfo      // local yt-dlp, for explaining a launch that never plays
}

// New returns a playback Controller. ctx is the app-lifetime context threaded
// into every backend call (H-1). pl may be nil, in which case play attempts
// surface an error status instead of launching. ytdlp describes the local yt-dlp
// (zero value when it could not be read) and is only used to explain failures.
func New(ctx context.Context, backend Backend, pl player.Backend, ytdlp YtdlpInfo) Controller {
	return Controller{ctx: ctx, backend: backend, player: pl, ytdlp: ytdlp}
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
	case endedMsg:
		return c.handleEnded(m), true
	}
	return nil, false
}

func (c Controller) handlePlayVideo(m tuipkg.PlayVideoMsg) tea.Cmd {
	evt := EvtStreamVideo
	if m.AudioOnly {
		evt = EvtStreamAudio
	}
	return c.playCmd(playRequest{
		id: m.Video.ID, url: m.Video.URL, title: m.Video.Title,
		audioOnly: m.AudioOnly, eventType: evt, attempt: 1,
	})
}

func (c Controller) handleLaunchLocal(m tuipkg.LaunchLocalVideoMsg) tea.Cmd {
	lv := m.Video
	// For local videos, pass empty fallbackURL — InProc returns the file path,
	// Remote returns the daemon's /media/{id} URL.
	return c.playCmd(playRequest{id: lv.ID, title: lv.Title, eventType: EvtPlayVideo, attempt: 1})
}

func (c Controller) handleStarted(m StartedMsg) tea.Cmd {
	req := m.req
	if req.id == "" {
		req.id = m.VideoID // a StartedMsg synthesized without a request (tests)
	}
	return tea.Batch(
		func() tea.Msg { return tuipkg.StatusMsg{Text: m.Text} },
		func() tea.Msg { return tuipkg.HistoryChangedMsg{} },
		c.waitCmd(req, m.Sess),
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
func (c Controller) playCmd(req playRequest) tea.Cmd {
	return func() tea.Msg {
		if c.player == nil {
			return tuipkg.StatusMsg{Text: "no video player found — install mpv or vlc", IsErr: true}
		}
		// A retry must not hijack a video the user started in the meantime: the
		// relaunch would kill it to take the player for itself. Abandon quietly —
		// the user has already moved on, and the failure they left behind is not
		// worth a status line. (No-op on the simple backend, which reports no
		// active playback at all; MPRIS is the default.)
		if req.attempt > 1 {
			if _, playing := c.player.Active(); playing {
				return nil
			}
		}
		id, title := req.id, req.title
		src, resolveErr := c.backend.ResolveSource(c.ctx, id, req.url)
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
		if req.audioOnly {
			sess, launchErr = c.player.LaunchAudio(id, src.URI, title, pos)
		} else {
			sess, launchErr = c.player.Launch(id, src.URI, title, pos)
		}
		if launchErr != nil {
			return tuipkg.StatusMsg{Text: "player: " + launchErr.Error(), IsErr: true}
		}
		// History records the intent to watch, so it is written once for the
		// request rather than once per launch attempt.
		if req.attempt == 1 {
			_ = c.backend.AddHistory(c.ctx, id, req.eventType, "")
		}
		return StartedMsg{VideoID: id, Sess: sess, Text: "Playing: " + render.Truncate(title, 60), req: req}
	}
}

// waitCmd blocks until the player process exits, saves the final position, then
// reports the session's end so tabs refresh their playback progress. It also asks
// the session how the process died: a player that exited without ever playing is
// the "video just doesn't start" symptom, and its captured stderr is the only
// place the reason (usually yt-dlp's) exists.
func (c Controller) waitCmd(req playRequest, sess *player.Session) tea.Cmd {
	id := req.id
	return func() tea.Msg {
		<-sess.Done()
		if p, _ := sess.Position(); p > 0 {
			_ = c.backend.SaveVideoPosition(c.ctx, id, p.Milliseconds())
		}
		res, ok := sess.Result()
		if !ok {
			return endedMsg{} // still running, or deliberately superseded
		}
		diag := diagnose(res, c.ytdlp)
		if diag != "" {
			debug.Log("playback %s: exit %d after %s; output: %s", id, res.ExitCode, res.Ran, res.Output)
		}
		if req.shouldRetry(res) {
			next := req
			next.attempt++
			return endedMsg{retry: &next}
		}
		return endedMsg{diag: req.withAttempts(diag)}
	}
}

// handleEnded closes out a session: tabs refresh their progress, and a failed
// launch is reported on the status line instead of vanishing silently.
func (c Controller) handleEnded(m endedMsg) tea.Cmd {
	refresh := func() tea.Msg { return tuipkg.RefreshPositionsMsg{} }
	if m.retry != nil {
		// Say so rather than appearing to hang: a fresh extraction takes a few
		// seconds, and silence there reads as the keypress having been ignored.
		return tea.Batch(refresh, func() tea.Msg {
			return tuipkg.StatusMsg{Text: fmt.Sprintf("Stream refused — retrying (%d/%d)…",
				m.retry.attempt, maxPlayAttempts)}
		}, c.playCmd(*m.retry))
	}
	if m.diag == "" {
		return refresh
	}
	return tea.Batch(refresh, func() tea.Msg {
		return tuipkg.StatusMsg{Text: m.diag, IsErr: true}
	})
}
