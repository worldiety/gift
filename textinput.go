package gift

import "github.com/worldiety/gift/geom"

// This file is the seam between gift's text fields and the input method of
// a platform that has one — the system keyboard of Android and iOS — and it
// is the counterpart of softinput.go, which is the seam to gift's own drawn
// keyboard. The bit both read is the same, [App.SoftKeyboardRequested]; what
// differs is who puts the keyboard on the screen.
//
// # Still no composition
//
// The project plan, section 14, keeps composition out of gift, and this does
// not bring it in: there is no preedit string here, no underline and no
// candidate window. A system keyboard composes a word while it is typed, and a
// backend shows that word as ordinary text, replacing it as the composition
// changes, through [App.ReplaceText] — the one editing operation an input
// method needs that a key press cannot express: "replace the selection and
// these bytes around it with this text", which is what a commit, an
// autocorrection and a backspace of the system keyboard all are.
//
// The project plan, section 52, records why the seam has this shape and what
// it leaves out.

// TextInputState is what a platform input method needs to know about the
// focused text: the text around the selection, for prediction and for the
// replacements an autocorrection makes, and where the caret is, so that the
// keyboard leaves it visible.
type TextInputState struct {
	// Before is the text before the selection, After the text after it. With
	// no selection the two meet at the caret. A selection itself is in
	// neither: the next replacement replaces it.
	//
	// A client may report only a window of a long text around the
	// selection, each side cut at a rune boundary; without one, the two
	// strings are the whole text. len(Before) is then an offset in the
	// window and not the caret's offset in the document, and the window
	// moves with the caret, so the start of Before does not stay put while
	// text is typed. A backend whose platform counts in absolute offsets
	// does not carry them from one state to the next: it compares states as
	// text, as the Ebitengine backend does, or starts the platform's session
	// again. The counts of [App.ReplaceText] are relative to the selection
	// and mean the same whatever the window is. ui's fields report a window
	// only for a document of several kilobytes.
	Before, After string

	// Selected reports that there is a selection between Before and After.
	// An input method that was shown Before and After only takes a backspace
	// for a deletion of the rune before the caret; with a selection the user
	// meant the selection.
	Selected bool

	// Caret is the caret. A [TextInputClient] reports it relative to the top
	// left corner of the element's bounds. [App.TextInput] returns it in the
	// space of [Event.Pos] and [App.HitTest]: the window's logical pixels,
	// with the transforms of the ancestors applied and without the density.
	// A backend that needs physical pixels multiplies by [App.Density].
	Caret geom.Rect

	// Secure is set for a password field. Before and After hold its text
	// all the same, as the buffer of a platform's own password field does,
	// because a keyboard that deletes the character before the caret has to
	// have it. A backend that can ask the platform for a keyboard in
	// password mode, one that neither learns nor suggests, does so for it.
	Secure bool
}

// TextInputClient is implemented by the interactor of an element that edits
// text and takes [EventTextReplace]. A focused node that implements it can be
// served by a platform input method; see [App.TextInput].
//
// # The contract
//
// Implementing the interface is half of it. A backend serves a node only
// while [App.SoftKeyboardRequested] is set, the same bit gift's drawn
// keyboard reads, so the node asks for a keyboard: on [EventFocusGained] it
// calls [EventContext.RequestSoftKeyboard] with true where
// [EventContext.PlatformKeyboardAvailable] reports a platform keyboard (ui
// also asks where its kiosk switch says gift draws one), and on
// [EventFocusLost] with false. A node that only implements the interface is
// never served.
//
// It applies an edit from [EventContext.TextReplacement] when it receives an
// [EventTextReplace], and takes the return key as a [KeyEnter], as a hardware
// key; see the return key of [App.ReplaceText]. ui.TextField is the
// reference implementation.
type TextInputClient interface {
	TextInputState() TextInputState
}

