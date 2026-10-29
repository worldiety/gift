---
title: Effects
---

The effects example puts gift's drawing effects on one screen. A masonry gallery of 4000 colored placeholder tiles
fills the window. A glass panel floats across the top of it, and a row of four cards sits along the bottom. The
window is always dark, in light and dark system themes alike.

The cards show borders and shadows. Each card is the same rounded rectangle with a 1-pixel border. From left to
right, the shadows are: none, a blur of 8, a blur of 24 with an offset of 8 down, and a blur of 20 with an offset
of 10 down and a spread of -6. The negative spread shrinks the shadow's shape, so it tucks in under the card and
shows mostly below it. The backend draws every shape and shadow analytically in its shape shader, without an
offscreen image.

The panel shows the experimental glass material. It sits over the gallery as a sibling in a `ZStack`, not inside
it, so there is one glass region however many tiles lie under it. Scroll the gallery with the wheel or by dragging,
and watch the backdrop follow. A glass panel that never moves is still redrawn every frame while the content behind
it moves.

{{< gallery-shot alt="A glass panel with quality buttons over a colorful tile gallery, and four dark cards with different shadows at the bottom" >}}

The screenshot is taken with `-quality=full`, so the panel blurs the tiles behind it.

## What to try

- Scroll the gallery under the panel and compare how the tiles look through the glass with how they look beside it.
- Click **Reduced** and **Full** to pin a quality level. Click **Adaptive** to hand the choice back to the backend.
  The line under the title shows the effective level and whether it is pinned or chosen by measurement. It
  follows the adaptive policy: twice a second the example compares the level in effect with the one shown and
  rebuilds the panel when they differ. The log on stderr reports those switches as well.
- On **Reduced** the backdrop is not blurred. The panel reads as a surface through the thin dark and bright lines
  along its rim and the slight bend of the backdrop at the edge.
- Compare the four cards along the bottom to see what blur, offset and spread each contribute.

## Running it

Run it from the repository root:

```sh
go run ./cmd/example-effects
go run ./cmd/example-effects -quality=full
```

The example takes two flags:

| Flag | Default | Meaning |
|---|---|---|
| `-quality` | `adaptive` | The glass quality to pin at start: `adaptive`, `reduced` or `full`. |
| `-count` | `4000` | The number of placeholder tiles behind the panel. |

The flag pins the level in the renderer through `backend.Config.GlassQuality`, and the buttons in the panel start
on the same level, so with `-quality=full` **Full** is highlighted and the line under the title says the level is
pinned.

Under `adaptive`, the backend starts at `Full` and chooses between `Reduced` and `Full` from the measured frame
intervals and from how much of the screen the glass covers. The example passes an `slog` logger to the backend, so
each change the adaptive policy makes shows up on stderr: a warning when quality drops to `Reduced` and an info
line when it returns to `Full`. Pinning a level, by flag or by button, logs nothing.

To see frame metrics as well, build with the `giftmetrics` tag and set `GIFT_METRICS`:

```sh
GIFT_METRICS=1 go run -tags giftmetrics ./cmd/example-effects
```

## Source

{{< example-code >}}

## Related

- [Glass](../../components/styling/glass/) explains the material and its quality levels.
- [Shadow and border](../../components/styling/shadow-and-border/) covers `Shadow`, `Border` and `CornerRadius`.
- [Image gallery](../../components/lists/image-gallery/) is the scrolling backdrop.
- [ZStack](../../components/layout/zstack/) layers the panel and the cards over the gallery.
- [Button](../../components/basic/button/) styles the three quality buttons.
- [Modifiers](../../concepts/modifiers/) and [State](../../concepts/state/) describe the building blocks the example
  uses.
- [Diagnostics](../../concepts/diagnostics/) covers logging and metrics.
- [Gallery](../example-gallery/) shows the same gallery with real pictures.
