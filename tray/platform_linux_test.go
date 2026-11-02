//go:build linux && !android

package tray

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/internal/dbus"
)

// This test talks to a real dbus-daemon and is off unless GIFT_TRAY_DBUS=1:
//
//	dbus-run-session -- env GIFT_TRAY_DBUS=1 go test ./tray/ -run DBus -v
//
// It needs a session bus and no panel: the StatusNotifierWatcher is played
// by the test itself, on a connection of its own, and the host — the panel
// that reads the icon and the menu and sends the click — by a third. That
// is what a desktop does, minus the drawing, and it is what can run in a
// container. Whether Plasma or the GNOME extension draw what this sends is a
// question for a person at the screen.
//
// The gate is an environment variable and not a build tag, for the reason
// clipboard_x11_test.go gives: the file is compiled and vetted by every cross
// build for linux, and only the body is skipped.
func TestDBusStatusNotifierItem(t *testing.T) {
	if os.Getenv("GIFT_TRAY_DBUS") != "1" {
		t.Skip("set GIFT_TRAY_DBUS=1 to talk to the session bus")
	}
	addr, err := dbus.SessionBusAddress(os.Getenv, func(p string) bool { _, err := os.Stat(p); return err == nil })
	if err != nil {
		t.Fatal(err)
	}
	dial := func() *dbus.Conn {
		c, err := dbus.Dial(addr, 2*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(c.Close)
		return c
	}

	// The application starts before the panel, as at login: Set succeeds,
	// and the item registers once the watcher is there.
	clicked := make(chan string, 4)
	tr := New(Config{
		AppID:   "org.worldiety.GiftTrayTest",
		Icon:    outline.User,
		Tooltip: "gift tray test",
		Post:    func(fn func()) { fn() },
	})
	err = tr.Set([]Item{
		{Title: "Show_window", Symbol: outline.User, OnClick: func() { clicked <- "show" }},
		Separator(),
		{Title: "Import", Items: []Item{{Title: "From camera", OnClick: func() { clicked <- "camera" }}}},
		{Title: "Quit", OnClick: func() { clicked <- "quit" }},
	})
	if err != nil {
		t.Fatalf("Set before there is a watcher: %v", err)
	}

	// The watcher.
	watcher := dial()
	type registration struct{ sender, service string }
	registered := make(chan registration, 4)
	err = watcher.Export(sniWatcherPath, &dbus.Interface{
		Name: sniWatcherIface,
		Methods: map[string]dbus.Method{
			"RegisterStatusNotifierItem": {In: "s", Call: func(m *dbus.Message) ([]any, error) {
				registered <- registration{m.Sender, m.Body[0].(string)}
				return nil, nil
			}},
		},
		Properties: map[string]dbus.Property{
			"IsStatusNotifierHostRegistered": {Sig: "b", Get: func() any { return true }},
			"ProtocolVersion":                {Sig: "i", Get: func() any { return int32(0) }},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := watcher.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "RequestName", "su",
		sniWatcherName, uint32(dbus.NameFlagDoNotQueue))
	if err != nil || reply[0] != uint32(dbus.RequestNamePrimaryOwner) {
		t.Fatalf("RequestName of the watcher: %v %v", reply, err)
	}

	var reg registration
	select {
	case reg = <-registered:
	case <-time.After(3 * time.Second):
		t.Fatal("the item did not register with the watcher")
	}
	if !strings.HasPrefix(reg.service, "org.kde.StatusNotifierItem-") {
		t.Fatalf("registered %q", reg.service)
	}
	t.Logf("registered %s from %s", reg.service, reg.sender)

	// The host.
	host := dial()
	signals := make(chan *dbus.Message, 16)
	host.SetSignalHandler(func(m *dbus.Message) { signals <- m })
	for _, rule := range []string{
		"type='signal',sender='" + reg.service + "'",
		"type='signal',sender='org.freedesktop.DBus',member='NameOwnerChanged',arg0='" + reg.service + "'",
	} {
		if _, err := host.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "AddMatch", "s", rule); err != nil {
			t.Fatal(err)
		}
	}
	reply, err = host.Call(reg.service, sniItemPath, dbus.PropertiesInterface, "GetAll", "s", sniItemIface)
	if err != nil {
		t.Fatal(err)
	}
	props := map[string]any{}
	for _, e := range reply[0].([]dbus.DictEntry) {
		props[e.Key.(string)] = e.Value.(dbus.Variant).Value
	}
	if props["ItemIsMenu"] != true || props["Menu"] != sniMenuPath || props["Id"] != "org.worldiety.GiftTrayTest" ||
		props["Title"] != "gift tray test" || len(props["IconPixmap"].([]any)) != len(sniIconSizes) {
		t.Fatalf("properties %v", props)
	}
	reply, err = host.Call(reg.service, sniMenuPath, dbus.IntrospectableInterface, "Introspect", "")
	if err != nil || !strings.Contains(reply[0].(string), `<interface name="com.canonical.dbusmenu">`) {
		t.Fatalf("Introspect: %v %v", reply, err)
	}

	// The menu, and a click on Quit.
	reply, err = host.Call(reg.service, sniMenuPath, menuIface, "GetLayout", "iias", int32(0), int32(-1), []string{})
	if err != nil {
		t.Fatal(err)
	}
	root := decodeLayout(t, reply[1])
	if len(root.children) != 4 || root.children[0].props["label"] != "Show__window" ||
		root.children[3].props["label"] != "Quit" || len(root.children[2].children) != 1 {
		t.Fatalf("layout %+v", root)
	}
	_, err = host.Call(reg.service, sniMenuPath, menuIface, "Event", "isvu",
		root.children[3].id, "clicked", dbus.Variant{Sig: "s", Value: ""}, uint32(0))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-clicked:
		if got != "quit" {
			t.Fatalf("clicked %q, want quit", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the click did not arrive")
	}

	// A new tooltip is announced.
	if err := tr.SetTooltip("changed"); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, signals, "NewToolTip")

	// Remove gives the name up, which is how the watcher learns of it.
	if err := tr.Remove(); err != nil {
		t.Fatal(err)
	}
	m := waitSignal(t, signals, "NameOwnerChanged")
	if m.Body[0] != reg.service || m.Body[2] != "" {
		t.Fatalf("NameOwnerChanged %v", m.Body)
	}
	if err := tr.Set([]Item{{Title: "Back", OnClick: func() {}}}); err != nil {
		t.Fatal(err)
	}
	select {
	case again := <-registered:
		if again.service != reg.service {
			t.Fatalf("registered again as %s, not %s", again.service, reg.service)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the item did not register again after Remove and Set")
	}

	// A second tray of the process whose well-known names are all taken —
	// the case of two sandboxed applications that are both pid 2 — registers
	// under the unique name of its connection, and Remove makes it passive.
	squatter := dial()
	next := sniSerial.Load()
	for i := range uint32(sniNameTries) {
		name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-%d", os.Getpid(), next+1+i)
		reply, err := squatter.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "RequestName", "su",
			name, uint32(dbus.NameFlagDoNotQueue))
		if err != nil || reply[0] != uint32(dbus.RequestNamePrimaryOwner) {
			t.Fatalf("RequestName %s: %v %v", name, reply, err)
		}
	}
	tr2 := New(Config{Icon: outline.User, Post: func(fn func()) { fn() }})
	if err := tr2.Set([]Item{{Title: "Second", OnClick: func() {}}}); err != nil {
		t.Fatal(err)
	}
	select {
	case second := <-registered:
		if !strings.HasPrefix(second.service, ":") || second.service != second.sender {
			t.Fatalf("the second tray registered %q from %s, want its unique name", second.service, second.sender)
		}
		if _, err := host.Call(dbus.BusName, dbus.BusPath, dbus.BusInterface, "AddMatch", "s",
			"type='signal',sender='"+second.sender+"',member='NewStatus'"); err != nil {
			t.Fatal(err)
		}
		if err := tr2.Remove(); err != nil {
			t.Fatal(err)
		}
		if m := waitSignal(t, signals, "NewStatus"); m.Body[0] != "Passive" {
			t.Fatalf("NewStatus %v, want Passive", m.Body)
		}
		reply, err := host.Call(second.service, sniItemPath, dbus.PropertiesInterface, "Get", "ss", sniItemIface, "Status")
		if err != nil || reply[0].(dbus.Variant).Value != "Passive" {
			t.Fatalf("Status %v %v", reply, err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the second tray did not register")
	}
}

func waitSignal(t *testing.T, signals chan *dbus.Message, member string) *dbus.Message {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case m := <-signals:
			if m.Member == member {
				return m
			}
		case <-deadline:
			t.Fatalf("no %s signal", member)
			return nil
		}
	}
}
