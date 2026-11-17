// Package droptest runs gift's drop support against a real window: an XDND
// source in an X server, a simulated drag on the real AppKit view, and calls
// into the OLE drop target on Windows. It consists of tests only.
//
// Each platform's test runs when its variable is set and is skipped
// otherwise, so that go test ./... passes everywhere:
//
//	GIFT_DROP_X11=1     go test ./backend/ebiten/droptest -run X11 -v -count=1
//	GIFT_DROP_MACOS=1   go test ./backend/ebiten/droptest -v -count=1
//	GIFT_DROP_WINDOWS=1 go test ./backend/ebiten/droptest -run Windows -v
//
// The X11 one is meant for a container with Xvfb; the project plan, section
// 48, has the command.
package droptest

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/worldiety/gift"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/example"
	"github.com/worldiety/gift/ui"
)

// The window is 400 by 300 logical pixels: a file destination over all of it
// and, in its top left corner, a text and address destination of 200 by 100.
const (
	winW, winH     = 400, 300
	innerW, innerH = 200, 100
)

func init() { runtime.LockOSThread() }

// event is one thing a destination reported.
type event struct {
	area     string // "outer" or "inner"
	targeted *bool  // set for OnTargeted
	info     ui.DropInfo
}

var (
	app    *gift.App
	events = make(chan event, 256)
	caps   atomic.Value // gift.DropCapabilities of the last build
)

// integration names the platform whose window test runs, or "".
func integration() string {
	for _, v := range []string{"GIFT_DROP_X11", "GIFT_DROP_MACOS", "GIFT_DROP_WINDOWS"} {
		if os.Getenv(v) != "" {
			return v
		}
	}
	return ""
}

func root(ctx *gift.Context) gift.View {
	caps.Store(ctx.DropCapabilities())
	on := func(area string) func(bool) {
		return func(b bool) { events <- event{area: area, targeted: &b} }
	}
	inner := ui.DropDestination(ui.Box().Frame(innerW, innerH)).Key("inner").
		Accepts(gift.DropText | gift.DropURL).
		OnTargeted(on("inner")).
		OnDrop(func(d ui.DropInfo) bool { events <- event{area: "inner", info: d}; return true })
	return ui.DropDestination(
		ui.Window(ui.VStack(inner).Align(geom.Alignment{}).Frame(winW, winH)),
	).Key("outer").Accepts(gift.DropFile).
		OnTargeted(on("outer")).
		OnDrop(func(d ui.DropInfo) bool { events <- event{area: "outer", info: d}; return true })
}

func TestMain(m *testing.M) {
	if integration() == "" {
		os.Exit(m.Run())
	}
	if err := example.LoadFont(); err != nil {
		panic(err)
	}
	app = gift.New(gift.Options{Root: root})
	go func() { os.Exit(m.Run()) }()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	err := backend.Run(app, backend.Config{Title: "gift droptest", Width: winW, Height: winH, Logger: log})
	fmt.Fprintln(os.Stderr, "droptest: the window closed before the tests ended:", err)
	os.Exit(1)
}

// need skips t unless the named platform test runs.
func need(t *testing.T, v string) {
	t.Helper()
	if integration() != v {
		t.Skipf("set %s=1 to run this against a real window", v)
	}
}

// next waits up to 10 s for the next event, skipping OnTargeted reports
// unless targeted is set.
func next(t *testing.T, targeted bool) event {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case e := <-events:
			if e.targeted != nil && !targeted {
				continue
			}
			return e
		case <-timeout:
			t.Fatal("no event from the window within 10 s")
			return event{}
		}
	}
}

// drain forgets every event so far.
func drain() {
	for {
		select {
		case <-events:
		default:
			return
		}
	}
}

// waitCaps waits until the builds saw capabilities with at least kinds.
func waitCaps(t *testing.T, kinds gift.DropKind) gift.DropCapabilities {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if c, ok := caps.Load().(gift.DropCapabilities); ok && c.Kinds&kinds == kinds {
			return c
		}
		time.Sleep(20 * time.Millisecond)
	}
	c, _ := caps.Load().(gift.DropCapabilities)
	t.Fatalf("capabilities %+v, want at least %v", c, kinds)
	return c
}

func TestNothingRunsWithoutAWindow(t *testing.T) {
	if integration() != "" {
		t.Skip("a window test runs")
	}
}
