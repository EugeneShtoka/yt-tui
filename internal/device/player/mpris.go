package player

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

// mprisBackend launches a player and tracks position via D-Bus MPRIS2.
type mprisBackend struct {
	driver Driver
	conn   *dbus.Conn

	// mu protects curSess/curCmd/curVideoID for Close(), Active() and exec()'s
	// replace-on-relaunch.
	mu         sync.Mutex
	curSess    *Session
	curCmd     *exec.Cmd
	curVideoID string
}

func newMPRISBackend(driver Driver) (*mprisBackend, error) {
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return nil, fmt.Errorf("newMPRISBackend: %w", err)
	}
	if err := conn.Auth(nil); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("newMPRISBackend auth: %w", err)
	}
	if err := conn.Hello(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("newMPRISBackend hello: %w", err)
	}
	return &mprisBackend{driver: driver, conn: conn}, nil
}

func (b *mprisBackend) exec(videoID string, args []string, startAt time.Duration) (*Session, error) {
	// Stop the previous session's poll goroutine and kill its player process —
	// otherwise launching video B while A's player is still open produces
	// double audio and A keeps answering MPRIS queries meant for B.
	b.mu.Lock()
	oldSess, oldCmd := b.curSess, b.curCmd
	b.mu.Unlock()
	if oldSess != nil {
		oldSess.stop()
	}
	if oldCmd != nil && oldCmd.Process != nil {
		_ = oldCmd.Process.Kill()
	}

	null, err := os.Open(os.DevNull)
	if err != nil {
		return nil, fmt.Errorf("exec: open devnull: %w", err)
	}
	defer null.Close()
	cmd := exec.CommandContext(context.Background(), b.driver.Path(), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = null
	cmd.Stderr = null
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("exec: %w", err)
	}

	sess := newSession(startAt)

	b.mu.Lock()
	b.curSess = sess
	b.curCmd = cmd
	b.curVideoID = videoID
	b.mu.Unlock()

	go b.pollSession(sess, cmd.Process.Pid)
	go func() {
		_ = cmd.Wait()
		sess.Finish()
	}()
	return sess, nil
}

func (b *mprisBackend) Launch(videoID, source, title string, startAt time.Duration) (*Session, error) {
	return b.exec(videoID, b.driver.Args(source, title, startAt), startAt)
}

func (b *mprisBackend) LaunchAudio(videoID, source, title string, startAt time.Duration) (*Session, error) {
	return b.exec(videoID, b.driver.AudioArgs(source, title, startAt), startAt)
}

// Active reports the currently-playing video and its process PID when a player
// launched by this backend is still running. It lets the app hand off to a
// background tracker that keeps saving resume position after the TUI exits.
func (b *mprisBackend) Active() (ActivePlayback, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.curVideoID == "" || b.curCmd == nil || b.curCmd.Process == nil {
		return ActivePlayback{}, false
	}
	if b.curSess != nil {
		select {
		case <-b.curSess.Done():
			return ActivePlayback{}, false // player process already exited
		default:
		}
	}
	return ActivePlayback{VideoID: b.curVideoID, PID: b.curCmd.Process.Pid}, true
}

func (b *mprisBackend) pollSession(sess *Session, pid int) {
	// Give the player a moment to register on D-Bus, but bail out immediately if
	// the session is stopped during the settle window (short playback / quick close).
	select {
	case <-sess.stopCh:
		return
	case <-time.After(1500 * time.Millisecond):
	}

	busName, ok := b.resolveBusNameRetry(sess, pid)
	if !ok {
		// No MPRIS name ever showed up for this PID (unsupported player build,
		// or it exited before registering) — leave position tracking disabled
		// rather than fall back to the well-known name, which may be owned by
		// an unrelated player instance and would corrupt this session's position.
		return
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-sess.stopCh:
			return
		case <-ticker.C:
			pos, ok := b.queryPosition(busName)
			sess.setPosition(pos, ok)
		}
	}
}

// resolveBusNameRetry resolves the D-Bus name the process pid registered as
// its MPRIS endpoint, retrying briefly since the player may not have called
// RequestName yet right after the settle window.
func (b *mprisBackend) resolveBusNameRetry(sess *Session, pid int) (string, bool) {
	for i := 0; i < 5; i++ {
		if name, err := b.resolveBusName(pid); err == nil {
			return name, true
		}
		select {
		case <-sess.stopCh:
			return "", false
		case <-time.After(500 * time.Millisecond):
		}
	}
	return "", false
}

// resolveBusName finds the MPRIS bus name owned by pid instead of trusting the
// driver's well-known name (e.g. "org.mpris.MediaPlayer2.mpv"), which is
// owned by whichever instance registered first and can't distinguish this
// launch from a prior or unrelated player process (H-11).
func (b *mprisBackend) resolveBusName(pid int) (string, error) {
	bus := b.conn.BusObject()
	var names []string
	if err := bus.Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return "", fmt.Errorf("resolveBusName: ListNames: %w", err)
	}
	prefix := b.driver.DBusName()
	for _, name := range names {
		if name != prefix && !strings.HasPrefix(name, prefix+".") {
			continue
		}
		var namePID uint32
		if err := bus.Call("org.freedesktop.DBus.GetConnectionUnixProcessID", 0, name).Store(&namePID); err != nil {
			continue
		}
		if int(namePID) == pid {
			return name, nil
		}
	}
	return "", fmt.Errorf("resolveBusName: no MPRIS name owned by pid %d", pid)
}

func (b *mprisBackend) queryPosition(busName string) (time.Duration, bool) {
	obj := b.conn.Object(busName, "/org/mpris/MediaPlayer2")
	v, err := obj.GetProperty("org.mpris.MediaPlayer2.Player.Position")
	if err != nil {
		return 0, false
	}
	us, ok := v.Value().(int64)
	if !ok {
		return 0, false
	}
	return time.Duration(us) * time.Microsecond, true
}

// Close tears down the backend on app exit. It deliberately does NOT kill the
// running player process: playback is meant to survive the TUI quitting, so the
// user can keep watching after closing yt-tui. We only stop the current
// session's poll goroutine (position tracking is meaningless once we're gone)
// and release the D-Bus connection. The per-launch cmd.Wait() reaper goroutine
// in exec is left to outlive us — in single-binary mode it dies with the
// process; the surviving player is intentional, not a leak.
func (b *mprisBackend) Close() {
	b.mu.Lock()
	sess := b.curSess
	b.mu.Unlock()
	if sess != nil {
		sess.stop()
	}
	_ = b.conn.Close()
}
