package playback

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/device/player"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// botOutput is what mpv really prints when yt-dlp is refused anonymously.
const botOutput = "[ytdl_hook] ERROR: [youtube] abc: Sign in to confirm you’re not a bot.\n" +
	"[ytdl_hook] youtube-dl failed: unexpected error occurred\n"

// forbiddenOutput is the dead-stream-URL failure this retry exists for: ffmpeg
// gets a 403 on a URL yt-dlp resolved seconds earlier.
const forbiddenOutput = "[ffmpeg] https: HTTP error 403 Forbidden\n" +
	"Exiting... (Errors when loading file)\n"

// collect runs a command and returns every leaf message it produced, flattening
// one level of tea.Batch (enough for these handlers).
func collect(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	msg := runCmd(cmd)
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, runCmd(c))
	}
	return out
}

func failedRes(output string) player.Result {
	return player.Result{ExitCode: 2, Ran: 2 * time.Second, Output: output}
}

// TestRetryableOnlyForTransientFailures: a fresh extraction fixes a dead stream
// URL, but it cannot conjure cookies or undelete a video, so those must not spin.
func TestRetryableOnlyForTransientFailures(t *testing.T) {
	tests := []struct {
		name string
		res  player.Result
		want bool
	}{
		{"403 on a resolved URL", failedRes(forbiddenOutput), true},
		{"extraction failed", failedRes("ERROR: [youtube] abc: nsig extraction failed"), true},
		{"ytdl_hook gave up", failedRes("[ytdl_hook] youtube-dl failed: unexpected error occurred"), true},
		{"bot verification", failedRes(botOutput), false},
		{"video unavailable", failedRes("ERROR: [youtube] abc: Video unavailable"), false},
		{"network down", failedRes("ERROR: unable to download webpage: name resolution"), false},
		{"unrecognized failure", failedRes("ERROR: something entirely new"), false},
		{"nothing captured", player.Result{ExitCode: 2, Ran: time.Second}, false},
		{"it actually played", player.Result{ExitCode: 1, Played: true, Output: forbiddenOutput}, false},
		{"clean exit", player.Result{ExitCode: 0, Output: forbiddenOutput}, false},
	}
	for _, tt := range tests {
		if got := retryable(tt.res); got != tt.want {
			t.Errorf("%s: retryable = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// TestShouldRetryStopsAtLimit: the retry is bounded, and only streams qualify.
func TestShouldRetryStopsAtLimit(t *testing.T) {
	res := failedRes(forbiddenOutput)
	stream := func(attempt int) playRequest {
		return playRequest{id: "v1", eventType: EvtStreamVideo, attempt: attempt}
	}
	for attempt := 1; attempt < maxPlayAttempts; attempt++ {
		if !stream(attempt).shouldRetry(res) {
			t.Errorf("attempt %d should retry", attempt)
		}
	}
	if stream(maxPlayAttempts).shouldRetry(res) {
		t.Errorf("attempt %d exceeded maxPlayAttempts and must not retry", maxPlayAttempts)
	}
	// A local file gets no retry however transient the error looks.
	for _, evt := range []string{EvtPlayVideo, EvtPlayAudio} {
		local := playRequest{id: "v1", eventType: evt, attempt: 1}
		if local.shouldRetry(res) {
			t.Errorf("local playback (%s) must not be retried", evt)
		}
	}
	// Audio streams are retried too — they hit the same dead URLs.
	if !(playRequest{id: "v1", eventType: EvtStreamAudio, attempt: 1}).shouldRetry(res) {
		t.Error("audio stream should retry")
	}
}

// TestHandleEndedRelaunches: a retry re-enters playCmd, which must launch the
// player again — with the same video and audio/video choice — and say so rather
// than leaving the user staring at an unchanged screen.
func TestHandleEndedRelaunches(t *testing.T) {
	be := &playFake{resolveURI: "https://y/v1"}
	pl := &fakePlayer{sess: player.NewSession(0)}
	c := New(context.Background(), be, pl, YtdlpInfo{})

	req := playRequest{id: "v1", url: "https://y/v1", title: "T", audioOnly: true,
		eventType: EvtStreamAudio, attempt: 2}
	msgs := collect(t, c.handleEnded(endedMsg{retry: &req}))

	if !pl.audioLaunched || pl.gotID != "v1" {
		t.Errorf("retry did not relaunch audio for v1: launched=%v id=%q", pl.audioLaunched, pl.gotID)
	}
	var toldUser bool
	for _, m := range msgs {
		if s, ok := m.(tuipkg.StatusMsg); ok && strings.Contains(s.Text, "retrying") {
			toldUser = true
			if s.IsErr {
				t.Error("a retry in progress is not yet an error")
			}
		}
	}
	if !toldUser {
		t.Errorf("retry did not report itself on the status line: %#v", msgs)
	}
}

// TestHandleEndedReportsWhenNotRetrying: with no retry left the diagnosis has to
// reach the status line as an error, or the failure vanishes silently.
func TestHandleEndedReportsWhenNotRetrying(t *testing.T) {
	c := New(context.Background(), &playFake{}, &fakePlayer{}, YtdlpInfo{})
	msgs := collect(t, c.handleEnded(endedMsg{diag: "Playback failed: nope"}))
	for _, m := range msgs {
		if s, ok := m.(tuipkg.StatusMsg); ok {
			if !s.IsErr || !strings.Contains(s.Text, "nope") {
				t.Errorf("status = %+v, want the diagnosis as an error", s)
			}
			return
		}
	}
	t.Errorf("no status reported: %#v", msgs)
}

// TestRetryDoesNotDuplicateHistory: history records the intent to watch, so a
// video retried three times must still appear once.
func TestRetryDoesNotDuplicateHistory(t *testing.T) {
	be := &playFake{resolveURI: "https://y/v1"}
	pl := &fakePlayer{sess: player.NewSession(0)}
	c := New(context.Background(), be, pl, YtdlpInfo{})

	req := playRequest{id: "v1", eventType: EvtStreamVideo, attempt: 2}
	runCmd(c.playCmd(req))
	if be.historyDone {
		t.Error("a retry attempt must not add a second history entry")
	}
	req.attempt = 1
	runCmd(c.playCmd(req))
	if !be.historyDone {
		t.Error("the first attempt must add the history entry")
	}
}

// activePlayer reports an in-progress playback, standing in for the user having
// started another video while a retry was pending.
type activePlayer struct {
	fakePlayer
	active string
}

func (p *activePlayer) Active() (player.ActivePlayback, bool) {
	if p.active == "" {
		return player.ActivePlayback{}, false
	}
	return player.ActivePlayback{VideoID: p.active, PID: 42}, true
}

// TestRetryAbandonedWhenSomethingElseIsPlaying: relaunching would kill the video
// the user just started, so a retry that finds a live player gives up instead.
func TestRetryAbandonedWhenSomethingElseIsPlaying(t *testing.T) {
	be := &playFake{resolveURI: "https://y/v1"}
	pl := &activePlayer{fakePlayer: fakePlayer{sess: player.NewSession(0)}, active: "v2"}
	c := New(context.Background(), be, pl, YtdlpInfo{})

	if msg := runCmd(c.playCmd(playRequest{id: "v1", eventType: EvtStreamVideo, attempt: 2})); msg != nil {
		t.Errorf("retry returned %#v, want nil (abandoned)", msg)
	}
	if pl.launched {
		t.Error("retry hijacked the playback the user had already started")
	}

	// With nothing playing, the same retry goes ahead.
	pl.active = ""
	if msg := runCmd(c.playCmd(playRequest{id: "v1", eventType: EvtStreamVideo, attempt: 2})); msg == nil {
		t.Error("retry abandoned even though no player was active")
	}
	if !pl.launched {
		t.Error("retry did not launch with an idle player")
	}
}

// TestFirstAttemptIgnoresActivePlayer: starting a video while another plays is
// the ordinary "switch video" path and must still take over the player.
func TestFirstAttemptIgnoresActivePlayer(t *testing.T) {
	pl := &activePlayer{fakePlayer: fakePlayer{sess: player.NewSession(0)}, active: "v2"}
	c := New(context.Background(), &playFake{resolveURI: "https://y/v1"}, pl, YtdlpInfo{})
	if msg := runCmd(c.playCmd(playRequest{id: "v1", eventType: EvtStreamVideo, attempt: 1})); msg == nil {
		t.Fatal("a first attempt must not be blocked by an active player")
	}
	if !pl.launched {
		t.Error("first attempt did not launch")
	}
}
