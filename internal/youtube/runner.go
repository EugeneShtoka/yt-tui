package youtube

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/procexec"
)

// pageSize is the number of items fetched per yt-dlp call for paginated requests.
const pageSize = 200

// maxRetries is the number of extra attempts made when yt-dlp reports a rate limit.
const maxRetries = 3

func isRateLimited(s string) bool {
	sl := strings.ToLower(s)
	return strings.Contains(sl, "http error 429") ||
		strings.Contains(sl, "too many requests") ||
		strings.Contains(sl, "rate-limited") ||
		strings.Contains(sl, "rate limit")
}

func retryDelay(attempt int) time.Duration {
	return time.Duration(1<<uint(attempt)) * 5 * time.Second
}

// retrySleep is the seam tests stub out so the rate-limit backoff doesn't
// actually block for tens of seconds. It returns early if ctx is canceled.
var retrySleep = func(ctx context.Context, d time.Duration) {
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

// cookieArgs returns the yt-dlp cookie flags for the configured auth source: an
// explicit cookies file takes precedence over a browser cookie jar. It is nil
// when neither is set. Single-sourcing these flags keeps the cookie precedence
// identical across every yt-dlp invocation (listing, detail, transcript) (L-5).
func cookieArgs(cfg *config.Config) []string {
	switch {
	case cfg.CookiesFile != "":
		return []string{"--cookies", cfg.CookiesFile}
	case cfg.Browser != "":
		return []string{"--cookies-from-browser", cfg.Browser}
	default:
		return nil
	}
}

// buildArgs builds yt-dlp arguments with an optional upper limit (0 = no limit).
// Used for requests that intentionally cap results (recommended feed, search, channel-latest).
func buildArgs(cfg *config.Config, url string, limit int) []string {
	args := []string{
		"--flat-playlist",
		"--dump-json",
		"--no-warnings",
		"--quiet",
		"--extractor-args", "youtubetab:approximate_date",
		"--sleep-requests", "1",
	}
	args = append(args, cookieArgs(cfg)...)
	if limit > 0 {
		args = append(args, "--playlist-end", fmt.Sprintf("%d", limit))
	}
	args = append(args, url)
	return args
}

// buildArgsPage builds yt-dlp arguments for one page of a paginated fetch.
// start is 1-indexed; each page covers [start, start+pageSize-1].
func buildArgsPage(cfg *config.Config, u string, start int) []string {
	end := start + pageSize - 1
	args := []string{
		"--flat-playlist",
		"--dump-json",
		"--no-warnings",
		"--quiet",
		"--extractor-args", "youtubetab:approximate_date",
		"--sleep-requests", "1",
	}
	args = append(args, cookieArgs(cfg)...)
	args = append(args,
		"--playlist-start", fmt.Sprintf("%d", start),
		"--playlist-end", fmt.Sprintf("%d", end),
		u)
	return args
}

// waitErr reaps the yt-dlp process and folds a non-zero exit into the parse
// error, but only when the run produced no usable output (parsed == 0). yt-dlp
// routinely exits non-zero when individual playlist entries are unavailable
// (deleted/members-only) while still emitting valid lines for the rest, so a
// non-zero exit alongside real output is treated as a partial success. An empty
// result plus a failed exit is the case we must not report as "success": it is
// the silent-empty-feed bug. A pre-existing scan error always takes precedence.
func waitErr(cmd procexec.Cmd, parsed int, scanErr error) error {
	err := cmd.Wait()
	if scanErr == nil && err != nil && parsed == 0 {
		return fmt.Errorf("yt-dlp exited without output: %w", err)
	}
	return scanErr
}

// runYtdlp starts yt-dlp with args, runs parse over its stdout, reaps the
// process (folding a non-zero exit into the error when no output was produced),
// and returns the parsed result, its raw count, captured stderr, and the error.
// ctx bounds the subprocess: canceling it (RPC disconnect, daemon shutdown)
// kills yt-dlp instead of leaving it to run to completion.
func runYtdlp[T any](ctx context.Context, r procexec.Runner, args []string, parse func(io.Reader) (T, int, error)) (T, int, string, error) {
	var zero T
	cmd := r.Command(ctx, "yt-dlp", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return zero, 0, "", fmt.Errorf("yt-dlp stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return zero, 0, "", fmt.Errorf("yt-dlp stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return zero, 0, "", fmt.Errorf("yt-dlp start: %w", err)
	}
	// Drain stderr concurrently so a large error stream can't deadlock the pipe
	// while we read stdout.
	var errBuf bytes.Buffer
	errDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&errBuf, stderr)
		close(errDone)
	}()
	result, raw, scanErr := parse(stdout)
	<-errDone
	scanErr = waitErr(cmd, raw, scanErr)
	return result, raw, errBuf.String(), scanErr
}

