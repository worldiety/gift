---
title: Testing and Automation
weight: 15
---

gift gives you two tools to check an application without a person in front of it. The package
[`gifttest`](https://pkg.go.dev/github.com/worldiety/gift/gifttest) runs your views inside a `go test`: it
mounts them in a headless application, finds nodes by what they say, clicks them and asserts what happened. The
package [`auto`](https://github.com/worldiety/gift/tree/main/auto) does the same from the outside: built with
the tag `giftauto`, a running application answers HTTP requests on loopback, so a script can read its tree,
send input and take screenshots of the real window.

This page explains both, the three build tags that switch on the heavier parts, and the pitfalls that make a UI
test lie.

| Tool              | Runs                                   | Use it for                                          |
|-------------------|----------------------------------------|-----------------------------------------------------|
| `gifttest`        | in `go test`, no window, no GPU        | behavior: state, input, focus, layout, overflow     |
| `gifttest` + `giftgpu` | in `go test`, renders offscreen   | golden images and pixel assertions                  |
| `auto` + `giftauto`    | inside the running application    | screenshots, driving the real window from a script  |

## A first test

Take a counter like the one on the [state page](../state/), with a minus button that is disabled at zero and a
key on the label and on each button:

```go
func counter(ctx *gift.Context) gift.View {
	count := ctx.State("count", 0)
	n := ctx.Read(count)

	return ui.Window(ui.VStack(
		ui.Text(strconv.Itoa(n)).Key("count").FontSize(32),
		ui.HStack(
			ui.Button(ui.Text("-"), func() { count.Set(count.Get() - 1) }).
				Key("minus").Frame(56, 44).Disabled(n == 0),
			ui.Button(ui.Text("+"), func() { count.Set(count.Get() + 1) }).
				Key("plus").Frame(56, 44),
		).Gap(8),
	).Gap(16).Padding(24))
}
```

The test mounts it, clicks the plus button and checks the label:

```go
import (
	"testing"

	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

var font = ui.MustFont(ui.FontQuery{Family: inter.Family})

func TestCounter(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{Root: counter, Font: font})

	h.Find(gifttest.ByKey("count")).AssertText("0")
	h.Find(gifttest.ByKey("minus")).AssertDisabled()

	h.Find(gifttest.ByText("+")).Click()
	h.Find(gifttest.ByKey("count")).AssertText("1")
	h.Find(gifttest.ByKey("minus")).AssertEnabled()
}
```

The test names no coordinate, no frame and no timer. If you restyle the counter or move the buttons, it keeps
passing. `gifttest.New` builds, lays out and paints the first frame before it returns, and every action settles
the application before the next line runs.

{{< callout type="warning" >}}
**Every test needs a font.** gift links no typeface into a program that does not ask for one, and a
[Text](../../components/basic/text/) without a font panics on its first build. Set `Options.Font`, as above, or
install a default font in your own setup. See [Text and Fonts](../text-and-fonts/).
{{< /callout >}}

### Options

`gifttest.Options` configures the harness:

| Field       | Meaning                                                                                      |
|-------------|----------------------------------------------------------------------------------------------|
| `Root`      | the root component function, as in `gift.Options.Root`                                       |
| `View`      | a fixed view for a test without state of its own; set exactly one of `Root` and `View`       |
| `Size`      | the viewport in logical pixels; zero means `gifttest.DefaultSize`, 800 × 600                 |
| `Font`      | the default font for the test; the previous one is restored when the test ends              |
| `Theme`     | the color theme for the test, for example `ui.DarkTheme()`; the zero value keeps the current one |
| `Density`   | the device density, rounded like a real monitor's; zero means 1                              |
| `MaxFrames` | the bound of `Settle`; zero means `gifttest.DefaultMaxFrames`, 64                            |
| `Logger`    | passed to gift; nil means no logging                                                         |

The harness clears every frame to transparent black, like a real window. It paints no background for you. Wrap
a component under test in [`ui.Window`](../../components/layout/window/) when the background matters, for
example `gifttest.Options{View: ui.Window(card)}`.

## Selectors

A selector is a predicate over the nodes of the retained tree. These match on what a user or the application
can name:

| Selector               | Matches a node                                                          |
|------------------------|-------------------------------------------------------------------------|
| `ByText(s)`            | whose label is exactly `s`                                              |
| `ByTextContains(sub)`  | whose label contains `sub`                                              |
| `ByKey(k)`             | whose key, set with `.Key(k)`, is `k`                                    |
| `ByType(name)`         | built from the view type registered as `name`, such as `"ui.Button"`    |
| `Interactive()`        | that takes part in hit testing; a disabled control still does           |
| `Focusable()`          | that can take the keyboard focus right now                              |
| `Enabled()`            | that is not disabled                                                    |
| `Visible()`            | that is not hidden and not clipped away entirely                        |
| `Under(parent)`        | below a node that matches `parent`                                      |
| `Where(desc, pred)`    | for which your own predicate over a `Node` is true                      |
| `Any()`                | every node                                                              |

Combine them with `.And`, `.Or` and `Not`. `Under` picks one row out of several identical ones:

```go
h.AssertCount(gifttest.ByText("Delete"), 3)
h.Find(gifttest.Under(gifttest.ByKey("row-3")).And(gifttest.ByText("Delete"))).Click()
```

The label `ByText` reads is the one the view declares, not the rasterized pixels. A
[Text](../../components/basic/text/) declares its string. An icon-only [Button](../../components/basic/button/)
has no string anywhere, so give it one with `.Label("Close")` and the test finds it like a text button.

Four calls run a selector:

- `h.Find(s)` returns the one node that matches, and fails unless there is exactly one.
- `h.First(s)` returns the first match in document order, and fails if there is none. Use it when several matches
  are expected and the first is meant.
- `h.FindAll(s)` returns every match; an empty result is not a failure.
- `h.Exists(s)` reports whether anything matches, without failing.

`Node.Find(s)` does the same as `h.Find`, but only within the subtree of that node.

When a selector matches too much, the failure prints the query, the matches and the tree with the matches
marked. When it matches nothing, it lists the labels, keys or types that do exist, so a typo stands out. Call
`h.Dump()` yourself to get the tree as text at any point. It shows the type, key, label, layout bounds and
input flags of every node, up to 120 lines:

```text
the tree was:
   gift.Component key="root" bounds=(0,0)-(106.578125,90)
     ui.VStack bounds=(0,0)-(106.578125,90)
       ui.HStack key="row-1" bounds=(0,0)-(103.78125,30)
         ui.Text text="row-1" bounds=(0,0)-(37.234375,18)
         ui.Button bounds=(37.234375,0)-(103.78125,30) interactive
           ui.Text text="Delete" bounds=(49.234375,6)-(91.78125,24)
       ui.HStack key="row-2" bounds=(0,30)-(106.15625,60)
       ...
```

Prefer `ByKey` when a test has to be certain. A key is under your control and survives a change of wording.
It only has to be unique among siblings, though, so combine it with `ByType` or `Under` when the same key
appears in several places.

## Actions

Actions are methods of `Node`. The harness works out where the node is; you never write the point.

| Action                        | What it does                                                          |
|-------------------------------|-----------------------------------------------------------------------|
| `Click()`                     | moves the mouse onto the node, presses and releases                   |
| `Press()`, `Release()`        | the two halves of a click, for "press here, release there"            |
| `Hover()`                     | moves the mouse onto the node                                         |
| `Tap()`                       | a touch: finger down and up, with no hover                            |
| `LongPress()`                 | holds a finger past `gift.LongPressDelay`                             |
| `Drag(dst)`, `DragTo(p)`      | presses, moves in several steps, releases                             |
| `Swipe(d)`                    | a drag with a finger                                                  |
| `Fling(d, over)`              | a finger drag that takes `over` on the injected clock, so it flings   |
| `Wheel(d)`                    | scrolls the mouse wheel over the node                                 |
| `Focus()`                     | moves the keyboard focus to the node; fails if it cannot take it      |
| `PressKey(k, mods...)`        | focuses the node and presses one key                                  |

A `ByText` selector usually lands on the text *inside* a button. The actions climb to the nearest interactive
ancestor, so `h.Find(gifttest.ByText("+")).Click()` clicks the button. `Wheel` is the exception: it scrolls at
the center of the node itself, and gift passes the wheel event up to the nearest scroll container.

The keyboard lives on the harness: `h.Tab()` and `h.ShiftTab()` move the focus, `h.Key(k, mods...)` presses a
physical key, and `h.TypeText(s)` delivers characters to whatever holds the focus. `Key(gift.KeySpace)` and
`TypeText(" ")` are different things: the first presses the space bar, the second types a space character.

```go
func TestCounterKeyboard(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{Root: counter, Font: font})

	h.Tab()
	h.AssertFocus(gifttest.ByKey("plus"))
	h.Key(gift.KeySpace)
	h.Find(gifttest.ByKey("count")).AssertText("1")
}
```

A [TextField](../../components/basic/text-field/) is driven the same way: focus it, type, check what your
application did with the text.

```go
func greeting(ctx *gift.Context) gift.View {
	name := ctx.State("name", "")
	ed := ui.Editor(ctx, "name-editor", "")
	return ui.Window(ui.VStack(
		ui.TextField(ed).Bind(name.Binding()).Key("name").Placeholder("Your name"),
		ui.Text("Hello, "+ctx.Read(name)).Key("greeting"),
	).Gap(12).Padding(24))
}

func TestGreeting(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{Root: greeting, Font: font})

	h.Find(gifttest.ByKey("name")).Focus()
	h.TypeText("Ada")
	h.Find(gifttest.ByKey("greeting")).AssertText("Hello, Ada")

	h.Key(gift.KeyBackspace)
	h.Find(gifttest.ByKey("greeting")).AssertText("Hello, Ad")
}
```

### Every action checks where it landed

Before `Click`, `Press`, `Hover`, `Tap`, `LongPress`, `Drag`, `DragTo`, `Swipe` or `Fling` dispatches its
event, it hit tests the point it aims at. If the hit test reaches a different node, because an overlay, a later
[ZStack](../../components/layout/zstack/) sibling or a full-size [Box](../../components/layout/box/) covers the
target, the action fails and names both nodes. It also fails if the point reaches no interactive node at all.
`Release` and `Wheel` do not check: a release goes to whatever captured the press anyway. Without this check
the covering control would get the click, and the test would fail later with a misleading message about the
counter.

Sometimes the overlap is what you want to test, for example that a [modal](../../components/dialogs/modal/)
swallows clicks to the screen behind it. Use the coordinate verbs for that: `h.ClickAt(p)`, `h.TapAt(p)`,
`h.PressAt(p)`, `h.ReleaseAt(p)` and `h.MoveTo(p)` dispatch at a point without a check, and `h.At(p)` returns
the interactive node a click at `p` would reach, without dispatching anything.

### Nodes go stale

A `Node` holds a reference into the retained tree. When a rebuild replaces the subtree that contained it, the
reference goes stale, and every method on it fails with a message that says so. Find the node again after an
action that rebuilds it. Holding on to a node across actions is fine as long as it survives, as `plus` does here:

```go
plus := h.Find(gifttest.ByText("+"))
plus.Click()
plus.Click()
```

## Assertions

Node assertions return the node, so you can chain them:

| Assertion                                             | Fails unless the node                                    |
|-------------------------------------------------------|----------------------------------------------------------|
| `AssertText`, `AssertKey`, `AssertType`               | has that label, key or view type                         |
| `AssertEnabled`, `AssertDisabled`                     | is enabled or disabled                                   |
| `AssertFocused`, `AssertHovered`, `AssertPressed`     | has that interaction state; each has an `AssertNot…` twin |
| `AssertBounds`, `AssertSize`                          | has that rectangle or size, within half a pixel          |
| `AssertVisible`, `AssertNotVisible`                   | survives, or does not survive, the clips above it        |
| `AssertDrawsGlyphs`, `AssertDrawsImage`               | drew text or a picture in the last frame                 |
| `AssertBackground`, `AssertPaints`                    | filled its shape, or painted an operation, in that color |
| `AssertMaterial`, `AssertGlassQuality`                | painted a material of that kind, or a [glass](../../components/styling/glass/) of that quality |
| `AssertScrollOffset`, `AssertAtScrollStart`, `AssertAtScrollEnd` | has its scroll container at that position     |

Harness assertions are about the whole scene: `AssertExists`, `AssertNone` and `AssertCount` take a selector,
`AssertFocus(s)` and `AssertNoFocus()` check the keyboard focus, and `AssertOps` counts display list operations
that satisfy a predicate.

The state assertions, `AssertEnabled` to `AssertNotPressed`, read the nearest interactive node, the node itself or
an ancestor, like the actions do. `h.Find(gifttest.ByText("+")).AssertDisabled()` checks the button around the
label; a failure then names the ancestor whose state was read.

`AssertNoOverflow` fails when any node's content does not fit the size it reported, and prints the tree. It is
the one line every layout test should end with. Combined with `Resize`, it checks a screen at several window
sizes without a second application. `Clip(true)` does not make it pass: a clip only hides the excess on screen,
and the overflow is still reported. Give the container more room or make the content need less.

```go
func TestCounterFitsSmallWindow(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{Root: counter, Size: geom.Sz(320, 240), Font: font})
	h.AssertNoOverflow()

	h.Resize(geom.Sz(200, 160))
	h.AssertNoOverflow()
}
```

See [Layout](../layout/) for what overflow means in gift and [Diagnostics](../diagnostics/) for the counters
behind it.

### Gone or hidden

The tree walk does not stop at hidden subtrees. An inactive tab keeps its views mounted, and a dismissed
[alert](../../components/dialogs/alert/) stays in the tree while it animates out. So "the dialog is closed" is
not `AssertNone(ByText(...))`. Ask whether it is still on screen:

```go
h.AssertNone(gifttest.ByText("Reset everything?").And(gifttest.Visible()))
```

## Frames and time

You rarely think about frames. `New` and every action call `h.Settle()`, which runs frames until one of them
builds nothing and lays out nothing. If the application never settles within `MaxFrames`, the test fails with
the number of builds in the last frame and the tree. The usual cause is a component whose build writes a state
that the same build reads.

The clock is injected and starts at zero. Nothing in `gifttest` sleeps or reads the wall clock. Move the clock
with `h.Advance(d)`; `LongPress` does this for you. `h.AdvanceTicks(n, step)` moves it in `n` steps, which is
what key repeat needs, because gift delivers a bounded number of repeats per tick. When the number of frames is
the subject of the test, call `h.Frame()` to run exactly one.

## Scrolling

Selectors find a node inside a [ScrollView](../../components/layout/scroll-view/) even when it is far outside
the viewport. The pointer actions that aim scroll it into view first, so a test does not have to. An assertion
does not scroll: call `ScrollIntoView` before `AssertVisible`. For a test
about scrolling itself, use `ScrollIntoView`, `ScrollTo`, `ScrollBy`, `Fling` and the scroll assertions.
`Node.Bounds` is the rectangle on screen, after scroll offsets; `Node.LayoutBounds` is the one the layout
computed.

## Golden images

Everything above runs on the display list, without a GPU. Pixel assertions need real pixels, so they compile
against the build tag `giftgpu`:

- `h.AssertGolden(name)` renders the current frame offscreen and compares it with `testdata/<name>.png`.
- `h.AssertOpaque()` fails when any pixel of the frame is not fully opaque.
- `h.AssertPixel(p, c)` checks one pixel of the rendered image, within the golden tolerance.
- `h.Image()` returns the rendered frame. Without the tag it fails the test instead of skipping.

A golden test pins the font and the theme, and the test package installs `gifttest.Main` as its `TestMain`:

```go
func TestMain(m *testing.M) { gifttest.Main(m) }

func TestCounterLooks(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{
		Root:  counter,
		Size:  geom.Sz(240, 180),
		Theme: ui.LightTheme(),
		Font:  ui.MustFont(ui.FontQuery{Family: inter.Family}),
	})
	h.AssertOpaque()
	h.AssertGolden("counter-at-zero")

	h.Find(gifttest.ByText("+")).Click()
	h.AssertGolden("counter-at-one")
}
```

Run it with the tag. The first run has no reference image yet; it fails and writes what it rendered, so you can
look at it before you accept it:

```bash
GIFT_UPDATE_GOLDEN=1 go test -tags giftgpu ./...   # write or rewrite testdata/*.png
go test -tags giftgpu ./...                        # compare
```

{{% steps %}}

### Without the tag, golden assertions skip

The same test file compiles in both modes. Under a plain `go test ./...` `AssertGolden`, `AssertOpaque` and
`AssertPixel` skip the test with a message that names the tag, and `gifttest.Main` is an ordinary `m.Run`. Set `GIFT_REQUIRE_GOLDEN=1` in
a CI job that is supposed to check rendering, and a skip becomes a failure.

### With the tag, `TestMain` is required

Ebitengine only reads pixels back inside its run loop. `gifttest.Main` runs the tests inside that loop; without
it, `AssertGolden` panics. Because it starts Ebitengine, a `giftgpu` run needs a machine with a display.

### A mismatch writes two files

Next to the reference, the failing run writes `<name>.actual.png` and `<name>.diff.png` and prints all three
paths. The diff is the reference dimmed, with every differing pixel in magenta. Slashes and spaces in the name
become underscores.

{{% /steps %}}

The comparison allows `gifttest.GoldenTolerance`, 4 of 255, per channel, to absorb differences between GPU
drivers. There is no percentage of pixels that may differ, so a shape that moves by one pixel fails. A rewrite
run logs `GOLDEN REWRITTEN` for every file and verifies nothing.

The image is the viewport times `Options.Density` in physical pixels, so a test at density 2 checks the
high-resolution glyphs and thumbnails. `AssertPixel` reads the pixel at `p` in that image, so at a density
other than 1 its point is in physical pixels. If the frame shows an [Image](../../components/basic/image/), call
`h.Warm()` before the golden: the frame that first draws a picture only requests its upload, the texture is
ready from the next frame on, and without a warm-up frame the golden would show the placeholder.

### Assert that the window is opaque

gift paints only what your views describe, and a real window starts every frame transparent. An application
without a background ships a window with holes in it. A golden of such a window is perfectly stable, so it
cannot catch the defect. `AssertOpaque` can. It fails with the number of holes and the first one, and the fix
is to wrap the root view in [`ui.Window`](../../components/layout/window/).

## Pitfalls

- **Do not call `t.Parallel`** in a test that uses `gifttest`. The text shaping cache, the default font and the
  theme are process wide, so two parallel tests race on them. Tests in different packages run in separate
  processes and are not affected.
- **Pin the font and the theme** in every test whose result depends on glyph shapes or colors, with
  `Options.Font` and `Options.Theme`. Otherwise the result depends on what another test in the package installed
  first. If a test changes the default font itself, restore it with `t.Cleanup`, not `defer`, so the harness and
  your test restore in the right order.
- **A `Harness` belongs to one goroutine**, like the `gift.App` inside it. `h.App()` gives you the application
  for anything the harness has no verb for.
- **The harness has one touch pointer** and no input method editor; multitouch and CJK composition are out of
  scope. Umlauts, accents and dead keys arrive as finished characters and work with `TypeText`.

## Automating the real application

`gifttest` tests your views. The `auto` package tests the program you ship: a real window, a real frame loop,
a real framebuffer. Build the application with the tag `giftauto` and the Ebitengine backend starts an HTTP
server in its `Run`, just before it opens the window. You add no import and no source file:

```bash
go run -tags giftauto ./cmd/example-kitchensink
# in a second terminal:
curl -fsS http://127.0.0.1:7391/tree
curl -fsS 'http://127.0.0.1:7391/screenshot?settle=8' -o screenshot.png
```

{{< callout type="error" >}}
**Never ship a build with `giftauto`.** The server has no authentication, can synthesize any input and can read
back every pixel. It answers on loopback only and refuses every other peer with `403`, but it does not belong
on a kiosk. Without the tag, the package compiles to nothing: no import, no goroutine, no server.
{{< /callout >}}

The server listens on `127.0.0.1:7391`. `GIFT_AUTO_ADDR` changes the address, and `GIFT_AUTO_TIMEOUT` changes
the time, in milliseconds, that a request waits for the UI goroutine or a frame (default two seconds). A
request that times out because nothing is being drawn answers `503`.

| Route              | Answers with                                                                         |
|--------------------|--------------------------------------------------------------------------------------|
| `GET /health`      | liveness and frame counters, without waiting for the UI goroutine                    |
| `GET /tree`        | the retained tree as JSON: type, key, text, `bounds`, `centre`, visibility, input flags, overflow, scroll state; `?depth=n` limits it |
| `GET /query`       | the matching nodes as a flat list; filters `key`, `text`, `contains`, `type`, `visible=1`, `interactive=1`, `limit` |
| `GET /screenshot`  | a PNG of the framebuffer the window drew; `?settle=n` waits for `n` drawn frames first |
| `POST /input`      | runs a batch of input steps in order and answers with frame counters                 |
| `GET /diag`        | the counters of `gift.Diagnostics`, the overflowing nodes, the theme, the focused node, viewport and density |
| `POST /theme`      | installs a theme: `{"mode":"dark"}`, or single roles such as `{"roles":{"accent":[255,0,255,255]}}` |

Every request except `/health` runs its work on the UI goroutine and waits for it, so a shell script that sends one request
after another is sequential, not racy. A screenshot shows a frame drawn after the input before it.

### Coordinates

All coordinates in and out of the interface are logical pixels, the same space as `Node.Bounds` in a test. A
screenshot is in physical pixels: logical times the density. The answer carries the density in the
`X-Gift-Density` header and the drawn frame number in `X-Gift-Frames`. Read the `centre` of a node from
`/query` and aim at it, rather than guessing a point from a screenshot:

```bash
curl -fsS 'http://127.0.0.1:7391/query?text=Save&interactive=1' | jq '.nodes[0].centre'
```

### Input steps

A `POST /input` body is a list of steps. Each step that touches gift runs in its own update, so a press and a
release behave like two events, not like one:

```json
{"steps":[
  {"op":"pointerDown","x":100,"y":200},
  {"op":"waitMs","ms":100},
  {"op":"pointerMove","x":100,"y":260},
  {"op":"waitFrames","frames":2},
  {"op":"pointerUp"}
]}
```

| Ops                                                              | Fields                                        |
|------------------------------------------------------------------|-----------------------------------------------|
| `pointerMove`, `pointerDown`, `pointerUp`, `pointerCancel`, `tap` | `x`, `y` (omitted: the last position), `mouse` |
| `click`                                                          | `x`, `y`; always a mouse, in one update       |
| `wheel`                                                          | `x`, `y`, `dx`, `dy` in notches; positive is up or left, so `"dy":-1` moves a list forward |
| `key`, `keyDown`, `keyUp`                                        | `key` such as `"tab"`, `"enter"`, `"escape"`; `mods` such as `["shift"]` |
| `text`                                                           | `text`, typed character by character          |
| `focus`                                                          | `forward`, default `true`                     |
| `waitMs`, `waitFrames`, `waitTicks`                              | `ms`, `frames`, `ticks`                       |
| `capture`                                                        | `frames`, `png`                               |

{{< callout type="warning" >}}
**Pointer steps are touches unless you say `"mouse":true`.** The backend polls the physical mouse every tick
and moves gift's mouse pointer to where your real cursor rests. A synthesized mouse press would be dragged
there on the next tick, and every button declines a dragged release. A touch is not affected. When you need a
mouse, for hover or the focus ring after a click, use the `click` op, which does the whole gesture inside one
update.
{{< /callout >}}

### Watching something move

A screenshot is a round trip, and the frame it captures is whichever one the window draws next. Two
screenshots that look alike do not prove that nothing animated in between. Put a `capture` step in front of the
input instead. It collects the next `frames` drawn frames, with a hash of each and the number of pixels that
changed against the frame before; `"png":true` includes the images. Motion is a run of non-zero `changed`
counts, and the end of a transition is the run of zeroes after it:

```bash
curl -fsS -XPOST http://127.0.0.1:7391/input -d '{"steps":[
  {"op":"capture","frames":16},
  {"op":"tap","x":100,"y":700}]}' | jq '.captured[] | {index,changed}'
```

The answer to `/input` also carries `animating` and `animations`, the state of
[animations](../animation/) right after the batch, so a script can wait for the screen to come to rest instead
of counting frames. `/diag` spells the same two fields `Animating` and `Animations`.

The screenshots on this site are taken this way. The program in
[`docs/capture`](https://github.com/worldiety/gift/tree/main/docs/capture) builds the gallery with
`-tags giftauto`, starts it once per demo, switches the theme through `/theme` and fetches `/screenshot`.

## Build tags

| Tag         | Effect                                                                                    |
|-------------|-------------------------------------------------------------------------------------------|
| `giftgpu`   | `gifttest` renders through the Ebitengine backend: golden images and pixel assertions run instead of skipping |
| `giftauto`  | the Ebitengine backend starts the HTTP automation interface on loopback                   |
| `giftdebug` | gift checks programming errors that are too expensive for a release build, and panics on them |

`giftdebug` is worth turning on for your test suite, `go test -tags giftdebug ./...`. With it, gift panics,
among other checks, when:

- state or the `gift.App` is touched from a goroutine other than the UI goroutine; see
  [Concurrency](../concurrency/),
- two siblings share the same key,
- the code that built a view keeps writing into the children slice it handed over,
- a semantic color reaches a painter without being resolved against the theme, which is otherwise a widget
  that draws nothing at all.

It also names every node that starts or stops overflowing, in the application's logger or on standard error.
Without the tag, these checks compile to nothing. The race detector, `go test -race`, finds the same class of
goroutine bug in a build without the tag.

## API

### Harness

{{< api pkg="gifttest" type="Harness" common="true" >}}

### Node

{{< api pkg="gifttest" type="Node" common="true" >}}

### Selector

{{< api pkg="gifttest" type="Selector" >}}

### Functions

{{< api-funcs pkg="gifttest" names="Main,Update" >}}

## Related

- [State](../state/): the counter this page tests.
- [Input and Focus](../input-and-focus/): what clicks, taps, keys and the focus order do.
- [Layout](../layout/): what overflow is and how to avoid it.
- [Diagnostics](../diagnostics/): the counters behind `Settle`, `AssertNoOverflow` and `/diag`.
- [Kiosks](../kiosks/): building the binary you ship, without `giftauto`.
- [Counter example](../../examples/example-counter/) and [Kitchen sink example](../../examples/example-kitchensink/).
