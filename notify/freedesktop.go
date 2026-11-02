package notify

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/worldiety/gift/internal/dbus"
)

// This file speaks the Desktop Notifications Specification, the D-Bus
// interface org.freedesktop.Notifications that GNOME Shell, KDE Plasma and
// the smaller servers (dunst, mako, xfce4-notifyd, …) all implement. It sits
// on [busCaller] rather than on a connection, carries no build tag, and is
// tested against a server that lives in the test.
//
// # The calls
//
//	GetCapabilities() → as                    once, when the platform opens
//	Notify(susssasa{sv}i) → u                  per Show
//	CloseNotification(u)                       per Withdraw
//	signal ActionInvoked(u id, s action_key)   a click; "default" is the body
//	signal NotificationClosed(u id, u reason)  the server forgot one of ours
//
// The id the server returns is a number of its own. [Notification.ID] is the
// application's name for a notification and is mapped onto it here, so that
// a second Show with the same ID passes the old number as replaces_id and the
// server updates the notification in place instead of stacking a second one.
//
// # Capabilities that change what is sent
//
//   - "body-markup": the server parses the body as a subset of HTML. A plain
//     text body is escaped, otherwise "a < b" loses its tail on GNOME and
//     KDE, which both announce it.
//   - "actions": only then is the "default" action sent, which is what turns
//     a click on the notification into ActionInvoked. A server without it
//     ignores clicks, and OnClick is never called.
//
// # What a click does not do
//
// It does not raise the window. GNOME and KDE send an ActivationToken signal
// before ActionInvoked, which a Wayland client hands to xdg-activation to be
// allowed to take the focus; under X11 the window would have to send
// _NET_ACTIVE_WINDOW. Ebitengine exposes neither, so the token is ignored and
// raising the window is left to the application, which at least has
// ebiten.RequestAttention.

const (
	notifyName      = "org.freedesktop.Notifications"
	notifyPath      = dbus.ObjectPath("/org/freedesktop/Notifications")
	notifyInterface = "org.freedesktop.Notifications"
)

// busCaller is the one method of [conn] this file needs, as an interface so
// that a test can answer the calls.
type busCaller interface {
	Call(dest string, path dbus.ObjectPath, iface, member string, sig dbus.Signature, args ...any) ([]any, error)
}

// freedesktop is the Linux platform: a [platform] over a notification server
// on the session bus.
type freedesktop struct {
	bus          busCaller
	appName      string
	desktopEntry string
	activated    func(func())

	actions bool
	markup  bool

	mu    sync.Mutex
	byID  map[string]uint32
	shown map[uint32]shownNote
}

// shownNote is what is remembered of a notification the server still shows:
// the application's ID for it, if it had one, and what a click runs.
type shownNote struct {
	id      string
	onClick func()
}

// newFreedesktop subscribes to the signals of the notification server and
// asks for its capabilities. A failure of the latter means that there is no
// server, and is reported as [ErrUnsupported].
//
// The subscription comes first, so that no signal can be lost between the
// first Notify and the match rule taking effect.
func newFreedesktop(bus busCaller, cfg openConfig) (*freedesktop, error) {
	rule := "type='signal',sender='" + notifyName + "',interface='" + notifyInterface +
		"',path='" + string(notifyPath) + "'"
	if _, err := bus.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "AddMatch", "s", rule); err != nil {
		return nil, fmt.Errorf("gift/notify: subscribing to the notification server: %w", err)
	}
	reply, err := bus.Call(notifyName, notifyPath, notifyInterface, "GetCapabilities", "")
	if err != nil {
		return nil, fmt.Errorf("%w: no notification server on the session bus: %v", ErrUnsupported, err)
	}
	f := &freedesktop{
		bus:          bus,
		appName:      cfg.appName,
		desktopEntry: cfg.appID,
		activated:    cfg.activated,
		byID:         map[string]uint32{},
		shown:        map[uint32]shownNote{},
	}
	if len(reply) == 1 {
		caps, _ := reply[0].([]any)
		for _, c := range caps {
			switch c {
			case "actions":
				f.actions = true
			case "body-markup":
				f.markup = true
			}
		}
	}
	return f, nil
}

func (f *freedesktop) show(n Notification) error {
	var replaces uint32
	if n.ID != "" {
		f.mu.Lock()
		replaces = f.byID[n.ID]
		f.mu.Unlock()
	}
	body := n.Body
	if f.markup {
		body = escapeMarkup(body)
	}
	actions := []string{}
	if n.OnClick != nil && f.actions {
		// The label of the default action is not shown by GNOME or KDE; a
		// server that lists actions in a menu shows an empty entry.
		actions = append(actions, "default", "")
	}
	hints := []dbus.DictEntry{}
	if f.desktopEntry != "" {
		hints = append(hints, dbus.DictEntry{Key: "desktop-entry", Value: dbus.Variant{Sig: "s", Value: f.desktopEntry}})
	}

	reply, err := f.bus.Call(notifyName, notifyPath, notifyInterface, "Notify", "susssasa{sv}i",
		f.appName, replaces, "", n.Title, body, actions, hints, int32(-1))
	if err != nil {
		return err
	}
	if len(reply) != 1 {
		return errors.New("gift/notify: Notify returned no id")
	}
	sid, ok := reply[0].(uint32)
	if !ok {
		return fmt.Errorf("gift/notify: Notify returned a %T, not an id", reply[0])
	}

	f.mu.Lock()
	if replaces != 0 && replaces != sid {
		delete(f.shown, replaces)
	}
	if n.ID != "" {
		f.byID[n.ID] = sid
	}
	f.shown[sid] = shownNote{id: n.ID, onClick: n.OnClick}
	f.mu.Unlock()
	return nil
}

// authorize answers yes: the specification has no permission, the user
// turns an application off in the server's settings and the server then
// drops its notifications without telling the sender.
func (f *freedesktop) authorize(done func(bool)) {
	done(true)
}

func (f *freedesktop) withdraw(id string) error {
	f.mu.Lock()
	sid, ok := f.byID[id]
	if ok {
		delete(f.byID, id)
		delete(f.shown, sid)
	}
	f.mu.Unlock()
	if !ok {
		return nil
	}
	_, err := f.bus.Call(notifyName, notifyPath, notifyInterface, "CloseNotification", "u", sid)
	return err
}

// signal handles the signals of the notification server. It runs on the
// reader goroutine of the connection and must not block, which it does not:
// the click is handed to activated, which posts it.
//
// The signals are broadcast on older servers, so ids this package did not
// receive from Notify are someone else's and are ignored.
func (f *freedesktop) signal(m *dbus.Message) {
	if m.Interface != notifyInterface || m.Path != notifyPath {
		return
	}
	switch m.Member {
	case "ActionInvoked":
		if m.Sig != "us" {
			return
		}
		sid, key := m.Body[0].(uint32), m.Body[1].(string)
		if key != "default" {
			return
		}
		f.mu.Lock()
		s, ok := f.shown[sid]
		f.mu.Unlock()
		if ok && s.onClick != nil {
			f.activated(s.onClick)
		}
	case "NotificationClosed":
		if m.Sig != "uu" {
			return
		}
		sid := m.Body[0].(uint32)
		f.mu.Lock()
		if s, ok := f.shown[sid]; ok {
			delete(f.shown, sid)
			if s.id != "" && f.byID[s.id] == sid {
				delete(f.byID, s.id)
			}
		}
		f.mu.Unlock()
	}
}

// markupEscaper escapes the three characters that the body markup of the
// specification gives a meaning.
var markupEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escapeMarkup(s string) string {
	return markupEscaper.Replace(s)
}
