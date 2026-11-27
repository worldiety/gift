package ui_test

import (
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/ui"
)

// The tests of the back action of the platform — Android's back button and
// back gesture — in the containers of this package and in ui.OnBack.
//
// Every test asks two questions and checks that they agree: App.CanGoBack,
// which Android asks before the gesture to decide whether the application
// wants it at all, and App.Back, which performs it. A disagreement is the
// worst kind of defect here, because on Android the first decides whether the
// second is ever called: an application that says no and would have gone back
// is left by the system, one that says yes and then does nothing traps the
// user.

// --- fixtures ----------------------------------------------------------------

// pressBack performs the back action through the harness, as a backend does
// in an input phase, after asserting that Harness.CanGoBack predicted the
// answer. It reports whether gift took the action; false is "the system goes
// back", which on the first view of an application leaves it.
func pressBack(t *testing.T, h *gifttest.Harness) bool {
	t.Helper()
	can := h.CanGoBack()
	took := h.Back()
	if took != can {
		t.Fatalf("Harness.CanGoBack said %v and Harness.Back then answered %v", can, took)
	}
	return took
}

// slideOut lets a pop or a dismissal finish its movement, so that what a test
// finds afterwards is the resting state.
func slideOut(h *gifttest.Harness) {
	h.Advance(ui.ControlAnimation + 32*time.Millisecond)
	h.Settle()
}

// backHarness is navHarness for a root function.
func backHarness(t *testing.T, root func(*gift.Context) gift.View) *gifttest.Harness {
	t.Helper()
	return gifttest.New(t, gifttest.Options{
		Theme: ui.LightTheme(),
		Font:  loadTestFont(t),
		Size:  geom.Sz(640, 480),
		Root:  root,
	})
}

// stackOf returns the screens of a stack of the given depth, the root first;
// every screen's body is a text keyed prefix plus its level.
func stackOf(prefix string, depth int, top gift.View) []ui.ScreenSpec {
	out := []ui.ScreenSpec{ui.Screen(prefix+" root", ui.Text(prefix+" root body").Key(prefix+"-0"))}
	for i := 1; i < depth; i++ {
		var body gift.View = ui.Text(prefix + " body").Key(prefix + "-" + string(rune('0'+i)))
		if i == depth-1 && top != nil {
			body = top
		}
		out = append(out, ui.Screen(prefix+" "+string(rune('0'+i)), body))
	}
	return out
}

// --- the navigation stack ----------------------------------------------------

// TestBackPopsAPushedScreenAndIsLeftToTheSystemOnTheRoot is the whole promise
// in one test: back goes back inside the application for as long as there is
// somewhere to go, and leaves it — answers false — only on the first screen.
func TestBackPopsAPushedScreenAndIsLeftToTheSystemOnTheRoot(t *testing.T) {
	pops := 0
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		depth := ctx.State("depth", 3)
		return ui.NavigationStack(func() { pops++; depth.Set(depth.Get() - 1) },
			stackOf("s", ctx.Read(depth), nil)...)
	})
	for want := 1; want <= 2; want++ {
		if !pressBack(t, h) {
			t.Fatalf("back on a pushed screen was left to the system (pop %d)", want)
		}
		slideOut(h)
		if pops != want {
			t.Fatalf("back asked to pop %d times, want %d", pops, want)
		}
	}
	h.Find(gifttest.ByKey("s-0")).AssertVisible()
	if pressBack(t, h) {
		t.Fatal("back on the root screen was taken; the application could never be left")
	}
	if pops != 2 {
		t.Fatalf("back on the root screen asked to pop: %d pops, want 2", pops)
	}
	if got := h.Diagnostics().BackRequests; got != 3 {
		t.Fatalf("Diagnostics.BackRequests is %d, want 3", got)
	}
}

// TestBackOnAPushedScreenWithoutOnBackIsTakenAndDoesNothing: a stack built
// with a nil onBack still shows a pushed screen, and a back that fell through
// to the system there would leave the whole application from its second
// screen.
func TestBackOnAPushedScreenWithoutOnBackIsTakenAndDoesNothing(t *testing.T) {
	h := navHarness(t, ui.NavigationStack(nil, stackOf("s", 2, nil)...))
	if !pressBack(t, h) {
		t.Fatal("back on a pushed screen of a stack without onBack left the application")
	}
	h.Find(gifttest.ByKey("s-1")).AssertVisible()
}

// TestEscapeStillStopsOnTheRootScreenWhereBackDoesNot pins the one place the
// two part ways, from the side of the key: escape is consumed on the root
// screen as it always was, so it cannot travel on and close something.
func TestEscapeStillStopsOnTheRootScreenWhereBackDoesNot(t *testing.T) {
	dismissed := 0
	h := backHarness(t, func(*gift.Context) gift.View {
		return ui.Sheet(ui.Text("behind"),
			ui.NavigationStack(nil, ui.Screen("Root", ui.Text("in sheet").Key("in"))),
		).OnDismiss(func() { dismissed++ })
	})
	h.Key(gift.KeyEscape)
	h.Settle()
	if dismissed != 0 {
		t.Fatalf("escape on the root screen of a stack in a sheet dismissed it %d time(s); "+
			"the stack consumes escape there", dismissed)
	}
	if !pressBack(t, h) || dismissed != 1 {
		t.Fatalf("back on the root screen of a stack in a sheet: %d dismissals, want 1", dismissed)
	}
}

