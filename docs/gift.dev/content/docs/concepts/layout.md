---
title: Layout
weight: 4
---

Layout decides how big every view is and where it sits. gift measures the whole tree in a single pass:
constraints go down, sizes come back up, and each parent places its children. This page explains that pass,
how stacks divide space, how `Flex`, `Frame`, `Min`/`Max`, `Gap` and alignment change it, what happens when
content does not fit, and why every number you write is a logical pixel.

## Constraints go down, sizes come up

Every node gets a `geom.Constraints` from its parent: a smallest and a largest acceptable size. The node
measures its children, picks a size inside that range and returns it. Then the parent places the node.
There is no separate arrange pass in which a child could change its mind.

```go
type Constraints struct {
	Min, Max Size
}
```

Either component of `Max` can be `geom.Unbounded()`, which is positive infinity. An unbounded axis means
"take as much as your content needs", for example along the scroll axis of a scroll view.

Three words describe the common cases:

| Constraints | Meaning                                    | Built with                 |
|-------------|--------------------------------------------|----------------------------|
| tight       | `Min == Max`, the size is fixed            | `geom.Tight(size)`         |
| loose       | `Min` is zero, `Max` is a limit            | `geom.Loose(size)`         |
| unbounded   | `Max` is infinite on an axis               | `geom.Unconstrained()`     |

The root of your application gets loose constraints of the window size. [`ui.Window`](../../components/layout/window/)
is a [ZStack](../../components/layout/zstack/), so it passes those loose, bounded constraints on to your
content.

Each view type has its own rule for picking a size. A [Text](../../components/basic/text/) wraps its string at
the width it is allowed and reports the result. A [Box](../../components/layout/box/) has no content, so it is
greedy: it takes the whole extent on every bounded axis and collapses to its padding on an unbounded one. A
stack sizes itself from its children, as the next section explains.

gift caches the size of every node together with the constraints it was measured with. A node that did not
change and gets the same constraints again is not measured again, so a relayout costs only the part of the tree
that changed.

## How a stack divides space

[`ui.VStack`](../../components/layout/stack/) places its children from top to bottom, `ui.HStack` from leading
to trailing. The stacking direction is the *main axis*, the other one the *cross axis*. A stack measures in two
passes.

{{% steps %}}

### Inflexible children

Every child without `Flex` is measured first. It gets the full cross axis of the stack, minus padding, as a
loose limit, and an **unbounded** main axis. It does not get "whatever is left": the size of a child never
depends on how many siblings come before it.

### Flexible children

Then the stack takes its own main axis limit, subtracts what the inflexible children took and the gaps, and
divides the rest among the children with `Flex`, in proportion to their flex. Each flexible child gets its share
as a *tight* main axis constraint. If nothing is left, every flexible child gets zero.

### Size and placement

The stack is as long as its children plus the gaps, and as wide as its widest child, both plus its padding
and clamped into its own constraints. Then it places the children one after another and positions each one on the cross axis by its
alignment.

{{% /steps %}}

Two consequences follow, and they explain most layouts:

- **A stack without a flexible child shrink-wraps its content.** A `VStack` of three labels is as tall as the
  three labels, however tall the window is.
- **A stack with a flexible child fills its main axis**, because the flexible child takes all the free space.
  An `HStack` with a [Spacer](../../components/layout/spacer/) is as wide as its parent allows.

This screen uses both. The header row contains a spacer, so it is as wide as the column and the button sits at
the trailing edge. The scroll view has `Flex(1)`, so it takes the height the header and the footer leave over,
and the column fills the window:

```go
func screen(ctx *gift.Context) gift.View {
	return ui.Window(ui.VStack(
		ui.HStack(
			ui.Text("Inbox").FontSize(22),
			ui.Spacer(),
			ui.Button(ui.Text("Compose"), func() {}),
		).Align(geom.Center),
		ui.VScroll(messages()...).Gap(8).Flex(1),
		ui.Text("2 unread").Foreground(ui.ColorSecondaryLabel),
	).Gap(12).Padding(16))
}
```

## Flex

`Flex(v)` makes a view take a share of the free main axis space of its parent stack, proportional to `v`. Zero,
the default, means inflexible; a negative value means the same. Two children with flex 1 and 3 split the free
space one to three. The shares always add up to the free space exactly.

`ui.Spacer()` is an invisible view with a flex of one. `Spacer().Flex(3)` changes its share, and
`Spacer().MinLength(16)` keeps at least 16 pixels even when no space is left.

Flex only works where there is something to divide:

