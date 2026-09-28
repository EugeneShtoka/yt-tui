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

// failureHint names the advice a cause has earned. A stale extractor and a
// missing cookie source produce failures that look alike in the output but need
// opposite fixes, so the signature — not the caller — decides which to print.
type failureHint int

const (
	hintNone      failureHint = iota // the cause speaks for itself
	hintYtdlp                        // an out-of-date extractor explains this
	hintCookies                      // an absent or expired cookie source explains this
	hintTransient                    // nothing local is wrong; YouTube refused this one
)

// failureSignature maps phrases players and yt-dlp print on the way down onto a
// plain-language cause plus the advice that fixes it, which is what turns a raw
// error into something worth acting on.
//
// retry marks the causes a second launch can plausibly survive. YouTube hands out
// stream URLs that are born dead: roughly half of authenticated extractions for a
// given video produce a URL that answers 403 forever, while a fresh extraction
// moments later produces a working one. yt-dlp's own downloader hides this by
// re-extracting on 403, which is why downloads are reliable and playback is not —
// mpv's ytdl_hook resolves once and gives up. Retrying restores the odds.
type failureSignature struct {
	phrases []string
	cause   string
	hint    failureHint
	retry   bool
}

// failureSignatures is ordered most-specific first: the first phrase found in the
// captured output wins, so the generic entries — the ones a player prints after
// the real cause, like mpv's "youtube-dl failed" — must come last.
var failureSignatures = []failureSignature{
	{[]string{"sign in to confirm"}, "YouTube demanded bot verification", hintCookies, false},
	{[]string{"http error 403", "403 forbidden", "access denied"}, "YouTube refused the stream (HTTP 403)", hintTransient, true},
	{[]string{"nsig extraction failed", "signature extraction failed", "unable to extract", "failed to extract"}, "yt-dlp could not extract a playable stream", hintYtdlp, true},
	{[]string{"requested format is not available", "no video formats found"}, "yt-dlp found no usable format", hintYtdlp, false},
	{[]string{"video unavailable", "video is unavailable", "private video", "members-only", "age-restricted", "removed by the uploader"}, "YouTube says the video is unavailable", hintNone, false},
	{[]string{"unable to download webpage", "name resolution", "network is unreachable", "connection refused"}, "the network request failed", hintNone, false},
	{[]string{"youtube-dl failed", "ytdl_hook"}, "yt-dlp could not hand the player a stream", hintYtdlp, true},
	{[]string{"failed to recognize file format", "failed to open"}, "the player could not open the stream", hintYtdlp, true},
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
	cause, hint := classify(res.Output)
	if cause == "" {
		cause = fmt.Sprintf("the player exited with status %d without playing anything", res.ExitCode)
	}
	msg := "Playback failed: " + cause
	if advice := advise(info, hint); advice != "" {
		msg += " — " + advice
	}
	return msg
}

// matchSignature finds the first signature whose phrase appears in the output.
func matchSignature(output string) (failureSignature, bool) {
	lower := strings.ToLower(output)
	for _, sig := range failureSignatures {
		for _, phrase := range sig.phrases {
			if strings.Contains(lower, phrase) {
				return sig, true
			}
		}
	}
	return failureSignature{}, false
}

// classify matches the captured output against the known signatures, falling back
// to the player's own error line: showing its words beats inventing a cause.
func classify(output string) (string, failureHint) {
	if sig, ok := matchSignature(output); ok {
		return sig.cause, sig.hint
	}
	return firstErrorLine(output), hintNone
}

// retryable reports whether a launch that never played is worth attempting again.
// An unrecognized failure is not retried: without knowing what went wrong, a
// second launch is as likely to be a pointless wait as a fix.
func retryable(res player.Result) bool {
	if res.Played || res.ExitCode == 0 {
		return false
	}
	sig, ok := matchSignature(res.Output)
	return ok && sig.retry
}

// firstErrorLine picks the most telling line out of a captured tail: the first one
// mentioning an error, since players report the cause first and then wind down
// ("Exiting... (Errors when loading file)"), which is why that wind-down line is
// skipped. With no error line at all, the last thing said is the best on offer.
func firstErrorLine(output string) string {
	var last string
	for raw := range strings.SplitSeq(output, "\n") {
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

// advise is the advice appended to a failure.
//
// A cookie failure is reported as such whatever the local yt-dlp's age: YouTube
// refuses anonymous playback extraction from every version, so "update yt-dlp"
// there is advice that cannot work — and saying it sends the reader off to check
// an extractor that turns out to be current. Otherwise a local yt-dlp past
// ytdlpSuspectAge is named outright with its version and age, and only then do
// the cause-specific lines get their turn: a stale extractor really can produce
// any of these, so it is worth naming before blaming YouTube.
func advise(info YtdlpInfo, hint failureHint) string {
	if hint == hintCookies {
		return "playback needs valid YouTube cookies; check 'browser' or 'cookies_file' in config.toml"
	}
	if info.Version != "" && info.Age >= ytdlpSuspectAge {
		return fmt.Sprintf("your yt-dlp (%s, %d days old) is the likely cause; update it",
			info.Version, int(info.Age.Hours()/24))
	}
	switch hint {
	case hintYtdlp:
		return "this usually means yt-dlp needs updating"
	case hintTransient:
		// YouTube hands out stream URLs that are dead on arrival roughly half the
		// time; nothing local is at fault and nothing local fixes it, so say what
		// actually works — asking again.
		return "usually random on YouTube's side, not a local fault; try again"
	case hintNone, hintCookies:
	}
	return ""
}
