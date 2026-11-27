package ebiten

import (
	"sync"
	"testing"

	eb "github.com/hajimehoshi/ebiten/v2"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

var backTestType = gift.RegisterType("ebiten.backTest")

// backView is the smallest navigation stack there is: a node whose
// interactor is a [gift.BackHandler] over a counter of screens, which goes
// back while the counter is above zero.
type backView struct{ depth *int }

func (backView) ViewType() gift.TypeID { return backTestType }

func (v backView) Build(*gift.BuildContext) gift.Element {
	n := &backNode{depth: v.depth}
	return gift.Element{Key: "stack", Layouter: n, Interactor: n}
}

type backNode struct{ depth *int }

func (n *backNode) Layout(*gift.LayoutContext, geom.Constraints) geom.Size {
	return geom.Sz(100, 100)
}

func (n *backNode) HandleEvent(*gift.EventContext, gift.Event) bool { return false }

func (n *backNode) CanGoBack() bool { return *n.depth > 0 }

func (n *backNode) GoBack(*gift.EventContext) { *n.depth-- }

// resetBack forgets every request of [Back], the published answer, the
// listener and the game they wake, so that a test starts from a process in
// which nothing happened. All of it is process wide.
func resetBack() {
	backRequests.Store(0)
	canGoBack.Store(false)
	backListener.Store(nil)
	resetSafeArea()
}

// backGame is the game of NewGame over a [backView] depth screens deep, with
// input — the touch panel off, so that a Linux test reads no /dev/input —
// created and laid out as Ebitengine's first call does, and its session taken
// down when the test ends.
func backGame(t *testing.T, depth *int, cfg Config) (*lazyGame, *game) {
	t.Helper()
	resetBack()
	return newBackGame(t, depth, cfg)
}

// newBackGame is [backGame] without forgetting what the process was told
// before it.
func newBackGame(t *testing.T, depth *int, cfg Config) (*lazyGame, *game) {
	t.Helper()
	t.Cleanup(resetBack)
	cfg.Touchscreen = "off"
	eg, _ := NewGame(func() *gift.App {
		return gift.New(gift.Options{Root: func(*gift.Context) gift.View { return backView{depth: depth} }})
	}, cfg)
	l := eg.(*lazyGame)
	g := started(t, l)
	g.scale = func() float64 { return 1 }
	l.LayoutF(400, 800)
	return l, g
}

func update(t *testing.T, g *game) {
	t.Helper()
	if err := g.Update(); err != nil {
		t.Fatal(err)
	}
}

// TestBackBeforeTheGameExistsIsPerformedOnceTheTreeIsBuilt is the order of a
// phone that is slow to start: the host may hand over a back action before
// the game has its App. The request waits — through the first update too,
// whose poll comes before the first build and would find no node to go back
// in — and the first tick that has a tree performs it.
func TestBackBeforeTheGameExistsIsPerformedOnceTheTreeIsBuilt(t *testing.T) {
	resetBack()
	Back()
	depth := 2
	_, g := newBackGame(t, &depth, Config{})
	update(t, g)
	if depth != 2 {
		t.Fatalf("the first update, before any tree, went back: depth %d", depth)
	}
	update(t, g)
	if depth != 1 {
		t.Fatalf("depth after the first update with a tree = %d, want 1", depth)
	}
	if n := backRequests.Load(); n != 0 {
		t.Fatalf("%d requests left after it was performed", n)
	}
}

// TestBackRequestsArePerformedOnePerTick: a handler decides with what its
// node was built with, so the second back of a quick double press has to see
// the tree the first one left. Three requests are three ticks, and a fourth
// tick with nothing to go back in performs nothing.
func TestBackRequestsArePerformedOnePerTick(t *testing.T) {
	depth := 5
	_, g := backGame(t, &depth, Config{})
	update(t, g) // the first build
	Back()
	Back()
	Back()
	for i, want := range []int{4, 3, 2, 2} {
		update(t, g)
		if depth != want {
			t.Fatalf("depth after tick %d = %d, want %d", i+1, depth, want)
		}
	}
	if g.app.Diagnostics().BackRequests != 3 {
		t.Fatalf("the App counted %d back requests, want 3", g.app.Diagnostics().BackRequests)
	}
}

// TestBackDeclinedByEveryHandlerDoesNothing: a request that finds nothing to
// go back in, because the state changed between the host's question and the
// tick, does nothing. Leaving the application is the host's decision, made
// before the gesture, never the game's.
func TestBackDeclinedByEveryHandlerDoesNothing(t *testing.T) {
	depth := 0
	_, g := backGame(t, &depth, Config{})
	update(t, g)
	Back()
	update(t, g)
	if depth != 0 || backRequests.Load() != 0 {
		t.Fatalf("depth %d, %d requests left", depth, backRequests.Load())
	}
}

// TestTheListenerHearsTheAnswerOnceAfterRegistrationAndOnEveryChange: the
// host enables its back callback with the listener's argument, so it has to
// hear the answer in force as soon as it registers, and then every change and
// nothing else. A new registration — even of the same function, as a host
// recreating its activity does — is a listener that has heard nothing yet.
func TestTheListenerHearsTheAnswerOnceAfterRegistrationAndOnEveryChange(t *testing.T) {
	depth := 2
	_, g := backGame(t, &depth, Config{})
	var heard []bool
	fn := func(v bool) { heard = append(heard, v) }
	onCanGoBack(fn)
	if len(heard) != 0 {
		t.Fatal("the listener was called on the goroutine that installed it")
	}
	update(t, g)
	if len(heard) != 1 || !heard[0] || !CanGoBack() {
		t.Fatalf("after the first update the listener heard %v and CanGoBack is %v, want [true] and true",
			heard, CanGoBack())
	}
	update(t, g)
	Back()
	update(t, g) // depth 1: still true, nothing new
	if len(heard) != 1 {
		t.Fatalf("an unchanged answer was reported again: %v", heard)
	}
	Back()
	update(t, g) // depth 0: the first view, back leaves
	if len(heard) != 2 || heard[1] || CanGoBack() {
		t.Fatalf("after the last screen was popped the listener heard %v and CanGoBack is %v",
			heard, CanGoBack())
	}

	onCanGoBack(fn)
	update(t, g)
	if len(heard) != 3 || heard[2] {
		t.Fatalf("a new registration heard %v, want the answer in force once", heard)
	}

	onCanGoBack(nil)
	depth = 3
	update(t, g)
	if len(heard) != 3 {
		t.Fatalf("a removed listener was called: %v", heard)
	}
}

// TestAnUnchangedTickPublishesWithoutAllocating: the answer is worked out
// after every update of every frame on a phone, so a tick whose answer did
// not change — the common one — allocates nothing and does not reach the
// listener.
func TestAnUnchangedTickPublishesWithoutAllocating(t *testing.T) {
	depth := 2
	_, g := backGame(t, &depth, Config{})
	calls := 0
	onCanGoBack(func(bool) { calls++ })
	update(t, g)
	if calls != 1 {
		t.Fatalf("listener calls after the first update = %d, want 1", calls)
	}
	if n := testing.AllocsPerRun(1000, func() { g.publishBack() }); n != 0 {
		t.Fatalf("an unchanged tick allocates %v times", n)
	}
	if calls != 1 {
		t.Fatalf("unchanged ticks reached the listener: %d calls", calls)
	}
}

// TestNoInputDiscardsBackRequestsAndCannotGoBack: a game that reads no input
// performs no back action either, and says so, so that a host does not route
// the gesture to it and the operating system keeps it.
func TestNoInputDiscardsBackRequestsAndCannotGoBack(t *testing.T) {
	depth := 2
	_, g := backGame(t, &depth, Config{NoInput: true})
	var heard []bool
	onCanGoBack(func(v bool) { heard = append(heard, v) })
	Back()
	update(t, g)
	update(t, g)
	if depth != 2 {
		t.Fatalf("a game without input went back: depth %d", depth)
	}
	if n := backRequests.Load(); n != 0 {
		t.Fatalf("%d requests are kept by a game that reads no input", n)
	}
	if len(heard) != 1 || heard[0] || CanGoBack() {
		t.Fatalf("the listener heard %v and CanGoBack is %v, want [false] and false", heard, CanGoBack())
	}
}

// TestBackRacesTheLoop is for -race: the host calls Back, CanGoBack,
// SetBackListener and the gesture's reports on its main thread while the loop
// updates on another. The listener appends to a slice nothing else
// synchronises, which the race detector reports unless every call is on the
// loop's goroutine.
func TestBackRacesTheLoop(t *testing.T) {
	depth := 1 << 20
	_, g := backGame(t, &depth, Config{})
	g.idleTPS = 10 // a request wakes an idle game; see [game.wake]
	var heard []bool
	listener := func(v bool) { heard = append(heard, v) }
	// One request and the listener before the host starts, so that a
	// scheduler that runs the loop first does not leave it with nothing.
	onCanGoBack(listener)
	Back()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			Back()
			_ = CanGoBack()
			if i%64 == 0 {
				onCanGoBack(listener)
				CancelTouches()
				CancelBack()
			}
		}
	}()
	for range 500 {
		update(t, g)
	}
	close(stop)
	wg.Wait()

	if depth >= 1<<20 {
		t.Fatal("no request reached the loop")
	}
	if len(heard) == 0 {
		t.Fatal("the listener was never called")
	}
	for _, v := range heard {
		if !v {
			t.Fatal("the listener heard false while the stack was deep")
		}
	}
}

