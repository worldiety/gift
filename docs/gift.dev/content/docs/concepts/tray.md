---
title: Tray and Menu Bar
weight: 19
---

A tray icon is a small symbol of the application outside its window: in the menu bar of macOS, in the notification
area of the Windows taskbar, in the status area of a Linux panel. A click on it opens a menu. The package
`github.com/worldiety/gift/tray` puts one there without cgo.

```go
t := tray.New(tray.Config{
	AppID:   "com.example.Photos",
	Icon:    outline.CameraPhoto,
	Tooltip: "Photos",
	Post:    app.Post,
	Logger:  logger,
})

_ = t.Set([]tray.Item{
	{Title: "Show window", Symbol: outline.Window, OnClick: showWindow},
	tray.Separator(),
	{Title: "Import", Items: []tray.Item{
		{Title: "From camera", OnClick: importCamera},
		{Title: "From folder", OnClick: importFolder},
	}},
	{Title: "Notifications", Checked: on, OnClick: toggle},
	{Title: "Quit", OnClick: quit},
})
```

Nothing is shown before the first `Set`: an icon without a menu would open nothing. `OnClick` runs on the UI goroutine,
through `Post`. `SetIcon` and `SetTooltip` change the icon and the tooltip in place, `Remove` takes the icon out of the
bar, and a later `Set` puts it back. The [kitchen sink](../../examples/example-kitchensink/) has a page to try it,
under System, and [`cmd/example-tray`](https://github.com/worldiety/gift/tree/main/cmd/example-tray) is a small
application that lives in the menu bar.

## The menu is a value

`Set` replaces the whole menu. There are no handles to single entries: to move a check mark, build the slice again
with the other entry checked and set it, in the same way a view is built again instead of being changed.

```go
func menu() []tray.Item {
	return []tray.Item{
		{Title: "Close to tray", Checked: closeToTray, OnClick: func() {
			closeToTray = !closeToTray
			_ = t.Set(menu())
		}},
	}
}
```

A click that arrives after a `Set` has replaced the menu it was made in is dropped, because the entry it was meant for
may no longer exist. That holds for a `Set` of the same entries as well: every `Set` makes a new menu. `SetIcon` and
`SetTooltip` keep the menu, and a click in a menu that is open while the tooltip changes still counts.

A menu needs at least one entry that is not a separator; `Set` refuses an empty one, since a click on the icon would
open nothing.

## What every system can do

A tray in gift is the part all supported systems have in common:

- one icon, a [symbol](../icons/), and a tooltip;
- a menu that opens on a click on the icon, left or right, on every system alike;
- entries with a title and optionally a symbol, separators, disabled entries, entries with a check mark, and
  submenus, up to `tray.MaxDepth` (4) levels deep;
- a click on an entry, delivered to `OnClick`.

The icon is a mask, not a picture: macOS draws it in the colour of the menu bar, and gift tints it for the taskbar of
Windows and for the panels of Linux. Keyboard shortcuts in the menu, coloured icons, an action of its own for the left
click, a badge on the icon and radio groups differ too much between the systems, or exist on one only, to promise
them. A message beside the icon is a [notification](../notifications/).

## Platforms

| Platform | Status |
|----------|--------|
| macOS | supported, once the application runs |
| Windows | supported, built but not yet tried on a real machine |
| Linux: KDE Plasma, xfce4-panel, waybar, LXQt, Budgie, … | supported, over the session bus |
| Linux: GNOME | with the AppIndicator extension, see below |
| Kiosk without a panel | no status area, nothing is shown |

Where there is nothing to put an icon into, every call returns an error that wraps `tray.ErrUnsupported`. The failure
is logged once and nothing happens, so an application may ignore the error; it only needs it to decide whether closing
the window should hide it or end the program.

### macOS

The icon is an `NSStatusItem` with an `NSMenu`. It needs an `NSApplication`, which Ebitengine creates inside
`backend.Run`, and nothing else: no app bundle, no signature, unlike [notifications](../notifications/). A `Set`
before `Run` is kept and shown as soon as the application runs, but returns `ErrUnsupported` meanwhile; call it in the
first `OnUpdate` or in a handler to get a meaningful error. A command line tool or a test binary has no
`NSApplication` and never shows an icon.

AppKit may only be touched on the main thread, which belongs to Ebitengine. `Set` and the other calls therefore return
at once and the change is applied on the main thread shortly after; several calls in a row are applied once.

To make the application live in the menu bar, keep it running when the window closes:

```go
ebiten.SetWindowClosingHandled(true)
cfg.OnUpdate = func() error {
	if ebiten.IsWindowBeingClosed() {
		ebiten.SetWindowVisible(false) // "Show window" calls SetWindowVisible(true)
	}
	return nil
}
```

The application keeps its Dock icon and its menu bar while its window is hidden.

### Windows

The icon is a notification icon of the taskbar, with a Win32 popup menu, run by a thread of its own so that an open
menu does not stop the frames of the window. Windows remembers whether the user wants an icon always visible or in the
overflow under a GUID, which gift derives from `AppID`; set it, or the choice is forgotten with every start. Windows
binds that GUID to the path of the executable that used it first; a build in another directory then gets an icon
without a GUID, and says so in the log.

The icon is white on a dark taskbar and black on a light one, and changes when the user switches. When Explorer
restarts it forgets every icon; gift adds its icon again when the new taskbar appears, and an application started
before the taskbar at logon gets its icon when the taskbar is there. A tooltip keeps 127 UTF-16 units, the rest is cut.

### Linux

gift shows a StatusNotifierItem with a `com.canonical.dbusmenu` menu on the D-Bus session bus, with the same D-Bus
client of its own that [notifications](../notifications/) use. A panel shows such an item when it runs a
StatusNotifierWatcher: KDE Plasma, xfce4-panel, waybar, LXQt and Budgie do. GNOME does not; it needs the
[AppIndicator extension](https://extensions.gnome.org/extension/615/appindicator-support/), which Ubuntu enables by
default.

A desktop without a watcher is logged once, and the icon appears as soon as a panel starts one. That covers an
application that starts at login before the panel does, and a panel that is restarted. On GNOME without the extension,
and on a kiosk without a panel, that is never, and the warning in the log is all there is. A machine without a session
bus gets `ErrUnsupported`.

The panel cannot be asked for its colour, so the icon is near white with a thin dark outline, which reads on the dark
bars of most desktops and on a light one. Inside a Flatpak, allow the application to own its name
(`--own-name=org.kde.StatusNotifierItem-2-1`, as the Flatpaks of KDE do), or gift falls back to the unique name of its connection, and `Remove`
can then only make the icon passive, which Plasma shows among the hidden icons.

## One tray, for the life of the program

There is no `Close`. A tray holds what its platform needs until the process ends: a thread with a hidden window on
Windows, a connection to the session bus on Linux. `Remove` only takes the icon out of the bar. Create one tray per
icon, once, at start, and change it with `Set`; creating a new one per window or per change of the settings leaks
those resources every time, and on Windows a second tray with the same `AppID` does not get the place the user gave
the first.

## Threads

`Set`, `SetIcon`, `SetTooltip` and `Remove` may be called from any goroutine. They check the menu at once and return
an error for an entry no system can display: an empty title, a line break, a submenu with an `OnClick`, a menu deeper
than `tray.MaxDepth` or longer than `tray.MaxItems`, a menu without an entry. The change itself may be applied after
they return, on the thread the platform needs. A system that is there but not ready keeps the change and catches up by
itself: only macOS before `Run` says so with `ErrUnsupported`; Windows without a taskbar and Linux without a watcher
return nil and log the wait once. `OnClick` always runs on the UI goroutine, through `Config.Post`; with a nil `Post`, clicks are not
delivered.
