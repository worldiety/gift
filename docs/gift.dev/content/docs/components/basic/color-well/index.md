---
title: Color Well
---

`ui.ColorWell` is the control of SwiftUI's `ColorPicker`: the colour in a ring of every hue, 28 points across, that
opens a [Color Picker](../color-picker/) in a [Popover](../../dialogs/popover/) when it is tapped.

{{< gallery-shot alt="A row Accent colour with a green swatch in a rainbow ring, and below it a popover with the grid of the colour picker" >}}

{{< gallery-code >}}

## Usage

```go
tint := ctx.State("tint", ui.RGB(52, 199, 89))
ui.HStack(
	ui.Text("Accent colour").Flex(1),
	ui.ColorWell(ctx.Read(tint), tint.Set).Label("Accent colour"),
)
```

Put a label next to the well, as a form row does, and give it the same `Label` for accessibility. Every change in the
picker reaches `onChange` at once; a tap outside the popover closes it. `SupportsOpacity(false)` keeps the colour
opaque.

A tap, or enter and space when it has the focus, opens the picker. A translucent colour shows over a checkerboard.

## API

{{< api "ColorWellView" >}}

## Related

- [Color Picker](../color-picker/), the panel inside the popover