// TestBackPopsTheInnerStackFirstAndTheOuterOneWhenTheInnerIsOnItsRoot is the
// nesting rule: the last stack in document order is asked first, and on its
// root screen it lets the action through to the one around it.
func TestBackPopsTheInnerStackFirstAndTheOuterOneWhenTheInnerIsOnItsRoot(t *testing.T) {
	var inner, outer int
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		di := ctx.State("inner", 2)
		do := ctx.State("outer", 2)
		in := ui.NavigationStack(func() { inner++; di.Set(di.Get() - 1) }, stackOf("in", ctx.Read(di), nil)...)
		return ui.NavigationStack(func() { outer++; do.Set(do.Get() - 1) }, stackOf("out", ctx.Read(do), in)...)
	})
	if !pressBack(t, h) {
		t.Fatal("back with two stacks pushed was not taken")
	}
	slideOut(h)
	if inner != 1 || outer != 0 {
		t.Fatalf("the first back popped inner %d and outer %d times, want 1 and 0", inner, outer)
	}
	if !pressBack(t, h) {
		t.Fatal("back with the inner stack on its root and the outer pushed was not taken")
	}
	slideOut(h)
	if inner != 1 || outer != 1 {
		t.Fatalf("the second back popped inner %d and outer %d times, want 1 and 1", inner, outer)
	}
	if pressBack(t, h) {
		t.Fatal("back with both stacks on their root screens was taken")
	}
}

// TestBackPopsTheStackWhileATabOfTheTabBarHoldsTheFocus: the tab that was
// tapped a moment ago holds the focus and is not inside the stack of its
// tab. The focused path finds nothing, and the stack is still the one back
// belongs to.
func TestBackPopsTheStackWhileATabOfTheTabBarHoldsTheFocus(t *testing.T) {
	pops := 0
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		depth := ctx.State("depth", 2)
		sel := ctx.State("tab", 0)
		return ui.TabBar(ctx.Read(sel), func(i int) { sel.Set(i) },
			ui.Tab("Home", ui.Symbol{}, ui.NavigationStack(func() { pops++; depth.Set(1) },
				stackOf("s", ctx.Read(depth), nil)...)),
			ui.Tab("Other", ui.Symbol{}, ui.Text("other")),
		)
	})
	h.Find(gifttest.ByKey("Home").And(gifttest.ByType("ui.Button"))).Tap()
	h.Settle()
	h.Find(gifttest.ByKey("Home").And(gifttest.ByType("ui.Button"))).AssertFocused()
	if !pressBack(t, h) || pops != 1 {
		t.Fatalf("back with a tab focused: %d pops, want 1", pops)
	}
}

// TestBackPopsTheStackOfAFocusedTabBeforeTheStackAroundTheTabBar: the tab
// bar is on a pushed screen of an outer stack, and the focused tab button
// lies inside that outer stack but not inside the stack of its tab. The
// focused path finds the outer stack first; the inner one, the last in
// document order, lies inside it and goes first. Only on its root screen,
// where it declines, does the outer stack pop.
func TestBackPopsTheStackOfAFocusedTabBeforeTheStackAroundTheTabBar(t *testing.T) {
	var inner, outer int
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		di := ctx.State("inner", 2)
		do := ctx.State("outer", 2)
		sel := ctx.State("tab", 0)
		tabs := ui.TabBar(ctx.Read(sel), func(i int) { sel.Set(i) },
			ui.Tab("Home", ui.Symbol{}, ui.NavigationStack(func() { inner++; di.Set(di.Get() - 1) },
				stackOf("in", ctx.Read(di), nil)...)),
			ui.Tab("Other", ui.Symbol{}, ui.Text("other")),
		)
		return ui.NavigationStack(func() { outer++; do.Set(do.Get() - 1) }, stackOf("out", ctx.Read(do), tabs)...)
	})
	slideOut(h)
	tab := gifttest.ByKey("Home").And(gifttest.ByType("ui.Button"))
	h.Find(tab).Tap()
	h.Settle()
	h.Find(tab).AssertFocused()
	if !pressBack(t, h) {
		t.Fatal("the first back was left to the system")
	}
	slideOut(h)
	if inner != 1 || outer != 0 {
		t.Fatalf("the first back with the tab focused popped inner %d and outer %d times, want 1 and 0", inner, outer)
	}
	if !pressBack(t, h) {
		t.Fatal("the second back was left to the system")
	}
	slideOut(h)
	if inner != 1 || outer != 1 {
		t.Fatalf("the second back popped inner %d and outer %d times, want 1 and 1", inner, outer)
	}
}

// TestADisabledOnBackLaterInTheTreeDoesNotHideAStackThatCanPop: a handler
// that declines is skipped, not taken as the last one in document order and
// then found wanting. An OnBack that is off after a stack must leave back to
// the stack.
func TestADisabledOnBackLaterInTheTreeDoesNotHideAStackThatCanPop(t *testing.T) {
	var pops, backs int
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		depth := ctx.State("depth", 2)
		return ui.VStack(
			ui.NavigationStack(func() { pops++; depth.Set(1) }, stackOf("s", ctx.Read(depth), nil)...).Flex(1),
			ui.OnBack(ui.Text("footer").Key("footer"), func() { backs++ }).Disabled(true),
		)
	})
	if !pressBack(t, h) {
		t.Fatal("back on a pushed screen was left to the system behind a disabled OnBack")
	}
	if pops != 1 || backs != 0 {
		t.Fatalf("back: %d pops, %d calls of the disabled OnBack; want 1, 0", pops, backs)
	}
}

