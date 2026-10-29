---
title: State
weight: 3
---

A view function in gift is a description: it runs, returns a tree of views and forgets everything. Whatever has to
survive from one build to the next lives in **state**. State belongs to a component instance, is typed, and knows
which components read it. When you write a new value, gift rebuilds exactly those components in the next update.

This page explains how to create and read state, which writes rebuild what, and which kinds of state gift keeps
for you without asking.

## A first example

```go
func counter(ctx *gift.Context) gift.View {
	count := ctx.State("count", 0)
	n := ctx.Read(count)

	return ui.VStack(
		ui.Text(strconv.Itoa(n)).FontSize(48),
		ui.Button(ui.Text("+1"), func() {
			count.Set(count.Get() + 1)
		}),
	).Gap(8)
}
```

Three calls do all the work:

- `ctx.State("count", 0)` returns the state named `count` of this component instance. The first build creates it
  with the value `0`; every later build returns the same `*gift.State[int]`.
- `ctx.Read(count)` returns the current value **and** registers the component as a dependency of the state.
- `count.Set(...)` in the button's action stores a new value and marks every component that read the state as
  needing a build.

The rebuild does not happen inside `Set`. It happens in the next `App.Update`, so several writes in one event
handler, or in several handlers before the next update, cost one rebuild. The full program is the
[counter example](../../examples/example-counter/).

## Creating state: `ctx.State`

