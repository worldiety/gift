//go:build windows

package filedialog

import (
	"log/slog"
	"os"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestTheFilterSpecHasTheLayoutOfTheSDK pins COMDLG_FILTERSPEC of shtypes.h,
// two string pointers: 16 bytes on 64 bit (amd64, arm64), 8 on 32 bit.
func TestTheFilterSpecHasTheLayoutOfTheSDK(t *testing.T) {
	var s winFilterSpec
	want, off := uintptr(16), uintptr(8)
	if unsafe.Sizeof(uintptr(0)) == 4 {
		want, off = 8, 4
	}
	if got := unsafe.Sizeof(s); got != want {
		t.Errorf("sizeof COMDLG_FILTERSPEC is %d, the SDK has %d", got, want)
	}
	if got := unsafe.Offsetof(s.spec); got != off {
		t.Errorf("COMDLG_FILTERSPEC.pszSpec is at %d, the SDK has %d", got, off)
	}
}

func TestTheWindowsGUIDsAreThoseOfTheSDK(t *testing.T) {
	for _, c := range []struct {
		name      string
		got, want windows.GUID
	}{
		{"CLSID_FileOpenDialog", winCLSIDFileOpenDialog, windows.GUID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE, Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}},
		{"CLSID_FileSaveDialog", winCLSIDFileSaveDialog, windows.GUID{Data1: 0xC0B4E2F3, Data2: 0xBA21, Data3: 0x4773, Data4: [8]byte{0x8D, 0xBA, 0x33, 0x5E, 0xC9, 0x46, 0xEB, 0x8B}}},
		{"IID_IFileOpenDialog", winIIDIFileOpenDialog, windows.GUID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768, Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}},
		{"IID_IFileSaveDialog", winIIDIFileSaveDialog, windows.GUID{Data1: 0x84BCCD23, Data2: 0x5FDE, Data3: 0x4CDB, Data4: [8]byte{0xAE, 0xA4, 0xAF, 0x64, 0xB8, 0x3D, 0x78, 0xAB}}},
		{"IID_IShellItem", winIIDIShellItem, windows.GUID{Data1: 0x43826D1E, Data2: 0xE718, Data3: 0x42EE, Data4: [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}},
	} {
		if c.got != c.want {
			t.Errorf("%s is %s, the SDK has %s", c.name, c.got.String(), c.want.String())
		}
	}
}

// TestWindowsDialog shows the real dialog in every mode and is off unless
// GIFT_FILEDIALOG_WINDOWS=1:
//
//	set GIFT_FILEDIALOG_WINDOWS=1
//	go test ./filedialog/ -run Windows -v
//
// Each dialog is closed from code after 500 ms and must answer with a
// cancel. A dialog that stays open, or a test that hangs, is a failure of
// the close; one that never appears is a failure of Show.
func TestWindowsDialog(t *testing.T) {
	if os.Getenv("GIFT_FILEDIALOG_WINDOWS") != "1" {
		t.Skip("set GIFT_FILEDIALOG_WINDOWS=1 to show the real file dialog")
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	p, err := openPlatform(openConfig{log: log, getenv: os.Getenv})
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	for _, m := range []Mode{Open, OpenMulti, Directory, Save} {
		r := request{
			mode:       m,
			title:      "gift/filedialog test: closes by itself",
			accept:     acceptLabel(m),
			start:      home,
			filterName: "Text files",
		}
		if m != Directory {
			r.exts = []string{".txt", ".md"}
		}
		if m == Save {
			r.suggested = "notes.txt"
		}
		got := make(chan result, 1)
		closeFn, err := p.open(r, func(res result) { got <- res })
		if err != nil {
			t.Fatalf("mode %d: %v", m, err)
		}
		time.Sleep(500 * time.Millisecond)
		if final := closeFn(); final {
			t.Errorf("mode %d: the close says it is final", m)
		}
		select {
		case res := <-got:
			if !res.cancelled || res.err != nil {
				t.Errorf("mode %d: answer %+v, want a cancel", m, res)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("mode %d: the dialog did not close within 10 s", m)
		}
		// A second close after the answer does nothing.
		closeFn()
	}
}
