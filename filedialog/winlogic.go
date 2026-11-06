package filedialog

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// This file holds the parts of the Windows half that are plain computation:
// the options of the dialog, its filter, its default extension, the
// interface ids and the reading of an HRESULT. It has no build tag so that
// its tests run on every machine, also on the Mac this package is mostly
// written on; nothing outside platform_windows.go calls it, and it does not
// import golang.org/x/sys/windows, which only builds for Windows.

// FILEOPENDIALOGOPTIONS of shobjidl_core.h, the ones this package sets or
// clears.
const (
	fosOverwritePrompt   = 0x2
	fosNoChangeDir       = 0x8
	fosPickFolders       = 0x20
	fosForceFileSystem   = 0x40
	fosAllowMultiSelect  = 0x200
	fosPathMustExist     = 0x800
	fosFileMustExist     = 0x1000
	sigdnFileSysPath     = 0x80058000 // SIGDN_FILESYSPATH
	winCoInitFlags       = 0x2 | 0x4  // COINIT_APARTMENTTHREADED | COINIT_DISABLE_OLE1DDE
	winDialogClass       = "#32770"   // the window class of a dialog box
	winGLFWClass         = "GLFW30"   // the class of a GLFW window, Ebitengine's
	winGLFWHelperClass   = "GLFW3 Helper"
	winDefaultFilterName = "Supported files"
)

// HRESULTs this package tells apart.
const (
	sOK             = 0
	sFalse          = 1          // S_FALSE: COM was initialised on this thread already
	hrCancelled     = 0x800704C7 // HRESULT_FROM_WIN32(ERROR_CANCELLED): the person cancelled
	rpcEChangedMode = 0x80010106 // RPC_E_CHANGED_MODE: the thread is in another apartment
)

// The class and interface ids of the common item dialog, as shobjidl_core.h
// and windows-rs spell them.
const (
	clsidFileOpenDialog = "{DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7}"
	clsidFileSaveDialog = "{C0B4E2F3-BA21-4773-8DBA-335EC946EB8B}"
	iidIFileOpenDialog  = "{D57C7288-D4AD-4768-BE02-9D969532D960}"
	iidIFileSaveDialog  = "{84BCCD23-5FDE-4CDB-AEA4-AF64B83D78AB}"
	iidIShellItem       = "{43826D1E-E718-42EE-BC55-A1E261C37BFE}"
)

// guidBytes reads a braced GUID literal into the 16 bytes of the UUID, most
// significant byte first; guidOf in platform_windows.go turns them into the
// layout of a Windows GUID.
func guidBytes(s string) ([16]byte, error) {
	var u [16]byte
	if len(s) != 38 || s[0] != '{' || s[37] != '}' || s[9] != '-' || s[14] != '-' || s[19] != '-' || s[24] != '-' {
		return u, fmt.Errorf("gift/filedialog: %q is no braced GUID", s)
	}
	b, err := hex.DecodeString(strings.ReplaceAll(s[1:37], "-", ""))
	if err != nil || len(b) != 16 {
		return u, fmt.Errorf("gift/filedialog: %q is no braced GUID", s)
	}
	return [16]byte(b), nil
}

// fosFor is the option set of a dialog in mode m, from the options base the
// dialog starts with (GetOptions). Every mode works on file system paths
// only, since the package reports paths, and leaves the working directory of
// the process alone. The open modes want what exists; Save asks before
// overwriting, and turns off what only an open dialog means.
func fosFor(m Mode, base uint32) uint32 {
	o := base | fosForceFileSystem | fosNoChangeDir | fosPathMustExist
	switch m {
	case Open:
		o = o&^(fosPickFolders|fosAllowMultiSelect|fosOverwritePrompt) | fosFileMustExist
	case OpenMulti:
		o = o&^(fosPickFolders|fosOverwritePrompt) | fosFileMustExist | fosAllowMultiSelect
	case Directory:
		o = o&^(fosAllowMultiSelect|fosOverwritePrompt|fosFileMustExist) | fosPickFolders
	case Save:
		o = o&^(fosPickFolders|fosAllowMultiSelect|fosFileMustExist) | fosOverwritePrompt
	}
	return o
}

