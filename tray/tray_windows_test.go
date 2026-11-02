//go:build windows

package tray

import (
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/worldiety/gift/icon/outline"
)

// TestStructuresHaveTheLayoutOfTheSDK pins size and offsets of every
// structure handed to Windows, for 64 bit (amd64, arm64) and 32 bit, as the
// Windows SDK lays them out: cbSize is checked by Windows, a wrong offset is
// not, it just reads the wrong field.
func TestStructuresHaveTheLayoutOfTheSDK(t *testing.T) {
	is64 := unsafe.Sizeof(uintptr(0)) == 8
	pick := func(b64, b32 uintptr) uintptr {
		if is64 {
			return b64
		}
		return b32
	}
	var nid winNotifyIconData
	var mii winMenuItemInfo
	var bih winBitmapInfoHeader
	var ii winIconInfo
	var wc winWndClassEx
	var m winMsg
	for _, c := range []struct {
		name     string
		got      uintptr
		b64, b32 uintptr
	}{
		{"sizeof NOTIFYICONDATAW", unsafe.Sizeof(nid), 976, 956},
		{"NOTIFYICONDATAW.hWnd", unsafe.Offsetof(nid.hWnd), 8, 4},
		{"NOTIFYICONDATAW.uID", unsafe.Offsetof(nid.uID), 16, 8},
		{"NOTIFYICONDATAW.uFlags", unsafe.Offsetof(nid.uFlags), 20, 12},
		{"NOTIFYICONDATAW.uCallbackMessage", unsafe.Offsetof(nid.uCallbackMessage), 24, 16},
		{"NOTIFYICONDATAW.hIcon", unsafe.Offsetof(nid.hIcon), 32, 20},
		{"NOTIFYICONDATAW.szTip", unsafe.Offsetof(nid.szTip), 40, 24},
		{"NOTIFYICONDATAW.dwState", unsafe.Offsetof(nid.dwState), 296, 280},
		{"NOTIFYICONDATAW.szInfo", unsafe.Offsetof(nid.szInfo), 304, 288},
		{"NOTIFYICONDATAW.uVersion", unsafe.Offsetof(nid.uVersion), 816, 800},
		{"NOTIFYICONDATAW.szInfoTitle", unsafe.Offsetof(nid.szInfoTitle), 820, 804},
		{"NOTIFYICONDATAW.dwInfoFlags", unsafe.Offsetof(nid.dwInfoFlags), 948, 932},
		{"NOTIFYICONDATAW.guidItem", unsafe.Offsetof(nid.guidItem), 952, 936},
		{"NOTIFYICONDATAW.hBalloonIcon", unsafe.Offsetof(nid.hBalloonIcon), 968, 952},

		{"sizeof MENUITEMINFOW", unsafe.Sizeof(mii), 80, 48},
		{"MENUITEMINFOW.wID", unsafe.Offsetof(mii.wID), 16, 16},
		{"MENUITEMINFOW.hSubMenu", unsafe.Offsetof(mii.hSubMenu), 24, 20},
		{"MENUITEMINFOW.dwItemData", unsafe.Offsetof(mii.dwItemData), 48, 32},
		{"MENUITEMINFOW.dwTypeData", unsafe.Offsetof(mii.dwTypeData), 56, 36},
		{"MENUITEMINFOW.cch", unsafe.Offsetof(mii.cch), 64, 40},
		{"MENUITEMINFOW.hbmpItem", unsafe.Offsetof(mii.hbmpItem), 72, 44},

		{"sizeof BITMAPINFOHEADER", unsafe.Sizeof(bih), 40, 40},
		{"BITMAPINFOHEADER.biBitCount", unsafe.Offsetof(bih.biBitCount), 14, 14},
		{"BITMAPINFOHEADER.biClrImportant", unsafe.Offsetof(bih.biClrImportant), 36, 36},

		{"sizeof ICONINFO", unsafe.Sizeof(ii), 32, 20},
		{"ICONINFO.hbmMask", unsafe.Offsetof(ii.hbmMask), 16, 12},
		{"ICONINFO.hbmColor", unsafe.Offsetof(ii.hbmColor), 24, 16},

		{"sizeof WNDCLASSEXW", unsafe.Sizeof(wc), 80, 48},
		{"WNDCLASSEXW.lpfnWndProc", unsafe.Offsetof(wc.lpfnWndProc), 8, 8},
		{"WNDCLASSEXW.hInstance", unsafe.Offsetof(wc.hInstance), 24, 20},
		{"WNDCLASSEXW.lpszClassName", unsafe.Offsetof(wc.lpszClassName), 64, 40},
		{"WNDCLASSEXW.hIconSm", unsafe.Offsetof(wc.hIconSm), 72, 44},

		{"sizeof MSG", unsafe.Sizeof(m), 48, 32},
		{"MSG.message", unsafe.Offsetof(m.message), 8, 4},
		{"MSG.wParam", unsafe.Offsetof(m.wParam), 16, 8},
		{"MSG.lParam", unsafe.Offsetof(m.lParam), 24, 12},
		{"MSG.time", unsafe.Offsetof(m.time), 32, 16},
		{"MSG.pt", unsafe.Offsetof(m.pt), 36, 20},
	} {
		if want := pick(c.b64, c.b32); c.got != want {
			t.Errorf("%s is %d, the SDK has %d", c.name, c.got, want)
		}
	}
	if len(nid.szTip) != winTipUnits+1 {
		t.Errorf("szTip holds %d units", len(nid.szTip))
	}
}

