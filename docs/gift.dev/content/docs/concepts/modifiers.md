---
title: Modifiers
weight: 2
---

A modifier is a method on a view that changes one of its properties and returns the changed view. You chain them
after the constructor:

```go
ui.VStack(
	ui.Text("Storage"),
	ui.Text("42 GB free").Foreground(ui.ColorSecondaryLabel),
).
	Gap(4).
	Padding(16).
	Background(ui.ColorSurface).
	CornerRadius(12)
```

Every modifier returns the same concrete type it was called on: `Padding` on a `ui.Stack` returns a `ui.Stack`,
`Padding` on a `ui.TextView` returns a `ui.TextView`. That is why you can keep chaining type specific modifiers
such as `Gap` or `Foreground` after a common one.

## Modifiers are fields, not wrappers

In gift a modifier sets a named field on the view value. It does not wrap the view in another view. The chain
above describes one stack with a few fields set, and it produces exactly one node, not five nested ones.

If you know SwiftUI, this is the main difference from it to keep in mind. Three consequences follow from it, and each is
explained below:

- the order of the calls does not matter, because the drawing order is fixed,
- setting the same modifier twice replaces the first value,
- a view only has the modifiers it actually honors.

### The order of the calls does not matter

Each styled node is painted in the same order, whatever order you called the modifiers in:

1. **shadow**,
2. **background**: a color, a [gradient](../../components/styling/gradient/) or [glass](../../components/styling/glass/),
3. **content**: the children, the glyphs of a text, the picture of an image,
4. **border**.

So these two lines describe the same view:

```go
ui.Text("Hello").Padding(16).Background(ui.ColorSurface)
ui.Text("Hello").Background(ui.ColorSurface).Padding(16)
```

Both fill the padding with the background, because the background always fills the bounds of the node and the
padding is inside those bounds. There is no margin modifier and no way to put padding "outside" a background by
calling it later. When you need space around a styled view, give the space to its parent instead, with the
parent's `Padding` or `Gap`, or wrap the view in a stack of its own:

```go
ui.VStack(
	ui.Text("Hello").Padding(12).Background(ui.ColorSurface).CornerRadius(8),
).Padding(16)
```

