//go:build linux

package droptest

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/worldiety/gift"
)

// xdndSource is an XDND drag source of its own, bound with purego for this
// test only: what Nautilus or Dolphin send under X11, reduced to one drag of
// a text/uri-list. The X server is Xvfb, which has no window manager, so the
// target is found as the child of the root that declares XdndAware.
type xdndSource struct {
	openDisplay          func(name string) uintptr
	defaultRootWindow    func(dpy uintptr) uint64
	internAtom           func(dpy uintptr, name string, onlyIfExists bool) uint64
	createSimpleWindow   func(dpy uintptr, parent uint64, x, y int32, w, h, border uint32, borderPixel, background uint64) uint64
	setSelectionOwner    func(dpy uintptr, selection, owner, t uint64) int32
	sendEvent            func(dpy uintptr, w uint64, propagate bool, mask int64, ev unsafe.Pointer) int32
	nextEvent            func(dpy uintptr, ev unsafe.Pointer) int32
	pending              func(dpy uintptr) int32
	changeProperty       func(dpy uintptr, w, property, typ uint64, format, mode int32, data unsafe.Pointer, n int32) int32
	queryTree            func(dpy uintptr, w uint64, root, parent *uint64, children **uint64, n *uint32) int32
	getWindowProperty    func(dpy uintptr, w, property uint64, offset, length int64, del bool, reqType uint64, actualType *uint64, actualFormat *int32, nitems, bytesAfter *uint64, prop *unsafe.Pointer) int32
	translateCoordinates func(dpy uintptr, src, dst uint64, x, y int32, dx, dy *int32, child *uint64) int32
	warpPointer          func(dpy uintptr, src, dst uint64, sx, sy int32, sw, sh uint32, dx, dy int32) int32
	free                 func(p unsafe.Pointer) int32
	flush                func(dpy uintptr) int32

	dpy       uintptr
	root, src uint64
	a         map[string]uint64
}

// xEvent is an XEvent, 192 bytes on a 64 bit system, as in
// clipboard/clipboard_x11.go.
type xEvent [24]uint64

const (
	evSelectionRequest = 30
	evSelectionNotify  = 31
	evClientMessage    = 33
)

func openXdndSource(t *testing.T) *xdndSource {
	t.Helper()
	h, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatalf("libX11: %v", err)
	}
	s := &xdndSource{a: map[string]uint64{}}
	for name, fn := range map[string]any{
		"XOpenDisplay": &s.openDisplay, "XDefaultRootWindow": &s.defaultRootWindow,
		"XInternAtom": &s.internAtom, "XCreateSimpleWindow": &s.createSimpleWindow,
		"XSetSelectionOwner": &s.setSelectionOwner, "XSendEvent": &s.sendEvent,
		"XNextEvent": &s.nextEvent, "XPending": &s.pending, "XChangeProperty": &s.changeProperty,
		"XQueryTree": &s.queryTree, "XGetWindowProperty": &s.getWindowProperty,
		"XTranslateCoordinates": &s.translateCoordinates, "XWarpPointer": &s.warpPointer,
		"XFree": &s.free, "XFlush": &s.flush,
	} {
		purego.RegisterLibFunc(fn, h, name)
	}
	s.dpy = s.openDisplay(os.Getenv("DISPLAY"))
	if s.dpy == 0 {
		t.Fatalf("XOpenDisplay(%q) failed", os.Getenv("DISPLAY"))
	}
	s.root = s.defaultRootWindow(s.dpy)
	for _, n := range []string{"XdndAware", "XdndEnter", "XdndPosition", "XdndStatus", "XdndDrop",
		"XdndFinished", "XdndSelection", "XdndActionCopy", "text/uri-list"} {
		s.a[n] = s.internAtom(s.dpy, n, false)
	}
	s.src = s.createSimpleWindow(s.dpy, s.root, 0, 0, 1, 1, 0, 0, 0)
	s.flush(s.dpy)
	return s
}

