//go:build windows

package tray

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// This file is the Windows half: an icon in the notification area of the
// taskbar through Shell_NotifyIconW, and a Win32 popup menu, reached without
// cgo through syscall and structures written by hand.
//
//	Shell_NotifyIconW(NIM_ADD, NIM_SETVERSION 4)   the icon appears
//	Shell_NotifyIconW(NIM_MODIFY)                  icon and tooltip change in place
//	Shell_NotifyIconW(NIM_DELETE)                  the icon goes
//	CreatePopupMenu, InsertMenuItemW, TrackPopupMenuEx
//	CreateDIBSection, CreateIconIndirect           the pictures
//
// The layouts of NOTIFYICONDATAW, MENUITEMINFOW, BITMAPINFOHEADER, ICONINFO,
// WNDCLASSEXW and MSG are those of shellapi.h, winuser.h and wingdi.h of the
// Windows SDK, checked on 02.11.2026; tray_windows_test.go pins their sizes
// for 64 and for 32 bit. shellapi.h packs its structures to one byte on 32
// bit only, where every field of NOTIFYICONDATAW is four bytes wide or an
// array of WCHAR anyway, so the natural layout of Go is the right one on
// both.
//
// # One thread, one window
//
// The shell reports clicks on the icon as a message to a window, and a popup
// menu runs a modal message loop on the thread that opens it. So every tray
// has a goroutine locked to an OS thread of its own, with a hidden window and
// a GetMessageW loop: Ebitengine's thread never waits for the menu, and the
// menu never waits for a frame. It is the shape of gift/notify's worker, with
// a message loop where notify has a channel, because a window's thread has to
// pump messages.
//
// The window is an ordinary top-level window that is never shown, not a
// message-only window under HWND_MESSAGE: those receive no broadcasts, and
// three broadcasts matter here. "TaskbarCreated", registered with
// RegisterWindowMessageW, comes when Explorer has restarted and forgotten
// every icon, which is then added again; WM_SETTINGCHANGE with
// "ImmersiveColorSet" comes when the taskbar turns light or dark; and
// WM_DISPLAYCHANGE and WM_DPICHANGED come when the size of a small icon may
// have changed. WS_EX_TOOLWINDOW keeps the window out of the taskbar and out
// of Alt+Tab, should anything ever show it.
//
// # Handing states over
//
// update and remove run on any goroutine, the main thread of the application
// among them, and must not wait for the window's thread, which may sit in an
// open menu for as long as the user looks at it. They put the state into a
// slot guarded by a mutex and post a private message, unless one is already
// on its way; the thread takes whatever the slot holds when the message
// arrives. A burst of updates is therefore applied once, with the latest
// state. An update while the menu is open is applied too, because the modal
// loop of the menu dispatches posted messages: the icon and the tooltip
// change under the open menu, the menu itself keeps what it was built from.
//
// # The menu
//
// The menu is built when it is opened, from the state shown at that moment,
// and destroyed when it closes, together with its bitmaps; DestroyMenu takes
// the submenus with it. Command ids are the ids of the entries, which are 1 to
// at most MaxItems and fit into the 16 bits Windows keeps of them; an entry
// without OnClick, a separator and a submenu have 0, which TrackPopupMenuEx
// also returns when the menu was dismissed. TPM_RETURNCMD makes it return the
// id instead of posting WM_COMMAND, and the click goes to the portable half on
// a goroutine of its own, never from inside the window procedure.
//
// SetForegroundWindow before the menu and the WM_NULL after it are the quirk
// documented under TrackPopupMenu: without the first, the menu does not close
// when the user clicks elsewhere; without the second, it may not open the
// next time. The shell allows the process that owns the icon to take the
// foreground while it handles the click, which is why the first call works.
// The menu opens above the point, as the menus of the taskbar do, and to its
// left where SM_MENUDROPALIGNMENT says menus open to the left; Windows moves
// it where it would leave the screen.
//
// A click in an open menu is reported with the generation the menu was built
// from, unless a later state still carries the same menu: SetIcon and
// SetTooltip pass the menu of the last Set on unchanged (sameMenu), and a
// tooltip that changes once a second should not make the open menu deaf.
// Remove ends an open menu, whose clicks would be dropped anyway.
//
// # Clicks on the icon
//
// With NOTIFYICON_VERSION_4 the low word of lParam is the event and wParam
// holds the anchor point in screen coordinates, x in its low and y in its
// high word, both signed. A left click arrives as NIN_SELECT, a right click
// and the context menu key as WM_CONTEXTMENU, Enter and Space on a focused
// icon as NIN_KEYSELECT; all three open the menu, which is what the package
// promises. While the menu is open the modal loop would deliver a second
// click and open a menu inside the menu, so a click then does nothing.
//
// # Identity
//
// The icon is identified by a GUID derived from the AppID (winTrayGUID in
// winlogic.go), under which Windows remembers whether the user wants it
// always visible. Windows binds such a GUID to the path of the executable
// that first used it, and refuses NIM_ADD for the same GUID from another path
// — a build in another directory, a moved installation. Then the icon is
// added without a GUID, by window and uID, and that is logged: it works, but
// the user's choice is not remembered. An empty AppID has no GUID from the
// start.
//
// Two processes of one application derive the same GUID, and a NIM_DELETE by
// GUID ignores the window: it takes the icon away from whichever process has
// it. So before a tray uses its GUID it claims it with a named mutex in the
// session's namespace, Local\gift.tray.{GUID}. The kernel removes the mutex
// with the last process holding it, also one that crashed, so a mutex that
// exists already means a live process shows the icon under that GUID — a
// second instance, or the same application at another path — and this tray
// goes without the GUID instead of taking the icon from the other one. A tray
// that holds the mutex knows that an icon still registered under its GUID
// belongs to nobody alive, typically a crashed run whose icon Explorer has
// not noticed is orphaned, and may delete it and try once more.
//
// # When the shell says no
//
// Shell_NotifyIconW returns FALSE without documenting why; the last error is
// often left at 0 (the Go runtime clears it before every call). One failure
// is well known: ERROR_TIMEOUT, when Explorer is too busy to answer in time,
// for instance right after logon, when an application started with the
// session adds its icon. The icon may have been added nonetheless, so a
// NIM_MODIFY tells, and otherwise the call is repeated later. No failure
// costs the GUID unless the same icon is accepted without it at once.
//
// A failure while Explorer's taskbar exists is retried with a timer of the
// window, half a second at first and at most every 30 seconds, so that the
// callers never wait and an application that calls Set once at start still
// gets its icon. While there is no taskbar at all — Explorer restarting, a
// session with another shell, Server Core — nothing is retried: the icon is
// added when TaskbarCreated arrives, and that is all the shell offers. A
// failure is logged once, as a warning, until the icon is added again; the
// repetitions in between are logged at debug level, so a tooltip that
// changes every second does not fill the log. A Windows without a taskbar is
// not reported as ErrUnsupported: the taskbar may come later, and the tray
// would then be dead for no reason.
//
// # Pictures and DPI
//
// The icon is a 32-bit icon made with CreateIconIndirect from a DIB with
// straight alpha, which is what icons carry, and a monochrome mask, at the
// size of a small icon for the DPI of the window. Its colour contrasts with
// the taskbar: white, unless SystemUsesLightTheme in the user's Personalize
// key says the taskbar is light, then black. The symbol of an entry is a
// 32-bit top-down DIB section with premultiplied alpha, which is what menus
// blend, in the colour of menu text, since a Win32 popup menu is light unless
// a program opts into the undocumented dark mode; a disabled entry gets the
// colour of grey text, because Windows does not grey a bitmap with alpha
// itself.
//
// The window's thread is made per-monitor aware (V2) by
// SetThreadDpiAwarenessContext, which changes the awareness of this one
// thread and nothing of the process. Ebitengine sets the same awareness for
// the process when it creates its window, which may be after the first Set;
// without this, the tray's window would be DPI unaware or aware depending on
// that order. Aware, the point of a click and the menu are in physical pixels
// and the icon is rendered at the size the taskbar shows it, not scaled up and
// blurred. On a system without the function the thread keeps the awareness of
// the process.