// --- the sheet ---------------------------------------------------------------

// TestBackOnTheRootOfAStackInsideASheetDismissesTheSheet: the stack in the
// sheet pops while it can, then lets back through to the sheet, and the
// stack behind the sheet is never reached while the sheet is up.
func TestBackOnTheRootOfAStackInsideASheetDismissesTheSheet(t *testing.T) {
	var inner, behind, dismissed int
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		open := ctx.State("open", true)
		di := ctx.State("inner", 2)
		db := ctx.State("behind", 2)
		in := ui.NavigationStack(func() { inner++; di.Set(di.Get() - 1) }, stackOf("in", ctx.Read(di), nil)...)
		bg := ui.NavigationStack(func() { behind++; db.Set(db.Get() - 1) }, stackOf("bg", ctx.Read(db), nil)...)
		return ui.Sheet(bg, in).
			Presented(ctx.Read(open)).
			OnDismiss(func() { dismissed++; open.Set(false) })
	})
	slideOut(h)
	pressBack(t, h)
	slideOut(h)
	if inner != 1 || dismissed != 0 || behind != 0 {
		t.Fatalf("first back: inner %d, dismissed %d, behind %d; want 1, 0, 0", inner, dismissed, behind)
	}
	pressBack(t, h)
	slideOut(h)
	if inner != 1 || dismissed != 1 || behind != 0 {
		t.Fatalf("second back: inner %d, dismissed %d, behind %d; want 1, 1, 0", inner, dismissed, behind)
	}
	// The sheet is down: the stack behind it is next.
	if !pressBack(t, h) || behind != 1 || dismissed != 1 {
		t.Fatalf("third back: dismissed %d, behind %d; want 1, 1", dismissed, behind)
	}
}

// TestASheetWithoutOnDismissTakesBackAndDoesNothing: a sheet that demands an
// answer is not closed by back, and back does not pass it to pop the screen
// behind it or to leave the application.
func TestASheetWithoutOnDismissTakesBackAndDoesNothing(t *testing.T) {
	pops := 0
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		depth := ctx.State("depth", 2)
		return ui.Sheet(
			ui.NavigationStack(func() { pops++; depth.Set(1) }, stackOf("bg", ctx.Read(depth), nil)...),
			ui.Text("question").Key("question"),
		)
	})
	slideOut(h)
	if !pressBack(t, h) {
		t.Fatal("back over a sheet without OnDismiss was left to the system")
	}
	if pops != 0 {
		t.Fatalf("back over a sheet popped the screen behind it %d time(s)", pops)
	}
	h.Find(gifttest.ByKey("question")).AssertVisible()
}

// --- the modal ---------------------------------------------------------------

// alertOver is a stack of two screens with an alert that is open from the
// start; withDismiss wires ModalView.OnDismiss. inScreen puts the modal
// inside the pushed screen rather than around the stack.
func alertOver(t *testing.T, withDismiss, inScreen bool, pops, dismissed *int) *gifttest.Harness {
	t.Helper()
	return backHarness(t, func(ctx *gift.Context) gift.View {
		depth := ctx.State("depth", 2)
		open := ctx.State("open", true)
		var alert gift.View
		if ctx.Read(open) {
			alert = ui.Alert("Careful", "Really?", ui.AlertAction("OK", func() { open.Set(false) }))
		}
		modal := func(content gift.View) gift.View {
			m := ui.Modal(content, alert)
			if withDismiss {
				m = m.OnDismiss(func() { *dismissed++; open.Set(false) })
			}
			return m
		}
		var top gift.View
		if inScreen {
			top = modal(ui.Text("detail").Key("detail"))
		}
		stack := ui.NavigationStack(func() { *pops++; depth.Set(1) }, stackOf("s", ctx.Read(depth), top)...)
		if inScreen {
			return stack
		}
		return modal(stack)
	})
}

// TestBackDismissesAnAlertAndNeverPopsTheScreenUnderIt covers both halves of
// ModalView.OnDismiss for back, and the focused path: with the alert's own
// button in the focus, back walks up from it and must stop at the modal,
// including when the modal is inside the screen of a stack whose stack is
// then its ancestor.
func TestBackDismissesAnAlertAndNeverPopsTheScreenUnderIt(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		dismiss, screen, tab  bool
		wantDismissed, wantUp int
	}{
		{name: "with OnDismiss", dismiss: true, wantDismissed: 1},
		{name: "without OnDismiss", wantUp: 1},
		{name: "with OnDismiss, inside the screen, its button focused", dismiss: true, screen: true, tab: true, wantDismissed: 1},
		{name: "without OnDismiss, inside the screen, its button focused", screen: true, tab: true, wantUp: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var pops, dismissed int
			h := alertOver(t, tc.dismiss, tc.screen, &pops, &dismissed)
			slideOut(h)
			if tc.tab {
				h.App().MoveFocus(true)
				h.Settle()
				h.Find(gifttest.ByType("ui.Button").And(gifttest.Under(gifttest.ByType("ui.Alert")))).AssertFocused()
			}
			if !pressBack(t, h) {
				t.Fatal("back over an open alert was left to the system")
			}
			slideOut(h)
			if pops != 0 {
				t.Fatalf("back over an open alert popped the screen under it %d time(s)", pops)
			}
			if dismissed != tc.wantDismissed {
				t.Fatalf("back dismissed the alert %d time(s), want %d", dismissed, tc.wantDismissed)
			}
			if got := len(h.FindAll(gifttest.ByType("ui.Alert"))); got != tc.wantUp {
				t.Fatalf("%d alert(s) after back, want %d", got, tc.wantUp)
			}
			if tc.dismiss {
				// Closed, the stack behind it is next.
				if !pressBack(t, h) || pops != 1 {
					t.Fatalf("back after the alert closed: %d pops, want 1", pops)
				}
			}
		})
	}
}

