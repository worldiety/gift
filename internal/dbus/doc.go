// Package dbus is the D-Bus client of gift: the wire format, a connection to
// the session bus, and the exported objects that let other programs call
// into this one. It needs no cgo and no module beyond the standard library.
//
// # Why there is a D-Bus implementation in gift at all
//
// The notification server of GNOME and of KDE and the status area of their
// panels are reached over the session bus, and there is no cgo-free way to
// reach the session bus other than to speak its protocol. The alternatives
// were weighed in the project plan, section 43: godbus/dbus would have been
// the first module this project adds for a platform feature, and libdbus
// through purego is a C library built around callbacks and a main loop. What
// gift needs of D-Bus is small, and so is this package.
//
// It started inside gift/notify, which only calls: Notify, CloseNotification
// and two signals. gift/tray needs the other direction too — a panel calls
// the application to read its icon and its menu, and to say that an entry
// was clicked — and so the client moved here, where both can use it, and
// grew the server side in export.go.
//
// # What a user of this package writes
//
//	c, err := dbus.Dial(addr, 2*time.Second)
//	reply, err := c.Call(dest, path, iface, "Method", "su", "text", uint32(1))
//	err = c.Export("/Object", &dbus.Interface{Name: "org.example.Iface", …})
//	err = c.Emit("/Object", "org.example.Iface", "Changed", "u", uint32(2))
//
// Values are plain Go values of the types listed in wire.go, chosen by the
// signature the caller states; there is no reflection and no struct tags.
//
// It is internal because its API is shaped by its two users and nothing
// else, and because a public D-Bus package is a promise this project does
// not want to make.
package dbus