var (
	winUser32                           = windows.NewLazySystemDLL("user32.dll")
	winProcRegisterClassExW             = winUser32.NewProc("RegisterClassExW")
	winProcCreateWindowExW              = winUser32.NewProc("CreateWindowExW")
	winProcDefWindowProcW               = winUser32.NewProc("DefWindowProcW")
	winProcGetMessageW                  = winUser32.NewProc("GetMessageW")
	winProcTranslateMessage             = winUser32.NewProc("TranslateMessage")
	winProcDispatchMessageW             = winUser32.NewProc("DispatchMessageW")
	winProcPostMessageW                 = winUser32.NewProc("PostMessageW")
	winProcRegisterWindowMessageW       = winUser32.NewProc("RegisterWindowMessageW")
	winProcChangeWindowMessageFilterEx  = winUser32.NewProc("ChangeWindowMessageFilterEx")
	winProcFindWindowW                  = winUser32.NewProc("FindWindowW")
	winProcSetForegroundWindow          = winUser32.NewProc("SetForegroundWindow")
	winProcCreatePopupMenu              = winUser32.NewProc("CreatePopupMenu")
	winProcInsertMenuItemW              = winUser32.NewProc("InsertMenuItemW")
	winProcTrackPopupMenuEx             = winUser32.NewProc("TrackPopupMenuEx")
	winProcDestroyMenu                  = winUser32.NewProc("DestroyMenu")
	winProcEndMenu                      = winUser32.NewProc("EndMenu")
	winProcCreateIconIndirect           = winUser32.NewProc("CreateIconIndirect")
	winProcDestroyIcon                  = winUser32.NewProc("DestroyIcon")
	winProcGetSysColor                  = winUser32.NewProc("GetSysColor")
	winProcGetSystemMetrics             = winUser32.NewProc("GetSystemMetrics")
	winProcGetSystemMetricsForDpi       = winUser32.NewProc("GetSystemMetricsForDpi")
	winProcGetDpiForWindow              = winUser32.NewProc("GetDpiForWindow")
	winProcSetThreadDpiAwarenessContext = winUser32.NewProc("SetThreadDpiAwarenessContext")
	winProcSetTimer                     = winUser32.NewProc("SetTimer")
	winProcKillTimer                    = winUser32.NewProc("KillTimer")

	winGdi32                = windows.NewLazySystemDLL("gdi32.dll")
	winProcCreateDIBSection = winGdi32.NewProc("CreateDIBSection")
	winProcCreateBitmap     = winGdi32.NewProc("CreateBitmap")
	winProcDeleteObject     = winGdi32.NewProc("DeleteObject")

	winShell32               = windows.NewLazySystemDLL("shell32.dll")
	winProcShell_NotifyIconW = winShell32.NewProc("Shell_NotifyIconW")

	winKernel32             = windows.NewLazySystemDLL("kernel32.dll")
	winProcGetModuleHandleW = winKernel32.NewProc("GetModuleHandleW")
)

