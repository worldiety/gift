---
title: Drop Destination
---

A drop destination takes what the user drags into the window from another application: files and folders from the
Finder, the Explorer, Nautilus or Dolphin, and on macOS and Windows also text and web addresses. It wraps any content
and leaves that content as it is; it adds a highlight while a drag it would take is over it, and a closure that
receives the drop.

{{< gallery-shot >}}

{{< gallery-code >}}

## Usage

### Taking files

`ui.DropDestination(content)` wraps the content, `Accepts` says which kinds it takes, and `OnDrop` receives them:

```go
ui.DropDestination(
	ui.Text("Drop a picture here").Padding(40),
).Accepts(gift.DropFile).OnDrop(func(d ui.DropInfo) bool {
	for _, path := range d.Files() {
		open(path)
	}
	return true
})
```

Paths are absolute. gift opens no file and reads nothing from it; that is up to you, and best done off the UI
goroutine for large files. `DropItem.Dir` says that a path is a folder. `DropInfo.Location` is where the drop
happened, in logical pixels from the top left corner of the destination.

`OnDrop` returns whether it took the drop. Returning false hands it to the destination around this one, as if this
one had not accepted it.

### Kinds

`gift.DropFile` is a file or a folder, `gift.DropText` plain text, `gift.DropURL` a web address. `Accepts` takes a
set of them, such as `gift.DropText | gift.DropURL`; without it a destination takes every kind. `OnDrop` only sees
the items of the kinds it accepts.

A drag from the Finder or the Explorer carries the paths as text as well. gift reports such a drag as files only,
so a file dragged over a text destination goes to the file destination around it. Text that only repeats a dragged
address arrives as the address alone.

### Nested destinations

Where destinations are nested, the innermost one that accepts the drag wins. A text field for notes inside a window
that takes files gets the text, and a file dragged over the same field goes to the window. A destination around the
whole window, outermost, takes a drop anywhere:

```go
ui.DropDestination(screen).Accepts(gift.DropFile).OnDrop(importFiles)
```

A modal [sheet](../../dialogs/sheet/) or [alert](../../dialogs/alert/) takes drops away from what is behind it, and
popovers and menus are never a destination.

### The highlight

While a drag the destination would take is over it, it draws an accent border and a faint accent fill over the
content, with the corners of `CornerRadius`. To show it yourself, switch it off with `Highlight(false)` and use
`OnTargeted`, which is told true when such a drag arrives and false when it leaves, is dropped or given up:

```go
over := ctx.State("over", false)
ui.DropDestination(card.Background(ifOver(ctx.Read(over)))).
	Highlight(false).OnTargeted(over.Set).OnDrop(importFiles)
```

`Disabled(true)` takes the destination away and leaves the content working.

## Platforms

What arrives depends on the platform. `ctx.DropCapabilities()` reports it, so a view can say what to expect:

| | macOS | Windows | Linux (GNOME, KDE) | elsewhere |
|---|---|---|---|---|
| Files and folders | yes | yes | yes | no |
| Text and addresses | yes | yes | no | no |
| Highlight during the drag | yes | yes | no | no |

On Linux gift only learns of a drag when it is dropped, so there is no highlight and `OnTargeted` is never told
anything; the drop arrives where the pointer is. On every other platform a drop destination is an ordinary view. See
[Drag and Drop](../../../concepts/drag-and-drop/) for the details.

## API

{{< api "DropDestinationView" >}}

{{< api type="DropInfo" decl="true" >}}

{{< api pkg="gift" type="DropData" >}}

{{< api pkg="gift" type="DropCapabilities" decl="true" >}}

## Related

- [Drag and Drop](../../../concepts/drag-and-drop/) for the platforms and the events underneath
- [File Picker](../../dialogs/file-picker/) for opening files from inside the window
- [Drop example](../../../examples/example-drop/)
- [Kitchen sink example](../../../examples/example-kitchensink/), page Drag and Drop under System
