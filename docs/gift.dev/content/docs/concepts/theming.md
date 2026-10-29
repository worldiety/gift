---
title: Theming
weight: 5
---

A gift color is either *literal* or *semantic*. A literal color is a fixed value such as `ui.RGB(200, 30, 40)`. A
semantic color names the role a color plays, such as "primary text" or "raised panel", and asks the theme for the
actual value. The theme is one process-wide table. Swap it, and every semantic color in the window changes with it.

This is how gift does light and dark mode. The same view function produces both appearances, because it never
names a concrete color for anything that should follow the theme.

## Semantic and literal colors

Every color field and every color modifier takes a [`ui.Color`](../../components/styling/colors/). The `Background`
modifier takes a `ui.Background`, and a `Color` is one. A semantic color is an ordinary `Color` value, so you write
it wherever you would write `ui.RGB`:

```go
func panel() gift.View {
	return ui.VStack(
		ui.Text("Settings"),
		ui.Text("Changes apply at once.").Foreground(ui.ColorSecondaryLabel),
	).Gap(8).Padding(16).
		Background(ui.ColorSurface).
		CornerRadius(12).
		Border(ui.Border{Width: 1, Color: ui.ColorSeparator})
}
```

A literal color stays the same under every theme:

```go
ui.Text("Always red").Foreground(ui.RGB(200, 30, 40))
```

Use semantic colors for the interface itself: text, panels, hairlines, controls. Use literal colors for things that
carry their own meaning independent of light or dark, such as a brand logo or a chart series. Both kinds mix freely
in one view tree.

### The roles

There are twelve roles. Each is named for what it is *for*, not for what it looks like.

| Color                  | Role                                                       | Default of                                              |
|------------------------|------------------------------------------------------------|---------------------------------------------------------|
| `ColorLabel`           | primary text                                               | [Text](../../components/basic/text/), [Icon](../../components/basic/icon/) |
| `ColorSecondaryLabel`  | supporting text: captions, units, hints                    | the placeholder of a [TextField](../../components/basic/text-field/) |
| `ColorBackground`      | the page behind everything                                 | [Window](../../components/layout/window/)               |
| `ColorSurface`         | a panel raised above the page                              | [Card](../../components/layout/card/), [Sheet](../../components/dialogs/sheet/), the bars of [TabBar](../../components/navigation/tab-bar/) and [NavigationStack](../../components/navigation/navigation-stack/) |
| `ColorSeparator`       | a hairline: border, divider, edge of a control             | [Divider](../../components/basic/divider/)              |
| `ColorAccent`          | what carries attention: primary control, selection, focus ring | [Badge](../../components/basic/badge/), the focus ring of a button |
| `ColorOnAccent`        | content drawn on `ColorAccent`                             | the text of a [Badge](../../components/basic/badge/)    |
| `ColorControl`         | the face of a control at rest                              | [Button](../../components/basic/button/), [TextField](../../components/basic/text-field/), the track of a [Slider](../../components/basic/slider/) |
| `ColorControlHover`    | the face under the pointer                                 | [Button](../../components/basic/button/)                |
| `ColorControlPressed`  | the face while pressed                                     | [Button](../../components/basic/button/)                |
| `ColorControlDisabled` | the face of a disabled control                             | [Button](../../components/basic/button/)                |
| `ColorDanger`          | a choice that destroys something                           | `AlertDestructive` in an [Alert](../../components/dialogs/alert/) |

`ColorBackground` and `ColorSurface` form a hierarchy. The page is `ColorBackground`, and what sits on it is
`ColorSurface`. If you fill the whole window with `ColorSurface`, you flatten that hierarchy. A control wears
`ColorControl`, not `ColorSurface`.

`ColorOnAccent` is not just the label color. On the light theme's blue it is white; on the dark theme's lighter
blue it is near black. Use it for text and icons on any accent fill.

The [Colors](../../components/styling/colors/) page shows every role in both built-in themes.

### Fading a role

`ui.Fade(c, f)` returns `c` at `f` times its opacity. On a semantic color the result is still semantic: the fade is
stored and applied after the theme lookup. That is how you express emphasis without inventing a role.

```go
// Package level and unresolved: follows every theme switch.
var track = ui.Fade(ui.ColorLabel, 0.08)
```

`f` must be between 0 and 1, otherwise `Fade` panics.

## Light, dark and the current theme

gift ships two themes, `ui.LightTheme()` and `ui.DarkTheme()`. The light theme is installed by default.
`ui.CurrentTheme()` returns the theme in force, and `Theme.IsDark` tells you whether it describes a dark appearance.

