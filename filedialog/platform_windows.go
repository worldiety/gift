//go:build windows

package filedialog

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// This file is the Windows half: the common item dialog, IFileOpenDialog and
// IFileSaveDialog of shobjidl_core.h, reached without cgo through syscall and
// vtables called by hand, in the way of gift/notify.
//
//	CoCreateInstance(FileOpenDialog | FileSaveDialog)
//	GetOptions, SetOptions                   fosFor in winlogic.go
//	SetTitle, SetOkButtonLabel, SetFileTypes, SetFileName, SetDefaultExtension
//	SHCreateItemFromParsingName, SetFolder   the start directory
//	Show(owner)                              modal, on the dialog's own thread
//	GetResults or GetResult, GetDisplayName  the paths
//
// The interface ids and the order of the vtables are those of
// shobjidl_core.h of the Windows SDK, checked on 05.11.2026 against the
// generated bindings of microsoft/windows-rs. The vtable of IFileOpenDialog
// and of IFileSaveDialog starts with IUnknown (0 to 2), IModalWindow (3) and
// IFileDialog (4 to 26); both are used through those slots, only GetResults
// is IFileOpenDialog's own.
//
// # One thread per dialog
//
// The dialog is an object of a single-threaded apartment and runs a modal
// message loop inside Show, which returns only when the dialog has closed.
// So every open has a goroutine of its own, locked to its OS thread, which
// joins a new STA with CoInitializeEx, creates and fills the dialog, shows
// it, reads the answer and leaves the apartment again. The goroutine never
// unlocks its thread: the thread then ends with the goroutine, and with it
// whatever COM and the dialog left behind on it, rather than going back to
// the Go scheduler in an apartment.
//
// open waits only until the dialog is created and filled, which takes
// milliseconds, so that a failure of CoCreateInstance can be reported as one
// that lasts; Show runs after open has returned.
//
// # Owner
//
// The dialog is owned by the application's window: the first visible
// top-level window of this process without an owner, preferring a window of
// the class GLFW30, which is Ebitengine's, and never GLFW's hidden helper
// window. Show disables its owner while the dialog is open, which is the
// modality the person expects, and attaches the input queues of the
// owner's thread and the dialog's for that time. That is the accepted price
// of an owned dialog; Ebitengine's thread keeps running its frames, since it
// is not the one inside Show. Without such a window the dialog has no owner.
//
// # Closing from code
//
// IFileDialog.Close must be called on the dialog's thread, which sits in
// Show, so close does not call into COM at all. It sets a flag that keeps a
// dialog that is not shown yet from being shown, and posts WM_COMMAND with
// IDCANCEL to the visible dialog boxes (class #32770) of the dialog's
// thread, which the modal loop takes as the Cancel button: Show returns with
// ERROR_CANCELLED and done reports a cancel, which the core drops. A dialog
// that has no window yet is looked for again every 50 ms, and the posting
// goes on until Show has returned, so that a message box in front of the
// dialog, such as the question before overwriting, is cancelled first and
// the dialog after it. The close is therefore never final.

var (
	winOle32             = windows.NewLazySystemDLL("ole32.dll")
	winProcCoCreateInst  = winOle32.NewProc("CoCreateInstance")
	winShell32           = windows.NewLazySystemDLL("shell32.dll")
	winProcSHCreateItem  = winShell32.NewProc("SHCreateItemFromParsingName")
	winUser32            = windows.NewLazySystemDLL("user32.dll")
	winProcEnumThreadWnd = winUser32.NewProc("EnumThreadWindows")
	winProcPostMessage   = winUser32.NewProc("PostMessageW")
	winProcGetWindow     = winUser32.NewProc("GetWindow")
)

// winMustGUID turns a GUID literal of winlogic.go into a Windows GUID. It is
// parsed here and not by windows.GUIDFromString, which would load ole32
// while the package initialises, in every program that imports it. A copy
// of mustGUID in gift/notify.
func winMustGUID(s string) windows.GUID {
	u, err := guidBytes(s)
	if err != nil {
		panic(err)
	}
	return winGUIDOf(u)
}

// winGUIDOf turns the bytes of a UUID into a GUID, whose first three fields
// are integers read most significant byte first. A copy of guidOf in
// gift/notify.
func winGUIDOf(u [16]byte) windows.GUID {
	g := windows.GUID{
		Data1: binary.BigEndian.Uint32(u[0:4]),
		Data2: binary.BigEndian.Uint16(u[4:6]),
		Data3: binary.BigEndian.Uint16(u[6:8]),
	}
	copy(g.Data4[:], u[8:16])
	return g
}