// TestBackSessionLetsGoOfTheGame: a session that ends is no longer the game
// a request wakes, publishes that it can go back in nothing and tells the
// listener so — no update follows to do it — so that a host's callback
// leaves the action to the operating system. Requests still waiting are
// dropped: no tick of this game will perform them, and the next game must
// not perform a gesture made for this one.
func TestBackSessionLetsGoOfTheGame(t *testing.T) {
	depth := 2
	l, g := backGame(t, &depth, Config{})
	var heard []bool
	onCanGoBack(func(v bool) { heard = append(heard, v) })
	update(t, g)
	if !CanGoBack() || len(heard) != 1 || !heard[0] {
		t.Fatalf("the game did not publish that it can go back: %v", heard)
	}
	Back()
	Back()
	l.s.close()
	if CanGoBack() {
		t.Fatal("a closed session still says it can go back")
	}
	if len(heard) != 2 || heard[1] {
		t.Fatalf("the listener heard %v when the session closed, want a final false", heard)
	}
	if n := backRequests.Load(); n != 0 {
		t.Fatalf("%d requests outlived the session they were made for", n)
	}
	Back() // must not touch the closed game
}

// TestAClosedSessionThatWasReplacedLeavesTheNewGamesBackAlone: the closer of
// a session that a later one has already replaced touches nothing — the
// requests, the answer and the listener belong to the new game now.
func TestAClosedSessionThatWasReplacedLeavesTheNewGamesBackAlone(t *testing.T) {
	depth := 2
	old, _ := backGame(t, &depth, Config{})
	_, g := newBackGame(t, &depth, Config{})
	var heard []bool
	onCanGoBack(func(v bool) { heard = append(heard, v) })
	update(t, g)
	Back()
	old.s.close()
	if !CanGoBack() || len(heard) != 1 || backRequests.Load() != 1 {
		t.Fatalf("closing a replaced session changed the new game's back: CanGoBack %v, heard %v, %d requests",
			CanGoBack(), heard, backRequests.Load())
	}
}

