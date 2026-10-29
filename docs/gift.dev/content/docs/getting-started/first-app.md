---
title: Your first app
linkTitle: First app
weight: 3
---

This page walks you through a complete gift program: a counter with a number and two buttons. It is short, but it
contains every part a gift application needs. Read it once from top to bottom, then take it apart piece by piece
below.

## The whole program

Create a new module, add gift as described in [Installation](../installation/), and put this into `main.go`:

```go {filename="main.go"}
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/worldiety/gift"
	backend "github.com/worldiety/gift/backend/ebiten"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func main() {
	ui.SetDefaultFont(ui.MustFont(ui.FontQuery{Family: inter.Family}))

	app := gift.New(gift.Options{Root: counter})

	err := backend.Run(app, backend.Config{Title: "Counter", Width: 400, Height: 300})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func counter(ctx *gift.Context) gift.View {
	count := ctx.State("count", 0)
	n := ctx.Read(count)

	return ui.Window(
		ui.VStack(
			ui.Text(strconv.Itoa(n)).FontSize(48),
			ui.HStack(
				ui.Button(ui.Text("-"), func() {
					count.Set(count.Get() - 1)
				}).Disabled(n == 0),
				ui.Button(ui.Text("+"), func() {
					count.Set(count.Get() + 1)
				}),
			).Gap(8),
		).Gap(16).Padding(24).Align(geom.Center),
	).Align(geom.Center)
}
```

Run it:

```bash
go run .
```

A window titled "Counter" opens. Click `+` and the number goes up. Click `-` and it goes down, but never below zero:
at zero the `-` button is disabled. You can also press <kbd>Tab</kbd> to move the focus to a button and activate
it with <kbd>Space</kbd> or <kbd>Enter</kbd>.

## Step by step

{{% steps %}}

### Install a font

```go
ui.SetDefaultFont(ui.MustFont(ui.FontQuery{Family: inter.Family}))
```

Gift links no font into your binary unless you ask for one. The package `font/inter` embeds the Inter typeface and
registers it when you import it. It does not make itself the default, because that choice belongs to the
application. `ui.MustFont` looks up the registered face and `ui.SetDefaultFont` makes it the font of every
`ui.Text` that does not name another one.

Install the font before the first frame. If a `ui.Text` is built without any font, gift panics with a message
that tells you how to fix it. A second bundled typeface is `font/ibmplexmono`, and `ui.LoadFont` loads your own
font file. See [Text and fonts](../../concepts/text-and-fonts/).

### Create the app

```go
app := gift.New(gift.Options{Root: counter})
```

`gift.New` creates the application and mounts `Root` as its root component. It builds nothing yet; that happens in
the first update. The goroutine that calls `gift.New` becomes the UI goroutine of the app. Every view function and
every state write runs on it. See [Concurrency](../../concepts/concurrency/) for how background work hands its
results back.

### Write the view function

```go
func counter(ctx *gift.Context) gift.View {
	count := ctx.State("count", 0)
	n := ctx.Read(count)
	// ...
}
```

A view function takes a `*gift.Context` and returns a `gift.View`. It describes what the screen looks like right
now and keeps nothing itself. Whatever has to survive between two builds lives in state:

- `ctx.State("count", 0)` returns the state named `count` of this component. The first build creates it with the
  value `0`; every later build returns the same `*gift.State[int]`.
- `ctx.Read(count)` returns the current value and records that this component depends on it.
- `count.Get()` returns the value without recording a dependency. Use it inside event handlers, like the button
  actions here.
- `count.Set(...)` stores a new value. Gift rebuilds every component that read the state in its next update, not
  inside `Set`.

See [State](../../concepts/state/) for the details.

### Compose the views

```go
ui.VStack(
	ui.Text(strconv.Itoa(n)).FontSize(48),
	ui.HStack(
		ui.Button(ui.Text("-"), func() { /* ... */ }).Disabled(n == 0),
		ui.Button(ui.Text("+"), func() { /* ... */ }),
	).Gap(8),
).Gap(16).Padding(24).Align(geom.Center)
```

