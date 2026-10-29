---
title: Alert
---

`ui.Alert` is a small card with a title, a message and a row of choices. Use it when the user has to
answer a question before going on, such as confirming that a file gets deleted or deciding what happens to
unsaved changes.

An alert is an ordinary view and knows nothing about being modal. You present it with
[Modal](../modal/), which dims the screen behind it, blocks its input and keeps the keyboard focus inside the
alert.

{{< gallery-shot >}}

{{< gallery-code >}}

## Usage

**Present it with Modal.** `ui.Modal(screen, alert)` draws the alert centered over `screen`, with a scrim
between the two. The scrim swallows every tap and wheel event, so nothing behind the alert can be reached. Tab
moves only between the alert's buttons. Modal must get a bounded size in both directions, so put it at the root
of your window, as in the demo. See [Modal](../modal/) for the details.

**Nil is the closed state.** Build the alert from your own state and pass `nil` while there is nothing to ask:

```go
confirm := ctx.State("confirm", false)

var alert gift.View
if ctx.Read(confirm) {
	alert = ui.Alert("Delete the file?", "This cannot be undone.",
		ui.AlertCancel("Cancel", func() { confirm.Set(false) }),
		ui.AlertDestructive("Delete", func() {
			deleteFile()
			confirm.Set(false)
		}),
	)
}
return ui.Modal(screen, alert)
```

**Presented animates both ways.** A `nil` modal unmounts the alert, so it slides in when it opens and simply
disappears when it closes. To animate the dismissal too, pass the alert all the time and say whether it shows
with `Presented`, as the demo does:

```go
return ui.Modal(screen, alert).Presented(ctx.Read(open))
```

The alert then stays mounted and hidden while it is closed.

**A button does not close the alert.** Each button calls its function and nothing else. Closing the alert is
your job: set the state that presents it, as every callback in the demo does. A tap on the scrim does nothing,
unless you pass a function to `ModalView.OnDismiss`.

**The roles set the color.** All three constructors take a title and a function. They differ only in the color
of the title:

| Constructor        | Color         | Use for                                              |
|--------------------|---------------|------------------------------------------------------|
| `AlertAction`      | `ColorAccent` | the ordinary choice, such as "Save" or "OK"          |
| `AlertCancel`      | `ColorLabel`  | the choice that backs out, such as "Cancel"          |
| `AlertDestructive` | `ColorDanger` | the choice that destroys something, such as "Delete" |

The buttons are laid out in one row in the order you pass them, and share its width equally. Each one is at
least `ui.ControlHitTarget` tall and wide, and its title is its accessible name. The row never wraps into a
column: keep the titles to a word or two, and use at most three buttons.

**The card has a fixed size.** The card is 320 logical pixels wide, whatever the window size, and as tall as its
content. The title and the message are centered; the message wraps. The colors are semantic: `ColorSurface` for
the card, `ColorLabel` for the title and `ColorSecondaryLabel` for the message, so the alert follows the
[theme](../../../concepts/theming/). An alert has no style of its own. For a different look, hand any other view
to `Modal`.

**Title, message and buttons are optional.** An empty title or message draws nothing and takes no space. An alert
without buttons is a card the user cannot answer, for example while an update runs:

```go
var alert gift.View
if !done {
	alert = ui.Alert("Updating", "The device restarts when the update is installed.")
}
return ui.Modal(screen, alert)
```

## API

{{< api "AlertView" >}}

{{< api "AlertButton" >}}

## Related

- [Modal](../modal/): presents the alert with a scrim and a focus trap
- [Sheet](../sheet/): a panel that slides in from an edge
- [Button](../../basic/button/): the view each choice is made of
- [Card](../../layout/card/): a raised surface on the page
- [Colors](../../styling/colors/): `ColorAccent`, `ColorDanger` and the other semantic colors
- [State](../../../concepts/state/): where the open flag of an alert lives
- [Navigation](../../../concepts/navigation/): how Modal fits next to TabBar and NavigationStack
- [Input and focus](../../../concepts/input-and-focus/): the focus trap of a modal
- [Animation](../../../concepts/animation/): the transition of `Presented`
