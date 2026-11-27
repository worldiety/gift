package gift

import (
	"testing"
	"time"

	"github.com/worldiety/gift/geom"
)

// The tests of the back route of [App.Back] and [App.CanGoBack], with plain
// elements and a BackHandler of their own, so that the rule is pinned apart
// from the widgets of ui that implement it. They are in package gift and not
// in gift_test, because [App.backHandlers] has no observable effect while it
// is wrong in the positive direction — a count left above zero only makes
// [App.backTarget] walk a tree with nothing in it — and the balance tests
// read it directly.

var bkViewType = RegisterType("test.BackView")

// bkRecord is the state of one handler: whether it goes back, and how often
// it was asked to. A counter and not a log, so that the allocation tests can
// run the route with a handler at its end.
type bkRecord struct {
	can  bool
	went int
	// node is the node GoBack saw as the current one of its context.
	node NodeRef
	// dismissOnPress makes a press on the node take the handler's reason
	// away, as a press outside a menu dismisses it: the stand-in for the
	// first touch of Android's back gesture closing what back was meant for.
	dismissOnPress bool
	// focusOnBack makes GoBack request the focus for its node, as a detail
	// pane that hands the focus back to the item it came from does.
	focusOnBack bool
}

// bkHandler is a [BackHandler], stored by value in Element.Interactor as an
// interactor of a real widget would be.
type bkHandler struct{ r *bkRecord }

func (h bkHandler) HandleEvent(_ *EventContext, e Event) bool {
	if e.Kind == EventPointerDown && h.r.dismissOnPress {
		h.r.can = false
	}
	return false
}
func (h bkHandler) CanGoBack() bool { return h.r.can }
func (h bkHandler) GoBack(ctx *EventContext) {
	if !h.r.can {
		panic("GoBack after CanGoBack answered false")
	}
	h.r.went++
	h.r.node = NodeRef{ctx.cur}
	if h.r.focusOnBack {
		ctx.RequestFocus()
	}
}

// bkPlain is an interactor that is not a BackHandler. With keyboard set it is
// a text field in the small: it asks for the on-screen keyboard while it holds
// the focus.
type bkPlain struct{ keyboard bool }

func (p bkPlain) HandleEvent(ctx *EventContext, e Event) bool {
	if p.keyboard {
		switch e.Kind {
		case EventFocusGained:
			ctx.RequestSoftKeyboard(true)
		case EventFocusLost:
			ctx.RequestSoftKeyboard(false)
		}
	}
	return false
}

// bk is a node that stacks its children on top of each other, every one
// 100 by 100 at the origin, and declares what a test sets.
type bk struct {
	key       string
	r         *bkRecord // nil: an interactor that is not a BackHandler, or none
	back      *bkRecord // non-nil: an Element.Back of its own
	plain     bool      // a bkPlain interactor where r is nil
	keyboard  bool      // the plain interactor asks for the on-screen keyboard
	focusable bool
	disabled  bool
	hidden    bool
	trap      bool
	multi     bool // MultiTouch, so that a further finger gets a slot
	overlay   View
	exit      time.Duration
	children  []View
}

func (bk) ViewType() TypeID { return bkViewType }

func (v bk) Build(*BuildContext) Element {
	e := Element{
		Key:         v.key,
		Layouter:    bkStack{},
		Focusable:   v.focusable,
		Disabled:    v.disabled,
		Hidden:      v.hidden,
		FocusTrap:   v.trap,
		MultiTouch:  v.multi,
		Overlay:     v.overlay,
		OverlayExit: v.exit,
		Children:    v.children,
	}
	if v.back != nil {
		e.Back = bkHandler{r: v.back}
	}
	switch {
	case v.r != nil:
		e.Interactor = bkHandler{r: v.r}
	case v.plain || v.keyboard || v.focusable:
		e.Interactor = bkPlain{keyboard: v.keyboard}
	}
	return e
}

type bkStack struct{}

func (bkStack) Layout(ctx *LayoutContext, c geom.Constraints) geom.Size {
	for i := range ctx.ChildCount() {
		ctx.Measure(i, c)
		ctx.Place(i, geom.Pt(0, 0))
	}
	return geom.Sz(100, 100)
}

// bkApp mounts the tree root returns and lays it out.
func bkApp(t *testing.T, root func() View) *App {
	t.Helper()
	a := New(Options{Root: func(*Context) View { return root() }})
	bkUpdate(t, a)
	return a
}

// bkUpdate rebuilds everything, as a state change of the application would,
// and lays out.
func bkUpdate(t *testing.T, a *App) {
	t.Helper()
	a.InvalidateAll()
	if err := a.Update(geom.Sz(400, 400)); err != nil {
		t.Fatal(err)
	}
}

// bkBack performs a back action in an input phase of its own.
func bkBack(a *App) bool {
	a.BeginInput(a.in.now)
	return a.Back()
}

// focusKey moves the focus forward until the node keyed key holds it.
func focusKey(t *testing.T, a *App, key string) {
	t.Helper()
	a.BeginInput(a.in.now)
	for range 32 {
		if r, ok := a.Focus(); ok && a.store.Get(r.h).Key == key {
			return
		}
		a.MoveFocus(true)
	}
	t.Fatalf("no tab focuses %q", key)
}

func TestBackWithNoHandlerAnywhereLeavesTheApplication(t *testing.T) {
	a := bkApp(t, func() View {
		return bk{key: "root", children: []View{bk{key: "leaf", focusable: true}}}
	})
	if a.CanGoBack() {
		t.Fatal("CanGoBack is true in an application without a BackHandler")
	}
	if bkBack(a) {
		t.Fatal("Back took the action in an application without a BackHandler")
	}
	bkUpdate(t, a) // the diagnostics are published by an update
	if got := a.Diagnostics().BackRequests; got != 1 {
		t.Fatalf("BackRequests = %d, want 1: a declined back action is counted too", got)
	}
}

