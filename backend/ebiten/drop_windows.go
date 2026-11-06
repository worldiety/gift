//go:build windows && (amd64 || arm64)

package ebiten

import (
	"log/slog"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/worldiety/gift"
	"golang.org/x/sys/windows"
)

// The Windows drag layer; see drop.go and the project plan, section 49.
//
// Ebitengine accepts files with DragAcceptFiles and WM_DROPFILES, which
// carries CF_HDROP and nothing before the drop. This layer registers an OLE
// drop target of its own on the same window, written in Go with a static
// vtable like the COM objects of package notify. OLE takes precedence over
// WM_DROPFILES for every source that speaks it, which is every source that
// matters; a WM_DROPFILES that still arrives is Ebitengine's report and is
// discarded when it repeats a drop of this layer.
//
// An elevated process is left alone: OLE drag and drop does not cross
// integrity levels, while Ebitengine's WM_DROPFILES is allowed through UIPI.
// Such a process keeps the drop of files without a live session.

var (
	ole32    = windows.NewLazySystemDLL("ole32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procOleInitialize            = ole32.NewProc("OleInitialize")
	procOleUninitialize          = ole32.NewProc("OleUninitialize")
	procRegisterDragDrop         = ole32.NewProc("RegisterDragDrop")
	procRevokeDragDrop           = ole32.NewProc("RevokeDragDrop")
	procReleaseStgMedium         = ole32.NewProc("ReleaseStgMedium")
	procDragQueryFileW           = shell32.NewProc("DragQueryFileW")
	procEnumThreadWindows        = user32.NewProc("EnumThreadWindows")
	procGetClassNameW            = user32.NewProc("GetClassNameW")
	procGetWindowLongPtrW        = user32.NewProc("GetWindowLongPtrW")
	procScreenToClient           = user32.NewProc("ScreenToClient")
	procRegisterClipboardFormatW = user32.NewProc("RegisterClipboardFormatW")
	procGlobalLock               = kernel32.NewProc("GlobalLock")
	procGlobalUnlock             = kernel32.NewProc("GlobalUnlock")
)

const (
	sOK              = 0
	sFalse           = 1
	eNoInterface     = 0x80004002
	rpcEChangedMode  = 0x80010106
	dragdropSDrop    = 0
	dropEffectNone   = 0
	dropEffectCopy   = 1
	cfUnicodeText    = 13
	cfHDrop          = 15
	tymedHGlobal     = 1
	dvaspectContent  = 1
	gwlExStyle       = ^uintptr(19) // -20
	wsExAcceptFiles  = 0x10
	glfwWindowClass  = "GLFW30"
	dataObjGetData   = 3
	dataObjQueryData = 5
)

// formatEtc is FORMATETC, 32 bytes.
type formatEtc struct {
	cfFormat uint16
	ptd      uintptr
	aspect   uint32
	lindex   int32
	tymed    uint32
}

// stgMedium is STGMEDIUM, 24 bytes.
type stgMedium struct {
	tymed   uint32
	hGlobal uintptr
	release uintptr
}

type pointL struct{ x, y int32 }

// The sizes are fixed by the ABI; the compiler checks them both ways.
var (
	_ [unsafe.Sizeof(formatEtc{}) - 32]byte
	_ [32 - unsafe.Sizeof(formatEtc{})]byte
	_ [unsafe.Sizeof(stgMedium{}) - 24]byte
	_ [24 - unsafe.Sizeof(stgMedium{})]byte
	_ [unsafe.Sizeof(pointL{}) - 8]byte
	_ [8 - unsafe.Sizeof(pointL{})]byte
)

var (
	iidIUnknown    = windows.GUID{Data1: 0x00000000, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIDropTarget = windows.GUID{Data1: 0x00000122, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
)

// dropTarget is the COM object Windows holds. It is a package variable so
// that the garbage collector never moves what Windows points at; there is one
// window.
type dropTarget struct {
	vtbl *[7]uintptr
}

var (
	targetOnce sync.Once
	target     dropTarget
	targetVtbl [7]uintptr
	activeWin  *winDrops
)

type winDrops struct {
	dropQueue
	hwnd   windows.HWND
	log    *slog.Logger
	cfURL  uint16
	kinds  gift.DropKind // of the drag in progress, from DragEnter
	inside bool
}

func openNativeDrops(wake func(), log *slog.Logger) nativeDrops {
	if windows.GetCurrentProcessToken().IsElevated() {
		if log != nil {
			log.Debug("gift/backend/ebiten: the process is elevated; OLE drag and drop does not reach it, files arrive through WM_DROPFILES")
		}
		return nil
	}
	if r, _, _ := procOleInitialize.Call(0); r != sOK && r != sFalse {
		if log != nil {
			log.Debug("gift/backend/ebiten: OleInitialize failed, the Windows drag layer stays off", slog.Uint64("hresult", uint64(r)))
		}
		return nil
	}
	w := &winDrops{log: log}
	w.wake = wake
	w.evs = make([]nativeDropEvent, 0, 8)
	w.hwnd = findGLFWWindow()
	if w.hwnd == 0 {
		procOleUninitialize.Call()
		w.debug("gift/backend/ebiten: no GLFW30 window that accepts files on this thread, the Windows drag layer stays off")
		return nil
	}
	name, _ := windows.UTF16PtrFromString("UniformResourceLocatorW")
	r, _, _ := procRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(name)))
	w.cfURL = uint16(r)
	targetOnce.Do(makeDropTarget)
	activeWin = w
	if r, _, _ := procRegisterDragDrop.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&target))); r != sOK {
		activeWin = nil
		procOleUninitialize.Call()
		w.debug("gift/backend/ebiten: RegisterDragDrop failed, the Windows drag layer stays off")
		return nil
	}
	return w
}

