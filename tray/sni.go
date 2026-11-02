package tray

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/worldiety/gift/internal/dbus"
	"github.com/worldiety/gift/ui"
)

// This file is the Linux half of the tray, as far as it is protocol and not
// socket: a StatusNotifierItem on the session bus, the icon the panel shows.
// Its menu is the com.canonical.dbusmenu object of dbusmenu.go. Like
// gift/notify's freedesktop.go it carries **no build tag on purpose**: it sits
// on [sniBus] rather than on a connection, and is tested on the Mac this
// project is developed on against a bus that lives in the test.
// platform_linux.go only dials the session bus and hands the connection over.
//
// # The protocol
//
// StatusNotifierItem is the freedesktop.org draft that KDE wrote to replace
// the XEmbed system tray, which needed an X11 window per icon and does not
// exist under Wayland. It is the only status area protocol left on Linux:
// KDE Plasma, the AppIndicator extension of GNOME Shell, xfce4-panel,
// waybar, the LXQt panel and Budgie all host it. There are three roles:
//
//   - the item, this file: an object at /StatusNotifierItem, under a bus name
//     of the form org.kde.StatusNotifierItem-<pid>-<n>, with the icon, the
//     tooltip and a reference to the menu as properties;
//   - the watcher, org.kde.StatusNotifierWatcher, a registry that the panel
//     (or a daemon beside it) owns; an item calls its
//     RegisterStatusNotifierItem with its bus name, and the watcher forgets
//     it when the name goes away;
//   - the host, the panel, which learns of items from the watcher and reads
//     their properties.
//
// Nothing is pushed to the host: it reads the properties when an item
// registers, and again for the signals NewIcon, NewToolTip and NewTitle,
// which carry no arguments and only say "read again".
//
// # A desktop without a watcher
//
// GNOME Shell ships without one. Its users install the AppIndicator
// extension (gnome-shell-extension-appindicator, which Ubuntu enables by
// default) or see no tray at all. But an unowned watcher name at open does
// not tell that desktop apart from one whose panel has not started yet, and
// the second is common: a tray application is started at login, and so is
// the panel — `exec waybar` and `exec myapp` in the same sway config, an
// autostart entry that Plasma runs before plasmashell has claimed the name.
// The portable half opens a platform once, so reporting [ErrUnsupported]
// there would lose the icon for the life of the process because of a race
// of a few hundred milliseconds. A missing watcher is therefore "not yet":
// the item is created anyway, logs one warning that names the extension,
// and registers when NameOwnerChanged reports an owner for the watcher's
// name. On GNOME without the extension that never happens, and the warning
// is all there is, which is the degradation the package promises.
//
// A watcher that goes away later — the panel crashed, the user restarted it
// — is the same case: the item stays on the bus, watches NameOwnerChanged,
// and registers again with the new owner, which is what KDE's own
// applications do. While there is no watcher, nothing is sent to it: an
// application that updates its tooltip once a second must not make a round
// trip, and a warning, once a second until the panel is back.
//
// # The bus name
//
// The specification asks for org.kde.StatusNotifierItem-<pid>-<n>, and
// that is the name requested first. It is not always to be had. In a
// sandbox with a PID namespace of its own — Flatpak, bubblewrap — every
// application is pid 2, so two of them want the same name on the one session
// bus; the bus answers EXISTS, and the next <n> is tried, a few times. And
// the D-Bus proxy of a Flatpak refuses RequestName with AccessDenied when the
// manifest grants no --own-name for it. The watchers do not need the
// well-known name: KDE's, the GNOME extension's and waybar's all take the
// unique name of the connection (":1.42") in RegisterStatusNotifierItem and
// look for the item at /StatusNotifierItem under it, which is how
// libappindicator registers. That is the fallback.
//
// A unique name cannot be released short of closing the connection, so in
// that mode remove cannot make the watcher forget the item. It sets Status to
// "Passive" instead and sends NewStatus, and the next update sets it back to
// "Active". The GNOME extension and waybar hide a passive item; Plasma moves
// it into the expander of hidden icons, which is the compromise of the
// fallback and the reason it is not the rule (see below).
//
// # The click
//
// The contract of this package is that a click on the icon, left or right,
// opens the menu. The item says ItemIsMenu = true, which tells the host that
// the item has no action of its own and the menu is all there is; Plasma,
// the GNOME extension and waybar then open the menu on a left click too. A
// host that calls Activate anyway receives an UnknownMethod error, which is
// what Plasma takes as the cue to show the menu instead. ContextMenu, the
// request to draw a menu of the item's own, is answered and ignored: the
// host draws the dbusmenu itself, and there is no other menu.
//
// # Removing the icon
//
// The protocol has no "unregister". A watcher forgets an item when its bus
// name loses its owner, so remove releases the name, and the next update
// requests it again and registers anew. Setting Status to "Passive" was the
// alternative, and is used only where the name cannot be released (see
// above): Plasma shows passive items in the expander of hidden icons, so the
// icon would not be gone, only moved. The objects stay exported on the
// connection; nobody finds them without the name.
//
// # The icon
//
// IconPixmap, the picture itself rather than the name of a theme icon: a
// [ui.Symbol] is not in any icon theme. It is a list of sizes, each ARGB32
// in network byte order — the bytes A, R, G, B per pixel, which on the
// little endian machines this runs on is a big endian uint32 — with straight
// alpha, as the specification and every host's reader of it expect. The
// host picks the size nearest to the one it draws, so the list covers the
// sizes panels use: 16, 22 and 24 at a scale of 1, 32 and 48 for scaled
// displays and large panels. There is no way for an item to learn the scale
// of the panel, so offering the sizes is the whole of section 18 here.
//
// The colour is the hard part. A host does not recolour a pixmap (only theme
// icons of the "-symbolic" kind), and the item cannot ask what colour the
// panel is: GNOME's top bar is dark whatever the colour scheme, Plasma's
// follows the theme, waybar's is whatever the user's CSS says. Following
// org.freedesktop.appearance color-scheme from the settings portal was
// considered and rejected for that reason: it describes the windows, not the
// panel, and would be wrong on GNOME in exactly the case it claims to fix.
// So the glyph is near white, which is right on the dark bars that are the
// majority, with a one pixel outline of translucent black around it, which
// keeps it legible on a light one. The glyph is rasterised two pixels smaller
// than the picture to leave room for the outline.

