package filedialog

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"unsafe"
)

// The computations of the Windows half, which run everywhere; see
// winlogic.go.

func TestTheOptionsOfEachMode(t *testing.T) {
	// The options a FileOpenDialog and a FileSaveDialog start with, as
	// GetOptions reports them: FOS_PATHMUSTEXIST | FOS_FILEMUSTEXIST |
	// FOS_NOCHANGEDIR, and FOS_OVERWRITEPROMPT | FOS_NOREADONLYRETURN |
	// FOS_PATHMUSTEXIST | FOS_NOCHANGEDIR.
	const openBase, saveBase = 0x1808, 0x8802
	const always = fosForceFileSystem | fosNoChangeDir | fosPathMustExist
	for _, c := range []struct {
		name      string
		m         Mode
		base      uint32
		set, none uint32
	}{
		{"Open", Open, openBase, always | fosFileMustExist, fosPickFolders | fosAllowMultiSelect | fosOverwritePrompt},
		{"OpenMulti", OpenMulti, openBase, always | fosFileMustExist | fosAllowMultiSelect, fosPickFolders | fosOverwritePrompt},
		{"Directory", Directory, openBase, always | fosPickFolders, fosAllowMultiSelect | fosOverwritePrompt | fosFileMustExist},
		{"Save", Save, saveBase, always | fosOverwritePrompt | 0x8000, fosPickFolders | fosAllowMultiSelect | fosFileMustExist},
		// Bits of another mode in the base are turned off.
		{"Open after all", Open, 0xffffffff, always | fosFileMustExist, fosPickFolders | fosAllowMultiSelect | fosOverwritePrompt},
		{"Save after all", Save, 0xffffffff, always | fosOverwritePrompt, fosPickFolders | fosAllowMultiSelect | fosFileMustExist},
	} {
		got := fosFor(c.m, c.base)
		if got&c.set != c.set {
			t.Errorf("%s: options 0x%x lack 0x%x", c.name, got, c.set&^got)
		}
		if got&c.none != 0 {
			t.Errorf("%s: options 0x%x have 0x%x", c.name, got, got&c.none)
		}
		if keep := c.base &^ (c.none | c.set); got&keep != keep {
			t.Errorf("%s: options 0x%x drop 0x%x of the base", c.name, got, keep&^got)
		}
	}
}

func TestTheFilterOfTheDialog(t *testing.T) {
	for _, c := range []struct {
		name           string
		exts           []string
		label, pattern string
	}{
		{"Images", []string{".jpg", ".png"}, "Images (*.jpg;*.png)", "*.jpg;*.png"},
		{"Archives", []string{".tar.gz"}, "Archives (*.tar.gz)", "*.tar.gz"},
		{"", []string{".txt"}, "Supported files (*.txt)", "*.txt"},
		{"  ", []string{".txt"}, "Supported files (*.txt)", "*.txt"},
		{"A\x00B", []string{".txt"}, "AB (*.txt)", "*.txt"},
		{"Images", nil, "", ""},
		// A semicolon would split the pattern, a NUL end it.
		{"Odd", []string{".a;b", ".c\x00", ".", ".d"}, "Odd (*.d)", "*.d"},
		{"Odd", []string{".a;b"}, "", ""},
	} {
		label, pattern := filterSpec(c.name, c.exts)
		if label != c.label || pattern != c.pattern {
			t.Errorf("filterSpec(%q, %q) = %q, %q, want %q, %q", c.name, c.exts, label, pattern, c.label, c.pattern)
		}
	}
	// What normalizeExts makes of the inputs of the in-app picker's tests.
	if _, pattern := filterSpec("x", normalizeExts([]string{".JPG", "png", " .Txt ", ""})); pattern != "*.jpg;*.png;*.txt" {
		t.Errorf("pattern %q", pattern)
	}
}

func TestTheDefaultExtension(t *testing.T) {
	for _, c := range []struct {
		exts []string
		want string
	}{
		{[]string{".jpg", ".png"}, "jpg"},
		{[]string{".tar.gz"}, "tar.gz"},
		{[]string{".a;b", ".png"}, "png"},
		{nil, ""},
		{[]string{"."}, ""},
	} {
		if got := defaultExtension(c.exts); got != c.want {
			t.Errorf("defaultExtension(%q) = %q, want %q", c.exts, got, c.want)
		}
	}
}

func TestHRESULTs(t *testing.T) {
	// The upper half of a 64-bit register may carry the sign extension of a
	// failure; a variable, so that the constant fits on 386 as well.
	var wide uint64 = 0xFFFFFFFF800704C7
	if unsafe.Sizeof(uintptr(0)) == 8 {
		if got := hresult(uintptr(wide)); got != hrCancelled {
			t.Errorf("hresult of a sign-extended cancel is 0x%x", got)
		}
	}
	if got := hresult(uintptr(uint32(hrCancelled))); got != hrCancelled {
		t.Errorf("hresult of a cancel is 0x%x", got)
	}
	if !hrFailed(hrCancelled) || !hrFailed(rpcEChangedMode) || hrFailed(sOK) || hrFailed(sFalse) {
		t.Error("hrFailed reads the severity bit wrongly")
	}
	if err := hrError("X", sFalse); err != nil {
		t.Errorf("S_FALSE is an error: %v", err)
	}
	if err := hrError("IFileDialog.SetTitle", 0x80070057); err == nil || !strings.Contains(err.Error(), "IFileDialog.SetTitle failed with HRESULT 0x80070057") {
		t.Errorf("error %v", err)
	}
}

