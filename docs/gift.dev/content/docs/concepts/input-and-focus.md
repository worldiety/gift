---
title: Input and Focus
weight: 9
---

gift turns mouse, touch and keyboard input into events and delivers each event to one node of the retained tree.
A pointer event goes to the node under the pointer. A key event goes to the node that holds the keyboard focus. The
built-in controls handle all of this for you: a [button](../../components/basic/button/) lights up on hover, fires
on a tap, takes the focus and reacts to space and enter, and you only write the action.

This page explains what happens underneath: how gift finds the node under the pointer, how a press is tracked until
the release, where hover, press and focus are kept, how the focus moves, how text, the clipboard and the on-screen
keyboard work, and what to watch out for when you write a control of your own.

## The input pipeline

Input is resolved once per tick, in the update phase, before gift builds and lays out. The
[Ebitengine backend](https://github.com/worldiety/gift/tree/main/backend/ebiten) polls the mouse, the touches and
the keyboard, detects the changes since the last tick and calls the matching methods of `gift.App`:
`PointerDown`, `PointerMove`, `PointerUp`, `PointerWheel`, `KeyDown`, `KeyUp` and `TypeRune`. gift then
dispatches the event to an **interactor**, the input half of a node.

Because input runs before the build, an event handler can write [state](../state/) directly. The rebuild it causes
is part of the same tick. Calling one of the input methods while gift paints panics.

Who receives an event depends on its kind:

| Event                                   | Delivered to                                     | Bubbles to ancestors |
|-----------------------------------------|--------------------------------------------------|----------------------|
| `EventPointerDown`                      | the topmost interactive node under the pointer   | yes                  |
| `EventPointerMove`, `EventPointerUp`    | the node that took the press, inside or not      | yes                  |
| `EventPointerMove` with no button down  | the node under the mouse                         | yes                  |
| `EventPointerCancel`, `EventLongPress`  | the node that took the press                     | no                   |
| `EventPointerEnter`, `EventPointerLeave`| the node the mouse moved onto or off             | no                   |
| `EventWheel`                            | the node under the pointer                       | yes                  |
| `EventKeyDown`, `EventKeyUp`, `EventRune` | the focused node                               | yes                  |
| `EventFocusGained`, `EventFocusLost`    | the node that gained or lost the focus           | no                   |

"Bubbles" means: when the receiving interactor returns `false`, gift offers the event to the nearest ancestor that
has an interactor, and so on up to the root. That is how a [scroll view](../../components/layout/scroll-view/)
sees a wheel event that happened over a label inside it. An event that nobody handles is dropped.

## Hit testing

A node takes part in hit testing only when its element declares an interactor. Everything else is transparent to
input. A plain `VStack` is not a hit target: a click in its padding or in the gap between two children hits
whatever is behind it, not the stack.

gift walks the retained tree front to back and returns the first interactive node whose bounds contain the point:

- **The last child wins.** Children are tested in reverse drawing order, so the child drawn on top gets the hit.
  Overlapping children of a [ZStack](../../components/layout/zstack/) resolve the same way.
- **A child wins over its parent.** A container only gets the hit where none of its children did.
- **Clips apply.** A node inside an ancestor with `Clip(true)` or inside a scroll view cannot be hit outside the
  visible part of that ancestor. Scroll offsets and transforms are applied exactly as they are for painting.
- **The hit area is the rectangle.** Rounded corners are not followed: a click in the corner of a rounded button
  hits the button.
- **Disabled nodes still block.** A disabled control is a hit target, so a click on it does not fall through to
  a sibling drawn behind it. Its own interactor receives no events; a pointer or key event that hits it bubbles on
  to its ancestors. It is skipped by the focus order, and the controls of package ui draw their disabled look
  whatever the hover and press state.
- **Hidden subtrees do not exist for input.** An inactive tab of a [TabBar](../../components/navigation/tab-bar/)
  and a covered screen of a [NavigationStack](../../components/navigation/navigation-stack/) are not hit tested
  and not in the focus order.

The scrim of a [Modal](../../components/dialogs/modal/) uses these rules. It is a window-sized interactive node on
top of the content and swallows every pointer event, so a tap next to an [Alert](../../components/dialogs/alert/)
cannot reach the button behind it.

`App.HitTest(p)` runs the same walk and returns a `gift.NodeRef`. It exists for tests and diagnostics.

## Pointers, capture and gestures

Mouse and touch go through the same events and the same rules. Each event carries the `Pointer` id and the
`Device`, `gift.PointerMouse` or `gift.PointerTouch`. Positions are in window coordinates, in logical pixels:
the space that `EventContext.DeviceBounds` returns. `EventContext.Bounds` is the node's rectangle before the scroll
offsets of its ancestors, so compare `Event.Pos` with `DeviceBounds`.

### Capture

The node that takes a press **captures** the pointer. Every move, the release and a cancel go to that node until
the pointer comes up, even when the pointer has left the node. Two fields of `gift.Event` tell the node what
happened:

- `Inside` reports whether the position is inside the node, with the clips of its ancestors taken into account.
- `Dragged` reports whether the pointer moved further than `gift.DragSlop` (8 logical pixels) from where it went
  down.

A tap is an `EventPointerUp` with `Inside` true and `Dragged` false. This is why pressing a button, sliding off it
and releasing next to it does not activate it: the button hears the release, but with `Inside` false. While the
pointer is outside, the button also loses its pressed look, and gets it back when the pointer returns.

### Gestures gift recognizes

- **Long press.** When a pointer stays down for `gift.LongPressDelay` (500 ms) without moving further than
  `DragSlop`, the capturing node receives one `EventLongPress`.
- **Wheel.** `EventWheel` carries the scroll amount in `Delta`. It goes to the node under the pointer and bubbles
  until a scroll container that can still move in that direction consumes it.
- **Drag to scroll.** A scroll container watches the moves that bubble up from its children. When a drag that
  started on a button inside it passes the slop, the container takes the pointer with
  `EventContext.StealPointer`. The button receives `EventPointerCancel`, loses its pressed look and never sees the
  release, so it cannot fire.

`EventPointerCancel` means the gesture ended without a release. A node must treat it as a reset, never as an
activation.

### Limits

- Only the primary mouse button drives the pointer model. The right and middle buttons produce no events.
- gift tracks one touch at a time. A second finger that comes down while the first is still down is dropped and
  counted in `Diagnostics.DiscardedTouches`. There is no pinch or multi-finger gesture.

## Hover, press and focus: `gift.Interaction`

Hover, press and focus are presentation state that gift keeps in the retained node, not in your view. The
dispatcher updates a [`gift.Interaction`](https://pkg.go.dev/github.com/worldiety/gift#Interaction) and marks the
node for repaint; the painter reads it back.

| Field          | Set while                                                                     |
|----------------|-------------------------------------------------------------------------------|
| `Hover`        | a mouse is over the node and no other node has captured the pointer           |
| `Pressed`      | a pointer this node captured is down and inside it                            |
| `Focused`      | the node holds the keyboard focus                                             |
| `FocusVisible` | the node holds the focus and the focus came from the keyboard                 |
| `Disabled`     | the node is disabled                                                          |

Two consequences matter for application code:

- **Hover and press rebuild nothing.** Moving the mouse across a toolbar runs no view function and measures
  nothing. You never store hover or press in your own state. You describe the looks instead, for example with
  `ButtonView.HoverStyle`, `PressedStyle` and `DisabledStyle`; see [Button](../../components/basic/button/).
- **The state survives rebuilds.** A rebuild replaces the view, not the node, so a button that is pressed while
  some state elsewhere changes stays pressed.

While a pointer is down, hover is frozen. A drag that passes over other buttons does not light them up, whether
or not the press captured anything.

## Touch input

Touch follows the same model with one difference: **a touch never produces hover**. A finger exists only while it
is down, so it sends no `EventPointerEnter` or `EventPointerLeave` and never sets `Interaction.Hover`. A tap on a
touchscreen therefore leaves no hover highlight behind on the button that was tapped.

{{< callout type="warning" >}}
Do not decide behavior on `Event.Device`. Ebitengine reports touches only on mobile platforms. On Linux under
X11, which is what a Raspberry Pi kiosk runs, a touchscreen arrives through the display server **as a mouse**:
every event is `PointerMouse`, and because the cursor stays where the finger lifted, the last tapped control
keeps its hover look until the pointer goes somewhere else. A
control that only worked for `PointerTouch` would never work on that hardware. Write controls that behave
correctly for both, and use explicit switches such as [`ui.SetOnScreenKeyboard`](#the-on-screen-keyboard) for
behavior that depends on the deployment.
{{< /callout >}}

## Keyboard focus

Key events go to the node that holds the keyboard focus. A node can hold it when its element declares an
interactor **and** `Focusable`, and it is not disabled. In package ui these controls are focusable:
[Button](../../components/basic/button/) (and everything built on it, such as tappable
[rows](../../components/lists/row/) and the items of a [TabBar](../../components/navigation/tab-bar/)),
[Toggle](../../components/basic/toggle/), [Slider](../../components/basic/slider/),
[SegmentedControl](../../components/basic/segmented-control/), [TextField](../../components/basic/text-field/),
[VirtualList](../../components/lists/virtual-list/) and [ImageGallery](../../components/lists/image-gallery/).
A disabled control is not focusable.

### How the focus moves

- **Press on a focusable control.** The control takes the focus from its own handler. Every focusable control in
  package ui does this on `EventPointerDown`.
- **Press on anything else.** A press on empty space, or on a hit target that cannot be focused, such as a scroll
  view, clears the focus. A click on the background therefore removes a stale caret and puts the on-screen
  keyboard away. The on-screen keyboard itself is the one exception; a press on one of its keys keeps the focus
  on the field it types into.
- **Tab and shift-tab.** When no node handles a tab key press, gift moves the focus to the next focusable node,
  or to the previous one with shift held.
- **From code.** `App.MoveFocus(forward)` does what tab does. Inside an interactor, `EventContext.RequestFocus`
  and `ClearFocus` take or drop the focus.

### Focus order

The tab order is **document order**: the order in which nodes appear in the tree you built, parents before their
children, earlier siblings before later ones. It is not the geometry on screen. If you want the focus to move left
to right and then down, build the views in that order.

The order wraps around at both ends. Disabled controls and hidden subtrees are skipped. Tab does not repeat when
held, so the user can count presses.

A focused control may handle tab itself by returning `true` for it; gift then does not move the focus. The text
field deliberately does not do that, so tab always leaves a field.

### Focus traps

A [Modal](../../components/dialogs/modal/) confines the focus order to its presented view. While an
[alert](../../components/dialogs/alert/) or a [sheet](../../components/dialogs/sheet/) is up, tab cycles through
its controls and never reaches the screen behind it. When a trap appears, gift also takes the focus away from
anything outside it. The innermost and the most recently presented trap wins, so an alert over a sheet traps
inside the alert.

The mechanism is `gift.Element.FocusTrap`. Hiding works the same way: a node that is hidden while it holds the
focus loses it and receives `EventFocusLost` after the build.

## The focus ring

The focus decides where key events go. The focus **ring** shows the keyboard user where they are, and it would be
noise for someone who does not use a keyboard. gift therefore records where the focus came from:

- A focus installed while a key event is dispatched, or by `App.MoveFocus`, is visible. `FocusVisible` is true.
- A focus installed while a pointer event is dispatched is not visible. `Focused` is true, `FocusVisible` false.

Every focus ring in package ui reads `FocusVisible`. Tab onto a button and it shows a ring; tap it and it does
not, although it holds the focus and space would activate it. A text field always shows its caret while focused,
because the caret reads `Focused`. This is the rule CSS calls `:focus-visible`.

The default ring is a 2 pixel border in `ColorAccent`. Buttons and text fields let you change it with
`FocusRing`; a zero width border removes it:

```go
ui.Button(ui.Text("Save"), save).
	FocusRing(ui.Border{Width: 3, Color: ui.ColorAccent})
```

Toggles, sliders and segmented controls draw the default ring.

## Keys and characters

The keyboard reaches gift on two separate channels, and a control that edits text listens to both.

- **Keys** (`EventKeyDown`, `EventKeyUp`) name a physical key by its position on a US keyboard. They carry
  commands: navigation, activation, deletion, shortcuts.
- **Characters** (`EventRune`) carry the text the platform's keyboard layout produced. On a German keyboard the
  key in the position of the US semicolon produces `ö`, and that only arrives as a rune.

Never insert text from a `gift.Key`. Read `Event.Rune` from `EventRune` instead. Umlauts, accents, AltGr and dead
keys arrive as resolved runes. There is no input method for composed scripts such as Chinese or Japanese.

### The keys gift knows

`gift.Key` is deliberately small: tab, space, enter, escape, the four arrows, home, end, page up, page down,
backspace and delete, plus `KeyA`, `KeyC`, `KeyV`, `KeyX` and `KeyZ` as operands of the editing shortcuts. The
Ebitengine backend reports only these keys as key events. Other keys produce no key event; if they type a
character, it arrives as a rune.

`Event.Mods` holds the modifiers (`ModShift`, `ModControl`, `ModAlt`, `ModMeta`) and is also set on pointer
events. Test it with `Has`:

```go
if e.Key == gift.KeyC && e.Mods.Has(gift.ShortcutModifier) {
	// copy
}
```

`gift.ShortcutModifier` is command on macOS and control elsewhere. `gift.WordModifier`, which turns a caret move
into a word move, is option on macOS and control elsewhere. gift dispatches no shortcuts itself; these two
variables only make all views agree on which modifier means what. If the attached keyboard does not match the
operating system, for example a Mac keyboard on a Raspberry Pi, assign the variable before the first frame:

```go
gift.ShortcutModifier = gift.ModMeta
```

### Key repeat

gift repeats a held key itself, because Ebitengine has no key repeat. After `gift.KeyRepeatDelay` (400 ms) it
sends a synthetic `EventKeyDown` every `gift.KeyRepeatInterval` (30 per second), with `Event.Repeat` set. Only the
arrows, page up, page down, backspace and delete repeat. Space, enter, escape, tab, home and end do not, so
holding enter on a button fires it once. The repeat stops when the focus moves. Held character keys repeat at
the rate of the operating system, as runes.

### Escape and keys without a focus

On a touch panel usually nothing is focused, because a tap on the background clears the focus and no keyboard
moves it. A key from a keyboard plugged into it would then reach nobody. A node can
declare `gift.Element.KeyFallback` to receive key events while nothing holds the focus; the last one in document
order wins. [NavigationStack](../../components/navigation/navigation-stack/) uses it so that escape pops the top
screen, and [Sheet](../../components/dialogs/sheet/) uses it so that escape calls its `OnDismiss`, whether or not
anything is focused.

When the on-screen keyboard is showing, escape first puts the keyboard away and is not delivered to anybody. A
second press reaches the screen.

## The clipboard

A [text field](../../components/basic/text-field/) supports select all, copy, cut and paste with the shortcut
modifier and `A`, `C`, `X` and `V`. It goes through a process-wide `ui.Clipboard`, an interface with two methods,
`Text` and `SetText`.

By default the clipboard is a `ui.MemoryClipboard`. It works completely inside your process but exchanges text
with no other program. That is enough for a kiosk and for tests. To use the clipboard of the operating system,
import the clipboard package for its side effect:

```go
import _ "github.com/worldiety/gift/clipboard"
```

or install it with your logger, so that its failures are logged:

```go
ui.SetClipboard(clipboard.New(logger))
```

The package supports macOS (NSPasteboard) and Linux under X11, without cgo. On other platforms, including Windows
and Wayland without XWayland, a read reports no text and a write is dropped. It transfers at most
`clipboard.MaxBytes` (256 KiB); longer text is cut off at a rune boundary without telling the caller. Failures and
truncations are logged only to the logger you pass to `clipboard.New`; the side effect import logs nothing. Under X11 the copied text disappears when
your process exits. See the [package documentation](https://pkg.go.dev/github.com/worldiety/gift/clipboard) for
the details.

Your own code reaches the installed clipboard with `ui.CurrentClipboard()`, which never returns nil. Call it from
an event handler on the UI goroutine:

```go
ui.Button(ui.Text("Copy code"), func() {
	ui.CurrentClipboard().SetText(code)
})
```

Call `SetClipboard` before the first frame. `SetClipboard(nil)` restores the in-process default. In a test,
install a fresh `MemoryClipboard` and restore the default afterwards:

```go
cb := &ui.MemoryClipboard{}
ui.SetClipboard(cb)
t.Cleanup(func() { ui.SetClipboard(nil) })
```

## The on-screen keyboard

A kiosk without a hardware keyboard needs a keyboard on screen. gift draws one itself, because Ebitengine cannot
open the platform's keyboard and a Raspberry Pi kiosk has none to open. It is off by default. Two steps turn it
on:

```go
func main() {
	ui.SetOnScreenKeyboard(nil, true)
	app := gift.New(gift.Options{Root: screen})
	// ...
}

func screen(ctx *gift.Context) gift.View {
	form := ui.VStack(
		ui.TextField(ui.Editor(ctx, "name", "")).Placeholder("Name"),
	)
	return ui.Window(form, ui.OnScreenKeyboard()).Align(geom.Bottom)
}
```

With the switch on, a text field asks for the keyboard when it gains the focus and withdraws the request when it
loses it. The keyboard view shows itself while a request is open and takes no space otherwise. The keys inject
their input through the same path as a physical keyboard, so a control cannot tell the difference.

- **Placement.** Put `ui.OnScreenKeyboard()` once, as a direct child of the `ui.Window`, and align the window to
  the bottom. While the keyboard shows, the other children are laid out in the space above it, so a scroll view
  gets shorter and a submit button at the bottom stays visible. `Overlay.AvoidKeyboard(false)` turns that off.
  A [Modal](../../components/dialogs/modal/) belongs inside the window, next to the keyboard, not around it.
- **Revealing the field.** A focused text field scrolls itself into view with `EventContext.ScrollIntoView`, and
  gift repeats that once the keyboard has appeared.
- **Dismissing.** Escape, the keyboard's own dismiss key, a tap on the background and `App.DismissSoftKeyboard`
  all put the keyboard away by clearing the focus.
- **Gate on the deployment.** The switch is a statement about the machine, not about the pointer: as explained
  under [Touch input](#touch-input), a kiosk touchscreen arrives as a mouse.

The [Keyboard](../../components/dialogs/keyboard/) page shows the keyboard and its styling modifiers.

## Writing your own interactive view

When you build a control yourself, its element declares an interactor and, if it should take part in the tab
order, `Focusable: true`. `Focusable` without an interactor is ignored. The interactor is any value with a
`HandleEvent` method:

```go
type tapTarget struct{ onTap func() }

func (t tapTarget) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	switch e.Kind {
	case gift.EventPointerDown:
		ctx.RequestFocus() // a press focuses the node, without a ring
		return true        // claim the press
	case gift.EventPointerUp:
		if e.Inside && !e.Dragged {
			t.onTap()
		}
		return true
	case gift.EventPointerCancel:
		// the gesture ended without a release: reset, never activate
	case gift.EventKeyDown:
		if e.Key == gift.KeySpace || e.Key == gift.KeyEnter {
			t.onTap()
			return true
		}
	}
	return false
}
```

The rules that keep a control well behaved:

- **Return what you handled.** `true` stops bubbling. Return `false` for everything else, in particular for tab,
  so that the focus can move, and for moves you do not need, so that an enclosing scroll view can recognize a drag.
- **Check `Inside` and `Dragged` on the release.** A control that only checks `Inside` fires after a drag.
- **Treat cancel as a reset.** It can arrive in the middle of your own dispatch, when a scroll container steals the
  pointer.
- **Mind `Event.Repeat`.** Moving a cursor on a repeated arrow key is the point of repeat. Activating something on
  a repeat is not.
- **Do not allocate.** `HandleEvent` runs inside gift's zero allocation contract for input.
- **Read the interaction, do not copy it.** A painter reads hover, press and focus through
  `PaintContext.Interaction()`; gift already marked the node for repaint when they changed.

The `EventContext` is valid only during the call. It gives the node its bounds, its interaction, the focus,
scrolling and the on-screen keyboard; see the table below. [Custom views](../custom-views/) explains the rest of a
view of your own: the layouter, the painter and `ControlState`.

## Testing and diagnostics

The [gifttest](../testing/) harness drives the same entry points as the backend. `Tab`, `ShiftTab`, `Key`,
`TypeText`, `ClickAt`, `TapAt` and the `Tap`, `Press`, `LongPress`, `Swipe` and `Wheel` methods of a found node
produce real events, and `AssertFocus`, `AssertNoFocus`, `AssertHovered` and `AssertPressed` check the result.
`TapAt` and `TouchDownAt` send `PointerTouch` events, so a test can assert that a tap leaves no hover.

Outside tests, `App.Focus`, `App.FocusVisible`, `App.HitTest` and `App.NodeInteraction` answer the same questions.
[Diagnostics](../diagnostics/) counts `HitTests`, `InputEvents`, `DiscardedTouches`, `RunesTyped` and
`KeyRepeats`.

## API

### gift.EventContext

{{< api pkg="gift" type="EventContext" >}}

### gift.Interaction

{{< api pkg="gift" type="Interaction" decl="true" >}}

### Clipboard and on-screen keyboard

{{< api-funcs pkg="ui" names="SetClipboard,SetOnScreenKeyboard,OnScreenKeyboardEnabled,OnScreenKeyboardHeight" >}}

{{< api type="Clipboard" >}}

{{< api type="MemoryClipboard" >}}

## Related

- [Button](../../components/basic/button/) for hover, press and focus ring styles.
- [Text field](../../components/basic/text-field/) for editing keys, the clipboard and `OnSubmit`.
- [Keyboard](../../components/dialogs/keyboard/) for the on-screen keyboard.
- [Modal](../../components/dialogs/modal/), [Alert](../../components/dialogs/alert/) and
  [Sheet](../../components/dialogs/sheet/) for scrims and focus traps.
- [Scroll view](../../components/layout/scroll-view/) for wheel, drag and kinetic scrolling.
- [State](../state/) for why interaction state is not your state, and [Custom views](../custom-views/) for writing
  an interactor.
- [Kiosks](../kiosks/) for touch-only deployments.
- [Kitchen sink example](../../examples/example-kitchensink/), which turns the on-screen keyboard on.
