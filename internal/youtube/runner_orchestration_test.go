package youtube

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/procexec"
)

// stubSleep replaces the backoff sleep for the duration of a test so the
// rate-limit retry loop runs instantly.
func stubSleep(t *testing.T) {
	t.Helper()
	orig := retrySleep
	retrySleep = func(context.Context, time.Duration) {}
	t.Cleanup(func() { retrySleep = orig })
}

func videoLine(id string) string {
	return fmt.Sprintf(`{"id":%q,"title":%q,"view_count":10}`, id, "T-"+id)
}

// runYtdlp must parse stdout, capture stderr, and (via waitErr) fold an
// output-less failed exit into an error.
func TestRunYtdlpParsesAndCapturesStderr(t *testing.T) {
	r := procexec.FakeRunner{New: func([]string) procexec.Cmd {
		return &procexec.FakeCmd{Stdout: videoLine("a") + "\n" + videoLine("b"), Stderr: "a warning"}
	}}
	got, raw, stderr, err := runYtdlp(context.Background(), r, nil, parseVideoLines)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 2 || raw != 2 {
		t.Fatalf("got %d videos (raw %d), want 2", len(got), raw)
	}
	if !strings.Contains(stderr, "a warning") {
		t.Errorf("stderr not captured: %q", stderr)
	}
}

func TestRunYtdlpFoldsEmptyExitFailure(t *testing.T) {
	r := procexec.FakeRunner{New: func([]string) procexec.Cmd {
		return &procexec.FakeCmd{Stdout: "", WaitErr: fmt.Errorf("exit status 1")}
	}}
	_, _, _, err := runYtdlp(context.Background(), r, nil, parseVideoLines)
	if err == nil {
		t.Fatal("want error for failed exit with no output, got nil")
	}
	if !strings.Contains(err.Error(), "without output") {
		t.Errorf("err = %v, want 'without output'", err)
	}
}

// A non-zero exit that still produced output is a partial success (H-2).
func TestRunYtdlpPartialSuccess(t *testing.T) {
	r := procexec.FakeRunner{New: func([]string) procexec.Cmd {
		return &procexec.FakeCmd{Stdout: videoLine("a"), WaitErr: fmt.Errorf("exit status 1")}
	}}
	got, _, _, err := runYtdlp(context.Background(), r, nil, parseVideoLines)
	if err != nil {
		t.Fatalf("partial success must not error, got %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 video, got %d", len(got))
	}
}

// runWithRetry retries while stderr reports a rate limit, then returns the last
// attempt's result once the retry budget is exhausted.
func TestRunWithRetryExhaustsOnPersistentRateLimit(t *testing.T) {
	stubSleep(t)
	var calls int32
	r := procexec.FakeRunner{New: func([]string) procexec.Cmd {
		atomic.AddInt32(&calls, 1)
		return &procexec.FakeCmd{Stdout: "", Stderr: "HTTP Error 429: Too Many Requests"}
	}}
	_, _, _ = runWithRetry(context.Background(), r, "video", nil, parseVideoLines)
	if got := atomic.LoadInt32(&calls); got != maxRetries+1 {
		t.Errorf("want %d attempts, got %d", maxRetries+1, got)
	}
}

// A transient rate limit that clears on the second attempt yields the video.
func TestRunWithRetryRecoversAfterRateLimit(t *testing.T) {
	stubSleep(t)
	var calls int32
	r := procexec.FakeRunner{New: func([]string) procexec.Cmd {
		if atomic.AddInt32(&calls, 1) == 1 {
			return &procexec.FakeCmd{Stderr: "rate-limited"}
		}
		return &procexec.FakeCmd{Stdout: videoLine("ok")}
	}}
	got, _, err := runWithRetry(context.Background(), r, "video", nil, parseVideoLines)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ok" {
		t.Fatalf("want single video 'ok', got %+v", got)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("want 2 attempts, got %d", calls)
	}
}

