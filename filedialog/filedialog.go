package filedialog

import (
	"errors"
	"log/slog"
	"strconv"

	"github.com/worldiety/gift/ui/filepicker"
)

// ErrUnsupported is wrapped by every error of a system that has no native
// file dialog, or whose dialog could not be reached.
var ErrUnsupported = errors.New("gift/filedialog: no native file dialog here")

// ErrBusy is returned by [Dialog.Open] while another native file dialog of
// this process is open.
var ErrBusy = errors.New("gift/filedialog: a native file dialog is already open")

// Mode says what a dialog picks. It is the type of the in-app picker, so one
// value configures both [Dialog.Mode] and [filepicker.PickerView.Mode].
type Mode = filepicker.Mode

// The modes, the same constants as those of package filepicker.
const (
	// Open picks one existing file.
	Open = filepicker.Open
	// OpenMulti picks one or more existing files.
	OpenMulti = filepicker.OpenMulti
	// Directory picks one directory.
	Directory = filepicker.Directory
	// Save picks the path of a file to save to. The system asks before an
	// existing file is chosen; nothing is written.
	Save = filepicker.Save
)

// Dialog describes one native file dialog. Create one with [New], configure
// it with its methods, which return a changed copy, and show it with
// [Present] or [Dialog.Open].
type Dialog struct {
	onPick     func([]string)
	onCancel   func()
	mode       Mode
	title      string
	start      string
	root       string
	exts       []string
	filterName string
	suggested  string
	log        *slog.Logger
	// op is the opener the dialog goes through; nil is the one of the
	// process. Tests set their own.
	op *opener
}

// New returns a dialog that calls onPick with the picked paths and onCancel
// when the person backs out, or when there is no native dialog to show. The
// default mode is [Open].
//
// onPick receives one path in every mode but [OpenMulti], and never an empty
// slice. Both run on the UI goroutine. With a nil onCancel, a cancelled or
// unavailable dialog reports nothing at all.
func New(onPick func(paths []string), onCancel func()) Dialog {
	if onPick == nil {
		panic("gift/filedialog: New with a nil onPick")
	}
	return Dialog{onPick: onPick, onCancel: onCancel}
}

// Mode sets what the dialog picks; see [Mode].
func (d Dialog) Mode(m Mode) Dialog {
	if m > Save {
		panic("gift/filedialog: Mode(" + strconv.Itoa(int(m)) + ") is not one of Open, OpenMulti, Directory and Save")
	}
	d.mode = m
	return d
}

// Title sets the title of the dialog. The default names the mode: "Open",
// "Choose a folder", "Save". macOS shows it as the message above the panel.
func (d Dialog) Title(s string) Dialog { d.title = s; return d }

// Start sets the directory the dialog opens in, an operating system path.
// A path that is not an existing directory is ignored. The default is the
// root set with [Dialog.Root], else the system's choice.
func (d Dialog) Start(dir string) Dialog { d.start = dir; return d }

// Root makes the dialog report fs.FS paths relative to dir, "/"-separated,
// the same paths [filepicker.New] reports for os.DirFS(dir). A pick outside
// dir is reported as a cancel, logged at Warn: the person cannot be kept
// inside it. Without a root, the paths are absolute operating system paths.
func (d Dialog) Root(dir string) Dialog { d.root = dir; return d }

// Extensions limits the files that can be picked to these extensions,
// ignoring case: ".jpg" and "JPG" are the same. They form one filter; there
// is no "All files" entry. Directory mode ignores them.
func (d Dialog) Extensions(exts ...string) Dialog {
	d.exts = normalizeExts(exts)
	return d
}

// FilterName sets the label of the filter made from [Dialog.Extensions],
// where the system shows one. The default is "Supported files".
func (d Dialog) FilterName(s string) Dialog { d.filterName = s; return d }

// SuggestedName fills the name field of [Save] mode.
func (d Dialog) SuggestedName(s string) Dialog { d.suggested = s; return d }

// Logger sets where the dialog reports what went wrong. The default is
// nowhere; gift never logs to slog.Default.
func (d Dialog) Logger(l *slog.Logger) Dialog { d.log = l; return d }

// Open shows the dialog now and returns at once; the answer arrives later,
// through post, on the UI goroutine. post is how a function reaches the UI
// executor: [gift.App.Post], or the Post method of a [gift.Token].
//
// Where there is no native dialog, nothing is shown and onCancel is posted.
// The error wraps [ErrUnsupported] when that is already known; otherwise it
// is nil and the answer comes asynchronously, like any other. With another
// dialog of this process open, onCancel is posted and the error is
// [ErrBusy]. With a nil post nothing is shown, nothing is reported, and the
// error wraps ErrUnsupported.
//
// The returned function closes the dialog if it is still open and drops any
// answer still to come. It is never nil and may be called more than once,
// and before the system has shown the dialog.
func (d Dialog) Open(post func(func())) (cancel func(), err error) {
	return d.open(post, nil)
}

// Available reports whether this process is one where a native dialog is
// attempted: macOS, Windows, or a GNOME or KDE session with a session bus,
// outside a gift kiosk. It does not block and does not ask the system, so a
// true can still end in a dialog that cannot be shown; after such a lasting
// failure it reports false for the rest of the process.
func Available() bool { return std.available() }

func (d Dialog) opener() *opener {
	if d.op != nil {
		return d.op
	}
	return std
}
