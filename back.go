package gift

import (
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/scene"
)

// BackHandler answers the back action of the platform, such as the back
// button and the back gesture of Android. A node has one through its
// [Interactor], when that implements the interface, or through
// [Element.Back]. A navigation stack that can pop, a sheet, a popover, a menu
// or a modal implements it.
//
// # Why its own interface, and not the escape key
//
// Escape is a key, and a node that consumes it says nothing about whether
// anything happened. ui.NavigationStack consumes escape on its root screen on
// purpose, so that the key does not travel on and close something the user
// cannot see. Back has a third party the escape key does not have: the
// operating system, which leaves the application when gift does not go back.
// Android has to know that *before* the gesture starts — its predictive back
// animates either the page underneath or the home screen, and it hands the
// gesture only to a callback the application enabled beforehand. So back is a
// question first and an action second, and the question must be answerable
// without doing anything: [App.CanGoBack].
//
// # The contract
//
// CanGoBack reports whether this node takes the back action now. It is a pure
// read of what the node was built with and of its own state: no side effect,
// no allocation, because the backend asks it every frame on Android. True is
// not a promise that something visible happens. A modal that the application
// made undismissable takes the action and does nothing, which is what an
// Android dialog that is not cancelable does, so that back never leaves an
// application from behind a dialog. False passes the action on to the
// ancestors and, when nobody takes it, to the platform.
//
// GoBack performs it. It is called only right after CanGoBack answered true on
// the same frame, inside the input phase, with the node as the current node
// of ctx, as [Interactor.HandleEvent] is.
//
// A custom view implements both methods on its interactor, which gift checks
// for when the element is applied and remembers on the node; or it sets
// [Element.Back], which answers back without making the node a hit target.
// ui.OnBack wraps a view with one.
type BackHandler interface {
	CanGoBack() bool
	GoBack(ctx *EventContext)
}

// Back performs the back action of the platform, and reports whether gift took
// it. A backend calls it in the input phase when the platform's back action
// arrives, on Android the back button or the end of the back gesture. On false
// the platform's own back applies: Android goes back itself, which on the
// first view of an application leaves it; where the platform has none,
// nothing happens.
//
// A backend calls it at most once per input phase. A [BackHandler] decides
// with what its node was built with, so a second call before the next
// [App.Update] asks the same tree again, and a stack one screen deep would be
// asked to pop twice. A backend that receives several requests at once, such
// as a quick double press or a burst of platform events, keeps the extra ones
// and performs one per input phase, as the Back function of backend/ebiten
// does.
//
// The route is the one of [App.CanGoBack], which answers the same question
// without acting:
//
//  1. An on-screen keyboard is put away, as the escape key puts it away; see
//     [App.DismissSoftKeyboard]. Nothing else happens.
//  2. Inside the active [Element.FocusTrap] — a modal, a sheet, a popover or a
//     menu that is up — the last node in document order whose [BackHandler]
//     answers true is the candidate.
//  3. The focused node and its ancestors are asked, innermost first. The first
//     one that answers true goes back, unless the candidate of step 2 lies
//     inside it: then the candidate does. Only a focus inside the active trap
//     counts.
//  4. Without a focus, or when nothing on its path answers true, the
//     candidate of step 2 goes back.
//
// "Last in document order" is the rule of [Element.KeyFallback]: a nested
// navigation stack wins over the one around it, a covered screen is hidden and
// no candidate, and a sheet or a popover presented over the content is later
// in the tree. A handler that declines is skipped, so a stack on its root
// screen lets back reach the stack or the sheet around it, or a second stack
// beside it. The focus decides between candidates side by side: of two
// navigation stacks next to each other, the one being worked in pops. It does
// not decide against a deeper one: a tab of a tab bar, tapped a moment ago and
// focused, is not inside the stack of its tab, and back pops that stack and
// not the one around the tab bar. A focus inside an overlay on its way out
// (see [Element.OverlayExit]) is ignored, and such an overlay is no candidate.
//
// The back gesture of Android begins with a touch at the edge of the screen
// that the application receives before the system takes it over; it would
// dismiss a menu or a popover as a press outside, and the back action that
// follows would then find nothing and leave the application. So, on a
// platform whose backend set [App.SetBackGestureEdge], the answer that held
// when a touch came down at the edge is kept while the platform takes that
// touch over, which it reports with [App.PointerCancel]. It holds until the
// back action arrives, another press begins after that touch, or the
// platform reports the gesture abandoned with [App.BackCancel]. The node that
// answered then goes back as long as it still can, even if the touch moved
// the focus elsewhere; the action is spent on nothing only when the touch
// closed what that node went back for, or covered it with a trap. With the
// on-screen keyboard up, the action is spent on the keyboard even if the
// touch already put it away. A touch at the edge that is released was a tap,
// not the gesture, and drops it; see [App.latchBack].
//
// A focus that GoBack requests is visible only if the focus before it was;
// see [Interaction.FocusVisible].
//
// It counts in [Diagnostics.BackRequests].
func (a *App) Back() bool {
	a.assertInputPhase("Back")
	a.diag.BackRequests++
	latched, latchedAt := a.in.backLatch, a.in.backLatchAt
	latchedKeyboard := a.in.backLatchKeyboard
	a.dropBackLatch()
	if a.dismissSoftKeyboard() {
		return true
	}
	if latchedKeyboard {
		// The keyboard was up when the gesture began, and its touch put the
		// keyboard away, as a press beside a text field does: the gesture
		// was meant for the keyboard and is spent on it.
		return true
	}
	// Back is neither a key nor a press: on Android it is mostly the edge
	// swipe, and the platform's touch mode survives it. A focus the handler
	// installs keeps the provenance the user is in, the ring only if the
	// current focus already shows one; see [Interaction.FocusVisible].
	prev := a.in.keyPhase
	a.in.keyPhase = a.FocusVisible()
	defer func() { a.in.keyPhase = prev }()
	h := a.backTarget()
	if latched && h != latchedAt {
		if !a.stillGoesBack(latchedAt) {
			// The gesture's own touch took away what it was meant for — a
			// menu closed as a press outside it — and the back action is
			// spent on that, not on what is under it.
			return true
		}
		// The touch only moved the focus, which a press on something that
		// is not focusable drops: the node the gesture began for still
		// goes back.
		h = latchedAt
	}
	if h.IsZero() {
		return false
	}
	n := a.store.Get(h)
	nd := &n.Payload
	c := &a.in.ectx
	prevH, prevD := c.cur, c.nd
	c.cur, c.nd = h, nd
	a.diag.InputEvents++
	nd.back.GoBack(c)
	c.cur, c.nd = prevH, prevD
	return true
}

