package ebiten

import (
	"math"
	"sync"
	"testing"

	"github.com/worldiety/gift/geom"
)

// safeGame is a game as the tests of the safe area need it: an App, the scale
// seam instead of the monitor, and a density override where d is positive.
func safeGame(t *testing.T, raw, d float64, static geom.Insets) *game {
	t.Helper()
	resetSafeArea()
	t.Cleanup(resetSafeArea)
	return &game{
		app:     newRecorderApp(),
		scale:   func() float64 { return raw },
		density: d,
		safe:    newSafeAreaState(static),
	}
}

// resetSafeArea forgets every report of [SetSafeArea] and the game a report
// wakes, so that a test starts from the desktop's nothing. The mailbox is
// process wide, and a test that reports would otherwise leave its report to
// every test after it.
func resetSafeArea() {
	safeAreaReport.Store(nil)
	safeAreaGame.Store(nil)
}

// TestSafeAreaReportedBeforeTheAppIsAppliedOnTheFirstUpdate is the order of a
// phone: Android tells the view about its insets before the game of NewGame
// has been laid out or updated, so before there is an App to tell. The report
// waits, and the first update hands it over.
func TestSafeAreaReportedBeforeTheAppIsAppliedOnTheFirstUpdate(t *testing.T) {
	resetSafeArea()
	t.Cleanup(resetSafeArea)
	SetSafeArea(geom.Insets{Top: 24, Bottom: 48})

	eg, _ := NewGame(newRecorderApp, Config{NoInput: true})
	l := eg.(*lazyGame)
	g := started(t, l)
	g.scale = func() float64 { return 2.75 }
	l.LayoutF(411, 914)
	if err := l.Update(); err != nil {
		t.Fatal(err)
	}
	want := geom.Insets{Top: 24, Bottom: 48}
	if got := g.app.SafeArea(); got != want {
		t.Fatalf("safe area after the first update = %+v, want %+v", got, want)
	}
}

// TestSafeAreaUnchangedTicksDoNothing: a tick with nothing new — no report, no
// other factor, no other viewport — neither calls into the App nor allocates,
// because it runs on every update of every frame. A report of the value in
// force is nothing new either.
func TestSafeAreaUnchangedTicksDoNothing(t *testing.T) {
	g := safeGame(t, 1, 0, geom.Insets{})
	g.LayoutF(400, 800)
	SetSafeArea(geom.Insets{Top: 24, Bottom: 16})
	if err := g.Update(); err != nil {
		t.Fatal(err)
	}
	if g.safe.applies != 1 {
		t.Fatalf("applications after the first update = %d, want 1", g.safe.applies)
	}

	SetSafeArea(geom.Insets{Top: 24, Bottom: 16})
	SetSafeArea(geom.Insets{Top: 24, Bottom: 16})
	if err := g.Update(); err != nil {
		t.Fatal(err)
	}
	if g.safe.applies != 1 {
		t.Fatalf("a repeated report was applied again: %d applications", g.safe.applies)
	}

	if n := testing.AllocsPerRun(1000, g.applySafeArea); n != 0 {
		t.Fatalf("an unchanged tick allocates %v times", n)
	}
	if g.safe.applies != 1 {
		t.Fatalf("unchanged ticks reached the App: %d applications", g.safe.applies)
	}

	SetSafeArea(geom.Insets{Bottom: 16})
	g.applySafeArea()
	if g.safe.applies != 2 || g.app.SafeArea() != (geom.Insets{Bottom: 16}) {
		t.Fatalf("a changed report: %d applications, safe area %+v", g.safe.applies, g.app.SafeArea())
	}
}

// TestSafeAreaDesktopIsZero: where nothing reports and nothing is configured,
// which is a desktop or a kiosk, the App keeps the zero it was created with.
func TestSafeAreaDesktopIsZero(t *testing.T) {
	g := safeGame(t, 2, 0, geom.Insets{})
	g.LayoutF(640, 480)
	if err := g.Update(); err != nil {
		t.Fatal(err)
	}
	if got := g.app.SafeArea(); got != (geom.Insets{}) {
		t.Fatalf("safe area with nothing reported = %+v, want zero", got)
	}
}

// TestSafeAreaFollowsTheDensityOverride: a density of the application's own
// divides the same screen into fewer logical pixels, and a report in the
// platform's units has to be divided the same way as the viewport is, or the
// bars of a phone would be twice their size at a density of half its own.
func TestSafeAreaFollowsTheDensityOverride(t *testing.T) {
	for _, c := range []struct {
		name    string
		raw, d  float64
		outH    float64
		viewH   int
		wantTop float32
	}{
		{"no override", 3, 0, 900, 900, 24},
		// Three device pixels per dp, laid out at two: 1.5 logical pixels
		// per dp, for the viewport and for the bar alike.
		{"density two on a 3x phone", 3, 2, 900, 1350, 36},
		{"density four on a 2x phone", 2, 4, 900, 450, 12},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := safeGame(t, c.raw, c.d, geom.Insets{})
			g.LayoutF(400, c.outH)
			if g.h != c.viewH {
				t.Fatalf("viewport height %d, want %d", g.h, c.viewH)
			}
			SetSafeArea(geom.Insets{Top: 24})
			g.applySafeArea()
			if got := g.app.SafeArea().Top; got != c.wantTop {
				t.Fatalf("top inset %v, want %v", got, c.wantTop)
			}
		})
	}
}

