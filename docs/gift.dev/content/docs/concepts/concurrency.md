---
title: Concurrency
weight: 12
---

A gift application runs on one goroutine. That goroutine builds the views, lays them out, dispatches input, runs
your event handlers and owns every piece of state. gift calls it the **UI executor**. Slow work, such as reading a
file, querying a database or talking to a device, belongs on other goroutines. Those goroutines never touch state.
They hand their result to the UI executor, and the UI executor applies it.

This page explains who owns what, how a result travels back with `App.Post`, how a `Token` drops results that
arrived too late, and how a `giftdebug` build catches the mistakes.

## One goroutine owns the app

The goroutine that calls `gift.New` becomes the UI executor of that app. From then on, the app, its tree and the
state of every component belong to that goroutine. `backend.Run` must be called from the same goroutine, because
the backend calls `App.Update` and `App.Paint` on it.

Everything you write in a gift program already runs there:

- view functions, during the build,
- event handlers and the actions of controls such as `ui.Button`,
- closures that you post with `App.Post`.

Inside these, you read and write state freely. There are no locks to take and nothing to synchronize.

Anything else is a programming error: calling `State.Set` or `State.Get` from a goroutine you started, calling
`App.Invalidate`, or changing a `*ui.TextEditor` or a `*ui.ListState` while the UI executor might read it. Neither
the state nor these objects use locks.

Exactly two methods of `*gift.App` may be called from any goroutine:

| Method            | What it does                                                                  |
|-------------------|-------------------------------------------------------------------------------|
| `App.Post`        | queues a closure that runs on the UI executor at the start of the next update |
| `App.Diagnostics` | returns a snapshot of the frame counters, see [Diagnostics](../diagnostics/)  |

`Token.Post`, described below, may also be called from any goroutine.

## Posting a result: `App.Post`

`App.Post(fn)` hands `fn` to the UI executor. The next `App.Update` runs every posted closure first, before it
builds anything. A state write inside the closure therefore reaches the screen in that same update.

The typical worker computes its result on its own goroutine and posts only the part that touches state. As in
[Your first app](../../getting-started/first-app/), `backend` is the package `github.com/worldiety/gift/backend/ebiten`:

```go
var app *gift.App

func main() {
	app = gift.New(gift.Options{Root: root})

	go func() {
		time.Sleep(5 * time.Second) // stands for any slow work
		app.Post(func() {
			ui.SetTheme(app, ui.DarkTheme())
		})
	}()

	if err := backend.Run(app, backend.Config{Title: "demo", Width: 640, Height: 480}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

`ui.SetTheme` rebuilds the whole tree through `App.InvalidateAll`, which only the UI executor may call, so a
goroutine that switches the theme, for example one that watches the desktop appearance, has to post the switch.
See [Theming](../theming/).

A few rules about posted closures:

- **They run in order.** Closures run in the order they were posted.
- **Posting is cheap.** `Post` takes a mutex, appends the closure and returns. It never waits for the UI executor.
- **A post from inside a posted closure waits for the next update.** A closure that posts again does not run in the
  same batch, so a feedback loop cannot stall a frame.
- **Nothing is coalesced.** A worker that posts a thousand progress updates runs a thousand closures. Writing the
  same value twice is cheap, because `State.Set` compares with `==` and does nothing for an equal value, but
  report progress at a sensible rate.
- **`Post(nil)` panics.**

A view function receives a `*gift.Context`, which has no handle to the app. A program with one window usually
keeps the app in a package variable, as above and in the [counter example](../../examples/example-counter/). A
component that starts its own work uses a `Token` instead, which carries the app for you.

## Stale results: `Token` and `ctx.Request`

A result must not be applied blindly. Between starting a request and its answer, two things can happen:

- the component that asked was **unmounted**, for example because the person navigated away, or
- the component **asked again**, for example because the person picked another file, and the first answer is now
  wrong.

Applying the old answer would show the size of the previous file, or the picture of the previous item in a reused
tile. A `Token` prevents that. `ctx.Request()` starts a new request for the component instance and returns its
token. `Token.Valid()` reports whether the result may still be applied: the instance is still mounted and no newer
request was started on it. Every call to `ctx.Request()` invalidates all earlier tokens of the same instance, so
the newest request always wins.

`Token.Post(fn)` combines both steps. It posts `fn` to the UI executor and runs it there only if the token is still
valid by then. The following component shows the size of a file and starts a new lookup whenever its path changes:

```go
type sized struct {
	path string
	size int64
	err  bool
}

