package player

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// fakeDriver points the backend at an arbitrary binary so the exec/launch path
// can be tested without a real media player.
type fakeDriver struct {
	path string
	args []string
}

func (d fakeDriver) Path() string { return d.path }
func (d fakeDriver) Args(_, _ string, _ time.Duration) []string {
	return append([]string(nil), d.args...)
}
func (d fakeDriver) AudioArgs(source, title string, startAt time.Duration) []string {
	return d.Args(source, title, startAt)
}
func (d fakeDriver) DBusName() string { return "org.mpris.MediaPlayer2.fake" }

// TestSimpleBackendLaunchLifecycle drives the previously-untested exec path: a
// launched process is reaped and its Session's Done() closes when it exits.
func TestSimpleBackendLaunchLifecycle(t *testing.T) {
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no 'true' binary available")
	}
	b := newSimpleBackend(fakeDriver{path: truePath})

	sess, err := b.Launch("vid1", "src", "title", 0)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	select {
	case <-sess.Done():
		// process exited and the reaper closed the session — success.
	case <-time.After(3 * time.Second):
		t.Fatal("session Done() never closed after the process exited")
	}
}

// TestSimpleBackendLaunchAudioLifecycle covers the audio launch path.
func TestSimpleBackendLaunchAudioLifecycle(t *testing.T) {
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no 'true' binary available")
	}
	b := newSimpleBackend(fakeDriver{path: truePath})

	sess, err := b.LaunchAudio("vid1", "src", "title", 0)
	if err != nil {
		t.Fatalf("LaunchAudio: %v", err)
	}
	select {
	case <-sess.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("audio session Done() never closed")
	}
}

// TestSimpleBackendLaunchBadBinaryErrors verifies a non-existent player binary
// surfaces an error instead of a phantom session.
func TestSimpleBackendLaunchBadBinaryErrors(t *testing.T) {
	b := newSimpleBackend(fakeDriver{path: "/nonexistent/player-binary-xyz"})
	if _, err := b.Launch("vid1", "src", "title", 0); err == nil {
		t.Error("Launch with a missing binary: got nil error, want non-nil")
	}
}

// TestSimpleBackendCapturesFailureOutput drives the whole diagnosis chain end to
// end: a "player" that reports a yt-dlp error and exits non-zero must come back as
// a Result carrying both, which is what turns "the video just doesn't start" into
// something the UI can explain. The error goes to stdout and a second line to
// stderr because mpv reports on stdout while other players use stderr — both have
// to survive the capture.
func TestSimpleBackendCapturesFailureOutput(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no 'sh' binary available")
	}
	const ytdlpError = "[ytdl_hook] ERROR: unable to download video data: HTTP Error 403: Forbidden"
	b := newSimpleBackend(fakeDriver{
		path: sh,
		args: []string{"-c", "echo '" + ytdlpError + "'; echo 'Exiting... (Errors when loading file)' >&2; exit 2"},
	})

	sess, err := b.Launch("vid1", "src", "title", 0)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	select {
	case <-sess.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session never finished")
	}

	res, ok := sess.Result()
	if !ok {
		t.Fatal("a failed launch must report a result")
	}
	if res.ExitCode != 2 {
		t.Errorf("ExitCode = %d, want 2", res.ExitCode)
	}
	if res.Played {
		t.Error("nothing played, yet Played is true")
	}
	if !strings.Contains(res.Output, "HTTP Error 403") {
		t.Errorf("captured output = %q, want the error the player printed on stdout", res.Output)
	}
	if !strings.Contains(res.Output, "Errors when loading file") {
		t.Errorf("captured output = %q, want the stderr line too", res.Output)
	}
}
