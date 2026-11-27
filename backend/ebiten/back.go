package ebiten

import (
	"runtime"
	"sync/atomic"
)

// The back action of the platform crosses the same border the safe area does
// (see safearea.go), in both directions. Android's back button and back
// gesture arrive on the main thread of the activity, which is not the
// goroutine the frame loop runs on, and [gift.App.Back] may only be called on
// the UI goroutine, inside the input phase, once there is an App. And the
// operating system has to know *before* the gesture starts whether the
// application will take it: with predictive back (API 33 and later, opted
// in, the default from targetSdk 36), Android asks no key handler any more,
// it delivers the gesture to a callback the application enabled beforehand,
// or, with none enabled, goes back itself — which on the first view of an
// application leaves it. So the host enables
// its callback exactly while gift can go back, and needs that answer on its
// own thread, without waiting for the loop.
//
// So both are mailboxes. A request of [Back] is a count the next ticks take
// out, one per tick, in the input phase; the answer of
// [gift.App.CanGoBack] is a flag the game publishes after every update, which
// [CanGoBack] reads and the listener of [SetBackListener] is told when it
// changes. [CancelBack], the gesture's abandonment, is a flag the next tick
// takes out. The touch the gesture began with is ended by the platform's touch
// cancel, which the back gesture is only one cause of; see [CancelTouches].

// backRequests is the number of back actions [Back] reported that no tick has
// performed yet. It is process wide, like the safe area, because a phone runs
// one game in a process and the button is about the one window it has.
var backRequests atomic.Int64

// canGoBack is the answer of [gift.App.CanGoBack] the game published last.
// False before the game exists, which is what the host should assume then:
// there is nothing to go back in.
var canGoBack atomic.Bool

// backCancels says that [CancelBack] was called and no tick has passed the
// report on yet.
var backCancels atomic.Bool

// backListener is the listener of [SetBackListener], boxed so that a
// registration is a new pointer: the game compares it with the box it
// reported to last, and a new box is a listener that has heard nothing yet,
// even when it wraps the same function.
var backListener atomic.Pointer[backListenerBox]

type backListenerBox struct{ fn func(canGoBack bool) }

// systemBack reports that the platform has a back action of its own that
// asks [CanGoBack] before it acts: Android's back button and back gesture.
// iOS has none, so there, as on a desktop, the answer is worked out only for
// a listener of [SetBackListener].
const systemBack = runtime.GOOS == "android"

// androidBackEdge is the edge zone of Android's back gesture in logical
// pixels, which the game reports with [gift.App.SetBackGestureEdge]. The
// system's own zone is 24 dp at the default sensitivity and wider at the
// highest; 40 covers both.
const androidBackEdge = 40

// Back performs the back action of the platform: the back button or the back
// gesture of Android. The next tick of the game performs it, on the UI
// goroutine and in the input phase, through [gift.App.Back]: an on-screen
// keyboard is put away, else the [gift.BackHandler] on the route of the
// focus or the last one in the document goes back — a navigation stack pops,
// a sheet, a popover or a menu closes. See [gift.App.Back].
//
// It may be called from any goroutine, and before [NewGame]'s game has
// created its App; it never blocks and never waits for the loop. A request
// made before the App has built its tree for the first time waits for that
// build, because before it there is nothing a back action could reach.
// Several requests are performed one per tick, not in a burst, as
// [gift.App.Back] requires: a handler decides with what its node was built
// with, so the second back of a quick double press has to see the tree the
// first one left, which only the build between the two ticks makes. A game
// ticking at its idle rate is woken, and kept awake while requests remain.
//
// Whether gift took the action is not reported back: the host calls Back only
// while [CanGoBack] is true, which is the promise that it will. A request
// that finds nothing to go back in, because the state changed in the tick
// between, does nothing; leaving the application is the host's decision,
// made before the gesture, and never this function's. With [Config.NoInput]
// the game reads no input and requests are discarded.
//
// The host of an application bound with ebitenmobile does not call this
// function directly; the bound package forwards it, see [NewGame].
func Back() {
	backRequests.Add(1)
	// A game ticking at its idle rate would go back up to a tick of that
	// rate later, and the user sees the gesture end now.
	wakePlatformGame()
}

