//go:build darwin && !ios

package filedialog

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// init keeps the main goroutine on the main thread, for TestMain.
func init() { runtime.LockOSThread() }

// TestMain runs an NSApplication on the main thread while real panels are
// under test, which is what an Ebitengine application has and a test binary
// has not: without NSApp this package refuses to show a panel, and without
// the run loop of the main thread nothing queued for it would ever run.
//
// The application is an accessory, as in tray's tests: no Dock icon and no
// menu bar of its own, only the panels.
func TestMain(m *testing.M) {
	if !macIntegration() {
		os.Exit(m.Run())
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
		panic(err)
	}
	app := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	const nsApplicationActivationPolicyAccessory = 1
	app.Send(objc.RegisterName("setActivationPolicy:"), nsApplicationActivationPolicyAccessory)
	go func() { os.Exit(m.Run()) }()
	// run never returns; the tests end the process with os.Exit.
	app.Send(objc.RegisterName("run"))
}

func macIntegration() bool { return os.Getenv("GIFT_FILEDIALOG_MACOS") == "1" }

// TestMacOSLoads opens the frameworks and finds the classes, on every Mac,
// with or without an NSApplication.
func TestMacOSLoads(t *testing.T) {
	p, err := openPlatform(openConfig{getenv: os.Getenv})
	if err != nil {
		t.Fatalf("the frameworks did not open: %v", err)
	}
	if _, ok := p.(*macPlatform); !ok {
		t.Fatalf("platform %T", p)
	}
	if !platformAvailable() {
		t.Fatal("platformAvailable is false on a Mac")
	}
	if mac.hasApp() != macIntegration() {
		t.Fatalf("hasApp %v with GIFT_FILEDIALOG_MACOS=%q", mac.hasApp(), os.Getenv("GIFT_FILEDIALOG_MACOS"))
	}
}

// TestMacOSWithoutApplication is the plain test binary: no NSApplication, so
// open answers this request with an ErrUnsupported that is not permanent,
// and asking did not create the application.
func TestMacOSWithoutApplication(t *testing.T) {
	if macIntegration() {
		t.Skip("TestMain runs an NSApplication")
	}
	p, err := openPlatform(openConfig{getenv: os.Getenv})
	if err != nil {
		t.Fatal(err)
	}
	c, err := p.open(request{mode: Open, title: "x", accept: "Open"}, func(result) {
		t.Error("done without a panel")
	})
	var pe permanentError
	if c != nil || !errors.Is(err, ErrUnsupported) || errors.As(err, &pe) {
		t.Fatalf("close %v, error %v; want a non-permanent ErrUnsupported", c != nil, err)
	}
	if mac.hasApp() {
		t.Fatal("asking for NSApp created it")
	}
}

