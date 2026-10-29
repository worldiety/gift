---
title: Button
---

A button runs a closure when the user activates it. Its label is any view: a text, an icon, or a stack of both.
The button handles the whole interaction for you. A press captures the pointer, so releasing outside the button
does not activate it. A touch taps without ever showing a hover. Tab moves the keyboard focus to the button, and
space or enter activates it.

Use a button for a command. For an on/off setting use a [Toggle](../toggle/), for one choice out of a few a
[Segmented Control](../segmented-control/).

{{< gallery-shot >}}

{{< gallery-code >}}

## Usage

### Label and action

`ui.Button` takes the label view and the action. The action usually changes state, and the next frame shows the
result:

```go
count := ctx.State("count", 0)
ui.Button(ui.Text("Reset"), func() { count.Set(0) })
```

The label stays yours. The button styles its own box, not the label, so it never recolors the text you hand it.
Set the color on the label itself, for example `ui.ColorOnAccent` on an accent button or `ui.ColorDanger` for a
destructive command. A disabled button dims its box but not its label; give the label `ui.ColorSecondaryLabel`
when it should look disabled too.

A label that is a text gives the button its accessible name. An icon carries no name, so set one with `Label`. It
is never drawn; tests and accessibility tools find the button through it:

```go
ui.Button(ui.Icon(outline.Cog).Size(18), openSettings).
	Label("Settings")
```

### Size

The label decides the size, and the padding grows it. The default padding is 6 points at the top and bottom and 12
at the sides; `Padding` and `PaddingInsets` replace it. `Frame` and the `Min…`/`Max…` modifiers override the
measured size. A button does not enforce a minimum hit area. For touch screens add
`MinHeight(ui.ControlHitTarget)`, which is 44 points, as the demo does.

Next to another button in an `HStack`, a button reports the baseline of its label, so `AlignBaseline` lines up the
text of buttons with different fonts or paddings.

### States and styles

A button draws in one of four states: normal, hover, pressed and disabled. Hover and press live in the retained
node, so they repaint the button without running your view function again. That is why you declare the look of
every state up front instead of choosing it per frame:

- `Background`, `Border` and `CornerRadius` describe the normal state, like on any other view. `Style` sets the
  same three from a `ButtonStyle` and only touches the fields you wrote.
- `HoverStyle`, `PressedStyle` and `DisabledStyle` each take a `ButtonStyle` that replaces the whole look of that
  state. An unset border means no border there, and an unset corner radius means square corners.

Without any styling, a button uses the theme: the face is `ui.ColorControl`, and `ColorControlHover`,
`ColorControlPressed` and `ColorControlDisabled` for the other states, with a one point hairline in
`ui.ColorSeparator` and a corner radius of 6. In the disabled state the hairline is drawn at half strength. If you set only a corner radius or a border, the button keeps the
themed faces for every state. If you set a background of your own, the hover, pressed and disabled states keep that
background unless you declare them, so an accent button does not turn gray under the pointer.

In a `ButtonStyle`, an unset `Background` means the themed face of that state, not transparency. Write
`ui.ColorClear` when you want nothing drawn. `BackgroundTo` turns the face of that state into a vertical
gradient from `Background` at the top to `BackgroundTo` at the bottom. A `Border` with a width and no color is
stroked in `ui.ColorSeparator`. This gives you a borderless button that only shows a face on hover and press:

```go
ui.Button(ui.Text("More"), more).
	Background(ui.ColorClear).
	Border(ui.Border{}).
	CornerRadius(8).
	HoverStyle(ui.ButtonStyle{CornerRadius: 8}).
	PressedStyle(ui.ButtonStyle{CornerRadius: 8})
```

The normal state takes a gradient too, through `Style` with a `BackgroundTo` or through `Background` with a
`ui.LinearGradient`. A hover or pressed state you do not declare keeps that gradient. In `Style`, a
`BackgroundTo` without a `Background` starts the gradient at the themed face. `Background` with the glass
material panics, because a `ButtonStyle` has no field for a material and the other states could not keep it. For
a glass control, put a glass `Box` behind the button in a `ZStack` and give the button `ui.ColorClear`.

`Shadow` draws a shadow behind the button. It is the same in every state.

To restyle every button of an application, change the theme instead of each button:

```go
ui.SetTheme(app, ui.LightTheme().With(ui.ColorControl, ui.RGB(226, 232, 240)))
```

### Focus ring and lift

While the button holds the keyboard focus and the focus is visible, it draws a two point ring in
`ui.ColorAccent` on top, three points inside its edge. `FocusRing` replaces it with another border; a zero width border removes it. The ring is
the same in every state, so it is not part of `ButtonStyle`.

`Lift(true)` adds the press feedback of iOS 26: the button grows slightly while pressed, lights up under
the finger and springs back on release. Only the drawing grows; the layout size and the neighbors stay where they
are. It is off by default, because the moving picture suits a primary action but not a row in a list.

### Disabled

`Disabled(true)` takes the button out of input. It is skipped by the focus order, receives no events and draws in
its disabled style. It still takes its place in the layout and still blocks clicks from reaching the views behind
it. Prefer it over a nil action: a nil action makes the button inert without telling the user.

## API

{{< api "ButtonView" >}}

{{< api type="ButtonStyle" decl="true" >}}

## Related

- [Text](../text/) and [Icon](../icon/), the usual labels
- [Toggle](../toggle/) and [Segmented Control](../segmented-control/), controls that hold a value
- [Alert](../../dialogs/alert/), whose buttons are laid out for you
- [Colors](../../styling/colors/) and [Shadow and Border](../../styling/shadow-and-border/)
- [Modifiers](../../../concepts/modifiers/), [State](../../../concepts/state/),
  [Theming](../../../concepts/theming/) and [Input and Focus](../../../concepts/input-and-focus/)
- The example [kitchen sink](../../../examples/example-kitchensink/), which uses buttons throughout