const (
	winNimAdd        = 0
	winNimModify     = 1
	winNimDelete     = 2
	winNimSetVersion = 4

	winNifMessage = 0x01
	winNifIcon    = 0x02
	winNifTip     = 0x04
	winNifGUID    = 0x20
	winNifShowTip = 0x80

	winNotifyIconVersion4 = 4

	winNinSelect    = 0x0400 // WM_USER
	winNinKeySelect = 0x0401 // NIN_SELECT | NINF_KEY

	winWmNull          = 0x0000
	winWmSettingChange = 0x001A
	winWmContextMenu   = 0x007B
	winWmDisplayChange = 0x007E
	winWmTimer         = 0x0113
	winWmDPIChanged    = 0x02E0
	winWmApp           = 0x8000
	winWmTrayIcon      = winWmApp + 1 // uCallbackMessage of the icon
	winWmApply         = winWmApp + 2 // the slot holds something new

	winWsPopup         = 0x80000000
	winWsExToolWindow  = 0x00000080
	winMsgfltAllow     = 1
	winSmMenuDropAlign = 40
	winSmCxSmIcon      = 49
	winColorMenuText   = 7
	winColorGrayText   = 17
	winDefaultDPI      = 96

	winTpmRightButton = 0x0002
	winTpmRightAlign  = 0x0008
	winTpmBottomAlign = 0x0020
	winTpmNoNotify    = 0x0080
	winTpmReturnCmd   = 0x0100

	winMiimState    = 0x0001
	winMiimID       = 0x0002
	winMiimSubmenu  = 0x0004
	winMiimString   = 0x0040
	winMiimBitmap   = 0x0080
	winMiimFType    = 0x0100
	winMftSeparator = 0x0800
	winMfsDisabled  = 0x0003
	winMfsChecked   = 0x0008

	winBiRGB        = 0
	winDibRGBColors = 0

	winErrorClassAlreadyExists = 1410
)

// winRetryTimer is the id of the timer that repeats a failed
// Shell_NotifyIconW; see "When the shell says no" above and winNextRetry.
const winRetryTimer = 1

// winDpiAwarenessContextPerMonitorAwareV2 is
// DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2, the pseudo handle -4.
const winDpiAwarenessContextPerMonitorAwareV2 = ^uintptr(3)

// winNotifyIconData is NOTIFYICONDATAW with every field up to Windows Vista,
// 976 bytes on 64 bit. uVersion is the union with uTimeout.
type winNotifyIconData struct {
	cbSize           uint32
	hWnd             windows.HWND
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            windows.Handle
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         windows.GUID
	hBalloonIcon     windows.Handle
}

// winMenuItemInfo is MENUITEMINFOW, 80 bytes on 64 bit.
type winMenuItemInfo struct {
	cbSize        uint32
	fMask         uint32
	fType         uint32
	fState        uint32
	wID           uint32
	hSubMenu      windows.Handle
	hbmpChecked   windows.Handle
	hbmpUnchecked windows.Handle
	dwItemData    uintptr
	dwTypeData    *uint16
	cch           uint32
	hbmpItem      windows.Handle
}

// winBitmapInfoHeader is BITMAPINFOHEADER, 40 bytes everywhere. The BITMAPINFO
// of a 32-bit BI_RGB bitmap has no colour table, so the header is all of it.
type winBitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

// winIconInfo is ICONINFO, 32 bytes on 64 bit.
type winIconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  windows.Handle
	hbmColor windows.Handle
}

// winWndClassEx is WNDCLASSEXW, 80 bytes on 64 bit.
type winWndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

// winMsg is MSG with the lPrivate of current SDKs, 48 bytes on 64 bit; a
// structure shorter than the one GetMessageW fills would be overrun.
type winMsg struct {
	hwnd     windows.HWND
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       struct{ x, y int32 }
	lPrivate uint32
}