// TextInput returns the input method state of the focused node, and false when
// nothing is focused or the focused node does not edit text. The caret is in
// the space of [Event.Pos], logical pixels; see [TextInputState.Caret].
func (a *App) TextInput() (TextInputState, bool) {
	h := a.in.focus
	if !a.store.Valid(h) {
		return TextInputState{}, false
	}
	c, ok := a.data(h).interactor.(TextInputClient)
	if !ok {
		return TextInputState{}, false
	}
	st := c.TextInputState()
	if _, m, ok := a.deviceSpace(h); ok {
		origin := a.store.Get(h).Bounds.Min
		st.Caret = m.TransformRect(st.Caret.Translate(origin))
	}
	return st, true
}

// ReplaceText delivers an [EventTextReplace] to the focused node: delete the
// selection, deleteBefore bytes before it and deleteAfter bytes after it,
// insert text, and put the caret after the insertion. A backend calls it with
// what a platform input method committed. It is dropped when nothing is
// focused: unlike a key, a replacement has no meaning for a fallback node.
//
// The byte counts are counts in [TextInputState.Before] and
// [TextInputState.After] as the node reported them, relative to the selection
// and not to the start of a window the node reported. A backend asks for no
// more than it was shown; the node clamps the counts to its whole text and to
// rune boundaries, and [EventContext.TextReplacement] has them never
// negative. The counts are bytes. A backend whose platform counts in UTF-16
// units or in characters converts them against TextInputState.Before and
// After first.
//
// # The return key
//
// The return key of a platform keyboard is a key, not text: a backend reports
// it with [App.KeyDown] and [App.KeyUp] of [KeyEnter], as it would a hardware
// Enter, and not also as a "\n" replacement, so that a single line field can
// submit. A line break inside text is a character: a receiver that holds line
// breaks inserts it, and one that does not drops it. The Ebitengine backend
// gets this from Ebitengine, which reports return as KeyEnter on Android and
// iOS.
func (a *App) ReplaceText(deleteBefore, deleteAfter int, text string) {
	a.assertInputPhase("ReplaceText")
	a.diag.TextReplacements++
	if !a.store.Valid(a.in.focus) {
		return
	}
	r := &a.in.replace
	prev := *r
	defer func() { *r = prev }()
	r.armed, r.r = true, TextReplacement{
		Text:         text,
		DeleteBefore: max(deleteBefore, 0),
		DeleteAfter:  max(deleteAfter, 0),
	}
	a.deliverKey(Event{Kind: EventTextReplace, Mods: a.in.mods, Time: a.in.now, Pointer: MousePointer})
}

// TextReplacement is the edit an [EventTextReplace] carries; see
// [App.ReplaceText].
//
// It is not in [Event] for the reason [DropInfo] is not: Event stays a value
// that points at nothing and is copied cheaply on every pointer move, and the
// one event kind with a string payload reaches it through the context, as the
// project plan, section 49, decided for the drop.
type TextReplacement struct {
	// Text is inserted. It may hold any number of runes, a line break among
	// them where the receiver accepts one.
	Text string

	// DeleteBefore and DeleteAfter are the bytes deleted before and after
	// the selection, never negative.
	DeleteBefore, DeleteAfter int
}

// replaceState is the replacement [App.ReplaceText] is delivering. armed is
// set for the whole delivery, delivering only while a handler of the
// replacement itself runs, so that an event that handler causes — a key it
// injects, a focus change — does not see it; see [App.deliver].
type replaceState struct {
	armed, delivering bool
	r                 TextReplacement
}

// TextReplacement returns the edit the [EventTextReplace] being handled
// carries, and false for every other event. A nil EventContext answers false.
func (c *EventContext) TextReplacement() (TextReplacement, bool) {
	if c == nil || c.app == nil || !c.app.in.replace.delivering {
		return TextReplacement{}, false
	}
	return c.app.in.replace.r, true
}

