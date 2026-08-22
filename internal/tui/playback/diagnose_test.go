package playback

import (
	"strings"
	"testing"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/device/player"
	runewidth "github.com/mattn/go-runewidth"
)

const staleYtdlpDays = 45

func staleYtdlp() YtdlpInfo {
	return YtdlpInfo{Version: "2026.03.31", Age: staleYtdlpDays * 24 * time.Hour}
}

func freshYtdlp() YtdlpInfo {
	return YtdlpInfo{Version: "2026.08.19", Age: 2 * 24 * time.Hour}
}

// failed builds the Result of a player that exited without ever playing.
func failed(stderr string) player.Result {
	return player.Result{ExitCode: 1, Ran: 2 * time.Second, Output: stderr}
}

// TestDiagnoseQuietOnNormalEnd: a video that actually played, or a player that
// exited cleanly, is not a failure — the status line must stay out of the way.
func TestDiagnoseQuietOnNormalEnd(t *testing.T) {
	played := player.Result{ExitCode: 1, Ran: time.Minute, Played: true, Output: "HTTP error 403"}
	if got := diagnose(played, staleYtdlp()); got != "" {
		t.Errorf("playback that ran produced a failure: %q", got)
	}
	clean := player.Result{ExitCode: 0, Ran: time.Minute}
	if got := diagnose(clean, staleYtdlp()); got != "" {
		t.Errorf("a clean exit produced a failure: %q", got)
	}
}

// TestDiagnoseNamesYtdlpCause: yt-dlp's own errors arrive on the player's stderr,
// and each recognized one is translated into a cause plus upgrade advice.
func TestDiagnoseNamesYtdlpCause(t *testing.T) {
	tests := []struct {
		name       string
		stderr     string
		wantCause  string
		wantsYtdlp bool
	}{
		{
			name:       "403 from rotated signatures",
			stderr:     "[ytdl_hook] ERROR: unable to download video data: HTTP Error 403: Forbidden\nFailed to open URL.",
			wantCause:  "HTTP 403",
			wantsYtdlp: true,
		},
		{
			name:       "bot check",
			stderr:     `ERROR: [youtube] abc123: Sign in to confirm you're not a bot.`,
			wantCause:  "bot verification",
			wantsYtdlp: true,
		},
		{
			name:       "extractor broke",
			stderr:     "ERROR: [youtube] abc123: nsig extraction failed: Some formats may be missing",
			wantCause:  "could not extract a playable stream",
			wantsYtdlp: true,
		},
		{
			name:       "video is gone",
			stderr:     "ERROR: [youtube] abc123: Video unavailable. This video has been removed by the uploader",
			wantCause:  "video is unavailable",
			wantsYtdlp: false,
		},
		{
			name:       "no network",
			stderr:     "ERROR: unable to download webpage: Temporary failure in name resolution",
			wantCause:  "network request failed",
			wantsYtdlp: false,
		},
	}
	for _, tt := range tests {
		got := diagnose(failed(tt.stderr), freshYtdlp())
		if !strings.Contains(got, tt.wantCause) {
			t.Errorf("%s: %q does not mention %q", tt.name, got, tt.wantCause)
		}
		mentionsUpdate := strings.Contains(got, "needs updating")
		if mentionsUpdate != tt.wantsYtdlp {
			t.Errorf("%s: update advice = %v, want %v (%q)", tt.name, mentionsUpdate, tt.wantsYtdlp, got)
		}
	}
}

// TestDiagnoseBlamesStaleYtdlp: past ytdlpSuspectAge the local yt-dlp is named
// outright, with its version and age, whatever the player said.
func TestDiagnoseBlamesStaleYtdlp(t *testing.T) {
	got := diagnose(failed("Failed to recognize file format."), staleYtdlp())
	for _, want := range []string{"2026.03.31", "45 days old", "likely cause"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q omits %q", got, want)
		}
	}
}

