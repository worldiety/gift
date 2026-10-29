---
title: Counter
weight: 1
---

The counter is the first gift program to read. It fits in one file and shows the core loop of the toolkit: a view
function reads a piece of [state](../../concepts/state/), a [button](../../components/basic/button/) writes it, and
gift rebuilds the view. The `-` and **Reset** buttons are disabled while the count is zero. Below them, a
[scroll view](../../components/layout/scroll-view/) holds forty rows, and an arrow marks the row that matches the
count.

The example is also the [theming](../../concepts/theming/) demo. Every color in it is a semantic color such as
`ui.ColorSurface` or `ui.ColorControl`. The accent button next to the title calls `ui.SetTheme` and switches the whole
window between the light and the dark theme at runtime. No other line changes: the same view function produces both
appearances.

{{< gallery-shot alt="The counter at 3 clicks, with the minus, plus and Reset buttons and a scrolling list of rows" >}}

## What to try

- Click `+` and `-`. The number and the arrow in the list follow the count.
- Press <kbd>Tab</kbd> to move the focus to a button, then press <kbd>Space</kbd> or <kbd>Enter</kbd> to activate it.
  See [Input and Focus](../../concepts/input-and-focus/).
- Scroll the list with the mouse wheel, drag it and let go to fling it, or drag the thumb of the scroll bar.
- Click **Dark** or **Light** to switch the theme. The label shows the theme you switch to next.
- Hover over and press the buttons. The `-`, `+` and **Reset** buttons change their background for the hovered and
  pressed state, and `-` and **Reset** have a style for the disabled state. The theme button keeps its accent color
  and shows its state with a ring instead.

## Run it

From the repository root:

```sh
go run ./cmd/example-counter
```

The program takes no flags. Two environment settings change what it does:

- `GIFT_FONT` names a font file. The example then uses that typeface instead of Inter, so you can see the layout
  under another font. If the file cannot be read or parsed, the program exits with an error. See
  [Text and Fonts](../../concepts/text-and-fonts/).
- `GIFT_METRICS=1` together with the `giftmetrics` build tag prints a performance report to standard output when
  the program exits. See [Diagnostics](../../concepts/diagnostics/).

```sh
GIFT_METRICS=1 go run -tags giftmetrics ./cmd/example-counter
```

## How it is built

- `ctx.State("count", 0)` creates the state, and `ctx.Read(count)` reads it and makes the view depend on it. The
  button closures call `count.Set`, which rebuilds the view. See [State](../../concepts/state/).
- [`ui.Window`](../../components/layout/window/) fills the window with `ui.ColorBackground`, and its `Align` call
  centers the panel. The panel is a [`ui.VStack`](../../components/layout/stack/) with a background, a corner
  radius and a [border](../../components/styling/shadow-and-border/).
- The `-`, `+` and **Reset** buttons share four `ui.ButtonStyle` values for their states. They are package
  variables that hold semantic colors, and they still follow every later theme switch. See [Colors](../../components/styling/colors/).
- `ui.VScroll` gets a fixed `Frame(300, 96)`. Without it, the stack would give the scroll view as much height as its
  content needs, and there would be nothing to scroll. See [Layout](../../concepts/layout/).
- The theme button reads `ui.CurrentTheme().IsDark()` for its label instead of keeping its own state. Its text uses
  `ui.ColorOnAccent`, which stays legible on the accent color in both themes.

## Source

{{< example-code >}}
