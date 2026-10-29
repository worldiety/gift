---
title: Kitchen Sink
weight: 2
---

The kitchen sink puts most of gift into one window. Four tabs run along the bottom: a navigation stack you can drill
into, a settings list with every control, a long list next to a virtual list, and a form with a text field, the
on-screen keyboard, a sheet, a file picker and an alert. A button in the top right corner of the Home tab switches
the whole window between the light and the dark theme. The views name semantic colors such as `ui.ColorAccent` and
`ui.ColorSecondaryLabel`, and the current theme resolves them, so the switch passes no color to any view.

Each tab is a [`gift.Component`](../../concepts/custom-views/) with a rebuild scope of its own, and it reads only the
state slots it shows. A flip of a toggle on the Settings tab therefore rebuilds that tab and not the other three.
An inactive tab stays mounted, so a slot that two tabs read rebuilds both: *Nudge it along* writes the progress,
which the Home tab reads for its Details screen. The comment on `Screen` in the source explains the measurement behind that
choice.

{{< gallery-shot alt="The Home tab of the kitchen sink: a heading with a theme switch, a card with badges, a list of rows inside a card, a strip of pictures, a strip of icons and the tab bar" >}}

## What to try

- **Home**: tap *Details* to push a screen onto the navigation stack, then *Go deeper* to push another one. Go back
  with the back button in the navigation bar. The escape key pops a screen too, also when nothing holds the
  focus, but not while the alert is open. Tap *Reset everything* to open a modal alert. The *Unavailable* row is
  disabled and ignores taps. Below the rows, five pictures load through the image pipeline, and a strip of twenty
  icons scrolls sideways.
- **Settings**: flip the toggles, drag the volume slider, pick a quality in the segmented control. *Nudge it along*
  advances the progress bar by ten percent, and *Busy* switches an indeterminate progress bar on and off.
- **List**: scroll two hundred rows built as an ordinary `ui.List`, or switch the segmented control to a
  `ui.VirtualList` with one hundred thousand rows. A tap on a row writes a line that the Form tab shows.
- **Form**: tap the text field. The on-screen keyboard comes up because the program turns it on with
  `ui.SetOnScreenKeyboard(nil, true)`, and the rest of the window is laid out above it. Press enter or *Save* to
  greet the name you typed. *Show a sheet* slides up a sheet, *Pick files* opens the file picker in a sheet,
  starting in your home directory.
- **Theme**: tap *Dark* or *Light* in the top right corner of the Home tab.

## Run it

From the repository root:

```bash
go run ./cmd/example-kitchensink
```

On first start, the program writes five small sample PNGs into `gift/kitchensink` below your user cache directory
and shows them on the Home tab. The window opens at 900 × 760 logical pixels.

The `-icons` flag prints the icon cache counters once per tick for the first one and a half seconds: drawn frames,
and icon masks rasterized, uploaded and resident. It shows how a screen with many new icons fills in over a few frames,
because gift uploads only a limited number of icon masks per drawn frame.

```bash
go run ./cmd/example-kitchensink -icons
```

To get frame timings, build with the `giftmetrics` tag and set `GIFT_METRICS`, as described in
[Diagnostics](../../concepts/diagnostics/):

```bash
GIFT_METRICS=1 go run -tags giftmetrics ./cmd/example-kitchensink
```

## Code

{{< example-code >}}

## Related

Concepts: [Views](../../concepts/views/), [Modifiers](../../concepts/modifiers/), [State](../../concepts/state/),
[Layout](../../concepts/layout/), [Theming](../../concepts/theming/), [Text and fonts](../../concepts/text-and-fonts/),
[Icons](../../concepts/icons/), [Images](../../concepts/images/), [Input and focus](../../concepts/input-and-focus/),
[Navigation](../../concepts/navigation/), [Custom views](../../concepts/custom-views/),
[Kiosks](../../concepts/kiosks/), [Testing](../../concepts/testing/).

Components used on the screen:

- Basic: [Text](../../components/basic/text/), [Button](../../components/basic/button/),
  [Icon](../../components/basic/icon/), [Image](../../components/basic/image/),
  [Badge](../../components/basic/badge/), [Divider](../../components/basic/divider/),
  [Toggle](../../components/basic/toggle/), [Slider](../../components/basic/slider/),
  [Segmented control](../../components/basic/segmented-control/),
  [Progress bar](../../components/basic/progress-bar/), [Text field](../../components/basic/text-field/)
- Layout: [Window](../../components/layout/window/), [Stack](../../components/layout/stack/),
  [Spacer](../../components/layout/spacer/), [Scroll view](../../components/layout/scroll-view/),
  [Card](../../components/layout/card/)
- Lists: [List](../../components/lists/list/), [Row](../../components/lists/row/),
  [Virtual list](../../components/lists/virtual-list/)
- Navigation: [Navigation stack](../../components/navigation/navigation-stack/),
  [Tab bar](../../components/navigation/tab-bar/)
- Dialogs: [Sheet](../../components/dialogs/sheet/), [Modal](../../components/dialogs/modal/),
  [Alert](../../components/dialogs/alert/), [Keyboard](../../components/dialogs/keyboard/),
  [File picker](../../components/dialogs/file-picker/)
- Styling: [Colors](../../components/styling/colors/)