func TestTheGUIDOfTheIconIsTheRFCSpelling(t *testing.T) {
	g := winGUIDOf(winTrayGUID("com.example.Photos", 1))
	if got, want := g.String(), "{CB7E7FE5-43D3-5C5D-922E-1F12C7C60DAD}"; got != want {
		t.Fatalf("GUID %s, want %s", got, want)
	}
}

// TestTheGUIDIsClaimedByOneTrayOnly is the case of two instances of one
// application: the second claim of a GUID fails, so the second tray goes
// without it instead of deleting the icon of the first.
func TestTheGUIDIsClaimedByOneTrayOnly(t *testing.T) {
	g, err := windows.GenerateGUID()
	if err != nil {
		t.Fatal(err)
	}
	first, second := &winTray{guid: g}, &winTray{guid: g}
	if !first.claimGUID() {
		t.Fatal("the first claim failed")
	}
	defer windows.CloseHandle(first.claim)
	if second.claimGUID() {
		windows.CloseHandle(second.claim)
		t.Fatal("the second claim of the same GUID succeeded")
	}
}

var (
	winProcGetMenuItemCount = winUser32.NewProc("GetMenuItemCount")
	winProcGetMenuItemID    = winUser32.NewProc("GetMenuItemID")
	winProcGetSubMenu       = winUser32.NewProc("GetSubMenu")
	winProcGetMenuState     = winUser32.NewProc("GetMenuState")
)

// TestTheMenuIsBuiltWithIDsStatesAndSubmenus builds a real menu, which needs
// no desktop, and reads it back.
func TestTheMenuIsBuiltWithIDsStatesAndSubmenus(t *testing.T) {
	menu := []entry{
		{id: 1, title: "Show & tell", symbol: outline.User},
		{separator: true},
		{title: "Import", children: []entry{
			{id: 2, title: "From camera"},
			{id: 3, title: "From folder", disabled: true},
		}},
		{id: 4, title: "Notifications", checked: true},
	}
	var bitmaps []windows.Handle
	h, err := winBuildMenu(menu, 16, &bitmaps)
	if err != nil {
		t.Fatal(err)
	}
	defer winProcDestroyMenu.Call(uintptr(h))
	defer func() {
		for _, b := range bitmaps {
			winProcDeleteObject.Call(uintptr(b))
		}
	}()
	if len(bitmaps) != 1 {
		t.Errorf("%d bitmaps, want 1", len(bitmaps))
	}
	count := func(h uintptr) int { r, _, _ := winProcGetMenuItemCount.Call(h); return int(int32(r)) }
	id := func(h uintptr, i int) int32 { r, _, _ := winProcGetMenuItemID.Call(h, uintptr(i)); return int32(r) }
	st := func(h uintptr, i int) uint32 {
		r, _, _ := winProcGetMenuState.Call(h, uintptr(i), 0x400 /* MF_BYPOSITION */)
		return uint32(r)
	}
	if n := count(uintptr(h)); n != 4 {
		t.Fatalf("%d items, want 4", n)
	}
	if got := id(uintptr(h), 0); got != 1 {
		t.Errorf("first id %d", got)
	}
	if st(uintptr(h), 1)&winMftSeparator == 0 {
		t.Error("the second item is no separator")
	}
	if got := id(uintptr(h), 2); got != -1 {
		t.Errorf("the submenu has id %d, want -1", got)
	}
	sub, _, _ := winProcGetSubMenu.Call(uintptr(h), 2)
	if sub == 0 || count(sub) != 2 || id(sub, 1) != 3 || st(sub, 1)&winMfsDisabled == 0 {
		t.Errorf("submenu %x with %d items, second id %d state %x", sub, count(sub), id(sub, 1), st(sub, 1))
	}
	if st(uintptr(h), 3)&winMfsChecked == 0 {
		t.Error("the last item is not checked")
	}
}

