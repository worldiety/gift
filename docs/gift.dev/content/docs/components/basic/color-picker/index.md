---
title: Color Picker
---

`ui.ColorPicker` is the colour picker of iOS, `UIColorPickerViewController`, as a panel 320 points wide: a grid of
swatches, a spectrum of every hue and sliders for red, green and blue, one at a time under a segmented control, with
an opacity slider, the colour and its hex code below them. [Color Well](../color-well/) puts it in a popover behind a
swatch.

{{< gallery-shot alt="A colour picker in a card showing the spectrum, with an orange loupe on it, an opacity slider at 90 percent over a checkerboard, the colour as a swatch and the hex code FF9500E6" >}}

{{< gallery-code >}}

## Usage

```go
c := ctx.State("c", ui.RGB(0, 122, 255))
ui.ColorPicker(ctx.Read(c), c.Set)
```

The picker is controlled, like a [Slider](../slider/): it shows the colour it is given and reports a new one to
`onChange`, with every move of a finger over the spectrum or a slider. Read the state with `ctx.Read`, not with
`Get`: only a build that read it runs again when it changes, and a picker nobody builds again keeps showing the old
colour. `SupportsOpacity(false)` hides the opacity slider and keeps every colour opaque.

## The three views

- **Grid**: twelve columns of swatches, greys from white to black on top and twelve hues from dark to pale below
  them. A tap or a drag chooses one; the chosen swatch has a white frame.
- **Spectrum**: every hue from left to right, from white at the top through the full colour in the middle to black
  at the bottom. A tap or a drag chooses the colour under the finger; a loupe shows where the colour is.
- **Sliders**: red, green and blue, each on a track that shows the colour with that channel from none to full, with
  its value from 0 to 255 beside it. Focused, the arrow keys step a slider by one.

The opacity slider works the same way over a checkerboard, from transparent to opaque. Choosing a swatch or a point
of the spectrum keeps the opacity.

The hex field takes `#RRGGBB` and `#RRGGBBAA` as they are typed, and shows the colour's code whenever it changes.
`ui.HexColor` and `ui.ParseHexColor` are the same conversion for your own code.

## Look

The picker stands on `ui.ColorSurface`, the colour of a popover and a card, which also rounds the corners of the grid
and the spectrum. `Background` sets another colour to stand on. The spectrum is drawn as six horizontal gradients,
one per pair of neighbouring primaries, under two vertical ones, so it is exact at every size and costs nine quads.

## API

{{< api "ColorPickerView" >}}

{{< api-funcs pkg="ui" names="HexColor" >}}

`ParseHexColor` returns a `ui.Color` and is listed with that type, next to `RGB` and `RGBA`:

{{< api "Color" >}}

## Related

- [Color Well](../color-well/), the swatch that opens the picker
- [Popover](../../dialogs/popover/), which the well opens it in
