// Package filepicker is a file picker made of gift views: it opens one or
// several files, chooses a directory or picks a path to save to, inside the
// application's own window and on a touchscreen as well as with a mouse and a
// keyboard.
//
//	picker := filepicker.New(os.DirFS("/media/usb"), open, cancel).
//	    Mode(filepicker.OpenMulti).
//	    Extensions(".jpg", ".png")
//
//	ui.Sheet(screen, picker).Presented(picking).OnDismiss(cancel)
//
// # Why not the platform's dialog
//
// Ebitengine has none to offer, a fullscreen kiosk often has no desktop that
// could show one, and one that did would cover the application with a window
// gift cannot test. This one is views all the way down, so gifttest drives it
// like any other screen.
//
// Where a desktop exists, package github.com/worldiety/gift/filedialog
// shows the system's own dialog with the same modes and callbacks, and its
// Fallback presents this picker where there is none.
//
// # What it reads
//
// It reads through an [io/fs.FS], so the application decides what the person
// may see — os.DirFS("/media/usb") on a kiosk exposes the stick and nothing
// else — and a test hands it a [testing/fstest.MapFS]. Paths it reports are
// fs.FS paths. It never writes, not even in [Save] mode, which only reports
// where the application should.
//
// # What it costs
//
// A directory is read when it is opened, off the UI goroutine, once: the
// listing is a snapshot. It is shown with [ui.VirtualList], which has nodes for
// the visible rows only, so a directory of a hundred thousand files costs
// about a screenful of rows to scroll; reading and sorting it is the part
// that grows with the count, and it happens off the frame path. The filter
// field narrows the listing in memory, on every keystroke, in time linear in
// the number of entries.
//
// # What it does not do
//
// No thumbnails, no list of places such as drives, no watching a directory
// for changes, and no selection across directories: in [OpenMulti] mode the
// selection belongs to the directory shown and is dropped when another one is
// opened.
package filepicker