func (w *winDrops) debug(msg string) {
	if w.log != nil {
		w.log.Debug(msg)
	}
}

// close runs after the game ended, when Ebitengine may have destroyed the
// window already; a revoke that fails then is only logged, the registration
// dies with the process.
func (w *winDrops) close() {
	if r, _, _ := procRevokeDragDrop.Call(uintptr(w.hwnd)); r != sOK && w.log != nil {
		w.log.Debug("gift/backend/ebiten: RevokeDragDrop failed", slog.Uint64("hresult", uint64(r)))
	}
	activeWin = nil
	procOleUninitialize.Call()
}

// findGLFWWindow is the window of class GLFW30 on this thread that accepts
// files. GLFW's hidden helper window has the same class and does not.
func findGLFWWindow() windows.HWND {
	var found windows.HWND
	cb := windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		var buf [32]uint16
		n, _, _ := procGetClassNameW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if windows.UTF16ToString(buf[:n]) != glfwWindowClass {
			return 1
		}
		ex, _, _ := procGetWindowLongPtrW.Call(uintptr(h), gwlExStyle)
		if ex&wsExAcceptFiles == 0 {
			return 1
		}
		found = h
		return 0
	})
	procEnumThreadWindows.Call(uintptr(windows.GetCurrentThreadId()), cb, 0)
	return found
}

func makeDropTarget() {
	addRef := windows.NewCallback(func(this unsafe.Pointer) uintptr { return 1 })
	targetVtbl = [7]uintptr{
		// QueryInterface
		windows.NewCallback(func(this unsafe.Pointer, riid *windows.GUID, ppv *unsafe.Pointer) uintptr {
			if *riid == iidIUnknown || *riid == iidIDropTarget {
				*ppv = this
				return sOK
			}
			*ppv = nil
			return eNoInterface
		}),
		addRef,
		addRef, // Release
		// DragEnter(pDataObj, grfKeyState, pt, pdwEffect)
		windows.NewCallback(func(this, data unsafe.Pointer, keys uintptr, pt uintptr, effect *uint32) uintptr {
			if w := activeWin; w != nil {
				w.kinds = w.queryKinds(data)
				w.inside = false
				*effect = w.over(pt, *effect)
			} else {
				*effect = dropEffectNone
			}
			return sOK
		}),
		// DragOver(grfKeyState, pt, pdwEffect)
		windows.NewCallback(func(this unsafe.Pointer, keys uintptr, pt uintptr, effect *uint32) uintptr {
			if w := activeWin; w != nil {
				*effect = w.over(pt, *effect)
			} else {
				*effect = dropEffectNone
			}
			return sOK
		}),
		// DragLeave
		windows.NewCallback(func(this unsafe.Pointer) uintptr {
			if w := activeWin; w != nil {
				w.leave()
			}
			return sOK
		}),
		// Drop(pDataObj, grfKeyState, pt, pdwEffect)
		windows.NewCallback(func(this, data unsafe.Pointer, keys uintptr, pt uintptr, effect *uint32) uintptr {
			w := activeWin
			if w == nil {
				*effect = dropEffectNone
				return sOK
			}
			x, y := w.client(pt)
			d := w.read(data)
			w.push(nativeDropEvent{op: nativeDrop, x: x, y: y, data: d})
			w.inside = false
			if len(d.Items) == 0 || *effect&dropEffectCopy == 0 {
				*effect = dropEffectNone
			} else {
				*effect = dropEffectCopy
			}
			return sOK
		}),
	}
	target.vtbl = &targetVtbl
}

// client is the drag position in client pixels, which is the unit of
// Ebitengine's cursor in gift's layout. POINTL arrives in one register: x in
// the low half, y in the high half.
func (w *winDrops) client(pt uintptr) (float64, float64) {
	p := pointL{x: int32(uint32(pt)), y: int32(uint32(pt >> 32))}
	procScreenToClient.Call(uintptr(w.hwnd), uintptr(unsafe.Pointer(&p)))
	return float64(p.x), float64(p.y)
}

