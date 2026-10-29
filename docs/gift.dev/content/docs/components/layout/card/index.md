---
title: Card
---

A card is a panel raised above the window background. It has a `ColorSurface` face, rounded corners, a hairline
border in `ColorSeparator` and an optional header. `ui.Card(children...)` stacks its children vertically inside
that face, with padding around them and a gap between them.

Use a card to group related content on a screen: a status message, a block of settings, a list of rows. When you
only need a styled rectangle with your own padding and colors, use a [stack](../../layout/stack/) with
`Background`, `Border` and `CornerRadius` instead.

{{< gallery-shot >}}

{{< gallery-code >}}

## Usage

**Children stack vertically.** A card lays out its children like a `VStack`. The content padding defaults to
`ui.CardPadding` (16) on all edges and the gap between children to 12. `Padding`, `PaddingInsets` and `Gap`
replace these defaults. A card has no other layout or paint modifiers: its face, radius (`ui.CardRadius`, 12)
and border are fixed.

**The header sits outside the content padding.** `Header` draws a single line caption in a smaller font and
`ColorSecondaryLabel` above the content. It keeps its own inset of `ui.CardPadding` on the top, leading and
trailing edges, whatever you pass to `Padding`. This matters for a card that wraps a [list](../../lists/list/): the rows bring their own horizontal
padding, so you set the card's content padding to zero and the pressed highlight of a row reaches both edges of
the card. The header still lines up with the row text.

```go
ui.Card(
	ui.List(
		ui.Row("Brightness").Value("80 %"),
		ui.Row("Night shift").Value("Off"),
	),
).Padding(0).Header("Display")
```

An empty header, the default, draws nothing.

**Sizing.** A card is as tall as its content and, like a stack, as wide as its content. A child that fills the
width it is offered, such as a list or an `HStack` with a `Spacer`, makes the card fill it too. That is why the
status card in the demo ends its row with a `Spacer`. To make a card take a share of the free space of its parent
stack, give it `Flex`. The card passes that height on to its children, with or without a header, so a scroll
view inside it takes the height with a `Flex` of its own:

```go
ui.VStack(
	ui.Card(
		ui.VScroll(lines...).Flex(1),
	).Flex(1),
).Flex(1)
```

With a header, the header and the content are two children of an inner `VStack`. A card with `Flex` makes the
content the flexible child there, so the header keeps its line and the content gets the rest.

**The corners are not a clip.** A card does not clip its children. A child that paints into a corner covers the
rounding. Leave the corners to the card's face: a list inside a card draws no background of its own, so the
face shows through.

**Keys.** `Key` sets the reconciliation key of the card among its siblings, as on any other view. Give cards
keys when cards on a screen come and go or change places, so that each one is matched to its previous self.

## API

{{< api "CardView" >}}

## Related

- [Stack](../../layout/stack/): the vertical stack inside a card, and styled containers of your own
- [Box](../../layout/box/): a styled rectangle without content
- [List](../../lists/list/) and [Row](../../lists/row/): the usual content of a settings card
- [Scroll View](../../layout/scroll-view/): scrolling content inside a flexible card
- [Shadow and Border](../../styling/shadow-and-border/)
- [Colors](../../styling/colors/): `ColorSurface`, `ColorSeparator` and the other roles
- [Layout](../../../concepts/layout/): shrink-wrapping, greedy views and flex
- [Views](../../../concepts/views/): keys and reconciliation
- [Kitchen sink example](../../../examples/example-kitchensink/): cards with headers, lists and pictures