// CanGoBack reports whether [App.Back] would take the back action now: whether
// an on-screen keyboard is up, a [BackHandler] on the route of App.Back answers
// true, or, on a platform whose backend set [App.SetBackGestureEdge], a touch
// at the edge of the window latched a true answer. It does nothing and
// allocates nothing.
//
// The backend asks it after every update and hands the answer to the
// platform, which on Android enables or disables the application's back
// callback with it: false means that the platform's own back runs, which on
// the first view leaves the application. It is the state of the last update:
// a handler reads what its node was built with.
func (a *App) CanGoBack() bool {
	a.assertUIGoroutine("CanGoBack")
	return a.in.backLatch || a.canGoBack()
}

func (a *App) canGoBack() bool {
	return a.in.soft.wanted || !a.backTarget().IsZero()
}

// SetBackGestureEdge records how far from the left and the right edge of the
// window, in logical pixels, a touch may be the start of the platform's back
// gesture; see [App.Back]. Zero, the default, means the platform has no such
// gesture, and no touch latches the answer of [App.CanGoBack]. A backend whose
// platform has an edge back gesture calls it once, when it sets up; the
// Ebitengine backend does so on Android. Nothing sets it on a desktop, on iOS,
// in gifttest or in auto; a test of the Android behaviour sets it itself.
func (a *App) SetBackGestureEdge(width float32) {
	a.assertUIGoroutine("SetBackGestureEdge")
	if width < 0 {
		width = 0
	}
	a.in.backEdge = width
	if width == 0 {
		a.dropBackLatch()
	}
}

// BackCancel reports that the platform's back gesture ended without a back
// action: the user let it go back to where it started. It drops the answer
// that the gesture's first touch latched (see [App.Back]), so that
// [App.CanGoBack] tells the truth again and the next back action is not spent
// on a menu that the touch closed. A backend whose platform reports the
// abandoned gesture calls it, in the input phase; on Android that is the
// back callback's handleOnBackCancelled. Without an edge from
// [App.SetBackGestureEdge] it does nothing.
func (a *App) BackCancel() {
	a.assertInputPhase("BackCancel")
	a.dropBackLatch()
}

// BackGestureEdge reports the width of the edge zone of the platform's back
// gesture; see [App.SetBackGestureEdge].
func (a *App) BackGestureEdge() float32 { return a.in.backEdge }

