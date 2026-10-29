---
title: Modal
---

A modal presents one view on top of another and takes the input of everything underneath. `ui.Modal(content,
modal)` draws `content`, a scrim over it, and `modal` centered on top. Taps outside the modal hit the scrim and
stop there, and the keyboard focus cannot leave the modal.

Use it for anything the user has to deal with before going on: a confirmation, a short form, a custom dialog. For
the common title, message and buttons card, hand it an [alert](../../dialogs/alert/). For a panel that slides up
from the bottom edge, use a [sheet](../../dialogs/sheet/).

{{< gallery-shot >}}

{{< gallery-code >}}

## Usage

**Nil is the closed state.** A nil `modal` costs nothing: no scrim, no focus trap, and the result draws exactly
what `content` draws. Build the modal view or nil from your own state. The presence of the value is the
presentation:

```go
confirm := ctx.State("confirm", false)

var dialog gift.View // nil: closed
if ctx.Read(confirm) {
	dialog = ui.Alert("Delete the file?", "This cannot be undone.",
		ui.AlertCancel("Cancel", func() { confirm.Set(false) }),
		ui.AlertDestructive("Delete", func() { confirm.Set(false) }),
	)
}
return ui.Modal(screen, dialog)
```

**Presented animates both ways.** A nil modal unmounts the dialog, so it moves in when it opens and disappears
with a cut when it closes. Pass the dialog every time and say with `Presented` whether it is on the screen, as
the demo does. The dialog then stays mounted and hidden while it is closed, keeps its state, and moves out as
well as in. The dialog rises from a little below its place and the scrim fades. Without `Presented`, a non-nil
modal is open. The price is that the dialog's subtree stays mounted; for something expensive, pass nil instead.

**The modal is any view.** `Modal` adds the scrim, the focus trap and the drawing order. It does not add a card,
a width or a shadow. The demo builds its own card from a `VStack` with `Frame`, `Background`, `CornerRadius` and
`Shadow`. The modal is centered and shrink-wraps, so give it a fixed width or let its content decide.

**Dismissing it.** `OnDismiss` is called when the user taps the scrim anywhere outside the modal. Without it, the
scrim swallows the tap and nothing happens, which is what a dialog that demands an answer wants. Either way the
tap never reaches the content below. A modal does not react to the escape key; a sheet does. Closing is always
your state change: set the state to closed in `OnDismiss` and in the buttons of the dialog.

**The scrim.** It defaults to `ColorLabel` at a fifth of its strength, so it darkens the light theme and lightens
the dark one. `Scrim` sets another color. `ui.ColorClear` gives an invisible scrim that still blocks input:

```go
ui.Modal(screen, panel).Scrim(ui.ColorClear)
```

**It is a screen.** An open modal fills the area it is given, and it panics when it is measured with an unbounded
axis, because a scrim of zero size would let every tap through. The check runs only while a modal view is passed,
so a layout that works while the dialog is closed can still panic the first time it opens. Put the modal at the
root of the window, or give it `Flex` inside a stack. It does not belong inside a scroll view:

```go
ui.Window(ui.VStack(ui.Modal(nav, dialog).Flex(1)))
```

**The content keeps running.** The content underneath stays mounted, is still drawn and still rebuilds. A modal
is a layer, not a pause. If your application must stop work while a dialog is open, it stops the work itself.

**Nesting decides the order.** A modal covers only the `content` it was given, and the outer container is drawn
on top. `ui.Sheet(ui.Modal(body, alert), panel)` draws the sheet over the alert, which is how the
[kitchen sink](../../../examples/example-kitchensink/) combines the two. To open a dialog over a sheet, put the
`Modal` outside the `Sheet` instead.

## API

{{< api "ModalView" >}}

## Related

- [Alert](../../dialogs/alert/): the title, message and buttons card made for `Modal`
- [Sheet](../../dialogs/sheet/): a modal panel that slides up from the bottom edge
- [Card](../../layout/card/) and [Shadow and Border](../../styling/shadow-and-border/): building a dialog face
- [Window](../../layout/window/): where a modal usually sits
- [Navigation](../../../concepts/navigation/): modals, sheets, tab bars and navigation stacks
- [Input and Focus](../../../concepts/input-and-focus/): the focus trap
- [Animation](../../../concepts/animation/): how `Presented` moves the dialog
- [Kitchen sink example](../../../examples/example-kitchensink/): an alert presented with `Modal`