// The window class, its procedure and the windows of every tray. The
// procedure is made once: windows.NewCallback hands out a limited number of
// callbacks that are never freed. The class name and the callback are package
// variables, so that nothing Windows holds a pointer to is ever collected.
var (
	winClassOnce sync.Once
	winClassErr  error
	winClassName = windows.StringToUTF16Ptr("gift.tray")

	winWindowsMu sync.Mutex
	winTrays     = map[windows.HWND]*winTray{}
	winTrayCount = map[string]int{} // AppID → trays opened with it

	// winMsgTaskbarCreated is written once, in winRegisterClass, before any window
	// of the class exists.
	winMsgTaskbarCreated uintptr
)

func winRegisterClass() error {
	winClassOnce.Do(func() {
		r, _, _ := winProcRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("TaskbarCreated"))))
		winMsgTaskbarCreated = r
		hinst, _, _ := winProcGetModuleHandleW.Call(0)
		wc := winWndClassEx{
			lpfnWndProc:   windows.NewCallback(winWndProc),
			hInstance:     windows.Handle(hinst),
			lpszClassName: winClassName,
		}
		wc.cbSize = uint32(unsafe.Sizeof(wc))
		if r, _, err := winProcRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 && !errors.Is(err, windows.Errno(winErrorClassAlreadyExists)) {
			winClassErr = fmt.Errorf("gift/tray: RegisterClassExW: %w", err)
		}
	})
	return winClassErr
}

// winWndProc is the window procedure of every tray window. Every argument is a
// uintptr: lParam is a number for the icon's messages, and an unsafe.Pointer
// holding a number would mislead the garbage collector.
func winWndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	winWindowsMu.Lock()
	t := winTrays[windows.HWND(hwnd)]
	winWindowsMu.Unlock()
	if t != nil {
		switch {
		case message == winWmApply:
			t.apply()
			return 0
		case message == winWmTrayIcon:
			switch lParam & 0xffff {
			case winNinSelect, winNinKeySelect, winWmContextMenu:
				t.openMenu(int32(int16(wParam)), int32(int16(wParam>>16)))
			}
			return 0
		case message == winMsgTaskbarCreated && winMsgTaskbarCreated != 0:
			t.taskbarCreated()
			return 0
		case message == winWmSettingChange:
			if winIsString(lParam, "ImmersiveColorSet") {
				t.refreshIcon()
			}
		case message == winWmDisplayChange, message == winWmDPIChanged:
			t.refreshIcon()
		case message == winWmTimer && wParam == winRetryTimer:
			t.retryNow()
			return 0
		}
	}
	r, _, _ := winProcDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return r
}

// winIsString reports whether p, the lParam of WM_SETTINGCHANGE, points to the
// NUL-terminated UTF-16 string s. It reads no further than s is long, plus
// the NUL: lParam is often NULL, and when it is not, nothing promises how
// long the string behind it is.
func winIsString(p uintptr, s string) bool {
	if p == 0 {
		return false
	}
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(&p))
	want := windows.StringToUTF16(s)
	for i, w := range want {
		if *(*uint16)(unsafe.Add(ptr, 2*i)) != w {
			return false
		}
	}
	return true
}

// winTray is one icon in the notification area.
type winTray struct {
	cfg  openConfig
	hwnd windows.HWND

	// The slot between update and remove and the window's thread; see
	// "Handing states over" above.
	mu     sync.Mutex
	want   state
	shown  bool // want is to be shown; false after remove
	posted bool // a winWmApply is on its way

	// Everything below belongs to the window's thread.
	cur     state // what is shown, while added
	added   bool  // the shell has the icon
	inMenu  bool
	guid    windows.GUID
	useGUID bool
	uid     uint32
	claim   windows.Handle // the mutex that claims guid; held as long as the process lives
	retry   time.Duration  // the delay of the retry timer, 0 while none is set
	failing bool           // a failure was logged as a warning and the icon is not back yet

	hicon   windows.Handle
	stale   windows.Handle // the icon hicon replaced, destroyed after the next NIM_MODIFY
	iconKey []byte         // the pixels of hicon, so that the same icon is not made twice
}