// latchBack keeps the answer of [App.CanGoBack] across a touch that may be the
// start of the back gesture, and drops it on any other press. Without an edge
// reported by [App.SetBackGestureEdge] it latches nothing.
//
// The gesture is a drag from the edge of the screen. The application gets
// its first touch, and the system cancels it when it recognises the gesture.
// That first touch is an ordinary press as far as gift can tell, and a press
// outside a menu, a popover or an open swipe action dismisses it. Without the
// latch, the back that the gesture then delivers finds nothing left to go back
// to, and the platform — which was told so by then — leaves the application:
// one gesture both closes the menu and leaves. With it, the gesture closes the
// menu and back is spent on that, which is what the user meant. The latch
// remembers which node would have gone back, so that a gesture whose touch
// changed nothing — the common case — goes back as usual. One whose touch
// took that node's reason away (the menu is gone, or a trap the press put up
// shuts it out) is spent without popping the stack as well, while one whose
// touch only dropped the focus, as a press on something that is not focusable
// does, still goes back on the node it latched. With the on-screen keyboard
// up the latch is the keyboard's: the touch may put the keyboard away, a
// press beside a text field blurs it, and the back that follows is spent on
// that rather than on popping the stack as well.
//
// Every touch at the edge latches afresh against the tree it finds, except
// while the latching finger is still down: a further finger then, at the
// edge or anywhere else, belongs to the same gesture and leaves the latch. A
// touch at the edge that is released — on the bar's own back button, which
// sits there, or outside a menu that it closed — is a tap and no gesture,
// and [App.unlatchBack] drops the latch when its finger lifts. Only a touch
// the platform took over keeps it; see [App.PointerCancel]. A finger gift
// refuses (see [Diagnostics.DiscardedTouches]) neither latches nor drops the
// latch, and a keyboard the platform puts away itself drops a latch that was
// the keyboard's; see [App.DismissSoftKeyboard].
func (a *App) latchBack(id PointerID, kind PointerKind, pos geom.Point) {
	e := a.in.backEdge
	if kind == PointerTouch && a.in.backLatch && a.in.backLatchDown {
		// A further finger, at the edge or not, while the latching one is
		// still down: the same gesture.
		return
	}
	if e == 0 || kind != PointerTouch || (pos.X >= e && pos.X <= a.viewport.W-e) {
		a.dropBackLatch()
		return
	}
	if a.in.soft.wanted {
		// The keyboard goes first whatever the touch does to the rest, and
		// the touch may well put it away itself, as a press beside the field
		// blurs it.
		a.in.backLatch, a.in.backLatchKeyboard = true, true
		a.in.backLatchAt = scene.Handle{}
		a.in.backLatchID = id
		a.in.backLatchDown = true
		return
	}
	a.in.backLatchKeyboard = false
	a.in.backLatchAt = a.backTarget()
	a.in.backLatch = !a.in.backLatchAt.IsZero()
	a.in.backLatchID = id
	a.in.backLatchDown = a.in.backLatch
}

// unlatchBack drops the latch when the touch that set it is released. That
// touch was a tap at the edge, not the start of the gesture, whose touch the
// platform cancels instead, and a latch kept past it would answer for a state
// that the tap changed — a pop to the first view, which must leave the
// application at the next back, a sheet it opens, which the next back must
// close, or a menu it closed, after which the next back is an ordinary one.
// It runs before the release is delivered, so that the tap's own action sees
// no latch.
func (a *App) unlatchBack(id PointerID, kind PointerKind) {
	if a.in.backLatch && kind == PointerTouch && a.in.backLatchID == id {
		a.dropBackLatch()
	}
}

// keepBackLatch records that the platform took the touch id over: if it is
// the one that latched, the latch now waits for the gesture's back, or for
// [App.BackCancel], and no longer belongs to a finger that is down.
func (a *App) keepBackLatch(id PointerID, kind PointerKind) {
	if a.in.backLatch && kind == PointerTouch && a.in.backLatchID == id {
		a.in.backLatchDown = false
	}
}

// dropBackLatch forgets the answer an edge touch latched; see
// [App.latchBack].
func (a *App) dropBackLatch() {
	a.in.backLatch, a.in.backLatchDown, a.in.backLatchKeyboard = false, false, false
	a.in.backLatchAt = scene.Handle{}
}