// The names of the StatusNotifierItem protocol.
const (
	sniWatcherName  = "org.kde.StatusNotifierWatcher"
	sniWatcherPath  = dbus.ObjectPath("/StatusNotifierWatcher")
	sniWatcherIface = "org.kde.StatusNotifierWatcher"
	sniItemPath     = dbus.ObjectPath("/StatusNotifierItem")
	sniItemIface    = "org.kde.StatusNotifierItem"
	sniMenuPath     = dbus.ObjectPath("/MenuBar")
)

// sniIconSizes are the edges of the pictures in IconPixmap.
var sniIconSizes = []int{16, 22, 24, 32, 48}

// sniGlyph and sniOutline are the colours of the icon in the bar; see the
// comment at the top of this file.
var (
	sniGlyph   = color.NRGBA{R: 0xf4, G: 0xf4, B: 0xf4, A: 0xff}
	sniOutline = color.NRGBA{A: 0x8c}
)

// sniSerial numbers the items of this process, for the -<n> of the bus
// name: two Trays are two items and need two names.
var sniSerial atomic.Uint32

// sniNameTries is how many well-known names an item tries when the bus
// says the name exists, before it falls back to its unique name.
const sniNameTries = 4

// sniName is the next well-known name of this process.
func sniName() string {
	return fmt.Sprintf("org.kde.StatusNotifierItem-%d-%d", os.Getpid(), sniSerial.Add(1))
}

// sniBus is what the item needs of a [dbus.Conn], as an interface so that a
// test can stand in for the bus.
type sniBus interface {
	Call(dest string, path dbus.ObjectPath, iface, member string, sig dbus.Signature, args ...any) ([]any, error)
	Emit(path dbus.ObjectPath, iface, member string, sig dbus.Signature, args ...any) error
	Export(path dbus.ObjectPath, ifaces ...*dbus.Interface) error
	SetSignalHandler(fn func(*dbus.Message))
	UniqueName() string
}

