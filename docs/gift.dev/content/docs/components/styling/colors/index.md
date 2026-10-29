---
title: Colors
---

Every color in gift is a `ui.Color`. You write one either as a literal, such as `ui.RGB(40, 110, 225)`, or as a
semantic color, such as `ui.ColorAccent`. A semantic color names a role, not a look. It takes its value from the
theme in force, so the same code draws correctly in the light and the dark theme.

Use semantic colors wherever a role fits, and literals only for colors that must not change with the theme, such as
a brand color or a chart series.

{{< gallery-shot alt="A grid of swatches for every semantic color, and the accent color faded to 60, 30 and 10 percent" >}}

{{< gallery-code >}}

## Usage

### Semantic colors

`ui.SemanticColors()` returns the twelve roles that a theme assigns, in this order:

| Color                     | Role                                                                 |
|---------------------------|----------------------------------------------------------------------|
| `ui.ColorLabel`           | primary text                                                         |
| `ui.ColorSecondaryLabel`  | supporting text: captions, units, hints                              |
| `ui.ColorBackground`      | the page behind everything; only [`ui.Window`](../../layout/window/) paints it |
| `ui.ColorSurface`         | a panel raised above the background: a card, a sheet, a bar           |
| `ui.ColorSeparator`       | a hairline: a border, a divider, the edge of a control                |
| `ui.ColorAccent`          | what carries attention: a filled primary control, a selection, focus  |
| `ui.ColorOnAccent`        | content drawn on `ui.ColorAccent`                                     |
| `ui.ColorControl`         | the face of a control at rest                                         |
| `ui.ColorControlHover`    | the face of a control under the pointer                               |
| `ui.ColorControlPressed`  | the face of a control being pressed                                   |
| `ui.ColorControlDisabled` | the face of a disabled control                                        |
| `ui.ColorDanger`          | a choice that destroys something, such as the Delete button of an alert |

A thirteenth color, `ui.ColorClear`, is explicit transparency. It is not part of a theme and is not in the list.
You need it because the zero `Color` means "not set" in style structs such as `ui.ButtonStyle`, where the theme
then picks a default. When you really want nothing drawn there, write `ui.ColorClear`.

You pass semantic colors anywhere a `Color` goes:

```go
ui.Text("3 files").Foreground(ui.ColorSecondaryLabel)
```

Pair colors by role: text on `ui.ColorAccent` uses `ui.ColorOnAccent`, text on a surface or a control uses
`ui.ColorLabel`. The built-in themes keep these pairs legible.

### Literal colors

`ui.RGB(r, g, b)` makes an opaque color from 8-bit components. `ui.RGBA(r, g, b, a)` adds a straight alpha, so
`ui.RGBA(255, 149, 0, 128)` is a half-transparent orange:

```go
ui.Box().Background(ui.RGBA(255, 149, 0, 128)).Frame(40, 40)
```

A `Color` stores premultiplied components in the range 0 to 1. `RGBA` does the premultiplication for you; build a
`Color` struct by hand only if you already have premultiplied values. The zero `Color` is fully transparent.

### Fade

`ui.Fade(c, f)` returns `c` at `f` times its opacity, with `f` between 0 and 1. Any other value panics. On a
semantic color the result stays semantic: the fade is applied after the theme is consulted. So a faded color works
as a package variable and still follows every later theme change:

```go
var track = ui.Fade(ui.ColorLabel, 0.08)
```

Use `Fade` to express emphasis without a new role: the theme decides the hue, you decide how present it is.

### Resolving colors

A semantic color is resolved to a literal one when a view is built. The views in `ui` do this for you. If you write
your own view that emits render operations, resolve every color yourself, once, in `Build`:

```go
accent := ui.ResolveColor(ui.ColorAccent)    // the accent of the theme in force
dark := ui.DarkTheme().Color(ui.ColorAccent) // the accent of the dark theme
ok := ui.IsSemantic(ui.ColorAccent)          // true
```

`ResolveColor` returns a literal color unchanged, so it is safe to call on any color. `Theme.Color` does the same
against a given theme instead of the installed one. `IsSemantic` tells the two kinds apart.

{{< callout type="warning" >}}
An unresolved semantic color does not draw a wrong color. It has an alpha of zero, so it draws nothing at all, and
your view stays invisible without an error. Under the `giftdebug` build tag, `render.List.Add` panics on an
operation that still carries one, but only if your painter does not skip it as transparent first. So check the color
where you decide whether to draw.
{{< /callout >}}

### Themes

A `ui.Theme` assigns a literal color to each role. `ui.LightTheme()` and `ui.DarkTheme()` are the built-in ones,
and the light theme is the default. You derive your own with `Theme.With` and install it with `ui.SetTheme`, which
also rebuilds the whole application:

```go
ui.SetTheme(app, ui.DarkTheme().With(ui.ColorAccent, ui.RGB(255, 149, 0)))
```

`With` takes a semantic color as its first argument and a literal one as its second, and panics otherwise. It
also panics on `ui.ColorClear`, which no theme can redefine. `Theme.IsDark` reports whether a theme is dark:
`DarkTheme` sets the flag, `With` keeps it, and `Theme.Dark` sets it on a palette you build yourself. The
[Theming](../../../concepts/theming/) page covers switching themes in depth.

To list the palette, for a theme editor or a contrast check, iterate `SemanticColors`. Each entry is a
`ui.NamedColor` with the Go name and the color:

```go
for _, c := range ui.SemanticColors() {
	fmt.Println(c.Name, ui.CurrentTheme().Color(c.Color))
}
```

The slice is freshly allocated on each call. Do not call it per frame.

## API

### Color

`ui.Color` is an alias of `render.Color`:

| Field     | Type      | Description                                        |
|-----------|-----------|----------------------------------------------------|
| `R, G, B` | `float32` | the color channels, premultiplied by `A`, 0 to 1   |
| `A`       | `float32` | the alpha channel, 0 fully transparent, 1 opaque   |

It has two methods: `IsOpaque()` reports whether `A` is 1, and `IsTransparent()` whether `A` is 0. Do not call them
on a semantic color; resolve it first.

{{< api "Color" >}}

{{< api-funcs pkg="ui" names="IsSemantic" >}}

### Theme

{{< api "Theme" >}}

{{< api-funcs pkg="ui" names="SetTheme" >}}

### NamedColor

| Field   | Type     | Description                                   |
|---------|----------|-----------------------------------------------|
| `Name`  | `string` | the Go identifier, for example `ColorAccent`  |
| `Color` | `Color`  | the semantic color itself                     |

{{< api "NamedColor" >}}

## Related

- [Theming](../../../concepts/theming/): light and dark themes, custom themes, switching at run time.
- [Modifiers](../../../concepts/modifiers/): `Background`, `Border` and `Shadow` take colors.
- [Shadow and border](../shadow-and-border/): the style values that carry a color.
- [Gradient](../gradient/) and [Glass](../glass/): backgrounds that are more than one color.
- [Custom views](../../../concepts/custom-views/): resolving colors in a view of your own.
- [Button](../../basic/button/) and [Badge](../../basic/badge/): components styled with semantic colors.
