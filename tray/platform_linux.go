//go:build linux && !android

package tray

import (
	"fmt"
	"os"
	"time"

	"github.com/worldiety/gift/internal/dbus"
)

// receiveLimit bounds the messages the connection decodes; see
// [dbus.Conn.SetReceiveLimit].
const receiveLimit = 1 << 20

// callTimeout bounds every round trip to the bus and to the watcher.
const callTimeout = 2 * time.Second

// openPlatform connects to the session bus and creates the
// StatusNotifierItem on it; sni.go has the protocol, and the reason
// everything beyond the connection lives there.
//
// Every failure is [ErrUnsupported]: a machine without a session bus, or
// one whose bus cannot be reached, is a machine without a tray, which is a
// normal state for a kiosk and not a defect. A bus without a
// StatusNotifierWatcher is not a failure here, because the panel that
// provides one may simply not have started yet; sni.go logs it once and
// shows the icon when a watcher appears, which on GNOME without its
// extension is never.
//
// It makes three round trips — Hello, AddMatch, NameHasOwner — on the
// goroutine of the first Set, as gift/notify does on its first Show. There
// is no main thread to protect on Linux, and every later round trip is made
// by the worker of sni.go.
//
// The connection is the tray's own and not shared with gift/notify's: the
// bus name of the item belongs to the connection that requested it, and a
// separate one keeps the two packages from having to agree on who closes
// it. A connection costs one socket.
func openPlatform(cfg openConfig) (platform, error) {
	addr, err := dbus.SessionBusAddress(os.Getenv, func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	c, err := dbus.Dial(addr, callTimeout)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot connect to the session bus: %v", ErrUnsupported, err)
	}
	// Nothing this package receives is larger than a few hundred bytes:
	// a call of the host, a reply of the bus, a signal. A peer that sends
	// more is answered with an error, not decoded.
	c.SetReceiveLimit(receiveLimit)
	s, err := newSNI(c, cfg)
	if err != nil {
		c.Close()
		return nil, err
	}
	return s, nil
}
