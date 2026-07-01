package player

import (
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

// mprisBackend launches a player and tracks position via D-Bus MPRIS2.
type mprisBackend struct {
	path string   // absolute path to player binary
	dest string   // D-Bus destination, e.g. "org.mpris.MediaPlayer2.mpv"
	conn *dbus.Conn

	mu      sync.Mutex
	lastPos time.Duration
	active  bool

	stopCh chan struct{}
	once   sync.Once
}

func newMPRISBackend(path string) (*mprisBackend, error) {
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return nil, err
	}
	if err := conn.Auth(nil); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.Hello(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	name := baseName(path)
	// Strip version suffixes like "mpv" or "vlc-4.0" → use just the base.
	if idx := strings.IndexByte(name, '-'); idx > 0 {
		name = name[:idx]
	}
	return &mprisBackend{
		path: path,
		dest: "org.mpris.MediaPlayer2." + name,
		conn: conn,
	}, nil
}

func (b *mprisBackend) Launch(filePath string, startAt time.Duration) error {
	b.Close() // stop any previous poll loop

	args := startArgs(b.path, filePath, startAt)
	cmd := exec.Command(b.path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}

	b.mu.Lock()
	b.lastPos = startAt
	b.active = true
	b.stopCh = make(chan struct{})
	b.once = sync.Once{}
	b.mu.Unlock()

	go b.poll()
	return nil
}

func (b *mprisBackend) poll() {
	// Give the player a moment to register on D-Bus.
	time.Sleep(1500 * time.Millisecond)

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			pos, ok := b.queryPosition()
			b.mu.Lock()
			if ok {
				b.lastPos = pos
				b.active = true
			} else {
				b.active = false
			}
			b.mu.Unlock()
		}
	}
}

func (b *mprisBackend) queryPosition() (time.Duration, bool) {
	obj := b.conn.Object(b.dest, "/org/mpris/MediaPlayer2")
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

func (b *mprisBackend) Position() (time.Duration, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastPos, b.active
}

func (b *mprisBackend) Close() {
	b.once.Do(func() {
		b.mu.Lock()
		b.active = false
		b.mu.Unlock()
		if b.stopCh != nil {
			close(b.stopCh)
		}
	})
}
