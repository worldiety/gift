//go:build windows

package web

import (
	"fmt"
	"runtime"

	"golang.org/x/sys/windows"
)

const canBrowse = true

// systemBrowse opens u with ShellExecuteW, the way Explorer opens a link, from a
// thread of its own with COM initialised, as the shell asks of its callers.
func systemBrowse(u string) error {
	file, err := windows.UTF16PtrFromString(u)
	if err != nil {
		return fmt.Errorf("gift/web: %w", err)
	}
	verb, _ := windows.UTF16PtrFromString("open")
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE) == nil {
			defer windows.CoUninitialize()
		}
		done <- windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
	}()
	if err := <-done; err != nil {
		return fmt.Errorf("gift/web: ShellExecute: %w", err)
	}
	return nil
}