// idleBackGame is [backGame] ticking at an idle rate, already fallen to it:
// the state in which a report of the platform can be lost.
func idleBackGame(t *testing.T, depth *int) *game {
	t.Helper()
	_, g := backGame(t, depth, Config{})
	g.busyTPS, g.idleTPS, g.idleAfter = 60, 10, 1
	g.activeTPS = g.busyTPS
	t.Cleanup(func() { eb.SetTPS(eb.DefaultTPS) })
	update(t, g)
	for range 3 {
		g.applyIdlePolicy(false)
	}
	if !g.idle.Load() || g.activeTPS != g.idleTPS {
		t.Fatalf("the game did not fall idle: idle %v, rate %d", g.idle.Load(), g.activeTPS)
	}
	return g
}

// TestAReportThatArrivesWhileTheGameFallsIdleRaisesTheRateAgain is the lost
// wakeup: a report made after the update looked at its mailboxes but before
// the idle flag was set finds the game busy, so its wake does nothing, and
// the rate the policy then lowers would make it wait a whole idle tick. The
// policy looks at the mailboxes again after lowering the rate. The reports
// are put in the mailboxes without their wake, which is the interleaving.
func TestAReportThatArrivesWhileTheGameFallsIdleRaisesTheRateAgain(t *testing.T) {
	for _, tc := range []struct {
		name   string
		arrive func()
	}{
		{"a back request", func() { backRequests.Add(1) }},
		{"a new listener", func() { backListener.Store(&backListenerBox{fn: func(bool) {}}) }},
		{"a safe area report", func() { safeAreaReport.Store(&geom.Insets{Top: 24}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			depth := 2
			g := idleBackGame(t, &depth)
			tc.arrive()
			g.applyIdlePolicy(false)
			if g.idle.Load() || g.activeTPS != g.busyTPS || eb.TPS() != g.busyTPS {
				t.Fatalf("after %s the game stayed idle: idle %v, rate %d, Ebitengine's %d",
					tc.name, g.idle.Load(), g.activeTPS, eb.TPS())
			}
			// The next tick takes it out, and the one after may idle again.
			update(t, g)
			for range 3 {
				g.applyIdlePolicy(false)
			}
			if !g.idle.Load() || g.activeTPS != g.idleTPS {
				t.Fatalf("with the mailbox empty again the game stays busy: idle %v, rate %d",
					g.idle.Load(), g.activeTPS)
			}
		})
	}
}

// TestRemovingTheListenerWithoutSystemBackMakesCanGoBackFalse: outside
// Android the game works the answer out only for a listener, so without one
// nothing would ever update
// the last answer published while it was there. Removing it makes the answer
// false at once, and a tick whose load of the listener came before the
// removal does not bring the stale one back.
func TestRemovingTheListenerWithoutSystemBackMakesCanGoBackFalse(t *testing.T) {
	if systemBack {
		t.Skip("Android publishes the answer without a listener")
	}
	depth := 2
	_, g := backGame(t, &depth, Config{})
	onCanGoBack(func(bool) {})
	update(t, g)
	if !CanGoBack() {
		t.Fatal("the game did not publish that it can go back")
	}
	onCanGoBack(nil)
	if CanGoBack() {
		t.Fatal("removing the listener left CanGoBack true")
	}
	canGoBack.Store(true) // the store of a tick that raced the removal
	update(t, g)
	if CanGoBack() {
		t.Fatal("a tick without a listener left a stale true in CanGoBack")
	}
	if n := testing.AllocsPerRun(100, func() { g.publishBack() }); n != 0 {
		t.Fatalf("a tick without a listener allocates %v times", n)
	}
}

// backRecorder is the host of a bound package: it implements an interface
// the bound package declares, which converts to [BackListener].
type backRecorder struct{ heard []bool }

func (r *backRecorder) CanGoBackChanged(v bool) { r.heard = append(r.heard, v) }

// TestSetBackListenerInstallsAndRemovesTheListener: the bound package
// forwards the host's listener as it is, a null included, and a nil value of
// the bound package's own interface must remove the listener rather than
// panic on its method value.
func TestSetBackListenerInstallsAndRemovesTheListener(t *testing.T) {
	type bound interface{ CanGoBackChanged(canGoBack bool) }
	depth := 1
	_, g := backGame(t, &depth, Config{})
	r := &backRecorder{}
	var l bound = r
	SetBackListener(l)
	update(t, g)
	if len(r.heard) != 1 || !r.heard[0] {
		t.Fatalf("the listener heard %v, want [true]", r.heard)
	}
	var none bound
	SetBackListener(none)
	if backListener.Load() != nil {
		t.Fatal("SetBackListener(nil) left the listener installed")
	}
	depth = 0
	update(t, g)
	if len(r.heard) != 1 {
		t.Fatalf("a removed listener was called: %v", r.heard)
	}
}