// TestBackFollowsTheFocusBeforeTheDocumentOrder: of two navigation stacks
// side by side, the one being worked in pops, even when it is not the last
// in the document.
func TestBackFollowsTheFocusBeforeTheDocumentOrder(t *testing.T) {
	first, second := &bkRecord{can: true}, &bkRecord{can: true}
	a := bkApp(t, func() View {
		return bk{key: "root", children: []View{
			bk{key: "first", r: first, children: []View{bk{key: "field", focusable: true}}},
			bk{key: "second", r: second},
		}}
	})
	if !bkBack(a) || second.went != 1 || first.went != 0 {
		t.Fatalf("with nothing focused: first went %d, second went %d; want the last in the document",
			first.went, second.went)
	}
	focusKey(t, a, "field")
	if !bkBack(a) || first.went != 1 || second.went != 1 {
		t.Fatalf("with the field focused: first went %d, second went %d; want the one around the focus",
			first.went, second.went)
	}
	if got := a.store.Get(first.node.h).Key; got != "first" {
		t.Fatalf("GoBack ran with %q as the current node of its context, want its own", got)
	}
}

// TestBackAsksTheAncestorsOfTheFocusInnermostFirst: a stack on its root
// screen declines, and the stack or the sheet around it gets the action.
func TestBackAsksTheAncestorsOfTheFocusInnermostFirst(t *testing.T) {
	outer, inner := &bkRecord{can: true}, &bkRecord{}
	a := bkApp(t, func() View {
		return bk{key: "outer", r: outer, children: []View{
			bk{key: "inner", r: inner, children: []View{bk{key: "field", focusable: true}}},
		}}
	})
	focusKey(t, a, "field")
	if !bkBack(a) || outer.went != 1 {
		t.Fatalf("the inner stack declined and the outer went back %d times, want 1", outer.went)
	}
	inner.can = true
	if !bkBack(a) || inner.went != 1 || outer.went != 1 {
		t.Fatalf("inner went %d, outer went %d; want the innermost that can", inner.went, outer.went)
	}
}

// TestBackWithAFocusOutsideEveryStackReachesTheLastStack: a focus outside
// every stack — a tab of a tab bar, tapped a moment ago — does
// not swallow the action.
func TestBackWithAFocusOutsideEveryStackReachesTheLastStack(t *testing.T) {
	stack := &bkRecord{can: true}
	a := bkApp(t, func() View {
		return bk{key: "root", children: []View{
			bk{key: "stack", r: stack},
			bk{key: "tab", focusable: true},
		}}
	})
	focusKey(t, a, "tab")
	if !bkBack(a) || stack.went != 1 {
		t.Fatalf("the stack went back %d times, want 1", stack.went)
	}
}

// TestBackWithNothingFocusedPrefersTheNestedStackAndThenItsAncestors: the
// last handler in document order is the innermost of nested stacks, and when
// it is on its root screen the stack around it goes back.
func TestBackWithNothingFocusedPrefersTheNestedStackAndThenItsAncestors(t *testing.T) {
	outer, inner := &bkRecord{can: true}, &bkRecord{can: true}
	a := bkApp(t, func() View {
		return bk{key: "outer", r: outer, children: []View{bk{key: "inner", r: inner}}}
	})
	if !bkBack(a) || inner.went != 1 || outer.went != 0 {
		t.Fatalf("inner went %d, outer went %d; want the nested stack", inner.went, outer.went)
	}
	inner.can = false
	if !a.CanGoBack() || !bkBack(a) || outer.went != 1 {
		t.Fatalf("with the nested stack on its root screen the outer went %d times, want 1", outer.went)
	}
	outer.can = false
	if a.CanGoBack() || bkBack(a) {
		t.Fatal("both stacks on their root screen and gift still takes the action")
	}
}

// TestBackSkipsHiddenHandlers: a covered screen of a navigation stack is
// hidden, and its own stack must not pop.
func TestBackSkipsHiddenHandlers(t *testing.T) {
	visible, covered := &bkRecord{can: true}, &bkRecord{can: true}
	a := bkApp(t, func() View {
		return bk{key: "root", children: []View{
			bk{key: "visible", r: visible},
			bk{key: "covered", hidden: true, children: []View{bk{key: "nested", r: covered}}},
		}}
	})
	if !bkBack(a) || visible.went != 1 || covered.went != 0 {
		t.Fatalf("visible went %d, covered went %d; want the hidden one skipped", visible.went, covered.went)
	}
}

// TestBackSkipsDisabledHandlers, on both routes: as the last handler in the
// document and as an ancestor of the focus.
func TestBackSkipsDisabledHandlers(t *testing.T) {
	enabled, disabled := &bkRecord{can: true}, &bkRecord{can: true}
	a := bkApp(t, func() View {
		return bk{key: "root", children: []View{
			bk{key: "enabled", r: enabled, children: []View{
				bk{key: "disabled", r: disabled, disabled: true},
			}},
			bk{key: "field", focusable: true},
		}}
	})
	if !bkBack(a) || enabled.went != 1 || disabled.went != 0 {
		t.Fatalf("enabled went %d, disabled went %d", enabled.went, disabled.went)
	}
}

// TestBackStaysInsideTheActiveFocusTrap: a modal presented over the content
// is a trap, and back closes it rather than popping the stack under it, even
// when that stack comes later in the document.
func TestBackStaysInsideTheActiveFocusTrap(t *testing.T) {
	modal, under := &bkRecord{can: true}, &bkRecord{can: true}
	a := bkApp(t, func() View {
		return bk{key: "root", children: []View{
			bk{key: "modal", r: modal, trap: true, children: []View{bk{key: "content", plain: true}}},
			bk{key: "under", r: under},
		}}
	})
	if !bkBack(a) || modal.went != 1 || under.went != 0 {
		t.Fatalf("modal went %d, the stack outside the trap went %d", modal.went, under.went)
	}
}

