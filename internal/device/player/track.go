package player

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

// mprisPrefix is the well-known prefix every MPRIS2 media-player bus name shares.
const mprisPrefix = "org.mpris.MediaPlayer2"

// resolveRetries / resolveDelay bound how long Track waits for the player to
// register on the bus before giving up (a non-MPRIS player never will).
const (
	resolveRetries = 5
	resolveDelay   = 500 * time.Millisecond
)

// Track polls the MPRIS playback position of process pid every interval and
// invokes save with each observed position, until the process exits, ctx is
// canceled, or the player's bus name never appears. It opens its own D-Bus
// session connection, so it can run in a standalone process that outlives the
// TUI (the post-quit position tracker). save receives the position in
// milliseconds.
func Track(ctx context.Context, pid int, interval time.Duration, save func(ms int64)) error {
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return fmt.Errorf("Track: dbus connect: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.Auth(nil); err != nil {
		return fmt.Errorf("Track: dbus auth: %w", err)
	}
	if err := conn.Hello(); err != nil {
		return fmt.Errorf("Track: dbus hello: %w", err)
	}

	busName, ok := resolveWithRetry(ctx, conn, pid)
	if !ok {
		return fmt.Errorf("Track: no MPRIS bus name for pid %d", pid)
	}

	trackLoop(ctx, interval,
		func() bool { return pidAlive(pid) },
		func() (int64, bool) { return queryPositionMs(conn, busName) },
		save,
	)
	return nil
}

// trackLoop is the pure position-tracking loop, with the process-liveness check,
// position poll, and save split out as seams so it can be tested without D-Bus
// or a real process. It saves each successful poll until alive reports false or
// ctx is canceled.
func trackLoop(ctx context.Context, interval time.Duration, alive func() bool, poll func() (int64, bool), save func(int64)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if !alive() {
			return
		}
		if ms, ok := poll(); ok {
			save(ms)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// resolveWithRetry looks up the MPRIS bus name owned by pid, retrying briefly
// since the player may not have registered yet.
func resolveWithRetry(ctx context.Context, conn *dbus.Conn, pid int) (string, bool) {
	for i := 0; i < resolveRetries; i++ {
		if name, err := resolveBusNameForPID(conn, pid); err == nil {
			return name, true
		}
		select {
		case <-ctx.Done():
			return "", false
		case <-time.After(resolveDelay):
		}
	}
	return "", false
}

// resolveBusNameForPID returns any MPRIS bus name owned by pid on conn. It
// matches by owning-process PID (not a well-known name), so it works regardless
// of which player build registered.
func resolveBusNameForPID(conn *dbus.Conn, pid int) (string, error) {
	bus := conn.BusObject()
	var names []string
	if err := bus.Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return "", fmt.Errorf("resolveBusNameForPID: ListNames: %w", err)
	}
	for _, name := range names {
		if !strings.HasPrefix(name, mprisPrefix) {
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
	return "", fmt.Errorf("resolveBusNameForPID: no MPRIS name owned by pid %d", pid)
}

// queryPositionMs reads the MPRIS Position property (microseconds) from busName
// and returns it in milliseconds.
func queryPositionMs(conn *dbus.Conn, busName string) (int64, bool) {
	obj := conn.Object(busName, "/org/mpris/MediaPlayer2")
	v, err := obj.GetProperty("org.mpris.MediaPlayer2.Player.Position")
	if err != nil {
		return 0, false
	}
	us, ok := v.Value().(int64)
	if !ok {
		return 0, false
	}
	return us / 1000, true
}

// pidAlive reports whether the process pid is still running. Signal 0 performs
// error checking without actually delivering a signal: a nil error (or EPERM,
// meaning the process exists but we can't signal it) means alive; ESRCH means
// gone.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