// CanGoBack reports whether the game takes a back action now: the answer of
// [gift.App.CanGoBack] that the game published after its last update. It may
// be called from any goroutine and never blocks; it is false before the game
// exists, after its session ended, with [Config.NoInput], and, outside
// Android, without a listener of [SetBackListener], where nothing asks and
// the game does not work it out: removing the last listener there makes it
// false at once, rather than leaving the answer of the last tick that had
// one.
//
// It is the value the host enables its back callback with: enabled, the
// callback receives the gesture and calls [Back]; disabled, the host's own
// fallback runs, which on the first view leaves the application.
func CanGoBack() bool {
	return canGoBack.Load()
}

// BackListener is told when [CanGoBack] changes; see [SetBackListener].
// gobind passes interfaces but not funcs, so a bound package declares an
// interface with the same method and forwards it.
type BackListener interface {
	CanGoBackChanged(canGoBack bool)
}

// SetBackListener installs l as the listener of [CanGoBack], replacing the
// one before it; nil (a null from the host) removes it. It may be called from
// any goroutine.
//
// CanGoBackChanged is called on the game's UI goroutine — never on the
// goroutine that installs it, and never on the host's main thread — after an
// update whose answer differs from the one l was told last, and once after it
// was installed, with the answer in force, so that the host does not have to
// read [CanGoBack] to start from. It must not block: the frame waits for it.
// A host that touches its views posts to its main thread, and in the posted
// code reads CanGoBack rather than trusting the argument, because two posts
// may run in an order other than the one they were made in, and the latest
// value is the right one either way.
//
// There is one call that is not the UI goroutine's: when the game's session
// ends, l is told false on the goroutine that ends it — the one [Run] returns
// on, after the loop has stopped — because no update follows to tell it, and
// a host whose callback stayed enabled would swallow the action for a game
// that is gone.
//
// A listener makes the game publish its answer on every platform; without one
// the game publishes only on Android.
func SetBackListener(l BackListener) {
	if l == nil {
		onCanGoBack(nil)
		return
	}
	onCanGoBack(l.CanGoBackChanged)
}

// onCanGoBack installs fn as the listener; SetBackListener and the tests use
// it.
func onCanGoBack(fn func(canGoBack bool)) {
	if fn == nil {
		backListener.Store(nil)
		if !systemBack {
			// Nothing works the answer out outside Android any more; the
			// last one would stand for ever. [game.publishBack] stores the
			// same, in case its tick loaded the listener before it went.
			canGoBack.Store(false)
		}
		return
	}
	backListener.Store(&backListenerBox{fn: fn})
	// The first report is the next tick's. An idle game would make the host
	// wait for it as long as an idle tick lasts.
	wakePlatformGame()
}

// CancelBack reports that the platform's back gesture ended without a back
// action: on Android the back callback's handleOnBackCancelled, when the user
// let the gesture go back to where it started. The next tick passes it on to
// [gift.App.BackCancel], so that a menu the gesture's first touch closed
// does not keep [CanGoBack] true and the next back action spent on nothing.
//
// It may be called from any goroutine and never blocks. With [Config.NoInput]
// the report is discarded.
func CancelBack() {
	backCancels.Store(true)
	wakePlatformGame()
}

// backState is the game's side of the two mailboxes.
type backState struct {
	// reportedTo is the listener box the game reported to last, and
	// reported the value it reported. A box other than the one installed now
	// has heard nothing yet.
	reportedTo *backListenerBox
	reported   bool
}

// takeBackRequest takes one request of [Back] out of the mailbox, and reports
// whether there was one.
func takeBackRequest() bool {
	for {
		n := backRequests.Load()
		if n <= 0 {
			return false
		}
		if backRequests.CompareAndSwap(n, n-1) {
			return true
		}
	}
}

