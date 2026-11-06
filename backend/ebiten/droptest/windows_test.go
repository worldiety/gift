//go:build windows && (amd64 || arm64)

package droptest

import (
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"
	"unsafe"

	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift"
	"golang.org/x/sys/windows"
)

// The Windows test calls the slots of gift's IDropTarget directly, as OLE's
// DoDragDrop would, with an IDataObject of its own that offers files, text
// and an address. It needs a desktop session:
//
//	set GIFT_DROP_WINDOWS=1 && go test ./backend/ebiten/droptest -run Windows -v -count=1

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	ole32    = windows.NewLazySystemDLL("ole32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procEnumThreadWindows        = user32.NewProc("EnumThreadWindows")
	procGetClassNameW            = user32.NewProc("GetClassNameW")
	procGetWindowLongPtrW        = user32.NewProc("GetWindowLongPtrW")
	procGetPropW                 = user32.NewProc("GetPropW")
	procClientToScreen           = user32.NewProc("ClientToScreen")
	procRegisterClipboardFormatW = user32.NewProc("RegisterClipboardFormatW")
	procRegisterDragDrop         = ole32.NewProc("RegisterDragDrop")
	procGlobalAlloc              = kernel32.NewProc("GlobalAlloc")
	procGlobalLock               = kernel32.NewProc("GlobalLock")
	procGlobalUnlock             = kernel32.NewProc("GlobalUnlock")
)

const (
	dragdropEAlreadyRegistered = 0x80040101
	gmemMoveable               = 0x0002
	cfUnicodeText              = 13
	cfHDrop                    = 15
	effectCopy                 = 1
	effectMove                 = 2
)

type formatEtc struct {
	cfFormat uint16
	ptd      uintptr
	aspect   uint32
	lindex   int32
	tymed    uint32
}

type stgMedium struct {
	tymed   uint32
	hGlobal uintptr
	release uintptr
}

// fakeData is an IDataObject; the test sets what it offers before each drag.
type fakeData struct{ vtbl *[12]uintptr }

var (
	data     fakeData
	dataVtbl [12]uintptr
	offer    struct {
		files []string
		url   string
		text  string
	}
	cfURL uint16
)

func global(b []byte) uintptr {
	h, _, _ := procGlobalAlloc.Call(gmemMoveable, uintptr(len(b)))
	p, _, _ := procGlobalLock.Call(h)
	copy(unsafe.Slice(*(**byte)(unsafe.Pointer(&p)), len(b)), b)
	procGlobalUnlock.Call(h)
	return h
}

func utf16z(s string) []byte {
	u, _ := windows.UTF16FromString(s)
	return unsafe.Slice((*byte)(unsafe.Pointer(&u[0])), len(u)*2)
}

func offered(cf uint16) bool {
	switch {
	case cf == cfHDrop:
		return len(offer.files) > 0
	case cf == cfUnicodeText:
		return offer.text != ""
	case cf == cfURL:
		return offer.url != ""
	}
	return false
}

func makeData() {
	addRef := windows.NewCallback(func(unsafe.Pointer) uintptr { return 1 })
	notImpl := windows.NewCallback(func(unsafe.Pointer) uintptr { return 0x80004001 })
	dataVtbl = [12]uintptr{
		windows.NewCallback(func(this unsafe.Pointer, _ *windows.GUID, ppv *unsafe.Pointer) uintptr { *ppv = this; return 0 }),
		addRef, addRef,
		// GetData
		windows.NewCallback(func(_ unsafe.Pointer, f *formatEtc, m *stgMedium) uintptr {
			if !offered(f.cfFormat) {
				return 0x80040064 // DV_E_FORMATETC
			}
			var b []byte
			switch f.cfFormat {
			case cfHDrop:
				// DROPFILES, 20 bytes, then the paths, each and the list
				// ending in a zero.
				b = make([]byte, 20)
				b[0], b[16] = 20, 1 // pFiles, fWide
				for _, p := range offer.files {
					b = append(b, utf16z(p)...)
				}
				b = append(b, 0, 0)
			case cfUnicodeText:
				b = utf16z(offer.text)
			default:
				b = utf16z(offer.url)
			}
			*m = stgMedium{tymed: 1, hGlobal: global(b)}
			return 0
		}),
		notImpl, // GetDataHere
		// QueryGetData
		windows.NewCallback(func(_ unsafe.Pointer, f *formatEtc) uintptr {
			if offered(f.cfFormat) {
				return 0
			}
			return 0x80040064
		}),
		notImpl, notImpl, notImpl, notImpl, notImpl, notImpl,
	}
	data.vtbl = &dataVtbl
}

// winTarget returns gift's window and the IDropTarget OLE keeps for it.
func winTarget(t *testing.T) (windows.HWND, unsafe.Pointer) {
	t.Helper()
	var hwnd windows.HWND
	var target uintptr
	deadline := time.Now().Add(10 * time.Second)
	for target == 0 && time.Now().Before(deadline) {
		eb.RunOnMainThread(func() {
			cb := windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
				var buf [32]uint16
				n, _, _ := procGetClassNameW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), 32)
				ex, _, _ := procGetWindowLongPtrW.Call(uintptr(h), ^uintptr(19))
				if windows.UTF16ToString(buf[:n]) == "GLFW30" && ex&0x10 != 0 {
					hwnd = h
					return 0
				}
				return 1
			})
			procEnumThreadWindows.Call(uintptr(windows.GetCurrentThreadId()), cb, 0)
			if hwnd != 0 {
				name, _ := windows.UTF16PtrFromString("OleDropTargetInterface")
				target, _, _ = procGetPropW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(name)))
			}
		})
		if target == 0 {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if target == 0 {
		t.Fatal("no registered drop target on gift's window within 10 s")
	}
	return hwnd, *(*unsafe.Pointer)(unsafe.Pointer(&target))
}