func TestAnIconIsMadeFromASymbol(t *testing.T) {
	im := tinted(outline.User, 32, winTaskbarInk(false))
	if im == nil {
		t.Fatal("the symbol draws nothing")
	}
	h, err := winMakeIcon(im)
	if err != nil {
		t.Fatal(err)
	}
	if r, _, err := winProcDestroyIcon.Call(uintptr(h)); r == 0 {
		t.Fatalf("DestroyIcon: %v", err)
	}
}

// TestWindowsTray puts a real icon into the notification area and is off
// unless GIFT_TRAY_WINDOWS=1:
//
//	set GIFT_TRAY_WINDOWS=1
//	set GIFT_TRAY_CLICK=60s
//	tray.test.exe -test.run WindowsTray -test.v
//
// It shows the icon with a menu, changes icon and tooltip, removes it and
// shows it again. With GIFT_TRAY_CLICK it waits that long for a click on an
// entry of the menu; restarting Explorer meanwhile should bring the icon
// back.
func TestWindowsTray(t *testing.T) {
	if os.Getenv("GIFT_TRAY_WINDOWS") != "1" {
		t.Skip("set GIFT_TRAY_WINDOWS=1 to show a real tray icon")
	}
	clicks := make(chan string, 4)
	click := func(s string) func() { return func() { clicks <- s } }
	tr := New(Config{
		AppID:   "dev.gift.tray-test",
		Icon:    outline.User,
		Tooltip: "gift/tray test",
		Post:    func(fn func()) { fn() },
		Logger:  slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	items := []Item{
		{Title: "Show & tell", Symbol: outline.Window, OnClick: click("show")},
		Separator(),
		{Title: "Import", Items: []Item{
			{Title: "From camera", Symbol: outline.CameraPhoto, OnClick: click("camera")},
			{Title: "From folder", Disabled: true, OnClick: click("folder")},
		}},
		{Title: "Notifications", Checked: true, OnClick: click("notifications")},
		{Title: "Quit", OnClick: click("quit")},
	}
	if err := tr.Set(items); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	if err := tr.SetIcon(outline.CameraPhoto); err != nil {
		t.Fatal(err)
	}
	if err := tr.SetTooltip("a tooltip of more than 127 UTF-16 units, cut there without splitting a pair: " +
		"😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀😀"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	if err := tr.Remove(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	if err := tr.Set(items); err != nil {
		t.Fatal(err)
	}
	if d, err := time.ParseDuration(os.Getenv("GIFT_TRAY_CLICK")); err == nil {
		t.Logf("waiting %v for a click on an entry of the menu (left or right click on the icon)", d)
		select {
		case s := <-clicks:
			t.Log("clicked", s)
		case <-time.After(d):
			t.Fatal("no click arrived")
		}
	}
	if err := tr.Remove(); err != nil {
		t.Fatal(fmt.Errorf("remove: %w", err))
	}
	time.Sleep(500 * time.Millisecond)
}