// sni is the Linux platform: a StatusNotifierItem and its menu.
//
// # Threads
//
// Three goroutines meet here. update and remove are called by the portable
// half, from any goroutine, and only record what is wanted and wake the
// worker; they never wait for the bus. The worker, [sni.run], does every
// round trip — RequestName, RegisterStatusNotifierItem — and builds the
// pictures and the menu, which takes a few milliseconds that are not the
// caller's. The reader goroutine of the connection answers the host's
// calls from what the worker last published, and hands a click to the
// clicker, [sni.deliver], which calls clicked, as the seam requires that it
// is not called from where the platform was called. The clicker takes clicks
// from a small queue and drops what does not fit, so that a host, or any
// other peer on the bus, that sends a flood of Events costs neither a
// goroutine per click nor a flood of OnClick on the UI executor.
type sni struct {
	bus     sniBus
	log     *slog.Logger
	clicked func(gen uint64, id uint32)

	// id is the Id property of the item.
	id string

	kick   chan struct{}
	flushc chan chan struct{}
	clicks chan menuClick
	done   chan struct{}

	// The worker's own state, touched by no other goroutine.
	name       string // the bus name the item is registered under
	unique     bool   // name is the connection's unique name; see "The bus name"
	owned      bool   // the bus name is ours
	registered bool   // the watcher took the name
	refused    bool   // the watcher answered the registration with an error
	nextNode   int32
	quiet      map[string]bool // failures logged once, until they stop

	mu sync.Mutex

	// What update and remove asked for, and what the signal handler learnt,
	// read by the worker.
	want       state
	visible    bool
	reregister bool // a new watcher appeared
	watcher    bool // the watcher's name has an owner

	// What the worker published, read by the methods and getters on the
	// reader goroutine. menu is never changed once published, only replaced.
	gen      uint64
	title    string
	tooltip  string
	status   string
	pixmaps  []any
	menu     *menuLayout
	revision uint32
	// menuOf is the menu slice of the Set that menu was built from; see
	// [sni.publish].
	menuOf []entry
}

// menuClick is a click on the entry id of the state gen, on its way to the
// clicker.
type menuClick struct {
	gen uint64
	id  uint32
}

// sniClickQueue is how many clicks wait for the clicker before more are
// dropped. A person clicks one entry and the menu closes.
const sniClickQueue = 8

// newSNI subscribes to the comings and goings of the watcher, asks whether
// one is there, and exports the item and its menu. Nothing is shown until
// the first update. A missing watcher is logged and waited for; see "A
// desktop without a watcher".
//
// The signal handler is installed first and the subscription comes before
// the question, so that a watcher appearing between the two is not missed:
// its NameOwnerChanged is handled whenever it arrives, and the answer to
// NameHasOwner only adds to what the handler learnt. A watcher that went
// away in between is believed there until the registration fails, which
// waits for the next NameOwnerChanged as any refusal does.
func newSNI(bus sniBus, cfg openConfig) (*sni, error) {
	s := &sni{
		bus:      bus,
		log:      cfg.log,
		clicked:  cfg.clicked,
		name:     sniName(),
		id:       sniID(cfg.appID),
		kick:     make(chan struct{}, 1),
		flushc:   make(chan chan struct{}),
		clicks:   make(chan menuClick, sniClickQueue),
		done:     make(chan struct{}),
		nextNode: 1,
		quiet:    map[string]bool{},
		status:   "Active",
		menu:     emptyMenu(),
		revision: 1,
	}
	bus.SetSignalHandler(s.signal)
	rule := "type='signal',sender='" + dbus.BusName + "',interface='" + dbus.BusInterface +
		"',member='NameOwnerChanged',path='" + string(dbus.BusPath) + "',arg0='" + sniWatcherName + "'"
	if _, err := bus.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "AddMatch", "s", rule); err != nil {
		return nil, fmt.Errorf("%w: subscribing to NameOwnerChanged: %v", ErrUnsupported, err)
	}
	reply, err := bus.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "NameHasOwner", "s", sniWatcherName)
	if err != nil {
		return nil, fmt.Errorf("%w: asking the bus for %s: %v", ErrUnsupported, sniWatcherName, err)
	}
	s.mu.Lock()
	if len(reply) == 1 && reply[0] == true {
		s.watcher = true
	}
	watcher := s.watcher
	s.mu.Unlock()
	if !watcher {
		s.logf(slog.LevelWarn, "nobody owns "+sniWatcherName+" on the session bus, so no panel shows the tray "+
			"icon yet; it appears when one starts, and on GNOME that takes the AppIndicator extension "+
			"(gnome-shell-extension-appindicator)")
	}
	if err := bus.Export(sniItemPath, s.itemInterface()); err != nil {
		return nil, err
	}
	if err := bus.Export(sniMenuPath, s.menuInterface()); err != nil {
		return nil, err
	}
	go s.run()
	go s.deliver()
	return s, nil
}