// TestTheBackActionOfAModalLeavesItsTapsAsTheyWere: the modal's back action
// sits on the node around the scrim and the dialog, as its Element.Back and
// not as an interactor, and a tap on the alert's button and on the scrim must
// reach exactly what they reached before it was there.
func TestTheBackActionOfAModalLeavesItsTapsAsTheyWere(t *testing.T) {
	hits := 0
	h := modalApp(t, &hits)
	h.Find(gifttest.ByKey("open")).Tap()
	slideOut(h)
	h.AssertExists(gifttest.ByType("ui.Alert"))
	h.TapAt(geom.Pt(10, 10))
	h.Settle()
	h.AssertExists(gifttest.ByType("ui.Alert"))
	if hits != 0 {
		t.Fatalf("a tap on the scrim reached the content: %d hits", hits)
	}
	if n := h.At(geom.Pt(10, 10)); n.Type() != "ui.scrim" {
		t.Fatalf("the corner of an open modal hits %s, want the scrim", n.Type())
	}
	h.Find(gifttest.ByKey("alert-button-0")).Tap()
	slideOut(h)
	h.AssertNone(gifttest.ByType("ui.Alert"))
}

// --- popovers and menus ------------------------------------------------------

// TestBackClosesAnOpenMenuAndThenReachesTheStack: a menu is the top of what
// the user sees, back closes it, and the one after reaches the stack under
// it — not the menu a second time while it shrinks away.
func TestBackClosesAnOpenMenuAndThenReachesTheStack(t *testing.T) {
	pops := 0
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		depth := ctx.State("depth", 2)
		menu := ui.Menu(ui.Text("More").Key("more").Padding(10),
			ui.MenuItem("Copy", func() {}),
		)
		return ui.NavigationStack(func() { pops++; depth.Set(1) }, stackOf("s", ctx.Read(depth), menu)...)
	})
	h.Find(gifttest.ByKey("more")).Tap()
	h.AdvanceTicks(20, gifttest.FrameInterval)
	h.AssertExists(gifttest.ByKey("menu-Copy"))
	if !pressBack(t, h) {
		t.Fatal("back over an open menu was left to the system")
	}
	if pops != 0 {
		t.Fatalf("back over an open menu popped the screen under it")
	}
	// Straight away, while the menu shrinks: it is on its way out and
	// no longer a candidate.
	if !pressBack(t, h) || pops != 1 {
		t.Fatalf("back right after the menu was dismissed: %d pops, want 1", pops)
	}
	h.AdvanceTicks(20, gifttest.FrameInterval)
	h.AssertNone(gifttest.ByKey("menu-Copy"))
}

// TestBackDismissesAPopoverAndBlocksWithoutOnDismiss: the popover's two cases,
// as for a modal.
func TestBackDismissesAPopoverAndBlocksWithoutOnDismiss(t *testing.T) {
	for _, wire := range []bool{true, false} {
		name := "without OnDismiss"
		if wire {
			name = "with OnDismiss"
		}
		t.Run(name, func(t *testing.T) {
			var pops, dismissed int
			h := backHarness(t, func(ctx *gift.Context) gift.View {
				depth := ctx.State("depth", 2)
				open := ctx.State("open", true)
				p := ui.Popover(ui.Text("anchor"), ui.Text("pop").Key("pop")).Presented(ctx.Read(open))
				if wire {
					p = p.OnDismiss(func() { dismissed++; open.Set(false) })
				}
				return ui.NavigationStack(func() { pops++; depth.Set(1) }, stackOf("s", ctx.Read(depth), p)...)
			})
			h.AdvanceTicks(20, gifttest.FrameInterval)
			h.AssertExists(gifttest.ByKey("pop"))
			if !pressBack(t, h) {
				t.Fatal("back over an open popover was left to the system")
			}
			h.AdvanceTicks(20, gifttest.FrameInterval)
			if pops != 0 {
				t.Fatal("back over an open popover popped the screen under it")
			}
			want := 0
			if wire {
				want = 1
			}
			if dismissed != want {
				t.Fatalf("back dismissed the popover %d time(s), want %d", dismissed, want)
			}
			if wire == h.Exists(gifttest.ByKey("pop")) {
				t.Fatalf("the popover is up: %v; want %v", !wire, !wire)
			}
		})
	}
}