// TestBackPassesOverAnOverlayOnItsWayOut: a popover that was just dismissed
// is still painted until its exit animation ends, and a second back must reach
// what is under it, not dismiss it again.
func TestBackPassesOverAnOverlayOnItsWayOut(t *testing.T) {
	popover, page := &bkRecord{can: true}, &bkRecord{can: true}
	open := true
	a := bkApp(t, func() View {
		p := bk{key: "presenter", plain: true, exit: 200 * time.Millisecond}
		if open {
			p.overlay = bk{key: "popover", r: popover}
		}
		return bk{key: "page", r: page, children: []View{p}}
	})
	if !bkBack(a) || popover.went != 1 || page.went != 0 {
		t.Fatalf("popover went %d, page went %d; want the open overlay", popover.went, page.went)
	}
	open = false
	bkUpdate(t, a)
	if !bkBack(a) || popover.went != 1 || page.went != 1 {
		t.Fatalf("popover went %d, page went %d; want the overlay on its way out skipped",
			popover.went, page.went)
	}
}

// TestBackPutsTheOnScreenKeyboardAwayFirst: the first back with a keyboard up
// only puts it away, as on Android, and the next one navigates.
func TestBackPutsTheOnScreenKeyboardAwayFirst(t *testing.T) {
	stack := &bkRecord{can: true}
	a := bkApp(t, func() View {
		return bk{key: "stack", r: stack, children: []View{bk{key: "field", keyboard: true, focusable: true}}}
	})
	focusKey(t, a, "field")
	if !a.SoftKeyboardRequested() {
		t.Fatal("the focused field asked for no keyboard; the fixture is wrong")
	}
	stack.can = false
	if !a.CanGoBack() {
		t.Fatal("CanGoBack is false with the keyboard up")
	}
	if !bkBack(a) {
		t.Fatal("Back did not take the action with the keyboard up")
	}
	if a.SoftKeyboardRequested() {
		t.Fatal("the keyboard is still requested after back")
	}
	if _, ok := a.Focus(); ok {
		t.Fatal("the field still holds the focus after back put its keyboard away")
	}
	if a.CanGoBack() {
		t.Fatal("CanGoBack is true with the keyboard down and the stack on its root screen")
	}
	stack.can = true
	if !bkBack(a) || stack.went != 1 {
		t.Fatalf("the second back popped %d times, want 1", stack.went)
	}
	bkUpdate(t, a)
	if got := a.Diagnostics().BackRequests; got != 2 {
		t.Fatalf("BackRequests = %d, want 2", got)
	}
}

// TestBackHandlerCountBalances: [App.backHandlers] is counted up and down
// over everything that changes an interactor — a mount, an interactor of
// another type, an unmount of the node and of a subtree around it — and is
// zero again when no handler is left. A giftdebug build also panics the
// moment it goes below zero; see checkTrapCount.
func TestBackHandlerCountBalances(t *testing.T) {
	rec := &bkRecord{can: true}
	type state int
	const (
		none state = iota
		one
		swapped
		nested
	)
	s := none
	a := bkApp(t, func() View {
		switch s {
		case one:
			return bk{key: "root", children: []View{bk{key: "h", r: rec}}}
		case swapped:
			return bk{key: "root", children: []View{bk{key: "h", plain: true}}}
		case nested:
			return bk{key: "root", children: []View{
				bk{key: "h", r: rec, children: []View{bk{key: "i", r: rec}, bk{key: "j", r: rec}}},
			}}
		}
		return bk{key: "root"}
	})
	for _, step := range []struct {
		s    state
		want int
	}{
		{one, 1}, {swapped, 0}, {one, 1}, {nested, 3}, {swapped, 0}, {nested, 3}, {none, 0}, {one, 1}, {none, 0},
	} {
		s = step.s
		bkUpdate(t, a)
		if a.backHandlers != step.want {
			t.Fatalf("after state %d the count is %d, want %d", step.s, a.backHandlers, step.want)
		}
	}
	if a.CanGoBack() {
		t.Fatal("CanGoBack with no handler left")
	}
}

// TestCanGoBackAndBackAllocateNothing: the backend asks CanGoBack after every
// update on Android, and a back action is a user's gesture that must not
// produce garbage either; both routes are pinned.
func TestCanGoBackAndBackAllocateNothing(t *testing.T) {
	outer, inner := &bkRecord{can: true}, &bkRecord{}
	a := bkApp(t, func() View {
		return bk{key: "root", children: []View{
			bk{key: "outer", r: outer, trap: true, children: []View{
				bk{key: "inner", r: inner, children: []View{bk{key: "field", focusable: true}}},
			}},
			bk{key: "other", r: &bkRecord{}},
		}}
	})
	if n := testing.AllocsPerRun(100, func() { a.CanGoBack() }); n != 0 {
		t.Fatalf("CanGoBack with nothing focused allocates %v times", n)
	}
	if n := testing.AllocsPerRun(100, func() { a.Back() }); n != 0 {
		t.Fatalf("Back with nothing focused allocates %v times", n)
	}
	focusKey(t, a, "field")
	if n := testing.AllocsPerRun(100, func() { a.CanGoBack() }); n != 0 {
		t.Fatalf("CanGoBack along the focus allocates %v times", n)
	}
	if n := testing.AllocsPerRun(100, func() { a.Back() }); n != 0 {
		t.Fatalf("Back along the focus allocates %v times", n)
	}
	if outer.went == 0 {
		t.Fatal("the route under test went nowhere; the fixture is wrong")
	}
}

