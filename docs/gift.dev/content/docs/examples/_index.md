---
title: Examples
weight: 4
sidebar:
  open: false
---

Each example is a small, runnable gift program from
[`cmd`](https://github.com/worldiety/gift/tree/main/cmd). Its page shows what it does, how to run it and its complete
source code.

{{< cards >}}
  {{< card link="example-counter/" title="Counter" image="example-counter/screenshot-light.png" subtitle="State, buttons, a scroll view and a theme switch in one file. Start here." >}}
  {{< card link="example-kitchensink/" title="Kitchen Sink" image="example-kitchensink/screenshot-light.png" subtitle="Most components in one window: four tabs, navigation, controls, lists, a form and dialogs." >}}
  {{< card link="example-gallery/" title="Image Gallery" image="example-gallery/screenshot-light.png" subtitle="A photo browser with the full image pipeline: files, HTTP, decode errors and a disk cache." >}}
  {{< card link="example-effects/" title="Effects" image="example-effects/screenshot-light.png" subtitle="Glass over a scrolling gallery, and cards with borders and shadows." >}}
  {{< card link="example-photos/" title="Photos" image="example-photos/screenshot-light.png" subtitle="A gallery of large photos and a full screen viewer to swipe, pinch and drag away." >}}
  {{< card link="example-video/" title="Video" image="example-video/screenshot-light.png" subtitle="A video with sound and touch controls, sized for a five inch panel." >}}
  {{< card link="example-typst/" title="Typst" image="example-typst/screenshot-light.png" subtitle="A document set by Typst, edited in its source and on its pages, and saved as a PDF." >}}
  {{< card link="example-kiosk/" title="Kiosk" image="example-kiosk/screenshot-light.png" subtitle="A program as the operating system of its device, with Wi-Fi, Ethernet, updates and a passcode in settings after iOS." >}}
  {{< card link="example-drop/" title="Drop" image="example-drop/screenshot-light.png" subtitle="Files, text and web addresses dragged in from other applications." >}}
{{< /cards >}}

## Run an example

You need the Go version from `go.mod` and a graphical session; see [Installation](../getting-started/installation/).
From a checkout of the [repository](https://github.com/worldiety/gift), run:

```bash
go run ./cmd/example-counter
```

Replace `example-counter` with the directory of any other example.