// TestASecondBackWithTheFocusInALeavingOverlayReachesTheStackUnderIt: the
// focus is moved into the panel of a popover or a menu, back dismisses it,
// and a second back while it is still on its way out — the focus still in it
// — pops the stack under it. The focus there is left over and must neither
// dismiss the overlay a second time nor stop the action.
func TestASecondBackWithTheFocusInALeavingOverlayReachesTheStackUnderIt(t *testing.T) {
	for _, menu := range []bool{false, true} {
		name := "popover"
		if menu {
			name = "menu"
		}
		t.Run(name, func(t *testing.T) {
			var pops, dismissed int
			h := backHarness(t, func(ctx *gift.Context) gift.View {
				depth := ctx.State("depth", 2)
				open := ctx.State("open", true)
				var top gift.View
				if menu {
					top = ui.Menu(ui.Text("More").Key("more").Padding(10),
						ui.MenuItem("Copy", func() {}),
						ui.MenuItem("Paste", func() {}),
					)
				} else {
					top = ui.Popover(ui.Text("anchor"),
						ui.Button(ui.Text("inside"), func() {}).Key("inside"),
					).Presented(ctx.Read(open)).OnDismiss(func() { dismissed++; open.Set(false) })
				}
				return ui.NavigationStack(func() { pops++; depth.Set(1) }, stackOf("s", ctx.Read(depth), top)...)
			})
			slideOut(h)
			panel := gifttest.ByKey("inside")
			if menu {
				h.Find(gifttest.ByKey("more")).Tap()
				panel = gifttest.ByKey("ui.panel")
			}
			h.AdvanceTicks(20, gifttest.FrameInterval)
			if !focusUnder(h, panel) {
				h.App().MoveFocus(true)
				h.Settle()
			}
			if !focusUnder(h, panel) {
				t.Fatalf("the focus is not in the overlay's panel; the fixture is wrong\n%s", h.Dump())
			}
			if !pressBack(t, h) || pops != 0 {
				t.Fatalf("the first back: %d pops, want 0", pops)
			}
			if !menu && dismissed != 1 {
				t.Fatalf("the first back dismissed the popover %d time(s), want 1", dismissed)
			}
			// Straight away, while the overlay is still on its way out
			// and the focus is still in it.
			if !pressBack(t, h) || pops != 1 {
				t.Fatalf("the second back during the exit: %d pops, want 1", pops)
			}
			if !menu && dismissed != 1 {
				t.Fatalf("the second back dismissed the popover again: %d dismissals, want 1", dismissed)
			}
		})
	}
}

// focusUnder reports whether the focus is on a node that matches sel or lies
// under one.
func focusUnder(h *gifttest.Harness, sel gifttest.Selector) bool {
	for _, n := range append(h.FindAll(sel), h.FindAll(gifttest.Under(sel))...) {
		if n.Interaction().Focused {
			return true
		}
	}
	return false
}

// --- the back gesture's first touch ------------------------------------------

// edgeMenu is a stack of two screens with a menu on the pushed one, the menu
// open: the case the emulator found, where the first touch of the back
// gesture closed the menu as a tap outside it and the back that followed then
// left the application.
func edgeMenu(t *testing.T, pops *int) *gifttest.Harness {
	t.Helper()
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		depth := ctx.State("depth", 2)
		menu := ui.Menu(ui.Text("More").Key("more").Padding(10),
			ui.MenuItem("Copy", func() {}),
		)
		return ui.NavigationStack(func() { *pops++; depth.Set(depth.Get() - 1) }, stackOf("s", ctx.Read(depth), menu)...)
	})
	androidEdge(h)
	slideOut(h)
	h.Find(gifttest.ByKey("more")).Tap()
	h.AdvanceTicks(20, gifttest.FrameInterval)
	h.AssertExists(gifttest.ByKey("menu-Copy"))
	return h
}

// androidEdge reports the edge zone of Android's back gesture to the App, as
// backend/ebiten does there; without it no touch latches anything.
func androidEdge(h *gifttest.Harness) {
	h.App().SetBackGestureEdge(40)
}

// edgeTouch is the start of Android's back gesture as the application sees
// it: a finger down at the left edge, then cancelled when the system takes
// the gesture over.
func edgeTouch(h *gifttest.Harness) {
	p := geom.Pt(5, h.Size().H-40)
	h.TouchDownAt(p)
	h.CancelTouch()
	h.AdvanceTicks(20, gifttest.FrameInterval)
}

// TestTheBackGestureThatClosedAMenuWithItsTouchDoesNotAlsoPop: the gesture's
// first touch closes the menu as a tap outside it does, and back is still
// taken — Android was told so before the gesture — and spent on that, not on
// the screen under the menu. The back after it pops as usual.
func TestTheBackGestureThatClosedAMenuWithItsTouchDoesNotAlsoPop(t *testing.T) {
	pops := 0
	h := edgeMenu(t, &pops)
	edgeTouch(h)
	h.AssertNone(gifttest.ByKey("menu-Copy"))
	if !h.CanGoBack() {
		t.Fatal("after the gesture's first touch closed the menu, CanGoBack is false; " +
			"Android would leave the application")
	}
	if !pressBack(t, h) {
		t.Fatal("the back of the gesture was left to the system")
	}
	slideOut(h)
	if pops != 0 {
		t.Fatalf("the back of a gesture whose touch closed the menu popped %d time(s), want 0", pops)
	}
	if !pressBack(t, h) || pops != 1 {
		t.Fatalf("the next back: %d pops, want 1", pops)
	}
}