// End-to-end through a Client method: injected runner → parse → strip-emoji.
func TestClientRecommendedUsesRunner(t *testing.T) {
	c := &Client{
		cfg:    &config.Config{},
		runner: procexec.FakeRunner{New: func([]string) procexec.Cmd { return &procexec.FakeCmd{Stdout: videoLine("x")} }},
	}
	got, err := c.Recommended(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 1 || got[0].ID != "x" {
		t.Fatalf("want single video 'x', got %+v", got)
	}
}

// Pagination must stop once a page returns fewer than pageSize raw entries.
func TestClientPaginationStops(t *testing.T) {
	var page int32
	c := &Client{
		cfg: &config.Config{},
		runner: procexec.FakeRunner{New: func([]string) procexec.Cmd {
			// First page: a full pageSize of videos → loop continues.
			// Second page: a short page → loop terminates.
			if atomic.AddInt32(&page, 1) == 1 {
				var b strings.Builder
				for i := range pageSize {
					b.WriteString(videoLine(fmt.Sprintf("p1-%d", i)))
					b.WriteByte('\n')
				}
				return &procexec.FakeCmd{Stdout: b.String()}
			}
			return &procexec.FakeCmd{Stdout: videoLine("last")}
		}},
	}
	got, err := c.ChannelVideos(context.Background(), "https://youtube.com/@x", "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p := atomic.LoadInt32(&page); p != 2 {
		t.Fatalf("want 2 pages fetched, got %d", p)
	}
	if len(got) != pageSize+1 {
		t.Fatalf("want %d videos, got %d", pageSize+1, len(got))
	}
}

// H-10: a canceled context must stop the retry loop instead of exhausting all
// maxRetries+1 attempts — without this, ctx cancellation (RPC disconnect,
// daemon shutdown) had no effect on an in-progress rate-limit backoff.
func TestRunWithRetryStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls int32
	r := procexec.FakeRunner{New: func([]string) procexec.Cmd {
		atomic.AddInt32(&calls, 1)
		return &procexec.FakeCmd{Stderr: "HTTP Error 429: Too Many Requests"}
	}}
	_, _, err := runWithRetry(ctx, r, "video", nil, parseVideoLines)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("want exactly 1 attempt before bailing on cancellation, got %d", got)
	}
}

// captureCtxRunner records the ctx passed to Command so tests can assert it is
// the caller's ctx and not context.Background() (H-10).
type captureCtxRunner struct {
	got context.Context
	cmd procexec.Cmd
}

func (r *captureCtxRunner) Command(ctx context.Context, _ string, _ ...string) procexec.Cmd {
	r.got = ctx
	return r.cmd
}

// H-10: runYtdlp must forward the caller's context to the runner instead of
// hardcoding context.Background(), so canceling it actually kills yt-dlp.
func TestRunYtdlpForwardsCallerContext(t *testing.T) {
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
	r := &captureCtxRunner{cmd: &procexec.FakeCmd{Stdout: videoLine("a")}}
	if _, _, _, err := runYtdlp(ctx, r, nil, parseVideoLines); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.got == nil || r.got.Value(ctxKey{}) != "marker" {
		t.Fatal("runYtdlp did not forward the caller's context to the runner")
	}
}

// H-10: runYtdlpOutput (VideoDetails' seam) must surface captured stderr on a
// failed exit instead of just the bare exit status — the bug was cmd.Output()'s
// ExitError.Stderr being silently discarded.
func TestRunYtdlpOutputSurfacesStderrOnFailure(t *testing.T) {
	r := procexec.FakeRunner{New: func([]string) procexec.Cmd {
		return &procexec.FakeCmd{Stderr: "ERROR: Video unavailable", WaitErr: fmt.Errorf("exit status 1")}
	}}
	_, err := runYtdlpOutput(context.Background(), r, nil)
	if err == nil || !strings.Contains(err.Error(), "Video unavailable") {
		t.Fatalf("want error containing stderr text, got %v", err)
	}
}

