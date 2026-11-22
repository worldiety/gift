//go:build linux && !android

package ebiten

import (
	"errors"
	"log/slog"
	"os"
	"strconv"
	"unsafe"

	eb "github.com/hajimehoshi/ebiten/v2"
)

// newDisplayReader reads the profile of the display from the X server, where
// the ICC Profiles in X specification puts it: the property _ICC_PROFILE of
// the root window for the first display, _ICC_PROFILE_1 for the second and
// so on. colord, GNOME and KDE put it there on X11, and so does a person
// with xcalib or dispwin.
//
// Under Wayland nothing is read: gift is an X client of the compositor
// there, and whether the compositor converts what X clients show is its own
// business. A person whose compositor does not names a profile in the user
// configuration.
func newDisplayReader(log *slog.Logger) displayReader {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if log != nil {
			log.Info("gift/backend/ebiten: Wayland, the display's colours are the compositor's")
		}
		return nil
	}
	x, ok := openX11()
	if !ok {
		return nil
	}
	return func() ([]byte, string, error) {
		name := iccPropertyName(monitorIndex())
		data, err := x.property(name)
		return data, name, err
	}
}

// iccPropertyName is the root window property holding the profile of
// display i.
func iccPropertyName(i int) string {
	if i <= 0 {
		return "_ICC_PROFILE"
	}
	return "_ICC_PROFILE_" + strconv.Itoa(i)
}

// monitorIndex is the place of the window's monitor among Ebitengine's,
// which are GLFW's, the primary one first, as the X screens of the
// specification are numbered on every desktop that writes them.
func monitorIndex() int {
	m := eb.Monitor()
	if m == nil {
		return 0
	}
	for i, o := range eb.AppendMonitors(nil) {
		if o == m {
			return i
		}
	}
	return 0
}

// property reads a property of the root window as bytes, nil when there is
// none.
func (x *x11Conn) property(name string) ([]byte, error) {
	cname := append([]byte(name), 0)
	atom := x.internAtom(x.dpy, &cname[0], true)
	if atom == 0 {
		return nil, nil
	}
	const (
		anyPropertyType = 0
		maxLength       = 1 << 22 // in 32 bit units, 16 MiB
	)
	var (
		actualType   uint64
		actualFormat int32
		nitems       uint64
		bytesAfter   uint64
		prop         unsafe.Pointer
	)
	status := x.getWindowProperty(x.dpy, x.root, atom, 0, maxLength, false, anyPropertyType,
		unsafe.Pointer(&actualType), unsafe.Pointer(&actualFormat),
		unsafe.Pointer(&nitems), unsafe.Pointer(&bytesAfter), &prop)
	if status != 0 {
		return nil, errors.New("gift/backend/ebiten: XGetWindowProperty failed")
	}
	if prop == nil {
		return nil, nil
	}
	defer x.free(prop)
	switch {
	case actualType == 0:
		return nil, nil
	case actualFormat != 8:
		return nil, errors.New("gift/backend/ebiten: " + name + " is not a byte array")
	case bytesAfter != 0:
		return nil, errors.New("gift/backend/ebiten: " + name + " is larger than 16 MiB")
	}
	return append([]byte(nil), unsafe.Slice((*byte)(prop), int(nitems))...), nil
}
