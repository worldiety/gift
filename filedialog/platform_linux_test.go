//go:build linux && !android

package filedialog

import (
	"errors"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/worldiety/gift/internal/dbus"
)

// The gates are environment variables and not build tags, for the reason
// clipboard_x11_test.go gives: this file is compiled and vetted by every
// cross build for linux, and only the bodies are skipped.

// TestLinuxOtherDesktopsShowNothing needs no bus: a session that is not
// GNOME or KDE, or a gift kiosk, has no native dialog, and the dialog
// answers with onCancel.
func TestLinuxOtherDesktopsShowNothing(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"XFCE":  {"XDG_CURRENT_DESKTOP": "XFCE", "DBUS_SESSION_BUS_ADDRESS": "unix:path=/nonexistent"},
		"kiosk": {"XDG_CURRENT_DESKTOP": "GNOME", "DBUS_SESSION_BUS_ADDRESS": "unix:path=/nonexistent", kioskEnv: "1"},
	} {
		getenv := func(k string) string { return env[k] }
		op := newOpener(openPlatform, func() bool { return portalAvailable(getenv, fileExists) }, getenv)
		if op.available() {
			t.Fatalf("%s: available", name)
		}
		var o outcome
		mb := newMailbox()
		d := New(o.pick, o.cancel)
		d.op = op
		if _, err := d.Open(mb.post); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%s: Open: %v, want ErrUnsupported", name, err)
		}
		mb.run(t)
		if o.cancelled != 1 || len(o.picked) != 0 {
			t.Fatalf("%s: got %+v", name, o)
		}
	}
}

// fakeAnswer is what the fake portal does with the next OpenFile or
// SaveFile: emit a Response with code and uris, or nothing, waiting for a
// Close.
type fakeAnswer struct {
	code   uint32
	uris   []string
	silent bool
}

// portalCall is an OpenFile or SaveFile the fake portal received.
type portalCall struct {
	member, title string
	opts          map[string]dbus.Variant
	handle        dbus.ObjectPath
}