// sniID is the Id of the item: the application id, or the name of the
// executable. Plasma remembers by it whether the user hid the icon, so it
// must be the same from one start to the next.
func sniID(appID string) string {
	if appID != "" {
		return appID
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Base(exe)
	}
	return filepath.Base(os.Args[0])
}

func (s *sni) update(st state) error {
	s.mu.Lock()
	s.want, s.visible = st, true
	s.mu.Unlock()
	s.wake()
	return nil
}

func (s *sni) remove() error {
	s.mu.Lock()
	s.visible = false
	s.mu.Unlock()
	s.wake()
	return nil
}

func (s *sni) wake() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// flush returns when the worker has applied everything asked for before
// the call. It is for the tests.
func (s *sni) flush() {
	ch := make(chan struct{})
	select {
	case s.flushc <- ch:
		<-ch
	case <-s.done:
	}
}

// stop ends the worker. The portable half never closes a tray; the tests
// do.
func (s *sni) stop() { close(s.done) }

// signal handles the signals of the bus. It runs on the reader goroutine and
// must not make a call, so it only records what changed and wakes the
// worker.
//
// Only the bus itself says who owns a name. The daemon delivers a signal
// addressed to this connection whatever its match rules, but it stamps the
// true sender on every message, so a NameOwnerChanged that any other peer
// sends is told apart by its Sender and ignored.
func (s *sni) signal(m *dbus.Message) {
	if m.Member != "NameOwnerChanged" || m.Interface != dbus.BusInterface || m.Sig != "sss" ||
		m.Sender != dbus.BusName || m.Path != dbus.BusPath {
		return
	}
	if m.Body[0] != sniWatcherName {
		return
	}
	owner, _ := m.Body[2].(string)
	s.mu.Lock()
	s.watcher = owner != ""
	if owner != "" {
		s.reregister = true
	}
	s.mu.Unlock()
	if owner == "" {
		s.logf(slog.LevelWarn, "the StatusNotifierWatcher went away; the tray icon is shown again when it is back")
	} else {
		s.logf(slog.LevelDebug, "a StatusNotifierWatcher appeared", "owner", owner)
	}
	s.wake()
}

func (s *sni) run() {
	for {
		select {
		case <-s.kick:
			s.step()
		case ch := <-s.flushc:
			select {
			case <-s.kick:
				s.step()
			default:
			}
			close(ch)
		case <-s.done:
			return
		}
	}
}

// click queues a click for the clicker, or drops it when the queue is full.
// It runs on the reader goroutine and does not block.
func (s *sni) click(gen uint64, id uint32) {
	select {
	case s.clicks <- menuClick{gen, id}:
	default:
		s.logf(slog.LevelDebug, "a tray menu click is dropped, too many are waiting", "id", id)
	}
}

// deliver is the clicker: it hands every queued click to clicked, one after
// the other.
func (s *sni) deliver() {
	for {
		select {
		case c := <-s.clicks:
			s.clicked(c.gen, c.id)
		case <-s.done:
			return
		}
	}
}