The same holds for the size modifiers. `Frame`, `MinWidth`, `MaxWidth`, `MinHeight` and `MaxHeight` are applied in
a fixed order too; see [Size](#size) below.

### Setting a modifier twice replaces it

A second call of the same modifier overwrites the first. `Padding(8).Padding(16)` means sixteen points of padding.
Modifiers that write the same field replace each other as well:

- `Padding` and `PaddingInsets` both set the padding, so the last one wins.
- `Background` holds one of a color, a gradient or a glass material. The last call decides which.

Nothing accumulates. To get two borders or two shadows, use two nodes, for example a [ZStack](../../components/layout/zstack/)
with a plate behind the content.

### A view only has the modifiers it honors

There is no shared modifier interface in gift. Each view type declares its own methods, and a view does not offer a
modifier it would ignore. A [Slider](../../components/basic/slider/) has no `Background`, a [Spacer](../../components/layout/spacer/)
has no `Padding`. If a modifier is missing, the compiler tells you, instead of the view silently doing nothing.

A few views accept a modifier only in one form. A [ScrollView](../../components/layout/scroll-view/),
[VirtualList](../../components/lists/virtual-list/), [ImageGallery](../../components/lists/image-gallery/),
[TextField](../../components/basic/text-field/) and the [on-screen keyboard](../../components/dialogs/keyboard/)
always clip, so they accept `Clip(true)` and panic on `Clip(false)`.

There is one documented exception to the rule: a [ZStack](../../components/layout/zstack/) ignores the `Flex` of its
children. A ZStack has no main axis to divide. Its children are measured with the full bounds of the box instead,
so an unframed [Box](../../components/layout/box/) fills it without any `Flex`. The `Flex` of the ZStack itself
works as usual in its parent stack.

## Views are values

A view is an ordinary Go value, and a modifier works on a copy. It does not change the view you call it on; it
returns a new one. You have to use the result:

```go
row := ui.HStack(ui.Text("Wi-Fi"), ui.Spacer(), ui.Text("On")).
	Padding(12).
	CornerRadius(8)
if selected {
	row = row.Background(ui.ColorAccent)
}
return row
```

{{< callout type="warning" >}}
Writing `row.Background(ui.ColorAccent)` without `row =` compiles and does nothing. Go does not complain about
an unused return value here, and neither does gift.
{{< /callout >}}

Because views are values, building a view variable once and using it in several places is safe. Each use gets
its own copy, and each copy turns into its own node. A semantic color such as `ui.ColorSurface` is resolved when
the view is built, not when you call `Background`, so a view stored in a package variable still follows a theme
switch. See [Theming](../theming/).

## The common modifiers

Most views draw a styled rectangle: [Box](../../components/layout/box/), [Stack](../../components/layout/stack/),
[ZStack](../../components/layout/zstack/) and [Window](../../components/layout/window/),
[Text](../../components/basic/text/), [Button](../../components/basic/button/),
[Icon](../../components/basic/icon/), [Image](../../components/basic/image/),
[TextField](../../components/basic/text-field/), [ScrollView](../../components/layout/scroll-view/),
[List](../../components/lists/list/), [VirtualList](../../components/lists/virtual-list/),
[ImageGallery](../../components/lists/image-gallery/) and the [on-screen keyboard](../../components/dialogs/keyboard/).
They share the modifiers in the table below. The API table on each component page lists them in one line and
links here.

| Modifier | What it does |
|---|---|
| `Align(v)` | Places the content inside the node. Takes a `geom.Alignment`, or a `ui.TextAlign` on a text. Not on every styled view, see below. |
| `Background(v)` | Fills the bounds, padding included, behind the content. Takes a `ui.Color`, a `ui.Gradient` or a `ui.GlassMaterial`. |
| `Border(v)` | Strokes a `ui.Border{Width, Color}` along the inside of the bounds, on top of the content. Does not change the layout. |
| `Clip(v)` | Confines the content to the bounds of the node, for drawing and for input. Off by default, except on the views that always clip. |
| `CornerRadius(v)` | Rounds the background, the border and the shadow. |
| `Flex(v)` | Takes a share of the space left over in the parent stack, in proportion to `v`. Zero or less means not flexible. |
| `Frame(w, h)` | Fixes the width and the height. Pass `geom.Unbounded()` to leave one axis free. |
| `Key(v)` | Gives the view a stable identity among its siblings. |
| `MaxHeight(v)` | Lowers the maximum height. |
| `MaxWidth(v)` | Lowers the maximum width. |
| `MinHeight(v)` | Raises the minimum height. |
| `MinWidth(v)` | Raises the minimum width. |
| `Padding(v)` | Sets the same padding on all four edges. Must be finite and not negative. |
| `PaddingInsets(v)` | Sets the padding per edge with a `geom.Insets{Top, Right, Bottom, Left}`. |
| `Shadow(v)` | Draws a blurred `ui.Shadow` behind the node. Does not change the layout or the hit area. |

Some views have only part of this set, because they would ignore the rest:

- `Align` exists on [Stack](../../components/layout/stack/), [ZStack](../../components/layout/zstack/),
  [Window](../../components/layout/window/), [Text](../../components/basic/text/), [Button](../../components/basic/button/) and
  [ScrollView](../../components/layout/scroll-view/). The other styled views have nothing to align.
- The controls [Toggle](../../components/basic/toggle/), [Slider](../../components/basic/slider/),
  [SegmentedControl](../../components/basic/segmented-control/) and [ProgressBar](../../components/basic/progress-bar/)
  draw themselves and only take `Key`, `Flex` and some of the size modifiers.
- Composed views such as [Row](../../components/lists/row/), [Card](../../components/layout/card/),
  [Badge](../../components/basic/badge/), [Divider](../../components/basic/divider/),
  [Spacer](../../components/layout/spacer/), [Sheet](../../components/dialogs/sheet/),
  [TabBar](../../components/navigation/tab-bar/) and [NavigationStack](../../components/navigation/navigation-stack/)
  take `Key` and `Flex`, plus the few modifiers their page lists.

The API table of each component shows exactly which modifiers it has.

### Align

`Align` positions the content of a node inside it. It does not position the node inside its parent; that is the
parent's `Align`. What "content" means depends on the view:

- a `Stack` aligns its children on the cross axis, a `VStack` reads the `X` component, an `HStack` the `Y`
  component,
- a ZStack and a `Window` use both components,
- a `Button` places its label inside the padding, centered by default,
- a `TextView` aligns its lines with `ui.AlignLeading`, `ui.AlignCenter` or `ui.AlignTrailing`.

`geom` has the nine standard values `geom.TopLeading`, `geom.Top`, `geom.TopTrailing`, `geom.Leading`,
`geom.Center`, `geom.Trailing`, `geom.BottomLeading`, `geom.Bottom` and `geom.BottomTrailing`. Alignment only
shows when the node is larger than its content. A label exactly as wide as its text looks the same under every
`TextAlign`; give it a width first:

```go
ui.Text("Centered").Frame(240, geom.Unbounded()).Align(ui.AlignCenter)
```

### Size

`Frame`, `MinWidth`, `MaxWidth`, `MinHeight` and `MaxHeight` all constrain the size of the node itself. They are
applied in this order, independent of the order of the calls:

1. `Frame` makes an axis fixed,
2. `MaxWidth` and `MaxHeight` lower it,
3. `MinWidth` and `MinHeight` raise it.

So a maximum beats a frame, and a minimum beats everything:

```go
ui.Box().Frame(200, 100).MaxWidth(50) // 50 wide
ui.Box().Frame(20, 20).MinWidth(80)   // 80 wide
ui.Box().Frame(geom.Unbounded(), 44)  // only the height is fixed
```

A minimum wins because it usually exists for content that cannot get any smaller. If the content does not fit,
gift lets it overflow and counts that in the diagnostics instead of squeezing it; see [Layout](../layout/) and
[Diagnostics](../diagnostics/).

`Flex` is not a size. It asks the parent stack for a share of the space that is left after the inflexible
children were measured. A child with `Flex(2)` gets twice as much of that space as a child with `Flex(1)`. See
[Layout](../layout/) for how stacks measure their children.

### Padding

Padding is space between the edge of the node and its content. It counts toward the size of the node, and the
background and the border cover it. `Padding` and `PaddingInsets` panic on a negative or non finite value, because
negative padding has no consistent meaning. For overlapping children, use a negative `Gap` on the stack.

`geom.InsetsAll` and `geom.InsetsSymmetric(vertical, horizontal)` build common insets:

```go
ui.HStack(ui.Text("Name"), ui.Spacer(), ui.Text("Timo")).
	PaddingInsets(geom.InsetsSymmetric(8, 16))
```

Some views have a default padding that any `Padding` or `PaddingInsets` call replaces. A `Button`, for example, starts with six
points vertically and twelve horizontally.

### Background, border, corner radius and shadow

These four together make a card:

```go
ui.VStack(
	ui.Text("Card"),
).
	Padding(20).
	Background(ui.ColorSurface).
	Border(ui.Border{Width: 1, Color: ui.ColorSeparator}).
	CornerRadius(16).
	Shadow(ui.Shadow{Blur: 16, OffsetY: 4, Color: ui.RGBA(0, 0, 0, 70)})
```

Things to know:

- The border lies inside the bounds and is drawn after the content. Content that reaches into the border area is
  covered by it, so keep the padding at least as wide as the border.
- The shadow takes the shape of the bounds and the corner radius. `Blur` follows the CSS convention, `Spread`
  grows or shrinks the shape, `OffsetX` and `OffsetY` move it. A shadow is drawn even without a background.
- The shadow extends the area that is painted, but not the layout size and not the hit area. It never moves a
  sibling. A parent that clips, such as a scroll view or a virtual list, cuts it off like any other content, so leave
  room for it in the parent's padding.
- On a `Button`, `Background`, `Border` and `CornerRadius` set the normal state. A hover, pressed or disabled
  state without a style of its own keeps that border and radius; a state you give its own style with
  `HoverStyle`, `PressedStyle` or `DisabledStyle` uses that style as written. See
  [Button](../../components/basic/button/).

[Shadow and border](../../components/styling/shadow-and-border/), [Colors](../../components/styling/colors/),
[Gradient](../../components/styling/gradient/) and [Glass](../../components/styling/glass/) show these values in
detail.

### Clip

gift clips nothing unless you ask for it, apart from the views that always clip (see
[above](#a-view-only-has-the-modifiers-it-honors)). Content that is larger than its node stays visible and draws
over its neighbors. `Clip(true)` confines the content to the bounds of the node, for drawing and for input together, so a
clipped part can neither be seen nor clicked.

The clip surrounds the content, not the node's own drawing: the node's shadow still extends past its bounds. The
clip is the bounding rectangle, even with a corner radius, so a clipped child can cover a rounded corner. An
[Image](../../components/basic/image/) is the exception: `Clip(true)` together with `CornerRadius` rounds the
picture itself.

### Key

`Key` gives a view a stable identity among its siblings. When gift rebuilds a container, it matches the new
children against the existing nodes: children with a key by key and type, children without a key by position and type. A
node that is matched keeps its state, its focus and its scroll offset.

Without keys, removing the first element of a list hands the state of every row to the row below it. Give each
child of a dynamic list a key from your data:

```go
rows := make([]gift.View, 0, len(items))
for _, it := range items {
	rows = append(rows, ui.Text(it.Name).Key(it.ID))
}
return ui.VStack(rows...)
```

A key only has to be unique among siblings. Duplicate keys panic in builds with `-tags giftdebug`. See
[State](../state/) for how state is attached to nodes.

## Related

- [Views](../views/) explains what a view is and how a screen is built from them.
- [Layout](../layout/) explains how sizes, flex and overflow work.
- [Theming](../theming/) explains semantic colors.
- [Custom views](../custom-views/) explains how to write view types of your own.
- The [kitchen sink](../../examples/example-kitchensink/) uses most of the modifiers on this page.