`IsDark` is a flag, not a guess from the colors. `DarkTheme` sets it, `Theme.With` keeps it, and `Theme.Dark(v)`
sets it on a palette you built yourself. Use it for decisions a color cannot express, such as which of two images
to show or which label a switch carries.

### Choosing a theme at startup

Call `ui.SetTheme` with a nil app before you create the application. That installs the theme and repaints nothing,
which is all you need before the first frame:

```go
func main() {
	// load and install a font first, see Text and fonts
	ui.SetTheme(nil, ui.DarkTheme())
	app = gift.New(gift.Options{Root: screen})
	// backend.Run(app, ...)
}
```

gift does not follow the appearance setting of the desktop on its own. If you want that, watch the setting yourself
and switch at runtime, as described next.

### Switching at runtime

`ui.SetTheme(app, t)` installs `t` and rebuilds every component in the application. The rebuild is what puts the
new colors on screen: gift resolves semantic colors when a view is built and stores the literal result in the
retained node. Until the views are built again, nothing changes.

That is why `SetTheme` takes the app. It calls `App.InvalidateAll` rather than `App.Invalidate`, so that a
`gift.Memo` whose props did not change is rebuilt too. Without that, a memoized subtree would keep the
old palette and the window would end up half light, half dark. The price is a rebuild of every component instance
once per switch. That is fine for a button the user presses; do not call it per frame.

A view function receives a `*gift.Context`, which has no handle to the application. A program with one window
therefore keeps the app in a package variable, as [example-counter](../../examples/example-counter/) does. This is
its theme switch:

```go
var app *gift.App

var hair = ui.Border{Width: 1, Color: ui.ColorSeparator}

func themeSwitch() ui.ButtonView {
	dark := ui.CurrentTheme().IsDark()
	label, next := "Dark", ui.DarkTheme()
	if dark {
		label, next = "Light", ui.LightTheme()
	}
	face := func(ring float32) ui.ButtonStyle {
		s := ui.ButtonStyle{Background: ui.ColorAccent, Border: hair, CornerRadius: 9}
		if ring > 0 {
			s.Border = ui.Border{Width: ring, Color: ui.ColorOnAccent}
		}
		return s
	}
	return ui.Button(ui.Text(label).FontSize(13).Foreground(ui.ColorOnAccent), func() {
		ui.SetTheme(app, next)
	}).
		Style(face(0)).
		HoverStyle(face(1)).
		PressedStyle(face(2)).
		PaddingInsets(geom.Insets{Top: 7, Right: 14, Bottom: 7, Left: 14})
}
```

Two details are worth copying:

- The label comes from `ui.CurrentTheme()`, not from a [state](../state/) slot. The theme is global, and a second
  copy of "are we dark" in state could disagree with it.
- The button is filled with `ColorAccent` and its label is `ColorOnAccent`, so it stays legible in both themes. The
  hover and pressed states change the ring, not the face: fading the accent would lower the contrast of the label.

Nothing else in example-counter knows about the theme. No color is passed down and no view is styled twice. The
[kitchen sink](../../examples/example-kitchensink/) carries the same switch, with an icon, on its overview screen.

### Switching from another goroutine

The theme itself is stored atomically, so a build always sees either the old or the new palette. The rebuild that
`SetTheme` requests is not safe to start from another goroutine, though. If a goroutine that is not the UI goroutine
decides to switch, for example one that watches the desktop appearance, it posts the call:

```go
func watch(changes <-chan bool) {
	go func() {
		for dark := range changes {
			t := ui.LightTheme()
			if dark {
				t = ui.DarkTheme()
			}
			app.Post(func() { ui.SetTheme(app, t) })
		}
	}()
}
```

Calling `SetTheme` with a non-nil app from another goroutine is a data race. With the `giftdebug` build tag, it
panics instead. See
[Concurrency](../concurrency/) for `App.Post` and the rest of the threading rules.

## Changing roles with Theme.With

A `Theme` is a value. `Theme.With(semantic, value)` returns a copy in which one role resolves to a literal color of
your choice. Chain it to change several roles, then install the result:

```go
func brand() {
	orange := ui.RGB(255, 149, 0)
	ui.SetTheme(app, ui.CurrentTheme().
		With(ui.ColorAccent, orange).
		With(ui.ColorOnAccent, ui.RGB(0, 0, 0)))
}
```

This restyles every view that uses those roles, in every widget, without touching a single view function. It is
also the only way to change the default look of a built-in widget for the whole application: there is no per-widget
default you could assign to.

When you change a role, check its partner. A new `ColorAccent` usually needs a matching `ColorOnAccent`, and a
derived dark theme should start from `DarkTheme()` so that `IsDark` stays true.

`With` panics if you misuse it:

- the first argument is not one of the named semantic colors,
- the value is itself semantic (a theme does not chain one role to another),
- the role is `ColorClear`, which is transparency and not part of any theme.

`Theme.Color(c)` returns the literal color a theme assigns to a role, without installing the theme.
`ui.SemanticColors()` lists every role a theme assigns, with its Go name, which is what a theme editor or a contrast
check of your palette needs:

```go
for _, c := range ui.SemanticColors() {
	fmt.Println(c.Name, ui.CurrentTheme().Color(c.Color))
}
```

## Zero means "the theme decides"

In [`ButtonStyle`](../../components/basic/button/), the zero `Color` does not mean transparent. It means "not set,
use the themed default". It lets a theme show through a style you only partly specified. `ui.ScrollBar` does the
same for its `Track` and `Thumb`: a transparent value falls back to `ui.DefaultScrollBar`.

```go
// Themed pressed face, no border, corner radius 10.
ui.Button(ui.Text("Save"), save).
	PressedStyle(ui.ButtonStyle{CornerRadius: 10})
```

When you really want nothing drawn, say so with `ui.ColorClear`. It resolves to full transparency under every
theme:

```go
// No face in any state. Border(ui.Border{}) also drops the themed hairline.
ui.Button(ui.Text("Cancel"), cancel).
	Style(ui.ButtonStyle{Background: ui.ColorClear}).
	Border(ui.Border{})
```

Without the `Border` call, the button keeps its themed hairline. The focus ring stays in either case; remove it with
`FocusRing(ui.Border{})`.

`ButtonStyle.Border` follows the same idea one level down: a width without a color is stroked in `ColorSeparator`.
This rule belongs to `ButtonStyle` alone. The `.Border` modifier of every view, including `ButtonView.Border`,
leaves a border without a color invisible.

The color modifiers of views work differently from the struct fields. A modifier you never call leaves the themed
default in place; a modifier you do call sets exactly the value you pass. So `ui.Text("x")` draws in
`ColorLabel`, while `ui.Text("x").Foreground(ui.Color{})` draws nothing.

## Pitfalls

{{< callout type="warning" >}}
An unresolved semantic color is invisible, not wrong.
{{< /callout >}}

A semantic color is encoded with an alpha of zero. Every view in gift resolves its colors with `ui.ResolveColor`
in Build, so you never notice. But if a semantic color reaches the display list unresolved, it is fully
transparent. Painters skip transparent work, so the widget lays out, measures and hit-tests correctly and draws
nothing. There is no wrong pixel to spot and, in a release build, no error.

This only concerns you when you write a [custom view](../custom-views/) with its own painter:

- Resolve every color with `ui.ResolveColor` during Build and store the literal result in your node, the way the
  built-in views do. `ResolveColor` returns literal colors unchanged, so calling it on every color is safe.
- If you test `ui.IsSemantic` yourself, do it where you decide whether to draw, not where you append the
  operation.
- Build with the `giftdebug` tag while you develop. The built-in views then panic when an unresolved semantic
  color reaches their painters, and the display list rejects impossible colors. See
  [Diagnostics](../diagnostics/).

The second pitfall is resolving too early. Keep semantic colors unresolved until Build:

```go
// Right: follows every theme switch.
var track = ui.Fade(ui.ColorLabel, 0.08)

// Wrong: resolved once, at initialization, under the light theme.
var frozen = ui.ResolveColor(ui.ColorLabel)
```

The same applies to `Theme.Color` and to any helper of your own that turns a role into a literal value. A view
value, on the other hand, may be built at any time: `ui.Box().Background(ui.ColorSurface)` stores the role and
resolves it when the view is built.

Finally, the theme only changes what you let it change. A literal color stays literal under every theme, and
no built-in view except [`ui.Window`](../../components/layout/window/) paints `ColorBackground`: gift paints no window
background of its own. The examples [example-gallery](../../examples/example-gallery/) and
[example-effects](../../examples/example-effects/) keep their own literal palettes on purpose, which is the other
half of the design: the theme is the default, not a policy.

Do not assign to `ui.ColorLabel` and the other role variables. They are variables only because Go has no struct
constants. Change the theme instead.

## API

{{< api "Theme" >}}

{{< api-funcs pkg="ui" names="SetTheme,IsSemantic" >}}

{{< api "Color" >}}

{{< api "NamedColor" >}}

## Related

- [Colors](../../components/styling/colors/): every role in both themes
- [Button](../../components/basic/button/): `ButtonStyle` and the state styles
- [Window](../../components/layout/window/): paints `ColorBackground`
- [Custom views](../custom-views/): resolving colors in your own painter
- [example-counter](../../examples/example-counter/): the theme switch in context