// TestSafeAreaFactorChangeReapplies: the factor alone changing — a density
// override that arrives, a desktop scale that changes — converts the same
// report anew.
func TestSafeAreaFactorChangeReapplies(t *testing.T) {
	g := safeGame(t, 2, 0, geom.Insets{})
	g.LayoutF(400, 800)
	SetSafeArea(geom.Insets{Left: 10})
	g.applySafeArea()
	if got := g.app.SafeArea().Left; got != 10 {
		t.Fatalf("left inset %v, want 10", got)
	}
	g.density = 1
	g.LayoutF(400, 800)
	g.applySafeArea()
	if got := g.app.SafeArea().Left; got != 20 {
		t.Fatalf("left inset after the factor doubled = %v, want 20", got)
	}
}

// TestSafeAreaIsClampedToTheViewport: a report that arrives before the size it
// belongs to, or a broken one, cannot claim more than the window. Rounding up
// to whole logical pixels is the core's; it is seen here as a phone sees it.
func TestSafeAreaIsClampedToTheViewport(t *testing.T) {
	g := safeGame(t, 1, 0, geom.Insets{})
	g.LayoutF(300, 200)
	SetSafeArea(geom.Insets{Top: 24.38, Right: float32(math.Inf(1)), Bottom: -5, Left: 1000})
	g.applySafeArea()
	want := geom.Insets{Left: 300, Top: 25}
	if got := g.app.SafeArea(); got != want {
		t.Fatalf("safe area %+v, want %+v", got, want)
	}

	// The same report on a larger window is clamped to that one.
	g.LayoutF(600, 200)
	g.applySafeArea()
	if got := g.app.SafeArea().Left; got != 600 {
		t.Fatalf("left inset on a wider window = %v, want 600", got)
	}
}

// TestSafeAreaConfigIsTheMinimum: the static safe area of Config is merged
// with a report edge by edge, the larger winning, and is in force with no
// report at all.
func TestSafeAreaConfigIsTheMinimum(t *testing.T) {
	g := safeGame(t, 1, 0, geom.Insets{Top: 10, Bottom: 30, Left: float32(math.NaN())})
	g.LayoutF(400, 800)
	g.applySafeArea()
	if got, want := g.app.SafeArea(), (geom.Insets{Top: 10, Bottom: 30}); got != want {
		t.Fatalf("safe area with nothing reported = %+v, want %+v", got, want)
	}
	SetSafeArea(geom.Insets{Top: 24, Bottom: 16, Left: 5})
	g.applySafeArea()
	if got, want := g.app.SafeArea(), (geom.Insets{Left: 5, Top: 24, Bottom: 30}); got != want {
		t.Fatalf("merged safe area = %+v, want %+v", got, want)
	}
}

// TestSafeAreaReportsRaceTheLoop is for -race: the platform reports on its own
// thread while the loop lays out and updates on another, which is the whole
// reason for the mailbox.
func TestSafeAreaReportsRaceTheLoop(t *testing.T) {
	g := safeGame(t, 2, 0, geom.Insets{})
	g.idleTPS = 10 // a report wakes an idle game; see [game.wake]
	safeAreaGame.Store(g)

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
			SetSafeArea(geom.Insets{Top: float32(i % 40), Bottom: float32(i % 7)})
		}
	}()
	for i := 0; i < 500; i++ {
		g.LayoutF(400, 800)
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()

	SetSafeArea(geom.Insets{Top: 12})
	g.applySafeArea()
	if got := g.app.SafeArea(); got != (geom.Insets{Top: 12}) {
		t.Fatalf("the last report did not win: %+v", got)
	}
}

// TestSafeAreaFollowsTheViewport: a rotation changes the viewport without a
// new report, or before it. The report in force is clamped to the new size,
// and a report that arrives a tick later is applied then.
func TestSafeAreaFollowsTheViewport(t *testing.T) {
	g := safeGame(t, 1, 0, geom.Insets{})
	g.LayoutF(400, 800)
	SetSafeArea(geom.Insets{Top: 600})
	if err := g.Update(); err != nil {
		t.Fatal(err)
	}
	if got := g.app.SafeArea().Top; got != 600 {
		t.Fatalf("top in portrait = %v, want 600", got)
	}
	// Landscape, the report not yet updated: clamped to the new height.
	g.LayoutF(800, 400)
	if err := g.Update(); err != nil {
		t.Fatal(err)
	}
	if got := g.app.SafeArea().Top; got != 400 {
		t.Fatalf("top after the rotation = %v, want 400", got)
	}
	SetSafeArea(geom.Insets{Left: 48})
	if err := g.Update(); err != nil {
		t.Fatal(err)
	}
	if got, want := g.app.SafeArea(), (geom.Insets{Left: 48}); got != want {
		t.Fatalf("safe area after the late report = %+v, want %+v", got, want)
	}
}

// TestSafeAreaSessionLetsGoOfTheGame: a session that ends no longer is the
// game a report wakes, so the mailbox does not keep it reachable.
func TestSafeAreaSessionLetsGoOfTheGame(t *testing.T) {
	resetSafeArea()
	t.Cleanup(resetSafeArea)
	eg, _ := NewGame(newRecorderApp, Config{NoInput: true})
	l := eg.(*lazyGame)
	g, err := l.game()
	if err != nil {
		t.Fatal(err)
	}
	if safeAreaGame.Load() != g {
		t.Fatal("the new session is not the game a report wakes")
	}
	l.s.close()
	if safeAreaGame.Load() != nil {
		t.Fatal("a closed session is still the game a report wakes")
	}
	SetSafeArea(geom.Insets{Top: 2, Right: 3, Bottom: 4, Left: 1}) // must not touch the closed game
}
