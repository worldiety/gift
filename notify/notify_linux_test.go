//go:build linux && !android

package notify

import (
	"os"
	"testing"
	"time"
)

// This test talks to the real session bus and the real notification server
// and is off unless GIFT_NOTIFY_DBUS=1:
//
//	GIFT_NOTIFY_DBUS=1 go test ./notify/ -run DBus -v
//
// It shows two notifications under one ID, the second replacing the first,
// and withdraws it. With GIFT_NOTIFY_CLICK=<duration> it waits that long for
// a click on the notification and fails without one; that part needs a person
// at the screen, or `dunstctl action` under dunst.
//
// The gate is an environment variable and not a build tag, for the reason
// clipboard_x11_test.go gives: the file is compiled and vetted by every cross
// build for linux, and only the body is skipped.
func TestDBusNotificationServer(t *testing.T) {
	if os.Getenv("GIFT_NOTIFY_DBUS") != "1" {
		t.Skip("set GIFT_NOTIFY_DBUS=1 to talk to the session bus")
	}
	clicks := make(chan struct{}, 1)
	n := New(Config{
		AppID:   "org.worldiety.GiftNotifyTest",
		AppName: "gift notify test",
		Post:    func(fn func()) { fn() },
	})
	if err := n.Show(Notification{ID: "test", Title: "gift/notify", Body: "first <b>not bold</b> & co"}); err != nil {
		t.Fatal(err)
	}
	f := n.p.(*freedesktop)
	t.Logf("server: actions=%v body-markup=%v", f.actions, f.markup)
	first := f.byID["test"]

	err := n.Show(Notification{ID: "test", Title: "gift/notify", Body: "second, replacing the first",
		OnClick: func() { clicks <- struct{}{} }})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.byID["test"]; got != first {
		t.Errorf("the server gave the replacement id %d, not %d", got, first)
	}

	if d, err := time.ParseDuration(os.Getenv("GIFT_NOTIFY_CLICK")); err == nil {
		t.Logf("waiting %v for a click on the notification", d)
		select {
		case <-clicks:
			t.Log("clicked")
		case <-time.After(d):
			t.Fatal("no click arrived")
		}
	}

	if err := n.Withdraw("test"); err != nil {
		t.Fatal(err)
	}
}
