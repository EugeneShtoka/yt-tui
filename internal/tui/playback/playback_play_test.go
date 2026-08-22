package playback

import (
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/device/player"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// playFake is a Backend that records the playCmd interactions and lets a test
// inject errors and a resume position.
type playFake struct {
	fakeBackend // embeds SaveVideoPosition recorder

	resolveURI  string
	resolveErr  error
	posMs       int64
	posErr      error
	historyID   string
	historyEvt  string
	historyDone bool
}

func (f *playFake) ResolveSource(context.Context, string, string) (api.PlayableSource, error) {
	return api.PlayableSource{URI: f.resolveURI}, f.resolveErr
}
func (f *playFake) VideoPosition(context.Context, string) (int64, bool, error) {
	return f.posMs, f.posMs > 0, f.posErr
}
func (f *playFake) AddHistory(_ context.Context, id, evt, _ string) error {
	f.historyID, f.historyEvt, f.historyDone = id, evt, true
	return nil
}

// fakePlayer implements player.Backend, recording launch calls and returning a
// canned session so no real player process is spawned.
type fakePlayer struct {
	launched      bool
	audioLaunched bool
	gotID         string
	gotSource     string
	gotStartAt    time.Duration
	sess          *player.Session
	err           error
}

func (p *fakePlayer) Launch(id, source, _ string, startAt time.Duration) (*player.Session, error) {
	p.launched, p.gotID, p.gotSource, p.gotStartAt = true, id, source, startAt
	return p.sess, p.err
}
func (p *fakePlayer) LaunchAudio(id, source, _ string, startAt time.Duration) (*player.Session, error) {
	p.audioLaunched, p.gotID, p.gotSource, p.gotStartAt = true, id, source, startAt
	return p.sess, p.err
}
func (p *fakePlayer) Active() (player.ActivePlayback, bool) { return player.ActivePlayback{}, false }
func (p *fakePlayer) Close()                                {}

func TestUpdate_IgnoresUnknownMsg(t *testing.T) {
	c := New(context.Background(), &fakeBackend{}, nil, YtdlpInfo{})
	if cmd, ok := c.Update(struct{ tea.Msg }{}); ok || cmd != nil {
		t.Errorf("Update consumed an unrelated msg: ok=%v cmd=%v", ok, cmd)
	}
}

func TestPlayVideo_ResolvesLaunchesAndRecordsHistory(t *testing.T) {
	be := &playFake{resolveURI: "file:///v.mp4", posMs: 12_000}
	pl := &fakePlayer{sess: player.NewSession(0)}
	c := New(context.Background(), be, pl, YtdlpInfo{})

	cmd, ok := c.Update(tuipkg.PlayVideoMsg{Video: domain.Video{ID: "v1", URL: "https://y/v1", Title: "My Video"}})
	if !ok {
		t.Fatal("Update did not handle PlayVideoMsg")
	}
	msg := runCmd(cmd)
	started, isStarted := msg.(StartedMsg)
	if !isStarted {
		t.Fatalf("want StartedMsg, got %#v", msg)
	}
	if started.VideoID != "v1" {
		t.Errorf("StartedMsg.VideoID = %q, want v1", started.VideoID)
	}
	if !pl.launched || pl.audioLaunched {
		t.Errorf("expected video Launch (not audio): launched=%v audio=%v", pl.launched, pl.audioLaunched)
	}
	if pl.gotSource != "file:///v.mp4" {
		t.Errorf("player got source %q, want the resolved URI", pl.gotSource)
	}
	if pl.gotStartAt != 12*time.Second {
		t.Errorf("player startAt = %v, want 12s resume position", pl.gotStartAt)
	}
	if !be.historyDone || be.historyID != "v1" || be.historyEvt != EvtStreamVideo {
		t.Errorf("history not recorded correctly: done=%v id=%q evt=%q", be.historyDone, be.historyID, be.historyEvt)
	}
}

func TestPlayVideo_AudioOnlyUsesLaunchAudioAndEvent(t *testing.T) {
	be := &playFake{resolveURI: "https://y/v1"}
	pl := &fakePlayer{sess: player.NewSession(0)}
	c := New(context.Background(), be, pl, YtdlpInfo{})

	cmd, _ := c.Update(tuipkg.PlayVideoMsg{Video: domain.Video{ID: "v1", Title: "T"}, AudioOnly: true})
	if _, ok := runCmd(cmd).(StartedMsg); !ok {
		t.Fatalf("want StartedMsg for audio play")
	}
	if !pl.audioLaunched || pl.launched {
		t.Errorf("expected LaunchAudio: audio=%v video=%v", pl.audioLaunched, pl.launched)
	}
	if be.historyEvt != EvtStreamAudio {
		t.Errorf("history event = %q, want %q", be.historyEvt, EvtStreamAudio)
	}
}

func TestLaunchLocal_PlaysLocalFileWithPlayEvent(t *testing.T) {
	be := &playFake{resolveURI: "file:///local.mp4"}
	pl := &fakePlayer{sess: player.NewSession(0)}
	c := New(context.Background(), be, pl, YtdlpInfo{})

	cmd, ok := c.Update(tuipkg.LaunchLocalVideoMsg{Video: domain.LocalVideo{ID: "l1", Title: "Local"}})
	if !ok {
		t.Fatal("Update did not handle LaunchLocalVideoMsg")
	}
	started, isStarted := runCmd(cmd).(StartedMsg)
	if !isStarted {
		t.Fatalf("want StartedMsg, got %#v", runCmd(cmd))
	}
	if started.VideoID != "l1" || be.historyEvt != EvtPlayVideo {
		t.Errorf("local play wrong: id=%q evt=%q", started.VideoID, be.historyEvt)
	}
}

func TestPlayCmd_NoPlayerReturnsError(t *testing.T) {
	c := New(context.Background(), &playFake{}, nil, YtdlpInfo{}) // nil player
	cmd, _ := c.Update(tuipkg.PlayVideoMsg{Video: domain.Video{ID: "v1"}})
	st, ok := runCmd(cmd).(tuipkg.StatusMsg)
	if !ok || !st.IsErr {
		t.Fatalf("want error StatusMsg when no player, got %#v", runCmd(cmd))
	}
}

func TestPlayCmd_ResolveErrorReturnsError(t *testing.T) {
	be := &playFake{resolveErr: errors.New("resolve boom")}
	pl := &fakePlayer{sess: player.NewSession(0)}
	c := New(context.Background(), be, pl, YtdlpInfo{})

	cmd, _ := c.Update(tuipkg.PlayVideoMsg{Video: domain.Video{ID: "v1"}})
	st, ok := runCmd(cmd).(tuipkg.StatusMsg)
	if !ok || !st.IsErr {
		t.Fatalf("want error StatusMsg on resolve failure, got %#v", runCmd(cmd))
	}
	if pl.launched {
		t.Error("player should not launch when source resolution fails")
	}
}

func TestPlayCmd_LaunchErrorReturnsError(t *testing.T) {
	be := &playFake{resolveURI: "u"}
	pl := &fakePlayer{err: errors.New("launch boom")}
	c := New(context.Background(), be, pl, YtdlpInfo{})

	cmd, _ := c.Update(tuipkg.PlayVideoMsg{Video: domain.Video{ID: "v1"}})
	st, ok := runCmd(cmd).(tuipkg.StatusMsg)
	if !ok || !st.IsErr {
		t.Fatalf("want error StatusMsg on launch failure, got %#v", runCmd(cmd))
	}
	if be.historyDone {
		t.Error("history must not be recorded when launch fails")
	}
}

// TestWaitCmd_SavesFinalPositionOnExit drives waitCmd via handleStarted: once the
// session finishes, it saves the final position and reports the session's end.
// A session with no recorded outcome carries no diagnosis.
func TestWaitCmd_SavesFinalPositionOnExit(t *testing.T) {
	be := &playFake{}
	c := New(context.Background(), be, nil, YtdlpInfo{})
	sess := player.NewSession(45 * time.Second) // position 45s

	// handleStarted batches [status, history, wait, tick]; run only the wait cmd.
	cmd := c.handleStarted(StartedMsg{VideoID: "v1", Sess: sess})
	batch, ok := runCmd(cmd).(tea.BatchMsg)
	if !ok || len(batch) != 4 {
		t.Fatalf("handleStarted must batch 4 cmds, got %#v", runCmd(cmd))
	}

	sess.Finish() // player exits
	msg := runCmd(batch[2])
	ended, ok := msg.(endedMsg)
	if !ok {
		t.Fatalf("waitCmd should return endedMsg, got %#v", msg)
	}
	if ended.diag != "" {
		t.Errorf("a session with no recorded outcome must carry no diagnosis, got %q", ended.diag)
	}
	if _, ok := runCmd(c.handleEnded(ended)).(tuipkg.RefreshPositionsMsg); !ok {
		t.Error("an ordinary session end must refresh positions")
	}
	if be.calls != 1 || be.savedID != "v1" || be.savedMs != 45_000 {
		t.Errorf("final save wrong: calls=%d id=%q ms=%d", be.calls, be.savedID, be.savedMs)
	}
}