- **Flex needs a bounded main axis.** In an unbounded axis there is no free space, so every flexible child gets
  zero. A spacer inside a `VScroll` has no height, and a spacer in an `HStack` that is itself an inflexible child
  of another `HStack` has no width.
- **Flex has to be passed down the chain.** An inflexible child of a stack is measured with an unbounded main
  axis, so its own flexible children get nothing. To fill a screen, give `Flex` to every stack between the window
  and the view that should grow, as the kitchen sink example does for its scroll views.
- **A ZStack ignores the flex of its children.** It has no main axis. Every child of a ZStack already gets the
  full, bounded box, so an unframed `Box` fills it without `Flex`.

### Wrapping text in a row

The unbounded main axis has one consequence for text that is easy to miss. An inflexible
[Text](../../components/basic/text/) in an `HStack` is measured with an unbounded width, so it never wraps. It
keeps its full width and the row overflows. In a `VStack` the same text wraps, because its width is the cross
axis, which is bounded as long as the `VStack` itself has a bounded width.

Give the text a share of the row, or a width of its own:

```go
ui.VStack(
	// Inflexible: measured with an unbounded width, so it never wraps.
	ui.HStack(ui.Icon(outline.InfoCircle).Size(20), ui.Text(longText)).Gap(8),
	// Flexible: gets the leftover width as a tight constraint and wraps in it.
	ui.HStack(ui.Icon(outline.InfoCircle).Size(20), ui.Text(longText).Flex(1)).Gap(8),
	// A maximum width wraps it without a share of the row.
	ui.HStack(ui.Icon(outline.InfoCircle).Size(20), ui.Text(longText).MaxWidth(240)).Gap(8),
)
```

The same rule applies to a [Row](../../components/lists/row/) outside a list: its flexible gap has no width to
spread into in an `HStack`, so the row shrink-wraps.

## Frame, Min and Max

Every styled view has the same five sizing modifiers. They change the constraints a view passes to its own
layout, before it measures its content:

| Modifier                 | Effect                                                               |
|--------------------------|----------------------------------------------------------------------|
| `Frame(w, h)`            | makes both axes tight; `geom.Unbounded()` leaves an axis free        |
| `MinWidth`, `MinHeight`  | raises the minimum, and the maximum with it if that is lower         |
| `MaxWidth`, `MaxHeight`  | lowers the maximum, and the minimum with it if that is higher        |

```go
ui.VStack(
	ui.Box().Frame(120, 40).Background(ui.ColorAccent),
	ui.Text("Name").Frame(80, geom.Unbounded()),
	ui.Button(ui.Text("OK"), func() {}).MinWidth(ui.ControlHitTarget).MinHeight(ui.ControlHitTarget),
	ui.Box().Frame(200, 100).MaxWidth(50),
	ui.Box().Frame(20, 20).MinWidth(80),
)
```

The modifiers are fields, not wrappers, so the order in which you call them does not matter. The order in which
gift applies them is fixed: first `Frame`, then `Max`, then `Min`.

- **Max beats Frame.** `Frame(200, 100).MaxWidth(50)` is 50 wide.
- **Min beats everything.** `Frame(20, 20).MinWidth(80)` is 80 wide, and `MinWidth(80).MaxWidth(40)` is 80 wide.
  A minimum usually exists because the content cannot be smaller, so gift never clamps it away.

Sizes include the padding. `Text("OK").Padding(8).Frame(80, 40)` is 80 by 40 and its text gets 64 by 24.

