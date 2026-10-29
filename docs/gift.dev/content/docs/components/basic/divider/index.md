---
title: Divider
---

A divider is a horizontal hairline that separates groups of content. `ui.Divider()` draws a line one logical
pixel tall in the theme's separator color.

Use it between blocks in a vertical stack, such as the fields of a form or the rows of a hand-built panel. For
a list of rows, use a [List](../../lists/list/) instead: it draws its own separators.

{{< gallery-shot >}}

{{< gallery-code >}}

## Usage

**It fills the width it is offered.** Like a box, a divider takes the full width on a bounded axis and none on
an unbounded one. In a `VStack` it spans the stack. An `HStack` measures its inflexible children with an
unbounded width, so there a divider has no width until you give it a flex:

```go
ui.HStack(
	ui.Divider().Flex(1),
	ui.Text("or"),
	ui.Divider().Flex(1),
).Gap(12).Align(geom.Center)
```

There is no vertical divider.

**Inset aligns it with the text.** `Inset(leading, trailing)` shortens the line at both ends, in logical pixels.
Use the same leading value as the padding of your rows, so the line starts where the text starts. Both values
must be finite and not negative. An inset larger than the width draws nothing.

**Thickness and color.** `Thickness` sets the height in logical pixels and panics unless the value is finite and
positive. `Color` replaces `ui.ColorSeparator`. A semantic color follows the theme, so prefer one of those over
a fixed color.

**Space around it comes from the parent.** A divider has no padding or frame modifiers. Set the spacing with the
`Gap` of the surrounding stack, or with the padding of its neighbors.

**It stays sharp.** The layout height is always the logical thickness, at every density. When painting, gift
moves the line to whole device pixels, so a hairline never blurs across two pixel rows, also inside a scroll
view.

## API

{{< api "DividerView" >}}

## Related

- [List](../../lists/list/): separators between rows, configured with `SeparatorInsets`, `SeparatorColor` and
  `SeparatorThickness`
- [Stack](../../layout/stack/): the parent that decides a divider's width
- [Spacer](../../layout/spacer/): empty flexible space instead of a line
- [Colors](../../styling/colors/): the semantic colors, including `ColorSeparator`
- [Layout](../../../concepts/layout/): how constraints and flex work
- [Theming](../../../concepts/theming/)
