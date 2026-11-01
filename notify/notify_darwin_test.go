//go:build darwin

package notify

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego"
)

// init keeps the main goroutine on the main thread, for TestMain.
func init() { runtime.LockOSThread() }

// TestMain serves the main dispatch queue on the main thread while the real
// notification centre is under test. The centre calls its delegate — the
// click — on the main dispatch queue, and only the main thread serves that
// queue, through its run loop or dispatch_main. An Ebitengine application
// runs NSApplication's run loop there; a test binary runs nothing, and without
// this the click arrives at the process and never reaches the delegate.
func TestMain(m *testing.M) {
	if os.Getenv("GIFT_NOTIFY_MACOS") != "1" {
		os.Exit(m.Run())
	}
	lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	// dispatch_main parks the main thread and serves the main queue; it
	// never returns, the tests end the process with os.Exit.
	var dispatchMain func()
	purego.RegisterLibFunc(&dispatchMain, lib, "dispatch_main")
	go func() { os.Exit(m.Run()) }()
	dispatchMain()
}

// inBundle reports whether this test binary runs inside an app bundle, which
// it does under `go test -exec giftbundle`.
func inBundle() bool {
	exe, err := os.Executable()
	return err == nil && strings.Contains(exe, ".app/Contents/MacOS/")
}

// TestNotABundleIsUnsupported is the reason the bundle check exists: a plain
// `go test` binary is not a bundle, and without the check the first Show
// would end this process with SIGABRT inside currentNotificationCenter. That
// this test reports anything at all is the assertion.
func TestNotABundleIsUnsupported(t *testing.T) {
	if inBundle() {
		t.Skip("running inside a bundle")
	}
	var logs bytes.Buffer
	err := New(Config{Logger: slog.New(slog.NewTextHandler(&logs, nil))}).Show(Notification{Title: "t"})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error %v, want ErrUnsupported", err)
	}
	if !strings.Contains(logs.String(), "giftbundle") {
		t.Fatalf("the log does not point to giftbundle:\n%s", logs.String())
	}
}

// TestMacOSNotificationCentre talks to the real notification centre and is
// off unless GIFT_NOTIFY_MACOS=1. It has to run in a bundle:
//
//	go install ./cmd/giftbundle
//	GIFT_NOTIFY_MACOS=1 go test -exec "giftbundle -id dev.gift.notify-test" ./notify/ -run MacOS -v
//
// The first run shows the system's question; allow notifications for
// "notify.test" within a minute. With GIFT_NOTIFY_CLICK=<duration> it also
// waits that long for a click on the notification.
func TestMacOSNotificationCentre(t *testing.T) {
	if os.Getenv("GIFT_NOTIFY_MACOS") != "1" {
		t.Skip("set GIFT_NOTIFY_MACOS=1 and run through giftbundle")
	}
	if !inBundle() {
		t.Fatal("not in a bundle; run with -exec giftbundle")
	}
	n := New(Config{Post: func(fn func()) { fn() }})

	answer := make(chan bool, 1)
	n.Authorize(func(granted bool) { answer <- granted })
	select {
	case granted := <-answer:
		if !granted {
			t.Fatal("notifications are not permitted; allow them in System Settings > Notifications")
		}
	case <-time.After(time.Minute):
		t.Fatal("no answer to the permission question within a minute")
	}

	if err := n.Show(Notification{ID: "test", Title: "gift/notify", Body: "first"}); err != nil {
		t.Fatal(err)
	}
	clicks := make(chan struct{}, 1)
	err := n.Show(Notification{ID: "test", Title: "gift/notify", Body: "second, replacing the first",
		OnClick: func() { clicks <- struct{}{} }})
	if err != nil {
		t.Fatal(err)
	}
	if d, err := time.ParseDuration(os.Getenv("GIFT_NOTIFY_CLICK")); err == nil {
		t.Logf("waiting %v for a click on the notification", d)
		select {
		case <-clicks:
			t.Log("clicked")
		case <-time.After(d):
			t.Fatal("no click arrived")
		}
	} else {
		time.Sleep(3 * time.Second) // long enough to see it
	}
	if err := n.Withdraw("test"); err != nil {
		t.Fatal(err)
	}
}