// TestAnEdgeTouchOnTheRootScreenDoesNotHoldOntoBack: with nothing to go back
// to there is nothing to latch, and the gesture must still leave the
// application.
func TestAnEdgeTouchOnTheRootScreenDoesNotHoldOntoBack(t *testing.T) {
	h := backHarness(t, func(*gift.Context) gift.View {
		return ui.NavigationStack(nil, ui.Screen("Root", ui.Text("root").Key("root")))
	})
	androidEdge(h)
	edgeTouch(h)
	if h.CanGoBack() {
		t.Fatal("an edge touch on the root screen made CanGoBack true; the gesture could never leave")
	}
	if pressBack(t, h) {
		t.Fatal("back on the root screen after an edge touch was taken")
	}
}

// TestATouchInTheMiddleDropsTheLatchOfAnEdgeTouch: the latch is for the
// gesture that began with the edge touch. A press anywhere else means the
// user did something else, and the next back is an ordinary one that pops.
func TestATouchInTheMiddleDropsTheLatchOfAnEdgeTouch(t *testing.T) {
	pops := 0
	h := edgeMenu(t, &pops)
	edgeTouch(h)
	h.AssertNone(gifttest.ByKey("menu-Copy"))
	// The title of the navigation bar: the middle of the window, and no
	// control — the menu's own button fills the screen under it and would
	// open the menu again.
	h.TapAt(geom.Pt(h.Size().W/2, 26))
	h.AdvanceTicks(20, gifttest.FrameInterval)
	if !pressBack(t, h) || pops != 1 {
		t.Fatalf("back after a touch in the middle: %d pops, want 1", pops)
	}
}

// TestATapOnTheBarsBackButtonDoesNotHoldOntoBack: the bar's back button sits
// in the edge zone of the gesture. A tap on it pops to the first screen, and
// the next back must leave the application, not be spent on the answer the
// tap's press latched.
func TestATapOnTheBarsBackButtonDoesNotHoldOntoBack(t *testing.T) {
	pops := 0
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		depth := ctx.State("depth", 2)
		return ui.NavigationStack(func() { pops++; depth.Set(depth.Get() - 1) }, stackOf("s", ctx.Read(depth), nil)...)
	})
	androidEdge(h)
	slideOut(h)
	c := h.Find(gifttest.ByKey("back")).Center()
	if c.X >= 40 {
		t.Fatalf("the bar's back button is at x %v, outside the edge zone; the fixture is wrong", c.X)
	}
	h.TapAt(c)
	slideOut(h)
	if pops != 1 {
		t.Fatalf("the tap on the back button popped %d times, want 1", pops)
	}
	if h.CanGoBack() {
		t.Fatal("after a tap on the bar's back button, CanGoBack is true on the first screen")
	}
	if pressBack(t, h) {
		t.Fatal("back on the first screen after a tap on the bar's back button was taken")
	}
}

// TestAnEdgeTapThatOpensAMenuLeavesBackToTheMenu: a tap at the edge opens the
// menu, and the next back closes it rather than being spent on nothing.
func TestAnEdgeTapThatOpensAMenuLeavesBackToTheMenu(t *testing.T) {
	pops := 0
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		depth := ctx.State("depth", 2)
		menu := ui.Menu(ui.Text("More").Key("more").Padding(10),
			ui.MenuItem("Copy", func() {}),
		)
		return ui.NavigationStack(func() { pops++; depth.Set(depth.Get() - 1) }, stackOf("s", ctx.Read(depth), menu)...)
	})
	androidEdge(h)
	slideOut(h)
	h.TapAt(geom.Pt(5, h.Size().H-40))
	h.AdvanceTicks(20, gifttest.FrameInterval)
	h.AssertExists(gifttest.ByKey("menu-Copy"))
	if !pressBack(t, h) {
		t.Fatal("back with the menu open was left to the system")
	}
	h.AdvanceTicks(20, gifttest.FrameInterval)
	h.AssertNone(gifttest.ByKey("menu-Copy"))
	if pops != 0 {
		t.Fatalf("the back that closed the menu also popped %d time(s)", pops)
	}
	if !pressBack(t, h) || pops != 1 {
		t.Fatalf("the next back: %d pops, want 1", pops)
	}
}

// edgeTap is a tap at the left edge whose press and release are frames
// apart, as a real finger's are: the press closes an open menu as a tap
// outside it, and the build between shows the menu gone before the release.
func edgeTap(h *gifttest.Harness) {
	p := geom.Pt(5, h.Size().H-40)
	h.TouchDownAt(p)
	h.AdvanceTicks(3, gifttest.FrameInterval)
	h.TouchUpAt(p)
	h.AdvanceTicks(20, gifttest.FrameInterval)
}

// TestAnEdgeTapThatClosedAMenuLeavesTheNextBackOrdinary: the most ordinary
// way to close a menu, a tap outside it, may land in the edge zone. It was no
// gesture, and the next back pops the screen, however long after it comes.
func TestAnEdgeTapThatClosedAMenuLeavesTheNextBackOrdinary(t *testing.T) {
	pops := 0
	h := edgeMenu(t, &pops)
	edgeTap(h)
	h.AssertNone(gifttest.ByKey("menu-Copy"))
	h.AdvanceTicks(120, gifttest.FrameInterval)
	if !pressBack(t, h) || pops != 1 {
		t.Fatalf("back after an edge tap closed the menu: %d pops, want 1", pops)
	}
}

