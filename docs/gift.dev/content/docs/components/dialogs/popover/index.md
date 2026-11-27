---
title: Popover
---

A popover shows a small view in a card next to the view it belongs to, with an arrow that points at it: a hint,
details, a few settings. It appears below its anchor when it fits there and above otherwise, always inside the
window, and grows out of the arrow with a spring when it opens and shrinks back into it when it closes, as on iOS.

{{< gallery-shot alt="A button labelled Opening hours with a white card below it whose arrow points at the button, showing the opening hours" >}}

{{< gallery-code >}}

## Usage

`ui.Popover(anchor, content)` wraps the anchor and presents the content while `Presented` is true. The application
owns that flag, like the presentation of a [Modal](../modal/):

```go
open := ctx.State("info", false)
ui.Popover(
	ui.Button(ui.Text("Info"), func() { open.Set(true) }),
	ui.Text("Opening hours: 9 to 17"),
).Presented(ctx.Read(open)).OnDismiss(func() { open.Set(false) })
```

A press outside the card or escape calls `OnDismiss`; that press goes nowhere else, so the first tap next to an open
popover only closes it. While the popover is open the keyboard focus is trapped inside it. Android's back button and
back gesture call `OnDismiss` as well; without it they do nothing while the popover is open. See
[Android and iOS](../../../concepts/mobile/#back).

`Edge` prefers a side — `EdgeBelow`, `EdgeAbove`, `EdgeLeading`, `EdgeTrailing`, or `EdgeAuto`, the default — and
the popover still takes the other side when only that one has room. `Arrow(false)` draws a card without an arrow,
`MaxWidth` caps the card (320 by default), `Padding` sets the space around the content (16) and `Background` its
colour ([`ColorSurface`](../../../concepts/theming/)).

## The top layer

A popover is drawn above everything else in the window, however deep in the tree its anchor is: a scroll view that
clips the anchor does not clip the popover, and a sibling painted after the anchor does not cover it. That is
`gift.Element.Overlay`, which any view can use to present something; see
[Custom Views](../../../concepts/custom-views/#presenting-above-everything). A popover costs nothing while it is
closed.

## API

- [`ui.Popover`](https://pkg.go.dev/github.com/worldiety/gift/ui#Popover)
- [`ui.PopoverView`](https://pkg.go.dev/github.com/worldiety/gift/ui#PopoverView)
