---
title: Animation
weight: 11
---

In gift, an animation never rebuilds a view. Your view function declares where things are, for example "this
switch is on" or "this sheet is presented". When that declaration changes, gift remembers where the picture was,
and the painter works out every frame in between from a clock. No component runs again while something moves.

Most of the time you do not have to do anything: the controls, the navigation containers and the dialogs of
package `ui` animate by themselves. This page explains what they do, how to keep those animations working, how
to animate your own views, and how animations decide when the window is redrawn. The last part matters most on a
kiosk that should stay cool and quiet while nobody touches it.

## What animates by itself

| What                                                                               | Animation                                                         | Duration                    |
|------------------------------------------------------------------------------------|-------------------------------------------------------------------|-----------------------------|
| [Toggle](../../components/basic/toggle/)                                           | the knob travels to the new value                                 | `ui.ControlAnimation`       |
| [Segmented control](../../components/basic/segmented-control/)                     | the indicator slides to the new segment                           | `ui.ControlAnimation`       |
| [Tab bar](../../components/navigation/tab-bar/)                                    | the old tab slides out and the new one slides in, in index order  | `ui.ControlAnimation`       |
| [Navigation stack](../../components/navigation/navigation-stack/)                  | a pushed screen slides in from the trailing edge, the covered one slides out to the leading edge | `ui.ControlAnimation`       |
| [Sheet](../../components/dialogs/sheet/)                                           | the panel slides up from the bottom edge, the scrim fades         | `ui.ControlAnimation`       |
| [Modal](../../components/dialogs/modal/), [Alert](../../components/dialogs/alert/) | the dialog rises by about a third of its height, the scrim fades  | `ui.ControlAnimation`       |
| [Button](../../components/basic/button/) with `Lift(true)`                         | grows under the finger and springs back                           | 140 ms in, 420 ms out       |
| [Scroll view](../../components/layout/scroll-view/)                                | fling, rubber band at the ends, scroll indicator fades out        | until it settles            |
| [Text field](../../components/basic/text-field/)                                   | the caret blinks, then stays solid                                | `ui.CaretBlinkWindow` (10 s) after the last input |
| [Progress bar](../../components/basic/progress-bar/) with `Indeterminate()`        | a pill travels across the track, again and again                  | as long as it is mounted    |

`ui.ControlAnimation` is 180 ms. It is a constant, not a modifier: every control in a window answers at the same
speed, and there is no per-view setting for it.

The controls never animate when they first appear. A toggle that is built switched on is drawn switched on. Only
a change after that moves. The same holds for the containers: at start-up every tab and every screen is simply
there.

An animation that is interrupted turns round from where it is. Flick a switch twice quickly, and the knob reverses
in the middle instead of jumping to the end first.

## Keep a view mounted to animate it out

A view can only move while it has a node. When you remove it from the tree, its node is gone in the same build,
and there is nothing left to draw. So removing a view is always a cut. Appearing is different: a view that is
added to a window that is already on screen can slide in, because its node exists from that build on.

The dialogs therefore have two ways to close:

```go
func screen(ctx *gift.Context) gift.View {
	confirm := ctx.State("confirm", false)
	open := ctx.Read(confirm)

	content := ui.VStack(
		ui.Button(ui.Text("Delete"), func() { confirm.Set(true) }).Lift(true),
	)
	alert := ui.Alert("Delete?", "This cannot be undone.",
		ui.AlertCancel("Cancel", func() { confirm.Set(false) }),
	)
	return ui.Modal(content, alert).Presented(open)
}
```

`Presented(false)` keeps the alert mounted and hidden, so it slides out when it closes and back in when it opens
again. Passing `nil` instead of the alert unmounts it: it still slides in, but it disappears in one frame.
`ui.SheetView` has the same `Presented` modifier. A hidden subtree costs almost nothing per frame, but it keeps its
state and its nodes. For an alert that is a handful of nodes. For something large that you want gone, pass `nil`
and accept the cut.

The navigation stack has the same limit for a pop. The screen you pop is removed from your list of screens, so it
is unmounted and cannot slide out. Only the screen underneath moves: it comes back from a short distance on the
leading side.

## Lift on buttons

`Lift(true)` gives a button the press feedback of iOS 26. While a pointer holds the button down, the button grows
by 12 points along its long side and by six tenths of that across it, and a soft light spreads from where the
finger touches. On release it settles back with a small overshoot, like a spring.

