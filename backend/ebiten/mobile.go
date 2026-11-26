package ebiten

import (
	"errors"
	"log/slog"

	eb "github.com/hajimehoshi/ebiten/v2"
	"github.com/worldiety/gift"
)

// errRunOnMobile is what [Run] returns on Android and iOS, in place of
// Ebitengine's "Run is not implemented" from deep inside RunGame.
var errRunOnMobile = errors.New("gift/backend/ebiten: Run does not work on Android or iOS; " +
	"hand the game of NewGame to github.com/hajimehoshi/ebiten/v2/mobile.SetGameWithOptions instead")

// errNilApp is what the game of [NewGame] stops with when its constructor
// returned no App.
var errNilApp = errors.New("gift/backend/ebiten: the constructor given to NewGame returned a nil App")

// NewGame returns an Ebitengine game that runs the App newApp creates, and the
// options it has to run with, for the platforms on which the program does not
// call RunGame itself: Android and iOS. There the operating system owns the
// process and the frame loop, and the game is handed to Ebitengine's mobile
// package from an init function of the package that ebitenmobile binds:
//
//	package mobile
//
//	import (
//		"github.com/hajimehoshi/ebiten/v2/mobile"
//		backend "github.com/worldiety/gift/backend/ebiten"
//		"github.com/worldiety/gift/geom"
//
//		"example.com/notes/app"
//	)
//
//	func init() {
//		g, opts := backend.NewGame(app.New, backend.Config{})
//		mobile.SetGameWithOptions(g, opts)
//	}
//
//	// SetSafeArea is called by the host: dp on Android, points on iOS; any thread.
//	func SetSafeArea(left, top, right, bottom float64) {
//		backend.SetSafeArea(geom.Insets{Top: float32(top), Right: float32(right),
//			Bottom: float32(bottom), Left: float32(left)})
//	}
//
// The application lives in a package of its own, whose New installs the font
// and the theme as a desktop's main would, because a bound library has no
// main and the first Text panics without a default font:
//
//	func New() *gift.App {
//		ui.SetDefaultFont(ui.MustFont(ui.FontQuery{Family: inter.Family}))
//		return gift.New(gift.Options{Root: func(*gift.Context) gift.View {
//			return ui.Window(screen(), ui.OnScreenKeyboard()).Align(geom.Bottom)
//		}})
//	}
//
// The desktop's main calls Run(app.New(), cfg) with the same New.
//
// A zero Config is the whole backend setup on mobile; the application still
// installs its font, as on a desktop. The mouse is not read, text fields get
// the system keyboard (see [Config.NoNativeKeyboard]), and the drawn keyboard
// placed in the window serves the date and time fields the system keyboard
// cannot. The host view calls the forwarder whenever the system bars change;
// see [SetSafeArea]. The project plan, section 52, records the design.
//
// # Why a constructor
//
// The goroutine that creates an App is its UI executor, and the frame loop
// has to run on it; see [Run]. On a desktop that holds by itself, because Run
// is called on the goroutine that created the App and RunGame calls back on
// that goroutine. On Android and iOS it cannot: init runs on one goroutine,
// and Ebitengine starts the frame loop on another one of its own. So the game
// calls newApp on the loop's goroutine, when Ebitengine first lays it out or
// updates it, and sets everything else up there too. Nothing that touches the
// App may run before that; the game is the only route to it.
//
// # What it is
//
// The game is the one Run drives — the same frame model, input bridge and
// renderer — so everything said there holds. What Run does to a window does
// not happen: Title, X11ClassName, X11InstanceName, Width and Height have no
// window to go to, and the size of the view, in Ebitengine's outside units
// (density independent pixels on Android, points on iOS), is what Ebitengine
// lays the game out in. [Config.Touchscreen] is ignored, because a phone has
// no /dev/input an application may read; its touches arrive through
// Ebitengine. [Config.NoMouse] is always set, because Ebitengine reads no
// mouse there.
//
// # What the host has to do
//
// On Android the activity keeps its EbitenView edge to edge and declares
//
//	android:configChanges="orientation|screenSize|smallestScreenSize|screenLayout|density|keyboard|keyboardHidden|navigation|uiMode"
//
// so that a rotation, a fold, a theme change or a hardware keyboard that comes
// or goes is a relayout and not a new activity with a second view over the
// same game. It sets windowSoftInputMode to adjustNothing for the system
// keyboard, and calls suspendGame and resumeGame from onPause and onResume.
// EbitenView consumes every key, so the activity overrides onKeyDown and
// onKeyUp to hand Back and the volume keys back to Android. Back moves the
// task to the background (moveTaskToBack) rather than finishing the activity,
// because Ebitengine cannot carry its GPU resources into a new view in the
// same process. How the host reports the insets is in [SetSafeArea]. On iOS
// the application creates exactly one subclass of the EbitenViewController
// that ebitenmobile generates (MobileEbitenViewController for a package named
// mobile) per process and reuses it when its scene reconnects, because
// Ebitengine attaches its Metal layer only once, and suspends and resumes it
// from the scene delegate.
//
// An error setting the game up — the renderer's shaders, or a nil App — is
// what the game's first Update returns, which ends the game the way any
// error from Update does.
//
// Nothing is ever taken down: the game lives as long as the process, which
// the operating system ends when it ends the application. That is why
// NewGame is for Android and iOS only. On a desktop use Run, which also
// writes the final metrics report, closes the touch panel and takes the drop
// layer down when the window closes.
func NewGame(newApp func() *gift.App, cfg Config) (eb.Game, *eb.RunGameOptions) {
	if newApp == nil {
		panic("gift/backend/ebiten: NewGame with a nil constructor")
	}
	l := &lazyGame{newApp: newApp, cfg: cfg}
	// The options are needed now, before there is a renderer. The working
	// space is the one the renderer will ask for; should its shaders fail,
	// it paints sRGB under a screen tagged as such, which only Apple's
	// platforms distinguish, and logs that it does.
	opts := runOptions(cfg, WorkingSpace(cfg.Logger))
	if cfg.DirectToScreen {
		// The rule of [session.game], judged before there is a game:
		// Ebitengine asks for the interface when it is handed the game, and
		// only a game that has the method gets the final screen. Neither
		// Android nor iOS has a display pass (newDisplayPass returns nil on
		// both), so the setting is the only reason for one there.
		return lazyDirectGame{l}, opts
	}
	return l, opts
}