func openPlatform(cfg openConfig) (platform, error) {
	if err := winProcShell_NotifyIconW.Find(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	if err := winRegisterClass(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	t := &winTray{cfg: cfg}
	winWindowsMu.Lock()
	winTrayCount[cfg.appID]++
	n := winTrayCount[cfg.appID]
	winWindowsMu.Unlock()
	t.uid = uint32(n)
	if cfg.appID != "" {
		t.guid = winGUIDOf(winTrayGUID(cfg.appID, n))
		t.useGUID = t.claimGUID()
	}

	errc := make(chan error)
	go t.run(errc)
	if err := <-errc; err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	return t, nil
}

// claimGUID creates the mutex that says this process shows the icon under
// t.guid, and reports whether it may; see "Identity" above. A mutex that
// exists already belongs to a live process. ERROR_ACCESS_DENIED means the
// same: the mutex was created by a process of another integrity level, an
// elevated instance of the application. Any other failure leaves the tray
// without the GUID too, because only the mutex makes deleting an icon by
// GUID safe.
func (t *winTray) claimGUID() bool {
	name, err := windows.UTF16PtrFromString(`Local\gift.tray.` + t.guid.String())
	if err != nil {
		return false
	}
	h, err := windows.CreateMutex(nil, false, name)
	switch {
	case err == nil:
		t.claim = h
		return true
	case errors.Is(err, windows.ERROR_ALREADY_EXISTS), errors.Is(err, windows.ERROR_ACCESS_DENIED):
		if h != 0 {
			windows.CloseHandle(h)
		}
		t.logf(slog.LevelInfo, "another process of this application shows its tray icon under the GUID; "+
			"adding this one without it, so Windows will not remember whether it is always shown",
			"appID", t.cfg.appID, "guid", t.guid.String())
	default:
		t.logf(slog.LevelWarn, "claiming the GUID of the tray icon failed; adding the icon without it",
			"appID", t.cfg.appID, "guid", t.guid.String(), "error", err)
	}
	return false
}

// winGUIDOf turns the bytes of a UUID into a GUID, whose first three fields are
// integers read most significant byte first.
func winGUIDOf(u [16]byte) windows.GUID {
	g := windows.GUID{
		Data1: binary.BigEndian.Uint32(u[0:4]),
		Data2: binary.BigEndian.Uint16(u[4:6]),
		Data3: binary.BigEndian.Uint16(u[6:8]),
	}
	copy(g.Data4[:], u[8:16])
	return g
}

// run is the window's thread: it creates the window, reports the outcome on
// errc and pumps messages for as long as the process lives. The seam has no
// Close, and a tray lives as long as its application.
func (t *winTray) run(errc chan<- error) {
	runtime.LockOSThread()
	if winProcSetThreadDpiAwarenessContext.Find() == nil {
		winProcSetThreadDpiAwarenessContext.Call(winDpiAwarenessContextPerMonitorAwareV2)
	}
	hinst, _, _ := winProcGetModuleHandleW.Call(0)
	r, _, err := winProcCreateWindowExW.Call(winWsExToolWindow, uintptr(unsafe.Pointer(winClassName)),
		uintptr(unsafe.Pointer(winClassName)), winWsPopup, 0, 0, 0, 0, 0, 0, hinst, 0)
	if r == 0 {
		errc <- fmt.Errorf("gift/tray: CreateWindowExW: %w", err)
		return
	}
	t.hwnd = windows.HWND(r)
	winWindowsMu.Lock()
	winTrays[t.hwnd] = t
	winWindowsMu.Unlock()
	// An elevated process does not receive messages from Explorer, which
	// runs at medium integrity, unless it lets them through; TaskbarCreated
	// is the one this window needs from it.
	if winMsgTaskbarCreated != 0 && winProcChangeWindowMessageFilterEx.Find() == nil {
		winProcChangeWindowMessageFilterEx.Call(uintptr(t.hwnd), winMsgTaskbarCreated, winMsgfltAllow, 0)
	}
	errc <- nil

	var m winMsg
	for {
		r, _, _ := winProcGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			// 0 is WM_QUIT, which nobody posts, -1 an invalid argument.
			t.logf(slog.LevelError, "the message loop of the tray ended", "result", int32(r))
			return
		}
		winProcTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		winProcDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (t *winTray) update(s state) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.want, t.shown = s, true
	return t.post()
}

func (t *winTray) remove() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.shown = false
	return t.post()
}

// post wakes the window's thread unless a wake-up is on its way. t.mu is
// held.
func (t *winTray) post() error {
	if t.posted {
		return nil
	}
	if r, _, err := winProcPostMessageW.Call(uintptr(t.hwnd), winWmApply, 0, 0); r == 0 {
		return fmt.Errorf("gift/tray: PostMessageW: %w", err)
	}
	t.posted = true
	return nil
}

// apply shows what the slot holds. It runs on the window's thread.
func (t *winTray) apply() {
	t.mu.Lock()
	s, shown := t.want, t.shown
	t.posted = false
	t.mu.Unlock()

	if !shown {
		if t.inMenu {
			winProcEndMenu.Call()
		}
		t.stopRetrying()
		t.failing = false
		if t.added {
			nid := t.nid()
			if ok, errno := t.notify(winNimDelete, &nid); !ok {
				t.logf(slog.LevelWarn, "Shell_NotifyIconW(NIM_DELETE) failed", winErrnoArgs(errno)...)
			}
			t.added = false
		}
		return
	}
	t.cur = s
	t.render()
	t.show()
}

// show adds the icon or changes it in place. It runs on the window's thread.
func (t *winTray) show() {
	defer t.dropStale()
	nid := t.nid()
	nid.uFlags |= winNifMessage | winNifIcon | winNifTip | winNifShowTip
	nid.uCallbackMessage = winWmTrayIcon
	nid.hIcon = t.hicon
	tip, cut := winCutUTF16(t.cur.tooltip, winTipUnits)
	copy(nid.szTip[:], tip)
	if cut {
		t.logf(slog.LevelDebug, "the tooltip is longer than Windows shows, truncating", "kept", len(tip))
	}
	if t.added {
		ok, errno := t.notify(winNimModify, &nid)
		if ok {
			t.recovered()
			return
		}
		if errno == windows.ERROR_TIMEOUT {
			// Explorer is busy, not without the icon; change it later.
			t.retryLater("NIM_MODIFY", errno)
			return
		}
		// Explorer may have lost the icon without a TaskbarCreated; add it
		// anew.
		t.added = false
	}
	t.add(&nid)
}

// add puts the icon into the notification area; see "Identity" and "When the
// shell says no" above. It runs on the window's thread.
func (t *winTray) add(nid *winNotifyIconData) {
	ok, errno := t.notify(winNimAdd, nid)
	if !ok && errno == windows.ERROR_TIMEOUT {
		// The add may have gone through all the same; a NIM_MODIFY succeeds
		// if it did.
		ok, _ = t.notify(winNimModify, nid)
	}
	if !ok && errno != windows.ERROR_TIMEOUT && t.useGUID && winTaskbarExists() {
		ok = t.reclaimGUID(nid)
	}
	if !ok {
		if winTaskbarExists() {
			t.retryLater("NIM_ADD", errno)
		} else {
			t.stopRetrying()
			t.warnOnce("there is no taskbar; the tray icon will be added when Explorer creates it", errno)
		}
		return
	}
	t.added = true
	t.recovered()
	v := t.nid()
	v.uVersion = winNotifyIconVersion4
	if ok, errno := t.notify(winNimSetVersion, &v); !ok {
		t.logf(slog.LevelWarn, "Shell_NotifyIconW(NIM_SETVERSION 4) failed, clicks on the icon may not open the menu",
			winErrnoArgs(errno)...)
	}
}

// reclaimGUID handles a NIM_ADD under the GUID that failed while the taskbar
// is there. The mutex of claimGUID is this process's, so no live process
// shows an icon under the GUID: one that is still registered is the orphan of
// a crashed run, which is deleted before the add is tried again. If that
// fails too, the same icon without the GUID is tried, and only if Windows
// takes that at once was it the GUID that was refused — bound to this
// application at another path — and the tray goes on without it. Otherwise
// the GUID is kept and the failure is retried like any other.
func (t *winTray) reclaimGUID(nid *winNotifyIconData) bool {
	del := t.nid()
	t.notify(winNimDelete, &del)
	if ok, _ := t.notify(winNimAdd, nid); ok {
		return true
	}
	plain := *nid
	plain.uFlags &^= winNifGUID
	plain.guidItem = windows.GUID{}
	if ok, _ := t.notify(winNimAdd, &plain); !ok {
		return false
	}
	t.logf(slog.LevelWarn, "the GUID of the tray icon is bound to this application at another path; "+
		"added the icon without it, so Windows will not remember whether it is always shown",
		"appID", t.cfg.appID, "guid", t.guid.String())
	t.useGUID = false
	return true
}

// retryLater sets the timer that repeats show, each time twice as late up to
// winRetryMax. It runs on the window's thread.
func (t *winTray) retryLater(what string, errno windows.Errno) {
	t.retry = winNextRetry(t.retry)
	if r, _, err := winProcSetTimer.Call(uintptr(t.hwnd), winRetryTimer, uintptr(t.retry.Milliseconds()), 0); r == 0 {
		t.retry = 0
		t.warnOnce("Shell_NotifyIconW("+what+") failed and SetTimer too; the tray icon is tried again on the next change",
			errno, "setTimer", err)
		return
	}
	t.warnOnce("Shell_NotifyIconW("+what+") failed; trying again", errno, "in", t.retry)
}

// retryNow is the retry timer going off. It runs on the window's thread.
func (t *winTray) retryNow() {
	// SetTimer repeats; this one fires once, and keeps its delay for the
	// next retryLater to double.
	winProcKillTimer.Call(uintptr(t.hwnd), winRetryTimer)
	t.mu.Lock()
	shown := t.shown
	t.mu.Unlock()
	if shown && t.cur.gen != 0 {
		t.show()
	}
}

func (t *winTray) stopRetrying() {
	if t.retry != 0 {
		winProcKillTimer.Call(uintptr(t.hwnd), winRetryTimer)
		t.retry = 0
	}
}

// warnOnce logs a failure to show the icon as a warning the first time and at
// debug level until the icon is back.
func (t *winTray) warnOnce(msg string, errno windows.Errno, args ...any) {
	level := slog.LevelDebug
	if !t.failing {
		level, t.failing = slog.LevelWarn, true
	}
	t.logf(level, msg, append(winErrnoArgs(errno), args...)...)
}

// recovered is called when the shell has taken the icon: it ends the retries
// and says so if a failure was logged.
func (t *winTray) recovered() {
	t.stopRetrying()
	if t.failing {
		t.failing = false
		t.logf(slog.LevelInfo, "the tray icon is shown again")
	}
}

// winErrnoArgs are the log attributes of the last error of Shell_NotifyIconW,
// none when it left it at 0, which says nothing.
func winErrnoArgs(errno windows.Errno) []any {
	if errno == 0 {
		return nil
	}
	return []any{"error", errno}
}

// winTaskbarExists reports whether Explorer's taskbar window is there.
func winTaskbarExists() bool {
	r, _, _ := winProcFindWindowW.Call(uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("Shell_TrayWnd"))), 0)
	return r != 0
}