// SetPlatformKeyboardActive records whether a platform input method is
// serving the current request for a keyboard, and asks for a rebuild when that
// changed. A backend that raises the system keyboard calls it, so that gift's
// own drawn keyboard — which reads [BuildContext.PlatformKeyboardActive] —
// stays down while the system's is up.
//
// While it is true, gift gets out of the way of no keyboard, because its drawn
// keyboard declares no obstruction. Keeping the caret of [App.TextInput] clear
// of the system keyboard is then up to the backend or the platform beneath it:
// it either shrinks the viewport passed to [App.Update], as a window that
// resizes for its keyboard does, or moves the picture and the input together
// from [TextInputState.Caret], as Ebitengine does on Android and iOS.
//
// # When to report
//
// A backend should report it on the UI goroutine after it dispatched the
// input of a tick and before [App.Update] builds, so that a field the input
// just focused is not handed to the drawn keyboard first. Where the platform
// confirms a session only later, the backend reports true optimistically and
// corrects it to false when the platform refuses. A report after the build
// lets ui.OnScreenKeyboard lay out for one frame; that frame declares its
// obstruction, the next one scrolls the field clear of it, and the scroll
// stays when the drawn keyboard steps aside. A focus that moves inside
// App.Update, by a timer or a build, is seen at the next input phase and
// shows the drawn keyboard for one frame. Nothing enforces the order: the
// method asserts only the UI goroutine.
func (a *App) SetPlatformKeyboardActive(active bool) {
	a.assertUIGoroutine("SetPlatformKeyboardActive")
	if a.in.soft.active == active {
		return
	}
	a.in.soft.active = active
	a.Invalidate()
}

// PlatformKeyboardActive reports whether a platform input method serves the
// current keyboard request; see [App.SetPlatformKeyboardActive].
func (a *App) PlatformKeyboardActive() bool { return a.in.soft.active }

// PlatformKeyboardActive is [App.PlatformKeyboardActive] read from inside a
// build. A nil BuildContext answers false, as
// [BuildContext.SoftKeyboardRequested] does.
func (b *BuildContext) PlatformKeyboardActive() bool {
	if b == nil {
		return false
	}
	return b.app.in.soft.active
}

// SetPlatformKeyboardAvailable records whether a platform input method is
// there to serve a request for a keyboard at all. A change rebuilds every
// view, as [App.SetDropCapabilities] does and for the same reason: the answer
// is read in builds — the default of ui.TextField for showing a password's
// last character, ui.KeyboardView — and a memoised subtree would keep the old
// one. A backend that can raise the system keyboard calls it once, when it
// sets up; the Ebitengine backend does so on Android and iOS.
//
// A field that is focused when it changes follows it only at its next focus
// change: it asks for a keyboard when it gains the focus, and it takes the
// request back whenever it loses it. A backend that can change it at runtime
// reports it before a field can take the focus, where it can.
//
// It is the counterpart of the kiosk switch of package ui. A text field asks
// for a keyboard where either says there is one to ask: the kiosk switch,
// because gift draws one, or this, because the platform has one. Without it a
// backend with a system keyboard would depend on a policy switch of ui that it
// may not even import (the project plan, section 3), and an application that
// forgot the switch would have fields that take the focus and never raise a
// keyboard. Nothing sets it on a desktop, where everything stays as it was.
func (a *App) SetPlatformKeyboardAvailable(available bool) {
	a.assertUIGoroutine("SetPlatformKeyboardAvailable")
	if a.in.soft.available == available {
		return
	}
	a.in.soft.available = available
	a.InvalidateAll()
}

// PlatformKeyboardAvailable reports whether a platform input method can serve
// a request for a keyboard; see [App.SetPlatformKeyboardAvailable].
func (a *App) PlatformKeyboardAvailable() bool { return a.in.soft.available }

// PlatformKeyboardAvailable is [App.PlatformKeyboardAvailable] read from inside
// an event handler, where a text field decides whether to ask for a keyboard.
// A nil EventContext answers false, as [EventContext.TextReplacement] does.
func (c *EventContext) PlatformKeyboardAvailable() bool {
	return c != nil && c.app != nil && c.app.in.soft.available
}

// PlatformKeyboardAvailable is [App.PlatformKeyboardAvailable] read from
// inside a build. A nil BuildContext answers false, as
// [BuildContext.SoftKeyboardRequested] does.
func (b *BuildContext) PlatformKeyboardAvailable() bool {
	if b == nil {
		return false
	}
	return b.app.in.soft.available
}
