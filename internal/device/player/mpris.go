package player

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

// mprisBackend launches a player and tracks position via D-Bus MPRIS2.
type mprisBackend struct {
	driver Driver
	conn   *dbus.Conn

	// mu protects curSess for Close().
	mu      sync.Mutex
	curSess *Session
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

func (b *mprisBackend) exec(args []string, startAt time.Duration) (*Session, error) {
	// Stop previous session's poll goroutine before starting a new one.
	b.mu.Lock()
	old := b.curSess
	b.mu.Unlock()
	if old != nil {
		old.stop()
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
	b.mu.Unlock()

	go b.pollSession(sess)
	go func() {
		_ = cmd.Wait()
		sess.stop()
		close(sess.doneCh)
	}()
	return sess, nil
}

func (b *mprisBackend) Launch(source, title string, startAt time.Duration) (*Session, error) {
	return b.exec(b.driver.Args(source, title, startAt), startAt)
}

func (b *mprisBackend) LaunchAudio(source, title string, startAt time.Duration) (*Session, error) {
	return b.exec(b.driver.AudioArgs(source, title, startAt), startAt)
}

func (b *mprisBackend) pollSession(sess *Session) {
	// Give the player a moment to register on D-Bus.
	time.Sleep(1500 * time.Millisecond)

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-sess.stopCh:
			return
		case <-ticker.C:
			pos, ok := b.queryPosition()
			sess.setPosition(pos, ok)
		}
	}
}

func (b *mprisBackend) queryPosition() (time.Duration, bool) {
	obj := b.conn.Object(b.driver.DBusName(), "/org/mpris/MediaPlayer2")
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

func (b *mprisBackend) Close() {
	b.mu.Lock()
	sess := b.curSess
	b.mu.Unlock()
	if sess != nil {
		sess.stop()
	}
}
