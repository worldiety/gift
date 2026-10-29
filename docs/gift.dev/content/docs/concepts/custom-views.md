---
title: Custom Views
weight: 13
---

Every view in package `ui` is an ordinary Go type that implements `gift.View`. You can write your own the same
way. A custom view decides its own size, draws itself with drawing operations and, if you want, reacts to input.
This page walks through a complete example, a meter that doubles as a slider, and explains the contracts each
part has to keep.

## When you need one

Most of the time you do not. Try these first:

- A **helper function** that returns a view composes existing views. It creates no node of its own; see
  [Views](../views/#composing-views).
- A **component** (`gift.Component`, `gift.Memo`) adds state and a rebuild boundary; see [State](../state/).
- **Styling** covers backgrounds, borders, shadows, gradients and glass on the built-in views; see
  [Modifiers](../modifiers/) and [Theming](../theming/).

Write a custom view when you need something that composition cannot express: a shape the built-in views do not
draw, a layout the stacks do not offer, or a control with its own gesture.

## The two halves

A custom view has two halves:

- The **view** is a small value type. It holds the configuration and implements `gift.View`. It lives for one
  build, like every view.
- The **node** is the object `Build` returns inside a `gift.Element`. gift keeps it in the retained tree until the
  next build of that view and calls it during layout, paint and input.

```go
type View interface {
	ViewType() TypeID
	Build(*BuildContext) Element
}
```

`ViewType` returns the process wide ID of your type. `Build` describes the node. gift calls `Build` during
reconciliation and never during paint. `Build` gets a `*gift.BuildContext`; most views ignore it.

The `gift.Element` that `Build` returns carries the node and everything gift needs to know about it. These are
the fields a custom view uses most:

| Field        | Meaning                                                                                         |
|--------------|-------------------------------------------------------------------------------------------------|
| `Key`        | stable identity among siblings, see [keys](../views/#keys-and-reconciliation)                    |
| `Label`      | what a person would call the node; read by tests, never drawn                                  |
| `Layouter`   | measures the node and places its children; nil means zero size                                 |
| `Painter`    | emits the drawing operations; nil paints the children and nothing else                         |
| `Children`   | the child views, in order; the slice belongs to gift after `Build` returns                     |
| `Flex`       | share of the leftover space in an enclosing stack, see [Layout](../layout/)                     |
| `Interactor` | receives input; nil makes the node transparent to input                                        |
| `Focusable`  | takes part in the tab order; only honored together with an `Interactor`                        |
| `Disabled`   | stays a hit target but receives no events and no focus                                         |
| `Clip`       | confines the children to the node's bounds, for paint and input alike                          |

The struct has more fields for scrolling, transitions, layers and focus traps. Package `ui` uses them for its
containers; see [`gift.Element`](https://pkg.go.dev/github.com/worldiety/gift#Element) for the full list.

## A meter

The example is a horizontal bar that shows a fraction between 0 and 1. It is about 60 lines and uses every part
except children.

### Register the type

Each concrete view type registers a name once, in a package level variable:

```go
var meterType = gift.RegisterType("myapp.Meter")
```

`gift.RegisterType` returns a `gift.TypeID`. The reconciler compares these IDs to decide whether an existing node
can be updated: a different `ViewType` under the same key discards the node and everything below it, state
included. Choose a name that is unique in the process, for example prefixed with your package. Registering the
same name twice panics. The IDs are assigned in registration order, so never persist them.

### The view

The view is a value with unexported fields and modifiers that return a changed copy, the same pattern package
`ui` uses (see [Modifiers](../modifiers/)):

```go
// MeterView is a horizontal bar that shows a fraction between 0 and 1.
type MeterView struct {
	value    float32
	tint     ui.Color
	key      string
	onChange func(float32)
}

// Meter returns a meter at value, tinted with the accent color.
func Meter(value float32) MeterView {
	return MeterView{value: value, tint: ui.ColorAccent}
}

// Tint sets the color of the filled part. It may be a semantic color.
func (v MeterView) Tint(c ui.Color) MeterView { v.tint = c; return v }

// Key sets the reconciliation key among siblings.
func (v MeterView) Key(k string) MeterView { v.key = k; return v }

func (v MeterView) ViewType() gift.TypeID { return meterType }
```

### Build

`Build` turns the configuration into a node. Do the work that depends on the view here, once per build, and store
the results in the node so that layout and paint only read them:

```go
func (v MeterView) Build(*gift.BuildContext) gift.Element {
	n := &meterNode{
		value:    min(max(v.value, 0), 1),
		fill:     ui.ResolveColor(v.tint),
		track:    ui.ResolveColor(ui.Fade(ui.ColorLabel, 0.12)),
		ring:     ui.ResolveColor(ui.ColorAccent),
		onChange: v.onChange,
	}
	e := gift.Element{Key: v.key, Label: "meter", Layouter: n, Painter: n}
	if v.onChange != nil {
		e.Interactor = n
		e.Focusable = true
	}
	return e
}

// meterNode is the retained half: gift keeps it until the next build.
type meterNode struct {
	value             float32
	fill, track, ring ui.Color
	onChange          func(float32)
}
```

One object can be layouter, painter and interactor at once, which is what the views of package `ui` do too.

`Build` must be free of side effects: gift may call it before it has decided whether the element updates an
existing node or mounts a new one. It creates a fresh node on every build, so the node cannot remember anything
from one build to the next. State that has to outlive a build belongs in your application state, or in the
retained control state described [below](#state-that-outlives-a-build).

### Resolve colors in Build

`ui.ColorAccent`, `ui.ColorLabel` and the other roles are semantic colors: they name a role, not a value, and
only `ui.ResolveColor` turns them into the color of the installed theme. Every view in package `ui` resolves its
colors in `Build`. Your view has to do the same.

{{< callout type="warning" >}}
A semantic color that reaches a painter unresolved draws nothing. It is encoded with an alpha of zero, so the
widget lays out and hit-tests correctly and stays invisible, without an error in a release build.
{{< /callout >}}

`ui.ResolveColor` returns literal colors unchanged, so call it on every color, including the ones a caller passed
in. Resolving in `Build` is also what makes your view follow a theme switch: `ui.SetTheme` rebuilds every view,
and a rebuild resolves again. Do not resolve into a package variable, which would freeze the theme that was
installed at startup; `ui.Fade` keeps a semantic color semantic, so `ui.Fade(ui.ColorLabel, 0.12)` is safe
anywhere. See [Theming](../theming/).

While you develop, build with the `giftdebug` tag. The display list then panics when an operation carries a
color with a negative channel, which is what an unresolved semantic color looks like. Your painter still has to
emit the operation for the check to see it; see [Diagnostics](../diagnostics/).

### Layout

A `gift.Layouter` gets the constraints from its parent and returns its size:

```go
func (n *meterNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	w := float32(160) // the width it wants when nobody bounds it
	if c.HasBoundedWidth() {
		w = c.Max.W
	}
	return c.Constrain(geom.Sz(w, 8))
}
```

The meter fills the width it is offered and is 8 logical pixels tall. Two things matter here:

- **Handle an unbounded axis.** A stack measures its children with an unbounded main axis, and a scroll view does
  the same along its axis. `c.Max.W` is then infinite, so a layouter that always returns `c.Max.W` has no valid
  size. The meter falls back to a width of its own.
- **Return a size that satisfies the constraints.** `c.Constrain` clamps it. gift passes a size that violates the
  constraints through unchanged, so the bug shows up in the layout of the parent.

A layouter must be a pure function of its constraints, its children and the view it came from. gift caches the
size per node and skips `Layout` as long as neither the constraints nor anything below the node changed. A
layouter that read the clock or a global would show stale sizes. See [Layout](../layout/) for the constraint
model.

### Paint

A `gift.Painter` appends drawing operations to the display list of the frame:

```go
func (n *meterNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	r := b.Height() / 2
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: b, Color: n.track, CornerRadius: r})
	if w := b.Width() * n.value; w > 0 {
		bar := geom.RcXYWH(b.Min.X, b.Min.Y, w, b.Height())
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: bar, Color: n.fill, CornerRadius: r})
	}
	if ctx.Interaction().FocusVisible {
		ring := b.Outset(geom.InsetsAll(3))
		ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: ring, Color: n.ring,
			CornerRadius: r + 3, StrokeWidth: 2})
	}
}
```

`ctx.Bounds()` is the rectangle the last layout gave the node, in the node's local space. `ctx.Add` fills in the
active clip and transform, so you never deal with the scroll offset of a container above you. A `render.Op` is
plain data; its `Kind` selects which fields count:

| Kind                       | Draws                                                          |
|----------------------------|----------------------------------------------------------------|
| `render.OpFillRect`        | `Bounds` filled with `Color`                                   |
| `render.OpFillRoundRect`   | the same with `CornerRadius`                                   |
| `render.OpStrokeRoundRect` | the outline, `StrokeWidth` wide, inside `Bounds`               |
| `render.OpShadow`          | a blurred rounded shape, `Blur` is the blur diameter           |

Text, images and materials have operation kinds too, but they need side tables and resources; for those, compose
`ui.Text`, `ui.Image` or a styled `ui.Box` instead of drawing them yourself.

The paint contract is stricter than the others. `Paint` runs once per drawn frame for every visible node, so it
must not allocate, must not log and must not change any state. Read what `Build` and `Layout` prepared and emit
operations, nothing else.

### Use it

The meter is now a view like any other:

```go
func volume(ctx *gift.Context) gift.View {
	level := ctx.State("level", float32(0.6))
	return ui.VStack(
		ui.Text("Volume"),
		Meter(ctx.Read(level)).OnChange(level.Set),
	).Gap(8).Padding(16)
}
```

Your type has only the modifiers you write. `Padding`, `Frame` and `Background` come from package `ui` and are not
available on it. Wrap the view in a [stack](../../components/layout/stack/) instead, as the example does with
`ui.VStack(...).Padding(16)`.

## Input

A node takes part in hit testing exactly when its element carries a `gift.Interactor`. The meter installs itself
only when the caller passed `OnChange`, so a read-only meter lets clicks fall through to what is behind it.

```go
// OnChange makes the meter adjustable and reports every new value.
func (v MeterView) OnChange(fn func(float32)) MeterView { v.onChange = fn; return v }

// HandleEvent makes the meter a slider: press or drag sets the value, the
// arrow keys step it.
func (n *meterNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	st := ctx.ControlState() // survives the rebuild every onChange causes
	switch e.Kind {
	case gift.EventPointerDown:
		ctx.RequestFocus()
		st.Grabbed = true
		ctx.SetControlState(st)
		n.setFromPointer(ctx, e)
		return true
	case gift.EventPointerMove:
		if st.Grabbed {
			n.setFromPointer(ctx, e)
			return true
		}
	case gift.EventPointerUp, gift.EventPointerCancel:
		if st.Grabbed {
			st.Grabbed = false
			ctx.SetControlState(st)
			return true
		}
	case gift.EventKeyDown:
		switch e.Key {
		case gift.KeyLeft:
			n.onChange(max(n.value-0.1, 0))
			return true
		case gift.KeyRight:
			n.onChange(min(n.value+0.1, 1))
			return true
		}
	}
	return false
}

func (n *meterNode) setFromPointer(ctx *gift.EventContext, e gift.Event) {
	b := ctx.DeviceBounds() // the space Event.Pos lives in
	f := (e.Pos.X - b.Min.X) / b.Width()
	n.onChange(min(max(f, 0), 1))
}
```

How events reach the node:

- **Pointer events** go to the node under the pointer. The node that gets a press captures the pointer and receives
  every move and the release, even outside its bounds; `e.Inside` says where the pointer is and `e.Dragged` whether
  it moved further than `gift.DragSlop`. A tap is an up with `Inside` true and `Dragged` false.
- **Key events** go to the focused node. The meter asks for the focus on a press with `ctx.RequestFocus()`, which
  only works because `Build` set `Focusable`.
- **Bubbling.** Return false for an event you do not handle. An unhandled pointer or key event bubbles to the
  nearest ancestor with an interactor, which is how a scroll view still sees a wheel event over your view. Enter,
  leave, focus and cancel events do not bubble.
- **Cancel.** `gift.EventPointerCancel` means the gesture ended without a release, for example because a scroll
  view took the pointer over. End the gesture as the meter does, but do not activate anything as you might on an
  up.

`HandleEvent` runs in the update phase, never during paint, so calling the application's callback from it is the
normal way to react; the callback writes state and gift rebuilds the view with the new value. Keep the handler free
of allocations of its own. `Event.Repeat` marks a key event that gift synthesized because a key is held; a
handler that activates something, such as a button on enter, should ignore it.

### Hover, press and focus

You do not track hover, press and focus yourself. gift keeps them in the retained node as a `gift.Interaction` and
marks the node for repaint when they change. The painter reads them with `ctx.Interaction()`, as the focus ring of
the meter does. No view is rebuilt when the mouse moves over it.

`Pressed` is only set while the captured pointer is inside the node. It clears when a press wanders off and
comes back when the pointer returns, which suits a button. A drag has to go on outside the bounds, so the meter
tracks it with the grab flag of its control state instead, see [below](#state-that-outlives-a-build).

Draw a focus ring from `FocusVisible`, not from `Focused`: `FocusVisible` is only set when the focus arrived from
the keyboard, so a tap with a finger does not leave a ring behind. `Focused` is what decides where key events go.
See [Input and focus](../input-and-focus/).

If your interactor changes something the painter reads without a rebuild, tell gift: `ctx.Repaint()` when only the
picture changed, `ctx.RequestLayout()` when a layout input changed. Neither runs a build. The meter needs neither,
because every change goes through its callback and comes back as a rebuild.

### State that outlives a build

A rebuild replaces the node object, so a gesture that remembers something in the node forgets it on the next
build. A slider that rebuilds on every move would lose its grab offset between two moves of the same finger. For
this, gift keeps a `gift.ControlState` per node that survives rebuilds: a grab flag and offset, an animation from
`From` to `Target` with a `Start` time, and the touch position `At`.

- An interactor reads and writes it with `ctx.ControlState()` and `ctx.SetControlState(...)`. Writing it repaints
  and does not rebuild. The meter sets `Grabbed` on the press and clears it on the up or cancel, so every move in
  between still knows it belongs to the drag, although each one rebuilds the meter.
- A layouter may write it with `LayoutContext.SetControlState`, which is how a control notices that a build changed
  its value and starts an animation with `LayoutContext.Animate`.
- A painter only reads it, with `PaintContext.ControlState()` and `PaintContext.Now()` to interpolate.

Every animation enrollment takes a duration, a deadline after which gift stops repainting the node. See
[Animation](../animation/).

## Children

A view with children lists them in `Element.Children`. Its layouter measures and places them, and its painter
decides where in its own drawing order they appear. This panel paints a rounded plate behind its content:

```go
var panelType = gift.RegisterType("myapp.Panel")

// PanelView paints a rounded plate behind its content.
type PanelView struct {
	content gift.View
	color   ui.Color
}

func Panel(content gift.View) PanelView {
	return PanelView{content: content, color: ui.ColorSurface}
}

func (v PanelView) ViewType() gift.TypeID { return panelType }

func (v PanelView) Build(*gift.BuildContext) gift.Element {
	n := &panelNode{color: ui.ResolveColor(v.color), pad: 12}
	return gift.Element{
		Layouter: n,
		Painter:  n,
		Children: []gift.View{v.content}, // a fresh slice, owned by gift from here on
	}
}

type panelNode struct {
	color ui.Color
	pad   float32
}

func (n *panelNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	in := geom.InsetsAll(n.pad)
	child := ctx.Measure(0, c.Deflate(in).Loosen())
	ctx.Place(0, geom.Pt(n.pad, n.pad))
	return c.Constrain(child.Outset(in))
}

func (n *panelNode) Paint(ctx *gift.PaintContext) {
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: ctx.Bounds(), Color: n.color, CornerRadius: 10})
	ctx.PaintChildren() // without this call the content is not drawn
}
```

The rules for containers:

- **Measure and place every child exactly once.** `ctx.Measure(i, c)` lays out child `i` and returns its size,
  `ctx.Place(i, p)` puts it at an offset relative to your node. A child you do not measure keeps its previous size,
  one you do not place keeps its previous position. Measuring twice is not an error, but it doubles the cost of
  the subtree.
- **A painter is responsible for its children.** With a non-nil `Painter`, children are drawn only where you call
  `ctx.PaintChildren()` or `ctx.PaintChild(i)`. A nil `Painter` paints all children in order and nothing else,
  which is the right choice for a purely structural container.
- **The children slice belongs to gift.** Do not modify or reuse it after `Build` returns. Builds with the
  `giftdebug` tag detect a violation.
- **Clip with the flag.** Set `Element.Clip` to confine the children to your bounds. gift applies it to paint and
  input together, around the children and not around your own drawing, so a shadow can still extend past the
  bounds. If you push a clip yourself with `PaintContext.PushClip`, pop it before `Paint` returns; an unbalanced
  clip panics.

[Layout](../layout/#writing-your-own-layout) has another layouter example and the full `LayoutContext` API,
including baselines and overflow reports.

## Testing

[gifttest](../testing/) finds your view by its registered type name and drives it without a window:

```go
func TestMeterSteps(t *testing.T) {
	var got float32
	h := gifttest.New(t, gifttest.Options{
		View: Meter(0.5).OnChange(func(v float32) { got = v }),
	})
	h.Find(gifttest.ByType("myapp.Meter")).PressKey(gift.KeyRight)
	if got != 0.6 {
		t.Fatalf("got %v, want 0.6", got)
	}
}
```

`gifttest.ByText` matches `Element.Label`, so set a label that names what the node stands for if tests or other
tools should find it by what it says.

## Checklist

- One `gift.RegisterType` per concrete view type, with a unique name, in a package level variable.
- `Build` has no side effects, resolves every color with `ui.ResolveColor` and stores the results in a fresh node.
- `Layout` handles unbounded constraints, returns `c.Constrain(...)` and measures and places every child once.
- `Paint` does not allocate, log or change state, calls `PaintChildren` when it has children and pops every clip
  and transform it pushed.
- An `Interactor` returns false for what it does not handle, asks for the focus only when `Focusable` is set and
  keeps gesture state in `ControlState`, not in the node.
- Develop and test with the `giftdebug` tag.

## API

### gift.View

{{< api pkg="gift" type="View" >}}

{{< api pkg="gift" type="TypeID" >}}

### Layout

{{< api pkg="gift" type="Layouter" >}}

{{< api pkg="gift" type="LayoutContext" >}}

### Paint

{{< api pkg="gift" type="Painter" >}}

{{< api pkg="gift" type="PaintContext" >}}

### Input

{{< api pkg="gift" type="Interactor" >}}

{{< api pkg="gift" type="EventContext" >}}

### Colors

{{< api "Color" >}}

## Related

- Concepts: [Views](../views/), [Layout](../layout/), [Theming](../theming/), [Input and focus](../input-and-focus/),
  [Animation](../animation/), [Testing](../testing/), [Diagnostics](../diagnostics/)
- Components built the same way: [Spacer](../../components/layout/spacer/), [Divider](../../components/basic/divider/),
  [Toggle](../../components/basic/toggle/), [Slider](../../components/basic/slider/)
- Source: [ui/spacer.go](https://github.com/worldiety/gift/tree/main/ui/spacer.go) is the smallest view in package
  `ui` with a layouter of its own,
  [ui/divider.go](https://github.com/worldiety/gift/tree/main/ui/divider.go) a leaf with its own painter and
  [ui/toggle.go](https://github.com/worldiety/gift/tree/main/ui/toggle.go) a control with an interactor.
