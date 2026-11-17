---
title: Drag and Drop
weight: 20
---

A gift window takes files, and on some platforms text and web addresses, that the user drags in from another
application. The view for it is [Drop Destination](../../components/basic/drop-destination/); this page is about
what happens underneath and what each platform can do. Dragging out of a gift window, and dragging inside one, are
not part of this.

## What each platform can do

| | macOS | Windows | GNOME and KDE | elsewhere |
|---|---|---|---|---|
| Files and folders | yes | yes | yes | – |
| Text and web addresses | yes | yes | – | – |
| Enter, move and leave during the drag | yes | yes | – | – |
| Position of the drop | yes | yes | yes, see below | – |

`ctx.DropCapabilities()`, and the same method on `gift.BuildContext`, report this at run time: `Kinds` is the set of
kinds that can arrive, `Live` that a drag is reported while it moves over the window. The zero value means nothing
arrives. The browser, Android and iOS are in that last column, and so is any platform gift does not name: a drop
destination there is an ordinary view, and gift writes one debug line when the first one is built.

Ebitengine, which gift runs on, reports dropped files on the three desktop systems and nothing else. On macOS and
Windows gift adds what it leaves out itself: on macOS by adding the missing drag methods to Ebitengine's view, on
Windows with an OLE drop target on its window. On Linux it does not, because under the Wayland sessions of GNOME and
KDE the drag reaches only the window's own X11 connection, which belongs to Ebitengine; adding the drag session there
is a change for Ebitengine itself.

A process running as administrator on Windows gets files only, without the highlight: OLE drag and drop does not
cross integrity levels, the older message Ebitengine uses does. `GIFT_DROP_NATIVE=0` in the environment switches the
macOS and Windows additions off, so that only Ebitengine's drop of files is left.

## Linux

Files arrive when they are dropped, at the position of the pointer. Before that gift knows nothing about the drag,
so no destination is highlighted, the hover of the controls under the pointer stays, and `OnTargeted` is never told
anything. Under XWayland the position of the pointer during a drag from a Wayland application is not guaranteed; a
drop at a position outside the window goes to the outermost destination that covers the whole window, and is
discarded when there is none.

A KDE location that is no local file, such as `smb://`, does not arrive, and Ebitengine stops reading the drop at
such an entry: the files after it in the same folder are lost too.

## The session

A drag is a session: it enters the window, moves over it, and ends with a drop or by leaving. gift sends the target
under the drag `EventDragEnter`, `EventDragOver` when the position or what is dragged changes, `EventDragLeave` when
the drag leaves it, and `EventDrop`. A view that is no `ui.DropDestination` takes part by setting
`gift.Element.DropTarget` to an `Interactor` of its own; `EventContext.Drop` says what the drag carries and, for the
drop, delivers it. `Interaction.DropTargeted` is set on the target that accepts the drag.

- The target is the innermost visible, enabled drop target under the position that accepts the kinds; one that
  declines hands the drag to the next one above it.
- An active focus trap, which every modal sets, limits the search to its own subtree. The top layer of popovers and
  menus is never a target.
- Every session a target accepted with `EventDragEnter` ends with `EventDragLeave`: after the drop, when the drag is
  given up, and when the target is hidden or disabled under it. A target that declined the enter hears nothing more,
  and a target above it that takes a drop it declined hears `EventDrop` alone. A target that is unmounted hears
  nothing more.
- A drop target is no target for the pointer: a node with a `DropTarget` and no `Interactor` lets hover and presses
  through to what is under and around it. While a drag session lasts, the hover of the mouse rests.
- The system needs an answer at once, whether the window would take the drag, and gift's tree lives on the UI
  goroutine. The answer it gets is that of the previous update, a frame late.

Only copying is offered: gift never moves or links what is dropped, and opens no file.

## Testing

`gifttest` drags without a window. `h.DragOverAt(p, kinds)` and `node.DragOver(kinds)` drag over a point or a node,
`h.DropAt(p, data)` and `node.DropOnto(data)` drop and report whether a target took it, and `h.DragLeave()` gives up.
`node.AssertDropTargeted()` checks the highlight. `gift.DropFiles(paths...)`, `gift.DropTextData` and
`gift.DropURLData` build what is dropped:

```go
h.Find(gifttest.ByKey("inbox")).DragOver(gift.DropFile).AssertDropTargeted()
if !h.Find(gifttest.ByKey("inbox")).DropOnto(gift.DropFiles("/tmp/a.pdf")) {
	t.Fatal("the inbox did not take the file")
}
```

The [automation interface](../testing/) has the steps `dragOver`, `drop` and `dragLeave` for the same, and `/tree`
reports `dropTarget` and `dropTargeted` for every node.