// pollBack performs one request of [Back], if there is one and the App has a
// tree to perform it in. It runs inside [inputBridge.poll], in the input
// phase, before the touches of the tick and before [imeBridge.report].
// Before the touches, because a back action or an abandoned gesture the
// platform reports belongs to a gesture that began earlier: a touch that
// came down at the edge in the same tick is the start of the next one, and
// the answer it latches must not be spent or dropped by its predecessor.
// Before the report, because a back action that put the on-screen keyboard
// away is then reported to the platform in this same tick, as an escape key
// that blurs a field is, instead of a tick later.
//
// A report of [CancelBack] is passed on before it, in the same tick: the
// platform reports an abandoned gesture only after the back of an earlier
// one, and a back still waiting finds the latch it would have dropped anyway.
//
// It records in mailboxTaken whether it performed one or passed a report on,
// as [inputBridge.cancelTouches] does, so that the idle policy counts the
// tick as busy: a handler's change shows in the paint flag anyway, but a back
// action that changed nothing visible would otherwise leave a woken game at
// the busy rate with nothing to bring it down again; see [game.wake].
func (b *inputBridge) pollBack() {
	if backCancels.Load() && backCancels.Swap(false) {
		b.app.BackCancel()
		b.mailboxTaken = true
	}
	if !b.backReady || !takeBackRequest() {
		return
	}
	b.app.Back()
	b.mailboxTaken = true
}

// publishBack works out [gift.App.CanGoBack] after an update and hands it to
// [CanGoBack] and to the listener of [SetBackListener]. It runs on the UI
// goroutine, after [gift.App.Update], so the answer is the one of the tree
// the platform is about to show.
//
// It does nothing without a listener outside Android, where nobody asks: the
// question walks the tree of an application whose navigation stacks are all
// on their root screen. A tick whose answer did not change is that walk, an
// atomic store and a comparison, and allocates nothing.
//
// It reports whether it called the listener, which the idle policy counts as
// work: [SetBackListener] woke the game for that call, and a tick that does
// nothing else must still bring the woken game back to its bookkeeping.
func (g *game) publishBack() bool {
	l := backListener.Load()
	if !systemBack && l == nil {
		// Nobody asks, so nothing is worked out; but an answer published
		// while a listener was installed must not outlive it. The load keeps
		// the common tick free of a store to a shared cache line.
		if canGoBack.Load() {
			canGoBack.Store(false)
		}
		g.back.reportedTo = nil
		return false
	}
	v := g.input != nil && g.app.CanGoBack()
	canGoBack.Store(v)
	if l == nil {
		g.back.reportedTo = nil
		return false
	}
	if l == g.back.reportedTo && v == g.back.reported {
		return false
	}
	g.back.reportedTo, g.back.reported = l, v
	l.fn(v)
	return true
}

// discardPlatformInput forgets what the platform left for the next tick: the
// requests of [Back] and the reports of [CancelBack] and [CancelTouches]. A
// game with [Config.NoInput] reads no input, and all three are input. Left in
// their mailboxes, they would keep the game awake for ever.
func discardPlatformInput() {
	if backRequests.Load() != 0 {
		backRequests.Store(0)
	}
	if touchCancels.Load() {
		touchCancels.Store(false)
	}
	if backCancels.Load() {
		backCancels.Store(false)
	}
}

// closeBack is the back action's part of a session that ends, run by its
// closer when the game was still the one the mailboxes wake: requests of
// [Back] and the reports still waiting, the touch cancel's among them, are
// dropped, since no tick will perform them and the next game must not perform
// a gesture made for this one; the answer becomes false; and the listener of
// [SetBackListener] is told so, on the closing goroutine, because no update
// follows to tell it and its callback must hand the action back to the
// operating system.
func closeBack() {
	discardPlatformInput()
	canGoBack.Store(false)
	if l := backListener.Load(); l != nil {
		l.fn(false)
	}
}

// platformInputPending reports whether requests of [Back] or reports of
// [CancelBack] and [CancelTouches] wait for a tick, which keeps the game at
// its busy rate until the last one is taken out.
func platformInputPending() bool {
	return backRequests.Load() > 0 || touchCancels.Load() || backCancels.Load()
}
