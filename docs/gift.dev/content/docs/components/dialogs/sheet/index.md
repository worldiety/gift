---
title: Sheet
---

A sheet presents a view in a panel that slides up from the bottom edge, over the screen it was opened from. While
it is up, a scrim dims the screen underneath and takes all of its input, and the keyboard focus stays inside the
panel. The panel has rounded top corners and the surface color of the theme.

Use a sheet for a task that belongs to the current screen and returns to it: a filter, a picker, a short form.
For a short question with a few answers use an [Alert](../alert/), for a dialog of your own in the middle of the
window a [Modal](../modal/).

{{< gallery-shot >}}

{{< gallery-code >}}

## Usage

### Presenting and dismissing

`ui.Sheet` takes two views: the screen underneath and the view in the panel. The application owns whether the sheet
is up, usually in a `bool` state, and passes it to `Presented`:

```go
open := ctx.State("open", false)

ui.Sheet(screen, picker).
	Presented(ctx.Read(open)).
	OnDismiss(func() { open.Set(false) })
```

Pass the sheet view on every frame and close it with `Presented(false)`, not by passing `nil`. A `nil` sheet
unmounts the panel, so it vanishes instead of sliding out. With `Presented(false)` the panel stays mounted and
hidden: it keeps its state, but it is not drawn, not hit tested and not in the focus order. Without `Presented`
a non-nil sheet is up.

A tap on the scrim and the escape key call `OnDismiss`. Escape reaches the sheet even when nothing in it holds the
focus, which is the usual case for a sheet opened by a tap on a touchscreen. Without `OnDismiss` both do nothing,
and only the application can close the sheet, for example from a button inside it. Use this for a sheet that
demands an answer. There is no drag to dismiss. A [NavigationStack](../../navigation/navigation-stack/) inside the
sheet takes the escape key for itself, so `OnDismiss` does not see it there.

### Placement

A sheet needs bounded space in both axes and panics when it is measured without it. Place it as the outermost view
inside [`ui.Window`](../../layout/window/), so that it covers the whole window:

```go
return ui.Window(
	ui.Sheet(screen, picker).Presented(ctx.Read(open)),
)
```

With [`ui.OnScreenKeyboard`](../keyboard/) among the content of `ui.Window`, the window lays its other content out
above the keyboard while that is showing. A sheet next to it therefore sits on top of the keyboard, and a text
field at the bottom of the sheet stays visible. The sheet gets shorter by the height of the keyboard while it is
up, because its height is a fraction of the space it is given.

```go
return ui.Window(
	ui.Sheet(screen, form).Presented(ctx.Read(open)),
	ui.OnScreenKeyboard(),
).Align(geom.Bottom)
```

### Size and scrim

`Height` sets the height of the panel as a fraction of the height the sheet is given, in (0, 1]. The default is 0.9,
the demo uses 0.5. `MaxWidth` caps the width of the panel in logical pixels. The default is 720; a narrower window
gets a panel as wide as itself, and a wider one gets the panel centered at the bottom. Both panic on values outside
their range.

The panel lays its view out at the top and does not scroll. Wrap tall content in a
[Scroll View](../../layout/scroll-view/).

`Scrim` replaces the wash drawn over the screen underneath. The default is `ui.ColorLabel` at 20 percent opacity,
which darkens the light theme and lightens the dark one. `ui.ColorClear` gives an invisible scrim that still blocks
input.

## API

{{< api "SheetView" >}}

## Related

- [Modal](../modal/) and [Alert](../alert/), the other dialogs
- [File Picker](../file-picker/), which the kitchen sink shows in a sheet
- [Window](../../layout/window/), where a sheet belongs, and [Scroll View](../../layout/scroll-view/) for tall
  content
- [List](../../lists/list/) and [Row](../../lists/row/), the content of the demo
- [State](../../../concepts/state/), [Navigation](../../../concepts/navigation/),
  [Animation](../../../concepts/animation/) and [Input and Focus](../../../concepts/input-and-focus/)
- The example [kitchen sink](../../../examples/example-kitchensink/), which opens a sheet from its Form tab
