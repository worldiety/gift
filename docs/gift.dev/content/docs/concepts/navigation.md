---
title: Navigation
weight: 10
---

Most applications have more than one screen. gift gives you three tools to move between them:

- a [NavigationStack](../../components/navigation/navigation-stack/) for drilling down from a list to a detail
  and back,
- a [TabBar](../../components/navigation/tab-bar/) for switching between a handful of top level screens,
- a [Modal](../../components/dialogs/modal/), an [Alert](../../components/dialogs/alert/) and a
  [Sheet](../../components/dialogs/sheet/) for content that appears on top of the current screen.

All of them follow one rule: **the navigation state is yours**. None of these views stores which screen is
showing, which tab is selected or whether a dialog is open. You keep that in [state](../state/), build the views
from it, and change it in event handlers. The views draw what you give them and report what the user asked for.
This page explains how that works, how "back" works, and the pitfalls that matter.

## Navigation state is application state

A navigation stack has no `Push` method and no history. A tab bar does not remember its selection. A modal has no
`Show` method. Instead:

| You want to          | You write                                              |
|----------------------|--------------------------------------------------------|
| push a screen        | a state change that adds one more `ui.Screen` to the list |
| pop a screen         | a state change that drops the last one                 |
| switch tabs          | a state change of the selected index                   |
| open or close a dialog | a state change of a `bool` that you pass to `Presented` |

This has three practical benefits. You can restore the navigation state, because it is a plain value. You can
deep link into any screen by setting that value directly. And you can refuse to leave a screen, for example one
with unsaved changes, simply by not changing the value.

The flip side: a tap on the back affordance or on a tab is only a **request**. gift calls your callback, and
nothing moves until your callback writes state and the next update rebuilds the view.

## NavigationStack

`ui.NavigationStack(onBack, screens...)` shows the last of `screens` under a title bar. Each screen is a
`ui.Screen(title, content)`. The bar shows the title of the top screen and, from the second screen on, a back
button with the title of the screen below it.