// TestDiagnoseStaleYtdlpEvenWhenVideoIsGone: an old yt-dlp is worth mentioning
// even when the message points elsewhere — a stale extractor misreports a
// perfectly available video as unavailable.
func TestDiagnoseStaleYtdlpEvenWhenVideoIsGone(t *testing.T) {
	got := diagnose(failed("ERROR: [youtube] abc: Video unavailable"), staleYtdlp())
	if !strings.Contains(got, "likely cause") {
		t.Errorf("stale yt-dlp not mentioned: %q", got)
	}
}

// TestDiagnoseQuotesUnknownError: an unrecognized failure shows the player's own
// last error line rather than a made-up cause.
func TestDiagnoseQuotesUnknownError(t *testing.T) {
	stderr := "[ao/pipewire] Could not connect\nERROR: something entirely new went wrong\n"
	got := diagnose(failed(stderr), freshYtdlp())
	if !strings.Contains(got, "something entirely new went wrong") {
		t.Errorf("%q does not quote the player's error", got)
	}
}

// TestDiagnoseWithoutOutput: capture can fail or the player can die silently, and
// the exit status is then all there is to report.
func TestDiagnoseWithoutStderr(t *testing.T) {
	got := diagnose(player.Result{ExitCode: 2, Ran: time.Second}, YtdlpInfo{})
	if !strings.Contains(got, "status 2") {
		t.Errorf("%q does not report the exit status", got)
	}
	if strings.Contains(got, "yt-dlp") {
		t.Errorf("unknown yt-dlp version must not be blamed: %q", got)
	}
}

// TestFirstErrorLineTruncates: a long line has to stay inside one status-bar row,
// measured the way the renderer measures it — in display columns.
func TestFirstErrorLineTruncates(t *testing.T) {
	long := "ERROR: " + strings.Repeat("x", causeLineMax*2)
	if got := firstErrorLine(long); runewidth.StringWidth(got) > causeLineMax {
		t.Errorf("line not truncated: %d columns", runewidth.StringWidth(got))
	}
}

// TestFirstErrorLinePrefersCause: mpv reports the cause first and then winds down
// with "Exiting... (Errors when loading file)". The cause is the useful half.
func TestFirstErrorLinePrefersCause(t *testing.T) {
	got := firstErrorLine("[ao/pipewire] connected\nERROR: the real problem\nExiting... (Errors when loading file)\n")
	if !strings.Contains(got, "the real problem") {
		t.Errorf("expected the first error line, got %q", got)
	}
}

// TestFirstErrorLineFallsBackToLast: with nothing error-shaped to latch onto, the
// player's parting words are still better than silence.
func TestFirstErrorLineFallsBackToLast(t *testing.T) {
	if got := firstErrorLine("starting\nsomething odd happened\n"); got != "something odd happened" {
		t.Errorf("fallback line = %q", got)
	}
}

// TestDiagnoseRealMpvFailure pins the exact output a real mpv prints when yt-dlp
// cannot resolve a video (captured from mpv 0.40 + yt-dlp 2026.08.19): the
// specific cause must win over mpv's generic "youtube-dl failed" that follows it,
// and its non-standard exit code 2 must still count as a failure.
func TestDiagnoseRealMpvFailure(t *testing.T) {
	const output = "[ytdl_hook] ERROR: [youtube] zzzzINVALID: This video is unavailable\n" +
		"[ytdl_hook] youtube-dl failed: unexpected error occurred\n" +
		"Failed to recognize file format.\n" +
		"Exiting... (Errors when loading file)\n"
	got := diagnose(player.Result{ExitCode: 2, Ran: 3 * time.Second, Output: output}, freshYtdlp())
	if !strings.Contains(got, "video is unavailable") {
		t.Errorf("%q does not report the specific cause", got)
	}
	if strings.Contains(got, "needs updating") {
		t.Errorf("an unavailable video is not yt-dlp's fault: %q", got)
	}
}