func TestCoInitializeExResults(t *testing.T) {
	for _, c := range []struct {
		hr     uint32
		uninit bool
		err    bool
	}{
		{sOK, true, false},
		{sFalse, true, false},
		{rpcEChangedMode, false, true},
		{0x8007000E, false, true}, // E_OUTOFMEMORY
	} {
		uninit, err := coInitResult(c.hr)
		if uninit != c.uninit || (err != nil) != c.err {
			t.Errorf("coInitResult(0x%x) = %v, %v", c.hr, uninit, err)
		}
	}
}

func TestTheAnswerOfShow(t *testing.T) {
	if r := winResult(hrCancelled, nil, nil); !r.cancelled || r.err != nil || r.paths != nil {
		t.Errorf("cancel: %+v", r)
	}
	if r := winResult(0x80004005, nil, nil); r.cancelled || r.err == nil {
		t.Errorf("failure: %+v", r)
	}
	boom := errors.New("boom")
	if r := winResult(sOK, nil, boom); r.cancelled || !errors.Is(r.err, boom) {
		t.Errorf("failure to read: %+v", r)
	}
	if r := winResult(sOK, []string{`C:\a`, `C:\b`}, nil); r.cancelled || r.err != nil || len(r.paths) != 2 {
		t.Errorf("pick: %+v", r)
	}
}

func TestTheOwnerIsGLFWsWindow(t *testing.T) {
	classes := map[int]string{1: winGLFWHelperClass, 2: "Console", 3: winGLFWClass, 4: winDialogClass}
	class := func(h int) string { return classes[h] }
	for _, c := range []struct {
		cands []int
		want  int
	}{
		{[]int{1, 2, 3}, 3},
		{[]int{1, 2}, 2},
		{[]int{1, 4}, 0},
		{nil, 0},
		{[]int{4, 2, 3}, 3},
	} {
		if got := winPickOwner(c.cands, class); got != c.want {
			t.Errorf("owner of %v is %d, want %d", c.cands, got, c.want)
		}
	}
}

// sdkGUID is a GUID as DEFINE_GUID and MIDL_INTERFACE in shobjidl_core.h
// write it, the fields of windows.GUID.
type sdkGUID struct {
	d1     uint32
	d2, d3 uint16
	d4     [8]byte
}

// TestTheGUIDsAreThoseOfTheSDK reads every GUID literal and lays it out in
// memory the way windows.GUID is, little-endian integers then eight bytes,
// against the fields the SDK defines.
func TestTheGUIDsAreThoseOfTheSDK(t *testing.T) {
	for _, c := range []struct {
		name, s string
		want    sdkGUID
	}{
		{"CLSID_FileOpenDialog", clsidFileOpenDialog, sdkGUID{0xDC1C5A9C, 0xE88A, 0x4DDE, [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}},
		{"CLSID_FileSaveDialog", clsidFileSaveDialog, sdkGUID{0xC0B4E2F3, 0xBA21, 0x4773, [8]byte{0x8D, 0xBA, 0x33, 0x5E, 0xC9, 0x46, 0xEB, 0x8B}}},
		{"IID_IFileOpenDialog", iidIFileOpenDialog, sdkGUID{0xD57C7288, 0xD4AD, 0x4768, [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}},
		{"IID_IFileSaveDialog", iidIFileSaveDialog, sdkGUID{0x84BCCD23, 0x5FDE, 0x4CDB, [8]byte{0xAE, 0xA4, 0xAF, 0x64, 0xB8, 0x3D, 0x78, 0xAB}}},
		{"IID_IShellItem", iidIShellItem, sdkGUID{0x43826D1E, 0xE718, 0x42EE, [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}},
	} {
		u, err := guidBytes(c.s)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		// The bytes of the GUID in memory.
		var mem [16]byte
		binary.LittleEndian.PutUint32(mem[0:4], binary.BigEndian.Uint32(u[0:4]))
		binary.LittleEndian.PutUint16(mem[4:6], binary.BigEndian.Uint16(u[4:6]))
		binary.LittleEndian.PutUint16(mem[6:8], binary.BigEndian.Uint16(u[6:8]))
		copy(mem[8:], u[8:])
		var want [16]byte
		binary.LittleEndian.PutUint32(want[0:4], c.want.d1)
		binary.LittleEndian.PutUint16(want[4:6], c.want.d2)
		binary.LittleEndian.PutUint16(want[6:8], c.want.d3)
		copy(want[8:], c.want.d4[:])
		if mem != want {
			t.Errorf("%s lays out as % x, the SDK has % x", c.name, mem, want)
		}
	}
	for _, bad := range []string{"", "DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7", "{DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEFZ}", "{DC1C5A9CE-88A-4DDE-A5A1-60F82A20AEF7}"} {
		if _, err := guidBytes(bad); err == nil {
			t.Errorf("guidBytes(%q) accepted", bad)
		}
	}
}

func TestWinTextDropsNULs(t *testing.T) {
	if got := winText("a\x00b\x00"); got != "ab" {
		t.Errorf("winText %q", got)
	}
}
