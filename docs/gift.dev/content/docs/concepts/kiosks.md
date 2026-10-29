---
title: Kiosks and Raspberry Pi
weight: 14
---

gift is built for touchscreen kiosks and appliances as much as for the desktop. The target hardware is a Raspberry
Pi 4 or 5 behind a fixed screen: a photo booth, a terminal on a factory floor, a control panel in a housing. Such a
device shows the same screen for hours, runs without a desktop around it, and often has no hardware keyboard.

Nothing about a kiosk application is a different mode of gift. You write the same views. What changes is how you
configure the window and the frame loop in `backend.Config`, and which of gift's own replacements for desktop
services you place in the window: the [on-screen keyboard](../../components/dialogs/keyboard/) and the
[file picker](../../components/dialogs/file-picker/).

## A kiosk main

This is the shape of `main` for a kiosk application. Every line is explained in the sections below.

```go
package main

import (
	"fmt"
	"os"

	"github.com/worldiety/gift"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func main() {
	ui.SetDefaultFont(ui.MustFont(ui.FontQuery{Family: inter.Family}))
	ui.SetOnScreenKeyboard(nil, true)

	app := gift.New(gift.Options{Root: screen})
	err := backend.Run(app, backend.Config{
		Title:           "Photo booth",
		Width:           800,
		Height:          480,
		X11ClassName:    "photobooth",
		X11InstanceName: "photobooth",
		DrawOnDemand:    true,
		IdleTPS:         10,
		GlassQuality:    ui.Reduced,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "photobooth:", err)
		os.Exit(1)
	}
}

func screen(ctx *gift.Context) gift.View {
	return ui.Window(form(ctx), ui.OnScreenKeyboard()).Align(geom.Bottom)
}
```

`form` stands for your own view function. The font is set explicitly, as in every gift application; see
[Text and fonts](../text-and-fonts/).

## The frame loop

`backend.Run` drives the app with Ebitengine. Ebitengine calls two functions: Update on every *tick*, and Draw on
every display refresh. gift reads input, runs posted work, builds and lays out in Update, and paints the display list
in Draw. On a 60 Hz display both run sixty times a second, whether anything changed or not.

On a desktop that costs little. On a Raspberry Pi in a closed housing it is heat, and heat leads to thermal
throttling. Three fields of `backend.Config` take the load off. All three are off by default.

### DrawOnDemand

`DrawOnDemand: true` skips drawing a frame when nothing on the screen changed. The previous frame stays on the
screen. A frame is drawn only when

- the tree changed since the last paint (`gift.App.NeedsPaint`). A state write, a layout change, a hover or a press
  marks it, and so does every running animation, a blinking caret and a control transition,
- the renderer still has picture uploads that its per-frame budget deferred,
- the window changed size, or the frame is one of the first two.

Every other refresh is skipped. On an idle screen no frame is drawn at all until something changes. Ticks still run
at the full rate; that is what `IdleTPS` is for.

The flag relies on one rule: **nothing may appear on the screen without asking for a paint.** gift's own views keep
that rule. Your code keeps it as long as it changes the screen through state:

```go
// clock updates the time from a goroutine. Set ignores an unchanged value,
// so only a new minute marks the tree and draws a frame.
func clock(app *gift.App, now *gift.State[string]) {
	for t := range time.Tick(time.Second) {
		s := t.Format("15:04")
		app.Post(func() { now.Set(s) })
	}
}
```

A [custom painter](../custom-views/) that reads the clock itself must enroll with `PaintContext.Animate`. One that
changes its output silently shows the old frame until something else repaints. Without `DrawOnDemand` such a bug is
hidden by the next frame a sixtieth of a second later; with it, the bug becomes visible.

{{< callout type="warning" >}}
Anything that animates without end holds the backend at full rate for as long as it is mounted. The prime example is
an indeterminate [progress bar](../../components/basic/progress-bar/), `ui.ProgressBar(0).Indeterminate()`: it
re-arms its repaint from its own painter, also while it is scrolled out of view. Remove it from the tree when the
work is done.
{{< /callout >}}

### IdleTPS and IdleFrames

`IdleTPS` lowers the tick rate once `IdleFrames` consecutive ticks changed nothing. `IdleFrames` defaults to sixty,
about one second at the default rate. The first tick that changes something switches back to `TPS` at once.

Keep in mind what a slow tick means. Input is polled and posted work is run once per tick. At `IdleTPS: 10` the
first touch after a quiet period is noticed up to 100 ms later, and a closure passed to `App.Post` waits up to the
same time. The ticks after it run at the full rate again. Pick a value that still feels responsive on your device.

