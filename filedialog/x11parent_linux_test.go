//go:build linux && !android

package filedialog

import (
	"os"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// TestX11ParentWithoutDisplay needs nothing: without DISPLAY, or with one
// that nothing listens on, the parent is "" and comes at once or within the
// bound.
func TestX11ParentWithoutDisplay(t *testing.T) {
	for name, display := range map[string]string{"no DISPLAY": "", "no server": ":4711"} {
		f := newX11ParentFinder(func(k string) string {
			if k == "DISPLAY" {
				return display
			}
			return ""
		}, nil)
		start := time.Now()
		if got := f.parent(); got != "" {
			t.Errorf("%s: parent %q, want \"\"", name, got)
		}
		if d := time.Since(start); d > x11ParentTimeout+100*time.Millisecond {
			t.Errorf("%s: the lookup took %v", name, d)
		}
	}
}

// TestX11ParentWindow needs an X server and libX11 and is off unless
// GIFT_FILEDIALOG_X11=1:
//
//	docker run --rm --platform linux/arm64 -v "$PWD":/src -w /src golang:1.27-trixie sh -c 'apt-get update -qq && apt-get install -y -qq xvfb && (Xvfb :99 & sleep 1; DISPLAY=:99 GIFT_FILEDIALOG_X11=1 go test ./filedialog/ -run X11Parent -v)'
//
// There is no window manager, so the test plays one: it creates a window
// with this process's _NET_WM_PID and lists it in the root window's
// _NET_CLIENT_LIST behind a window that no longer exists, whose BadWindow
// must be swallowed by the connection's error hook rather than reach Xlib's
// default handler, which would end the test binary.
func TestX11ParentWindow(t *testing.T) {
	if os.Getenv("GIFT_FILEDIALOG_X11") != "1" || os.Getenv("DISPLAY") == "" {
		t.Skip("set GIFT_FILEDIALOG_X11=1 and DISPLAY to run this against an X server")
	}
	handle, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	var (
		openDisplay   func(name string) uintptr
		closeDisplay  func(dpy uintptr) int32
		root          func(dpy uintptr) uintptr
		internAtom    func(dpy uintptr, name string, onlyIfExists bool) uintptr
		createWindow  func(dpy, parent uintptr, x, y int32, w, h, border uint32, borderPixel, background uintptr) uintptr
		destroyWindow func(dpy, w uintptr) int32
		change        func(dpy, w, property, typ uintptr, format, mode int32, data unsafe.Pointer, n int32) int32
		sync          func(dpy uintptr, discard bool) int32
	)
	purego.RegisterLibFunc(&openDisplay, handle, "XOpenDisplay")
	purego.RegisterLibFunc(&closeDisplay, handle, "XCloseDisplay")
	purego.RegisterLibFunc(&root, handle, "XDefaultRootWindow")
	purego.RegisterLibFunc(&internAtom, handle, "XInternAtom")
	purego.RegisterLibFunc(&createWindow, handle, "XCreateSimpleWindow")
	purego.RegisterLibFunc(&destroyWindow, handle, "XDestroyWindow")
	purego.RegisterLibFunc(&change, handle, "XChangeProperty")
	purego.RegisterLibFunc(&sync, handle, "XSync")

	dpy := openDisplay(os.Getenv("DISPLAY"))
	if dpy == 0 {
		t.Fatal("XOpenDisplay failed")
	}
	defer closeDisplay(dpy)
	r := root(dpy)
	gone := createWindow(dpy, r, 0, 0, 1, 1, 0, 0, 0)
	destroyWindow(dpy, gone)
	win := createWindow(dpy, r, 0, 0, 10, 10, 0, 0, 0)
	pid := []uintptr{uintptr(os.Getpid())}
	const propModeReplace = 0
	change(dpy, win, internAtom(dpy, "_NET_WM_PID", false), xaCardinal, 32, propModeReplace, unsafe.Pointer(&pid[0]), 1)
	list := []uintptr{gone, win}
	change(dpy, r, internAtom(dpy, "_NET_CLIENT_LIST", false), xaWindow, 32, propModeReplace, unsafe.Pointer(&list[0]), int32(len(list)))
	sync(dpy, false)

	before := x11Swallowed.Load()
	got := newX11ParentFinder(os.Getenv, nil).parent()
	if want := formatParent(uint64(win)); got != want {
		t.Fatalf("parent %q, want %q", got, want)
	}
	if n := x11Swallowed.Load() - before; n != 1 {
		t.Fatalf("%d X errors swallowed, want 1 for the destroyed window", n)
	}
	t.Logf("parent %s; the destroyed window %#x answered BadWindow and was skipped", got, gone)
}