// nid is the identifying part of the icon's NOTIFYICONDATAW.
func (t *winTray) nid() winNotifyIconData {
	nid := winNotifyIconData{hWnd: t.hwnd, uID: t.uid}
	nid.cbSize = uint32(unsafe.Sizeof(nid))
	if t.useGUID {
		nid.uFlags = winNifGUID
		nid.guidItem = t.guid
	}
	return nid
}

// notify calls Shell_NotifyIconW and reports whether it succeeded, and the
// last error, which it sets only sometimes; see "When the shell says no".
func (t *winTray) notify(message uintptr, nid *winNotifyIconData) (bool, windows.Errno) {
	r, _, err := winProcShell_NotifyIconW.Call(message, uintptr(unsafe.Pointer(nid)))
	if r != 0 {
		return true, 0
	}
	errno, _ := err.(windows.Errno)
	return false, errno
}

// taskbarCreated adds the icon again after Explorer has restarted. It runs on
// the window's thread.
func (t *winTray) taskbarCreated() {
	t.added = false
	t.mu.Lock()
	shown := t.shown
	t.mu.Unlock()
	if !shown || t.cur.gen == 0 {
		return
	}
	t.render()
	t.show()
}

// refreshIcon renders the icon again after the colour of the taskbar or the
// DPI may have changed, and hands it to the shell if it did change. It runs
// on the window's thread.
func (t *winTray) refreshIcon() {
	if t.added && t.render() {
		t.show()
	}
}