// lazyGame is the game of [NewGame]: [game], created on the first call
// Ebitengine makes into it.
type lazyGame struct {
	newApp func() *gift.App
	cfg    Config

	s   *session
	err error
}

// game returns the game, creating the App and the session on first use; see
// NewGame for why that is the first use and not earlier.
func (l *lazyGame) game() (*game, error) {
	if l.s == nil && l.err == nil {
		if app := l.newApp(); app == nil {
			l.err = errNilApp
		} else {
			l.s, l.err = newSession(app, l.cfg)
		}
		if l.err == nil && l.cfg.Logger != nil {
			l.cfg.Logger.Info("gift/backend/ebiten: game created",
				slog.Int("tps", l.s.g.busyTPS), slog.Int("idle_tps", l.cfg.IdleTPS),
				slog.Bool("mobile", mobilePlatform))
		}
	}
	if l.err != nil {
		return nil, l.err
	}
	return l.s.g, nil
}

// Update is [game.Update], after creating the game if it was not yet.
func (l *lazyGame) Update() error {
	g, err := l.game()
	if err != nil {
		return err
	}
	return g.Update()
}

// Draw is [game.Draw]. Ebitengine updates before it draws, so a game that
// does not exist here failed to be created, and Update has already said so.
func (l *lazyGame) Draw(screen *eb.Image) {
	if l.s != nil {
		l.s.g.Draw(screen)
	}
}

// Layout answers what LayoutF does, as [game.Layout] does.
func (l *lazyGame) Layout(outsideWidth, outsideHeight int) (int, int) {
	w, h := l.LayoutF(float64(outsideWidth), float64(outsideHeight))
	return int(w), int(h)
}

// LayoutF is [game.LayoutF]. Ebitengine lays the game out before it first
// updates it, so this is usually where the game is created. A game that could
// not be created answers the outside size; Update reports why.
func (l *lazyGame) LayoutF(outsideWidth, outsideHeight float64) (float64, float64) {
	g, err := l.game()
	if err != nil {
		return outsideWidth, outsideHeight
	}
	return g.LayoutF(outsideWidth, outsideHeight)
}

// lazyDirectGame is [lazyGame] for [Config.DirectToScreen], as [directGame]
// is [game] for it.
type lazyDirectGame struct{ *lazyGame }

// DrawFinalScreen is [directGame.DrawFinalScreen].
func (d lazyDirectGame) DrawFinalScreen(screen eb.FinalScreen, offscreen *eb.Image, geoM eb.GeoM) {
	if d.s != nil {
		directGame{d.s.g}.DrawFinalScreen(screen, offscreen, geoM)
	}
}
