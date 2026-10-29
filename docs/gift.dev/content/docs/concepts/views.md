---
title: Views
weight: 1
---

A gift user interface is a tree of views. A view is a small Go value that describes one piece of the
interface: a text, a button, a stack of other views. You write functions that return views, gift turns them
into a retained tree of nodes, and on every change it compares the new description with that tree and applies
the difference. This page explains what a view is, how you compose views, and how gift decides which parts of
the tree to keep.

## What a view is

`gift.View` is an interface with two methods:

```go
type View interface {
	ViewType() TypeID
	Build(*BuildContext) Element
}
```

You rarely implement it yourself. The constructors of package `ui` return concrete view types such as
`ui.TextView`, `ui.Stack` or `ui.ButtonView`, and all of them satisfy `gift.View`. You call their modifiers to
configure them, and each modifier returns a changed copy of the same type (see
[Modifiers](../modifiers/)):

```go
ui.Text("Hello").FontSize(24).Foreground(ui.ColorAccent)
```

A view is a value, not an object with identity. It lives only for one build: gift reads it, updates or creates
the matching node, and drops the view again. Keeping a view in a variable and changing it later has no effect
on the screen. What lasts between builds lives elsewhere:

- **State** lives in the scope of a component, see [Components](#components-giftcomponent) below and
  [State](../state/).
- **Presentation state** such as hover, press, focus and the offset of a scroll view lives in the retained node.
  Changing it repaints the window but rebuilds nothing.

## View functions

A view function has the signature `func(ctx *gift.Context) gift.View`. It reads state through the context and
returns the views for the current values:

```go
func counter(ctx *gift.Context) gift.View {
	count := ctx.State("count", 0)
	n := ctx.Read(count)

	return ui.HStack(
		ui.Text(strconv.Itoa(n)).FontSize(32),
		ui.Button(ui.Text("+1"), func() { count.Set(count.Get() + 1) }),
	).Gap(12)
}
```

`ctx.State` returns the named state of this component instance and creates it with the initial value on the
first build. `ctx.Read` returns its value and records that this instance depends on it. When the button calls
`count.Set`, gift marks the instance as dirty and calls `counter` again in the next update. The new views
replace the old ones; the state stays.

Keep two rules in mind:

- **The context is valid only during the build that received it.** Do not keep it, and do not use it from an
  event handler or a goroutine; `ctx.State` and `ctx.Read` panic outside the build. Capture the `*gift.State` in
  the closure instead, as the button above does, and use `Get` and `Set` there.
- **A view function can run often.** It runs whenever a state it read changes and whenever an enclosing
  component is rebuilt, unless a [Memo](#build-boundaries-giftmemo) boundary skips it. Keep it cheap and free
  of work that belongs in an event handler or a background task; see [Concurrency](../concurrency/).

## Composing views

You compose views by passing them to other views. Containers such as `ui.VStack`, `ui.HStack` and `ui.ZStack`
take their children as arguments, and a button takes its label as a view:

```go
func greeting(ctx *gift.Context) gift.View {
	return ui.VStack(
		ui.Text("Hello").FontSize(24),
		ui.Text("This is gift.").Foreground(ui.ColorSecondaryLabel),
	).Gap(8).Padding(16)
}
```

To reuse a piece of interface, write an ordinary Go function that returns a view. It needs no context unless it
reads state:

```go
func header(title, subtitle string) gift.View {
	return ui.VStack(
		ui.Text(title).FontSize(20),
		ui.Text(subtitle).Foreground(ui.ColorSecondaryLabel),
	).Gap(4)
}

func settings(ctx *gift.Context) gift.View {
	return ui.VStack(
		header("Settings", "Changes apply at once"),
		ui.Divider(),
		ui.Text("..."),
	).Gap(12)
}
```

Such a helper is inlined into the build of its caller. It creates no node and no scope of its own; only the
views it returns become nodes.

### Lists of children

For a variable number of children, build a slice and pass it with `...`. Give each child a key that identifies
the item it shows, so that gift can match it again after the list changes (see
[Keys and reconciliation](#keys-and-reconciliation)):

```go
type Item struct{ ID, Title string }

func items(list []Item) gift.View {
	rows := make([]gift.View, 0, len(list))
	for _, it := range list {
		rows = append(rows, ui.Text(it.Title).Key(it.ID))
	}
	return ui.VStack(rows...).Gap(8)
}
```

{{< callout type="warning" >}}
`ui.VStack(rows...)` hands your slice to gift without copying it. From that moment its elements belong to gift:
do not overwrite them and do not reuse the slice as a buffer for the next build. `ui.VStack(a, b, c)` creates a
fresh slice and is always safe. Builds with the `giftdebug` tag panic when an element was replaced by a view of
another type, the usual symptom of a reused buffer.
{{< /callout >}}

### Optional children

A nil view in a list of children panics. To leave a child out, build the slice conditionally:

```go
func page(ctx *gift.Context) gift.View {
	showHint := ctx.Read(ctx.State("hint", true))

	children := []gift.View{ui.Text("Inbox").FontSize(20)}
	if showHint {
		children = append(children, ui.Text("Swipe to archive"))
	}
	children = append(children, ui.Text("3 messages"))
	return ui.VStack(children...).Gap(8)
}
```

A few containers accept nil on purpose and document it. For example, `ui.Modal(content, nil)` is a modal with
nothing presented; see [Modal](../../components/dialogs/modal/).

## The root view and ui.Window

Every application has exactly one root view function. You pass it to `gift.New` in `gift.Options.Root`, and a
backend such as `backend/ebiten` opens the window and drives the app:

```go
func main() {
	ui.SetDefaultFont(ui.MustFont(ui.FontQuery{Family: inter.Family}))

	app := gift.New(gift.Options{Root: root})
	if err := backend.Run(app, backend.Config{Title: "Hello", Width: 480, Height: 320}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func root(ctx *gift.Context) gift.View {
	return ui.Window(
		ui.VStack(
			ui.Text("Hello, gift").FontSize(24),
			gift.Component("counter", counter),
		).Gap(16).Padding(24),
	)
}
```

gift mounts the root function as a component with the key `root`, so it has a state scope like any other
component. `gift.New` builds nothing yet; the first update does. The goroutine that calls `gift.New` becomes the
UI goroutine of the app, and `backend.Run` must be called from it.

The root view must cover the whole window. gift draws exactly what the view tree describes, and the screen
starts every frame as transparent black. `ui.Window` puts your content on top of a box that fills the window
with `ui.ColorBackground`, so make it the outermost view of the root function.

{{< callout type="warning" >}}
Forgetting `ui.Window` fails silently. Nothing panics and nothing is logged, but every part of the window that
no view covers shows whatever was in the framebuffer before: a transparent hole on a desktop, old console
output on a kiosk. The test harness has `Harness.AssertOpaque` to catch it; see [Testing](../testing/).
{{< /callout >}}

`ui.Window` returns an ordinary `ui.Overlay`, so the alignment and modifiers of a [ZStack](../../components/layout/zstack/)
are available on it. See [Window](../../components/layout/window/) for the details.

## Components: gift.Component

`gift.Component(key, fn)` mounts a view function as a component instance with a state scope of its own. This is
the difference to a plain helper function, and it is the most common mistake in a first gift program.

A helper function that uses `ctx.State` creates the state in the scope of its **caller**. Two calls of the same
helper with the same state name share one state:

```go
// Wrong: both buttons show and change the same number.
func badCounter(ctx *gift.Context, label string) gift.View {
	count := ctx.State("count", 0) // belongs to the caller's scope
	n := ctx.Read(count)
	return ui.Button(ui.Text(label+": "+strconv.Itoa(n)), func() { count.Set(n + 1) })
}

func twoBad(ctx *gift.Context) gift.View {
	return ui.HStack(
		badCounter(ctx, "Left"),
		badCounter(ctx, "Right"), // same state as "Left"
	)
}
```

Wrap the function in `gift.Component` and each instance gets its own scope, and with it its own state:

```go
func labeledCounter(label string) gift.View {
	return gift.Component(label, func(ctx *gift.Context) gift.View {
		count := ctx.State("count", 0)
		n := ctx.Read(count)
		return ui.Button(ui.Text(label+": "+strconv.Itoa(n)), func() { count.Set(n + 1) })
	})
}

func twoGood(ctx *gift.Context) gift.View {
	return ui.HStack(
		labeledCounter("Left"),
		labeledCounter("Right"),
	).Gap(12)
}
```

State names are local to the instance, so both components can call their state `count`. The key identifies the
instance among its siblings and must stay the same from build to build. A component adds identity, not
appearance: it takes the size of the view its function returns and draws nothing of its own.

### What a component rebuilds

A component is a boundary in one direction only:

- **Bottom up**: a state write inside the component rebuilds this component and its subtree, not its parent.
- **Top down**: when the enclosing scope rebuilds, the component rebuilds too, every time and all the way down.
  Its inputs are captured in the closure `fn`, and gift cannot compare two functions to see whether they
  changed.

Two consequences follow from the second point. gift cannot see that you passed a different function under the
same key, so it keeps the existing instance and its state, and calls the new function. And a tree of plain
components under a root that rebuilds often rebuilds completely each time. When that matters, use `gift.Memo`.

`ctx.Path()` returns the slash separated keys from the root to the current instance, for example
`/root/counter`. gift uses this path in its panic messages, and you can use it in your own diagnostics.

## Build boundaries: gift.Memo

`gift.Memo(key, props, fn)` is a component that takes its inputs as an explicit value instead of a closure. The
props type must be `comparable`. When the parent rebuilds, gift compares the new props with the props of the
last build using `==`. If they are equal and no state this instance read has changed, gift does not call `fn`
and leaves the whole subtree untouched.

```go
type rowProps struct {
	Title    string
	Selected bool
}

func renderRow(ctx *gift.Context, p rowProps) gift.View {
	t := ui.Text(p.Title)
	if p.Selected {
		t = t.Foreground(ui.ColorAccent)
	}
	return t
}

func list(ctx *gift.Context) gift.View {
	sel := ctx.Read(ctx.State("selected", "b"))
	all := []Item{{"a", "Apples"}, {"b", "Bananas"}, {"c", "Cherries"}}

	rows := make([]gift.View, 0, len(all))
	for _, it := range all {
		rows = append(rows, gift.Memo(it.ID, rowProps{Title: it.Title, Selected: it.ID == sel}, renderRow))
	}
	return ui.VStack(rows...).Gap(4)
}
```

When the selection moves from `b` to `c`, `list` rebuilds, but only the rows of `b` and `c` call `renderRow`
again. The row of `a` gets equal props and is skipped.

A struct of exactly the values the component reads is the intended shape of the props. A pointer to a state is
a good prop too: the pointer stays the same across builds, so the props stay equal, and the child reads the
state itself. Only the child then depends on the state, so a write rebuilds the child and not the parent:

```go
type clockProps struct {
	Seconds *gift.State[int]
}

func clockFace(ctx *gift.Context, p clockProps) gift.View {
	s := ctx.Read(p.Seconds) // this instance depends on the state now
	return ui.Text(strconv.Itoa(s) + " s")
}

func dashboard(ctx *gift.Context) gift.View {
	secs := ctx.State("seconds", 0) // created here, not read here
	return ui.VStack(
		ui.Text("Dashboard").FontSize(20),
		gift.Memo("clock", clockProps{Seconds: secs}, clockFace),
	)
}
```

Pitfalls with props:

- **No slices, maps or functions.** A struct with such a field is not comparable and does not compile. An
  interface field that holds one compiles, but panics when gift compares the props. Keep such values in state,
  or pass a stable pointer to them.
- **Equal props mean no rebuild.** If the component reads something that is not in its props and not in a state
  it read with `ctx.Read`, for example a package variable, a change of it does not reach the component.
- **The props type is fixed for the lifetime of an instance.** A `Memo` with a different props type under the
  same key panics, and so does a `Memo` in the place of a `Component` with the same key, or the other way round.

You can observe the effect with [Diagnostics](../diagnostics/): `Diagnostics.Builds` counts the component
scopes that were rebuilt.

## Keys and reconciliation

After a view function returns, gift reconciles: it walks the new views and the existing nodes side by side and
decides, for every child, whether to update an existing node or to mount a new one. Updating keeps everything
attached to the node: its presentation state, such as focus or a scroll offset, and, for a component, its scope
and state. Mounting starts from scratch. Every old node that no new view matched is unmounted, together with
every scope and every state below it.

Children are matched among their siblings only, by these rules:

1. A child with a **key** is matched with the previous sibling that has the same key **and** the same view type,
   wherever that sibling was in the old list. A reordered list therefore keeps its state.
2. A child **without a key** is matched with the previous sibling at the **same position**, if that sibling has no
   key either and has the same view type.
3. A different view type never matches. A `ui.Text` in the place of a `ui.Button` remounts, even under the same
   key. `ui.VStack` and `ui.HStack` are different view types too, and so are `ui.VScroll` and `ui.HScroll`.

Every `ui` view has a `Key` modifier, and the first argument of `gift.Component` and `gift.Memo` is their key. A
key only has to be unique among the children of one parent, not globally.

### When you need a key

Position based matching is fine for a fixed structure. It goes wrong when children are inserted, removed or
reordered in front of a child that holds state. In the following stack a hint button disappears after a tap:

```go
func inbox(ctx *gift.Context, rows []gift.View) gift.View {
	hint := ctx.State("hint", true)

	var children []gift.View
	if ctx.Read(hint) {
		children = append(children, ui.Button(ui.Text("Got it"), func() { hint.Set(false) }))
	}
	children = append(children, ui.VScroll(rows...).Key("rows"))
	return ui.VStack(children...)
}
```

Without `.Key("rows")`, the scroll view would move from the second to the first position when the hint goes
away. At that position the old node is a button, the types differ, and gift would mount a fresh scroll view: the
user loses the scroll offset. With the key, the scroll view is matched by key and keeps its offset. The same
applies to a text field that holds the focus, and to anything else with presentation state.

Give a key:

- to every item of a list whose content changes, using a stable ID of the item and never its index,
- to a child that holds state and follows optional siblings,
- to every component and memo, which the API already asks for.

{{< callout type="info" >}}
Duplicate keys among siblings are not diagnosed in a release build: the duplicates are matched with each other
in the order they appear, which is position based matching again. Build with `-tags giftdebug` and gift panics
with the positions of the two children.
{{< /callout >}}

### Remounting on purpose

Because a different key means a different instance, you can reset a component by changing its key. A form keyed
by the ID of the record it edits starts with fresh state when the user opens another record:

```go
gift.Component("edit-"+record.ID, editForm)
```

### Hidden is not unmounted

Navigation containers keep screens you cannot see. An inactive tab of a [TabBar](../../components/navigation/tab-bar/)
and a covered screen of a [NavigationStack](../../components/navigation/navigation-stack/) stay mounted and are only
taken out of the frame: they are not painted, not hit tested and not in the focus order. Their state, scroll
offsets and half typed text survive. The price is memory, plus a build and a layout when something writes state
inside them. See [Navigation](../navigation/).

## Your own view types

When composition is not enough, for example for a control that draws itself, you implement `gift.View` directly.
Each concrete view type registers a process wide type ID with `gift.RegisterType`, which is what the reconciler
compares in rule 3 above, and `Build` returns an `Element` with a layouter, a painter and the children. `Build`
must be free of side effects, because gift calls it before it knows whether the result updates a node or mounts
a new one. See [Custom views](../custom-views/).

## API

### gift.View

{{< api pkg="gift" type="View" >}}

### gift.Context

{{< api pkg="gift" type="Context" >}}

### gift.Options

`gift.Options` has two fields: `Root func(*gift.Context) gift.View`, the root view function, and
`Logger *slog.Logger`, which is used outside the frame path only. A nil logger turns logging off.

### View types

{{< api pkg="gift" type="TypeID" >}}

{{< api-funcs pkg="gift" names="TypeName,RegisteredTypeNames" >}}

## Related

- [State](../state/): state, bindings and what triggers a rebuild.
- [Modifiers](../modifiers/): how views are configured.
- [Layout](../layout/): how the tree of nodes is measured and placed.
- [Custom views](../custom-views/): implementing `gift.View`.
- [Diagnostics](../diagnostics/): counting builds, layouts and live scopes.
- [Window](../../components/layout/window/), [Stack](../../components/layout/stack/),
  [Text](../../components/basic/text/), [Button](../../components/basic/button/).
- [Counter example](../../examples/example-counter/): a complete first program.