// render makes the HICON of the current state at the current size and
// colour, unless the one there shows the same pixels already. It reports
// whether it replaced it; the replaced one is destroyed by show, once the
// shell has the new one.
func (t *winTray) render() bool {
	px := t.metric(winSmCxSmIcon)
	im := tinted(t.cur.icon, px, winTaskbarInk(winLightTaskbar()))
	if im == nil {
		// The symbol has no ink at this size. An empty icon keeps the slot,
		// and with it the menu, reachable.
		t.logf(slog.LevelWarn, "the tray icon draws nothing at this size, showing it empty", "px", px)
		im = image.NewNRGBA(image.Rect(0, 0, px, px))
	}
	if t.hicon != 0 && string(im.Pix) == string(t.iconKey) {
		return false
	}
	h, err := winMakeIcon(im)
	if err != nil {
		t.logf(slog.LevelError, "creating the tray icon failed", "error", err)
		return false
	}
	t.dropStale()
	t.stale, t.hicon, t.iconKey = t.hicon, h, im.Pix
	return true
}

func (t *winTray) dropStale() {
	if t.stale != 0 {
		winProcDestroyIcon.Call(uintptr(t.stale))
		t.stale = 0
	}
}

// metric is GetSystemMetricsForDpi for the window's DPI, or GetSystemMetrics
// on a system without it.
func (t *winTray) metric(index uintptr) int {
	if winProcGetSystemMetricsForDpi.Find() == nil && winProcGetDpiForWindow.Find() == nil {
		dpi, _, _ := winProcGetDpiForWindow.Call(uintptr(t.hwnd))
		if dpi == 0 {
			dpi = winDefaultDPI
		}
		if r, _, _ := winProcGetSystemMetricsForDpi.Call(index, dpi); int32(r) > 0 {
			return int(int32(r))
		}
	}
	if r, _, _ := winProcGetSystemMetrics.Call(index); int32(r) > 0 {
		return int(int32(r))
	}
	return 16
}

// winLightTaskbar reads whether the taskbar is light. The value is missing on
// systems whose theme was never changed and on versions of Windows 10 before
// the light taskbar, where the taskbar is dark.
func winLightTaskbar() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("SystemUsesLightTheme")
	return err == nil && v != 0
}

// winDIB creates a 32-bit top-down DIB section of w by h pixels holding px, in
// the byte order of winBGRA.
func winDIB(w, h int, px []byte) (windows.Handle, error) {
	bi := winBitmapInfoHeader{
		biWidth:       int32(w),
		biHeight:      -int32(h), // negative: the first row is the top one
		biPlanes:      1,
		biBitCount:    32,
		biCompression: winBiRGB,
	}
	bi.biSize = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	r, _, err := winProcCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bi)), winDibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if r == 0 || bits == nil {
		return 0, fmt.Errorf("gift/tray: CreateDIBSection: %w", err)
	}
	// The section's memory belongs to GDI, outside the Go heap.
	copy(unsafe.Slice((*byte)(bits), w*h*4), px)
	return windows.Handle(r), nil
}

