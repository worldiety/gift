//go:build windows

package notify

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// TestGUIDLayoutMatchesTheRegistrySpelling: the class id is written into the
// registry as text (guidString) and handed to CoRegisterClassObject as a GUID
// (guidOf). If the two disagreed, every click would go to a class nobody
// registered.
func TestGUIDLayoutMatchesTheRegistrySpelling(t *testing.T) {
	u := activatorCLSID("com.example.Photos")
	if got, want := guidOf(u).String(), guidString(u); got != want {
		t.Fatalf("GUID %s, registry %s", got, want)
	}
	if got := mustGUID("{53E31837-6600-4A81-9395-75CFFE746F94}").String(); got != "{53E31837-6600-4A81-9395-75CFFE746F94}" {
		t.Fatalf("mustGUID parsed %s", got)
	}
}

// TestWindowsToasts talks to the real notification platform and is off
// unless GIFT_NOTIFY_WINDOWS=1:
//
//	set GIFT_NOTIFY_WINDOWS=1
//	set GIFT_NOTIFY_CLICK=60s
//	notify.test.exe -test.run Windows -test.v
//
// It registers the AppUserModelID dev.gift.notify-test for the current user,
// shows a toast, replaces it under the same ID and withdraws it at the end.
// With GIFT_NOTIFY_CLICK it waits that long for a click on the toast; with
// GIFT_NOTIFY_UNREGISTER=1 it removes the registry entry afterwards.
func TestWindowsToasts(t *testing.T) {
	if os.Getenv("GIFT_NOTIFY_WINDOWS") != "1" {
		t.Skip("set GIFT_NOTIFY_WINDOWS=1 to show real toasts")
	}
	cfg := Config{AppID: "dev.gift.notify-test", AppName: "gift notify test", Post: func(fn func()) { fn() },
		Logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	n := New(cfg)

	answer := make(chan bool, 1)
	n.Authorize(func(granted bool) { answer <- granted })
	granted := <-answer

	k, err := registry.OpenKey(registry.CURRENT_USER, aumidKey+cfg.AppID, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("the registry entry is missing: %v", err)
	}
	for _, v := range []string{"DisplayName", "CustomActivator"} {
		s, _, err := k.GetStringValue(v)
		t.Logf("HKCU\\%s%s %s = %q %v", aumidKey, cfg.AppID, v, s, err)
	}
	k.Close()
	if !granted {
		t.Error("Authorize: toasts are off for dev.gift.notify-test or for all apps (Settings > System > Notifications); showing anyway")
	}

	if err := n.Show(Notification{ID: "test", Title: "gift/notify", Body: "first"}); err != nil {
		t.Fatal(err)
	}
	clicks := make(chan struct{}, 1)
	err = n.Show(Notification{ID: "test", Title: "gift/notify", Body: "second, replacing the first <&>",
		OnClick: func() { clicks <- struct{}{} }})
	if err != nil {
		t.Fatal(err)
	}
	if d, err := time.ParseDuration(os.Getenv("GIFT_NOTIFY_CLICK")); err == nil {
		t.Logf("waiting %v for a click on the toast", d)
		select {
		case <-clicks:
			t.Log("clicked")
		case <-time.After(d):
			t.Fatal("no click arrived")
		}
	} else {
		time.Sleep(5 * time.Second)
	}
	if err := n.Withdraw("test"); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GIFT_NOTIFY_UNREGISTER") == "1" {
		if err := Unregister(cfg); err != nil {
			t.Fatal(err)
		}
	}
}
