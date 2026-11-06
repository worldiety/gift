---
title: File Dialogs
weight: 20
---

gift has two ways to let the person pick a file. The [file picker](../../components/dialogs/file-picker/) is made of
gift views and runs inside your window, on a kiosk as well as on a desktop. The package
`github.com/worldiety/gift/filedialog` shows the dialog of the operating system instead, the one the person knows
from every other application, on macOS, on Windows, and in a GNOME or KDE session. Both are called the same way, so
you can choose one, or use the native dialog where there is one and the picker where there is not.

```go
dlg := filedialog.New(open, cancel).
	Mode(filedialog.OpenMulti).
	Extensions(".jpg", ".png")

return filedialog.Present(screen, dlg).Presented(ctx.Read(picking))
```

`Present` shows the dialog over `screen` while `Presented` is true, in the way a [sheet](../../components/dialogs/sheet/)
is presented. The dialog opens when `Presented` turns true. Set it back to false yourself, from `onPick` and from
`onCancel`, as you do for a sheet. Setting it to false while the dialog is open closes the dialog without a callback.

## Native, in-app, or both

The two pickers share their modes and callbacks. `filedialog.Mode` is the same type as `filepicker.Mode`, so one value
configures both. With `Root`, the native dialog reports the same paths as the file picker over `os.DirFS` of that
directory, so one `onPick` serves both:

```go
picker := filepicker.New(os.DirFS(root), open, cancel).Mode(mode).Extensions(".jpg", ".png")
dlg := filedialog.New(open, cancel).Mode(mode).Extensions(".jpg", ".png").Root(root)

// in-app
ui.Sheet(screen, picker).Presented(picking).OnDismiss(cancel)

// native
filedialog.Present(screen, dlg).Presented(picking)

// native where possible, else in-app
filedialog.Present(screen, dlg).Fallback(picker).Presented(picking)
```

With a `Fallback`, a system without a native dialog shows the given view in a sheet instead, already on the first
try, and `onCancel` is not called for the missing dialog. Like a sheet, `Present` with a fallback is a screen and fills
the area it is given.

`Dialog.Open(post)` shows a dialog without a view, for example from a menu or a tray entry. Pass `app.Post`; the answer
arrives through it, on the UI goroutine. It returns at once, together with a function that closes the dialog.

## Systems

| System | Dialog |
|--------|--------|
| macOS | `NSOpenPanel` and `NSSavePanel`, as a sheet on the window of the application |
| Windows | the common item dialog, owned by the window of the application |
| GNOME and KDE | the file chooser of xdg-desktop-portal, over the session bus, modal to the window of the application when its X11 window can be found |
| other Linux desktops, kiosks, BSD, Android, iOS, the web | nothing |

All of them are reached without cgo. On Linux the session must name GNOME or KDE in `XDG_CURRENT_DESKTOP` (or in
`XDG_SESSION_DESKTOP` or `DESKTOP_SESSION`), and xdg-desktop-portal must be installed, which both desktops do by
default. Choosing a folder needs version 3 of the portal's file chooser.

## When nothing happens

Where there is no native dialog, nothing is shown. `onPick` is never called, and `onCancel` is, as the answer "no
file", so that your presented state goes back to false and the button that opened the dialog keeps working. The reason
is logged once, at Error, to the first dialog that has a `Logger`. `filedialog.Available()` tells you in advance, without
asking the system, whether a native dialog is attempted at all; it turns false after a lasting failure.

A dialog that cannot be shown only right now gets the same answer for that one request, logged at Warn, and
`Available()` stays true: on macOS before the application runs, and a `Directory` dialog on a portal older than
version 3.

One native dialog is open at a time. Opening another one meanwhile answers it with `onCancel`, and `Open` returns
`filedialog.ErrBusy`.

## Paths

Without `Root`, the dialog reports absolute operating system paths. With `Root(dir)`, it reports paths relative to
`dir`, separated by `/`, and `"."` for `dir` itself in `Directory` mode. The person can still browse anywhere; a pick
outside `dir` is reported as a cancel and logged at Warn. An application in a Flatpak or Snap gets paths under
`/run/user/<uid>/doc` from the portal, which lie outside any root.

## What the systems have in common

The native dialog offers what all systems can do: the four modes, a title, a start folder, one filter made of
extensions, a suggested name for `Save`, and a question before an existing file is replaced. It has no filter function,
no switch for hidden files, no root the person cannot leave, and no "All files" entry. On macOS a filter matches
by file type, so `.jpg` also lets `.jpeg` through. In `Save` mode, Windows appends the first extension to a typed
name, macOS appends the preferred extension of its file type (`.jpg` may become `.jpeg`), and the portal appends
none, so check the extension of the path you get.

## API

{{< api pkg="filedialog" type="Dialog" >}}

{{< api pkg="filedialog" type="PresentView" >}}

See [pkg.go.dev](https://pkg.go.dev/github.com/worldiety/gift/filedialog) for the full documentation of the package.

## Related

- [File Picker](../../components/dialogs/file-picker/): the picker made of gift views.
- [Sheet](../../components/dialogs/sheet/): how `Present` and its fallback are presented.
- [Kiosks](../kiosks/): why a kiosk gets no native dialog.
- [Concurrency](../concurrency/): how an answer from another thread reaches the UI goroutine.
