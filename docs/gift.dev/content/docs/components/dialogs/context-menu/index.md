---
title: Context Menu
---

A context menu offers the actions of one thing on the screen — a picture, a row, a file — where the thing is. On a
touch screen a long press opens it: the rest of the window dims, the thing stays bright, and the menu appears below
it, as on iOS. On a desktop the secondary mouse button opens it at the pointer.

{{< gallery-shot alt="A gradient picture, bright, over a dimmed window, with a menu below it: Copy, Share, a disabled Add to album, and a red Delete" >}}

{{< gallery-code >}}

## Usage

`ui.ContextMenu(content, items...)` takes the same items as a [Menu](../menu/):

```go
ui.ContextMenu(
	ui.Image(photo).Frame(200, 150).CornerRadius(12).Clip(true),
	ui.MenuItem("Copy", copyPhoto).Icon(outline.FileCopy),
	ui.MenuItem("Delete", deletePhoto).Icon(outline.TrashBin).Destructive(),
).CornerRadius(12)
```

`CornerRadius` tells the menu the shape of its content, so that the window dims around it exactly.

## The long press

After `gift.LongPressDelay`, half a second, the menu opens while the finger is still down, and the finger can slide
on onto an item and lift there, as in a [Menu](../menu/). The long press takes the finger from whatever content
captured it: a button inside the context menu is told its press was cancelled and does not fire on the release. A
short tap is the content's as before.

On a desktop the secondary mouse button sends a `gift.EventContextMenu` to the node under the pointer, which bubbles
to the context menu around it. The menu then opens at the pointer and nothing dims.

Android's back button and back gesture close an open context menu, as a press outside it does, and go back no
further. See [Android and iOS](../../../concepts/mobile/#back).

## API

- [`ui.ContextMenu`](https://pkg.go.dev/github.com/worldiety/gift/ui#ContextMenu)
- [`gift.EventContextMenu`](https://pkg.go.dev/github.com/worldiety/gift#EventContextMenu)
