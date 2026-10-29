---
title: Stack
---

A stack places its children one after another along an axis. `ui.VStack` arranges them from top to bottom,
`ui.HStack` from leading to trailing. Most screens are built from nested stacks: a column of sections, each a row
of controls.

Both functions return the same type, `ui.Stack`. It sets the space between the children, how they line up on the
other axis, and how the free space is shared. Like any container, it can also carry padding, a background, a
border and a corner radius. To layer children on top of each other, use a [ZStack](../zstack/) instead.

{{< gallery-shot >}}

{{< gallery-code >}}

## Usage

### Gap and alignment

`Gap` sets the space between two adjacent children. It never goes before the first or after the last child. A
negative gap overlaps the children by that much.

`Align` takes a `geom.Alignment` and positions the children on the cross axis. A `VStack` reads only its `X`
component, an `HStack` only its `Y` component, so `geom.Leading`, `geom.Center` and `geom.Trailing` work for both.
Without `Align`, children sit at the leading edge of a `VStack` and at the top of an `HStack`:

```go
ui.VStack(
	ui.Text("Wi-Fi"),
	ui.Text("Connected").Foreground(ui.ColorSecondaryLabel),
).Gap(4)
```

A stack does not stretch its children on the cross axis. Each child gets its own size, and `Align` places it in
the width (or height) the stack ended up with. A child that needs a certain extent sets it with `Frame` or
`MinWidth`. A [Box](../box/) is the exception: it has no content of its own, so it fills the cross axis and
collapses to its padding on the main axis unless you give it a `Frame`, a `MinHeight` (or `MinWidth`) or a `Flex`.

### Baselines

In an `HStack`, `AlignBaseline` lines the children up on their first text baseline instead of an edge or the
center. Labels of different font sizes then sit on one line, as the temperature reading in the demo does. A child
that reports no baseline, such as a [Box](../box/), is placed by the alignment value of `Align` instead, so the two
combine when `Align` comes first: `.Align(geom.Center).AlignBaseline()` centers the boxes and lines up the text.
An `Align` after `AlignBaseline` switches baseline alignment off again.

`AlignBaseline` on a `VStack` panics. A baseline positions a child vertically, and in a vertical stack the
stacking already decides that.

### Size and flex

A stack measures its children first and is as large as they are together, plus the gaps and its padding, within
the limits its parent allows. It offers each inflexible child as much of the main axis as it wants, so a child's
size never depends on its siblings.

A child with a flex takes part in a second pass: the space left after the other children and the gaps is shared
between all flexible children in proportion to their flex. Most views have a `Flex` modifier, including `Stack`
itself. [Spacer](../spacer/) is a flexible, invisible gap with a flex of one; it pushes its neighbors apart:

```go
ui.HStack(
	ui.Text("Inbox"),
	ui.Spacer(),
	ui.Button(ui.Text("Compose"), compose),
).Gap(8).Align(geom.Center)
```

A fixed sidebar next to a content area that takes the rest of the width:

```go
ui.HStack(
	sidebar.Frame(220, geom.Unbounded()),
	content.Flex(1),
)
```

Flex only shares space the stack has. When the main axis is unbounded, every flexible child gets zero. That is the
case for the content of a [Scroll View](../scroll-view/) along its scroll axis, and for a stack that sits as an
inflexible child in a parent stack along the same axis: an `HStack` inside another `HStack` is offered unlimited
width. If a `Spacer` seems to do nothing, give the stack a `Frame` or a `Flex` of its own.
`Spacer().MinLength(16)` keeps a gap of at least 16 points even when there is no space left.

### Overflow

When the children need more than the stack is allowed to take, the stack still reports the allowed size. The
children keep their real sizes and positions and are not cut off; they draw past the edge. `Clip(true)` confines
them to the bounds of the stack; the clip is a rectangle, even with a corner radius. Every overflowing node is
counted in `gift.Diagnostics.OverflowNodes`, and a test asserts that there is none with
`Harness.AssertNoOverflow` from `gifttest`.

### Style

Padding goes inside the stack, between its edge and the children. `Background`, `Border`, `CornerRadius` and
`Shadow` turn a stack into a panel, as the toolbar of the demo shows:

```go
ui.VStack(ui.Text("Settings")).
	Padding(16).
	Background(ui.ColorSurface).
	Border(ui.Border{Width: 1, Color: ui.ColorSeparator}).
	CornerRadius(12)
```

For the common case of a raised panel, a [Card](../card/) does this for you. The border is drawn inside the bounds
and does not change the layout.

### Children and keys

`ui.VStack(a, b, c)` is always safe. `ui.VStack(items...)` hands your slice to gift without copying it, so do not
change the slice after that call. When the children come from a loop, give each one a `Key`, so that its state
stays with it when items are inserted, removed or moved:

```go
rows := make([]gift.View, 0, len(names))
for _, n := range names {
	rows = append(rows, ui.Text(n).Key(n))
}
ui.VStack(rows...).Gap(8)
```

A `VStack` and an `HStack` are different view types. Turning one into the other under the same key mounts a new
subtree, and the state below it starts over.

## API

{{< api type="Stack" >}}

{{< api type="SpacerView" >}}

## Related

- [Spacer](../spacer/), the flexible gap
- [ZStack](../zstack/), which layers its children instead of stacking them
- [Box](../box/), [Card](../card/) and [Scroll View](../scroll-view/)
- [List](../../lists/list/) and [Row](../../lists/row/), for rows with a fixed structure
- [Layout](../../../concepts/layout/), how constraints, flex and overflow work in gift
- [Modifiers](../../../concepts/modifiers/), [Views](../../../concepts/views/) and
  [Diagnostics](../../../concepts/diagnostics/)
- The example [kitchen sink](../../../examples/example-kitchensink/), which is built from stacks throughout
