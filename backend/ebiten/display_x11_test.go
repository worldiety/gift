//go:build linux && !android

package ebiten

import (
	"bytes"
	"os"
	"testing"

	"github.com/ebitengine/purego"

	"github.com/worldiety/gift/cms"
)

// The profile is read from the root window as the ICC Profiles in X
// specification puts it there. The test writes properties on the root
// window of whatever X server DISPLAY names, so it runs only when
// GIFT_XVFB says that is a server of its own:
//
//	GIFT_XVFB=1 xvfb-run go test -run X11 ./backend/ebiten/
func TestTheProfileIsReadFromTheXRootWindow(t *testing.T) {
	if os.Getenv("GIFT_XVFB") == "" {
		t.Skip("writes on the root window; run under xvfb-run with GIFT_XVFB=1")
	}
	x, ok := openX11()
	if !ok {
		t.Fatal("no X server")
	}
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Fatal(err)
	}
	var (
		change func(dpy uintptr, w, property, typ uint64, format, mode int32, data *byte, n int32) int32
		remove func(dpy uintptr, w, property uint64) int32
	)
	purego.RegisterLibFunc(&change, lib, "XChangeProperty")
	purego.RegisterLibFunc(&remove, lib, "XDeleteProperty")
	set := func(name string, data []byte) {
		cname := append([]byte(name), 0)
		atom := x.internAtom(x.dpy, &cname[0], false)
		const cardinal, replace = 6, 0
		change(x.dpy, x.root, atom, cardinal, 8, replace, &data[0], int32(len(data)))
		t.Cleanup(func() { remove(x.dpy, x.root, atom) })
	}

	first, second := cms.DisplayP3().Bytes(), cms.AdobeRGB().Bytes()
	set("_ICC_PROFILE", first)
	set("_ICC_PROFILE_1", second)
	for i, want := range [][]byte{first, second, nil} {
		got, err := x.property(iccPropertyName(i))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("display %d: %d bytes, %v; want %d", i, len(got), err, len(want))
		}
	}

	// Under Wayland nothing is read.
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	if r := newDisplayReader(nil); r != nil {
		t.Error("a reader under Wayland")
	}
}
