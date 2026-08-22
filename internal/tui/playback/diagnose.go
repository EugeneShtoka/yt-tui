package playback

import (
	"fmt"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/device/player"
	"github.com/EugeneShtoka/yt-tui/internal/tui/render"
)

// YtdlpInfo describes the local yt-dlp for playback diagnostics. mpv resolves
// YouTube URLs through yt-dlp (its ytdl_hook), so the local copy — not the
// backend, even in remote mode — is what fails when a stream will not start, and
// its version is the first thing worth suspecting. The zero value means "unknown"
// and only softens the wording. It is injected from cmd because the TUI layer
// cannot import internal/youtube.
type YtdlpInfo struct {
	Version string        // as yt-dlp reports it, e.g. "2026.08.19"
	Age     time.Duration // since that version's release
}

// ytdlpSuspectAge is how old the local yt-dlp must be before a failed launch
// names it as the likely cause. YouTube rotates its playback signing constantly,
// and an extractor a month behind has usually stopped keeping up.
const ytdlpSuspectAge = 30 * 24 * time.Hour

// causeLineMax keeps a quoted player error to one status-bar line.
const causeLineMax = 140

// failureSignature maps phrases players and yt-dlp print on the way down onto a
// plain-language cause. ytdlp marks the causes an out-of-date extractor explains,
// which is what turns a raw error into advice worth acting on.
type failureSignature struct {
	phrases []string
	cause   string
	ytdlp   bool
}

// failureSignatures is ordered most-specific first: the first phrase found in the
// captured output wins, so the generic entries — the ones a player prints after
// the real cause, like mpv's "youtube-dl failed" — must come last.
var failureSignatures = []failureSignature{
	{[]string{"sign in to confirm"}, "YouTube demanded bot verification", true},
	{[]string{"http error 403", "403 forbidden", "access denied"}, "YouTube refused the stream (HTTP 403)", true},
	{[]string{"nsig extraction failed", "signature extraction failed", "unable to extract", "failed to extract"}, "yt-dlp could not extract a playable stream", true},
	{[]string{"requested format is not available", "no video formats found"}, "yt-dlp found no usable format", true},
	{[]string{"video unavailable", "video is unavailable", "private video", "members-only", "age-restricted", "removed by the uploader"}, "YouTube says the video is unavailable", false},
	{[]string{"unable to download webpage", "name resolution", "network is unreachable", "connection refused"}, "the network request failed", false},
	{[]string{"youtube-dl failed", "ytdl_hook"}, "yt-dlp could not hand the player a stream", true},
	{[]string{"failed to recognize file format", "failed to open"}, "the player could not open the stream", true},
}

// diagnose turns a player run that never played anything into a single status
// line: what went wrong — quoted from the player when it said something we do not
// recognize — plus an upgrade hint when a stale yt-dlp is the plausible culprit.
// Players are inconsistent about exit codes (mpv uses 2 for a file it cannot
// load), so any non-zero status counts.
// It returns "" for anything that does not look like a failed launch, so a video
// the user watched and closed stays silent.
func diagnose(res player.Result, info YtdlpInfo) string {
	if res.Played || res.ExitCode == 0 {
		return ""
	}
	cause, blameYtdlp := classify(res.Output)
	if cause == "" {
		cause = fmt.Sprintf("the player exited with status %d without playing anything", res.ExitCode)
	}
	msg := "Playback failed: " + cause
	if hint := ytdlpHint(info, blameYtdlp); hint != "" {
		msg += " — " + hint
	}
	return msg
}

// classify matches the captured output against the known signatures, falling back
// to the player's own error line: showing its words beats inventing a cause.
func classify(output string) (string, bool) {
	lower := strings.ToLower(output)
	for _, sig := range failureSignatures {
		for _, phrase := range sig.phrases {
			if strings.Contains(lower, phrase) {
				return sig.cause, sig.ytdlp
			}
		}
	}
	return firstErrorLine(output), false
}

// firstErrorLine picks the most telling line out of a captured tail: the first one
// mentioning an error, since players report the cause first and then wind down
// ("Exiting... (Errors when loading file)"), which is why that wind-down line is
// skipped. With no error line at all, the last thing said is the best on offer.
func firstErrorLine(output string) string {
	var last string
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		last = line
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "exiting") {
			continue // the player giving up, not the reason it did
		}
		if strings.Contains(lower, "error") {
			return render.Truncate(line, causeLineMax)
		}
	}
	return render.Truncate(last, causeLineMax)
}

// ytdlpHint is the advice appended to a failure. A local yt-dlp past
// ytdlpSuspectAge is named outright with its version and age; otherwise the hint
// only appears for causes an outdated extractor is known to produce.
func ytdlpHint(info YtdlpInfo, blameYtdlp bool) string {
	if info.Version != "" && info.Age >= ytdlpSuspectAge {
		return fmt.Sprintf("your yt-dlp (%s, %d days old) is the likely cause; update it",
			info.Version, int(info.Age.Hours()/24))
	}
	if blameYtdlp {
		return "this usually means yt-dlp needs updating"
	}
	return ""
}
