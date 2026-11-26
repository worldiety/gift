package ebiten

import (
	"math"
	"sync/atomic"

	"github.com/worldiety/gift/geom"
)

// The safe area on this side is a mailbox. The platform knows the bars and the
// cutout of a phone, and knows them on its own thread: Android tells a view
// about its window insets on the main thread of the activity, which is not the
// goroutine the frame loop runs on, and may do so before the game of [NewGame]
// has created its App. gift's [gift.App.SetSafeArea] may only be called on the
// UI goroutine, in logical pixels, and only once there is an App. So the
// report is left here, in the platform's units, and [game.Update] takes it
// out on the UI goroutine, converts it with the factor [game.LayoutF] worked
// out, and hands it to the App before the update that builds and lays out
// with it.
//
// Posting the report to the UI goroutine, as an application does with state
// that arrives from elsewhere, is deliberately not the pattern here: the
// factor from the platform's units to gift's logical pixels is known only to
// the backend, and only on the loop's goroutine, so the conversion has to
// happen there anyway. The project plan, section 52, records the model.

// safeAreaReport is the last report of [SetSafeArea], in Ebitengine's outside
// units. Nil means nothing was ever reported, which is a desktop or a kiosk.
// Every report that changes anything stores a new value, so the pointer
// itself says whether anything changed since the last update looked.
var safeAreaReport atomic.Pointer[geom.Insets]

// safeAreaGame is the game a report wakes when it ticks at its idle rate; see
// [game.wake]. One game per process is what a phone runs, and what a report
// from the platform is about. It is nil until the first session exists.
var safeAreaGame atomic.Pointer[game]

// SetSafeArea reports the safe area of the window: per edge, how much of the
// view the system covers or cuts off — the status bar, the navigation bar, the
// cutout of a camera — in Ebitengine's outside units, which are density
// independent pixels on Android and points on iOS. These Insets are not in
// gift's logical pixels, as [Config.SafeArea] and [gift.App.SetSafeArea]
// are: the game converts them, before its next update, with the factor only
// it knows, and hands them to its App; see [gift.App.SetSafeArea] for what
// gift does with them.
//
// It may be called from any goroutine, and before [NewGame]'s game has
// created its App: the report waits for the first update. It is process wide,
// because a phone runs one game in a process and the report is about the one
// window it has. A negative or non finite edge is zero. A report equal to the
// one in force does nothing, so a platform may report on every layout pass.
//
// The host of an application bound with ebitenmobile does not call this
// function directly: backend/ebiten is not a package an application binds,
// and gobind does not carry a struct such as geom.Insets by value. The bound
// package re-exports it with a one line forwarder that takes the four edges
// by name, and that forwarder is what the host calls.
//
// # Android
//
// The values are the insets of the window, systemBars() | displayCutout() of
// WindowInsets.getInsets, which are in pixels of the display, divided by
// display.getRealMetrics().density: the same density EbitenView divides the
// size of the view by, so that the insets and the outside size Ebitengine
// lays the game out in are in the same unit. Not the insets ignoring
// visibility, so that an immersive mode with hidden bars reports zero; never
// ime(), because the system keyboard is not part of the safe area; and not
// systemGestures(), which is where a swipe goes, not where something is drawn.
// The insets are those of the window, so the view is assumed to fill it, as
// an edge to edge EbitenView does; a view placed inside a smaller frame would
// have to subtract its own offsets first. Report from the view's
// onApplyWindowInsets and from its onLayout, where EbitenView hands
// Ebitengine the new size. Neither call waits for the frame loop, which runs
// on a thread of its own, so after a rotation an update may see the new size
// before the new insets or the other way round. That lasts one tick: the
// report is clamped to whatever viewport is in force, and is applied again
// when either of the two changes.
//
// # iOS
//
// The values are the safeAreaInsets of the view, which are in points, from
// viewSafeAreaInsetsDidChange and viewDidLayoutSubviews. Points are
// Ebitengine's outside units on iOS, so they need no conversion.
//
// # A desktop
//
// A desktop or a kiosk reports nothing and its App sees zero. A bezel or an
// overscanned television that hides an edge of the picture is a static fact,
// not a report; see [Config.SafeArea].
func SetSafeArea(in geom.Insets) {
	v := usableInsets(in)
	for {
		old := safeAreaReport.Load()
		if old != nil && *old == v {
			// The report in force, again: nothing for an update to do.
			return
		}
		if safeAreaReport.CompareAndSwap(old, &v) {
			break
		}
	}
	// A game ticking at its idle rate would apply the report up to a tick of
	// that rate later. The rotation it belongs to is happening now.
	if g := safeAreaGame.Load(); g != nil {
		g.wake()
	}
}