// step brings the bus in line with what was asked for last.
//
// A registration is attempted only while a watcher is there, and only once
// for each watcher that refused it: the next attempt waits for
// NameOwnerChanged, not for the next update.
func (s *sni) step() {
	s.mu.Lock()
	want, visible, reregister, watcher := s.want, s.visible, s.reregister, s.watcher
	s.reregister = false
	s.mu.Unlock()

	if reregister || !watcher {
		// A new watcher knows nothing of the item, and one that is gone has
		// forgotten it.
		s.registered, s.refused = false, false
	}
	if !visible {
		s.hide()
		return
	}
	icon, tip, title, menu := s.publish(want)
	status := s.setStatus("Active")
	if s.owned && s.registered {
		s.emitChanges(icon, tip, title, menu, status)
		return
	}
	if !watcher || s.refused {
		return
	}
	if !s.owned && !s.acquire() {
		return
	}
	s.register()
}

// publish makes want what the host reads, and reports what changed.
//
// The menu is built anew, with new ids and a LayoutUpdated, for every Set,
// even one of the same entries, and kept for SetIcon and SetTooltip, which
// hand on the slice of the last Set ([sameMenu]); that is the rule of the
// other platforms. A click in the kept menu is reported under the newest
// generation, and one in a replaced menu names an id that no longer exists.
func (s *sni) publish(want state) (icon, tip, title, menu bool) {
	pixmaps := sniPixmaps(want.icon)
	s.mu.Lock()
	keep := s.menuOf != nil && sameMenu(s.menuOf, want.menu)
	s.mu.Unlock()
	var next *menuLayout
	if !keep {
		next = buildMenu(want.menu, &s.nextNode)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.gen = want.gen
	newTitle := want.tooltip
	if newTitle == "" {
		newTitle = s.id
	}
	icon = !slices.EqualFunc(s.pixmaps, pixmaps, pixmapEqual)
	tip = s.tooltip != want.tooltip
	title = s.title != newTitle
	s.pixmaps, s.tooltip, s.title = pixmaps, want.tooltip, newTitle
	if !keep {
		s.menu, s.menuOf = next, want.menu
		if s.menuOf == nil {
			s.menuOf = []entry{}
		}
		s.revision++
		menu = true
	}
	return icon, tip, title, menu
}

// setStatus publishes the Status property and reports whether it changed.
// It is "Active" but for the hidden item of the unique name fallback.
func (s *sni) setStatus(st string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := s.status != st
	s.status = st
	return changed
}

// emitChanges sends a signal for every property that changed. A connection
// that is gone fails every one of them, every update; that is logged once
// until a signal goes out again.
func (s *sni) emitChanges(icon, tip, title, menu, status bool) {
	emit := func(path dbus.ObjectPath, iface, member string, sig dbus.Signature, args ...any) {
		if err := s.bus.Emit(path, iface, member, sig, args...); err != nil {
			s.failed("emit", slog.LevelError, "sending a tray signal failed", "signal", member, "error", err)
			return
		}
		s.recovered("emit")
	}
	if icon {
		emit(sniItemPath, sniItemIface, "NewIcon", "")
	}
	if title {
		emit(sniItemPath, sniItemIface, "NewTitle", "")
	}
	if tip {
		emit(sniItemPath, sniItemIface, "NewToolTip", "")
	}
	if status {
		s.mu.Lock()
		st := s.status
		s.mu.Unlock()
		emit(sniItemPath, sniItemIface, "NewStatus", "s", st)
	}
	if menu {
		s.mu.Lock()
		rev := s.revision
		s.mu.Unlock()
		emit(sniMenuPath, menuIface, "LayoutUpdated", "ui", rev, int32(0))
	}
}

// acquire requests a bus name for the item: the well-known one, the next
// ones when the bus says it exists, and the unique name of the connection
// when none is to be had or the bus refuses; see "The bus name". A call that
// does not get through at all is not a refusal and is tried again with the
// next update.
func (s *sni) acquire() bool {
	for range sniNameTries {
		reply, err := s.bus.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "RequestName", "su",
			s.name, uint32(dbus.NameFlagDoNotQueue))
		var refused *dbus.Error
		if errors.As(err, &refused) {
			s.logf(slog.LevelDebug, "the bus refused the name of the tray icon", "name", s.name, "error", err)
			break
		}
		if err != nil {
			s.failed("acquire", slog.LevelError, "requesting the bus name of the tray icon failed",
				"name", s.name, "error", err)
			return false
		}
		s.recovered("acquire")
		if len(reply) == 1 && (reply[0] == uint32(dbus.RequestNamePrimaryOwner) ||
			reply[0] == uint32(dbus.RequestNameAlreadyOwner)) {
			s.owned = true
			return true
		}
		s.logf(slog.LevelDebug, "the name of the tray icon is taken", "name", s.name, "reply", reply)
		s.name = sniName()
	}
	unique := s.bus.UniqueName()
	if unique == "" {
		s.failed("acquire", slog.LevelError, "the tray icon has no bus name", "name", s.name)
		return false
	}
	s.logf(slog.LevelDebug, "the tray icon registers under the unique name of its connection", "name", unique)
	s.name, s.unique, s.owned = unique, true, true
	return true
}

