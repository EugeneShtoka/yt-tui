package player

import (
	"os"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

type fakeMPRISDriver struct{ dbusName string }

func (d fakeMPRISDriver) Path() string                                    { return "/bin/true" }
func (d fakeMPRISDriver) Args(_, _ string, _ time.Duration) []string      { return nil }
func (d fakeMPRISDriver) AudioArgs(_, _ string, _ time.Duration) []string { return nil }
func (d fakeMPRISDriver) DBusName() string                                { return d.dbusName }

// newTestMPRISBackend opens a real private session-bus connection (same as
// newMPRISBackend) so resolveBusName/Close can be exercised against the
// genuine D-Bus name registry instead of a mock.
func newTestMPRISBackend(t *testing.T, driverName string) *mprisBackend {
	t.Helper()
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		t.Skipf("no session bus available: %v", err)
	}
	if err := conn.Auth(nil); err != nil {
		t.Skipf("dbus auth failed: %v", err)
	}
	if err := conn.Hello(); err != nil {
		t.Skipf("dbus hello failed: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &mprisBackend{driver: fakeMPRISDriver{dbusName: driverName}, conn: conn}
}

// H-11: resolveBusName must find the name this process itself registered,
// proving the launched instance is identified by PID rather than by trusting
// whichever process registered the well-known name first.
func TestResolveBusNameMatchesOwnPID(t *testing.T) {
	b := newTestMPRISBackend(t, "org.mpris.MediaPlayer2.yttuitest1")
	if _, err := b.conn.RequestName(b.driver.DBusName(), dbus.NameFlagDoNotQueue); err != nil {
		t.Fatalf("RequestName: %v", err)
	}
	name, err := b.resolveBusName(os.Getpid())
	if err != nil {
		t.Fatalf("resolveBusName: %v", err)
	}
	if name != b.driver.DBusName() {
		t.Errorf("got %q, want %q", name, b.driver.DBusName())
	}
}

// Some MPRIS players append ".instanceN" to the well-known name on conflict;
// resolveBusName must still match it by prefix.
func TestResolveBusNameMatchesInstanceSuffix(t *testing.T) {
	b := newTestMPRISBackend(t, "org.mpris.MediaPlayer2.yttuitest2")
	instanceName := b.driver.DBusName() + ".instance12345"
	if _, err := b.conn.RequestName(instanceName, dbus.NameFlagDoNotQueue); err != nil {
		t.Fatalf("RequestName: %v", err)
	}
	name, err := b.resolveBusName(os.Getpid())
	if err != nil {
		t.Fatalf("resolveBusName: %v", err)
	}
	if name != instanceName {
		t.Errorf("got %q, want %q", name, instanceName)
	}
}

// A name owned by a different PID must not match — this is the exact
// cross-session corruption H-11 describes (an unrelated/stale player
// answering position queries for the wrong session).
func TestResolveBusNameNoMatchForDifferentPID(t *testing.T) {
	b := newTestMPRISBackend(t, "org.mpris.MediaPlayer2.yttuitest3")
	if _, err := b.conn.RequestName(b.driver.DBusName(), dbus.NameFlagDoNotQueue); err != nil {
		t.Fatalf("RequestName: %v", err)
	}
	if _, err := b.resolveBusName(os.Getpid() + 1); err == nil {
		t.Fatal("want error when no name is owned by the given pid, got nil")
	}
}

// H-11: Close() must close the private D-Bus connection, not just stop the
// polling goroutine — otherwise every backend leaks a socket for the process
// lifetime.
func TestMPRISBackendCloseClosesConnection(t *testing.T) {
	b := newTestMPRISBackend(t, "org.mpris.MediaPlayer2.yttuitest4")
	b.Close()
	if b.conn.Connected() {
		t.Fatal("Close() must close the private D-Bus connection")
	}
}