Views are plain Go values. `ui.VStack` arranges its children from top to bottom, `ui.HStack` from left to right.
`ui.Button` takes a label, which is itself a view, and an action. Methods such as `FontSize`, `Gap`, `Padding` and
`Disabled` are modifiers: each one returns a changed copy of the view, so you can chain them. `Align(geom.Center)`
on the vertical stack centers its children horizontally.

The buttons need no styling: without a style they take their colors from the current theme, which is the light
theme by default. See [Views](../../concepts/views/), [Modifiers](../../concepts/modifiers/),
[Layout](../../concepts/layout/) and [Theming](../../concepts/theming/).

### Wrap everything in a window

```go
return ui.Window(
	ui.VStack(/* ... */),
).Align(geom.Center)
```

Gift draws exactly what the view tree describes and nothing else. It paints no background of its own, so a root
view that does not cover the window leaves holes in it. `ui.Window` fills the whole window with the theme's
background color and puts your content on top. `Align(geom.Center)` places the content in the middle of the
window. Make `ui.Window` the outermost view of every root function.

### Open the window

```go
err := backend.Run(app, backend.Config{Title: "Counter", Width: 400, Height: 300})
```

The package `backend/ebiten` connects the app to [Ebitengine](https://ebitengine.org/). `Run` opens a window,
forwards mouse, keyboard and touch input to the app, and draws it on the GPU until the window is closed. Call it
from the goroutine that called `gift.New`.

`Config` sets the window title and the initial size in logical pixels; zero selects 1280x720. It has more fields
for kiosks and measurements, for example `DrawOnDemand`, which skips drawing frames in which nothing changed. See
[Kiosks](../../concepts/kiosks/) and the
[package documentation](https://pkg.go.dev/github.com/worldiety/gift/backend/ebiten#Config).

{{% /steps %}}

## Test it

Because the interface is plain Go, you can test it without a window. `gifttest` builds the view tree headlessly
and clicks what the user sees:

```go {filename="main_test.go"}
package main

import (
	"testing"

	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

func TestMain(m *testing.M) { gifttest.Main(m) }

func TestCounter(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{
		Root: counter,
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}),
	})

	h.Find(gifttest.ByText("+")).Click()
	h.Find(gifttest.ByText("+")).Click()
	h.Find(gifttest.ByText("2"))

	h.Find(gifttest.ByText("-")).Click()
	h.Find(gifttest.ByText("1"))
}

func TestCounterPaintsTheWindow(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{
		Root: counter,
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}),
	})
	h.AssertOpaque()
}
```

`Options.Font` installs the font for the duration of the test and restores the previous one afterwards.
`Find` fails the test unless exactly one node matches, so `h.Find(gifttest.ByText("2"))` is already an
assertion. `Click` moves the mouse onto the node, presses and releases it, and lets the app settle before it
returns.

`AssertOpaque` checks that the root view paints every pixel of the window, which is what `ui.Window` is for. It
needs real pixels: a plain `go test` skips it, and `go test -tags giftgpu` runs it. The `TestMain` line is required
for that mode and does nothing without the tag. See [Testing](../../concepts/testing/).

## Next steps

The [counter example](../../examples/example-counter/) is the same program with more to it: styled buttons with
hover and pressed states, a scroll view, and a button that switches between the light and the dark theme at
runtime. Run it from a checkout of the repository:

```bash
go run ./cmd/example-counter
```

{{< cards >}}
  {{< card link="../../examples/example-counter/" title="Counter example" icon="play" subtitle="The full counter with styling, scrolling and a theme switch." >}}
  {{< card link="../../concepts/views/" title="Views" icon="cube" subtitle="What a view is and how views compose." >}}
  {{< card link="../../concepts/state/" title="State" icon="database" subtitle="Typed state, dependencies and targeted rebuilds." >}}
  {{< card link="../../concepts/layout/" title="Layout" icon="template" subtitle="Stacks, frames, alignment and how sizes are negotiated." >}}
{{< /cards >}}