// target waits for gift's window: a child of the root with XdndAware.
func (s *xdndSource) target(t *testing.T) uint64 {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var r, p uint64
		var kids *uint64
		var n uint32
		if s.queryTree(s.dpy, s.root, &r, &p, &kids, &n) != 0 && n > 0 {
			list := unsafe.Slice(kids, n)
			for _, w := range list {
				var typ uint64
				var format int32
				var items, after uint64
				var prop unsafe.Pointer
				s.getWindowProperty(s.dpy, w, s.a["XdndAware"], 0, 1, false, 0, &typ, &format, &items, &after, &prop)
				if prop != nil {
					s.free(prop)
				}
				if typ != 0 && w != s.src {
					s.free(unsafe.Pointer(kids))
					return w
				}
			}
			s.free(unsafe.Pointer(kids))
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("no window with XdndAware appeared within 10 s")
	return 0
}

// origin is where the window's top left corner is on the root.
func (s *xdndSource) origin(w uint64) (int32, int32) {
	var x, y int32
	var child uint64
	s.translateCoordinates(s.dpy, w, s.root, 0, 0, &x, &y, &child)
	return x, y
}

func (s *xdndSource) client(to, typ uint64, l ...uint64) {
	var ev xEvent
	ev[0] = evClientMessage
	ev[4] = to
	ev[5] = typ
	ev[6] = 32
	copy(ev[7:12], l)
	s.sendEvent(s.dpy, to, false, 0, unsafe.Pointer(&ev[0]))
	s.flush(s.dpy)
}

// await reads events until one satisfies ok, answering the selection request
// of the drop with uris on the way.
func (s *xdndSource) await(t *testing.T, what string, uris string, ok func(*xEvent) bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if s.pending(s.dpy) == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		var ev xEvent
		s.nextEvent(s.dpy, unsafe.Pointer(&ev[0]))
		if uint32(ev[0]) == evSelectionRequest {
			requestor, selection, target, property, tm := ev[5], ev[6], ev[7], ev[8], ev[9]
			data := []byte(uris)
			s.changeProperty(s.dpy, requestor, property, target, 8, 0, unsafe.Pointer(&data[0]), int32(len(data)))
			var n xEvent
			n[0] = evSelectionNotify
			n[4], n[5], n[6], n[7], n[8] = requestor, selection, target, property, tm
			s.sendEvent(s.dpy, requestor, false, 0, unsafe.Pointer(&n[0]))
			s.flush(s.dpy)
		}
		if ok(&ev) {
			return
		}
	}
	t.Fatalf("no %s from gift's window within 10 s", what)
}

// drag performs one drag of uris to the root position (x, y): the pointer is
// put there first, because Ebitengine reads the drop position from the
// pointer, not from XdndPosition.
func (s *xdndSource) drag(t *testing.T, w uint64, x, y int32, uris string) {
	t.Helper()
	s.setSelectionOwner(s.dpy, s.a["XdndSelection"], s.src, 0)
	s.warpPointer(s.dpy, 0, s.root, 0, 0, 0, 0, x, y)
	s.flush(s.dpy)
	time.Sleep(300 * time.Millisecond) // a few polls of the cursor
	s.client(w, s.a["XdndEnter"], s.src, 5<<24, s.a["text/uri-list"], 0, 0)
	s.client(w, s.a["XdndPosition"], s.src, 0, uint64(x)<<16|uint64(y), 0, s.a["XdndActionCopy"])
	s.await(t, "XdndStatus", uris, func(ev *xEvent) bool {
		return uint32(ev[0]) == evClientMessage && ev[5] == s.a["XdndStatus"]
	})
	s.client(w, s.a["XdndDrop"], s.src, 0, 0)
	s.await(t, "XdndFinished", uris, func(ev *xEvent) bool {
		return uint32(ev[0]) == evClientMessage && ev[5] == s.a["XdndFinished"]
	})
}

func fileURI(p string) string { return (&url.URL{Scheme: "file", Path: p}).String() }

func TestX11DropOfFilesReachesTheFileDestination(t *testing.T) {
	need(t, "GIFT_DROP_X11")
	if c := waitCaps(t, gift.DropFile); c.Live || c.Kinds != gift.DropFile {
		t.Errorf("capabilities %+v, want files and no live session on X11", c)
	}

	dirA, dirB := t.TempDir(), t.TempDir()
	space := filepath.Join(dirA, "a b.txt")
	umlaut := filepath.Join(dirA, "Grüße.txt")
	folder := filepath.Join(dirA, "Ordner")
	other := filepath.Join(dirB, "c.txt")
	for _, f := range []string{space, umlaut, other} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	uris := strings.Join([]string{fileURI(space), fileURI(umlaut), fileURI(folder), fileURI(other),
		"smb://host/share/x"}, "\r\n") + "\r\n"
	if !strings.Contains(uris, "Gr%C3%BC%C3%9Fe") {
		t.Fatalf("the umlaut is not percent-encoded: %s", uris)
	}

	s := openXdndSource(t)
	w := s.target(t)
	ox, oy := s.origin(w)
	drain()

	// Over the inner text destination: files go to the outer one.
	s.drag(t, w, ox+50, oy+50, uris)
	e := next(t, false)
	if e.area != "outer" {
		t.Fatalf("the drop went to %q, want the outer file destination", e.area)
	}
	got := e.info.Files()
	slices.Sort(got)
	want := []string{space, umlaut, folder, other}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("files %q, want %q (the smb: address must not arrive)", got, want)
	}
	for _, it := range e.info.Items {
		if it.Path == folder && !it.Dir {
			t.Error("the folder arrived as a file")
		}
	}
	if e.info.Location.X < 40 || e.info.Location.X > 60 || e.info.Location.Y < 40 || e.info.Location.Y > 60 {
		t.Errorf("Location %v, want near (50, 50)", e.info.Location)
	}

	// The pointer outside the window, as a stale cursor under XWayland
	// would be: the window-wide destination still gets the drop.
	s.drag(t, w, ox+winW+100, oy+winH+100, fileURI(other)+"\r\n")
	e = next(t, false)
	if e.area != "outer" || len(e.info.Files()) != 1 || e.info.Files()[0] != other {
		t.Errorf("a drop with the pointer outside the window delivered %+v", e)
	}
	fmt.Fprintln(os.Stderr, "droptest: X11 drops verified")
}