Build the list of screens from your state. A comparable struct is a good shape for a route, because state values
must be [comparable](../state/#values-and-comparison):

```go
type route struct {
	Folder  string // "" shows the list of folders
	Message string // "" shows the folder
}

func mail(ctx *gift.Context) gift.View {
	r := ctx.State("route", route{})
	cur := ctx.Read(r)

	screens := []ui.ScreenSpec{
		ui.Screen("Mail", folders(func(f string) { r.Set(route{Folder: f}) })),
	}
	if cur.Folder != "" {
		screens = append(screens, ui.Screen(cur.Folder, messages(cur.Folder, func(m string) {
			r.Set(route{Folder: cur.Folder, Message: m})
		})).Key("folder/"+cur.Folder))
	}
	if cur.Message != "" {
		screens = append(screens, ui.Screen("Message", message(cur.Message)).Key("message/"+cur.Message))
	}

	back := func() {
		c := r.Get()
		if c.Message != "" {
			c.Message = ""
		} else {
			c.Folder = ""
		}
		r.Set(c)
	}
	return ui.NavigationStack(back, screens...).BackIcon(outline.AngleLeft)
}
```

`folders`, `messages` and `message` are your own view functions. A push is `r.Set(...)` with one more field
filled in, a pop is `back` clearing the last one, and a deep link is `r.Set(route{Folder: "Inbox", Message: id})`
from anywhere.

Some details of the stack:

- **An empty list panics.** There is always at least the root screen.
- **The root screen has no back button.** An empty slot of `ui.ControlHitTarget` (44) pixels takes its place,
  matching an empty slot at the trailing edge, so the title of the root screen is centered in the bar.
- **`BackIcon` is optional.** Package `ui` cannot import an icon package, so without `BackIcon` the back button is
  the previous title alone. `outline.AngleLeft` is the usual choice. See [Icons](../icons/).
- **A nil `onBack` draws an inert back button.** It is still visible, as a [Button](../../components/basic/button/)
  with a nil action is.
- **The bar is `ui.NavigationBarHeight` (52) pixels tall.** It is a constant, not a modifier.
- **Push and pop animate.** A pushed screen slides in from the trailing edge while the covered one slides out to
  the leading edge. On a pop, the popped screen disappears at once and the revealed screen slides back in over a
  short distance, because a screen that you removed from the list is unmounted and has nothing left to animate.

### Screen identity

Each screen is matched with the previous build by its key, which defaults to the title. Two consequences:

- **Two screens with the same title need `Key`.** Otherwise they are duplicate siblings; see
  [Keys and reconciliation](../views/#keys-and-reconciliation).
- **A screen whose title changes is a new screen.** If the title shows something that changes while the screen is
  open, such as a count, give the screen a stable `Key`, or it remounts and loses its state.

The example above keys each screen by what it shows. Opening another folder therefore mounts a fresh folder
screen instead of reusing the state of the previous one.

### Covered screens stay mounted, popped screens do not

A screen below the top one stays mounted but is hidden: it is not painted, not hit tested and not in the focus
order. Its state, its scroll offset and half typed text survive, and when you pop back to it, it is exactly where
the user left it. A fling that was still running when the screen was covered is stopped with its offset intact.

A screen that you pop is gone. Its component scopes and every state inside them are dropped, and pushing it again
starts from scratch. If a screen has to come back with its old values, keep those values in state above the
navigation stack and pass them down.

Every screen in the list costs memory for its whole subtree. Ten levels of drill-down are ten live subtrees.
Because the list is yours, so is that cost: keep the stack as deep as the user can actually go.

## How back works

There are two ways to go back in a navigation stack, and both call your `onBack`:

1. **The back button** in the bar. It is a button of at least `ui.ControlHitTarget` on both axes with the
   accessible label "Back" and the key `back`, so a test can find it with `gifttest.ByKey("back")`; see
   [Testing](../testing/).
2. **The escape key.** It reaches the stack when the focus is inside it, and also when nothing is focused at all,
   which is the normal case on a touch panel. On the root screen the stack consumes escape and does nothing, so
   the key does not travel on and close something else.

There is no edge swipe and no system back button. A horizontal drag near the edge belongs to horizontally
scrolling content such as an [ImageGallery](../../components/lists/image-gallery/).

When stacks are nested, or there is one stack per tab, an unfocused escape goes to the last stack in document
order that is not hidden. A stack in an inactive tab or on a covered screen is never a candidate, so escape pops
the stack the user is looking at.

### Refusing to go back

Because back is a request, a screen with unsaved changes can ask first instead of popping:

```go
func editScreen(ctx *gift.Context) gift.View {
	depth := ctx.State("depth", 1)
	dirty := ctx.State("dirty", false)
	confirm := ctx.State("confirm", false)

	screens := []ui.ScreenSpec{ui.Screen("Documents", ui.Text("..."))}
	if ctx.Read(depth) == 1 {
		screens = append(screens, ui.Screen("Edit", editor(dirty)))
	}

	back := func() {
		if dirty.Get() {
			confirm.Set(true) // ask first, see the alert below
			return
		}
		depth.Set(0)
	}

	alert := ui.Alert("Discard your changes?", "",
		ui.AlertCancel("Keep editing", func() { confirm.Set(false) }),
		ui.AlertDestructive("Discard", func() {
			dirty.Set(false)
			confirm.Set(false)
			depth.Set(0)
		}),
	)
	return ui.Modal(ui.NavigationStack(back, screens...), alert).
		Presented(ctx.Read(confirm))
}
```

{{< callout type="warning" >}}
A modal does not handle the escape key, and its focus trap does not stop an unfocused escape from reaching a
navigation stack **underneath** it. With an alert open over a stack and nothing focused, escape calls the stack's
`onBack` and pops the screen behind the alert. If that matters, check in `onBack` whether a dialog is open. The
example above is safe: while the alert is open, `dirty` is still true, so `onBack` only asks again. A [Sheet](#sheet) does not have this problem: it handles escape itself.
{{< /callout >}}

## TabBar

`ui.TabBar(selected, onSelect, tabs...)` shows the content of `tabs[selected]` above a row of tab items along the
bottom edge. Each tab is a `ui.Tab(title, icon, content)`. The selected item is drawn in `ui.ColorAccent`, the
others in `ui.ColorSecondaryLabel`.

```go
func root(ctx *gift.Context) gift.View {
	tab := ctx.State("tab", 0)
	return ui.Window(
		ui.TabBar(ctx.Read(tab), tab.Set,
			ui.Tab("Home", outline.Home, gift.Component("home", home)).SelectedIcon(solid.Home),
			ui.Tab("Settings", outline.Cog, gift.Component("settings", settings)).SelectedIcon(solid.Cog),
		),
	)
}
```

- **`onSelect` is called with the index the user tapped.** Passing `tab.Set` directly is the common case. A tab
  that must not be left, such as an unsaved form, is a callback that does not store the new index.
- **An out-of-range `selected` shows the first tab** instead of panicking, because the index is
  application state and may be stale for one frame after you remove a tab. An empty tab list panics.
- **The icon may be the zero `ui.Symbol`.** The item then shows the caption alone. `SelectedIcon` sets a second
  symbol for the selected state, typically the solid variant of an outline icon.
- **A tab's identity is its title**, exactly as for a screen. Use `Key` when two tabs share a title, and when the
  title changes, for example to show a count.
- **The bar is `ui.TabBarHeight` (56) pixels tall**, and the items divide the width equally.
- **Switching animates.** When you select a tab to the right of the current one, the current tab slides out to
  the left and the new one slides in from the right, and the other way round.

### Inactive tabs stay mounted and are not drawn

Every tab is built before the first frame and stays mounted. An inactive tab is hidden: not painted, not hit
tested, not in the focus order. Animations that were running in it, such as a fling, a caret blink or the slide
of a control, end when it is hidden, and its images load at a lower priority than those of the visible tab.
Switching back rebuilds nothing; the tab is repainted with its state, scroll offsets and text intact.

What it does cost:

- **Memory and start-up time.** All tabs exist from the first frame. A tab bar is meant for a handful of screens.
- **State writes in a hidden tab still rebuild it.** A background task that keeps adding rows to the list in an
  inactive tab costs a build and a layout per change, just no paint. Pause such work yourself if it matters.

Wrap each tab's content in a [`gift.Component`](../views/#components-giftcomponent) that reads only the state of
that tab, as the example above does. If the root function reads the state of every tab, a write to any of them
rebuilds the root and therefore all tabs, including the hidden ones. The
[kitchen sink](../../examples/example-kitchensink/) measured a factor of ten between the two for one tap.

### Tabs and stacks together

For a drill-down inside a tab, put a navigation stack in the tab, one per tab that needs it. Each stack keeps its
own route in state, and the hidden tabs keep their position in the stack while the user is elsewhere. The other
way round, a tab bar as a screen of a stack, works too but hides the tab bar while a deeper screen is on top.

## Overlays: Modal, Alert and Sheet

An overlay is drawn on top of a screen and takes all input away from it. gift has two hosts for overlays:

- `ui.Modal(content, modal)` centers any view over `content`.
- `ui.Sheet(content, sheet)` slides a panel up from the bottom edge over `content`.

Both make three guarantees while the overlay is up:

1. **A scrim covers the whole host.** It swallows every pointer event outside the overlay, including the mouse
   wheel, so nothing underneath can be tapped or scrolled. It also dims the content with a wash of the label
   color; `Scrim(color)` changes it, and `ui.ColorClear` gives an invisible scrim that still blocks input.
2. **A focus trap keeps tab navigation inside the overlay.** The build that opens it takes the focus away from
   whatever held it outside. When overlays are stacked, the last one in document order traps the focus.
3. **The overlay is drawn last and hit tested first.**

The content underneath stays mounted and is still drawn, laid out and rebuilt. A modal is a layer, not a pause
button: timers and background work in the content keep running.

### Modal and Alert

`ui.Alert(title, message, buttons...)` is a card with a title, a message and a row of buttons. It knows nothing
about being modal; you hand it to `ui.Modal`. The buttons are `ui.AlertAction` (accent color),
`ui.AlertCancel` (label color) and `ui.AlertDestructive` (danger color).

```go
func files(ctx *gift.Context) gift.View {
	confirm := ctx.State("confirm", false)

	alert := ui.Alert("Delete the file?", "This cannot be undone.",
		ui.AlertCancel("Cancel", func() { confirm.Set(false) }),
		ui.AlertDestructive("Delete", func() {
			deleteFile()
			confirm.Set(false)
		}),
	)
	screen := ui.VStack(
		ui.Text("report.pdf"),
		ui.Button(ui.Text("Delete"), func() { confirm.Set(true) }),
	).Gap(12).Padding(24)

	return ui.Window(
		ui.Modal(screen, alert).
			Presented(ctx.Read(confirm)).
			OnDismiss(func() { confirm.Set(false) }),
	)
}
```

An alert button does not close the alert by itself. Every action has to set your state back, as both actions above
do. `OnDismiss` makes a tap on the scrim call your function; without it, a tap outside the card does nothing,
which is what an alert that demands an answer wants. Either way the tap never reaches the content.

`ui.Modal` takes any view, not only an alert. A custom dialog is a `ui.VStack` with a background, padding and a
corner radius, handed to `Modal` in the same way.

### Presented, or nil

There are two ways to close an overlay:

- **`Presented(false)`** keeps the overlay mounted and hidden. It animates out: the card sinks back down while the
  scrim fades, and its state survives until the next time it opens. This is the usual choice.
- **A nil overlay**, `ui.Modal(screen, nil)`, unmounts it. The closed host costs one node and no scrim, but the
  overlay disappears without an animation, because an unmounted view has nothing left to draw.

The default of `Presented` is `true`, so a non-nil overlay without `Presented` is open. Use nil for an overlay that
is expensive to keep around, and `Presented` for everything else.

### Sheet

A sheet is a modal whose panel is anchored to the bottom edge. It is as wide as the window up to `MaxWidth`
(default 720) and as tall as the fraction `Height` of it (default 0.9), and it slides in from below the window.

```go
func withSheet(ctx *gift.Context) gift.View {
	open := ctx.State("filters", false)

	screen := ui.Button(ui.Text("Filters"), func() { open.Set(true) })
	panel := ui.VStack(
		ui.Text("Filters").FontSize(20),
		ui.Button(ui.Text("Done"), func() { open.Set(false) }),
	).Gap(12).Padding(24)

	return ui.Window(
		ui.Sheet(screen, panel).
			Presented(ctx.Read(open)).
			OnDismiss(func() { open.Set(false) }).
			Height(0.5),
	)
}
```

`OnDismiss` is called by a tap on the scrim **and** by the escape key, whether or not anything inside the sheet is
focused. A sheet without `OnDismiss` can only be closed by your code. There is no drag to dismiss.

One exception: a navigation stack inside the sheet gets escape first, and on its root screen it consumes the key.
Escape then never reaches the sheet, so give such a sheet a button that closes it.

Inside a `ui.Window`, a sheet sits above gift's on-screen keyboard while the keyboard is showing and gets shorter
by its height, so a text field at the bottom of the sheet stays visible. See
[Keyboard](../../components/dialogs/keyboard/).

### Which overlay is on top

When you combine a sheet and a modal, the **outer** host draws above the inner one. To let an alert come up over
an open sheet, make the modal the outer host:

```go
func app(ctx *gift.Context) gift.View {
	tab := ctx.State("tab", 0)
	sheet := ctx.State("sheet", false)
	alert := ctx.State("alert", false)

	tabs := ui.TabBar(ctx.Read(tab), tab.Set,
		ui.Tab("Home", outline.Home, gift.Component("home", home)),
		ui.Tab("Settings", outline.Cog, gift.Component("settings", settings)),
	)
	sheetView := ui.Text("sheet")
	alertView := ui.Alert("Sure?", "", ui.AlertCancel("Cancel", func() { alert.Set(false) }))

	return ui.Window(
		ui.Modal( // outermost: drawn above everything, also above the sheet
			ui.Sheet(tabs, sheetView).
				Presented(ctx.Read(sheet)).
				OnDismiss(func() { sheet.Set(false) }),
			alertView,
		).Presented(ctx.Read(alert)),
	)
}
```

Put overlay hosts outside the tab bar, as here. A modal inside one tab covers only that tab's content area, and the
user can still switch tabs and walk away from it.

## Give navigation views the whole area

`TabBar`, `NavigationStack`, `Modal` and `Sheet` are screens: they fill the area they are given. They panic when
they are measured with an unbounded axis instead of shrinking (a `Modal` or `Sheet` only while its overlay is not
nil), because a scrim of zero size would let every tap
through a dialog that is visible on the screen. You get an unbounded axis in two places:

- **inside a scroll container**, where a screen does not belong,
- **as an inflexible child of a stack**: `ui.VStack(header, nav)` measures `nav` with an unbounded height. Write
  `ui.VStack(header, nav.Flex(1))` instead, or give it a `Frame` with a finite size on both axes.

The safe place is the one the examples above use: directly inside `ui.Window`, or as the content of a tab or a
screen, which already have a bounded size. See [Layout](../layout/).

## API

The full API of each view is on its component page. The specs you pass to the containers:

### ui.ScreenSpec

{{< api "ScreenSpec" >}}

### ui.TabSpec

{{< api "TabSpec" >}}

### ui.AlertButton

{{< api "AlertButton" >}}

## Related

- [NavigationStack](../../components/navigation/navigation-stack/), [TabBar](../../components/navigation/tab-bar/),
  [Sheet](../../components/dialogs/sheet/), [Modal](../../components/dialogs/modal/),
  [Alert](../../components/dialogs/alert/): the component pages with screenshots.
- [State](../state/): how the navigation state is stored and what a write rebuilds.
- [Views](../views/#hidden-is-not-unmounted): why hidden screens keep their state.
- [Input and focus](../input-and-focus/): focus order, focus traps and key delivery.
- [Kitchen sink example](../../examples/example-kitchensink/): a tab bar, a navigation stack in the first tab, an
  alert and a sheet in one program.