func call(target unsafe.Pointer, slot int, args ...uintptr) uintptr {
	vtbl := *(**[7]uintptr)(target)
	r, _, _ := syscall.SyscallN(vtbl[slot], append([]uintptr{uintptr(target)}, args...)...)
	return r
}

// screen is the packed POINTL of the client point (x, y).
func screen(hwnd windows.HWND, x, y int32) uintptr {
	p := struct{ x, y int32 }{x, y}
	procClientToScreen.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&p)))
	return uintptr(uint32(p.x)) | uintptr(uint32(p.y))<<32
}

// winDrag runs DragEnter, three DragOvers and then Drop or DragLeave; it
// returns the effects the target answered.
func winDrag(hwnd windows.HWND, target unsafe.Pointer, x, y int32, leave bool) []uint32 {
	pt := screen(hwnd, x, y)
	var answers []uint32
	slot := func(i int, withData bool) {
		eb.RunOnMainThread(func() {
			effect := uint32(effectCopy | effectMove)
			if withData {
				call(target, i, uintptr(unsafe.Pointer(&data)), 0, pt, uintptr(unsafe.Pointer(&effect)))
			} else {
				call(target, i, 0, pt, uintptr(unsafe.Pointer(&effect)))
			}
			answers = append(answers, effect)
		})
	}
	slot(3, true) // DragEnter
	for range 3 {
		time.Sleep(60 * time.Millisecond)
		slot(4, false) // DragOver
	}
	if leave {
		eb.RunOnMainThread(func() { call(target, 5) })
		return answers
	}
	slot(6, true) // Drop
	return answers
}

func TestWindowsOLEDropTarget(t *testing.T) {
	need(t, "GIFT_DROP_WINDOWS")
	if c := waitCaps(t, gift.DropFile|gift.DropText|gift.DropURL); !c.Live {
		t.Fatalf("capabilities %+v, want a live session; is the test elevated?", c)
	}
	name, _ := windows.UTF16PtrFromString("UniformResourceLocatorW")
	r, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(name)))
	cfURL = uint16(r)
	makeData()
	hwnd, target := winTarget(t)
	scale := 1.0
	if m := eb.Monitor(); m != nil {
		scale = m.DeviceScaleFactor()
	}
	px := func(v float64) int32 { return int32(v * scale) }

	t.Run("registered once", func(t *testing.T) {
		var r uintptr
		eb.RunOnMainThread(func() {
			r, _, _ = procRegisterDragDrop.Call(uintptr(hwnd), uintptr(target))
		})
		if r != dragdropEAlreadyRegistered {
			t.Errorf("a second RegisterDragDrop answered %#x, want DRAGDROP_E_ALREADYREGISTERED", r)
		}
	})

	t.Run("files", func(t *testing.T) {
		dir := t.TempDir()
		a := filepath.Join(dir, "Grüße a.txt")
		folder := filepath.Join(dir, "Ordner")
		if err := os.WriteFile(a, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		offer.files, offer.url, offer.text = []string{a, folder}, "", a
		drain()
		answers := winDrag(hwnd, target, px(300), px(200), false)
		if answers[2] != effectCopy {
			t.Errorf("effects %v, want copy once gift saw the drag", answers)
		}
		if e := next(t, true); e.area != "outer" || e.targeted == nil || !*e.targeted {
			t.Errorf("first event %+v, want the outer destination targeted", e)
		}
		e := next(t, false)
		if e.area != "outer" || !slices.Equal(e.info.Files(), []string{a, folder}) || !e.info.Items[1].Dir {
			t.Errorf("drop at %q: %+v", e.area, e.info.Items)
		}
		if l := e.info.Location; l.X < 295 || l.X > 305 || l.Y < 195 || l.Y > 205 {
			t.Errorf("Location %v, want (300, 200)", l)
		}
	})

	t.Run("text", func(t *testing.T) {
		offer.files, offer.url, offer.text = nil, "", "Hallo Welt"
		drain()
		winDrag(hwnd, target, px(50), px(50), false)
		if e := next(t, false); e.area != "inner" || len(e.info.Items) != 1 || e.info.Items[0].Text != "Hallo Welt" {
			t.Errorf("text delivered %q %+v", e.area, e.info.Items)
		}
	})

	t.Run("url", func(t *testing.T) {
		const u = "https://example.org/a?b=c"
		offer.files, offer.url, offer.text = nil, u, u
		drain()
		winDrag(hwnd, target, px(50), px(50), false)
		if e := next(t, false); e.area != "inner" || len(e.info.Items) != 1 || e.info.Items[0].URL != u {
			t.Errorf("an address delivered %q %+v", e.area, e.info.Items)
		}
	})

	t.Run("leave", func(t *testing.T) {
		offer.files, offer.url, offer.text = nil, "", "x"
		drain()
		winDrag(hwnd, target, px(50), px(50), true)
		on, off := next(t, true), next(t, true)
		if on.targeted == nil || !*on.targeted || off.targeted == nil || *off.targeted {
			t.Errorf("events %+v then %+v, want targeted true then false", on, off)
		}
	})

	t.Run("refused", func(t *testing.T) {
		offer.files, offer.url, offer.text = nil, "", "x"
		answers := winDrag(hwnd, target, px(300), px(200), true)
		if answers[len(answers)-1] != 0 {
			t.Errorf("effects %v, want none for text nobody there takes", answers)
		}
	})
}