// TestDBusPortal talks to a real dbus-daemon and is off unless
// GIFT_FILEDIALOG_DBUS=1:
//
//	docker run --rm --platform linux/arm64 -v "$PWD":/src -w /src golang:1.27-trixie sh -c 'apt-get update -qq && apt-get install -y -qq dbus && dbus-run-session -- env GIFT_FILEDIALOG_DBUS=1 go test ./filedialog/ -run DBus -v'
//
// It needs a session bus and no desktop: org.freedesktop.portal.Desktop is
// played by the test itself, on a connection of its own, with the version
// property, OpenFile, SaveFile and a Request object per call whose Response
// is sent after the reply, as xdg-desktop-portal does. What the dialog looks
// like on GNOME and KDE, and what a person picks in it, is
// TestInteractivePortal, for a person at the screen.
func TestDBusPortal(t *testing.T) {
	if os.Getenv("GIFT_FILEDIALOG_DBUS") != "1" {
		t.Skip("set GIFT_FILEDIALOG_DBUS=1 to talk to the session bus")
	}
	addr, err := dbus.SessionBusAddress(os.Getenv, fileExists)
	if err != nil {
		t.Fatal(err)
	}
	fake, err := dbus.Dial(addr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fake.Close)

	var version atomic.Uint32
	version.Store(4)
	answers := make(chan fakeAnswer, 8)
	calls := make(chan portalCall, 8)
	closes := make(chan dbus.ObjectPath, 8)
	method := func(member string) dbus.Method {
		return dbus.Method{In: "ssa{sv}", Out: "o", Call: func(m *dbus.Message) ([]any, error) {
			opts := map[string]dbus.Variant{}
			for _, e := range m.Body[2].([]dbus.DictEntry) {
				opts[e.Key.(string)] = e.Value.(dbus.Variant)
			}
			token, _ := opts["handle_token"].Value.(string)
			handle := requestPath(m.Sender, token)
			err := fake.Export(handle, &dbus.Interface{Name: requestIface, Methods: map[string]dbus.Method{
				"Close": {Call: func(*dbus.Message) ([]any, error) {
					closes <- handle
					fake.Unexport(handle)
					return nil, nil
				}},
			}})
			if err != nil {
				return nil, err
			}
			calls <- portalCall{member: member, title: m.Body[1].(string), opts: opts, handle: handle}
			a := fakeAnswer{silent: true}
			select {
			case a = <-answers:
			default:
			}
			if !a.silent {
				// The reply is written when this function returns; the
				// Response comes after it, as from the real portal.
				go func() {
					time.Sleep(50 * time.Millisecond)
					uris := make([]any, 0, len(a.uris))
					for _, u := range a.uris {
						uris = append(uris, u)
					}
					fake.Emit(handle, requestIface, "Response", "ua{sv}", a.code,
						[]dbus.DictEntry{{Key: "uris", Value: dbus.Variant{Sig: "as", Value: uris}}})
					fake.Unexport(handle)
				}()
			}
			return []any{handle}, nil
		}}
	}
	err = fake.Export(portalPath, &dbus.Interface{
		Name:       fileChooserIface,
		Methods:    map[string]dbus.Method{"OpenFile": method("OpenFile"), "SaveFile": method("SaveFile")},
		Properties: map[string]dbus.Property{"version": {Sig: "u", Get: func() any { return version.Load() }}},
	})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := fake.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "RequestName", "su",
		portalName, uint32(dbus.NameFlagDoNotQueue))
	if err != nil || reply[0] != uint32(dbus.RequestNamePrimaryOwner) {
		t.Fatalf("RequestName of the portal: %v %v", reply, err)
	}

	getenv := func(k string) string {
		if k == "XDG_CURRENT_DESKTOP" {
			return "GNOME"
		}
		return os.Getenv(k)
	}
	newOp := func() *opener {
		return newOpener(openPlatform, func() bool { return portalAvailable(getenv, fileExists) }, getenv)
	}
	op := newOp()
	if !op.available() {
		t.Fatal("not available on GNOME with a session bus")
	}
	var logs records
	var o outcome
	mb := newMailbox()
	dialog := func(m Mode) Dialog {
		d := New(o.pick, o.cancel).Mode(m).Logger(slog.New(&logs))
		d.op = op
		return d
	}
	waitCall := func() portalCall {
		t.Helper()
		select {
		case c := <-calls:
			return c
		case <-time.After(5 * time.Second):
			t.Fatal("the portal was not called")
		}
		return portalCall{}
	}
	tmp := t.TempDir()
	uri := func(p string) string { return (&url.URL{Scheme: "file", Path: p}).String() }

	// Open, with a start folder and a filter.
	answers <- fakeAnswer{uris: []string{uri(filepath.Join(tmp, "a b.jpg"))}}
	if _, err := dialog(Open).Title("Pick one").Start(tmp).Extensions(".JPG").Open(mb.post); err != nil {
		t.Fatal(err)
	}
	c := waitCall()
	if c.member != "OpenFile" || c.title != "Pick one" || string(c.opts["current_folder"].Value.([]byte)) != tmp+"\x00" ||
		c.opts["multiple"].Sig != "" || c.opts["directory"].Sig != "" {
		t.Fatalf("Open sent %+v", c)
	}
	mb.run(t)
	if len(o.picked) != 1 || !slices.Equal(o.picked[0], []string{filepath.Join(tmp, "a b.jpg")}) {
		t.Fatalf("Open: %+v", o)
	}

	// OpenMulti.
	answers <- fakeAnswer{uris: []string{uri("/x/1"), uri("/x/2")}}
	dialog(OpenMulti).Open(mb.post)
	if c := waitCall(); c.opts["multiple"].Value != true {
		t.Fatalf("OpenMulti sent %+v", c)
	}
	mb.run(t)
	if len(o.picked) != 2 || !slices.Equal(o.picked[1], []string{"/x/1", "/x/2"}) {
		t.Fatalf("OpenMulti: %+v", o)
	}

	// Directory, reported relative to a root.
	answers <- fakeAnswer{uris: []string{uri(tmp)}}
	dialog(Directory).Root(tmp).Open(mb.post)
	if c := waitCall(); c.member != "OpenFile" || c.opts["directory"].Value != true || c.opts["filters"].Sig != "" {
		t.Fatalf("Directory sent %+v", c)
	}
	mb.run(t)
	if len(o.picked) != 3 || !slices.Equal(o.picked[2], []string{"."}) {
		t.Fatalf("Directory: %+v", o)
	}

	// Save, with a name, in a root.
	answers <- fakeAnswer{uris: []string{uri(filepath.Join(tmp, "out.txt"))}}
	dialog(Save).Root(tmp).SuggestedName("out.txt").Extensions("txt").Open(mb.post)
	c = waitCall()
	if c.member != "SaveFile" || c.opts["current_name"].Value != "out.txt" ||
		string(c.opts["current_folder"].Value.([]byte)) != tmp+"\x00" || c.opts["filters"].Sig != "a(sa(us))" {
		t.Fatalf("Save sent %+v", c)
	}
	mb.run(t)
	if len(o.picked) != 4 || !slices.Equal(o.picked[3], []string{"out.txt"}) {
		t.Fatalf("Save: %+v", o)
	}

	// The person backs out.
	answers <- fakeAnswer{code: 1}
	dialog(Open).Open(mb.post)
	waitCall()
	mb.run(t)
	if o.cancelled != 1 {
		t.Fatalf("cancel: %+v", o)
	}

	// Closed from code: Request.Close, no callback, and the dialog is free
	// again at once.
	cancel, err := dialog(Open).Open(mb.post)
	if err != nil {
		t.Fatal(err)
	}
	c = waitCall()
	cancel()
	select {
	case h := <-closes:
		if h != c.handle {
			t.Fatalf("Close on %s, want %s", h, c.handle)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Request.Close did not arrive")
	}
	waitFree(t, op)
	mb.drain()
	if o.cancelled != 1 || len(o.picked) != 4 {
		t.Fatalf("after close: %+v", o)
	}

	// Version 2: no folders, files still, and no current_folder in
	// OpenFile. A new opener opens a new connection and reads the version
	// anew.
	version.Store(2)
	op = newOp()
	dialog(Directory).Open(mb.post)
	mb.run(t)
	if o.cancelled != 2 || !op.available() {
		t.Fatalf("Directory on version 2: %+v, available %v", o, op.available())
	}
	answers <- fakeAnswer{uris: []string{uri("/v2")}}
	dialog(Open).Start(tmp).Open(mb.post)
	if c := waitCall(); c.opts["current_folder"].Sig != "" {
		t.Fatalf("OpenFile on version 2 sent %+v", c)
	}
	mb.run(t)
	if len(o.picked) != 5 || !slices.Equal(o.picked[4], []string{"/v2"}) {
		t.Fatalf("Open on version 2: %+v", o)
	}
	if logs.count(slog.LevelError) != 0 {
		t.Fatalf("%d records at Error", logs.count(slog.LevelError))
	}

	// No portal at all: a lasting failure.
	if _, err := fake.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "ReleaseName", "s", portalName); err != nil {
		t.Fatal(err)
	}
	op = newOp()
	dialog(Open).Open(mb.post)
	mb.run(t)
	if o.cancelled != 3 || op.available() || logs.count(slog.LevelError) != 1 {
		t.Fatalf("without a portal: %+v, available %v, %d at Error", o, op.available(), logs.count(slog.LevelError))
	}
}

