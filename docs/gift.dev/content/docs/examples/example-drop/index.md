---
title: Drop
weight: 9
---

This example takes what is dragged into its window from other applications, with a
[Drop Destination](../../components/basic/drop-destination/). The whole window takes files and folders and lists
each with its kind, its path and its size; the note in the middle takes text and web addresses. The line at the top
says what this platform can drop.

{{< gallery-shot alt="A window with a heading that asks to drop files anywhere, a highlighted note that takes text and addresses, and below it a list of two dropped files and a dropped address" >}}

## Run it

From a checkout of the [repository](https://github.com/worldiety/gift):

```bash
go run ./cmd/example-drop
```

## What to try

- Drag a few files and a folder from the file manager onto the window.
- Drag a file onto the note: it still goes to the window, because the note takes only text and addresses.
- On macOS or Windows, drag a link from a browser onto the note, and a selection from a text editor. Watch the
  window and the note light up while the drag is over them, and go dark when you drag back out.
- On Linux, the files arrive on the drop, without a highlight before it; see
  [Drag and Drop](../../concepts/drag-and-drop/).

{{< example-code >}}