// hide takes the icon away: by releasing the bus name, which is what makes
// the watcher forget the item, or, for the unique name that cannot be
// released, by making the item passive.
func (s *sni) hide() {
	if !s.owned {
		return
	}
	if s.unique {
		if s.setStatus("Passive") && s.registered {
			s.emitChanges(false, false, false, false, true)
		}
		return
	}
	s.owned, s.registered, s.refused = false, false, false
	if _, err := s.bus.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "ReleaseName", "s", s.name); err != nil {
		s.logf(slog.LevelError, "releasing the bus name of the tray icon failed", "name", s.name, "error", err)
	}
}

// register hands the bus name to the watcher. A watcher that refuses is not
// asked again until a new one appears; a call that does not get through is
// tried again with the next update. Either is logged once.
func (s *sni) register() {
	_, err := s.bus.Call(sniWatcherName, sniWatcherPath, sniWatcherIface, "RegisterStatusNotifierItem", "s", s.name)
	if err != nil {
		var refused *dbus.Error
		s.refused = errors.As(err, &refused)
		s.failed("register", slog.LevelWarn,
			"the StatusNotifierWatcher did not take the tray icon; it is shown when a panel starts", "error", err)
		return
	}
	s.recovered("register")
	s.registered = true
}

// failed logs a failure of the worker the first time it happens, and not
// again until [sni.recovered] says it stopped.
func (s *sni) failed(key string, level slog.Level, msg string, args ...any) {
	if s.quiet[key] {
		return
	}
	s.quiet[key] = true
	s.logf(level, msg, args...)
}

func (s *sni) recovered(key string) { delete(s.quiet, key) }

func (s *sni) logf(level slog.Level, msg string, args ...any) {
	if s.log == nil {
		return
	}
	s.log.Log(context.Background(), level, msg, args...)
}

// itemInterface is org.kde.StatusNotifierItem, from the specification's
// list of properties, methods and signals. The properties an item without
// attention or overlay icons does not need are there anyway, empty: Plasma
// reads them all with GetAll and logs the ones that are missing.
func (s *sni) itemInterface() *dbus.Interface {
	str := func(f func() string) dbus.Property { return dbus.Property{Sig: "s", Get: func() any { return f() }} }
	constant := func(sig dbus.Signature, v any) dbus.Property {
		return dbus.Property{Sig: sig, Get: func() any { return v }}
	}
	none := func(*dbus.Message) ([]any, error) { return nil, nil }
	return &dbus.Interface{
		Name: sniItemIface,
		Properties: map[string]dbus.Property{
			"Category":            constant("s", "ApplicationStatus"),
			"Id":                  constant("s", s.id),
			"Title":               str(func() string { s.mu.Lock(); defer s.mu.Unlock(); return s.title }),
			"Status":              str(func() string { s.mu.Lock(); defer s.mu.Unlock(); return s.status }),
			"WindowId":            constant("i", int32(0)),
			"IconName":            constant("s", ""),
			"IconThemePath":       constant("s", ""),
			"IconPixmap":          {Sig: "a(iiay)", Get: func() any { s.mu.Lock(); defer s.mu.Unlock(); return s.pixmaps }},
			"OverlayIconName":     constant("s", ""),
			"OverlayIconPixmap":   constant("a(iiay)", []any{}),
			"AttentionIconName":   constant("s", ""),
			"AttentionIconPixmap": constant("a(iiay)", []any{}),
			"AttentionMovieName":  constant("s", ""),
			"ToolTip": {Sig: "(sa(iiay)ss)", Get: func() any {
				s.mu.Lock()
				defer s.mu.Unlock()
				return []any{"", []any{}, s.tooltip, ""}
			}},
			"ItemIsMenu": constant("b", true),
			"Menu":       constant("o", sniMenuPath),
		},
		Methods: map[string]dbus.Method{
			"Activate": {In: "ii", Call: func(*dbus.Message) ([]any, error) {
				return nil, &dbus.Error{Name: dbus.ErrorUnknownMethod,
					Message: "this item is a menu (ItemIsMenu), it has no action of its own"}
			}},
			"SecondaryActivate": {In: "ii", Call: none},
			"ContextMenu":       {In: "ii", Call: none},
			"Scroll":            {In: "is", Call: none},
		},
		Signals: map[string]dbus.Signature{
			"NewTitle":         "",
			"NewIcon":          "",
			"NewAttentionIcon": "",
			"NewOverlayIcon":   "",
			"NewToolTip":       "",
			"NewStatus":        "s",
		},
	}
}

