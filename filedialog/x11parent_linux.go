//go:build linux && !android

package filedialog

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// This file finds the X11 window of this process, so that the portal's
// dialog can be given it as parent_window ("x11:<XID>"): GNOME and KDE then
// show the dialog modal to the gift window, centred on it and above it,
// instead of as a free window somewhere on the screen. Ebitengine 2.10 runs
// on X11 or, under a Wayland session, on XWayland, so an X11 handle is the
// only one there is; a Wayland handle would take xdg-foreign, which GLFW
// does not offer.
//
// # Finding the window
//
// Ebitengine does not hand out its window, but GLFW declares the process on
// it: _NET_WM_PID. The window manager lists the windows it manages in
// _NET_CLIENT_LIST on the root window. The window wanted is the first one in
// that list whose _NET_WM_PID is os.Getpid(). There is no WM_CLASS fallback:
// a process whose PID does not match its window is one in another PID
// namespace, and a name match there could as well be a second instance of
// the same application.
//
// # Bounds
//
// The parent is a nicety, and a dialog without one is what this package
// showed before. So every failure — no DISPLAY, no libX11, a server that
// refuses the connection, a window manager without EWMH, no window yet — is
// a parent of "", logged at Debug. A lookup gets [x11ParentTimeout]; one that
// takes longer is abandoned, and the dialog opens without a parent. An Xlib
// call cannot be interrupted, so the abandoned lookup's goroutine runs on,
// and no second lookup is started while it does.
//
// # X errors
//
// A window in the list may be destroyed between reading the list and
// reading its _NET_WM_PID, and the server answers that with BadWindow.
// Xlib passes errors to a single, process-wide handler, whose default
// prints and calls exit(1), and which GLFW and gift/clipboard also install
// and chain. This file does not touch it. It registers an error hook on its
// own connection instead (XAddExtension, XESetError), which Xlib consults
// before the process-wide handler for every error that answers a request
// with a reply, and which swallows the error for that connection only. All
// requests here have replies.

// x11ParentTimeout bounds one lookup of the parent window.
const x11ParentTimeout = 200 * time.Millisecond

// The predefined atoms and constants of Xlib used here.
const (
	xaCardinal     = 6
	xaWindow       = 33
	xAnyProperty   = 0
	xSuccess       = 0
	xPropertyLongs = 1 << 16 // how much of a property is read, in 32-bit units
)

// parentX11 is the part of libX11 this file binds. Window, Atom and the
// items of a format 32 property are C longs, which on Linux are as wide as a
// pointer.
type parentX11 struct {
	openDisplay       func(name string) uintptr
	closeDisplay      func(dpy uintptr) int32
	defaultRootWindow func(dpy uintptr) uintptr
	internAtom        func(dpy uintptr, name string, onlyIfExists bool) uintptr
	getWindowProperty func(dpy uintptr, w, property uintptr, offset, length int, del bool, reqType uintptr,
		actualType *uintptr, actualFormat *int32, nitems, bytesAfter *uintptr, prop *unsafe.Pointer) int32
	free         func(p unsafe.Pointer) int32
	addExtension func(dpy uintptr) unsafe.Pointer
	setExtError  func(dpy uintptr, extension int32, proc uintptr) uintptr

	// swallow is the error hook, a C function pointer.
	swallow uintptr
}

// x11Swallowed counts the X errors the hook swallowed. One lookup runs at a
// time, so the count of a lookup is the difference across it.
var x11Swallowed atomic.Uint64

// x11ParentLib is libX11, loaded on the first lookup and kept.
var x11ParentLib = sync.OnceValues(loadParentX11)

// loadParentX11 opens libX11 and binds the functions of [parentX11], the
// sonames in the order and for the reasons of clipboard/clipboard_x11.go.
func loadParentX11() (lib *parentX11, err error) {
	var handle uintptr
	for _, name := range []string{"libX11.so.6", "libX11.so"} {
		handle, err = purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			break
		}
	}
	if handle == 0 {
		return nil, fmt.Errorf("libX11.so.6 could not be loaded: %w", err)
	}
	// RegisterLibFunc panics on a missing symbol; a libX11 without one is
	// a parent of "" and no reason to end the application.
	defer func() {
		if r := recover(); r != nil {
			lib, err = nil, fmt.Errorf("binding libX11 failed: %v", r)
		}
	}()
	l := &parentX11{}
	purego.RegisterLibFunc(&l.openDisplay, handle, "XOpenDisplay")
	purego.RegisterLibFunc(&l.closeDisplay, handle, "XCloseDisplay")
	purego.RegisterLibFunc(&l.defaultRootWindow, handle, "XDefaultRootWindow")
	purego.RegisterLibFunc(&l.internAtom, handle, "XInternAtom")
	purego.RegisterLibFunc(&l.getWindowProperty, handle, "XGetWindowProperty")
	purego.RegisterLibFunc(&l.free, handle, "XFree")
	purego.RegisterLibFunc(&l.addExtension, handle, "XAddExtension")
	purego.RegisterLibFunc(&l.setExtError, handle, "XESetError")
	// Bool (*)(Display *, xError *, XExtCodes *, int *ret_code): the error
	// is handled, and the request that caused it returns no reply.
	l.swallow = purego.NewCallback(func(dpy uintptr, xerr, codes, ret unsafe.Pointer) uintptr {
		if ret != nil {
			*(*int32)(ret) = 0
		}
		x11Swallowed.Add(1)
		return 1
	})
	return l, nil
}

