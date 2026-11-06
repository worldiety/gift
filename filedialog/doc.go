// Package filedialog shows the operating system's own file dialog — open one
// or several files, choose a directory, pick a path to save to — on macOS,
// on Windows, and in a GNOME or KDE session, and does nothing elsewhere.
//
//	dlg := filedialog.New(open, cancel).
//	    Mode(filedialog.OpenMulti).
//	    Extensions(".jpg", ".png")
//
//	filedialog.Present(screen, dlg).Presented(picking)
//
// It is the native counterpart of [filepicker], the picker made of gift
// views, and is called the same way: the same modes, the same callbacks,
// presented like a [ui.Sheet]. An application chooses one, or both:
//
//	picker := filepicker.New(os.DirFS(root), open, cancel).Extensions(".jpg")
//	dlg := filedialog.New(open, cancel).Extensions(".jpg").Root(root)
//	filedialog.Present(screen, dlg).Fallback(picker).Presented(picking)
//
// shows the system's dialog where there is one, and the in-app picker where
// there is not. With [Dialog.Root], both report the same fs.FS paths, so one
// onPick serves both.
//
// # Where it works
//
//   - macOS: NSOpenPanel and NSSavePanel, as a sheet on the application's
//     window.
//   - Windows: the common item dialog (IFileOpenDialog, IFileSaveDialog),
//     owned by the application's window.
//   - GNOME and KDE: the FileChooser of xdg-desktop-portal, over the session
//     bus. The dialog is modal to the application's window when its X11
//     window can be found, under XWayland too, and a window of its own
//     otherwise.
//
// # When nothing happens
//
// Everywhere else — another Linux desktop, no session bus, a gift kiosk, the
// BSDs, Android, iOS, the web — and where the system's dialog cannot be
// reached, nothing is shown. onPick is never called, onCancel is, as the
// answer "no file", so that an application that closes its presented state
// from onCancel is not left with a button that does nothing; with a
// [PresentView.Fallback], the fallback is shown instead and onCancel is not
// called. The reason is logged once per process, at Error, to the first
// dialog that has a [Dialog.Logger], and [Available] reports false. With a
// nil onCancel, nothing is reported at all.
//
// A dialog that cannot be shown only right now answers just that request the
// same way, logged at Warn, and leaves Available true: on macOS before the
// application runs, which is when NSApplication exists, and a [Directory]
// dialog on a portal whose FileChooser is older than version 3.
//
// # Threading
//
// Both callbacks run on the UI goroutine. Showing a dialog never blocks it:
// [Dialog.Open] returns at once and the answer arrives through the post
// function it was given; [Present] uses the Post of its component's
// [gift.Token]. One native dialog is open at a time per process; another
// [Dialog.Open] meanwhile answers with onCancel and [ErrBusy]. A dialog
// closed from code, by Presented(false) or the cancel function of Open,
// reports nothing. On macOS and Windows the system confirms such a close
// asynchronously, and a dialog opened again before it did gets ErrBusy.
//
// # Paths
//
// Without [Dialog.Root] the paths are absolute operating system paths. With
// it they are fs.FS paths relative to the root, as package filepicker
// reports them for os.DirFS(root); a pick outside the root is a cancel,
// logged at Warn. A sandboxed application (Flatpak, Snap) gets paths under
// /run/user/<uid>/doc from the portal, which are outside any root.
//
// # What it does not do
//
// No custom filter function, no hidden files switch, no root the person
// cannot leave, no more than one filter group and no "All files" entry: the
// common ground of the three systems. Whether the system appends an extension
// to a typed Save name differs: Windows appends the first of
// [Dialog.Extensions]; macOS appends the system's preferred extension of its
// file type, so ".jpg" may become ".jpeg"; the portal appends none. The real dialog cannot be driven
// by package gifttest; the portable parts are tested with a fake system.
package filedialog
