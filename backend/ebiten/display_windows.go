//go:build windows

package ebiten

import (
	"log/slog"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// newDisplayReader reads the profile Windows associates with the display the
// window is on: the file GetICMProfile names for a device context of that
// display, which is the default profile of Color Management in the control
// panel, the one a calibration installs.
//
// A display with Advanced Color on, HDR or the automatic colour management
// of Windows 11, gets nothing: Windows converts what an application shows
// there itself, from sRGB, and converting before it would be twice.
//
// It is not run on Windows by gift's tests, which run on macOS and Linux; it
// is checked by compiling, and the layouts of the structures by the
// assertions below.
func newDisplayReader(*slog.Logger) displayReader {
	return func() ([]byte, string, error) {
		hwnd := displayWindow()
		if hwnd == 0 {
			return nil, "", nil
		}
		mon, _, _ := procMonitorFromWindow.Call(uintptr(hwnd), monitorDefaultToNearest)
		if mon == 0 {
			return nil, "", nil
		}
		mi := monitorInfoEx{size: uint32(unsafe.Sizeof(monitorInfoEx{}))}
		if ok, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); ok == 0 {
			return nil, "", nil
		}
		device := windows.UTF16ToString(mi.device[:])
		if advancedColor(device) {
			return nil, device + " (Advanced Color)", nil
		}
		hdc, _, _ := procCreateDCW.Call(uintptr(unsafe.Pointer(&mi.device[0])), uintptr(unsafe.Pointer(&mi.device[0])), 0, 0)
		if hdc == 0 {
			return nil, device, nil
		}
		defer procDeleteDC.Call(hdc)
		var buf [windows.MAX_PATH]uint16
		n := uint32(len(buf))
		if ok, _, _ := procGetICMProfileW.Call(hdc, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&buf[0]))); ok == 0 {
			return nil, device, nil
		}
		path := windows.UTF16ToString(buf[:])
		data, err := os.ReadFile(path)
		return data, path, err
	}
}

var (
	displayUser32 = windows.NewLazySystemDLL("user32.dll")
	displayGDI32  = windows.NewLazySystemDLL("gdi32.dll")

	procMonitorFromWindow           = displayUser32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW             = displayUser32.NewProc("GetMonitorInfoW")
	procEnumThreadWindowsDisplay    = displayUser32.NewProc("EnumThreadWindows")
	procGetClassNameWDisplay        = displayUser32.NewProc("GetClassNameW")
	procIsWindowVisible             = displayUser32.NewProc("IsWindowVisible")
	procGetDisplayConfigBufferSizes = displayUser32.NewProc("GetDisplayConfigBufferSizes")
	procQueryDisplayConfig          = displayUser32.NewProc("QueryDisplayConfig")
	procDisplayConfigGetDeviceInfo  = displayUser32.NewProc("DisplayConfigGetDeviceInfo")
	procCreateDCW                   = displayGDI32.NewProc("CreateDCW")
	procDeleteDC                    = displayGDI32.NewProc("DeleteDC")
	procGetICMProfileW              = displayGDI32.NewProc("GetICMProfileW")
	displayWindowCallback           uintptr
	displayWindowCallbackOnce       sync.Once
	displayWindowFound              windows.HWND
)

const (
	monitorDefaultToNearest = 2
	qdcOnlyActivePaths      = 2
	dcGetSourceName         = 1
	dcGetAdvancedColorInfo  = 9
	// The bits of DISPLAYCONFIG_GET_ADVANCED_COLOR_INFO.value: HDR, and the
	// automatic colour management of an SDR display.
	advancedColorEnabled = 1 << 1
	wideColorEnforced    = 1 << 2
)

// displayWindow is the visible window of class GLFW30 on this thread, the
// main thread, where GLFW made it. GLFW's hidden helper window has the same
// class and is not visible.
//
// The callback is made once: Windows keeps every callback a process makes
// until it exits, and there are only so many.
func displayWindow() windows.HWND {
	displayWindowCallbackOnce.Do(func() {
		displayWindowCallback = windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
			var buf [32]uint16
			n, _, _ := procGetClassNameWDisplay.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			if windows.UTF16ToString(buf[:n]) != "GLFW30" {
				return 1
			}
			if v, _, _ := procIsWindowVisible.Call(uintptr(h)); v == 0 {
				return 1
			}
			displayWindowFound = h
			return 0
		})
	})
	displayWindowFound = 0
	procEnumThreadWindowsDisplay.Call(uintptr(windows.GetCurrentThreadId()), displayWindowCallback, 0)
	return displayWindowFound
}