// x11ParentFinder looks up the parent window, one lookup at a time.
type x11ParentFinder struct {
	getenv func(string) string
	log    *slog.Logger
	busy   atomic.Bool
}

func newX11ParentFinder(getenv func(string) string, log *slog.Logger) *x11ParentFinder {
	return &x11ParentFinder{getenv: getenv, log: log}
}

// parent is the parent_window of the next dialog: "x11:<XID>", or "". It
// blocks for at most [x11ParentTimeout] and is called on the goroutine that
// opens the dialog.
func (f *x11ParentFinder) parent() string {
	display := f.getenv("DISPLAY")
	if display == "" {
		logf(f.log, slog.LevelDebug, "no DISPLAY, the dialog has no parent window")
		return ""
	}
	if !f.busy.CompareAndSwap(false, true) {
		logf(f.log, slog.LevelDebug, "a previous lookup of the X11 window still hangs, the dialog has no parent window")
		return ""
	}
	type found struct {
		xid uint64
		err error
	}
	ch := make(chan found, 1)
	go func() {
		defer f.busy.Store(false)
		xid, err := lookupX11Window(display, os.Getpid())
		ch <- found{xid, err}
	}()
	timer := time.NewTimer(x11ParentTimeout)
	defer timer.Stop()
	select {
	case r := <-ch:
		if r.err != nil {
			logf(f.log, slog.LevelDebug, "the dialog has no parent window", "display", display, "error", r.err)
			return ""
		}
		logf(f.log, slog.LevelDebug, "the dialog's parent window", "display", display, "xid", formatParent(r.xid))
		return formatParent(r.xid)
	case <-timer.C:
		logf(f.log, slog.LevelDebug, "the X11 window was not found in time, the dialog has no parent window",
			"display", display, "timeout", x11ParentTimeout)
		return ""
	}
}

// lookupX11Window connects to display and returns the first window of
// _NET_CLIENT_LIST whose _NET_WM_PID is pid.
func lookupX11Window(display string, pid int) (xid uint64, err error) {
	defer func() {
		if r := recover(); r != nil {
			xid, err = 0, fmt.Errorf("looking up the X11 window: %v", r)
		}
	}()
	lib, err := x11ParentLib()
	if err != nil {
		return 0, err
	}
	dpy := lib.openDisplay(display)
	if dpy == 0 {
		return 0, fmt.Errorf("cannot connect to the X server %q", display)
	}
	defer lib.closeDisplay(dpy)
	codes := lib.addExtension(dpy)
	if codes == nil {
		return 0, errors.New("XAddExtension failed")
	}
	// XExtCodes begins with the int that names the extension.
	lib.setExtError(dpy, *(*int32)(codes), lib.swallow)
	before := x11Swallowed.Load()

	clientList := lib.internAtom(dpy, "_NET_CLIENT_LIST", true)
	wmPID := lib.internAtom(dpy, "_NET_WM_PID", true)
	if clientList == 0 || wmPID == 0 {
		return 0, errors.New("the window manager does not declare _NET_CLIENT_LIST and _NET_WM_PID")
	}
	windows, ok := lib.longs(dpy, lib.defaultRootWindow(dpy), clientList, xaWindow)
	if !ok {
		return 0, errors.New("the root window has no _NET_CLIENT_LIST")
	}
	for _, w := range windows {
		if p, ok := lib.longs(dpy, w, wmPID, xaCardinal); ok && len(p) == 1 && int(p[0]) == pid && w != 0 {
			return uint64(w), nil
		}
	}
	return 0, fmt.Errorf("none of the %d windows of _NET_CLIENT_LIST has _NET_WM_PID %d (%d vanished meanwhile)",
		len(windows), pid, x11Swallowed.Load()-before)
}

// longs reads the format 32 property of w, of type typ, as C longs. ok is
// false for a property that is absent, of another type or format, or of a
// window that does not exist.
func (lib *parentX11) longs(dpy, w, property, typ uintptr) (items []uintptr, ok bool) {
	var actualType, nitems, bytesAfter uintptr
	var format int32
	var prop unsafe.Pointer
	status := lib.getWindowProperty(dpy, w, property, 0, xPropertyLongs, false, xAnyProperty,
		&actualType, &format, &nitems, &bytesAfter, &prop)
	if status != xSuccess {
		return nil, false
	}
	if prop != nil {
		defer lib.free(prop)
	}
	if actualType != typ || format != 32 || prop == nil || nitems == 0 {
		return nil, false
	}
	return append([]uintptr(nil), unsafe.Slice((*uintptr)(prop), nitems)...), true
}
