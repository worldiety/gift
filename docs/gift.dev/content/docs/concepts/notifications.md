---
title: Notifications
weight: 18
---

A notification is a message that appears outside the application's window: in the notification center of the
desktop and, if the user allows it, on the lock screen. The package `github.com/worldiety/gift/notify` shows them
without cgo.

```go
n := notify.New(notify.Config{
	AppID:   "com.example.Photos", // the desktop entry, without .desktop
	AppName: "Photos",
	Post:    app.Post,
	Logger:  logger,
})

_ = n.Show(notify.Notification{
	ID:      "import",
	Title:   "Import finished",
	Body:    "1 024 pictures",
	OnClick: func() { selected.Set("import") },
})
```

`Show` with an `ID` that is already shown replaces that notification instead of adding a second one, which is how a
progress message stays one message. `Withdraw` takes it back. `OnClick` runs on the UI goroutine, through `Post`, when
the user clicks the notification while the application is running.
The [kitchen sink](../../examples/example-kitchensink/) has a page to try them, under System.

## What every system can do

A notification in gift is the part all supported systems have in common: a title, a plain text body, an ID to
replace or withdraw it, and a click while the application runs. Buttons, images, sounds, urgency and expiry differ
too much between the systems to promise them. The name and icon come from the registration of the application, not
from the notification. Whether a notification appears on the lock screen is the user's setting on every system.

## Platforms

| Platform | Status |
|----------|--------|
| Linux: GNOME, KDE Plasma, dunst, mako, xfce4-notifyd, … | supported, over the session bus |
| macOS | supported for an app bundle, see below |
| Windows 11 | supported for the current user, see below |
| Kiosk without a desktop | no notification server, nothing is shown |

Where there is nothing to show a notification on, `Show` returns an error that wraps `notify.ErrUnsupported`. The
failure is logged once and nothing happens. Where the user has not allowed notifications, the error wraps
`notify.ErrNotPermitted`. An application can ignore both.

### Linux

gift speaks the [Desktop Notifications Specification](https://specifications.freedesktop.org/notification-spec/latest/)
over the D-Bus session bus, with a small D-Bus client of its own. GNOME and KDE file the notification under the
application whose desktop entry `AppID` names, take its icon from there, and give the user a switch for it in their
settings. Install a desktop entry of that name, `com.example.Photos.desktop`, and set its `StartupWMClass` to the
`X11ClassName` of your `backend.Config`, so that window and notifications belong to the same application. Without an
entry the notification still appears, under `AppName` and with a generic icon.

A click does not raise the window: Ebitengine has no way to take the focus. Call `ebiten.RequestAttention` in
`OnClick` if the application should ask for it.

### macOS

macOS shows notifications only for an application that is an `.app` bundle with a bundle identifier, signed (an ad-hoc
signature is enough) and known to LaunchServices. A binary started with `go run` is none of these; gift notices that
and reports `ErrUnsupported`, because the system would end the process otherwise. During development,
`cmd/giftbundle` runs a program as such a bundle:

```bash
go install github.com/worldiety/gift/cmd/giftbundle@latest
go run -exec "giftbundle -id com.example.Photos" ./cmd/photos
```

It copies the binary into `~/Library/Application Support/gift/bundles/com.example.Photos.app`, signs it ad hoc,
registers it and starts it, with the terminal and the exit status unchanged. The system takes the name and the icon
of the notification from the bundle, not from `AppName`.

macOS asks the user once whether the application may show notifications. Call `Authorize` early, at start or when
the user turns notifications on in your settings: a `Show` before the answer returns `ErrNotPermitted` and is not
shown later.

```go
notifier.Authorize(func(granted bool) {
	notificationsOn.Set(granted)
})
```

A click on the notification brings the application to the front by itself.

### Windows

Windows shows a toast only for an application whose AppUserModelID is registered, and drops every other toast
without an error. gift writes the registration itself before the first toast, for the current user and without
administrator rights, under `HKCU\Software\Classes\AppUserModelId\<AppID>`. Neither an installer, an MSIX package, a
code signature nor a Start menu shortcut is needed. `AppID` is therefore required on Windows, `AppName` is the name
above every toast, and `Icon` may name a PNG or ICO file for it:

```go
notify.New(notify.Config{
	AppID:   "com.example.Photos",
	AppName: "Photos",
	Icon:    `C:\Program Files\Photos\photos.png`,
	Post:    app.Post,
})
```

An uninstaller calls `notify.Unregister` with the same configuration to remove the entry. A click on a toast, on the
banner or later in the notification center, reaches `OnClick` and brings the window to the front while the
application runs. A process running as administrator cannot show toasts at all.

Windows does not tell an application without a package whether the user has turned its toasts off. `Show` then
succeeds and Windows drops the toast, and `Authorize` reports `true`.
