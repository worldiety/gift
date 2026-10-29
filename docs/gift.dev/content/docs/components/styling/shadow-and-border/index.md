---
title: Shadow and Border
---

A border strokes the outline of a view, and a shadow draws a blurred copy of its shape behind it. Together with
`CornerRadius` and `Clip` they shape the box of a view. They are not views of their own but modifiers that every
styled view has: `VStack`, `HStack`, `ZStack`, `Box`, `Text`, `Icon`, `Image`, `Button`, `TextField`, `List`, `ScrollView` and
the others.

Use a border to separate a surface from its surroundings, or to mark a state such as a selection. Use a shadow to
lift a surface above the content around it.

{{< gallery-shot >}}

{{< gallery-code >}}

## Usage

### Border

`Border` takes a `ui.Border` with a `Width` in logical pixels and a `Color`. The stroke lies inside the bounds of
the view, so a border never changes the layout: a tile keeps its size whether its border is 1 or 4 pixels wide. A
border with a width of zero or a fully transparent color draws nothing.

```go
ui.VStack(ui.Text("Selected")).Padding(16).
	Background(ui.ColorSurface).
	CornerRadius(12).
	Border(ui.Border{Width: 2, Color: ui.ColorAccent})
```

Use semantic colors such as `ui.ColorSeparator` or `ui.ColorAccent`. They are resolved against the current theme
each time the view is built, so the border follows a switch between light and dark.

### Shadow

`Shadow` takes a `ui.Shadow`. `Blur` is the blur diameter, the same value CSS and design tools use: the Gaussian
has a standard deviation of `Blur/2`, and the visible falloff reaches about one and a half times `Blur` past the
shape. A `Blur` of zero gives a hard edge. `OffsetX` and `OffsetY` move the shadow, positive right and down.
`Spread` grows the shape before it is blurred; a negative `Spread` shrinks it and tucks the shadow under the view,
so only the offset side shows.

```go
card := ui.VStack(ui.Text("Lifted")).Padding(16).
	Background(ui.ColorSurface).
	CornerRadius(12).
	Shadow(ui.Shadow{Blur: 20, Spread: -6, OffsetY: 12, Color: ui.RGBA(0, 0, 0, 90)})
```

A shadow extends the painted area, but neither the layout size nor the hit area. It never pushes a sibling away and
never makes the gap around a view clickable. Leave room for it: a parent that clips, such as a `ScrollView`, cuts
the shadow off at its edge like anything else, so give the content some padding.

The shadow is the shape of the view's bounds, drawn whether or not the view has a background. Give a shadowed view
an opaque background, or the shadow shows through it. On a dark background a black shadow has little contrast, so
in the dark theme a border or a lighter surface often separates better.

A `Blur` that is negative or not finite, and a `Spread` or offset that is not finite, panic when the modifier is
applied, so the mistake shows up at the call site and not as an empty window.

### Corner radius

`CornerRadius` rounds the background, the border and the shadow together. The shadow's corners grow with a positive
`Spread` and shrink with a negative one, down to square corners. The radius does not cut the content; that is what
`Clip` is for.

### Clip

Content clipping is off by default, so a child may paint past the bounds of its parent. `Clip(true)` confines the
children to the bounds of the view, for painting and for input alike: a clipped part of a child neither shows nor
receives taps. A few views always clip; see the end of this section.

The clip of a container is the bounding rectangle, not the rounded shape. A child that fills a rounded stack covers
its rounded corners even with `Clip(true)`. Pictures are the exception: `Image` with a `CornerRadius` and
`Clip(true)` rounds the picture itself, as the last tile of the demo does.

```go
ui.Image(src).Fit(ui.FitCover).Frame(160, 96).
	CornerRadius(12).
	Clip(true)
```

The clip surrounds the content, not the view's own drawing. Its shadow still extends past the bounds, and its
border is drawn on top of the clipped content. `ScrollView`, `VirtualList`, `ImageGallery`, `TextField` and
`OnScreenKeyboard` always clip; `Clip(false)` on them panics.

### Drawing order

A view draws in a fixed order: shadow, background, content, border. The order does not depend on the order in which
you call the modifiers. The border is drawn last, so it lies on top of the children, which is how the demo frames a
picture with a border in the surface color. Calling a modifier again replaces its value; the last call wins.

## API

`ui.Border` and `ui.Shadow` are aliases of `render.Border` and `render.Shadow`. They are plain structs; you set their
fields directly.

### Border

| Field   | Type      | Description                                                 |
|---------|-----------|-------------------------------------------------------------|
| `Width` | `float32` | Stroke width in logical pixels, drawn inside the bounds.    |
| `Color` | `Color`   | Stroke color. A transparent color draws nothing.            |

### Shadow

| Field     | Type      | Description                                                                         |
|-----------|-----------|-------------------------------------------------------------------------------------|
| `Blur`    | `float32` | Blur diameter in logical pixels; the standard deviation is `Blur/2`. Zero is a hard edge. |
| `Spread`  | `float32` | Grows the shape before the blur; a negative value shrinks it.                       |
| `OffsetX` | `float32` | Horizontal offset, positive to the right.                                           |
| `OffsetY` | `float32` | Vertical offset, positive downwards.                                                |
| `Color`   | `Color`   | Color at full coverage. The zero Shadow draws nothing.                              |

See [render.Border](https://pkg.go.dev/github.com/worldiety/gift/render#Border) and
[render.Shadow](https://pkg.go.dev/github.com/worldiety/gift/render#Shadow) on pkg.go.dev for the helper methods
`IsVisible`, `Sigma` and `Extent`.

## Related

- [Colors](../colors/): the semantic colors to use for borders and backgrounds.
- [Gradient](../gradient/) and [Glass](../glass/): the other kinds of background.
- [Card](../../layout/card/): a surface with a corner radius and a hairline border.
- [Image](../../basic/image/): rounded, clipped pictures.
- [Button](../../basic/button/): borders and corner radii per button state.
- [Modifiers](../../../concepts/modifiers/): how style modifiers work on every view.
- [Theming](../../../concepts/theming/): how semantic colors resolve in light and dark.