```go
ui.Button(ui.Text("Save"), save).Style(accent).Lift(true)
```

Things to know:

- It is off by default. It moves the picture of the button, and a row in a list should not move.
- The button grows past its own bounds while it is pressed. Leave a little room around it; an ancestor with
  `Clip` cuts off the part that sticks out.
- It reacts to the pointer only. Activating a button with space or enter does not lift it.
- Layout does not change. The growth is a transform of what the button draws, so neighbors stay where they are,
  and the hit area is still the button's own bounds.

## Rubber band scrolling

A scroll container lets you pull the content past either end with growing resistance and springs back when you
let go. A fling that reaches an end with speed left bounces off it. All of this is a translation of the container;
nothing is built or laid out for it. To turn it off, so that the content stops dead at its ends:

```go
ui.VScroll(rows...).Bounce(false)
```

`Bounce(false)` sets `gift.ScrollConfig.HardStop`, which you can also set through `ScrollView.Config`. See
[Scroll view](../../components/layout/scroll-view/).

## Transitions in your own views

A **transition** turns a change of `Element.Hidden` into a movement. It is the mechanism behind the tab bar, the
navigation stack, the sheet and the modal, and you can use it in a custom view. You declare it with
[`gift.TransitionSpec`](https://pkg.go.dev/github.com/worldiety/gift#TransitionSpec) in the element your view
builds:

| Field      | Meaning                                                                                                     |
|------------|-------------------------------------------------------------------------------------------------------------|
| `Parked`   | where the node is while it is hidden, as a fraction of its own size: `{X: 1}` is one width to the right      |
| `Entry`    | where it comes from when it is mounted visible into a window that is already on screen; zero means `Parked`  |
| `Return`   | where it comes from when it is shown again after being hidden; zero means "from where it went"               |
| `Duration` | how long the movement takes; zero turns the transition off, whatever the other fields say                    |

This view slides its content up from below when it is shown and back down when it is hidden. It builds a
`ZStack` for the layout and adds the two fields to its element, which is how the modal of package `ui` does it:

```go
var slidePanelType = gift.RegisterType("app.slidePanel")

type slidePanel struct {
	hidden bool
	child  gift.View
}

func (p slidePanel) ViewType() gift.TypeID { return slidePanelType }

func (p slidePanel) Build(bc *gift.BuildContext) gift.Element {
	e := ui.ZStack(p.child).Build(bc)
	e.Hidden = p.hidden
	e.Transition = gift.TransitionSpec{
		Parked:   geom.Pt(0, 1), // one full height down
		Duration: ui.ControlAnimation,
	}
	return e
}

func withPanel(ctx *gift.Context) gift.View {
	show := ctx.State("show", false)
	return ui.ZStack(
		ui.Button(ui.Text("Details"), func() { show.Set(!show.Get()) }),
		slidePanel{hidden: !ctx.Read(show), child: ui.Text("Ships in two days.")},
	)
}
```

How a transition behaves:

- **It is a straight movement with ease-in-out.** The node is translated from where it is to where it is going.
  Nothing else changes: no fade, no scale. Two nodes that move at once read the same clock and stay exactly in
  step, which is how the outgoing and the incoming tab meet edge to edge.
- **It starts at the right end.** A node that is built hidden is parked from the start and does not move. A node
  that is built visible in the first frame of the application is simply there. Only a node that is mounted visible
  later, or whose `Hidden` changes, moves.
- **Interrupting it turns it round** from where it is, just like a control.
- **It remembers where it went.** A node that is hidden waits at the `Parked` value of the build that hid it, and
  comes back from there even if a later build declares a different `Parked`. That is what lets a tab bar compute
  the direction of every tab from the current selection alone. `Return` overrides this for a node that waited
  somewhere nobody could see it.
- **Input does not follow the picture.** A node that is being hidden takes no more taps from the first frame of its
  exit, although it is still drawn. A node that is appearing takes taps at its final position from the first
  frame, although it has not arrived there yet.
- **A hidden node keeps its place in the layout.** `Hidden` takes a node out of paint, hit testing and the focus
  order, not out of layout. Put a view that comes and goes on a layer of a `ZStack`, as above, and not between
  siblings in a stack.
- **It costs a full paint while it runs.** A node on its way out is drawn although it is hidden, until it has
  arrived. gift draws a moving node as a cached layer (see `Element.Layer`), so the backend composites a picture
  of the screen instead of drawing every card again; a subtree with glass in it gains nothing from that.
  Afterwards the hidden node costs nothing again.

A painter inside a transitioning subtree can read `PaintContext.TransitionPhase()`: 0 at rest, 1 fully parked. It
is how something that cannot move with the rest fades instead. The scrim of a modal covers the whole window and
therefore fades with `1 - TransitionPhase()` while the dialog rises.

Unlike the containers of package `ui`, a custom view that sets `Element.Hidden` takes on the bookkeeping that
comes with it: a hidden subtree stays mounted, keeps its state and is still rebuilt and laid out when its state
changes. See [Custom views](../custom-views/) for writing a view type and
[Navigation](../navigation/) for how the containers use `Hidden`.

## Animating a custom painter

Anything that is not a change of `Hidden` you animate in the painter. Two things make that possible, and gift
offers no more than these two on purpose:

- **A clock.** `Now()` on `EventContext`, `LayoutContext` and `PaintContext` returns the timestamp of the current
  frame on the clock the backend passes to `App.BeginInput`. It is the only clock a view may read: tests move it,
  and every painter in one frame sees the same value.
- **A repaint request.** `Animate(d)` keeps the node marked for repaint for the next `d`. It does not interpolate,
  ease or call anything back. It only makes sure that frames keep coming, and the painter decides what to draw
  from `Now()`.

What the animation remembers between frames goes into the node's
[`gift.ControlState`](https://pkg.go.dev/github.com/worldiety/gift#ControlState), not into the painter or the
interactor: those are fresh objects after every build. `ControlState` has the fields a typical animation needs:
`From`, `Target`, `Start` and `Armed`.

This square lights up when you press it and fades back over 400 ms:

```go
const flashDuration = 400 * time.Millisecond

var flashType = gift.RegisterType("app.flash")

type flash struct{}

func (flash) ViewType() gift.TypeID { return flashType }

func (flash) Build(bc *gift.BuildContext) gift.Element {
	e := ui.Box().Frame(64, 64).Build(bc)
	e.Painter = flashPainter{}
	e.Interactor = flashInput{}
	return e
}

type flashInput struct{}

func (flashInput) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	if e.Kind != gift.EventPointerDown {
		return false
	}
	st := ctx.ControlState()
	st.Armed, st.Start = true, ctx.Now()
	ctx.SetControlState(st) // marks the node for repaint
	ctx.Animate(flashDuration)
	return true
}

type flashPainter struct{}

func (flashPainter) Paint(ctx *gift.PaintContext) {
	k := float32(0) // 1 right after the press, 0 at rest
	if st := ctx.ControlState(); st.Armed {
		if d := ctx.Now() - st.Start; d < flashDuration {
			k = 1 - float32(d)/float32(flashDuration)
		}
	}
	ctx.Add(render.Op{
		Kind:         render.OpFillRoundRect,
		Bounds:       ctx.Bounds(),
		CornerRadius: 12,
		Color:        render.RGBA(0, 122, 255, uint8(80+175*k)),
	})
}
```

The interactor writes, the painter only reads. A painter must not change any state; it is a pure function of the
retained state and the clock. Once the window is over, the painter has to draw the end state exactly, as this one
does with `k = 0`, so that the last frame stays correct after the repaints stop.

### The rules of `Animate`

- **`d` is a deadline, not a duration to add.** Calling `Animate(d)` again before the window ends moves the end to
  `now + d`; it does not queue a second window. A handler may call it on every event without counting.
- **A `d` of zero or less cancels.** One more repaint still happens, so the frame in which the animation ends is
  drawn. The text field does this when it loses the focus.
- **There is always an end.** An animation that never expires keeps the backend at its full tick rate for good. An
  animation whose end is unknown re-arms itself, or stops.
- **Hidden nodes do not animate.** Hiding a subtree ends every enrolment inside it, and a node under a hidden
  ancestor cannot start a new one. Unmounting a node drops its enrolment too.

Which context you call it on depends on what started the animation:

| Call                       | Use it when                                                                                       |
|----------------------------|---------------------------------------------------------------------------------------------------|
| `EventContext.Animate`     | an event started it: a press, a key, a focus change                                               |
| `LayoutContext.Animate`    | a new value from a build started it, for example a toggle the application switched from a timer   |
| `PaintContext.Animate`     | it has no end at all, like an indeterminate progress bar, and has to re-arm from the frame it drew |

A value that arrives through a build reaches no event handler. The layouter is the place that sees both the new
declaration and the retained `ControlState`, so that is where a control compares them, stores the new target with
`LayoutContext.SetControlState` and calls `LayoutContext.Animate`. A painter may not write `ControlState`.

`PaintContext.Animate` never repaints the frame it is called from, only the following ones. A node that calls it
in every frame keeps the backend at its full rate for as long as it is mounted, even when it is scrolled out of
view or covered by a sheet, because gift paints every mounted node that is not hidden.

For a one-off change to presentation state that is not an animation, `EventContext.Repaint()` marks the node
without starting a window. `SetControlState` already marks the node when the value changed.

## Repaints, idle and DrawOnDemand

`App.NeedsPaint()` reports whether anything changed since the last paint. A state write, a layout, a hover or a
press sets it, and so does every running animation, transition, fling and caret blink in every frame of its
window. When the last window has closed, it stays false and the application is idle.

By default this flag is informational: the ebiten backend draws every frame. Two settings of
[`ebiten.Config`](https://pkg.go.dev/github.com/worldiety/gift/backend/ebiten#Config) build on it:

```go
backend.Run(app, backend.Config{
	Title:        "Kiosk",
	IdleTPS:      10,
	DrawOnDemand: true,
})
```

- **`IdleTPS`** lowers the tick rate once `IdleFrames` updates in a row (60 by default, about a second) did not
  set `NeedsPaint`. The next update that does raises it again.
- **`DrawOnDemand`** skips drawing a frame when nothing changed and leaves the previous frame on the screen. A
  frame is drawn when `NeedsPaint` is set, when the renderer still has picture uploads to finish, when the window
  changed size, and for the first two frames.

Both are off by default. Kiosks and appliances want both on: a screen that shows the same thing for an hour then
costs almost nothing.

With `DrawOnDemand`, `NeedsPaint` is load-bearing, and that has one consequence for your painters: **nothing may
change on screen without a repaint request.** A painter that reads `time.Now()`, a value from another goroutine,
or anything else gift does not know about, shows its old picture until something unrelated causes a frame. With
`DrawOnDemand` off, the same bug is hidden by sixty redraws a second. The rules that avoid it:

- Read time only through `Now()` and keep the frames coming with `Animate`.
- Bring new data in through state, written on the UI goroutine; see [Concurrency](../concurrency/).
- Call `EventContext.Repaint()` after changing presentation state of your own from an interactor.

The opposite mistake keeps the machine awake. An enrolment that keeps re-arming holds the full tick rate and
defeats both settings. The usual culprit is an indeterminate progress bar left mounted after the work finished.
`App.Diagnostics()` reports `Animations`, the number of enrolments in force, and `Animating`; if `Animating` stays
true on a screen that looks still, something is still asking. See [Diagnostics](../diagnostics/).

## Testing animations

Animations run on the injected clock, so a test steps through them without sleeping. `gifttest.Harness.Advance`
moves the clock forward and settles, and `Diagnostics().Animating` tells you whether anything is still moving:

```go
func TestFlashGoesIdle(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{View: ui.Window(flash{})})
	h.TapAt(geom.Pt(10, 10))
	if !h.Diagnostics().Animating {
		t.Fatal("the press should start the flash")
	}
	h.Advance(flashDuration)
	if h.Diagnostics().Animating {
		t.Fatal("the flash should be over")
	}
}
```

The automation interface of package `auto` reports the same two values as `animating` and `animations`, so an
external driver can wait for motion to stop instead of guessing frame counts. See [Testing](../testing/).

## Related

- [Custom views](../custom-views/): writing a view type with its own painter and interactor.
- [Navigation](../navigation/): tab bars, navigation stacks and how `Hidden` keeps screens alive.
- [Kiosks](../kiosks/): running unattended with `IdleTPS` and `DrawOnDemand`.
- [Button](../../components/basic/button/), [Sheet](../../components/dialogs/sheet/),
  [Modal](../../components/dialogs/modal/), [Progress bar](../../components/basic/progress-bar/).
- [Kitchen sink example](../../examples/example-kitchensink/): tabs, a navigation stack, sheets and alerts in
  motion.
- [`gift.TransitionSpec`](https://pkg.go.dev/github.com/worldiety/gift#TransitionSpec) and
  [`gift.EventContext.Animate`](https://pkg.go.dev/github.com/worldiety/gift#EventContext.Animate) on pkg.go.dev
  for the full reasoning.