// TestElementBackAnswersWithoutMakingTheNodeAHitTarget: a wrapper that only
// means "back goes here" sits on top of the node under it and must not take
// its taps, and still takes the back action as its own current node.
func TestElementBackAnswersWithoutMakingTheNodeAHitTarget(t *testing.T) {
	wrapper := &bkRecord{can: true}
	a := bkApp(t, func() View {
		return bk{key: "root", children: []View{
			bk{key: "under", plain: true},
			bk{key: "wrapper", back: wrapper},
		}}
	})
	h := a.hitTest(geom.Pt(50, 50))
	if h.IsZero() || a.store.Get(h).Key != "under" {
		t.Fatalf("the hit test at the wrapper's bounds found %v, want the node under it", h)
	}
	if !a.CanGoBack() || !bkBack(a) || wrapper.went != 1 {
		t.Fatalf("the Element.Back went back %d times, want 1", wrapper.went)
	}
	if got := a.store.Get(wrapper.node.h).Key; got != "wrapper" {
		t.Fatalf("GoBack ran with %q as the current node of its context, want its own", got)
	}
}

// TestElementBackTakesPrecedenceOverTheInteractor: when a node has both, the
// field is asked and the interactor is not, in either direction of their
// answers.
func TestElementBackTakesPrecedenceOverTheInteractor(t *testing.T) {
	field, inter := &bkRecord{can: true}, &bkRecord{can: true}
	a := bkApp(t, func() View {
		return bk{key: "node", r: inter, back: field}
	})
	if !bkBack(a) || field.went != 1 || inter.went != 0 {
		t.Fatalf("Element.Back went %d, the interactor went %d; want the field", field.went, inter.went)
	}
	field.can = false
	if a.CanGoBack() || bkBack(a) || inter.went != 0 {
		t.Fatalf("with Element.Back declining the interactor was asked after all (went %d)", inter.went)
	}
}

// TestBackHandlerCountBalancesAcrossElementBackAndTheInteractor: switching a
// node between an Element.Back, an interactor that is a handler, both, and
// neither counts one handler per node at most and returns to zero.
func TestBackHandlerCountBalancesAcrossElementBackAndTheInteractor(t *testing.T) {
	rec := &bkRecord{can: true}
	type state int
	const (
		none state = iota
		field
		inter
		both
		plain
	)
	s := none
	a := bkApp(t, func() View {
		n := bk{key: "h"}
		switch s {
		case field:
			n.back = rec
		case inter:
			n.r = rec
		case both:
			n.r, n.back = rec, rec
		case plain:
			n.plain = true
		}
		return bk{key: "root", children: []View{n}}
	})
	for _, step := range []struct {
		s    state
		want int
	}{
		{field, 1}, {inter, 1}, {field, 1}, {none, 0}, {both, 1}, {plain, 0},
		{inter, 1}, {both, 1}, {field, 1}, {plain, 0}, {both, 1}, {none, 0},
	} {
		s = step.s
		bkUpdate(t, a)
		if a.backHandlers != step.want {
			t.Fatalf("after state %d the count is %d, want %d", step.s, a.backHandlers, step.want)
		}
	}
}

// TestALaterDecliningHandlerDoesNotHideAnEarlierOne: a second stack on its
// root screen, later in the document, lets back reach the first one.
func TestALaterDecliningHandlerDoesNotHideAnEarlierOne(t *testing.T) {
	first, second := &bkRecord{can: true}, &bkRecord{}
	a := bkApp(t, func() View {
		return bk{key: "root", children: []View{
			bk{key: "first", r: first},
			bk{key: "second", r: second},
		}}
	})
	if !a.CanGoBack() || !bkBack(a) || first.went != 1 {
		t.Fatalf("the first stack went back %d times, want 1", first.went)
	}
}

// TestBackPrefersADeeperCandidateInsideTheHandlerOfTheFocus: a tab of a tab
// bar is focused, it lies inside the outer stack but not inside the stack of
// its tab, and back pops the stack of the tab.
func TestBackPrefersADeeperCandidateInsideTheHandlerOfTheFocus(t *testing.T) {
	outer, inner := &bkRecord{can: true}, &bkRecord{can: true}
	a := bkApp(t, func() View {
		return bk{key: "outer", r: outer, children: []View{
			bk{key: "tab", focusable: true},
			bk{key: "inner", r: inner},
		}}
	})
	focusKey(t, a, "tab")
	if !bkBack(a) || inner.went != 1 || outer.went != 0 {
		t.Fatalf("inner went %d, outer went %d; want the deeper stack", inner.went, outer.went)
	}
	inner.can = false
	if !bkBack(a) || outer.went != 1 {
		t.Fatalf("with the inner stack on its root screen the outer went %d times, want 1", outer.went)
	}
}

// TestBackIgnoresAFocusInsideAnOverlayOnItsWayOut: the field of a popover
// that was just dismissed may still hold the focus while the popover fades,
// and the back action reaches the page, not the popover a second time.
func TestBackIgnoresAFocusInsideAnOverlayOnItsWayOut(t *testing.T) {
	popover, page := &bkRecord{can: true}, &bkRecord{can: true}
	open := true
	a := bkApp(t, func() View {
		p := bk{key: "presenter", plain: true, exit: 200 * time.Millisecond}
		if open {
			p.overlay = bk{key: "popover", r: popover, children: []View{bk{key: "field", focusable: true}}}
		}
		return bk{key: "page", r: page, children: []View{p}}
	})
	focusKey(t, a, "field")
	open = false
	bkUpdate(t, a)
	if !bkBack(a) || popover.went != 0 || page.went != 1 {
		t.Fatalf("popover went %d, page went %d; want the focus in the leaving overlay ignored",
			popover.went, page.went)
	}
}

// TestBackIgnoresAFocusInsideAHiddenSubtree: a screen covered by a push may
// keep the focus it had, and its own stack must not pop.
func TestBackIgnoresAFocusInsideAHiddenSubtree(t *testing.T) {
	page, covered := &bkRecord{can: true}, &bkRecord{can: true}
	hidden := false
	a := bkApp(t, func() View {
		return bk{key: "page", r: page, children: []View{
			bk{key: "screen", hidden: hidden, children: []View{
				bk{key: "covered", r: covered, children: []View{bk{key: "field", focusable: true}}},
			}},
		}}
	})
	focusKey(t, a, "field")
	hidden = true
	bkUpdate(t, a)
	if !bkBack(a) || covered.went != 0 || page.went != 1 {
		t.Fatalf("covered went %d, page went %d; want the focus in the hidden screen ignored",
			covered.went, page.went)
	}
}