// advancedColor reports whether the display of the GDI device name has
// Advanced Color on. Where that cannot be found out it reports false, and
// the profile is used.
func advancedColor(device string) bool {
	var np, nm uint32
	if r, _, _ := procGetDisplayConfigBufferSizes.Call(qdcOnlyActivePaths, uintptr(unsafe.Pointer(&np)), uintptr(unsafe.Pointer(&nm))); r != 0 || np == 0 {
		return false
	}
	paths := make([]dcPathInfo, np)
	modes := make([]dcModeInfo, max(nm, 1))
	if r, _, _ := procQueryDisplayConfig.Call(qdcOnlyActivePaths, uintptr(unsafe.Pointer(&np)), uintptr(unsafe.Pointer(&paths[0])),
		uintptr(unsafe.Pointer(&nm)), uintptr(unsafe.Pointer(&modes[0])), 0); r != 0 {
		return false
	}
	for _, p := range paths[:np] {
		src := dcSourceName{header: dcHeader{kind: dcGetSourceName, size: uint32(unsafe.Sizeof(dcSourceName{})),
			adapter: p.srcAdapter, id: p.srcID}}
		if r, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&src))); r != 0 ||
			windows.UTF16ToString(src.name[:]) != device {
			continue
		}
		ac := dcAdvancedColor{header: dcHeader{kind: dcGetAdvancedColorInfo, size: uint32(unsafe.Sizeof(dcAdvancedColor{})),
			adapter: p.tgtAdapter, id: p.tgtID}}
		if r, _, _ := procDisplayConfigGetDeviceInfo.Call(uintptr(unsafe.Pointer(&ac))); r != 0 {
			return false
		}
		return ac.value&(advancedColorEnabled|wideColorEnforced) != 0
	}
	return false
}

// monitorInfoEx is MONITORINFOEXW.
type monitorInfoEx struct {
	size    uint32
	monitor [4]int32
	work    [4]int32
	flags   uint32
	device  [32]uint16
}

// dcPathInfo is DISPLAYCONFIG_PATH_INFO.
type dcPathInfo struct {
	srcAdapter windows.LUID
	srcID      uint32
	srcMode    uint32
	srcFlags   uint32
	tgtAdapter windows.LUID
	tgtID      uint32
	tgtMode    uint32
	technology uint32
	rotation   uint32
	scaling    uint32
	refreshNum uint32
	refreshDen uint32
	scanline   uint32
	available  int32
	tgtFlags   uint32
	flags      uint32
}

// dcModeInfo is DISPLAYCONFIG_MODE_INFO, which is only passed through.
type dcModeInfo [8]uint64

// dcHeader is DISPLAYCONFIG_DEVICE_INFO_HEADER.
type dcHeader struct {
	kind    uint32
	size    uint32
	adapter windows.LUID
	id      uint32
}

// dcSourceName is DISPLAYCONFIG_SOURCE_DEVICE_NAME.
type dcSourceName struct {
	header dcHeader
	name   [32]uint16
}

// dcAdvancedColor is DISPLAYCONFIG_GET_ADVANCED_COLOR_INFO.
type dcAdvancedColor struct {
	header   dcHeader
	value    uint32
	encoding uint32
	bits     uint32
}

// The sizes Windows checks the structures against; a wrong one fails to
// compile here rather than to answer there.
var (
	_ [unsafe.Sizeof(monitorInfoEx{}) - 104]byte
	_ [104 - unsafe.Sizeof(monitorInfoEx{})]byte
	_ [unsafe.Sizeof(dcPathInfo{}) - 72]byte
	_ [72 - unsafe.Sizeof(dcPathInfo{})]byte
	_ [unsafe.Sizeof(dcModeInfo{}) - 64]byte
	_ [64 - unsafe.Sizeof(dcModeInfo{})]byte
	_ [unsafe.Sizeof(dcSourceName{}) - 84]byte
	_ [84 - unsafe.Sizeof(dcSourceName{})]byte
	_ [unsafe.Sizeof(dcAdvancedColor{}) - 32]byte
	_ [32 - unsafe.Sizeof(dcAdvancedColor{})]byte
)