var (
	winCLSIDFileOpenDialog = winMustGUID(clsidFileOpenDialog)
	winCLSIDFileSaveDialog = winMustGUID(clsidFileSaveDialog)
	winIIDIFileOpenDialog  = winMustGUID(iidIFileOpenDialog)
	winIIDIFileSaveDialog  = winMustGUID(iidIFileSaveDialog)
	winIIDIShellItem       = winMustGUID(iidIShellItem)
)

// Vtable slots, each with its interface.
const (
	winSlotRelease             = 2  // IUnknown
	winSlotShow                = 3  // IModalWindow
	winSlotSetFileTypes        = 4  // IFileDialog
	winSlotSetOptions          = 9  // IFileDialog
	winSlotGetOptions          = 10 // IFileDialog
	winSlotSetFolder           = 12 // IFileDialog
	winSlotSetFileName         = 15 // IFileDialog
	winSlotSetTitle            = 17 // IFileDialog
	winSlotSetOkButtonLabel    = 18 // IFileDialog
	winSlotGetResult           = 20 // IFileDialog
	winSlotSetDefaultExtension = 22 // IFileDialog
	winSlotGetResults          = 27 // IFileOpenDialog
	winSlotGetDisplayName      = 5  // IShellItem
	winSlotGetCount            = 7  // IShellItemArray
	winSlotGetItemAt           = 8  // IShellItemArray
)

const (
	winClsctxInprocServer = 1
	winWMCommand          = 0x0111
	winIDCancel           = 2
	winGWOwner            = 4
	winClosePoll          = 50 * time.Millisecond
)

// winFilterSpec is COMDLG_FILTERSPEC of shtypes.h.
type winFilterSpec struct {
	name *uint16
	spec *uint16
}

// winVcall calls method slot of the COM object obj with obj as the first
// argument. uintptrescapes keeps pointers that callers convert to uintptr in
// the argument list alive and on the heap for the duration of the call, as
// it does for the call of a syscall. A copy of vcall in gift/notify, which
// returns the HRESULT.
//
//go:uintptrescapes
func winVcall(obj unsafe.Pointer, slot int, args ...uintptr) uint32 {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(slot)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return hresult(r)
}

func winRelease(obj unsafe.Pointer) {
	if obj != nil {
		winVcall(obj, winSlotRelease)
	}
}

// winUTF16 is s as a NUL-terminated UTF-16 string. winText has removed every
// NUL, so the conversion cannot fail.
func winUTF16(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(winText(s))
	if err != nil {
		p, _ = windows.UTF16PtrFromString("")
	}
	return p
}

// The one callback of EnumWindows and EnumThreadWindows: the number of
// callbacks a process can make is limited, so it is made once and collects
// every window into winEnumed, which the caller filters afterwards, outside
// the callback. winEnumMu is held around each enumeration.
var (
	winEnumMu       sync.Mutex
	winEnumed       []windows.HWND
	winEnumCallback = windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		winEnumed = append(winEnumed, h)
		return 1
	})
)

// winTopLevel lists the top-level windows of thread tid, or of every thread
// with tid 0.
func winTopLevel(tid uint32) []windows.HWND {
	winEnumMu.Lock()
	defer winEnumMu.Unlock()
	winEnumed = winEnumed[:0]
	if tid == 0 {
		_ = windows.EnumWindows(winEnumCallback, nil)
	} else {
		winProcEnumThreadWnd.Call(uintptr(tid), winEnumCallback, 0)
	}
	return append([]windows.HWND(nil), winEnumed...)
}