`ctx.State(name, initial)` is a generic method on [`*gift.Context`](https://pkg.go.dev/github.com/worldiety/gift#Context).
The type parameter comes from `initial`, so the result is fully typed and you never write a type assertion.

- **The name is local to the component instance.** Two instances of the same function, or two different
  functions, can both use `"count"` without interfering.
- **`initial` is used on the first build only.** It is still evaluated on every build, because it is an ordinary
  argument. Do not compute something expensive there; create a cheap zero value and fill it in later.
- **The type of a name is fixed.** Asking for `"count"` as an `int` in one build and as a `string` in the next
  panics. That always means the identity of the component is wrong, and gift reports it instead of silently
  starting over.
- **The element type must be `comparable`.** gift detects a change with `==`. A slice or a map does not compile as
  state; see [Values and comparison](#values-and-comparison).

State lives as long as its component instance is mounted. It does not disappear when a build stops calling
`ctx.State` for that name: a branch that is not taken this time keeps its state for when it comes back. It
disappears when the component instance is unmounted, that is, when its parent no longer returns it.

## Reading state: `ctx.Read` and `Get`

There are two ways to get the value, and the difference matters.

| Call               | Registers a dependency | Use it in                       |
|--------------------|------------------------|---------------------------------|
| `ctx.Read(s)`      | yes                    | the view function, during build |
| `s.Get()`          | no                     | event handlers and callbacks    |

Inside a build, always use `ctx.Read`. A view that reads a value with `Get` displays it correctly once and then
never updates, because no write knows that this component depends on it.

Inside an event handler, use `Get`. The handler runs long after the build, and its `Context` is no longer valid:
calling `ctx.Read`, `ctx.State` or `ctx.Request` outside the build that received the context panics.
That is why the counter above captures the `*gift.State` in the closure, not the context.

The dependency set is replaced after every build, not accumulated. A component that read a state in its last
build depends on it; a component that stopped reading it, for example because the branch that showed it is gone,
is no longer rebuilt when it changes.

## Writing state: `Set`

`Set` compares the new value with the current one using `==`.

- An equal value changes nothing and invalidates nothing.
- A different value is stored and every component that read the state in its last build is queued for a build.

Where you may call it:

- **Event handlers** are the normal place. Input is dispatched before build and layout, so a write in a handler
  is picked up by the build of the same tick.
- **Another goroutine** is a programming error. A build with the `giftdebug` tag panics; a release build does not
  check, and the race detector finds it. Hand worker results to the UI goroutine with `App.Post` or `Token.Post`,
  see [Concurrency](../concurrency/).
- **During paint** it panics in every build, because it would mean building while drawing.
- **During a build** it is allowed, but a write to a state that a component reads queues that component again
  for the next update. A view function that writes what it reads on every build rebuilds on every frame.

## Which writes rebuild what

The unit of rebuild is the **component instance**. Your root function, passed as `gift.Options.Root`, is one.
Each [`gift.Component`](https://pkg.go.dev/github.com/worldiety/gift#Component) and
[`gift.Memo`](https://pkg.go.dev/github.com/worldiety/gift#Memo) adds another. A plain Go function that returns a
`gift.View` is not one: it is inlined into the build of its caller.

A write rebuilds the components that read the state, and nothing above them:

```go
func clock(ctx *gift.Context) gift.View {
	seconds := ctx.State("seconds", 0)
	return ui.VStack(
		ui.Text("Elapsed"),
		gift.Component("display", func(ctx *gift.Context) gift.View {
			return ui.Text(strconv.Itoa(ctx.Read(seconds)) + " s")
		}),
		ui.Button(ui.Text("Tick"), func() { seconds.Set(seconds.Get() + 1) }),
	).Gap(8)
}
```

`clock` owns the state but never reads it, so a tick rebuilds only the inner `display` component. Reading is what
subscribes, not owning.

Going down, the two kinds of component behave differently:

- **`gift.Component`** has no boundary towards its parent. Its inputs are captured in a closure, and gift cannot
  compare functions, so every rebuild of the parent rebuilds the component too, all the way down.
- **`gift.Memo`** takes its inputs as an explicit `comparable` value. When the props are equal to the last build
  and no state it read has changed, its function is not called and its whole subtree stays as it is.

```go
type rowProps struct {
	Title    string
	Selected bool
}

func row(ctx *gift.Context, p rowProps) gift.View {
	t := ui.Text(p.Title)
	if p.Selected {
		return t.Foreground(ui.ColorAccent)
	}
	return t
}

func list(ctx *gift.Context) gift.View {
	sel := ctx.State("sel", 0)
	s := ctx.Read(sel)
	return ui.VStack(
		gift.Memo("a", rowProps{Title: "Alpha", Selected: s == 0}, row),
		gift.Memo("b", rowProps{Title: "Beta", Selected: s == 1}, row),
	)
}
```

Changing the selection rebuilds `list` and the rows whose `Selected` flag changed. A row whose props are equal is
skipped. Do not put a slice, a map or a function in the props: a slice or a map does not compile, and a func field
panics when gift compares it.

Use `Memo` where a parent invalidates often and the subtree below it is large. For most screens, `Component`
around the part that changes is enough.

### Identity and keys

State is attached to a component instance, and the instance is identified by its **key** among its siblings. Two
builds that return `gift.Component("left", ...)` in the same parent refer to the same instance and the same state,
even if the function value differs.

```go
func screen(ctx *gift.Context) gift.View {
	return ui.VStack(
		ui.Text("Two independent counters"),
		gift.Component("left", counter),
		gift.Component("right", counter),
	).Gap(16)
}
```

- Keep keys stable across builds. A key that changes, such as one derived from a position that shifts, unmounts
  the old instance and mounts a fresh one with fresh state.
- A helper function that calls `ctx.State` stores the state in the **caller's** instance. Calling that helper
  twice with the same state name gives both calls the same state. If each call needs its own state, wrap it in
  `gift.Component` with a distinct key.

`ctx.Key()` and `ctx.Path()` return the key and the path of the current instance. They are meant for diagnostics;
they also appear in gift's panic messages. Like every other method of the context, they panic when called after
the build, from a context kept in a callback.

## Bindings

A [`gift.Binding[T]`](https://pkg.go.dev/github.com/worldiety/gift#Binding) is a read and write handle to a state,
obtained with `state.Binding()`. It lets a control or a helper read and write a value without owning it: the
state stays with its component and outlives every binding handed out.

`Binding.Get` behaves like `ctx.Read` when it is called during a build: it registers the component that is
currently being built as a dependency. Outside a build it is a plain read. `Binding.Set` has the same semantics as
`State.Set`. The zero `Binding` panics on use; `IsZero` tells you whether you have one.

That makes a binding the natural parameter of a helper that edits a value:

```go
func stepper(b gift.Binding[int]) gift.View {
	return ui.HStack(
		ui.Button(ui.Text("-"), func() { b.Set(b.Get() - 1) }),
		ui.Text(strconv.Itoa(b.Get())),
		ui.Button(ui.Text("+"), func() { b.Set(b.Get() + 1) }),
	).Gap(8)
}

func order(ctx *gift.Context) gift.View {
	qty := ctx.State("qty", 1)
	return stepper(qty.Binding())
}
```

The `b.Get()` in the text runs during the build of `order`, so `order` is rebuilt when the quantity changes. The
two calls in the actions run in event handlers and register nothing.

Most controls in `ui` do not take a binding. They take the current value and a callback, and they draw only what
they were given; nothing changes until you store the new value and the component rebuilds:

```go
func settings(ctx *gift.Context) gift.View {
	wifi := ctx.State("wifi", true)
	return ui.HStack(
		ui.Text("Wi-Fi"),
		ui.Toggle(ctx.Read(wifi), wifi.Set),
	).Gap(12)
}
```

This one-way flow is what lets you refuse a change: a callback that does not write leaves the
[toggle](../../components/basic/toggle/) where it was. [Slider](../../components/basic/slider/) and
[segmented control](../../components/basic/segmented-control/) work the same way. The
[text field](../../components/basic/text-field/) is the one control with a `Bind` method, described next.

## Text fields and `ui.Editor`

A text field has more state than its text: the caret, the selection, the horizontal scroll. Each keystroke can
cause a rebuild, and a rebuild replaces the view, so that state cannot live in the view. It lives in a
[`*ui.TextEditor`](https://pkg.go.dev/github.com/worldiety/gift/ui#TextEditor) that your component owns.

`ui.Editor(ctx, name, initial)` is the short way to get one. It is an ordinary `ctx.State` underneath, holding a
`*ui.TextEditor`, so the editor lives and dies with the component like any other state. `initial` is used on the
first build only; to replace the text later, call `SetText` on the editor or go through a binding.

To keep the text itself in your own state, give the field both an editor and a binding:

```go
func form(ctx *gift.Context) gift.View {
	name := ctx.State("name", "")
	ed := ui.Editor(ctx, "name-editor", "")

	return ui.VStack(
		ui.TextField(ed).Placeholder("Your name").Bind(name.Binding()),
		ui.Text("Hello, "+ctx.Read(name)),
		ui.Button(ui.Text("Clear"), func() { name.Set("") }),
	).Gap(8)
}
```

The binding works in both directions. Every edit writes `name`, and a value written to `name` from anywhere else,
like the Clear button, replaces the field's text. The editor and the binding are not alternatives: the state holds
the value, the editor holds the caret. One editor drives one field; mounting two fields over the same editor
panics. `OnChange` composes with `Bind`; both run, the binding first.

## Values and comparison

Because change detection is `==`, the shape of the value decides what gift can see.

- **Plain values** (numbers, strings, bools) and **structs of plain values** work as expected. To change one field
  of a struct, copy it, change the copy and write it back:

  ```go
  next := f.Get()
  next.Limit += 20
  f.Set(next)
  ```

- **Pointers** compare by address. A `State[*Model]` only notices when you store a different pointer. Mutating
  the pointee in place changes nothing gift can detect and rebuilds nothing. Either store a new pointer, or keep
  the parts that the screen depends on in their own states.
- **Slices and maps** cannot be state. Keep them behind a pointer (with the caveat above) or in your model, and
  keep a comparable value in state that changes when they do, such as a version counter or a length.

## The three invalidation levels

gift separates three kinds of work in every update, and each change triggers only the levels it needs:

| Level  | What runs                                          | Triggered by                                                     |
|--------|----------------------------------------------------|------------------------------------------------------------------|
| build  | view functions of the affected components          | `State.Set`, `Binding.Set`, a changed `Memo` prop, `App.Invalidate` |
| layout | layouters of the nodes on the dirty path            | every build; `RequestLayout` from a custom view                  |
| paint  | the display list for the frame                      | every layout; a hover, a press, a focus change, a scroll        |

A build implies a layout and a layout implies a paint, never the other way round.

Layout is partial. A rebuilt node marks itself and its ancestors as needing layout, and the layout pass descends
only along that path. A clean node that is offered the same constraints as last time returns its cached size
without running its layouter, so a write in one subtree does not measure a sibling subtree.

Paint is cheap but not skipped: gift redraws the whole screen every frame. What invalidation saves is CPU work,
not fill rate. A frame where nothing changed calls no view function and measures nothing. The paint flag mainly
tells the backend whether something changed, so that it can drop to its idle tick rate.

Two `App` methods force a build from outside. `App.Invalidate` rebuilds the root, which also reaches every plain
`Component` but stops at a `Memo` whose props are unchanged. `App.InvalidateAll` rebuilds every mounted component
instance; `ui.SetTheme` uses it, because colors are resolved during build. Neither belongs in the frame path. See
[Theming](../theming/).

## Interaction state lives in the retained tree

Not everything that changes on screen is your state. Hover, press and focus are presentation state that gift owns.
They are kept in the retained node as a [`gift.Interaction`](https://pkg.go.dev/github.com/worldiety/gift#Interaction),
next to the scroll offset of a scroll container and the control state a widget keeps between frames, such as an
animation in progress.

- Changing them repaints and does not rebuild anything. Moving the mouse over a button runs no view function.
- They survive a rebuild that happens for an unrelated reason, because a rebuild replaces the view, not the node.
- You do not store them yourself. A button chooses its hover look through `HoverStyle`, not through a state
  that you toggle on enter and leave.

The same split explains `ui.TextEditor` and the [image gallery](../../components/lists/image-gallery/): where a
widget has a long-lived model that is not gift's to own, such as a text document, the application holds it and the
view is a declaration over it. If you write your own interactive views, see [Custom views](../custom-views/) for
`EventContext.Repaint` and `RequestLayout`, which invalidate paint or layout without a build.

## Observing it

[`App.Diagnostics`](../diagnostics/) counts what each level did. `Builds` counts rebuilt component scopes,
`Layouts` the layouters that ran, `PaintedNodes` the painters that ran. A test can assert that a hover moves none
of the first two, or that a write in one component rebuilds one scope; see [Testing](../testing/).

## API

{{< api pkg="gift" type="Context" >}}

{{< api pkg="gift" type="State" >}}

{{< api pkg="gift" type="Binding" >}}

## Related

- [Views](../views/) for components and the view tree.
- [Concurrency](../concurrency/) for `App.Post`, `ctx.Request` and tokens.
- [Text field](../../components/basic/text-field/) for `ui.Editor` and `Bind` in detail.
- [Counter example](../../examples/example-counter/) and [kitchen sink](../../examples/example-kitchensink/),
  which keeps every control's value in state.