// TestInteractivePortal opens the real dialog of the desktop, for a person
// at the screen of a GNOME or KDE session; it is off unless
// GIFT_FILEDIALOG_PORTAL=1:
//
//	GIFT_FILEDIALOG_PORTAL=1 go test ./filedialog/ -run InteractivePortal -v
//
// Each mode is opened in turn and waits up to a minute for the person to
// pick or back out; what was picked is logged. The last dialog is closed
// from code after five seconds.
func TestInteractivePortal(t *testing.T) {
	if os.Getenv("GIFT_FILEDIALOG_PORTAL") != "1" {
		t.Skip("set GIFT_FILEDIALOG_PORTAL=1 in a GNOME or KDE session to open the real dialog")
	}
	op := newOpener(openPlatform, platformAvailable, os.Getenv)
	if !op.available() {
		t.Fatal("not a GNOME or KDE session with a session bus")
	}
	home, _ := os.UserHomeDir()
	answered := make(chan string, 1)
	for _, m := range []Mode{Open, OpenMulti, Directory, Save} {
		d := New(func(p []string) { answered <- "picked " + strings.Join(p, ", ") },
			func() { answered <- "cancelled" }).
			Mode(m).Start(home).Extensions(".txt", ".png").SuggestedName("gift-test.txt").
			Logger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
		d.op = op
		if _, err := d.Open(func(fn func()) { fn() }); err != nil {
			t.Fatal(err)
		}
		select {
		case s := <-answered:
			t.Logf("mode %d: %s", m, s)
		case <-time.After(time.Minute):
			t.Fatalf("mode %d: no answer within a minute", m)
		}
	}
	d := New(func([]string) { answered <- "picked" }, func() { answered <- "cancelled" })
	d.op = op
	cancel, err := d.Open(func(fn func()) { fn() })
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Second)
	cancel()
	select {
	case s := <-answered:
		t.Fatalf("a dialog closed from code reported %s", s)
	case <-time.After(time.Second):
	}
}
