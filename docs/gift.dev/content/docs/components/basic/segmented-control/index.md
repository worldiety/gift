---
title: Segmented Control
---

A segmented control is a row of mutually exclusive choices. A sliding indicator marks the chosen one. Use it for a
small, fixed set of alternatives that are all worth showing at once, typically three or four, such as "which of
these views am I looking at".

Every segment is always on screen and all segments share the width equally. The control does not scroll, so it is
the wrong choice for a long list or for choices that change at runtime.

{{< gallery-shot >}}

{{< gallery-code >}}

## Usage

### State flows in and out

`ui.SegmentedControl(selected, labels, onChange)` takes the index of the selected segment and calls `onChange` with
the index the user asks for. The control keeps no selection of its own: store the index in state and pass the
setter as the callback.

```go
tab := ctx.State("tab", 0)
ui.SegmentedControl(ctx.Read(tab), []string{"Day", "Week", "Month"}, tab.Set)
```

`onChange` is not called when the user picks the segment that is already selected. The labels slice is not
retained; the control turns the strings into text views during the build.

- An empty `labels` slice panics.
- A `selected` index outside the range draws no indicator and is not clamped. The control reports nothing until
  the user picks a segment.
- A nil `onChange` makes the control inert. For a control that is unavailable, prefer `Disabled(true)`: it skips
  the control in the focus order and draws it in its disabled colors.

### Input

The user taps a segment. The tap counts where the finger lifts, so a finger that lands on the wrong segment can
slide to the right one before it lifts.

The whole control is one focus stop. Left and up select the previous segment, right and down the next one, and
home and end jump to the ends. The arrows stop at the ends instead of wrapping. The selection changes immediately,
without a confirming key press.

When the selection changes, the indicator slides over `ui.ControlAnimation`, no matter who changed it.

### Sizing

The segments divide the width equally, and each label is centered in its segment. The control asks for at least
`ui.ControlHitTarget` (44 logical pixels) of height. A `Frame` or a parent may make it shorter, but never shorter
than its labels; below that it keeps the larger size and reports an overflow.

When the parent offers a bounded width, the control takes all of it. In a `VStack` that is the full width of the
stack. A stack measures an inflexible child with an unbounded main axis, so in an `HStack` the control takes its
natural width instead: each segment is as wide as the widest label plus its padding. Use `Flex`, `Frame`,
`MinWidth` or `MaxWidth` to choose a width.

```go
ui.HStack(
	ui.Text("Quality"),
	ui.SegmentedControl(ctx.Read(q), []string{"Low", "Medium", "High"}, q.Set).Flex(1),
).Gap(12)
```

### Appearance

`Capsule(true)` rounds the tray and the indicator fully and lifts the indicator with a soft shadow. The default is
a rounded rectangle. `Font` and `FontSize` apply to every label; `FontSize` is the em size in pixels and panics
unless it is positive and finite. The selected label draws in `ui.ColorLabel`, the
others in `ui.ColorSecondaryLabel`, so the control follows the theme.

### Labels and tests

`Label` sets the accessible name of the control as a whole, such as "Period". It changes nothing visual. Each
segment is an ordinary text view that carries its own string as its label, so a test finds a segment by its text.

## API

{{< api "SegmentedControlView" >}}

## Related

- [Toggle](../../basic/toggle/) and [Slider](../../basic/slider/), the other value controls
- [Tab Bar](../../navigation/tab-bar/) to switch between the main screens of an app
- [Row](../../lists/row/), which can hold a segmented control as its accessory
- [State](../../../concepts/state/), [Input and Focus](../../../concepts/input-and-focus/),
  [Animation](../../../concepts/animation/), [Testing](../../../concepts/testing/)
- [Kitchen sink example](../../../examples/example-kitchensink/)