`IdleTPS` and `DrawOnDemand` are independent and meant to be combined: one lowers the update rate, the other skips
the drawing.

### DirectToScreen

Ebitengine hands Draw an offscreen image and copies it onto the real framebuffer afterwards. On a Raspberry Pi at
1920x1080 that copy is a full-screen pass of its own: measured on a Pi 400, one fill through the offscreen took
22.1 ms, the same fill straight into the final screen 14.8 ms.

`DirectToScreen: true` skips the copy and draws the display list into the final screen. gift still takes the
offscreen path for a frame whose offscreen and framebuffer differ in size, such as a letterboxed window, and for a
frame that the automation interface captures, because the framebuffer cannot be read back. It works together with
`DrawOnDemand`.

Measure on the device before and after you turn it on. The gain is the copy, so it matters most at large screen
sizes.

### TPS

`TPS` is the tick rate, the number of Update calls per second. Zero selects Ebitengine's default of sixty. It does
not set the frame rate: Ebitengine draws as often as the display and the driver allow. You rarely need to change it.

## Glass quality

The experimental [glass](../../components/styling/glass/) material has two levels. `ui.Reduced` is one composite pass
over an unblurred copy of the backdrop. `ui.Full` adds a blur chain. The default, `ui.Adaptive`, lets the backend
switch between them from the measured frame intervals and the glass area on screen.

`GlassQuality` in `backend.Config` pins one level for the whole run. A pinned level also overrides the level a single
material asks for with `ui.Glass().Quality(...)`. Pin a level on a device whose performance you know, so that the
look does not change part way through a session. A Raspberry Pi 4 may not reach the frame time target with `Full`;
`Reduced` is the safe choice there. Pin a level as well for any measurement you want to compare with another.

## The window

**Size.** `Width` and `Height` are the initial window size in logical pixels. Zero or less selects 1280x720. Set
them to the size of your panel, for example 800x480.

**Full screen and cursor.** `backend.Config` has no field for either. Call Ebitengine's own functions before
`backend.Run`. `cmd/gift-glassbench` calls `SetFullscreen` this way for its `-fullscreen` flag:

```go
import eb "github.com/hajimehoshi/ebiten/v2"

func fullscreen() {
	eb.SetFullscreen(true)
	eb.SetCursorMode(eb.CursorModeHidden)
}
```

This is the one place where a gift application imports Ebitengine directly.

**WM_CLASS.** `X11ClassName` and `X11InstanceName` set the class and instance names of the window on X11, including
XWayland. Empty selects Ebitengine's defaults, `Ebitengine-Application` and `ebitengine-application`. Desktop shells
such as GNOME and KDE match a window to its desktop entry by this class. With the defaults every gift application
carries the same class, so two of them are grouped as one in the panel. Set both to a name unique to your
application and put the class into the `StartupWMClass` key of its `.desktop` file.

**NoInput.** `NoInput: true` disables the input bridge: the window reads no mouse, keyboard or touch input. It exists
for measurement runs that a resting cursor must not disturb, and for a test harness that sends events into
`gift.App` itself. A kiosk application leaves it off.

**Density.** gift reads the device scale factor of the monitor and rounds it to a whole number. On a display that
reports 1, one logical pixel is one screen pixel.

## Raspberry Pi OS

The intended setup is **Raspberry Pi OS 64-bit** on a Raspberry Pi 4 or 5, with an X11 or XWayland session and
hardware-accelerated Mesa. gift needs a graphical session with GPU support.