// bkEdgeApp is [bkApp] on a platform with Android's back gesture: an edge zone
// of 40 logical pixels, as backend/ebiten reports it there.
func bkEdgeApp(t *testing.T, root func() View) *App {
	t.Helper()
	a := bkApp(t, root)
	a.SetBackGestureEdge(40)
	return a
}

// bkEdgeTouch presses a finger at pos and cancels it, as Android does when it
// recognises the back gesture that began with it.
func bkEdgeTouch(a *App, pos geom.Point) {
	a.BeginInput(a.in.now)
	a.PointerDown(1, PointerTouch, pos)
	a.PointerCancel(1)
}

// TestATouchAtEitherEdgeLatchesTheBackAnswer and another press drops it: a
// touch in the middle, or a mouse press even at the edge, is no gesture.
func TestATouchAtEitherEdgeLatchesTheBackAnswer(t *testing.T) {
	stack := &bkRecord{can: true}
	a := bkEdgeApp(t, func() View { return bk{key: "stack", r: stack} })
	for _, edge := range []geom.Point{geom.Pt(10, 200), geom.Pt(390, 200)} {
		bkEdgeTouch(a, edge)
		if !a.in.backLatch {
			t.Fatalf("a touch at %v latched nothing", edge)
		}
		bkEdgeTouch(a, geom.Pt(200, 200))
		if a.in.backLatch {
			t.Fatal("a touch in the middle left the latch set")
		}
		bkEdgeTouch(a, edge)
		a.PointerDown(MousePointer, PointerMouse, edge)
		a.PointerUp(MousePointer, PointerMouse, edge)
		if a.in.backLatch {
			t.Fatal("a mouse press left the latch set")
		}
	}
	if stack.went != 0 {
		t.Fatalf("the presses went back %d times", stack.went)
	}
}

// TestALatchedBackWithAnUnchangedTargetGoesBack: the common case, a gesture
// whose touch changed nothing.
func TestALatchedBackWithAnUnchangedTargetGoesBack(t *testing.T) {
	stack := &bkRecord{can: true}
	a := bkEdgeApp(t, func() View { return bk{key: "stack", r: stack} })
	bkEdgeTouch(a, geom.Pt(10, 10))
	if !bkBack(a) || stack.went != 1 {
		t.Fatalf("the stack went back %d times, want 1", stack.went)
	}
	if a.in.backLatch {
		t.Fatal("the back action left the latch set")
	}
}

// TestALatchedBackWhoseTouchClosedTheMenuIsSpentOnThat: the first touch of
// the gesture lands on the menu and dismisses it; the back action that
// follows must neither leave the application nor pop the page under it.
func TestALatchedBackWhoseTouchClosedTheMenuIsSpentOnThat(t *testing.T) {
	page, menu := &bkRecord{can: true}, &bkRecord{can: true, dismissOnPress: true}
	a := bkEdgeApp(t, func() View {
		return bk{key: "root", children: []View{
			bk{key: "page", r: page},
			bk{key: "menu", r: menu},
		}}
	})
	bkEdgeTouch(a, geom.Pt(10, 10))
	if menu.can {
		t.Fatal("the edge touch did not reach the menu; the fixture is wrong")
	}
	if !a.CanGoBack() {
		t.Fatal("CanGoBack is false while the answer of the edge touch is latched")
	}
	if !bkBack(a) || page.went != 0 || menu.went != 0 {
		t.Fatalf("page went %d, menu went %d; want the back action consumed on nothing",
			page.went, menu.went)
	}
	if !bkBack(a) || page.went != 1 {
		t.Fatalf("the next back popped the page %d times, want 1", page.went)
	}
}

// TestALatchedBackWhoseTouchOnlyDroppedTheFocusStillGoesBack: the gesture's
// first touch lands on content that is not focusable, beside the stack being
// worked in, and drops the focus, which moves the route's answer to the stack
// last in the document. Nothing lost its reason to go back, so the stack the
// gesture began for pops; the action is not spent on nothing. A press that
// puts up a trap is different: the latched stack is shut out, and the back
// action is spent.
func TestALatchedBackWhoseTouchOnlyDroppedTheFocusStillGoesBack(t *testing.T) {
	first, second, sheet := &bkRecord{can: true}, &bkRecord{can: true}, &bkRecord{can: true}
	sheetUp := false
	a := bkEdgeApp(t, func() View {
		children := []View{
			bk{key: "first", r: first, children: []View{bk{key: "field", focusable: true}}},
			bk{key: "second", r: second},
		}
		if sheetUp {
			children = append(children, bk{key: "sheet", r: sheet, trap: true,
				children: []View{bk{key: "inside", focusable: true}}})
		}
		return bk{key: "root", children: children}
	})
	focusKey(t, a, "field")
	bkEdgeTouch(a, geom.Pt(10, 10))
	if _, ok := a.Focus(); ok {
		t.Fatal("the edge touch kept the focus; the fixture is wrong")
	}
	if !bkBack(a) || first.went != 1 || second.went != 0 {
		t.Fatalf("first went %d, second went %d; want the stack the gesture began for to pop",
			first.went, second.went)
	}

	focusKey(t, a, "field")
	bkEdgeTouch(a, geom.Pt(10, 10))
	sheetUp = true
	bkUpdate(t, a)
	if !bkBack(a) || first.went != 1 || second.went != 0 || sheet.went != 0 {
		t.Fatalf("first went %d, second went %d, sheet went %d; want the back action spent on the sheet the touch put up",
			first.went, second.went, sheet.went)
	}
}