A frame wins over the parent. If you fix a view at 400 pixels inside a 300 pixel column, it is 400 pixels wide,
and the column reports an overflow instead of squeezing it; see [When content does not fit](#when-content-does-not-fit).

`MaxWidth` is also how you keep a column readable on a wide screen. This settings column is never wider than 640
pixels and sits centered at the top of the window:

```go
func column(ctx *gift.Context) gift.View {
	return ui.Window(
		ui.VStack(ui.Text("Settings").FontSize(22), ui.Text("...")).
			Gap(12).Padding(24).MaxWidth(640),
	).Align(geom.Top)
}
```

## Gap and padding

`Gap(v)` inserts space between two adjacent children of a stack. It is never added before the first or after
the last child, so three children have two gaps. A negative gap is allowed and overlaps neighbors. A gap that is
not a finite number panics.

`Padding(v)` and `PaddingInsets(geom.Insets{...})` add space inside the edges of a view. The children are
measured against the constraints minus the padding, and the padding is added back to the size. Padding must be
finite and not negative, otherwise the modifier panics. Use a negative `Gap` for overlap and `Frame` or
`MinWidth` for a bigger container.

The [Modifiers](../modifiers/) page lists the other modifiers every view shares.

## Alignment

A `geom.Alignment` is a pair of factors from 0 to 1: 0 is leading or top, 0.5 is the center, 1 is trailing or
bottom. The package defines the nine usual values: `geom.TopLeading`, `geom.Top`, `geom.TopTrailing`,
`geom.Leading`, `geom.Center`, `geom.Trailing`, `geom.BottomLeading`, `geom.Bottom` and `geom.BottomTrailing`.
The zero value is `geom.TopLeading`.

- In a stack, `Align` only positions children on the cross axis. A `VStack` reads the `X` factor, an `HStack`
  the `Y` factor. The main axis position follows from the order of the children; use a `Spacer` to push them
  apart.
- In a [ZStack](../../components/layout/zstack/) and in `ui.Window`, `Align` uses both factors, because neither
  axis is a stacking axis.

Alignment only moves a child inside the space its parent has. A `VStack` is as wide as its widest child, so
`Align(geom.Center)` centers the narrower children relative to the widest one, not relative to the window. To
center the whole stack, align its parent.

`TextView.Align` is different: it takes a `ui.TextAlign` and aligns the lines of a paragraph inside the width of
the text view. See [Text and fonts](../text-and-fonts/).

### Baseline alignment

Labels of different font sizes look wrong when you center them: their letters do not sit on one line.
`HStack(...).AlignBaseline()` lines the children up on their first text baseline instead:

```go
ui.HStack(
	ui.Text("21").FontSize(44),
	ui.Text("°C").FontSize(20),
	ui.Text("Living room").Foreground(ui.ColorSecondaryLabel),
).Gap(6).AlignBaseline()
```

A text view, a text field and a button whose label is a text report their first baseline during layout. A
child that reports none, such as a `Box`, an icon or a nested stack, is not guessed at: it keeps the ordinary
`Align` placement inside the row. `AlignBaseline` replaces a
previous `Align`, and calling it on a `VStack` panics, because in a vertical stack the vertical position is
already decided by the stacking.

## Scroll views need a bounded extent

A [scroll view](../../components/layout/scroll-view/) measures its children with the scroll axis unbounded and
reports the size its own constraints permit. The difference is what there is to scroll. So a scroll view needs a
limit on its scroll axis, and inside a stack it does not get one by default: an inflexible child gets an
unbounded main axis. A bare `VScroll` in a `VStack` is as tall as its content, nothing scrolls, and the stack
around it overflows.

Give it `Flex(1)` to take the leftover space, as in the screen above, or a fixed limit:

```go
ui.VStack(
	ui.Text("Recent"),
	ui.VScroll(messages()...).MaxHeight(240),
)
```

## When content does not fit

gift does not hide a layout that does not fit. When the content of a node is bigger than the size its
constraints allow:

- the node reports the size its constraints permit, so a child never resizes its parent,
- the children keep their honest sizes and positions, and nothing is clipped automatically,
- the excess is recorded as a number, the *overflow* of the node.

A word that is wider than its label is not broken or scaled; it overflows. A row of fixed-width buttons that is
wider than the window overflows. Both are visible on screen, and both are counted.

{{< callout type="info" >}}
Overflow is not an error and nothing panics. It is a number you can check, in a test or at runtime, so that "the
content does not fit" never turns into a row that silently collapses to zero.
{{< /callout >}}

### Finding overflow

`App.Diagnostics()` returns two counters, and it is safe to call from any goroutine:

- `OverflowNodes` is the number of nodes whose content does not fit. A healthy scene has zero.
- `OverflowExtent` is the sum of their horizontal and vertical overflow in logical pixels, so you can tell a
  half-pixel problem from an 800 pixel one.

`App.NodeOverflow(ref)` tells you how much one node overflows. A build with the `giftdebug` tag also logs every
node that starts or stops overflowing, with its type and key, to the logger of `gift.Options` or to standard
error:

```bash
go run -tags giftdebug ./cmd/myapp
```

In a test, `AssertNoOverflow` checks the whole scene in one line. Resize the harness to check the narrow and the
wide case:

```go
func TestScreenFits(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{
		Root: screen,
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}),
		Size: geom.Sz(360, 640),
	})
	h.AssertNoOverflow()
	h.Resize(geom.Sz(1280, 800))
	h.AssertNoOverflow()
}
```

See [Testing](../testing/) and [Diagnostics](../diagnostics/) for more.

### Fixing overflow

Fix the layout, not the symptom:

- Give text a width to wrap in: `Flex`, `Frame` or `MaxWidth`; see [Wrapping text in a row](#wrapping-text-in-a-row).
- Give a long list a scroll view with a bounded extent.
- Let a label end in an ellipsis with `TextView.MaxLines(1)` or `TextView.Truncation`. A shortened label still
  reports what the ellipsis hides as overflow, so `AssertNoOverflow` still sees it.
- Cut content off at the bounds of a container with `Clip(true)`. Clipping changes what you see, not the layout:
  the overflow stays and is still counted.

## Logical pixels and density

Every number in layout is a *logical pixel*: sizes, paddings, gaps, font sizes, the constraints a layouter gets
and the bounds a test reads. The same application has the same layout on every display.

The *density* says how many physical pixels one logical pixel covers along each axis. On a Retina display it is
2. gift only works with whole numbers:

- `gift.RoundDensity` rounds the factor the platform reports to the nearest integer between 1 and
  `gift.MaxDensity`, which is 4. A factor of 1.5 becomes 2 and gift renders at twice the logical size, which the
  backend then scales down; 1.4 becomes 1.
- The Ebitengine backend reads the factor of the monitor on almost every frame and calls `App.SetDensity`, so a window
  dragged to another monitor follows. You do not call it yourself.
- `gifttest.Options.Density` runs a test at a given density.

A density change changes the presentation, not the layout. gift scales the whole display list at its root, so
shapes and text become sharper, not bigger. Pointer positions and hit testing stay in logical pixels, so a drag
of 100 logical pixels scrolls 100 logical pixels on every display.

{{< api-funcs pkg="gift" names="RoundDensity" >}}

## The geom package

Package `geom` holds the value types layout, input and rendering share: `Point`, `Size`, `Rect`, `Insets`,
`Constraints`, `Alignment` and `Affine2D`. All coordinates are `float32`, every type is a small comparable value,
and every operation returns a new value without allocating. Short constructors keep code compact:

| Constructor                                   | Result                                     |
|-----------------------------------------------|--------------------------------------------|
| `geom.Pt(x, y)`                               | a `Point`                                  |
| `geom.Sz(w, h)`                               | a `Size`                                   |
| `geom.Rc(minX, minY, maxX, maxY)`             | a `Rect` from two corners                  |
| `geom.RcXYWH(x, y, w, h)`, `geom.RcSize(p, s)`| a `Rect` from an origin and a size         |
| `geom.InsetsAll(v)`, `geom.InsetsSymmetric(v, h)` | `Insets` for `PaddingInsets`           |
| `geom.Unbounded()`                            | the infinite constraint maximum            |

`geom.Insets` is ordered `Top, Right, Bottom, Left`, clockwise like CSS.

{{< api pkg="geom" type="Constraints" >}}

{{< api pkg="geom" type="Alignment" >}}

## Writing your own layout

The containers of package `ui` cover most screens. When you need a different arrangement, a view can bring its
own `gift.Layouter`. It gets the constraints, measures each child once with `LayoutContext.Measure`, places it
with `LayoutContext.Place` and returns its own size. This layouter centers its only child in all the space it
is offered:

```go
type centerLayout struct{}

func (centerLayout) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	child := ctx.Measure(0, c.Loosen())
	size := child
	if c.HasBoundedWidth() {
		size.W = c.Max.W
	}
	if c.HasBoundedHeight() {
		size.H = c.Max.H
	}
	size = c.Constrain(size)
	at := geom.Center.Position(child, geom.RcSize(geom.Pt(0, 0), size)).Min
	ctx.Place(0, at)
	return size
}
```

A layouter must measure and place every child exactly once and must be a pure function of its constraints, its
children and its view, because gift caches its result. It reports overflow with `LayoutContext.ReportOverflow`
and a text baseline with `LayoutContext.ReportBaseline`. [Custom views](../custom-views/) shows how to wrap it in
a view.

{{< api pkg="gift" type="LayoutContext" >}}

## Related

- Components: [Stack](../../components/layout/stack/), [ZStack](../../components/layout/zstack/),
  [Spacer](../../components/layout/spacer/), [Box](../../components/layout/box/),
  [Scroll view](../../components/layout/scroll-view/), [Window](../../components/layout/window/),
  [Card](../../components/layout/card/)
- Concepts: [Modifiers](../modifiers/), [Text and fonts](../text-and-fonts/), [Custom views](../custom-views/),
  [Testing](../testing/), [Diagnostics](../diagnostics/)
- Examples: [Kitchen sink](../../examples/example-kitchensink/), [Counter](../../examples/example-counter/)
