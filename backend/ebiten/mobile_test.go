package ebiten

import (
	"errors"
	"os"
	"testing"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

func newRecorderApp() *gift.App {
	return gift.New(gift.Options{
		Root: func(*gift.Context) gift.View { return recorderView{app: &keyboardApp{}} },
	})
}

// started returns the game of l, created as Ebitengine's first call creates
// it, and takes its session down when the test ends: NewGame never does, and
// a test on Linux would leave a touch panel reader running.
func started(t *testing.T, l *lazyGame) *game {
	t.Helper()
	g, err := l.game()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.s.close)
	return g
}

// TestNewGameIsTheGameRunDrives: the game NewGame hands out drives the game
// Run would have driven, built from the same Config, so a mobile application
// gets the frame model, the input bridge and the renderer of a desktop one.
func TestNewGameIsTheGameRunDrives(t *testing.T) {
	var app *gift.App
	g, opts := NewGame(func() *gift.App { app = newRecorderApp(); return app },
		Config{TPS: 30, IdleTPS: 5, IdleFrames: 7, NoMouse: true, Touchscreen: "off"})
	if opts == nil {
		t.Fatal("NewGame returned no RunGameOptions")
	}
	l, ok := g.(*lazyGame)
	if !ok {
		t.Fatalf("NewGame returned %T, want *lazyGame", g)
	}
	if app != nil {
		t.Fatal("NewGame created the App itself; that is the first call's job")
	}
	gg := started(t, l)
	if gg.app != app {
		t.Fatal("the game drives another App than the constructor made")
	}
	if gg.busyTPS != 30 || gg.idleTPS != 5 || gg.idleAfter != 7 {
		t.Fatalf("tick rates busy=%d idle=%d after=%d, want 30, 5 and 7", gg.busyTPS, gg.idleTPS, gg.idleAfter)
	}
	if gg.input == nil || !gg.input.noMouse {
		t.Fatal("the input bridge is missing or reads the mouse despite NoMouse")
	}
	if gg.r == nil {
		t.Fatal("the game has no renderer")
	}
	if again, _ := l.game(); again != gg {
		t.Fatal("a second call created a second game")
	}
}

// TestNewGameCreatesTheAppOnTheLoopGoroutine: the App is created by whatever
// goroutine first calls into the game — Ebitengine's loop on a phone — and is
// driven from it, which is the UI executor contract of gift.New. Under the
// giftdebug tag a violation panics in App.Invalidate, which asserts the
// executor; an App created on the test goroutine instead panics here.
func TestNewGameCreatesTheAppOnTheLoopGoroutine(t *testing.T) {
	g, _ := NewGame(newRecorderApp, Config{NoInput: true})
	l := g.(*lazyGame)
	done := make(chan error)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- errors.New("panic on the loop goroutine")
			}
		}()
		gg, err := l.game()
		if err == nil {
			gg.app.Invalidate()
			err = gg.app.Update(geom.Sz(320, 240))
		}
		done <- err
	}()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.s.close)
}

// TestNewGameDirectToScreen: a game that draws into the final screen is handed
// out with DrawFinalScreen, as Run hands directGame to RunGame, because that
// is the only way Ebitengine learns it wants the final screen.
func TestNewGameDirectToScreen(t *testing.T) {
	g, _ := NewGame(newRecorderApp, Config{DirectToScreen: true, NoInput: true})
	d, ok := g.(lazyDirectGame)
	if !ok {
		t.Fatalf("NewGame with DirectToScreen returned %T, want lazyDirectGame", g)
	}
	started(t, d.lazyGame)
}

// TestNewGameNilApp: a constructor that returns no App stops the game at its
// first Update, and every later call reports the same error.
func TestNewGameNilApp(t *testing.T) {
	g, _ := NewGame(func() *gift.App { return nil }, Config{})
	if err := g.Update(); !errors.Is(err, errNilApp) {
		t.Fatalf("Update = %v, want errNilApp", err)
	}
	if err := g.Update(); !errors.Is(err, errNilApp) {
		t.Fatalf("second Update = %v, want errNilApp", err)
	}
	if w, h := g.(*lazyGame).LayoutF(100, 50); w != 100 || h != 50 {
		t.Fatalf("LayoutF of a failed game = %v x %v, want the outside size", w, h)
	}
}

// TestSessionClosesInReverse: the closers of a session run as the defers of
// Run did, last registered first, and only once.
func TestSessionClosesInReverse(t *testing.T) {
	var order []int
	s := &session{}
	s.closers = append(s.closers, func() { order = append(order, 1) }, func() { order = append(order, 2) })
	s.close()
	s.close()
	if len(order) != 2 || order[0] != 2 || order[1] != 1 {
		t.Fatalf("closers ran as %v, want [2 1]", order)
	}
}

// TestNewGameNilConstructor: a nil constructor is a defect at the call.
func TestNewGameNilConstructor(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewGame(nil, ...) did not panic")
		}
	}()
	NewGame(nil, Config{})
}

// TestAZeroConfigIsTheMobileSetup: on Android and iOS a zero Config reads no
// mouse and serves text fields with the system keyboard, which the App is told
// is there, and on Android the App is told the edge zone of the back gesture;
// on a desktop it does none of it, so a desktop application is what it was.
// The test runs on the desktop, and the android and ios vet gates build the
// other branch.
func TestAZeroConfigIsTheMobileSetup(t *testing.T) {
	g, _ := NewGame(newRecorderApp, Config{Touchscreen: "off"})
	gg := started(t, g.(*lazyGame))
	if gg.input == nil {
		t.Fatal("the game has no input bridge")
	}
	capture := autoEnabled && os.Getenv("GIFT_CAPTURE") != ""
	if got, want := gg.input.noMouse, mobilePlatform || capture; got != want {
		t.Errorf("noMouse is %v with a zero Config, want %v", got, want)
	}
	if got := gg.input.ime != nil; got != mobilePlatform {
		t.Errorf("the system keyboard bridge is there: %v, want %v", got, mobilePlatform)
	}
	if got := gg.app.PlatformKeyboardAvailable(); got != mobilePlatform {
		t.Errorf("the App reports a platform keyboard: %v, want %v", got, mobilePlatform)
	}
	wantEdge := float32(0)
	if systemBack {
		wantEdge = androidBackEdge
	}
	if got := gg.app.BackGestureEdge(); got != wantEdge {
		t.Errorf("the App's back gesture edge is %v, want %v", got, wantEdge)
	}

	g, _ = NewGame(newRecorderApp, Config{Touchscreen: "off", NoNativeKeyboard: true})
	gg = started(t, g.(*lazyGame))
	if gg.input.ime != nil || gg.app.PlatformKeyboardAvailable() {
		t.Error("NoNativeKeyboard left the system keyboard in place")
	}
}