// winMakeIcon creates an HICON from im, with straight alpha.
func winMakeIcon(im *image.NRGBA) (windows.Handle, error) {
	w, h := im.Rect.Dx(), im.Rect.Dy()
	color, err := winDIB(w, h, winBGRA(im, false))
	if err != nil {
		return 0, err
	}
	defer winProcDeleteObject.Call(uintptr(color))
	maskBits := winANDMask(im)
	mask, _, err := winProcCreateBitmap.Call(uintptr(w), uintptr(h), 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))
	if mask == 0 {
		return 0, fmt.Errorf("gift/tray: CreateBitmap: %w", err)
	}
	defer winProcDeleteObject.Call(mask)
	// CreateIconIndirect copies both bitmaps, which are deleted above.
	ii := winIconInfo{fIcon: 1, hbmMask: windows.Handle(mask), hbmColor: color}
	r, _, err := winProcCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	if r == 0 {
		return 0, fmt.Errorf("gift/tray: CreateIconIndirect: %w", err)
	}
	return windows.Handle(r), nil
}

// openMenu builds the menu of the state shown, tracks it at x, y and hands a
// click on. It runs on the window's thread and returns when the menu closes.
func (t *winTray) openMenu(x, y int32) {
	if t.inMenu || !t.added || len(t.cur.menu) == 0 {
		return
	}
	built := t.cur
	var bitmaps []windows.Handle
	defer func() {
		for _, b := range bitmaps {
			winProcDeleteObject.Call(uintptr(b))
		}
	}()
	hmenu, err := winBuildMenu(built.menu, t.metric(winSmCxSmIcon), &bitmaps)
	if err != nil {
		t.logf(slog.LevelError, "building the tray menu failed", "error", err)
		return
	}
	defer winProcDestroyMenu.Call(uintptr(hmenu))

	flags := uintptr(winTpmReturnCmd | winTpmRightButton | winTpmNoNotify | winTpmBottomAlign)
	if r, _, _ := winProcGetSystemMetrics.Call(winSmMenuDropAlign); r != 0 {
		flags |= winTpmRightAlign
	}
	winProcSetForegroundWindow.Call(uintptr(t.hwnd))
	t.inMenu = true
	cmd, _, _ := winProcTrackPopupMenuEx.Call(uintptr(hmenu), flags, uintptr(x), uintptr(y), uintptr(t.hwnd), 0)
	t.inMenu = false
	winProcPostMessageW.Call(uintptr(t.hwnd), winWmNull, 0, 0)

	id := uint32(cmd) & 0xffff
	if id == 0 {
		return
	}
	gen := built.gen
	t.mu.Lock()
	if t.shown && sameMenu(t.want.menu, built.menu) {
		gen = t.want.gen
	}
	t.mu.Unlock()
	go t.cfg.clicked(gen, id)
}

// winBuildMenu creates the popup menu of entries with its submenus, and the
// bitmaps of their symbols at px pixels, which it appends to bitmaps for the
// caller to delete after the menu: a menu does not own its bitmaps.
func winBuildMenu(entries []entry, px int, bitmaps *[]windows.Handle) (windows.Handle, error) {
	r, _, err := winProcCreatePopupMenu.Call()
	if r == 0 {
		return 0, fmt.Errorf("gift/tray: CreatePopupMenu: %w", err)
	}
	hmenu := windows.Handle(r)
	text := winColorRef(winSysColor(winColorMenuText))
	grey := winColorRef(winSysColor(winColorGrayText))
	for i, e := range entries {
		mii := winMenuItemInfo{fMask: winMiimFType}
		mii.cbSize = uint32(unsafe.Sizeof(mii))
		if e.separator {
			mii.fType = winMftSeparator
		} else {
			mii.fMask |= winMiimID | winMiimString | winMiimState
			mii.wID = e.id
			mii.dwTypeData, _ = windows.UTF16PtrFromString(winMenuText(e.title))
			if e.disabled {
				mii.fState |= winMfsDisabled
			}
			if e.checked {
				mii.fState |= winMfsChecked
			}
			if len(e.children) > 0 {
				sub, err := winBuildMenu(e.children, px, bitmaps)
				if err != nil {
					winProcDestroyMenu.Call(uintptr(hmenu))
					return 0, err
				}
				mii.fMask |= winMiimSubmenu
				mii.hSubMenu = sub
			}
			ink := text
			if e.disabled {
				ink = grey
			}
			if im := tinted(e.symbol, px, ink); im != nil {
				if b, err := winDIB(px, px, winBGRA(im, true)); err == nil {
					*bitmaps = append(*bitmaps, b)
					mii.fMask |= winMiimBitmap
					mii.hbmpItem = b
				}
			}
		}
		if r, _, err := winProcInsertMenuItemW.Call(uintptr(hmenu), uintptr(i), 1, uintptr(unsafe.Pointer(&mii))); r == 0 {
			if mii.hSubMenu != 0 {
				winProcDestroyMenu.Call(uintptr(mii.hSubMenu))
			}
			winProcDestroyMenu.Call(uintptr(hmenu))
			return 0, fmt.Errorf("gift/tray: InsertMenuItemW: %w", err)
		}
	}
	return hmenu, nil
}

func winSysColor(index uintptr) uint32 {
	r, _, _ := winProcGetSysColor.Call(index)
	return uint32(r)
}

func (t *winTray) logf(level slog.Level, msg string, args ...any) {
	if t.cfg.log != nil {
		t.cfg.log.Log(context.Background(), level, msg, args...)
	}
}