func (w *winDrops) over(pt uintptr, allowed uint32) uint32 {
	if w.kinds == 0 {
		// Nothing gift can read, a virtual file from Outlook say: no
		// session, and the system is told no.
		w.leave()
		return dropEffectNone
	}
	if !w.inside {
		w.inside = true
		w.open()
	}
	x, y := w.client(pt)
	w.push(nativeDropEvent{op: nativeOver, x: x, y: y, kinds: w.kinds})
	if w.wouldAccept() && allowed&dropEffectCopy != 0 {
		return dropEffectCopy
	}
	return dropEffectNone
}

func (w *winDrops) leave() {
	if !w.inside {
		return
	}
	w.inside = false
	w.push(nativeDropEvent{op: nativeLeave})
}

// dataObject calls slot i of the IDataObject vtable.
func dataObject(data unsafe.Pointer, slot int, args ...uintptr) uintptr {
	vtbl := *(**[16]uintptr)(data)
	r, _, _ := syscall.SyscallN(vtbl[slot], append([]uintptr{uintptr(data)}, args...)...)
	return r
}

func (w *winDrops) has(data unsafe.Pointer, cf uint16) bool {
	f := formatEtc{cfFormat: cf, aspect: dvaspectContent, lindex: -1, tymed: tymedHGlobal}
	return cf != 0 && dataObject(data, dataObjQueryData, uintptr(unsafe.Pointer(&f))) == sOK
}

// queryKinds follows [dropPrecedence], so the kinds during the drag are those
// of the drop.
func (w *winDrops) queryKinds(data unsafe.Pointer) gift.DropKind {
	switch {
	case w.has(data, cfHDrop):
		return gift.DropFile
	case w.has(data, w.cfURL):
		k := gift.DropURL
		if t, ok := w.getText(data, cfUnicodeText); ok && !sameAsURLs(t, w.getURLs(data)) {
			k |= gift.DropText
		}
		return k
	case w.has(data, cfUnicodeText):
		return gift.DropText
	}
	return 0
}

// getData runs fn on the locked HGLOBAL of format cf.
func (w *winDrops) getData(data unsafe.Pointer, cf uint16, fn func(h uintptr, p unsafe.Pointer)) bool {
	if cf == 0 {
		return false
	}
	f := formatEtc{cfFormat: cf, aspect: dvaspectContent, lindex: -1, tymed: tymedHGlobal}
	var m stgMedium
	if dataObject(data, dataObjGetData, uintptr(unsafe.Pointer(&f)), uintptr(unsafe.Pointer(&m))) != sOK {
		return false
	}
	defer procReleaseStgMedium.Call(uintptr(unsafe.Pointer(&m)))
	if m.tymed != tymedHGlobal || m.hGlobal == 0 {
		return false
	}
	p, _, _ := procGlobalLock.Call(m.hGlobal)
	if p == 0 {
		return false
	}
	defer procGlobalUnlock.Call(m.hGlobal)
	// Memory Windows owns, locked: not a Go pointer, so no rule of
	// unsafe.Pointer is bent by reading it.
	fn(m.hGlobal, *(*unsafe.Pointer)(unsafe.Pointer(&p)))
	return true
}

func (w *winDrops) getText(data unsafe.Pointer, cf uint16) (string, bool) {
	var s string
	ok := w.getData(data, cf, func(_ uintptr, p unsafe.Pointer) { s = windows.UTF16PtrToString((*uint16)(p)) })
	return s, ok
}

func (w *winDrops) getURLs(data unsafe.Pointer) []string {
	u, ok := w.getText(data, w.cfURL)
	if !ok || strings.TrimSpace(u) == "" {
		return nil
	}
	return []string{strings.TrimSpace(u)}
}

func (w *winDrops) getFiles(data unsafe.Pointer) []string {
	var out []string
	// DragQueryFileW takes the HDROP, which is the HGLOBAL itself.
	w.getData(data, cfHDrop, func(h uintptr, _ unsafe.Pointer) {
		n, _, _ := procDragQueryFileW.Call(h, 0xFFFFFFFF, 0, 0)
		for i := range n {
			l, _, _ := procDragQueryFileW.Call(h, i, 0, 0)
			buf := make([]uint16, l+1)
			procDragQueryFileW.Call(h, i, uintptr(unsafe.Pointer(&buf[0])), l+1)
			out = append(out, windows.UTF16ToString(buf))
		}
	})
	return out
}

// read is the dropped content.
func (w *winDrops) read(data unsafe.Pointer) gift.DropData {
	files := w.getFiles(data)
	urls := w.getURLs(data)
	text, hasText := w.getText(data, cfUnicodeText)
	return dropPrecedence(files, urls, text, hasText)
}
