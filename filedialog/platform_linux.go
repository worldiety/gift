//go:build linux && !android

package filedialog

import (
	"fmt"
	"os"
	"time"

	"github.com/worldiety/gift/internal/dbus"
)

// receiveLimit bounds the messages the connection decodes; see
// [dbus.Conn.SetReceiveLimit]. The portal sends small replies and a
// Response with one URI per picked file. A Response beyond the limit is
// dropped by the connection without a word, which would leave the dialog
// held for good, so the limit is not the mebibyte of gift/notify and
// gift/tray: a person who selects every file of a large directory in an
// OpenMulti dialog sends ten thousand URIs, a mebibyte or more. Sixteen
// mebibytes are a hundred thousand of them, and still an eighth of what
// the specification allows a peer to make this process decode.
const receiveLimit = 16 << 20

// callTimeout bounds every round trip to the bus and to the portal. It is
// long because the first call may start xdg-desktop-portal by D-Bus
// activation, which in turn starts its GNOME or KDE backend.
const callTimeout = 25 * time.Second

// platformAvailable reports whether this is a GNOME or KDE session with a
// session bus, from the environment alone; see [portalAvailable].
func platformAvailable() bool { return portalAvailable(os.Getenv, fileExists) }

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// openPlatform connects to the session bus and finds the FileChooser of
// xdg-desktop-portal on it; portal.go has the protocol. Every failure is
// lasting: a session without a reachable bus or without the portal is one
// without a native dialog, which is a normal state and not a defect.
//
// It makes its round trips — Hello, AddMatch, the version, GetNameOwner — on
// the goroutine of the first Open, never on the UI goroutine. The connection
// is the package's own, not shared with gift/notify or gift/tray, so that no
// package has to agree with another on who closes it.
func openPlatform(cfg openConfig) (platform, error) {
	addr, err := dbus.SessionBusAddress(cfg.getenv, fileExists)
	if err != nil {
		return nil, permanent(fmt.Errorf("%w: %v", ErrUnsupported, err))
	}
	c, err := dbus.Dial(addr, callTimeout)
	if err != nil {
		return nil, permanent(fmt.Errorf("%w: cannot connect to the session bus: %v", ErrUnsupported, err))
	}
	c.SetReceiveLimit(receiveLimit)
	p, err := newPortal(c, cfg.log)
	if err != nil {
		c.Close()
		return nil, err
	}
	p.parent = newX11ParentFinder(cfg.getenv, cfg.log).parent
	return p, nil
}