// TestNoLatchWhenNothingCanGoBack: on the first screen the gesture leaves
// the application, and the latch must not hold it back.
func TestNoLatchWhenNothingCanGoBack(t *testing.T) {
	stack := &bkRecord{}
	a := bkEdgeApp(t, func() View { return bk{key: "stack", r: stack} })
	bkEdgeTouch(a, geom.Pt(10, 10))
	if a.in.backLatch || a.CanGoBack() {
		t.Fatal("an edge touch latched a back action that nothing takes")
	}
}

// TestAnEdgeGestureWithTheKeyboardUpIsSpentOnTheKeyboard: the gesture's
// touch lands beside the field and blurs it, which puts the keyboard away
// before the back arrives; the back is spent on the keyboard and does not pop
// the stack as well. A second back then pops.
func TestAnEdgeGestureWithTheKeyboardUpIsSpentOnTheKeyboard(t *testing.T) {
	stack := &bkRecord{can: true}
	a := bkEdgeApp(t, func() View {
		return bk{key: "stack", r: stack, children: []View{
			bk{key: "field", keyboard: true, focusable: true},
			bk{key: "cover", plain: true},
		}}
	})
	focusKey(t, a, "field")
	if !a.SoftKeyboardRequested() {
		t.Fatal("the focused field asked for no keyboard; the fixture is wrong")
	}
	bkEdgeTouch(a, geom.Pt(10, 50))
	if a.SoftKeyboardRequested() {
		t.Fatal("the press beside the field left the keyboard up; the fixture is wrong")
	}
	if !a.CanGoBack() {
		t.Fatal("CanGoBack is false after the gesture's touch")
	}
	if !bkBack(a) || stack.went != 0 {
		t.Fatalf("the gesture went back %d times; want it spent on the keyboard", stack.went)
	}
	if !bkBack(a) || stack.went != 1 {
		t.Fatalf("the next back went %d times, want 1", stack.went)
	}
}

// TestAnEdgeGestureWithTheKeyboardUpThatLeavesItUpPutsItAway: a touch on the
// field itself does not blur it and leaves the keyboard for the back to put
// away.
func TestAnEdgeGestureWithTheKeyboardUpThatLeavesItUpPutsItAway(t *testing.T) {
	stack := &bkRecord{can: true}
	a := bkEdgeApp(t, func() View {
		return bk{key: "stack", r: stack, children: []View{bk{key: "field", keyboard: true, focusable: true}}}
	})
	focusKey(t, a, "field")
	bkEdgeTouch(a, geom.Pt(10, 50))
	if !a.SoftKeyboardRequested() {
		t.Fatal("a touch on the field put the keyboard away; the fixture is wrong")
	}
	if !bkBack(a) || stack.went != 0 || a.SoftKeyboardRequested() {
		t.Fatalf("went %d, keyboard %v; want the keyboard put away and nothing popped",
			stack.went, a.SoftKeyboardRequested())
	}
}

// TestAKeyboardThatTookTheGestureItselfLeavesNoLatch: the system keyboard
// takes a back gesture and goes down on its own, which the backend reports
// with DismissSoftKeyboard; no back action follows. On the first view the
// platform must then be told that gift cannot go back, and on a pushed screen
// the next back must pop.
func TestAKeyboardThatTookTheGestureItselfLeavesNoLatch(t *testing.T) {
	for _, can := range []bool{false, true} {
		stack := &bkRecord{can: can}
		a := bkEdgeApp(t, func() View {
			return bk{key: "stack", r: stack, children: []View{bk{key: "field", keyboard: true, focusable: true}}}
		})
		focusKey(t, a, "field")
		bkEdgeTouch(a, geom.Pt(10, 50))
		a.BeginInput(a.in.now)
		a.DismissSoftKeyboard()
		bkUpdate(t, a)
		if a.CanGoBack() != can {
			t.Fatalf("can=%v: CanGoBack is %v after the keyboard went down on its own", can, a.CanGoBack())
		}
		if can && (!bkBack(a) || stack.went != 1) {
			t.Fatalf("the next back went %d times, want 1", stack.went)
		}
	}
}

// TestAFocusedNodeInATrapWithoutAHandlerDoesNotReachBehindIt: the trap
// bounds the route whether something is focused or not, so the stack behind
// a dialog of the application's own does not pop.
func TestAFocusedNodeInATrapWithoutAHandlerDoesNotReachBehindIt(t *testing.T) {
	stack := &bkRecord{can: true}
	a := bkApp(t, func() View {
		return bk{key: "stack", r: stack, children: []View{
			bk{key: "dialog", trap: true, children: []View{bk{key: "button", focusable: true}}},
		}}
	})
	if a.CanGoBack() {
		t.Fatal("CanGoBack is true behind the trap with nothing focused")
	}
	focusKey(t, a, "button")
	if a.CanGoBack() || bkBack(a) || stack.went != 0 {
		t.Fatalf("with the button focused the stack behind the trap went %d times", stack.went)
	}
}

// TestCanGoBackAndBackAllocateNothingAfterAnEdgeTouch: the latch is two
// fields, set and cleared on every press, and must not cost a press an
// allocation either.
func TestCanGoBackAndBackAllocateNothingAfterAnEdgeTouch(t *testing.T) {
	stack := &bkRecord{can: true}
	a := bkEdgeApp(t, func() View { return bk{key: "stack", r: stack} })
	a.BeginInput(a.in.now)
	gesture := func() {
		a.PointerDown(1, PointerTouch, geom.Pt(10, 10))
		a.PointerCancel(1)
		a.CanGoBack()
		a.Back()
	}
	gesture() // the pointer slot is made once
	if n := testing.AllocsPerRun(100, gesture); n != 0 {
		t.Fatalf("an edge touch and back allocate %v times", n)
	}
	if stack.went == 0 {
		t.Fatal("the route under test went nowhere; the fixture is wrong")
	}
}

