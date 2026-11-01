---
title: Kitchen Sink
weight: 2
---

The kitchen sink shows every component of gift in one window. A list grouped like these pages, Basic, Layout, Lists,
Navigation, Dialogs & Overlays, Styling, Media and Documents, then System, runs down the left; beside it stands the
component you picked, with its name, what it does, and its demo, which is live: tap it, drag it, type into it.

{{< gallery-shot alt="The kitchen sink: a list of every component grouped like the documentation, with Color Picker selected, and beside it the colour picker" >}}

The demos are the very ones the component pages of this documentation are drawn from. They live in
[`internal/showcase`](https://github.com/worldiety/gift/tree/main/internal/showcase), one file per component, and
the docs, their screenshots and the kitchen sink all use them. A component with a page is therefore in the kitchen
sink, and in there as its page shows it.

A section of its own, System, follows the components with what is no component: the
[notifications](../../concepts/notifications/) of the operating system, shown, replaced and withdrawn with the
text you type, and a count of the clicks on them.

## What to try

- **Pick a component** in the list. Each demo is a component of its own: leaving it unmounts it with its state,
  and coming back starts it afresh. The video demo closes its player on the way out.
- **Search**: type into the search field above the list. It matches names and descriptions, so *slider* finds the
  slider and the circular slider, and *date* the calendar and the wheels.
- **Narrow the window** below 900 points, or start it at the size of a five inch panel. The list becomes the root
  of a [navigation stack](../../components/navigation/navigation-stack/), and a component is pushed onto it, with a
  back button and the escape key to go back.
- **Dark appearance**: the moon button switches the whole window between light and dark. The views name semantic
  colors such as `ui.ColorAccent`, and the theme resolves them, so the switch passes no color to any view.
- **On-screen keyboard**: the keyboard button turns on the keyboard of a kiosk without a hardware keyboard; every
  text field of every demo then brings it up. `-keyboard` starts with it on.
- **Notifications**: under System, *Show or replace* sends the text of the two fields to the notification center;
  showing it again replaces it, and a click on it counts. On macOS the kitchen sink has to run as an app bundle for
  that, see below; elsewhere the status line says why nothing appears.

## Run it

From the repository root:

```bash
go run ./cmd/example-kitchensink
go run ./cmd/example-kitchensink -show color-picker            # start with this component
go run ./cmd/example-kitchensink -width 800 -height 480 -keyboard   # a five inch kiosk
```

macOS shows notifications only for an app bundle, which `giftbundle` makes:

```bash
go install ./cmd/giftbundle
go run -exec "giftbundle -id dev.gift.kitchensink" ./cmd/example-kitchensink -show notifications
```

On first start, the program writes a dozen small sample pictures and a three second video clip into
`gift/kitchensink` below your user cache directory, for the image and video demos. The window opens at 1180 × 800
logical pixels.

## How it is built

The list and the component are two panes of one layouter, which also notices when the window crosses the width at
which the kitchen sink switches between side by side and the navigation stack, and reports it once through a timer
event; a layout may not write state, an event may. Each demo is a [`gift.Component`](../../concepts/custom-views/)
keyed by its name, and the video demo registers `ctx.OnUnmount` to close its player when that component goes.

## Code

{{< example-code >}}
