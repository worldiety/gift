//go:build darwin || freebsd || linux

package turbojpeg

import (
	"fmt"
	"image"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// TurboJPEG constants, from turbojpeg.h. They are part of the ABI and have
// not changed since TurboJPEG 1.2.
const (
	tjpfRGBA  = 7 // TJPF_RGBA: R, G, B, A bytes; A is filled with 0xFF.
	tjcsCMYK  = 3 // TJCS_CMYK
	tjcsYCCK  = 4 // TJCS_YCCK
	tjerrWarn = 0 // TJERR_WARNING, from tjGetErrorCode
)

// lib holds the bound functions. Every signature is the C one with int as
// int32 and unsigned long as uint, which is the width of a C long on every
// Unix purego runs on, 32 and 64 bit alike.
var lib struct {
	once sync.Once
	err  error

	scales []scale

	initDecompress func() uintptr
	destroy        func(h uintptr) int32
	header3        func(h uintptr, buf unsafe.Pointer, size uint, w, ht, subsamp, colorspace *int32) int32
	decompress2    func(h uintptr, buf unsafe.Pointer, size uint, dst unsafe.Pointer, w, pitch, ht, pixelFormat, flags int32) int32
	scalingFactors func(n *int32) unsafe.Pointer
	errorStr2      func(h uintptr) unsafe.Pointer
	errorCode      func(h uintptr) int32
}

// candidates are the names libturbojpeg is found under, most specific first.
//
// The versioned soname comes first on Linux because the unversioned
// libturbojpeg.so is the development symlink from libturbojpeg0-dev, which a
// Raspberry Pi that only runs programs does not have. On macOS the plain
// names go through dyld's search path, which does not include Homebrew's
// prefix, so both Homebrew prefixes are named explicitly.
func candidates() []string {
	if runtime.GOOS == "darwin" {
		return []string{
			"libturbojpeg.0.dylib",
			"/opt/homebrew/lib/libturbojpeg.0.dylib",
			"/usr/local/lib/libturbojpeg.0.dylib",
			"/opt/homebrew/opt/jpeg-turbo/lib/libturbojpeg.0.dylib",
			"/usr/local/opt/jpeg-turbo/lib/libturbojpeg.0.dylib",
			"libturbojpeg.dylib",
		}
	}
	return []string{"libturbojpeg.so.0", "libturbojpeg.so"}
}

// load opens and binds the library, once per process.
func load() error {
	lib.once.Do(func() { lib.err = bind() })
	return lib.err
}

func bind() (err error) {
	var h uintptr
	for _, name := range candidates() {
		if h, err = purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_LOCAL); err == nil {
			break
		}
	}
	if h == 0 {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	// RegisterLibFunc panics on a missing symbol. A libturbojpeg without
	// tjDecompressHeader3 or tjGetErrorCode is older than 2.0, and a
	// program that finds one should lose the speed, not its life.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: binding failed: %v", ErrUnavailable, r)
		}
	}()
	purego.RegisterLibFunc(&lib.initDecompress, h, "tjInitDecompress")
	purego.RegisterLibFunc(&lib.destroy, h, "tjDestroy")
	purego.RegisterLibFunc(&lib.header3, h, "tjDecompressHeader3")
	purego.RegisterLibFunc(&lib.decompress2, h, "tjDecompress2")
	purego.RegisterLibFunc(&lib.scalingFactors, h, "tjGetScalingFactors")
	purego.RegisterLibFunc(&lib.errorStr2, h, "tjGetErrorStr2")
	purego.RegisterLibFunc(&lib.errorCode, h, "tjGetErrorCode")

	// The table is static in the library and never changes, so it is read
	// once here rather than on every decode.
	var n int32
	p := lib.scalingFactors(&n)
	if p == nil || n <= 0 {
		return fmt.Errorf("%w: tjGetScalingFactors returned nothing", ErrUnavailable)
	}
	for _, f := range unsafe.Slice((*[2]int32)(p), n) {
		lib.scales = append(lib.scales, scale{int(f[0]), int(f[1])})
	}
	return nil
}

// scales is the library's table once it is loaded and the documented one
// before.
func scales() []scale {
	if load() == nil {
		return lib.scales
	}
	return fallbackScales
}

// decompress is one decode on its own handle.
//
// A handle per call rather than a pool: tjInitDecompress allocates a few
// kilobytes of structs, which is nothing next to a decode, and a handle is
// not safe for concurrent use while a sync.Pool would drop handles without
// giving tjDestroy a chance to run. So a decode costs one malloc more and the
// package has no state to leak.
func decompress(data []byte, minW, minH int) (*image.RGBA, error) {
	if len(data) == 0 {
		return nil, errorf("decode", "empty input")
	}
	h := lib.initDecompress()
	if h == 0 {
		return nil, errorf("init", "tjInitDecompress failed")
	}
	defer lib.destroy(h)

	var w, ht, subsamp, cs int32
	src := unsafe.Pointer(&data[0])
	if lib.header3(h, src, uint(len(data)), &w, &ht, &subsamp, &cs) != 0 && !tolerable(h) {
		runtime.KeepAlive(data)
		return nil, errorf("header", message(h))
	}
	if w <= 0 || ht <= 0 {
		return nil, errorf("header", "no dimensions")
	}
	if cs == tjcsCMYK || cs == tjcsYCCK {
		return nil, ErrUnsupported
	}

	s := chooseScale(lib.scales, int(w), int(ht), minW, minH)
	sw, sh := s.of(int(w)), s.of(int(ht))
	img := image.NewRGBA(image.Rect(0, 0, sw, sh))
	// tjDecompress2 picks the largest factor whose result fits the width
	// and height it is given, and s's own result is exactly that, so this
	// is how a factor is chosen through the 2.x API.
	rc := lib.decompress2(h, src, uint(len(data)), unsafe.Pointer(&img.Pix[0]),
		int32(sw), int32(img.Stride), int32(sh), tjpfRGBA, 0)
	runtime.KeepAlive(data)
	runtime.KeepAlive(img)
	if rc != 0 && !tolerable(h) {
		return nil, errorf("decode", message(h))
	}
	return img, nil
}

// tolerable decides whether a failed call only warned.
//
// TurboJPEG 2 reports libjpeg's warnings as failures, with tjGetErrorCode
// telling them apart, and most warnings are about files every viewer shows
// anyway: extraneous bytes before a marker, a restart marker out of place.
// Those are accepted, as image/jpeg accepts most of them. The exception is a
// premature end of file. libjpeg fills the missing rows with grey and
// carries on, which is the right behaviour for a viewer and the wrong one
// for a cache: a photograph still being written by an upload would be
// thumbnailed half grey, stored, and served from the disk cache until its
// revision changed. image/jpeg fails on it, and so does this.
func tolerable(h uintptr) bool {
	if lib.errorCode(h) != tjerrWarn {
		return false
	}
	return !strings.Contains(message(h), "Premature end")
}

// message is the handle's last error text.
func message(h uintptr) string {
	p := lib.errorStr2(h)
	if p == nil {
		return "unknown error"
	}
	var b strings.Builder
	for q := (*byte)(p); *q != 0; q = (*byte)(unsafe.Add(unsafe.Pointer(q), 1)) {
		b.WriteByte(*q)
	}
	return b.String()
}
