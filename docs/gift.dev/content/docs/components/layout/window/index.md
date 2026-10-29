---
title: Window
---

`ui.Window` wraps the root view of your application in the window background. gift paints nothing you did not
ask for, and the backend hands every frame a transparent black screen. Without a background view, the gaps
between your views show whatever the framebuffer held: a composited hole on a desktop, the old console on a
kiosk. `ui.Window` fills the whole window with `ui.ColorBackground` and stacks your content on top of it.

Use it exactly once, as the outermost view of your root function. It is also where the
[on-screen keyboard](../../dialogs/keyboard/) goes.

{{< gallery-shot >}}

{{< gallery-code >}}

## Usage

**Return it from the root view.** The root function you pass to `gift.New` returns the window:

```go
func screen(ctx *gift.Context) gift.View {
	return ui.Window(ui.VStack(
		ui.Text("Hello"),
	).Padding(24))
}

app := gift.New(gift.Options{Root: screen})
```

The demo above frames and outlines the window only to show it as a miniature. In an application, leave the
frame off so the window background fills the whole window.

**It is a ZStack with a background plate.** `Window(content...)` builds a [ZStack](../../layout/zstack/) whose
first child is an unframed [Box](../../layout/box/) filled with `ui.ColorBackground`, followed by your
content in the order you pass it. An unframed box in a ZStack is greedy, so the background covers the whole
window even when your content does not. The result is an ordinary `Overlay`, so all of its modifiers apply.

**Align places the content.** `Align` positions every child inside the window. The zero alignment is the top
leading corner. A screen that fills the window is not moved by it; a smaller panel is, for example
`Align(geom.Center)` for a centered dialog-like screen.

**Page and surface.** `ColorBackground` is the page. [Cards](../../layout/card/), [Modal](../../dialogs/modal/)
sheets and the bars of a [TabBar](../../navigation/tab-bar/) or a
[NavigationStack](../../navigation/navigation-stack/) use `ColorSurface` and sit on that page. Do not fill a
whole screen with `ColorSurface`; that flattens the two levels. See [Colors](../../styling/colors/).

**The on-screen keyboard is a direct child.** Pass `ui.OnScreenKeyboard()` as the last child and align the
window to the bottom:

```go
return ui.Window(form, ui.OnScreenKeyboard()).Align(geom.Bottom)
```

The keyboard only appears when you turn it on with `ui.SetOnScreenKeyboard(app, true)` (it is off by default)
and a text field has the focus.
While it shows, the other children are laid out in the part of the window above it: scroll views get a shorter
viewport and a button at the bottom of the screen stays above the keys. When the keyboard goes away, they get
the whole window back. The background plate always keeps the full size, so the page is painted behind the
keyboard too.

**AvoidKeyboard turns that off.** `AvoidKeyboard(true)` is the default. `AvoidKeyboard(false)` lets the
keyboard cover the content instead. Use it only for a screen that must not re-flow while someone types, such
as a full screen camera preview with one field on top of it, never for a form.

```go
return ui.Window(preview, ui.OnScreenKeyboard()).Align(geom.Bottom).AvoidKeyboard(false)
```

Avoidance has two limits. The keyboard must be a direct child of the window; one wrapped in another container
is laid out like any other child. And only a vertical alignment at the top or bottom edge reserves space; a
centered keyboard reserves nothing.

**Put a Modal inside the window.** A [Modal](../../dialogs/modal/) goes inside the window, next to the
keyboard, so its scrim and sheet are laid out above the keyboard:

```go
ui.Window(ui.Modal(screen, sheet), ui.OnScreenKeyboard()).Align(geom.Bottom)
```

The other way round, with the keyboard inside the modal's content, puts the keyboard under the scrim, where no
key can be pressed.

**Test that the window is opaque.** Forgetting `ui.Window` is silent: nothing panics and every structural test
passes. `Harness.AssertOpaque` in package `gifttest` fails when any pixel of the framebuffer is not fully
opaque. See [Testing](../../../concepts/testing/).

## API

{{< api "Overlay" >}}

## Related

- [ZStack](../../layout/zstack/): the same `Overlay` without the background plate
- [Box](../../layout/box/): the greedy view that paints the background
- [Keyboard](../../dialogs/keyboard/): the on-screen keyboard and how it avoids content
- [Modal](../../dialogs/modal/): a dialog placed inside the window
- [Card](../../layout/card/): a surface raised on the page
- [Colors](../../styling/colors/): `ColorBackground`, `ColorSurface` and the other semantic colors
- [Theming](../../../concepts/theming/)
- [Kiosks](../../../concepts/kiosks/): the on-screen keyboard on a touch panel
- [Testing](../../../concepts/testing/): `Harness.AssertOpaque`
- [Counter example](../../../examples/example-counter/): a minimal application with a window