// winExts are the extensions of exts that a filter pattern can carry: a
// semicolon separates patterns and a NUL ends the string, so an extension
// with either is left out.
func winExts(exts []string) []string {
	out := make([]string, 0, len(exts))
	for _, e := range exts {
		if e == "" || e == "." || strings.ContainsAny(e, ";\x00") {
			continue
		}
		out = append(out, e)
	}
	return out
}

// filterSpec is the one entry of the dialog's file type list for exts:
// pattern "*.jpg;*.png", and label the name followed by the pattern in
// parentheses, the way the filters of Windows itself read ("Text Documents
// (*.txt)"). Without an extension the pattern is "" and no list is set.
func filterSpec(name string, exts []string) (label, pattern string) {
	es := winExts(exts)
	if len(es) == 0 {
		return "", ""
	}
	ps := make([]string, len(es))
	for i, e := range es {
		ps[i] = "*" + e
	}
	pattern = strings.Join(ps, ";")
	name = strings.TrimSpace(winText(name))
	if name == "" {
		name = winDefaultFilterName
	}
	return name + " (" + pattern + ")", pattern
}

// defaultExtension is what SetDefaultExtension wants: the first extension,
// without its leading dot, which a Save dialog appends to a name typed
// without one. "" for none.
func defaultExtension(exts []string) string {
	es := winExts(exts)
	if len(es) == 0 {
		return ""
	}
	return strings.TrimPrefix(es[0], ".")
}

// winText removes the NULs from s, which would end a UTF-16 string early;
// the request already made it valid UTF-8.
func winText(s string) string { return strings.ReplaceAll(s, "\x00", "") }

// hresult is the HRESULT a COM call returned in r. An HRESULT is 32 bits; on
// 64 bit the upper half of the register is not defined by the ABI and may
// hold the sign extension, so it is cut off, and on 386 this is the identity.
func hresult(r uintptr) uint32 { return uint32(r) }

// hrFailed reports whether hr is a failure, whose severity bit is set.
func hrFailed(hr uint32) bool { return int32(hr) < 0 }

// hrError is nil for a success and otherwise names the call that failed.
func hrError(what string, hr uint32) error {
	if !hrFailed(hr) {
		return nil
	}
	return fmt.Errorf("gift/filedialog: %s failed with HRESULT 0x%08X", what, hr)
}

// coInitResult reads the HRESULT of CoInitializeEx: S_OK and S_FALSE both
// leave COM initialised on the thread, and both must be paired with
// CoUninitialize. RPC_E_CHANGED_MODE means the thread is in the
// multithreaded apartment, where the dialog does not work, and is not to be
// paired; the same for any other failure.
func coInitResult(hr uint32) (uninit bool, err error) {
	switch hr {
	case sOK, sFalse:
		return true, nil
	case rpcEChangedMode:
		return false, fmt.Errorf("gift/filedialog: CoInitializeEx: the thread is in the multithreaded apartment (HRESULT 0x%08X)", hr)
	}
	if err := hrError("CoInitializeEx", hr); err != nil {
		return false, err
	}
	// Another success code: COM is initialised.
	return true, nil
}

// winResult is the answer to a Show that returned hr, with the paths read
// afterwards.
func winResult(hr uint32, paths []string, err error) result {
	switch {
	case hr == hrCancelled:
		return result{cancelled: true}
	case hrFailed(hr):
		return result{err: hrError("IModalWindow.Show", hr)}
	case err != nil:
		return result{err: err}
	}
	return result{paths: paths}
}

// winPickOwner chooses the window that owns the dialog among the candidates,
// the visible top-level windows of this process without an owner, in the
// order Windows enumerated them: the first GLFW window, Ebitengine's; else
// the first that is not GLFW's hidden helper window nor a dialog; else none.
func winPickOwner[H comparable](cands []H, class func(H) string) H {
	var zero, first H
	for _, h := range cands {
		switch class(h) {
		case winGLFWClass:
			return h
		case winGLFWHelperClass, winDialogClass:
			continue
		}
		if first == zero {
			first = h
		}
	}
	return first
}