**No CGO.** gift builds with `CGO_ENABLED=0`. You cross-compile the whole application on your development machine
and copy one binary to the Pi:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o photobooth ./cmd/photobooth
scp photobooth pi@kiosk.local:
```

The optional parts that talk to system libraries keep that property. They load the library at run time through
purego, and they degrade gracefully without it:

- **Clipboard.** Import `github.com/worldiety/gift/clipboard` for its side effect to exchange text with other
  programs. On Linux it needs libX11 and an X11 or XWayland session. Without the import, copy and paste work inside
  the process only.
- **Faster JPEG decoding.** For galleries of camera photos, install `libturbojpeg0` on the Pi and register the
  decoder explicitly. Without the library, `Register` returns false and `image/jpeg` stays in charge. See
  [Images](../images/).

```go
if turbojpeg.Register() { // github.com/worldiety/gift/asset/turbojpeg
	asset.RegisterDecoder(asset.MIMEJPEG, turbojpeg.Decoder{})
}
```

**Touch arrives as a mouse.** Under X11 and XWayland a touchscreen reports to Ebitengine as a mouse. gift's
controls and scroll views handle that: a finger taps a button and drag-scrolls a list. But gift cannot tell a kiosk
from a desktop by the input, which is why the on-screen keyboard has an explicit switch.

**Performance.** 1080p at 60 Hz on a Raspberry Pi is a target, not a verified guarantee. Measure on your device
with the `giftmetrics` build tag and `GIFT_METRICS=1`; see [Diagnostics](../diagnostics/). If you pass a `Logger` in `backend.Config`,
`backend.Run` logs on exit how many frames it drew and how many refreshes `DrawOnDemand` skipped. On an idle kiosk
the skipped refreshes outnumber the drawn frames by far; if they do not, something keeps marking the tree.

## Kiosk features

### On-screen keyboard

Ebitengine cannot raise a native keyboard, and a kiosk has none to raise. gift draws its own: a German layout with
umlauts and ß, a digit row and a symbol page. Its keys send the same events as a physical keyboard.

It takes two steps. Turn it on once in `main` with `ui.SetOnScreenKeyboard(nil, true)`. Place
`ui.OnScreenKeyboard()` as a direct child of the [Window](../../components/layout/window/) and align the window to
the bottom, as in the `main` above. From then on the keyboard comes up when a
[text field](../../components/basic/text-field/) takes the focus and goes away when it loses it. While it shows, the
other children of the window are laid out in the space above it, so a focused field and a submit button stay
visible.

The switch is off by default, because a keyboard that pops up on a developer's desktop is worse than none. You can tie
it to a command line flag that you set on the device only.

{{< api-funcs pkg="ui" names="SetOnScreenKeyboard,OnScreenKeyboardEnabled,OnScreenKeyboardHeight" >}}

The [Keyboard](../../components/dialogs/keyboard/) page covers placement, modals and the cases in which the
keyboard covers content.

### File picker

There is no native file dialog on a full-screen kiosk. `ui/filepicker` is a picker made of gift views. It opens one
or several files, chooses a folder or picks a path to save to, and works with a finger as well as with a mouse. It
reads through an `fs.FS` that you choose, so the application decides what may be browsed:

```go
ui.Sheet(screen, filepicker.New(os.DirFS("/media/usb"), open, cancel).
	Mode(filepicker.OpenMulti).
	Extensions(".jpg", ".png")).
	Presented(picking).
	OnDismiss(cancel)
```

`os.DirFS("/media/usb")` exposes the USB stick and nothing else. The picker never writes, not even in save mode; it
only reports the path. Directories are read off the UI goroutine and shown in a
[virtual list](../../components/lists/virtual-list/), so large folders scroll smoothly. See
[File picker](../../components/dialogs/file-picker/).

## Pitfalls

- **Benchmarks.** `DrawOnDemand`, `IdleTPS` and adaptive glass change what a frame-time measurement means. A
  benchmark of a still scene with `DrawOnDemand` takes the draw time only of the frames that were drawn, but a
  frame interval for every display refresh. State the settings with
  every number, and pin `GlassQuality`.
- **Silent painters.** With `DrawOnDemand`, a painter that changes its output without asking for a paint shows a
  stale frame. Change the screen through state, or enroll with `PaintContext.Animate`.
- **Endless animations.** An indeterminate progress bar or any view that re-arms its animation every frame keeps
  the device at full rate and full draw.
- **Keyboard placement.** The keyboard must be a *direct* child of the window or a [ZStack](../../components/layout/zstack/).
  Wrapped in another container, the window cannot recognize it and does not make room for it.
- **Automation in production.** Never ship a kiosk binary built with `-tags giftauto`. It opens an unauthenticated
  local interface for input and screen capture. See [Testing](../testing/).

## Related

- [Keyboard](../../components/dialogs/keyboard/), [File picker](../../components/dialogs/file-picker/),
  [Glass](../../components/styling/glass/)
- [Input and focus](../input-and-focus/), [Animation](../animation/), [Concurrency](../concurrency/),
  [Diagnostics](../diagnostics/)
- [example-gallery](../../examples/example-gallery/) for an image-heavy kiosk screen,
  [example-effects](../../examples/example-effects/) for glass quality levels
- [`backend/ebiten` on pkg.go.dev](https://pkg.go.dev/github.com/worldiety/gift/backend/ebiten#Config) for every
  field of `Config`