// rootMenu is a root screen with a menu open, its trigger a small button.
func rootMenu(t *testing.T) *gifttest.Harness {
	t.Helper()
	h := backHarness(t, func(*gift.Context) gift.View {
		menu := ui.Menu(ui.Text("More").Key("more").Padding(10),
			ui.MenuItem("Copy", func() {}),
		)
		return ui.NavigationStack(nil, ui.Screen("Root", ui.VStack(menu)))
	})
	androidEdge(h)
	h.Find(gifttest.ByKey("more")).Tap()
	h.AdvanceTicks(20, gifttest.FrameInterval)
	h.AssertExists(gifttest.ByKey("menu-Copy"))
	return h
}

// TestAnEdgeTapThatClosedAMenuOnTheRootScreenLetsBackLeave: on the first
// view, once the tap closed the menu, back is the system's again.
func TestAnEdgeTapThatClosedAMenuOnTheRootScreenLetsBackLeave(t *testing.T) {
	h := rootMenu(t)
	edgeTap(h)
	h.AssertNone(gifttest.ByKey("menu-Copy"))
	if h.CanGoBack() {
		t.Fatal("CanGoBack is true on the root screen after an edge tap closed the menu")
	}
	edgeTap(h)
	if pressBack(t, h) {
		t.Fatal("back on the root screen was taken after a second edge tap")
	}
}

// TestAnAbandonedGestureOnTheRootScreenLetsTheNextOneLeave: the gesture's
// touch closed the menu, and the user let the gesture go back. Android says
// so, and the callback the latch kept enabled must be disabled again, or the
// next gesture would be given to gift and do nothing.
func TestAnAbandonedGestureOnTheRootScreenLetsTheNextOneLeave(t *testing.T) {
	h := rootMenu(t)
	edgeTouch(h)
	h.AssertNone(gifttest.ByKey("menu-Copy"))
	if !h.CanGoBack() {
		t.Fatal("the gesture's touch latched nothing; the fixture is wrong")
	}
	h.CancelBack()
	if h.CanGoBack() {
		t.Fatal("CanGoBack is true on the root screen after the gesture was abandoned")
	}
}

// TestTheBackGestureStartedOnTheBarsBackButtonPopsOnce: the bar's back
// button sits in the edge zone, and the gesture may start on it. The
// platform cancels that touch, so the button does not fire, and the back the
// gesture delivers pops one screen, not two.
func TestTheBackGestureStartedOnTheBarsBackButtonPopsOnce(t *testing.T) {
	pops := 0
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		depth := ctx.State("depth", 3)
		return ui.NavigationStack(func() { pops++; depth.Set(depth.Get() - 1) }, stackOf("s", ctx.Read(depth), nil)...)
	})
	androidEdge(h)
	slideOut(h)
	c := h.Find(gifttest.ByKey("back")).Center()
	if c.X >= 40 {
		t.Fatalf("the bar's back button is at x %v, outside the edge zone; the fixture is wrong", c.X)
	}
	h.TouchDownAt(c)
	h.TouchMoveTo(c.Add(geom.Pt(2, 0)))
	h.CancelTouch()
	h.AdvanceTicks(3, gifttest.FrameInterval)
	if !pressBack(t, h) {
		t.Fatal("the back of the gesture was left to the system")
	}
	slideOut(h)
	if pops != 1 {
		t.Fatalf("a gesture started on the bar's back button popped %d times, want 1", pops)
	}
}

// TestWithoutAnEdgeAnEdgeTouchLatchesNothing: the default, as on a desktop,
// on iOS and in a test that does not opt in. The touch that closed the menu
// was no gesture there, and the next back pops.
func TestWithoutAnEdgeAnEdgeTouchLatchesNothing(t *testing.T) {
	pops := 0
	h := edgeMenu(t, &pops)
	h.App().SetBackGestureEdge(0)
	edgeTouch(h)
	h.AssertNone(gifttest.ByKey("menu-Copy"))
	if !pressBack(t, h) || pops != 1 {
		t.Fatalf("back after an edge touch without an edge: %d pops, want 1", pops)
	}
}

// --- the on-screen keyboard --------------------------------------------------

// TestBackPutsTheOnScreenKeyboardAwayBeforeItPops is the order Android has:
// the first back closes the keyboard, the second goes back.
func TestBackPutsTheOnScreenKeyboardAwayBeforeItPops(t *testing.T) {
	ui.SetOnScreenKeyboard(nil, true)
	t.Cleanup(func() { ui.SetOnScreenKeyboard(nil, false) })
	ed := ui.NewTextEditor("")
	pops := 0
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		depth := ctx.State("depth", 2)
		return ui.VStack(
			ui.NavigationStack(func() { pops++; depth.Set(1) },
				stackOf("s", ctx.Read(depth), ui.TextField(ed).Key("field"))...).Flex(1),
			ui.OnScreenKeyboard().Key("kb"),
		)
	})
	h.Find(gifttest.ByKey("field")).Tap()
	h.Settle()
	if !h.App().SoftKeyboardRequested() {
		t.Fatal("tapping the field did not ask for the on-screen keyboard; the fixture is wrong")
	}
	if !pressBack(t, h) {
		t.Fatal("back with the on-screen keyboard up was left to the system")
	}
	if h.App().SoftKeyboardRequested() || pops != 0 {
		t.Fatalf("the first back: keyboard still up %v, %d pops; want false, 0",
			h.App().SoftKeyboardRequested(), pops)
	}
	if !pressBack(t, h) || pops != 1 {
		t.Fatalf("the second back popped %d times, want 1", pops)
	}
}