// reportedEdge is one edge of a report or a configuration made usable: a
// negative, NaN or infinite value is zero.
func reportedEdge(v float32) float32 {
	if !(v > 0) || math.IsInf(float64(v), 0) {
		return 0
	}
	return v
}

// usableInsets is [reportedEdge] for every edge.
func usableInsets(in geom.Insets) geom.Insets {
	return geom.Insets{
		Top: reportedEdge(in.Top), Right: reportedEdge(in.Right),
		Bottom: reportedEdge(in.Bottom), Left: reportedEdge(in.Left),
	}
}

// safeAreaState is what [game.applySafeArea] last applied, so that a tick
// with nothing new returns after a comparison.
type safeAreaState struct {
	// static is [Config.SafeArea], made usable.
	static geom.Insets

	// applied says that an update has applied anything at all. The first
	// update always does, so that a static safe area or a report made before
	// the App existed reaches the App.
	applied bool

	// report, factor, w and h are the inputs of the last application: the
	// report it converted, the factor it converted with and the viewport it
	// clamped to.
	report *geom.Insets
	factor float64
	w, h   int

	// applies counts the applications that reached the App, for the tests
	// that pin that an unchanged tick does not.
	applies int
}

// newSafeAreaState makes the state of a game from its configuration.
func newSafeAreaState(static geom.Insets) safeAreaState {
	return safeAreaState{static: usableInsets(static)}
}

// applySafeArea hands the safe area to the App, if anything it depends on
// changed since the last update: the report of [SetSafeArea], the factor of
// [game.LayoutF] or the viewport. It runs on the UI goroutine, right before
// [gift.App.Update], so the update after a report builds and lays out with
// it.
//
// The edges are the report times the factor, each clamped to the viewport so
// that a report that arrived before the size it belongs to cannot claim more
// than the window, and then the larger of that and [Config.SafeArea]. gift
// rounds them up to whole logical pixels; see [gift.App.SetSafeArea].
//
// A tick with nothing new is an atomic load and four comparisons, and
// allocates nothing. On a desktop, where nothing reports and the static safe
// area is zero, the one application there is hands the App the zero it
// already has, which the core ignores, so nothing is built or laid out
// differently.
func (g *game) applySafeArea() {
	s := &g.safe
	p := safeAreaReport.Load()
	f := g.outsideToLogical
	if !(f > 0) {
		f = 1
	}
	if s.applied && p == s.report && f == s.factor && g.w == s.w && g.h == s.h {
		return
	}
	s.applied, s.report, s.factor, s.w, s.h = true, p, f, g.w, g.h
	s.applies++

	var in geom.Insets
	if p != nil {
		w, h := float64(g.w), float64(g.h)
		in = geom.Insets{
			Top:    float32(clampEdge(float64(p.Top)*f, h)),
			Right:  float32(clampEdge(float64(p.Right)*f, w)),
			Bottom: float32(clampEdge(float64(p.Bottom)*f, h)),
			Left:   float32(clampEdge(float64(p.Left)*f, w)),
		}
	}
	in.Top = max(in.Top, s.static.Top)
	in.Right = max(in.Right, s.static.Right)
	in.Bottom = max(in.Bottom, s.static.Bottom)
	in.Left = max(in.Left, s.static.Left)
	g.app.SetSafeArea(in)
}

// clampEdge limits one edge to the extent of the viewport along it. A
// viewport not known yet, zero, limits nothing.
func clampEdge(v, extent float64) float64 {
	if extent > 0 && v > extent {
		return extent
	}
	return v
}
