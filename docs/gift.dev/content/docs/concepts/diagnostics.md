---
title: Diagnostics and Metrics
weight: 16
---

A release build of gift does not log from the frame path. Instead, it keeps plain counters while it builds, lays out and paints,
and you read them when you want to. On top of these counters there are two optional builds: a **metrics build**
that measures frame timings and writes a machine-readable report, and a **debug build** that turns silent
programming errors into panics and names every node that overflows.

This page explains the three layers and how to use each one:

| Layer                     | How you get it                              | What it tells you                                 |
|---------------------------|---------------------------------------------|---------------------------------------------------|
| `App.Diagnostics()`       | always there, in every build                 | counters: frames, builds, layouts, nodes, overflow |
| `metrics` package         | `-tags giftmetrics` and `GIFT_METRICS=1`     | frame time distributions and renderer counters, as JSON |
| `giftdebug` checks        | `-tags giftdebug`                            | panics on misuse, one log line per overflowing node |

Logging is the fourth piece. Outside the `giftdebug` build, gift logs only outside the frame path, and only into
the `*slog.Logger` you give it. See [Logging](#logging).

## The counters

`App.Diagnostics()` returns a `gift.Diagnostics` struct, a snapshot of the counters as of the end of the last
`Update` or `Paint`. It is safe to call from any goroutine. Taking the snapshot costs a struct copy under a
mutex, so you can poll it from a monitoring goroutine without slowing the frame loop.

Most counters only ever grow, from the moment the `App` is created:

| Field              | Counts                                                                         |
|--------------------|--------------------------------------------------------------------------------|
| `Frames`           | calls to `App.Paint`, that is drawn frames                                     |
| `Updates`          | calls to `App.Update`, that is ticks                                           |
| `Builds`           | component scopes that were rebuilt                                             |
| `Layouts`          | nodes whose layouter actually ran; a clean node that was skipped is not counted |
| `PaintedNodes`     | nodes whose painter actually ran; a node without background, border and clip is not counted |
| `PaintedOps`       | drawing operations emitted                                                     |
| `HitTests`         | hit tests, roughly the pointer events that had to find a target                |
| `InputEvents`      | calls into an `Interactor`, including the ones made while an event bubbles up  |
| `Scrolls`          | times a scroll offset actually changed                                         |
| `DiscardedTouches` | additional fingers that were recognized and dropped                            |
| `RunesTyped`       | characters that arrived through `App.TypeRune`, focused or not                 |
| `KeyRepeats`       | synthetic key presses produced from a held key                                 |

The others describe the current state of the tree:

| Field            | Value                                                                              |
|------------------|------------------------------------------------------------------------------------|
| `LiveNodes`      | nodes in the retained tree                                                         |
| `LiveScopes`     | mounted component instances                                                        |
| `OverflowNodes`  | nodes whose content does not fit into the size they reported                       |
| `OverflowExtent` | sum of their horizontal plus vertical overflow, in logical pixels                  |
| `Animations`     | repaint requests in force: `Animate` windows, flings, scroll indicators, transitions |
| `Animating`      | `Animations > 0`                                                                   |

### Reading the counters at runtime

Because most counters only grow, subtract two snapshots to get a rate. This goroutine writes one line every ten
seconds:

```go
func watch(app *gift.App, log *slog.Logger) {
	var last gift.Diagnostics
	for range time.Tick(10 * time.Second) {
		d := app.Diagnostics()
		log.Info("gift",
			slog.Uint64("frames", d.Frames-last.Frames),
			slog.Uint64("builds", d.Builds-last.Builds),
			slog.Uint64("live_nodes", d.LiveNodes),
			slog.Uint64("overflow_nodes", d.OverflowNodes))
		last = d
	}
}
```

Start it with `go watch(app, logger)` before `backend.Run`. A `LiveNodes` value that climbs while the screen
stays the same is a leak in your view tree, for example a list that appends children on every build.

### What the numbers say

The counters are evidence for how the frame model works, so a few combinations are worth knowing:

- **Frames stands still, Updates climbs.** The window is not visible. Ebitengine keeps calling `Update` for a
  hidden window and stops calling `Draw`. With only `Frames` to look at, this looks like a frozen application.
- **Scrolls climbs, Builds and Layouts do not.** This is the scroll fast path working: scrolling moves a
  retained subtree and runs no view function.
- **Builds climbs on hover.** Hover and press are presentation state of the retained node and run no view
  function by themselves. A build on hover comes from state that your own code writes; see [State](../state/).
- **RunesTyped stays zero while you type.** The keyboard does not reach gift, so the backend is at fault. A
  non-zero value with nothing happening on screen points at your view instead.
- **Animating stays true.** Something keeps asking to be redrawn, for example an indeterminate progress bar that
  is still on screen. A blinking caret stops after ten seconds on purpose. On a kiosk this is heat; see [Kiosks](../kiosks/).

### Asserting on the counters in a test

The `gifttest` harness exposes the same snapshot as `Harness.Diagnostics()`. That turns a performance property
into an ordinary assertion:

```go
func TestHoverCostsNoBuild(t *testing.T) {
	h := gifttest.New(t, gifttest.Options{Root: counter})
	before := h.Diagnostics().Builds

	h.Find(gifttest.ByKey("plus")).Hover()

	if got := h.Diagnostics().Builds; got != before {
		t.Errorf("hover rebuilt %d scope(s)", got-before)
	}
	h.AssertNoOverflow()
}
```

The automation interface of the `gift/auto` package serves the snapshot over HTTP, at `/diag`, together with
the list of overflowing nodes. See [Testing](../testing/).

## Overflow reporting

When content does not fit, gift keeps the honest sizes of the children, does not clip, and records the excess
as the *overflow* of the node. [Layout](../layout/#when-content-does-not-fit) explains the model. This section is
about finding overflow.

Three tools work in every build:

- `Diagnostics().OverflowNodes` and `OverflowExtent` say how much overflow the tree carries. A healthy scene has
  zero nodes. The extent tells a half-pixel problem from an 800 pixel one.
- `App.NodeOverflow(ref)` returns the overflow of one node and whether it overflows at all.
- `Harness.AssertNoOverflow()` fails a test when any node overflows, and prints the tree.

The counters describe the retained tree, not the last layout pass. A node that overflowed and was then skipped
by the layout cache still counts, so the number does not drop to zero just because nothing was re-measured.

{{< callout type="info" >}}
Clipping and truncation change what you see, not the layout. A container with `Clip(true)` or a label that ends
in an ellipsis still reports its overflow, and `AssertNoOverflow` still sees it.
{{< /callout >}}

### Reporting overflow from a custom layouter

If you write your own `Layouter`, report the excess with `LayoutContext.ReportOverflow`. Call it at most once per
`Layout` call, with the overflow per axis in logical pixels. Pass the zero size when the content fits again. A
layouter that never calls it counts as never overflowing. Negative, infinite and NaN values are treated as zero.
`ReportOverflow` allocates nothing, so it is fine in the frame path. See [Custom views](../custom-views/).

### Naming the overflowing node

The counters tell you *that* something overflows. A build with the `giftdebug` tag also tells you *which* node:

```bash
go run -tags giftdebug ./cmd/myapp
```

Every time the overflow of a node changes, gift writes one line with the registered view type, the key or the
component path and the slot index. When the node overflows, the line also gives the overflow per axis in logical
pixels; when it fits again, the line says that it no longer overflows. It goes to the logger of `gift.Options`
at `Warn` level with the attribute `giftdebug=overflow`, or to standard error when you did not pass a logger.
A node whose overflow stays the same produces one line, not one per frame.

A release build does not have this, because naming a node means formatting a string, and the frame path of a
release build formats nothing.

## The metrics build

The `metrics` package measures frame timings and collects the counters of gift, the renderer, the text shaper
and the image pipeline into one JSON report. You write no code for it. The Ebitengine backend wires it up
inside `backend.Run`; you rebuild with a tag and start the program with an environment variable:

```bash
go build -tags giftmetrics -o myapp ./cmd/myapp
GIFT_METRICS=1 ./myapp
```

Both are needed:

- **Without the tag**, `metrics.Enabled()` is the constant `false`. Every recording call is dead code that the
  compiler removes, so a shipped binary carries nothing. The environment variable is ignored.
- **With the tag**, measurement is still off until `GIFT_METRICS` says otherwise.

### Configuration

The configuration comes from the environment, because a library must not register command line flags in your
program. It is read once, when the package is initialized:

| Variable                | Value                                                                            |
|-------------------------|----------------------------------------------------------------------------------|
| `GIFT_METRICS`          | `1`, `true`, `yes` or `on` enables reporting; `0`, `false`, `no`, `off` or unset disable it |
| `GIFT_METRICS_INTERVAL` | a Go duration such as `5s`: also write a report this often, not only at the end  |
| `GIFT_METRICS_OUT`      | a file path to write the reports to; the file is created or truncated. Empty means standard output |

A malformed value never stops the program. gift writes one warning to standard error and ignores the value. If
the output file cannot be created, the reports go to standard output and a warning says so.

{{< callout type="warning" >}}
Set the variables before the program starts. Calling `os.Setenv("GIFT_METRICS", "1")` in `main` has no effect,
because the package has already read the environment by then.
{{< /callout >}}

### The report

Each report is one line of JSON. The final one is written when `backend.Run` returns, also when the frame loop
ends with an error. A periodic one is written every `GIFT_METRICS_INTERVAL`, checked once per update. A process that is
killed never returns from `Run` and writes no final report, so use an interval for runs that may not end
cleanly.

A report, shortened and spread over several lines, with illustrative values:

```json
{
  "kind": "final",
  "elapsed_s": 61.2,
  "gift": {"paints": 3640, "builds": 12, "layouts": 840, "live_nodes": 212, "overflow_nodes": 0},
  "renderer": {"frames": 3640, "draw_calls": 21840, "skipped_empty_bounds": 0},
  "shaper": {"hits": 50211, "misses": 64},
  "update_cpu":     {"count": 3600, "p50_ms": 0.21, "p95_ms": 0.48, "p99_ms": 0.9,  "max_ms": 3.1},
  "draw_cpu":       {"count": 3580, "p50_ms": 1.1,  "p95_ms": 1.9,  "p99_ms": 2.6,  "max_ms": 5.4},
  "frame_interval": {"count": 3580, "p50_ms": 16.7, "p95_ms": 16.9, "p99_ms": 17.1, "max_ms": 33.4},
  "nominal_interval_ms": 16.667,
  "interval_tolerance_ms": 0.5,
  "missed_threshold_ms": 17.167,
  "missed_intervals": 4,
  "missed_ratio": 0.0011,
  "warmup_frames": 60,
  "updates": 3672,
  "draws": 3640,
  "mem": {"heap_alloc_bytes": 7295312, "mallocs": 98607, "num_gc": 4}
}
```

Besides the frame timings, the report has blocks for `gift` (a subset of the `Diagnostics` counters: paints,
builds, layouts, painted nodes and operations, live nodes and scopes, overflow), `renderer` (draw
calls, skipped operations, glyph atlas, textures, render targets, glass and layer caches), `shaper` (the text
shaping cache), `asset` (the image pipeline, see below) and `mem` (Go heap statistics).

### Frame timing

gift keeps three timings apart and never adds them up:

| Series           | Measures                                                                   |
|------------------|----------------------------------------------------------------------------|
| `update_cpu`     | CPU time inside the update callback: input, build, reconciliation, layout  |
| `draw_cpu`       | CPU time inside the draw callback: display list and queuing the draw calls |
| `frame_interval` | wall clock time between the start of two draw callbacks                    |

None of them is a GPU time. A draw call returns once the command is queued, not when the GPU has executed it or
the frame has been presented. `frame_interval` is the series that answers "did we hold sixty frames per
second", because it contains everything the other two do not: GPU work, the swap and the vsync wait.

Each series is a distribution over the last 4096 samples, a little over a minute at sixty hertz: `count`, `min`,
`mean`, the nearest rank percentiles `p50`, `p95`, `p99`, `p999`, and `max`, all in milliseconds. Below about a
thousand samples, `p999` is simply the maximum.

A frame interval counts as **missed** when it exceeds `missed_threshold_ms`, which is the nominal interval plus a
tolerance of 0.5 ms. Without the tolerance, a well-timed run reports half of its frames as missed, because no
display runs at exactly its nominal rate. The report always prints the threshold next to the count, so you can
check what was compared against what.

Two kinds of samples are excluded and counted separately:

- the first 60 intervals (`warmup_frames`, with `warmup_dropped` counting the ones seen), because opening a
  window costs a first interval of well over a hundred milliseconds,
- intervals of 1 ms or less (`sub_frame_intervals`), because Ebitengine sometimes calls `Draw` twice in a row and
  those are not two presented frames.

`updates` and `draws` are the totals since the start. `Update` runs at the tick rate and `Draw` as often as the
display allows, so the two differ.

### Configuring a measurement run

The measurement settings live in `backend.Config`:

| Field                  | Effect                                                                                  |
|------------------------|-----------------------------------------------------------------------------------------|
| `NominalFrameInterval` | the interval the display is expected to hold; zero derives it from `TPS`. Set it for a display that is not 60 Hz |
| `IntervalTolerance`    | the slack added before a frame counts as missed; zero means 0.5 ms, a negative value means none |
| `WarmupFrames`         | leading intervals to discard; zero means 60, a negative value keeps all                 |
| `AssetStats`           | supplies the image pipeline counters; set it to `ui.ImagePipelineStats`                 |
| `GlassQuality`         | pins the quality of the glass material instead of the adaptive policy                   |
| `NoInput`              | ignores mouse and keyboard, so a resting cursor does not disturb the run                |
| `OnUpdate`             | runs once per update; returning `backend.Terminate` makes `Run` return `nil`            |

This run stops after sixty seconds on a 50 Hz panel, with the glass quality pinned and the image pipeline in the
report:

```go
func bench(app *gift.App) error {
	deadline := time.Now().Add(60 * time.Second)
	return backend.Run(app, backend.Config{
		Title:                "benchmark",
		NominalFrameInterval: 20 * time.Millisecond, // a 50 Hz panel
		GlassQuality:         render.Full,
		AssetStats:           ui.ImagePipelineStats,
		NoInput:              true,
		OnUpdate: func() error {
			if time.Now().After(deadline) {
				return backend.Terminate
			}
			return nil
		},
	})
}
```

The backend cannot fill in `AssetStats` by itself, because it does not own the image pipeline. Without that line
the report has no `asset` block. [example-gallery](../../examples/example-gallery/) sets it.

### Pitfalls of measuring

- **The measured build is a different binary.** Timing three callbacks per frame costs a few tens of nanoseconds
  each. A number from a `giftmetrics` build describes that build.
- **Do not measure a debug build.** The `giftdebug` checks cost time on every state access. Never combine the
  two tags for numbers you want to compare.
- **Pin the glass quality.** The adaptive policy changes how much work each frame does part of the way through a
  run, so the distribution has a seam in it. Set `GlassQuality` for any run you compare with another one.
- **Idle settings change what is measured.** With `DrawOnDemand`, most draw callbacks of a still scene paint
  nothing. `frame_interval` is still taken once per display refresh, drawn or skipped, so a still scene reports
  the refresh rate and not the rate of painted frames. `draw_cpu` leaves skipped refreshes out, except under
  `DirectToScreen`, where each one counts as a draw of almost no time. `IdleTPS` lowers the tick rate. State
  whether they were on.
  See [Kiosks](../kiosks/).

## The debug build

The `giftdebug` build tag compiles in checks that are too expensive for the frame path of a release build.
Build with it during development and in CI:

```bash
go test -tags giftdebug ./...
go run -tags giftdebug ./cmd/myapp
```

Without the tag, the checks compile to nothing. With it, each detected mistake panics with a message that says
what went wrong and what to do instead. The checks cover errors that otherwise produce a wrong picture rather
than a crash:

| Check                       | Panics when                                                                                   |
|-----------------------------|-----------------------------------------------------------------------------------------------|
| UI goroutine                | state is read or written, or `Invalidate`, `ScrollTo`, `MoveFocus`, `HitTest`, `SetDensity` and other `App` methods are called, from a goroutine other than the one that created the `App` |
| children ownership          | an element of a children slice handed to gift in an `Element` is replaced afterwards by a view of another type |
| duplicate keys              | two siblings share the same non-empty key                                                      |
| unresolved colors           | a semantic color such as `ui.ColorLabel` reaches a painter without `ui.ResolveColor`           |
| focus trap bookkeeping      | the internal count of focus traps or key fallbacks goes negative                               |
| text paragraph borrow       | a shaped paragraph is used after the shaping cache has evicted it                              |

On top of the panics, the debug build logs every overflowing node, as described in
[Naming the overflowing node](#naming-the-overflowing-node).

The goroutine check is the one you will hit most. The fix is always the same: do the work in a goroutine and
hand the result back with `Post`. See [Concurrency](../concurrency/). The race detector finds the same class of
bug in a build without the tag.

The children ownership check matters if you write [custom views](../custom-views/): the slice in
`Element.Children` belongs to gift from the moment `Build` returns, so do not reuse a scratch buffer for it.

{{< callout type="info" >}}
The debug build is slower: the goroutine check alone costs about a microsecond per state access. Use it to find
mistakes, not to judge performance.
{{< /callout >}}

## Logging

gift logs through `log/slog`, into the logger you pass, and nowhere else. A nil logger means logging is off.
gift never falls back to `slog.Default`, so it never writes into a destination you did not choose. The one
exception is the overflow report of a `giftdebug` build, which goes to standard error when there is no logger.

Outside the `giftdebug` build, nothing is logged from the frame path. The `App` logs once, at `Info` level, when
it is created. The backend logs when the window starts and stops, at `Warn` level when the adaptive glass policy
degrades quality, and at `Info` level when it restores it.

Pass the same logger to every piece that takes one:

```go
func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ui.SetClipboard(clipboard.New(logger))
	app := gift.New(gift.Options{Logger: logger, Root: root})

	go watch(app, logger)

	if err := backend.Run(app, backend.Config{
		Title:  "My app",
		Logger: logger,
	}); err != nil {
		logger.Error("run", "err", err)
		os.Exit(1)
	}
}
```

| Where                              | Logs                                                                  |
|------------------------------------|-----------------------------------------------------------------------|
| `gift.Options.Logger`              | creation of the `App`; in a `giftdebug` build, overflowing nodes       |
| `backend.Config.Logger`            | window start and stop, glass quality changes                           |
| `asset.Config.Logger`              | start and stop of the image pipeline; at `Debug` level, thumbnails that could not be written to the disk cache |
| `clipboard.New(logger)`            | clipboard failures; the clipboard installed by the side-effect import logs nothing |
| `PickerView.Logger` of `ui/filepicker` | directories the file picker cannot read; see [File picker](../../components/dialogs/file-picker/) |
| `gifttest.Options.Logger`          | passed to gift; nil, the default, is what a test normally wants        |

`App.Logger()` returns the logger from `gift.Options`, or nil. Use it in your own code instead of reaching for
`slog.Default`, and keep log calls out of view functions, painters and layouters: they run every frame, and a
log call allocates.

## Related

- [Layout](../layout/): the overflow model
- [Testing](../testing/): `gifttest`, `AssertNoOverflow` and the automation interface
- [Concurrency](../concurrency/): the UI goroutine and `Post`
- [Custom views](../custom-views/): `Element.Children` and `ReportOverflow`
- [Kiosks](../kiosks/): `IdleTPS`, `DrawOnDemand` and heat
- [example-gallery](../../examples/example-gallery/): a program that adds the image pipeline to the report
- [Package metrics on pkg.go.dev](https://pkg.go.dev/github.com/worldiety/gift/metrics)