// --- ui.OnBack ---------------------------------------------------------------

// TestOnBackTakesTheBackActionOnlyWhileEnabled: the application's own
// navigation, on the first view and around a stack.
func TestOnBackTakesTheBackActionOnlyWhileEnabled(t *testing.T) {
	steps := 0
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		step := ctx.State("step", 2)
		return ui.OnBack(ui.Text("wizard").Key("wizard"), func() { steps++; step.Set(step.Get() - 1) }).
			Disabled(ctx.Read(step) == 0)
	})
	for want := 1; want <= 2; want++ {
		if !pressBack(t, h) || steps != want {
			t.Fatalf("back on step %d: %d steps back, want %d", 3-want, steps, want)
		}
	}
	if pressBack(t, h) || steps != 2 {
		t.Fatalf("back with OnBack disabled was taken: %d steps back, want 2", steps)
	}

	h = navHarness(t, ui.OnBack(ui.Text("x"), nil))
	if pressBack(t, h) {
		t.Fatal("OnBack with a nil action took the back action")
	}
}

// TestOnBackAroundAStackGetsBackOnlyAfterTheStackDeclines: a pushed screen
// pops first; on the root screen the OnBack around the stack has its turn.
func TestOnBackAroundAStackGetsBackOnlyAfterTheStackDeclines(t *testing.T) {
	var pops, outer int
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		depth := ctx.State("depth", 2)
		return ui.OnBack(
			ui.NavigationStack(func() { pops++; depth.Set(1) }, stackOf("s", ctx.Read(depth), nil)...),
			func() { outer++ })
	})
	pressBack(t, h)
	slideOut(h)
	if pops != 1 || outer != 0 {
		t.Fatalf("first back: %d pops, OnBack %d; want 1, 0", pops, outer)
	}
	pressBack(t, h)
	if pops != 1 || outer != 1 {
		t.Fatalf("second back: %d pops, OnBack %d; want 1, 1", pops, outer)
	}
}

// TestOnBackLeavesTapsAndEscapeAsTheyWere: its node is no hit target and no
// stop for bubbling events, so a tap on content without a hit target of its
// own reaches the button around it, and escape still reaches the stack.
func TestOnBackLeavesTapsAndEscapeAsTheyWere(t *testing.T) {
	var taps, backs, pops int
	h := backHarness(t, func(ctx *gift.Context) gift.View {
		depth := ctx.State("depth", 2)
		btn := ui.Button(ui.OnBack(ui.Text("label").Key("label"), func() { backs++ }),
			func() { taps++ }).Key("button")
		return ui.OnBack(
			ui.NavigationStack(func() { pops++; depth.Set(1) }, stackOf("s", ctx.Read(depth), btn)...),
			func() { backs++ })
	})
	h.Find(gifttest.ByKey("label")).Tap()
	if taps != 1 || backs != 0 {
		t.Fatalf("a tap on the label inside OnBack: %d taps, %d backs; want 1, 0", taps, backs)
	}
	h.Key(gift.KeyEscape)
	h.Settle()
	if pops != 1 || backs != 0 {
		t.Fatalf("escape: %d pops, %d backs; want 1, 0", pops, backs)
	}
}

// TestOnBackLeavesTheSiblingUnderItTappable: the wrapper is no hit target, so
// a ZStack with a button under an OnBack of the same size still delivers the
// tap to the button, whether the OnBack takes back or not.
func TestOnBackLeavesTheSiblingUnderItTappable(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			taps := 0
			h := backHarness(t, func(*gift.Context) gift.View {
				return ui.ZStack(
					ui.Button(ui.Text("under"), func() { taps++ }).Key("under").Frame(200, 100),
					ui.OnBack(ui.Box().Frame(200, 100), func() {}).Disabled(!enabled).Key("onback"),
				)
			})
			c := h.Find(gifttest.ByKey("onback")).Center()
			if n := h.At(c); n.Key() != "under" {
				t.Fatalf("the centre of the OnBack hits %q (%s), want the button under it", n.Key(), n.Type())
			}
			h.TapAt(c)
			if taps != 1 {
				t.Fatalf("a tap on the OnBack over a button: %d taps, want 1", taps)
			}
		})
	}
}

// --- cost --------------------------------------------------------------------

// TestAskingWhetherBackIsTakenAllocatesNothing: the backend asks after every
// update, so the question must cost no garbage, in a tree with every kind of
// handler on the route.
func TestAskingWhetherBackIsTakenAllocatesNothing(t *testing.T) {
	var pops, dismissed int
	h := alertOver(t, false, true, &pops, &dismissed)
	slideOut(h)
	a := h.App()
	if !a.CanGoBack() {
		t.Fatal("the fixture does not take back; the measurement would be meaningless")
	}
	if got := testing.AllocsPerRun(200, func() { a.CanGoBack() }); got != 0 {
		t.Fatalf("App.CanGoBack allocated %v times per run, want 0", got)
	}
	// A blocking alert: Back takes the action and nothing rebuilds.
	if got := testing.AllocsPerRun(200, func() { a.BeginInput(h.Now()); a.Back() }); got != 0 {
		t.Fatalf("App.Back over a blocking alert allocated %v times per run, want 0", got)
	}
}