func winClassName(h windows.HWND) string {
	var buf [64]uint16
	n, err := windows.GetClassName(h, &buf[0], int32(len(buf)))
	if err != nil || n <= 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// winOwner is the window the dialog is shown on; see "Owner" at the top of
// this file.
func winOwner() windows.HWND {
	self := uint32(os.Getpid())
	var cands []windows.HWND
	for _, h := range winTopLevel(0) {
		var pid uint32
		if _, err := windows.GetWindowThreadProcessId(h, &pid); err != nil || pid != self {
			continue
		}
		if !windows.IsWindowVisible(h) {
			continue
		}
		if owner, _, _ := winProcGetWindow.Call(uintptr(h), winGWOwner); owner != 0 {
			continue
		}
		cands = append(cands, h)
	}
	return winPickOwner(cands, winClassName)
}

func platformAvailable() bool { return true }

// winPlatform is the Windows layer; it holds nothing but the logger, every
// dialog has its own thread.
type winPlatform struct {
	log *slog.Logger
}

func openPlatform(cfg openConfig) (platform, error) {
	return &winPlatform{log: cfg.log}, nil
}

// winDialog is one open dialog.
type winDialog struct {
	log *slog.Logger
	// tid is the dialog's thread, set before open returns.
	tid uint32
	// closeRequested is set by close; a dialog that is not shown yet is
	// not shown then.
	closeRequested atomic.Bool
	closeOnce      sync.Once
	// returned is closed when Show has returned or was skipped, by
	// showEnded.
	returned     chan struct{}
	returnedOnce sync.Once
}

func (d *winDialog) showEnded() { d.returnedOnce.Do(func() { close(d.returned) }) }

func (p *winPlatform) open(r request, done func(result)) (func() bool, error) {
	d := &winDialog{log: p.log, returned: make(chan struct{})}
	ready := make(chan error, 1)
	go d.run(r, ready, done)
	if err := <-ready; err != nil {
		return nil, err
	}
	return d.close, nil
}

// close is the close of [platform]; see "Closing from code" at the top of
// this file.
func (d *winDialog) close() bool {
	d.closeRequested.Store(true)
	d.closeOnce.Do(func() { go d.cancelLoop() })
	return false
}

func (d *winDialog) cancelLoop() {
	t := time.NewTicker(winClosePoll)
	defer t.Stop()
	for {
		for _, h := range winTopLevel(d.tid) {
			if windows.IsWindowVisible(h) && winClassName(h) == winDialogClass {
				winProcPostMessage.Call(uintptr(h), winWMCommand, winIDCancel, 0)
			}
		}
		select {
		case <-d.returned:
			return
		case <-t.C:
		}
	}
}

// run is the dialog's goroutine. It reports on ready whether the dialog
// could be made, and when it could, calls done once with the answer.
func (d *winDialog) run(r request, ready chan<- error, done func(result)) {
	// Never unlocked; see "One thread per dialog" at the top of this file.
	runtime.LockOSThread()
	defer d.showEnded()
	res, ok := d.show(r, ready)
	if ok {
		done(res)
	}
}

// show makes, shows and reads the dialog, and frees all of it and leaves the
// apartment before it returns. ok is false when the dialog could not be made
// and the failure went to ready instead.
func (d *winDialog) show(r request, ready chan<- error) (res result, ok bool) {
	d.tid = windows.GetCurrentThreadId()
	var hr uint32
	if err := windows.CoInitializeEx(0, winCoInitFlags); err != nil {
		var errno syscall.Errno
		if !errors.As(err, &errno) {
			ready <- fmt.Errorf("gift/filedialog: CoInitializeEx: %w", err)
			return result{}, false
		}
		hr = uint32(errno)
	}
	uninit, err := coInitResult(hr)
	if uninit {
		defer windows.CoUninitialize()
	}
	if err != nil {
		ready <- err
		return result{}, false
	}

	clsid, iid, name := &winCLSIDFileOpenDialog, &winIIDIFileOpenDialog, "FileOpenDialog"
	if r.mode == Save {
		clsid, iid, name = &winCLSIDFileSaveDialog, &winIIDIFileSaveDialog, "FileSaveDialog"
	}
	var dlg unsafe.Pointer
	rc, _, _ := winProcCoCreateInst.Call(uintptr(unsafe.Pointer(clsid)), 0, winClsctxInprocServer,
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&dlg)))
	if err := hrError("CoCreateInstance("+name+")", hresult(rc)); err != nil || dlg == nil {
		if err == nil {
			err = fmt.Errorf("gift/filedialog: CoCreateInstance(%s) returned no object", name)
		}
		ready <- permanent(fmt.Errorf("%w: %w", ErrUnsupported, err))
		return result{}, false
	}
	defer winRelease(dlg)

	keep, err := d.fill(dlg, r)
	if err != nil {
		ready <- err
		return result{}, false
	}
	ready <- nil

	if d.closeRequested.Load() {
		d.showEnded()
		return result{cancelled: true}, true
	}
	owner := winOwner()
	hr = winVcall(dlg, winSlotShow, uintptr(owner))
	d.showEnded()
	runtime.KeepAlive(keep)
	if hr == hrCancelled || hrFailed(hr) {
		return winResult(hr, nil, nil), true
	}
	paths, err := winPaths(dlg, r.mode)
	return winResult(hr, paths, err), true
}

// winKeep holds what the dialog was handed until Show has returned. The
// dialog copies its strings, as far as the documentation tells, but keeping
// them costs nothing and does not depend on it.
type winKeep struct {
	strs []*uint16
	spec *winFilterSpec
}

