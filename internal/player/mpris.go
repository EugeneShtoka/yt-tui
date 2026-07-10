package player

import (
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

	mu      sync.Mutex
	lastPos time.Duration
	active  bool

	stopCh chan struct{}
	once   sync.Once
}

func newMPRISBackend(driver Driver) (*mprisBackend, error) {
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
	return &mprisBackend{driver: driver, conn: conn}, nil
}

func (b *mprisBackend) exec(args []string, startAt time.Duration) error {
	b.Close()

	cmd := exec.Command(b.driver.Path(), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}

	b.mu.Lock()
	b.lastPos = startAt
	b.active = true
	b.stopCh = make(chan struct{})
	b.once = sync.Once{}
	stop := b.stopCh
	b.mu.Unlock()

	go b.poll(stop)
	return nil
}

func (b *mprisBackend) Launch(source string, startAt time.Duration) error {
	return b.exec(b.driver.Args(source, startAt), startAt)
}

func (b *mprisBackend) LaunchAudio(source string, startAt time.Duration) error {
	return b.exec(b.driver.AudioArgs(source, startAt), startAt)
}

func (b *mprisBackend) poll(stopCh chan struct{}) {
	// Give the player a moment to register on D-Bus.
	time.Sleep(1500 * time.Millisecond)

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
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

func (b *mprisBackend) Position() (time.Duration, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastPos, b.active
}

func (b *mprisBackend) Close() {
	b.once.Do(func() {
		b.mu.Lock()
		b.active = false
		ch := b.stopCh
		b.mu.Unlock()
		if ch != nil {
			close(ch)
		}
	})
}