// runWithRetry runs yt-dlp through parse, retrying with exponential backoff when
// stderr reports a rate limit. kind labels the fetch for debug logs. It returns
// the parsed result and its raw count (for pagination decisions). The backoff
// sleep and each yt-dlp invocation both honor ctx cancellation.
func runWithRetry[T any](ctx context.Context, r procexec.Runner, kind string, args []string, parse func(io.Reader) (T, int, error)) (T, int, error) {
	var zero T
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			d := retryDelay(attempt - 1)
			debug.Log("%s fetch rate-limited, retry %d/%d after %v", kind, attempt, maxRetries, d)
			retrySleep(ctx, d)
			if err := ctx.Err(); err != nil {
				return zero, 0, fmt.Errorf("runWithRetry: %w", err)
			}
		}
		result, raw, stderrStr, err := runYtdlp(ctx, r, args, parse)
		if stderrStr != "" {
			debug.Log("yt-dlp stderr: %s", strings.TrimSpace(stderrStr))
		}
		if !isRateLimited(stderrStr) || attempt >= maxRetries {
			return result, raw, err
		}
	}
	return zero, 0, fmt.Errorf("yt-dlp: max retries exceeded (rate limited)")
}

// runYtdlpOutput runs yt-dlp via r and returns its raw stdout. Unlike
// runYtdlp/waitErr (tuned for playlist-style partial success), a failed exit
// here is always an error — VideoDetails has no notion of partial output —
// and the error message surfaces captured stderr instead of just the bare
// exit status.
func runYtdlpOutput(ctx context.Context, r procexec.Runner, args []string) ([]byte, error) {
	cmd := r.Command(ctx, "yt-dlp", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("yt-dlp start: %w", err)
	}
	var errBuf bytes.Buffer
	errDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&errBuf, stderr)
		close(errDone)
	}()
	out, readErr := io.ReadAll(stdout)
	<-errDone
	if waitErr := cmd.Wait(); waitErr != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return nil, fmt.Errorf("yt-dlp: %s", msg)
	}
	if readErr != nil {
		return nil, fmt.Errorf("yt-dlp stdout read: %w", readErr)
	}
	return out, nil
}

// runYtdlpOutputWithRetry wraps runYtdlpOutput with the same rate-limit backoff
// runWithRetry gives the listing fetches. Transcript/detail fetches hit YouTube's
// subtitle endpoints, which throttle with HTTP 429 just like the listing tab does;
// without this a single 429 surfaced as a permanent "no transcript", which retrying
// by hand would then paper over. runYtdlpOutput folds stderr into its error message,
// so the rate-limit signal is recovered from err.Error(). kind labels the fetch for
// debug logs. The backoff sleep and each yt-dlp invocation both honor ctx.
func runYtdlpOutputWithRetry(ctx context.Context, r procexec.Runner, kind string, args []string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			d := retryDelay(attempt - 1)
			debug.Log("%s fetch rate-limited, retry %d/%d after %v", kind, attempt, maxRetries, d)
			retrySleep(ctx, d)
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("runYtdlpOutputWithRetry: %w", err)
			}
		}
		out, err := runYtdlpOutput(ctx, r, args)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !isRateLimited(err.Error()) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%s: max retries exceeded (rate limited): %w", kind, lastErr)
}

// ── typed wrappers around runWithRetry ────────────────────────────────────────

func (c *Client) runAndParseVideos(ctx context.Context, args []string) ([]domain.Video, int, error) {
	return runWithRetry(ctx, c.runner, "video", args, parseVideoLines)
}

func (c *Client) runAndParseChannels(ctx context.Context, args []string) ([]domain.Channel, int, error) {
	return runWithRetry(ctx, c.runner, "channel", args, parseChannelLines)
}

func (c *Client) runAndParsePlaylists(ctx context.Context, args []string) ([]domain.YTPlaylist, int, error) {
	return runWithRetry(ctx, c.runner, "playlist", args, parsePlaylistLines)
}

func (c *Client) runAndParseMixed(ctx context.Context, args []string) (channels []domain.Channel, videos []domain.Video, err error) {
	res, _, err := runWithRetry(ctx, c.runner, "mixed", args, parseMixedLines)
	return res.channels, res.videos, err
}
