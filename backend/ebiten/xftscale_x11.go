//go:build linux && !android

package ebiten

import (
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// newSystemScale returns the reading [game.systemScale] uses on X11: the
// Xft.dpi resource as it is now, divided by 96.
//
// # Why Ebitengine's own reading is not enough
//
// GLFW on X11 reads Xft.dpi once, when it initialises, and every monitor's
// content scale is that one number until the process exits (ebiten/v2@v2.10.1,
// internal/glfw/x11_init_linbsd.go, getSystemContentScale). A desktop that
// changes the scale while the application runs is therefore not seen. GNOME
// on Wayland does exactly that for XWayland clients: unplugging an external
// monitor from a 2x laptop panel moves Xft.dpi from 96 to 192, plugging it in
// again moves it back. An application started on the panel then stays at 2x
// on the external monitor, twice as large as everything around it, and
// resizing the window does not help.
//
// # What this reads
//
// The RESOURCE_MANAGER property of the root window, which is where xrdb and
// the desktop keep the resources; XResourceManagerString would not do, as it
// is the copy Xlib took when the display was opened. It goes to the server at
// most once a second and answers the last reading in between, because
// [game.LayoutF] calls it every frame.
//
// The returned function reports 0 when there is nothing to read: no libX11,
// no display, or no Xft.dpi. LayoutF then keeps Ebitengine's factor.
func newSystemScale() func() float64 {
	x, ok := openXftReader()
	if !ok {
		return nil
	}
	return x.scale
}

type xftReader struct {
	*x11Conn

	next time.Time
	last float64
}

func openXftReader() (*xftReader, bool) {
	c, ok := openX11()
	if !ok {
		return nil, false
	}
	return &xftReader{x11Conn: c}, true
}

// x11Conn is a connection of its own to the X server, through libX11 and
// without cgo, for reading properties of the root window.
type x11Conn struct {
	dpy               uintptr
	root              uint64
	getWindowProperty func(dpy uintptr, w, property uint64, offset, length int64, del bool, reqType uint64, actualType, actualFormat, nitems, bytesAfter unsafe.Pointer, prop *unsafe.Pointer) int32
	free              func(p unsafe.Pointer) int32
	internAtom        func(dpy uintptr, name *byte, onlyIfExists bool) uint64
}

func openX11() (x *x11Conn, ok bool) {
	var handle uintptr
	for _, name := range []string{"libX11.so.6", "libX11.so"} {
		if h, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
			handle = h
			break
		}
	}
	if handle == 0 {
		return nil, false
	}
	// RegisterLibFunc panics on a missing symbol; a scale that cannot be read
	// is no reason to stop the application.
	defer func() {
		if recover() != nil {
			x, ok = nil, false
		}
	}()
	var (
		openDisplay       func(name *byte) uintptr
		defaultRootWindow func(dpy uintptr) uint64
	)
	x = &x11Conn{}
	purego.RegisterLibFunc(&openDisplay, handle, "XOpenDisplay")
	purego.RegisterLibFunc(&defaultRootWindow, handle, "XDefaultRootWindow")
	purego.RegisterLibFunc(&x.getWindowProperty, handle, "XGetWindowProperty")
	purego.RegisterLibFunc(&x.free, handle, "XFree")
	purego.RegisterLibFunc(&x.internAtom, handle, "XInternAtom")

	// A connection of its own, used only from LayoutF on the main thread, so
	// it shares nothing with GLFW's. Each reader opens one.
	x.dpy = openDisplay(nil)
	if x.dpy == 0 {
		return nil, false
	}
	x.root = defaultRootWindow(x.dpy)
	return x, true
}

func (x *xftReader) scale() float64 {
	now := time.Now()
	if now.Before(x.next) {
		return x.last
	}
	x.next = now.Add(time.Second)
	x.last = parseXftDPI(x.resources()) / 96
	return x.last
}

// resources reads the RESOURCE_MANAGER property of the root window.
func (x *x11Conn) resources() string {
	const (
		xaResourceManager = 23 // predefined by the protocol
		xaString          = 31
		maxLength         = 1 << 16 // in 32 bit units; the property is a few KiB
	)
	var (
		actualType   uint64
		actualFormat int32
		nitems       uint64
		bytesAfter   uint64
		prop         unsafe.Pointer
	)
	status := x.getWindowProperty(x.dpy, x.root, xaResourceManager, 0, maxLength, false, xaString,
		unsafe.Pointer(&actualType), unsafe.Pointer(&actualFormat),
		unsafe.Pointer(&nitems), unsafe.Pointer(&bytesAfter), &prop)
	if status != 0 || prop == nil {
		return ""
	}
	defer x.free(prop)
	if actualFormat != 8 {
		return ""
	}
	return string(unsafe.Slice((*byte)(prop), int(nitems)))
}

// parseXftDPI finds Xft.dpi in a resource string as xrdb writes it, one
// "name:\tvalue" per line, or returns 0.
func parseXftDPI(res string) float64 {
	for line := range strings.Lines(res) {
		name, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) != "Xft.dpi" {
			continue
		}
		dpi, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || !(dpi > 0) {
			return 0
		}
		return dpi
	}
	return 0
}