// sniPixmaps is the IconPixmap of sym: one (width, height, ARGB32) per size
// of [sniIconSizes], skipping a size at which sym draws nothing.
func sniPixmaps(sym ui.Symbol) []any {
	out := []any{}
	for _, px := range sniIconSizes {
		im := sniOutlined(sym, px)
		if im == nil {
			continue
		}
		out = append(out, []any{int32(px), int32(px), argb32(im)})
	}
	return out
}

// sniOutlined is sym at px by px in [sniGlyph] with a one pixel outline of
// [sniOutline] around it, in straight alpha. The glyph is rasterised at px-2
// and centred, so that the outline has room at the edges.
func sniOutlined(sym ui.Symbol, px int) *image.NRGBA {
	inner := px - 2
	if inner < 1 {
		return nil
	}
	m := sym.Rasterize(inner)
	if m == nil {
		return nil
	}
	n := m.Rect.Dx()
	cov := func(x, y int) int {
		x, y = x-1, y-1
		if x < 0 || y < 0 || x >= n || y >= n {
			return 0
		}
		return int(m.Pix[y*m.Stride+x])
	}
	out := image.NewNRGBA(image.Rect(0, 0, px, px))
	for y := range px {
		for x := range px {
			g := cov(x, y)
			// The outline is the coverage dilated by one pixel: the
			// largest coverage among the pixel and its eight neighbours.
			d := 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					d = max(d, cov(x+dx, y+dy))
				}
			}
			// The glyph over the outline, "source over" in straight alpha.
			ga := g * int(sniGlyph.A) / 255
			oa := d * int(sniOutline.A) / 255
			a := ga + oa*(255-ga)/255
			j := out.PixOffset(x, y)
			if a == 0 {
				continue
			}
			blend := func(gc, oc uint8) uint8 {
				return uint8((int(gc)*ga + int(oc)*oa*(255-ga)/255 + a/2) / a)
			}
			out.Pix[j] = blend(sniGlyph.R, sniOutline.R)
			out.Pix[j+1] = blend(sniGlyph.G, sniOutline.G)
			out.Pix[j+2] = blend(sniGlyph.B, sniOutline.B)
			out.Pix[j+3] = uint8(a)
		}
	}
	return out
}

// argb32 is im in the pixel format of IconPixmap: A, R, G, B per pixel, row
// by row, straight alpha.
func argb32(im *image.NRGBA) []byte {
	w, h := im.Rect.Dx(), im.Rect.Dy()
	out := make([]byte, 0, w*h*4)
	for y := range h {
		row := im.Pix[y*im.Stride : y*im.Stride+w*4]
		for i := 0; i < len(row); i += 4 {
			out = append(out, row[i+3], row[i], row[i+1], row[i+2])
		}
	}
	return out
}

func pixmapEqual(a, b any) bool {
	x, y := a.([]any), b.([]any)
	return x[0] == y[0] && x[1] == y[1] && string(x[2].([]byte)) == string(y[2].([]byte))
}