// TestMacOSPanel shows real panels and is off unless GIFT_FILEDIALOG_MACOS=1:
//
//	GIFT_FILEDIALOG_MACOS=1 go test ./filedialog/ -run MacOS -v
//
// It needs a logged in GUI session but no app bundle. Each mode shows a free
// panel (the test binary has no window) for 500 ms, which close cancels.
// Then a save panel with a name in a temporary directory is confirmed with
// ok:, as the Save button would, and answers with that path.
func TestMacOSPanel(t *testing.T) {
	if !macIntegration() {
		t.Skip("set GIFT_FILEDIALOG_MACOS=1 in a GUI session")
	}
	pl, err := openPlatform(openConfig{getenv: os.Getenv})
	if err != nil {
		t.Fatal(err)
	}
	p := pl.(*macPlatform)
	shown := make(chan objc.ID, 1)
	p.shown = func(panel objc.ID) { shown <- panel }

	wait := func(what string, ch <-chan result) result {
		t.Helper()
		select {
		case res := <-ch:
			return res
		case <-time.After(10 * time.Second):
			t.Fatalf("%s: no answer within 10 s", what)
		}
		return result{}
	}
	waitShown := func(what string) objc.ID {
		t.Helper()
		select {
		case panel := <-shown:
			return panel
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: the panel did not begin within 5 s", what)
		}
		return 0
	}

	tmp := t.TempDir()
	for _, m := range []Mode{Open, OpenMulti, Directory, Save} {
		r := request{mode: m, title: "gift filedialog test " + defaultTitle(m), accept: acceptLabel(m), start: tmp}
		if m != Directory {
			r.exts = []string{".jpg", ".png", ".nosuchextensionatall"}
		}
		if m == Save {
			r.suggested = "name.jpg"
		}
		ch := make(chan result, 2)
		closePanel, err := p.open(r, func(res result) { ch <- res })
		if err != nil {
			t.Fatalf("mode %d: %v", m, err)
		}
		waitShown("mode " + defaultTitle(m))
		time.Sleep(500 * time.Millisecond)
		if final := closePanel(); final {
			t.Fatal("close reported final, macOS still answers")
		}
		closePanel() // idempotent: the second finds the panel finished or cancels nothing twice
		res := wait("close", ch)
		if !res.cancelled || res.err != nil || len(res.paths) != 0 {
			t.Fatalf("mode %d: closed with %+v, want cancelled", m, res)
		}
		select {
		case res := <-ch:
			t.Fatalf("mode %d: a second answer %+v", m, res)
		case <-time.After(200 * time.Millisecond):
		}
		// close after the answer: the panel is finished, and nothing is
		// sent to it.
		closePanel()
	}

	// close straight after open, before the main thread has begun the
	// panel: the close queues behind the begin and cancels the panel as
	// soon as it is there.
	{
		ch := make(chan result, 2)
		closePanel, err := p.open(request{mode: Open, title: "Open", accept: "Open"}, func(res result) { ch <- res })
		if err != nil {
			t.Fatal(err)
		}
		closePanel()
		waitShown("close at once")
		if res := wait("close at once", ch); !res.cancelled {
			t.Fatalf("closed at once with %+v, want cancelled", res)
		}
	}

	const name = "gift-filedialog-test.txt"
	ch := make(chan result, 1)
	if _, err := p.open(request{mode: Save, title: "Save", accept: "Save", start: tmp, suggested: name}, func(res result) { ch <- res }); err != nil {
		t.Fatal(err)
	}
	panel := waitShown("save")
	time.Sleep(500 * time.Millisecond)
	// [panel ok:] is what the Save button sends, but since the panel became
	// a remote view of another process, AppKit refuses it from code with
	// "not implemented". completeWithReturnCode:url:urls: is the private
	// method the remote side ends a panel with; the test uses it in place
	// of the button, with the path the name field holds. It ends the panel
	// through the same completion handler, and URL then reads what it was
	// given. Production code never calls it.
	complete := objc.RegisterName("completeWithReturnCode:url:urls:")
	mac.async(func() {
		if objc.Send[bool](panel, objc.RegisterName("respondsToSelector:"), complete) {
			u := objc.ID(mac.sel.urlClass).Send(mac.sel.fileURLWithFileSystemRepresentation, filepath.Join(tmp, name), false, objc.ID(0))
			urls := objc.ID(mac.sel.arrayClass).Send(mac.sel.arrayWithCapacity, uint(1))
			urls.Send(mac.sel.addObject, u)
			panel.Send(complete, nsModalResponseOK, u, urls)
		} else {
			panel.Send(mac.sel.cancel, objc.ID(0))
		}
	})
	res := wait("ok:", ch)
	if res.err != nil || res.cancelled || len(res.paths) != 1 {
		t.Fatalf("ok: answered %+v, want one path", res)
	}
	want, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatal(err)
	}
	got := res.paths[0]
	if d, err := filepath.EvalSymlinks(filepath.Dir(got)); err == nil {
		got = filepath.Join(d, filepath.Base(got))
	}
	if got != filepath.Join(want, name) {
		t.Fatalf("ok: answered %q, want %q", res.paths[0], filepath.Join(tmp, name))
	}
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Fatalf("the save panel wrote something: %v", err)
	}
}