// runYtdlpOutputWithRetry backs the transcript/detail fetches, which hit
// YouTube's throttled subtitle endpoints. A rate limit that clears on the second
// attempt must yield the output rather than surfacing as a permanent failure —
// this is the "no transcript, works on retry" bug the wrapper fixes.
func TestRunYtdlpOutputWithRetryRecoversAfterRateLimit(t *testing.T) {
	stubSleep(t)
	var calls int32
	r := procexec.FakeRunner{New: func([]string) procexec.Cmd {
		if atomic.AddInt32(&calls, 1) == 1 {
			return &procexec.FakeCmd{Stderr: "HTTP Error 429: Too Many Requests", WaitErr: fmt.Errorf("exit status 1")}
		}
		return &procexec.FakeCmd{Stdout: `{"id":"x"}`}
	}}
	out, err := runYtdlpOutputWithRetry(context.Background(), r, "transcript", nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(out) != `{"id":"x"}` {
		t.Fatalf("got %q, want stdout after retry", out)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("want 2 attempts, got %d", got)
	}
}

// A non-rate-limit failure (e.g. a private video) must fail fast without burning
// the retry budget.
func TestRunYtdlpOutputWithRetryNoRetryOnHardError(t *testing.T) {
	stubSleep(t)
	var calls int32
	r := procexec.FakeRunner{New: func([]string) procexec.Cmd {
		atomic.AddInt32(&calls, 1)
		return &procexec.FakeCmd{Stderr: "ERROR: Private video", WaitErr: fmt.Errorf("exit status 1")}
	}}
	_, err := runYtdlpOutputWithRetry(context.Background(), r, "detail", nil)
	if err == nil || !strings.Contains(err.Error(), "Private video") {
		t.Fatalf("want error containing stderr text, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("want exactly 1 attempt for a non-rate-limit error, got %d", got)
	}
}

// A canceled context must stop the backoff loop instead of exhausting all
// attempts, mirroring runWithRetry's cancellation contract.
func TestRunYtdlpOutputWithRetryStopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls int32
	r := procexec.FakeRunner{New: func([]string) procexec.Cmd {
		atomic.AddInt32(&calls, 1)
		return &procexec.FakeCmd{Stderr: "HTTP Error 429: Too Many Requests", WaitErr: fmt.Errorf("exit status 1")}
	}}
	_, err := runYtdlpOutputWithRetry(ctx, r, "transcript", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("want exactly 1 attempt before bailing on cancellation, got %d", got)
	}
}

func TestRunYtdlpOutputReturnsStdoutOnSuccess(t *testing.T) {
	r := procexec.FakeRunner{New: func([]string) procexec.Cmd {
		return &procexec.FakeCmd{Stdout: `{"id":"x"}`}
	}}
	out, err := runYtdlpOutput(context.Background(), r, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if string(out) != `{"id":"x"}` {
		t.Fatalf("got %q, want raw stdout passthrough", out)
	}
}

// H-10: VideoDetails bypassed procexec.Runner entirely via exec.CommandContext,
// making it untestable and uncancellable. It must now go through c.runner.
func TestClientVideoDetailsUsesRunner(t *testing.T) {
	c := &Client{
		cfg: &config.Config{},
		runner: procexec.FakeRunner{New: func([]string) procexec.Cmd {
			return &procexec.FakeCmd{Stdout: `{"id":"vid1","title":"T","channel":"C"}`}
		}},
	}
	got, err := c.VideoDetails(context.Background(), "https://youtu.be/vid1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.ID != "vid1" || got.Title != "T" {
		t.Fatalf("got %+v, want vid1/T", got)
	}
}

func TestClientVideoDetailsSurfacesStderr(t *testing.T) {
	c := &Client{
		cfg: &config.Config{},
		runner: procexec.FakeRunner{New: func([]string) procexec.Cmd {
			return &procexec.FakeCmd{Stderr: "ERROR: Private video", WaitErr: fmt.Errorf("exit status 1")}
		}},
	}
	_, err := c.VideoDetails(context.Background(), "https://youtu.be/private")
	if err == nil || !strings.Contains(err.Error(), "Private video") {
		t.Fatalf("want error containing stderr text, got %v", err)
	}
}