// TestNoLatchWithoutABackGestureEdge: the default, as on a desktop, on iOS, in
// gifttest and in auto. An edge touch whose press closed the menu leaves the
// back action to what is under it, as there is no gesture it could have been.
func TestNoLatchWithoutABackGestureEdge(t *testing.T) {
	page, menu := &bkRecord{can: true}, &bkRecord{can: true, dismissOnPress: true}
	a := bkApp(t, func() View {
		return bk{key: "root", children: []View{
			bk{key: "page", r: page},
			bk{key: "menu", r: menu},
		}}
	})
	bkEdgeTouch(a, geom.Pt(10, 10))
	if a.in.backLatch {
		t.Fatal("an edge touch latched without an edge reported")
	}
	if !bkBack(a) || page.went != 1 {
		t.Fatalf("page went %d, want the back action to reach it", page.went)
	}
}

// TestSettingTheBackGestureEdgeToZeroDropsTheLatch: a backend that withdraws
// the gesture leaves no answer behind.
func TestSettingTheBackGestureEdgeToZeroDropsTheLatch(t *testing.T) {
	stack := &bkRecord{can: true}
	a := bkEdgeApp(t, func() View { return bk{key: "stack", r: stack} })
	bkEdgeTouch(a, geom.Pt(10, 10))
	if !a.in.backLatch {
		t.Fatal("the edge touch latched nothing; the fixture is wrong")
	}
	a.SetBackGestureEdge(0)
	if a.in.backLatch || a.BackGestureEdge() != 0 {
		t.Fatal("an edge of zero left the latch set")
	}
}

// bkEdgeTap presses a finger at pos and lifts it in place: a tap at the edge,
// on the back button of a navigation bar, say, and not the gesture.
func bkEdgeTap(a *App, pos geom.Point) {
	a.BeginInput(a.in.now)
	a.PointerDown(1, PointerTouch, pos)
	a.PointerUp(1, PointerTouch, pos)
}

// TestAnEdgeTapDropsTheLatch: the tap pops the stack to its first view, and
// the next back must leave the application and not be spent on the answer
// the press latched.
func TestAnEdgeTapDropsTheLatch(t *testing.T) {
	stack := &bkRecord{can: true}
	a := bkEdgeApp(t, func() View { return bk{key: "stack", r: stack} })
	bkEdgeTap(a, geom.Pt(10, 10))
	stack.can = false // the tap's release popped to the first view
	bkUpdate(t, a)
	if a.CanGoBack() {
		t.Fatal("CanGoBack is true on the first view after an edge tap")
	}
	if bkBack(a) {
		t.Fatal("Back took the action on the first view after an edge tap")
	}
}

// TestAnEdgeTapThatOpensASheetLeavesBackToTheSheet: a trailing button at the
// right edge opens a sheet, and the next back closes it.
func TestAnEdgeTapThatOpensASheetLeavesBackToTheSheet(t *testing.T) {
	stack, sheet := &bkRecord{can: true}, &bkRecord{}
	a := bkEdgeApp(t, func() View {
		return bk{key: "root", children: []View{
			bk{key: "stack", r: stack},
			bk{key: "sheet", r: sheet},
		}}
	})
	bkEdgeTap(a, geom.Pt(390, 10))
	sheet.can = true // the tap's release presented the sheet
	bkUpdate(t, a)
	if !bkBack(a) || sheet.went != 1 || stack.went != 0 {
		t.Fatalf("sheet went %d, stack went %d; want the sheet closed", sheet.went, stack.went)
	}
}

// bkEdgeMenu is a page under an open menu that a press closes, the page
// able to go back or not.
func bkEdgeMenu(t *testing.T, pageCan bool) (*App, *bkRecord, *bkRecord) {
	t.Helper()
	page, menu := &bkRecord{can: pageCan}, &bkRecord{can: true, dismissOnPress: true}
	a := bkEdgeApp(t, func() View {
		return bk{key: "root", children: []View{
			bk{key: "page", r: page},
			bk{key: "menu", r: menu},
		}}
	})
	return a, page, menu
}

// TestAnEdgeTapThatClosedTheMenuDropsTheLatch: a tap outside the open menu
// that lands in the edge zone, its press and release a frame apart, closes
// the menu as any tap outside does. It was no gesture, whose touch the
// platform cancels, and the next back is an ordinary one: it pops the page,
// and on the first view it leaves the application.
func TestAnEdgeTapThatClosedTheMenuDropsTheLatch(t *testing.T) {
	for _, pageCan := range []bool{true, false} {
		a, page, menu := bkEdgeMenu(t, pageCan)
		a.BeginInput(a.in.now)
		a.PointerDown(1, PointerTouch, geom.Pt(10, 10))
		bkUpdate(t, a)
		a.BeginInput(a.in.now)
		a.PointerUp(1, PointerTouch, geom.Pt(10, 10))
		bkUpdate(t, a)
		if menu.can {
			t.Fatal("the edge tap did not close the menu; the fixture is wrong")
		}
		if a.CanGoBack() != pageCan {
			t.Fatalf("page can %v: CanGoBack = %v after the tap closed the menu",
				pageCan, a.CanGoBack())
		}
		if bkBack(a) != pageCan || page.went != map[bool]int{true: 1}[pageCan] {
			t.Fatalf("page can %v: page went %d; want an ordinary back", pageCan, page.went)
		}
	}
}

// TestASecondFingerDuringTheGestureKeepsTheLatch: a further finger at the
// edge while the gesture's own is down belongs to the same gesture, and must
// not latch the page that the first finger uncovered.
func TestASecondFingerDuringTheGestureKeepsTheLatch(t *testing.T) {
	a, page, _ := bkEdgeMenu(t, true)
	a.BeginInput(a.in.now)
	a.PointerDown(1, PointerTouch, geom.Pt(10, 10))
	a.PointerDown(2, PointerTouch, geom.Pt(390, 10))
	a.PointerCancel(1)
	a.PointerCancel(2)
	if !bkBack(a) || page.went != 0 {
		t.Fatalf("page went %d; want the back action spent on the closed menu", page.went)
	}
}