func fileSize(ctx *gift.Context, path string) gift.View {
	result := ctx.State("result", sized{})
	started := ctx.State("started", "")

	if started.Get() != path {
		started.Set(path)
		tok := ctx.Request()
		go func() {
			r := sized{path: path}
			if fi, err := os.Stat(path); err != nil {
				r.err = true
			} else {
				r.size = fi.Size()
			}
			tok.Post(func() { result.Set(r) })
		}()
	}

	r := ctx.Read(result)
	switch {
	case r.path != path:
		return ui.Text("Loading…")
	case r.err:
		return ui.Text("Cannot read " + path)
	default:
		return ui.Text(fmt.Sprintf("%d bytes", r.size))
	}
}
```

The parent mounts it with `gift.Memo("size", path, fileSize)`, so the component rebuilds when the path changes.
A few details matter here:

- **The request starts during the build.** `ctx.Request()` is a `Context` method, and a `Context` is valid only
  during the build that received it. You cannot call it from a button action or from the goroutine. Let the
  action write state, such as the selected path, and let the build notice that the result does not match and
  start the request.
- **The build guards against starting twice.** A component rebuilds for many reasons. `started` remembers what
  the newest request was for, and the build reads it with `Get`, so writing it does not cause another rebuild.
  Without the guard, every rebuild would start a new goroutine and invalidate the previous answer.
- **The result carries what it answers.** `result` stores the path next to the size, so the view can tell "the
  answer for this path" from "an older answer that is still on screen".
- **The goroutine captures values, not the context.** It uses `path`, `tok` and the `*gift.State` `result`, all of
  which are safe to carry. It never touches `ctx`, and it touches `result` only inside the posted closure.

If you need the check in your own closure, for example because you post through `App.Post` anyway, call
`Token.Valid` on the UI executor:

```go
tok := ctx.Request()
go func() {
	v := expensive()
	app.Post(func() {
		if !tok.Valid() {
			return // unmounted, or a newer request won
		}
		result.Set(v)
	})
}()
```

`Token.Valid` must run on the UI executor, which is where a posted closure runs. The zero `Token` is never valid,
and its `Post` does nothing.

{{< callout type="info" >}}
Hiding is not unmounting. A component in a tab that is not selected stays mounted, and its tokens stay valid. A
result that arrives meanwhile is applied, and you see it when the tab comes back.
{{< /callout >}}

A token does not cancel the work. The goroutine of an outdated request still runs to its end; only its result is
dropped. If the work is expensive, pass a `context.Context` of your own to it and cancel that when you start the
next request.

The [file picker](../../components/dialogs/file-picker/) reads directories this way: each directory listing runs
on its own goroutine, and a listing that was overtaken or that finishes after the picker closed is dropped.

## Values you hand over

A posted closure shares memory with the goroutine that created it. Keep the handover one way:

- Build the result completely on the worker, then post it. Do not change it after posting.
- Store comparable values in state, as `sized` above does. gift detects a change with `==`; a slice cannot be
  state at all. For a list, keep the data behind a stable pointer and bump a version counter in state when you
  replace it. See [Values and comparison](../state/#values-and-comparison).
- Do not read state from the worker either. Copy what the worker needs into local variables during the build,
  like `path` above, and pass those.

## Images and other services

The image pipeline in package `asset` decodes pictures on worker goroutines and delivers them through a closure
you provide. Wire that closure to `App.Post`, as the [gallery example](../../examples/example-gallery/) does:

```go
pipe := asset.NewPipeline(asset.Config{
	Deliver: app.Post,
	Disk:    asset.DiskCacheConfig{Dir: cacheDir},
})
defer func() {
	pipe.Close()
	app.DrainPosts()
}()
ui.SetImagePipeline(pipe)
```

The deferred function matters. `App.DrainPosts` runs everything posted since the last update, outside a frame.
The pipeline requires that every closure it delivers eventually runs, including the ones it delivers during
`Close`, because a delivered thumbnail releases its memory inside the closure. When `backend.Run` returns, the
frame loop has stopped and nothing else would run them. Call `DrainPosts` once, after the last frame and after
closing the pipeline, on the goroutine that ran the frame loop. See [Images](../images/).

## When a post runs

Posted closures run at the start of `App.Update`, which the backend calls once per tick, by default sixty times a
second. A result therefore usually appears within one tick after it was posted. If you set `IdleTPS` in the backend
configuration, the tick rate drops while nothing changes, and a result posted during that time waits for the next,
slower tick. See [Kiosks](../kiosks/).

Keep the UI executor free. A view function or an event handler that blocks, on a file, a network call or a
mutex that a worker holds for long, blocks input, layout and painting for that whole time. Start a goroutine and
post the answer instead.

In a test, the `gifttest` harness runs `App.Update` in every frame, so posted closures run there as well. A test
that waits for a worker can call `Harness.Frame` after the worker has posted. See [Testing](../testing/).

## Catching mistakes: `giftdebug` and the race detector

A release build does not check which goroutine calls into gift. The check needs the goroutine id, which Go only
exposes by parsing the stack header, and that is too slow for the event handler path. Build with the `giftdebug`
tag while you develop:

```bash
go run -tags giftdebug .
```

With the tag, gift remembers the goroutine that called `gift.New` and panics when another goroutine calls one of
these:

- `State.Get`, `State.Set`, `Binding.Get` and `Binding.Set` (which reports itself as `State.Set`),
- `App.Invalidate`, `App.InvalidateAll` and `App.SetDensity`,
- `App.MoveFocus`, `App.HitTest`, `App.ScrollTo`, `App.ScrollBy`, `App.ScrollIntoView` and
  `App.DismissSoftKeyboard`,
- the input entry points such as `App.PointerDown`, `App.KeyDown` and `App.TypeRune`.

The panic names both goroutines and tells you what to do:

```text
gift: State.Set called from goroutine 62, but the UI executor is goroutine 66; post the result to the UI executor instead of touching state directly
```

Without the tag, the race detector finds the same class of bug. Run your tests with `go test -race`. It reports a
conflict when it observes the worker and the UI executor touching the same memory, so it needs a test that
actually exercises the worker. The `giftdebug` check, in contrast, fires on the wrong call itself.

Two related checks are active in every build, with or without the tag:

- **A `Context` used outside its build panics.** Calling `ctx.State`, `ctx.Read` or `ctx.Request` from an event
  handler or a goroutine is rejected with a message that names the component.
- **A state write during `App.Paint` panics**, because it would mean building while drawing.

See [Diagnostics](../diagnostics/) for the other checks of a `giftdebug` build.

## API

{{< api pkg="gift" type="Token" >}}

| Function                                 | Description                                                                                            |
|------------------------------------------|--------------------------------------------------------------------------------------------------------|
| `func (a *App) Post(fn func())`          | Queues `fn` to run on the UI executor at the beginning of the next update. Safe from any goroutine.    |
| `func (a *App) DrainPosts()`             | Runs everything posted since the last update, outside a frame. Call it once after the last frame.     |
| `func (a *App) Diagnostics() Diagnostics`| Returns a snapshot of the counters as of the end of the last update or paint. Safe from any goroutine. |
| `func (c *Context) Request() Token`      | Starts a new asynchronous request for this component instance and invalidates all earlier tokens.     |

The full documentation is on [pkg.go.dev](https://pkg.go.dev/github.com/worldiety/gift#App.Post).

## Related

- [State](../state/): where results end up, and which writes rebuild what
- [Theming](../theming/): switching the theme from another goroutine
- [Images](../images/): the asset pipeline and its delivery through `App.Post`
- [Diagnostics](../diagnostics/): the `giftdebug` build and the frame counters
- [Testing](../testing/): driving updates from a test
- [File picker](../../components/dialogs/file-picker/): reads directories with `ctx.Request`
- [example-gallery](../../examples/example-gallery/): the image pipeline wired to `App.Post`