// backTarget is the node whose [BackHandler] takes the back action, or the
// zero handle when none does; see [App.Back] for the route.
func (a *App) backTarget() scene.Handle {
	if a.backHandlers == 0 || a.root == nil {
		return scene.Handle{}
	}
	root := a.backRoot()
	last := scene.Handle{}
	a.findBackHandler(root, 0, &last)
	// A focus outside the active trap is left over from before the trap was
	// shown, as one in a hidden subtree is; see [App.backAlong].
	if h := a.in.focus; a.store.Valid(h) && (h == root || a.isAncestor(root, h)) {
		if t := a.backAlong(h, root); !t.IsZero() {
			if !last.IsZero() && last != t && a.isAncestor(t, last) {
				return last
			}
			return t
		}
	}
	return last
}

// backRoot is the subtree the back route searches: the active trap, or the
// whole tree.
func (a *App) backRoot() scene.Handle {
	root := a.root.node
	if a.traps != 0 {
		trap := scene.Handle{}
		a.findBackTrap(root, 0, &trap)
		if !trap.IsZero() {
			root = trap
		}
	}
	return root
}

// stillGoesBack reports whether h, latched by an edge touch, would still take
// the back action: it lies in the active trap, is neither hidden nor inside an
// overlay on its way out, and answers true. It is what tells a touch that only
// moved the focus from one that took the latched node's reason away; see
// [App.Back].
func (a *App) stillGoesBack(h scene.Handle) bool {
	if !a.store.Valid(h) || a.root == nil {
		return false
	}
	root := a.backRoot()
	return (h == root || a.isAncestor(root, h)) && a.backAlong(h, root) == h
}

// backAlong asks h and its ancestors up to root, the active trap of
// [App.backRoot], innermost first, and returns the first one that takes the
// back action; an ancestor outside the trap is not asked, as the trap bounds
// the route whether something is focused or not. It returns the zero handle
// as well when h lies inside a hidden subtree or an overlay on its way out: a
// focus there is left over and says nothing about where the user is.
func (a *App) backAlong(h, root scene.Handle) scene.Handle {
	found := scene.Handle{}
	inside := true
	for depth := 0; !h.IsZero() && a.store.Valid(h); depth++ {
		if depth > scene.MaxDepth {
			panic("gift: back route deeper than the maximum tree depth")
		}
		n := a.store.Get(h)
		nd := &n.Payload
		if nd.hidden || leavingOverlay(nd) {
			return scene.Handle{}
		}
		if inside && found.IsZero() && nd.back != nil && !nd.disabled && nd.back.CanGoBack() {
			found = h
		}
		if h == root {
			inside = false
		}
		h = n.Parent
	}
	return found
}

// findBackTrap is [App.findTrap] without the overlays on their way out: a
// popover that was just dismissed is still a trap for the focus order until it
// is gone, and the back action must reach what is under it, not dismiss it a
// second time.
func (a *App) findBackTrap(h scene.Handle, depth int, out *scene.Handle) {
	if depth > scene.MaxDepth || !a.store.Valid(h) {
		return
	}
	nd := a.data(h)
	if nd.hidden || leavingOverlay(nd) {
		return
	}
	if nd.focusTrap {
		*out = h
	}
	for _, c := range nd.children {
		a.findBackTrap(c, depth+1, out)
	}
}

// findBackHandler finds the last node in document order below h whose
// [BackHandler] answers true, skipping hidden subtrees, disabled nodes and
// overlays on their way out. It is [App.findKeyFallback] with the question in
// place of the flag; asking it of every handler, and not only of the last one,
// is what keeps a handler that declines from hiding the ones before it.
func (a *App) findBackHandler(h scene.Handle, depth int, out *scene.Handle) {
	if depth > scene.MaxDepth || !a.store.Valid(h) {
		return
	}
	nd := a.data(h)
	if nd.hidden || leavingOverlay(nd) {
		return
	}
	if nd.back != nil && !nd.disabled && nd.back.CanGoBack() {
		*out = h
	}
	for _, c := range nd.children {
		a.findBackHandler(c, depth+1, out)
	}
}

// applyBackHandler writes the node's [BackHandler] — [Element.Back], or else
// the interactor when it is one — and maintains the counter that lets
// [App.backTarget] skip its tree walk in an application without one, as
// [App.applyKeyFallback] does for [Element.KeyFallback].
func (a *App) applyBackHandler(nd *nodeData, back BackHandler) {
	if back == nil {
		back, _ = nd.interactor.(BackHandler)
	}
	switch {
	case back != nil && nd.back == nil:
		a.backHandlers++
	case back == nil && nd.back != nil:
		a.backHandlers--
		checkTrapCount(a)
	}
	nd.back = back
}