// fill sets options, texts, filter and start directory. A start directory
// that Windows does not find is left to the system, with a Debug record.
func (d *winDialog) fill(dlg unsafe.Pointer, r request) (*winKeep, error) {
	k := &winKeep{}
	str := func(s string) uintptr {
		p := winUTF16(s)
		k.strs = append(k.strs, p)
		return uintptr(unsafe.Pointer(p))
	}

	var opts uint32
	if err := hrError("IFileDialog.GetOptions", winVcall(dlg, winSlotGetOptions, uintptr(unsafe.Pointer(&opts)))); err != nil {
		return nil, err
	}
	if err := hrError("IFileDialog.SetOptions", winVcall(dlg, winSlotSetOptions, uintptr(fosFor(r.mode, opts)))); err != nil {
		return nil, err
	}
	if err := hrError("IFileDialog.SetTitle", winVcall(dlg, winSlotSetTitle, str(r.title))); err != nil {
		return nil, err
	}
	if err := hrError("IFileDialog.SetOkButtonLabel", winVcall(dlg, winSlotSetOkButtonLabel, str(r.accept))); err != nil {
		return nil, err
	}
	if r.mode != Directory {
		if label, pattern := filterSpec(r.filterName, r.exts); pattern != "" {
			k.spec = &winFilterSpec{name: winUTF16(label), spec: winUTF16(pattern)}
			if err := hrError("IFileDialog.SetFileTypes", winVcall(dlg, winSlotSetFileTypes, 1, uintptr(unsafe.Pointer(k.spec)))); err != nil {
				return nil, err
			}
		}
	}
	if r.mode == Save {
		if r.suggested != "" {
			if err := hrError("IFileDialog.SetFileName", winVcall(dlg, winSlotSetFileName, str(r.suggested))); err != nil {
				return nil, err
			}
		}
		if ext := defaultExtension(r.exts); ext != "" {
			if err := hrError("IFileDialog.SetDefaultExtension", winVcall(dlg, winSlotSetDefaultExtension, str(ext))); err != nil {
				return nil, err
			}
		}
	}
	if r.start != "" {
		var item unsafe.Pointer
		rc, _, _ := winProcSHCreateItem.Call(str(r.start), 0, uintptr(unsafe.Pointer(&winIIDIShellItem)), uintptr(unsafe.Pointer(&item)))
		err := hrError("SHCreateItemFromParsingName", hresult(rc))
		if err == nil && item != nil {
			err = hrError("IFileDialog.SetFolder", winVcall(dlg, winSlotSetFolder, uintptr(item)))
			winRelease(item)
		}
		if err != nil {
			logf(d.log, slog.LevelDebug, "the start directory is left to the system", "start", r.start, "error", err)
		}
	}
	return k, nil
}

// winPaths reads the answer of a dialog whose Show succeeded: all items of
// an open dialog, the one item of a folder or save dialog.
func winPaths(dlg unsafe.Pointer, m Mode) ([]string, error) {
	if m == Save || m == Directory {
		var item unsafe.Pointer
		if err := hrError("IFileDialog.GetResult", winVcall(dlg, winSlotGetResult, uintptr(unsafe.Pointer(&item)))); err != nil {
			return nil, err
		}
		defer winRelease(item)
		p, err := winPath(item)
		if err != nil {
			return nil, err
		}
		return []string{p}, nil
	}
	var arr unsafe.Pointer
	if err := hrError("IFileOpenDialog.GetResults", winVcall(dlg, winSlotGetResults, uintptr(unsafe.Pointer(&arr)))); err != nil {
		return nil, err
	}
	defer winRelease(arr)
	var n uint32
	if err := hrError("IShellItemArray.GetCount", winVcall(arr, winSlotGetCount, uintptr(unsafe.Pointer(&n)))); err != nil {
		return nil, err
	}
	paths := make([]string, 0, n)
	for i := range n {
		var item unsafe.Pointer
		if err := hrError("IShellItemArray.GetItemAt", winVcall(arr, winSlotGetItemAt, uintptr(i), uintptr(unsafe.Pointer(&item)))); err != nil {
			return nil, err
		}
		p, err := winPath(item)
		winRelease(item)
		if err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}

// winPath is the file system path of a shell item.
func winPath(item unsafe.Pointer) (string, error) {
	if item == nil {
		return "", errors.New("gift/filedialog: the dialog returned no item")
	}
	var s *uint16
	if err := hrError("IShellItem.GetDisplayName", winVcall(item, winSlotGetDisplayName, sigdnFileSysPath, uintptr(unsafe.Pointer(&s)))); err != nil {
		return "", err
	}
	if s == nil {
		return "", errors.New("gift/filedialog: the dialog returned an item without a path")
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(s))
	return windows.UTF16PtrToString(s), nil
}
