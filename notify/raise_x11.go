//go:build linux && !android && !faketime

package notify

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// This file raises the window of this process when the user clicks one of its
// notifications; see "Raising the window" in freedesktop.go for why that is
// the application's job.
//
// # Which window
//
// Ebitengine keeps its X11 connection and its window inside GLFW, inside
// internal/, where nothing outside it can reach, the wall gift/clipboard
// describes. So the window is found the way a pager finds it: the window
// manager lists the top-level windows of the screen in _NET_CLIENT_LIST on
// the root window, and each window names the process it belongs to in
// _NET_WM_PID, which GLFW sets. The first window of this process is raised. A
// gift application has one.
//
// # The request
//
// EWMH: a ClientMessage _NET_ACTIVE_WINDOW to the root window, with source
// indication 1, an application, and the time of the user action that caused
// it. That time is what focus stealing prevention judges: mutter turns a
// request older than the last input into "demands attention" and nothing
// more. It is taken from the activation token, which on X11 and XWayland is
// a startup notification id ending in "_TIME<server time>". Without a token,
// or with one that carries no time, it is 0, which mutter and KWin read as
// "now", the moment just after the click.
//
// Source indication 2, a pager, would also pass, and would be a lie: the
// request comes from the application, because the user clicked its
// notification.
//
// # Connection and errors
//
// Each raise opens a connection of its own and closes it again. A click is
// rare, a connection costs a round trip, and keeping none open means no
// thread and no state between clicks.
//
// Reading _NET_WM_PID of another client's window that closes in between is a
// BadWindow error, and the error handler Xlib installs by default prints it
// and calls exit(1). So a handler is chained in front, in the way
// gift/clipboard does it and for the reasons it gives there: errors on our
// connection are ignored, everything else goes to the handler that was
// installed before.

const (
	xaCardinal       = 6  // predefined by the protocol
	xaWindow         = 33 // predefined by the protocol
	evClientMessage  = 33
	maxClientWindows = 1 << 12 // in 32 bit units, far more top-level windows than a desktop has

	sourceApplication = 1

	substructureNotifyMask   = 1 << 19
	substructureRedirectMask = 1 << 20
)

type x11lib struct {
	openDisplay       func(name string) uintptr
	closeDisplay      func(dpy uintptr) int32
	defaultRootWindow func(dpy uintptr) uint64
	internAtom        func(dpy uintptr, name string, onlyIfExists bool) uint64
	getWindowProperty func(dpy uintptr, w, property uint64, offset, length int64, del bool, reqType uint64, actualType, actualFormat, nitems, bytesAfter unsafe.Pointer, prop *unsafe.Pointer) int32
	sendEvent         func(dpy uintptr, w uint64, propagate bool, mask int64, ev unsafe.Pointer) int32
	free              func(p unsafe.Pointer) int32
	flush             func(dpy uintptr) int32
	setErrorHandler   func(h uintptr) uintptr
}

// x11Raiser raises the window of this process. It loads libX11 on the first
// click, not at start: an application whose notifications are never clicked
// never loads it.
type x11Raiser struct {
	log *slog.Logger

	once sync.Once
	lib  x11lib
	err  error

	mu sync.Mutex // one raise at a time; two clicks in a row raise the same window
}

// newRaiser returns the raise function of the Linux platform, or nil when
// there is no X server: a pure Wayland session without XWayland, where
// Ebitengine has no window either, or a machine without a display.
func newRaiser(log *slog.Logger) func(token string) {
	if os.Getenv("DISPLAY") == "" {
		return nil
	}
	r := &x11Raiser{log: log}
	return r.raise
}

func (r *x11Raiser) raise(token string) {
	r.once.Do(func() {
		r.lib, r.err = loadX11()
		if r.err == nil {
			installErrorHandler(r.lib, r.log)
		} else {
			r.logf(slog.LevelWarn, "a notification was clicked, but its window cannot be raised", "error", r.err)
		}
	})
	if r.err != nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.activate(tokenTime(token)); err != nil {
		r.logf(slog.LevelWarn, "a notification was clicked, but its window cannot be raised", "error", err)
	}
}

func (r *x11Raiser) activate(t uint32) error {
	lib := r.lib
	display := os.Getenv("DISPLAY")
	dpy := lib.openDisplay(display)
	if dpy == 0 {
		return fmt.Errorf("XOpenDisplay(%q) failed", display)
	}
	// Ours before the first request that can fail; see installErrorHandler.
	errorDisplays.Store(dpy, struct{}{})
	defer func() {
		errorDisplays.Delete(dpy)
		lib.closeDisplay(dpy)
	}()

	root := lib.defaultRootWindow(dpy)
	clientList := lib.internAtom(dpy, "_NET_CLIENT_LIST", false)
	wmPID := lib.internAtom(dpy, "_NET_WM_PID", false)
	activeWindow := lib.internAtom(dpy, "_NET_ACTIVE_WINDOW", false)

	win, ok := r.ownWindow(dpy, root, clientList, wmPID, uint64(os.Getpid()))
	if !ok {
		return fmt.Errorf("no window of process %d in _NET_CLIENT_LIST", os.Getpid())
	}

	// ClientMessage: window 4, message_type 5, format 6, data 7..11; see
	// xEvent in gift/clipboard for the layout.
	var ev [24]uint64
	ev[0] = evClientMessage
	ev[2] = 1 // send_event
	ev[3] = uint64(dpy)
	ev[4] = win
	ev[5] = activeWindow
	ev[6] = 32
	ev[7] = sourceApplication
	ev[8] = uint64(t)
	ev[9] = 0 // the window of ours that is active now: none, since the click was elsewhere
	lib.sendEvent(dpy, root, false, substructureNotifyMask|substructureRedirectMask, unsafe.Pointer(&ev[0]))
	lib.flush(dpy)
	r.logf(slog.LevelDebug, "raising the window of a clicked notification", "window", win, "time", t)
	return nil
}