// TestAFurtherFingerAwayFromTheEdgeDuringTheGestureKeepsTheLatch: on a node
// that takes several fingers, a second one in the middle while the gesture's
// own is down is part of the same gesture too.
func TestAFurtherFingerAwayFromTheEdgeDuringTheGestureKeepsTheLatch(t *testing.T) {
	page, menu := &bkRecord{can: true}, &bkRecord{can: true, dismissOnPress: true}
	a := bkEdgeApp(t, func() View {
		return bk{key: "root", multi: true, children: []View{
			bk{key: "page", r: page},
			bk{key: "menu", r: menu},
		}}
	})
	a.BeginInput(a.in.now)
	a.PointerDown(1, PointerTouch, geom.Pt(10, 10))
	a.PointerDown(2, PointerTouch, geom.Pt(60, 10))
	if a.diag.DiscardedTouches != 0 {
		t.Fatal("the second finger was refused; the fixture is wrong")
	}
	a.PointerCancel(1)
	a.PointerCancel(2)
	if !bkBack(a) || page.went != 0 {
		t.Fatalf("page went %d; want the back action spent on the closed menu", page.went)
	}
}

// TestARefusedEdgeFingerLatchesNothing: a thumb resting at the edge that gift
// refuses while another finger taps must not outlive the tap. Its release
// never reaches a slot, so a latch it set would never be dropped, and the
// first view would keep the back the platform should have.
func TestARefusedEdgeFingerLatchesNothing(t *testing.T) {
	stack := &bkRecord{can: true}
	a := bkEdgeApp(t, func() View { return bk{key: "stack", r: stack} })
	a.BeginInput(a.in.now)
	a.PointerDown(1, PointerTouch, geom.Pt(200, 50))
	a.PointerDown(2, PointerTouch, geom.Pt(10, 50))
	if a.in.backLatch {
		t.Fatal("a refused finger at the edge latched")
	}
	a.PointerUp(2, PointerTouch, geom.Pt(10, 50))
	a.PointerUp(1, PointerTouch, geom.Pt(200, 50))
	stack.can = false // finger 1's tap popped to the first view
	bkUpdate(t, a)
	if a.CanGoBack() || bkBack(a) {
		t.Fatal("back is still answered on the first view after a refused edge finger")
	}
}

// TestANewEdgeTouchAfterACancelledOneLatchesAfresh: the gesture whose touch
// closed the menu was not finished, and the next one begins with a touch of
// its own. It finds the page, not the menu, and its back pops it.
func TestANewEdgeTouchAfterACancelledOneLatchesAfresh(t *testing.T) {
	a, page, _ := bkEdgeMenu(t, true)
	bkEdgeTouch(a, geom.Pt(10, 10))
	bkUpdate(t, a)
	bkEdgeTouch(a, geom.Pt(10, 10))
	if !bkBack(a) || page.went != 1 {
		t.Fatalf("page went %d after a second gesture, want 1", page.went)
	}
}

// TestAnAbandonedGestureDropsTheLatch: the user starts the gesture, which
// closes the menu, and lets it go back. The platform reports that with
// [App.BackCancel], and on the first view CanGoBack is false again, so
// that the next gesture leaves the application instead of being swallowed.
func TestAnAbandonedGestureDropsTheLatch(t *testing.T) {
	a, page, _ := bkEdgeMenu(t, false)
	bkEdgeTouch(a, geom.Pt(10, 10))
	if !a.CanGoBack() {
		t.Fatal("the gesture's touch latched nothing; the fixture is wrong")
	}
	a.BeginInput(a.in.now)
	a.BackCancel()
	bkUpdate(t, a)
	if a.CanGoBack() {
		t.Fatal("CanGoBack is true on the first view after the gesture was abandoned")
	}
	bkEdgeTouch(a, geom.Pt(10, 10))
	if bkBack(a) || page.went != 0 {
		t.Fatalf("the next gesture on the first view was taken (page went %d)", page.went)
	}
}

// TestAFocusRequestedOnBackKeepsTheProvenance: back is neither a key nor a
// press. A focus its handler requests shows the ring only if the focus
// before it did: not after nothing was focused, as after touches, and yes
// after the keyboard moved the focus.
func TestAFocusRequestedOnBackKeepsTheProvenance(t *testing.T) {
	for _, keyboard := range []bool{false, true} {
		r := &bkRecord{can: true, focusOnBack: true}
		a := bkApp(t, func() View {
			return bk{key: "root", children: []View{
				bk{key: "other", focusable: true},
				bk{key: "detail", r: r, focusable: true},
			}}
		})
		if keyboard {
			focusKey(t, a, "other")
		}
		if !bkBack(a) || r.went != 1 {
			t.Fatalf("keyboard %v: the handler went %d times, want 1", keyboard, r.went)
		}
		if f, ok := a.Focus(); !ok || a.store.Get(f.h).Key != "detail" {
			t.Fatalf("keyboard %v: the handler's request did not move the focus", keyboard)
		}
		if a.FocusVisible() != keyboard {
			t.Fatalf("keyboard %v: FocusVisible = %v after back", keyboard, a.FocusVisible())
		}
	}
}

// TestBackIgnoresAFocusOutsideTheActiveTrap: a trap that was always declared
// and is merely un-hidden does not pull the focus out (only installing one
// does), and back must still stay inside it.
func TestBackIgnoresAFocusOutsideTheActiveTrap(t *testing.T) {
	stack, modal := &bkRecord{can: true}, &bkRecord{can: true}
	hidden := true
	a := bkApp(t, func() View {
		return bk{key: "root", children: []View{
			bk{key: "stack", r: stack, children: []View{bk{key: "field", focusable: true}}},
			bk{key: "modal", r: modal, trap: true, hidden: hidden},
		}}
	})
	focusKey(t, a, "field")
	hidden = false
	bkUpdate(t, a)
	if !bkBack(a) || stack.went != 0 || modal.went != 1 {
		t.Fatalf("stack went %d, modal went %d; want the focus outside the trap ignored", stack.went, modal.went)
	}
}