// ownWindow finds the first window in _NET_CLIENT_LIST whose _NET_WM_PID is
// pid.
func (r *x11Raiser) ownWindow(dpy uintptr, root, clientList, wmPID, pid uint64) (uint64, bool) {
	for _, w := range r.cardinals(dpy, root, clientList, xaWindow, maxClientWindows) {
		if p := r.cardinals(dpy, w, wmPID, xaCardinal, 1); len(p) == 1 && p[0] == pid {
			return w, true
		}
	}
	return 0, false
}

// cardinals reads a property of format 32, whose elements Xlib hands out as C
// longs, eight bytes here. A missing property, a window that is gone and a
// property of another type all read as nothing.
func (r *x11Raiser) cardinals(dpy uintptr, w, property, typ uint64, max int64) []uint64 {
	var (
		actualType   uint64
		actualFormat int32
		nitems       uint64
		bytesAfter   uint64
		prop         unsafe.Pointer
	)
	status := r.lib.getWindowProperty(dpy, w, property, 0, max, false, typ,
		unsafe.Pointer(&actualType), unsafe.Pointer(&actualFormat),
		unsafe.Pointer(&nitems), unsafe.Pointer(&bytesAfter), &prop)
	if status != 0 || prop == nil {
		return nil
	}
	defer r.lib.free(prop)
	if actualType != typ || actualFormat != 32 {
		return nil
	}
	return append([]uint64(nil), unsafe.Slice((*uint64)(prop), int(nitems))...)
}

// tokenTime is the X server time in an activation token, which on X11 is a
// startup notification id and ends in "_TIME" and the time, by the convention
// of the startup notification specification. A token without one, a Wayland
// token for instance, gives 0.
func tokenTime(token string) uint32 {
	i := strings.LastIndex(token, "_TIME")
	if i < 0 {
		return 0
	}
	t, err := strconv.ParseUint(token[i+len("_TIME"):], 10, 32)
	if err != nil {
		return 0
	}
	return uint32(t)
}

func (r *x11Raiser) logf(level slog.Level, msg string, args ...any) {
	if r.log == nil {
		return
	}
	r.log.Log(context.Background(), level, msg, args...)
}

// loadX11 opens libX11 and binds the functions in [x11lib]. The sonames and
// the recover are gift/clipboard's; see loadX11 there.
func loadX11() (lib x11lib, err error) {
	var handle uintptr
	for _, name := range []string{"libX11.so.6", "libX11.so"} {
		handle, err = purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			break
		}
	}
	if handle == 0 {
		return lib, fmt.Errorf("libX11.so.6 could not be loaded: %w", err)
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("binding libX11 failed: %v", r)
		}
	}()
	purego.RegisterLibFunc(&lib.openDisplay, handle, "XOpenDisplay")
	purego.RegisterLibFunc(&lib.closeDisplay, handle, "XCloseDisplay")
	purego.RegisterLibFunc(&lib.defaultRootWindow, handle, "XDefaultRootWindow")
	purego.RegisterLibFunc(&lib.internAtom, handle, "XInternAtom")
	purego.RegisterLibFunc(&lib.getWindowProperty, handle, "XGetWindowProperty")
	purego.RegisterLibFunc(&lib.sendEvent, handle, "XSendEvent")
	purego.RegisterLibFunc(&lib.free, handle, "XFree")
	purego.RegisterLibFunc(&lib.flush, handle, "XFlush")
	purego.RegisterLibFunc(&lib.setErrorHandler, handle, "XSetErrorHandler")
	return lib, err
}

// The error handler, chained as in gift/clipboard. Xlib's handler is one
// function pointer for the process; whichever of the two packages installs
// its handler second passes the errors of the other's connections on to it.
var (
	errorHandlerOnce sync.Once
	previousHandler  uintptr
	errorLog         *slog.Logger
	errorDisplays    sync.Map // uintptr -> struct{}, the connections that are ours
)

func installErrorHandler(lib x11lib, log *slog.Logger) {
	errorHandlerOnce.Do(func() {
		errorLog = log
		cb := purego.NewCallback(func(dpy uintptr, ev unsafe.Pointer) uintptr {
			_, ours := errorDisplays.Load(dpy)
			if !ours && previousHandler != 0 {
				r, _, _ := purego.SyscallN(previousHandler, dpy, uintptr(ev))
				return r
			}
			if errorLog != nil && !ours {
				b := unsafe.Slice((*byte)(ev), 40)
				errorLog.Error("an X11 error arrived on a connection that is not gift/notify's, and there is no handler to pass it to, ignoring it",
					"error_code", b[32], "request_code", b[33], "minor_code", b[34])
			}
			// An error on our connection is a window that closed while its
			// properties were read, which is routine and reads as nothing.
			return 0
		})
		previousHandler = lib.setErrorHandler(cb)
	})
}
