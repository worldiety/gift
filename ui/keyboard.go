package ui

import (
	"sync/atomic"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/text"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui/motion"
)

var keyboardType = gift.RegisterType("ui.OnScreenKeyboard")

// The on-screen keyboard of the project plan, section 19.
//
// # Why gift draws one at all
//
// The kiosk of section 1 — a Raspberry Pi behind a touchscreen, X11, no
// desktop — has no operating system keyboard that could be asked, and
// Ebitengine has no way to ask for one there. A touch application on that
// machine has no way to enter text unless the framework draws the keys itself.
// So it does.
//
// # And why it steps aside for the platform's
//
// Android and iOS do have a keyboard, a better one than this — the user's own
// layout, their language, prediction, dictation — and a backend that can raise
// it does, through [gift.TextInputClient] and [gift.App.ReplaceText]; see
// textinput.go in package gift. Both keyboards answer the same request,
// [gift.BuildContext.SoftKeyboardRequested], so the split is a matter of who
// answers it: while a backend reports that the system keyboard is up, with
// [gift.App.SetPlatformKeyboardActive], this view stays down and declares no
// obstruction, and the platform keeps the field visible. Where there is no
// system keyboard, or where a backend declines to hand a field to one, nothing
// reports it, and this view serves the request as it always has.
//
// A backend declines a field that is not a [gift.TextInputClient]: DateField
// and TimeField are not, so on a phone they keep this keyboard, and a mobile
// application places [OnScreenKeyboard] in its [Window] for them. TextField,
// SecureField, SearchField, TextArea and CodeEditor get the system keyboard.
// On a platform that reports a keyboard of its own with
// [gift.App.SetPlatformKeyboardAvailable] this view serves such a field
// without the kiosk switch, because the platform's report is the statement
// about the machine the switch would otherwise have to make.
//
// # Why it is not raised by the pointer kind
//
// This is the one constraint section 19 states in bold and it is worth
// repeating where the code is. Ebitengine's documentation for AppendTouchIDs
// says the function does nothing on desktops; backend/ebiten/input.go records
// the same thing. On Raspberry Pi OS under X11 a touchscreen enumerates as a
// mouse, so every event gift sees from a finger arrives as [gift.PointerMouse]
// with [gift.PointerTouch] never occurring. A keyboard gated on the pointer
// kind would therefore be dead on exactly the hardware it exists for, and
// would pop up on the developer's laptop where it is not wanted — both
// failures at once, from the same line. The gate is [SetOnScreenKeyboard], an
// explicit statement about the machine.

// onScreenKeyboard is the kiosk switch. It is one bit for the whole process,
// and it says that the machine has no hardware keyboard and no keyboard of its
// own: text fields ask for one, and gift draws it. A platform with a system
// keyboard says the same through [gift.App.SetPlatformKeyboardAvailable],
// which a backend sets, and needs no switch; see [softKeyboardWanted].
//
// # Why process wide, and why with the App
//
// "Is this program running on a touch kiosk with no hardware keyboard" is a
// property of the deployment, not of a field, not of a window and not of a
// call site. It is decided once, in main, by whoever built the image — the
// same place and the same kind of decision as [SetDefaultFont] and
// [SetClipboard], and for the same reason the plan gives for the theme: a
// per-field switch would have to be repeated on every field of every form,
// and the failure mode of forgetting it once is a field that cannot be typed
// into at all.
//
// It takes the [gift.App] for the reason [SetTheme] takes it: the views decide
// in Build whether there is a keyboard, so nothing changes on the screen until
// something rebuilds, and a caller who had to remember [gift.App.Invalidate]
// separately would have a switch that works only after the next unrelated
// interaction. A nil App is the ordinary case in main, before an App exists.
//
// It is an atomic rather than a plain bool because [SetTheme] set that
// precedent for a value that may legitimately be flipped while the program
// runs — a kiosk whose hardware keyboard was unplugged — and because the read
// is once per build of one view rather than anywhere in the frame path. It is
// deliberately less than the theme offers in one way: flipping it does not
// interpolate anything and does not preserve what somebody was typing.
var onScreenKeyboard atomic.Bool

// SetOnScreenKeyboard turns gift's own on-screen keyboard on or off for the
// whole process and asks app to rebuild, which is what makes the change
// visible. The default is off.
//
// Off is the default because a keyboard that appears on a developer's desktop
// whenever a field takes the focus is worse than no keyboard at all, and
// because gift cannot detect the kiosk: see [onScreenKeyboard] for why the
// pointer kind is not the evidence it looks like.
//
// With the switch on, a [TextFieldView] asks for the keyboard when it gains
// the focus and dismisses it when it loses it, and a [SecureField] shows the
// last character typed into it for a moment, as touch keyboards do. A backend
// that reports a system keyboard with [gift.App.SetPlatformKeyboardAvailable]
// has the same effect without the switch — Android and iOS with the
// Ebitengine backend — so an application that runs there and on a desktop
// leaves the switch off and gets the system keyboard on the phone and none on
// the desktop. The keyboard itself is a view the application places, once, at
// the top of its window:
//
//	ui.Window(
//	    form,
//	    ui.OnScreenKeyboard(),
//	).Align(geom.Bottom)
//
// The form is then laid out above the keyboard while it is showing; see
// [Overlay.AvoidKeyboard].
//
// An application that never places [OnScreenKeyboard] pays nothing for having
// the switch on beyond one bool per focus change.
//
// app may be nil, which sets the switch and repaints nothing; that is the call
// in main, next to [SetDefaultFont]. Calling it with a non nil app from a
// goroutine other than the UI one is a defect for the reason [SetTheme] gives:
// [gift.App.Invalidate] is not concurrency safe. Post it instead.
func SetOnScreenKeyboard(app *gift.App, on bool) {
	onScreenKeyboard.Store(on)
	if app != nil {
		app.Invalidate()
	}
}

// OnScreenKeyboardEnabled reports whether the switch of [SetOnScreenKeyboard]
// is on. It does not see a platform keyboard; see
// [gift.App.PlatformKeyboardAvailable].
func OnScreenKeyboardEnabled() bool { return onScreenKeyboard.Load() }

// softKeyboardWanted reports whether a text field that gains the focus asks
// for a keyboard: where the kiosk switch says gift draws one, or where the
// backend reported one of the platform's. Both are one load, so a desktop
// application that is neither pays that per focus change and no rebuild.
//
// It is asked only when a field gains the focus. A field that loses it takes
// the request back without asking, so that a request made before the backend
// withdrew its keyboard does not outlive the field; that costs one compare,
// because [gift.EventContext.RequestSoftKeyboard] does nothing when the
// request does not change. [keyboardOffered] is the same rule in a build.
func softKeyboardWanted(ctx *gift.EventContext) bool {
	return onScreenKeyboard.Load() || ctx.PlatformKeyboardAvailable()
}

// keyboardOffered is [softKeyboardWanted] read from inside a build: whether
// text typed here is typed on a screen, gift's drawn keyboard or a phone's.
func keyboardOffered(bc *gift.BuildContext) bool {
	return onScreenKeyboard.Load() || bc.PlatformKeyboardAvailable()
}

// OnScreenKeyboardHeight is how tall [OnScreenKeyboard] makes itself, in
// logical pixels, with its default padding and no [KeyboardView.Frame]. It is
// a constant of the metrics and does not depend on the window.
//
// It does not see the modifiers of any particular keyboard: one given its own
// [KeyboardView.Padding], [KeyboardView.PaddingInsets] or a fixed height with
// [KeyboardView.Frame] is a different height, and a form that reserves this
// number under it is off by the difference. The overlay's own avoidance has
// no such gap, because it uses the height the keyboard actually measured.
//
// A form inside a [Window] or [ZStack] that holds the keyboard does not need
// it: the overlay takes this much height away from the content by itself; see
// [Overlay.AvoidKeyboard]. It is exported for the compositions where that does
// not happen — avoidance turned off, or a keyboard wrapped in another
// container — where a form has to leave room under its last field itself; see
// [KeyboardView] for why a reveal alone cannot lift that field.
func OnScreenKeyboardHeight() float32 {
	return 2*kbPadding + kbRows*kbKeyHeight + (kbRows-1)*kbGap
}

// Metrics of the keyboard, in logical pixels. They are constants and not
// modifiers for the reason [CaretBlinkInterval] is one: a key has to be big
// enough for a fingertip, and how big that is, is a property of fingers.
//
// Forty four is the touch target both Apple and Google document, at forty four
// points and forty eight density independent pixels respectively; the value is
// in logical pixels here and the density transform of section 18 scales it
// with everything else.
const (
	kbKeyHeight  = float32(44)
	kbGap        = float32(6)
	kbPadding    = float32(8)
	kbKeyRadius  = float32(6)
	kbFontSize   = float32(18)
	kbRows       = 5
	kbKeyCount   = 48
	kbLabelInset = float32(2)

	// kbDefaultWidth is how wide the keyboard makes itself when nothing
	// bounds it. A keyboard in an unbounded row is a mistake rather than a
	// layout, but a mistake that produced an infinite rectangle would fail
	// far away from its cause; see [defaultFieldWidth] for the same argument.
	kbDefaultWidth = float32(640)
)

// kbCmd is what pressing a key does beyond producing a character.
type kbCmd uint8

const (
	kbChar kbCmd = iota
	kbShift
	kbPage
	kbKeyCode
	// kbDismiss puts the keyboard away; see [keyboardNode.press] and
	// [gift.EventContext.DismissSoftKeyboard].
	kbDismiss
)

// kbKey is one drawn key.
//
// It carries both a rune and a [gift.Key] because the two channels of gift's
// input are genuinely different questions and a key is on one side or the
// other; see [gift.Key]. A letter is a character and has no position gift
// names. Backspace is a position and has no character.
type kbKey struct {
	label string
	r     rune
	key   gift.Key
	cmd   kbCmd
	w     float32
}

// The layouts. German, because section 19 names umlauts and ß as the
// requirement and those are where they are on a German keyboard: ü after p, ö
// and ä after l, ß on the bottom row, and z and y swapped against QWERTY.
//
// # Three tables and not one with a transformation
//
// The obvious alternative is one table plus strings.ToUpper at paint time, and
// it is rejected because it allocates: a string that is upper cased every
// frame is one allocation per key per frame, which is fifty in the warmed
// frame path of a screen that is doing nothing. These are built once, at
// package initialisation, and the frame path only indexes them.
//
// # The rows have the same shape on both pages
//
// Row for row the key counts and the widths are identical between the letter
// page and the symbol page. That is not a coincidence, it is a requirement:
// the geometry is then a function of the bounds alone, so switching the page
// or pressing shift is a repaint and never a relayout, and [keyboardNode] can
// compute its rectangles once per layout instead of once per press.
var (
	kbLower   [kbRows][]kbKey
	kbUpper   [kbRows][]kbKey
	kbSymbols [kbRows][]kbKey
)

func init() {
	// The bottom row is the same on both pages except for the label of the
	// page switch, so it is built twice from one description.
	bottom := func(page string) []kbKey {
		return []kbKey{
			{label: page, cmd: kbPage, w: 1.5},
			{label: ",", r: ',', w: 1},
			{label: "space", r: ' ', key: gift.KeySpace, cmd: kbKeyCode, w: 3.5},
			{label: ".", r: '.', w: 1},
			{label: "\u21b5", key: gift.KeyEnter, cmd: kbKeyCode, w: 1.5},
			// The dismiss key; see [kbDismiss]. It is the last key of the
			// bottom row, where a touch keyboard's "hide" affordance sits on
			// both mobile platforms, and it is the reason a kiosk user with
			// no physical keyboard can ever put this thing away: escape
			// needs a keyboard, a tap outside needs somewhere to tap that is
			// not covered, and the return key of a single line field is not
			// a dismissal on any platform.
			//
			// U+2304 DOWN ARROWHEAD is the glyph, which is the chevron
			// Android draws for the same key. It is present in the bundled
			// Inter; a caller who installs a font without it gets whatever
			// that font draws for a missing glyph, which is the same deal
			// the shift, backspace and return keys above have always had.
			{label: "\u2304", cmd: kbDismiss, w: 1.5},
		}
	}
	shift := kbKey{label: "\u21e7", cmd: kbShift, w: 1}
	back := kbKey{label: "\u232b", key: gift.KeyBackspace, cmd: kbKeyCode, w: 1}

	kbLower = [kbRows][]kbKey{
		kbChars("1234567890"),
		kbChars("qwertzuiopü"),
		kbChars("asdfghjklöä"),
		append(append([]kbKey{shift}, kbChars("yxcvbnmß")...), back),
		bottom("?123"),
	}
	kbUpper = [kbRows][]kbKey{
		kbChars("1234567890"),
		kbChars("QWERTZUIOPÜ"),
		kbChars("ASDFGHJKLÖÄ"),
		// ß has no upper case here on purpose. The capital sharp s exists as
		// U+1E9E, but it is a recent addition that many fonts do not carry
		// and that German orthography treats as optional even where it does;
		// a key that drew a missing glyph box would be worse than one that
		// keeps producing the letter it is labelled with.
		append(append([]kbKey{shift}, kbChars("YXCVBNMß")...), back),
		bottom("?123"),
	}
	kbSymbols = [kbRows][]kbKey{
		kbChars("!\"§$%&/()="),
		kbChars("@€#~*+-_|\\^"),
		kbChars("'`´;:<>[]{}"),
		append(kbChars("§?°±«»¿¡…"), back),
		bottom("ABC"),
	}
}

// kbChars turns a string into one unit wide character keys.
func kbChars(s string) []kbKey {
	out := make([]kbKey, 0, len(s))
	for _, r := range s {
		out = append(out, kbKey{label: string(r), r: r, w: 1})
	}
	return out
}

// keyboardState is the presentation state of the keyboard: which page is
// showing, whether shift is armed, and which key is under the finger.
//
// # Why it is a package variable
//
// Because there is one screen, one focus and therefore one keyboard in a
// process, and because this state has to survive a rebuild while being
// reachable from a view value the application constructs fresh in every build.
// The two mechanisms gift offers for surviving state are a component scope,
// which [OnScreenKeyboard] cannot use without losing its modifiers to the
// [gift.View] boundary of section 4, and a caller owned object like
// [TextEditor], which would make every application declare and thread a
// keyboard handle for something it never reads.
//
// The cost is stated rather than hidden: two [gift.App] instances in one
// process share this, so shift pressed in one would show in the other. The
// same is already true of [SetTheme], [SetDefaultFont] and [SetClipboard], and
// the plan's own note on t.Parallel and process wide state in section 13 is
// about exactly this family of values. It is reset whenever the keyboard is
// built hidden, so a test that finishes with the keyboard down leaves nothing
// behind.
var kbState keyboardState

type keyboardState struct {
	shift   bool
	symbols bool
	// pressed is the index of the key under the finger, or -1.
	pressed int
	// down is the key code injected on the press and not yet released. It is
	// remembered because the release has to name the same key, and because
	// gift's key repeat runs until it arrives; see
	// [gift.EventContext.KeyDown].
	down gift.Key
}

func (s *keyboardState) rows() *[kbRows][]kbKey {
	switch {
	case s.symbols:
		return &kbSymbols
	case s.shift:
		return &kbUpper
	}
	return &kbLower
}

// reset puts the keyboard back into its resting state. It runs when the
// keyboard is built hidden, so that the next field to take the focus gets a
// lower case letter page rather than whatever the previous one left armed.
func (s *keyboardState) reset() {
	s.shift, s.symbols, s.pressed, s.down = false, false, -1, gift.KeyOther
}

func init() { kbState.reset() }

// kbSlideSpring moves the keyboard in and out: critically damped, so that it
// arrives without bouncing, in about the quarter second of iOS.
var kbSlideSpring = motion.Spring{Response: 280 * time.Millisecond, Damping: 1}

// keyboardSlide is where the keyboard is on its way in or out. It is process
// wide for the reason [kbState] is, and it belongs to the application whose
// build context owner is, so that a test that ends with the keyboard up does
// not leave one sliding away in the next.
//
// The slide is drawn and not laid out: the content makes room for the
// keyboard, and takes it back, at once, and the keyboard moves over it. A size
// that changed with the clock would lay the whole window out on every frame of
// the slide, and a layout must not depend on the clock; see
// [gift.LayoutContext.Now].
type keyboardSlide struct {
	owner *gift.BuildContext
	// shown is true from the build that shows the keyboard to the end of the
	// slide that takes it away, and leaving says that slide is under way.
	shown, leaving bool
	// from is the fraction the slide set off from and at the clock value it
	// did, which the first paint of the slide stamps; frac is the fraction of
	// the last paint. 0 is in place, 1 out of sight below it.
	from, frac float64
	at         time.Duration
	stamped    bool
	// bleed is how far the panel reached past the keyboard behind the
	// system bars on the last frame it was in place, and bleedSafe the safe
	// area of the window it was measured in; see [keyboardNode.panelBleed].
	bleed, bleedSafe geom.Insets
}

var kbSlide = keyboardSlide{frac: 1}

// toward sets the slide off towards out of sight or towards its place, from
// wherever the last frame showed it.
func (s *keyboardSlide) toward(leaving bool) {
	s.leaving, s.from, s.stamped = leaving, s.frac, false
}

// step returns the fraction of the slide at now and whether it is still under
// way, and ends a slide out.
func (s *keyboardSlide) step(now time.Duration) (frac float64, moving bool) {
	if !s.stamped {
		s.at, s.stamped = now, true
	}
	to := 0.0
	if s.leaving {
		to = 1
	}
	t := now - s.at
	if kbSlideSpring.Settled(t, s.from, to, 0, 0.002) {
		s.frac = to
		if s.leaving {
			s.shown, s.leaving = false, false
		}
		return to, false
	}
	s.frac, _ = kbSlideSpring.At(t, s.from, to, 0)
	return s.frac, true
}

// KeyboardView is gift's own on-screen keyboard. It is created by
// [OnScreenKeyboard]; the zero value is not useful.
//
// # What it is
//
// A German layout with umlauts and ß, a digit row, a symbol page, shift, space,
// backspace, enter and a key that puts the keyboard away. It is one node:
// forty eight keys are forty eight rectangles this view draws and hit tests
// itself, not forty eight child views. That is a deliberate departure from how
// the rest of this package composes, and the reason is in the numbers — see
// [keyboardNode] — but the short version is that a keyboard is a grid of
// identical cells with no independent state, which is the one shape where a
// subtree per cell buys nothing and costs a build, a layout and a paint per
// cell.
//
// It draws nothing, occupies nothing and allocates nothing unless a keyboard
// can be asked for (the kiosk switch, or a platform keyboard reported with
// [gift.App.SetPlatformKeyboardAvailable]) *and* a field has asked for it
// *and* no platform keyboard serves that request; see [SetOnScreenKeyboard],
// [gift.EventContext.RequestSoftKeyboard] and
// [gift.App.SetPlatformKeyboardActive].
//
// # What it deliberately is not
//
// It is not an IME. Section 14 excludes CJK composition, a preedit string and
// a candidate window, and a German layout is none of those: every key here
// produces one finished character, exactly as a physical German keyboard does.
//
// There are no long press accents. [gift.EventLongPress] exists and would be
// the hook, but an accent popup is a second floating surface with its own
// geometry, its own hit testing and its own dismissal rules, and the German
// layout it would serve already has ä, ö, ü and ß on keys of their own. It is
// the feature to build when a layout needs it.
//
// There is no arrow key, no tab and no modifier other than shift. A caret is
// placed by touching the text, which [TextFieldView] already supports, and a
// kiosk form is walked with the fingers rather than with tab.
//
// # Keeping the field clear of it
//
// The overlay does most of it. A [ZStack] — and so a [Window] — that has this
// view as a direct child lays the other children out in the part of the box
// the keyboard leaves free while it is showing; see [Overlay.AvoidKeyboard].
// The viewport of a scrolling form really is shorter then, so the ordinary
// reveal of [gift.App.ScrollIntoView] is exact, a field in the last row of a
// form can be lifted like any other, and a button pinned to the bottom of the
// screen sits on top of the keys instead of under them. It is the composition
// of a column,
//
//	ui.VStack(ui.VScroll(form).Flex(1), ui.OnScreenKeyboard())
//
// without having to restructure the application around the keyboard, and the
// column still works unchanged: the keyboard takes the height it needs and
// there is nothing for it to cover.
//
// Section 19 names [gift.App.ScrollIntoView] as the mechanism, and it is still
// the one that moves the field. The field asks to be revealed from its own
// focus handler, before the keyboard was built; gift re-runs the reveal on the
// frame after the keyboard appeared, when the layout has already shrunk the
// content, which is the order that makes the second reveal see the final
// viewport.
//
// # The system bars
//
// In a [Window] that fills a phone's screen the keyboard is laid out like the
// rest of the content, inside the safe area of [gift.App.SetSafeArea]: its
// lowest row of keys sits above the navigation bar, where a finger can reach
// it without starting the home gesture. Its panel does not stop there. The
// background is painted on down to the edge of the window behind the
// navigation bar, and, on a phone turned on its side, out to the side edges
// behind the cutout and the side bar, so the keyboard reads as one surface
// that rises from the bottom of the device and not as a slab floating above
// a strip of page. [TabBar] and [NavigationStack] paint their bars the same
// way, and a bar of an application's own asks for it with
// [Stack.BleedSafeArea].
//
// It is paint and nothing else. The keyboard's size, the rectangles of its
// keys, the area a tap lands in and the rectangle it declares as its
// obstruction are what layout made them; a tap on the part of the panel that
// lies behind the navigation bar belongs to the system, and a reveal keeps a
// field clear of the keys and not of the bar under them, which the content
// does not reach in the first place. The border keeps the bounds, as it does
// on every bleeding view. A keyboard given a [KeyboardView.CornerRadius] does
// not bleed, because no rounded background does; see [Stack.BleedSafeArea].
//
// The panel only bleeds where the keyboard lies on the edge of the safe area.
// A keyboard in the middle of a screen, inside a container that does not reach
// the bottom, or in a window without a safe area — a desktop, a kiosk without
// a configured bezel — paints its bounds as it always has.
//
// # The obstruction, for the compositions that overlap
//
// This view also declares [gift.Element.Obstructs], so a reveal aims at the
// part of a scroll container the keyboard is not sitting on. With avoidance on
// that never changes anything — the viewport ends where the keyboard begins —
// and it is there for the two compositions in which the keyboard does cover
// the content: [Overlay.AvoidKeyboard] turned off, and a keyboard wrapped in
// another container, which the overlay cannot recognise.
//
// Those keep the limit the obstruction has always had, and it is arithmetic:
// **a container can only lift the field by as much as it can still scroll**,
// and it can only scroll while there is content left below. A field that is
// the last row of a form stays at the bottom of the viewport with the keyboard
// over it, unless the form leaves room under it:
//
//	ui.VScroll(form.PaddingInsets(geom.Insets{Bottom: ui.OnScreenKeyboardHeight()}))
type KeyboardView struct {
	base
	font Font
	size float32

	fg, keyFace, keyPressed, keyAccent    Color
	hasFG, hasFace, hasPressed, hasAccent bool
}

// OnScreenKeyboard returns the keyboard view. Place it once, on top of
// everything else, as a direct child of the [Window] and align it to the
// bottom:
//
//	ui.Window(form, ui.OnScreenKeyboard()).Align(geom.Bottom)
//
// The alignment is the [Overlay]'s and applies to every child. A form that
// fills the window cannot be moved by it; one that does not is aligned to the
// bottom of the space above the keyboard while the keyboard is showing, which
// is where it has to be to stay in view. See [Overlay.AvoidKeyboard] for how
// the content makes room.
func OnScreenKeyboard() KeyboardView { return KeyboardView{} }

// ViewType implements gift.View.
func (v KeyboardView) ViewType() gift.TypeID { return keyboardType }

// Build implements gift.View.
//
// It has two shapes. When the keyboard is not showing it returns a node with
// no painter, no interactor and a layouter that reports a zero size — one
// scene node that costs a zero returning call per layout pass and nothing
// else. When it is showing it allocates the one [keyboardNode] and resolves
// every colour, which is the rule of this package and the reason the frame
// path never consults a theme.
//
// The conditions are read in the cheap order on purpose: the kiosk switch is
// one atomic load and is false in every application that is not a kiosk, so an
// application that never turns it on never reaches the other questions, unless
// a backend reported a platform keyboard, which serves what it can and leaves
// this view the fields it cannot serve; see the type's documentation. The
// last one is whether a platform keyboard already serves the request; see
// [gift.App.SetPlatformKeyboardActive]. A system keyboard that comes up while
// this one is showing sends it away the way a blur does — sliding, with the
// room given back at once — so that two keyboards are never on the screen
// together.
func (v KeyboardView) Build(bc *gift.BuildContext) gift.Element {
	if kbSlide.owner != bc {
		kbSlide = keyboardSlide{owner: bc, frac: 1}
	}
	leaving := false
	asked := keyboardOffered(bc)
	if !asked || !bc.SoftKeyboardRequested() || bc.PlatformKeyboardActive() {
		if !kbSlide.shown {
			kbState.reset()
			kbLive = nil
			return gift.Element{Key: v.key, Flex: v.flex, Layouter: kbHidden{}}
		}
		// Up until now: it slides away, over the content that has its room
		// back already, and takes no more keys.
		if !kbSlide.leaving {
			kbSlide.toward(true)
			kbState.reset()
		}
		leaving = true
	} else if !kbSlide.shown || kbSlide.leaving {
		kbSlide.shown = true
		kbSlide.toward(false)
	}

	size := v.size
	if size <= 0 {
		size = kbFontSize
	}
	st := v.style.resolved()
	if !v.style.isSet(bitBackground) {
		st.background = ResolveColor(ColorSurface)
	}
	if !v.style.isSet(bitBorder) {
		st.border = resolveBorder(Border{Width: 1, Color: ColorSeparator})
	}
	// The panel reaches the edge of the window behind the navigation bar of
	// a phone, and behind the cutout and the side bar of one on its side;
	// see "The system bars" on [KeyboardView]. It is set on the resolved
	// style and not on the view, because nothing about it is the caller's to
	// change: a keyboard that stopped short of the edge it is drawn against
	// is a defect, not a look.
	st.bleed = kbBleed

	n := &keyboardNode{
		fr:   v.frame,
		st:   st,
		pad:  geom.InsetsAll(kbPadding),
		st8:  &kbState,
		req:  text.Request{Font: resolveFont(v.font), Size: size, MaxWidth: geom.Unbounded()},
		face: ResolveColor(ColorControl),
		safe: bc.SafeArea(),
	}
	if v.hasPadding() {
		n.pad = v.pad
	}
	n.fg = ResolveColor(ColorLabel)
	if v.hasFG {
		n.fg = ResolveColor(v.fg)
	}
	if v.hasFace {
		n.face = ResolveColor(v.keyFace)
	}
	n.pressed = ResolveColor(ColorControlPressed)
	if v.hasPressed {
		n.pressed = ResolveColor(v.keyPressed)
	}
	n.accent = ResolveColor(ColorAccent)
	if v.hasAccent {
		n.accent = ResolveColor(v.keyAccent)
	}
	n.onAccent = ResolveColor(ColorOnAccent)
	n.keyBorder = resolveBorder(Border{Width: 1, Color: ColorSeparator})

	if leaving {
		n.leaving = true
		return gift.Element{Key: v.key, Flex: v.flex, Layouter: n, Painter: n}
	}
	return gift.Element{
		Key:      v.key,
		Flex:     v.flex,
		Layouter: n,
		Painter:  n,
		Label:    "on-screen keyboard",
		// An interactor, and deliberately not a focusable one. A key that
		// took the focus would take it away from the field it is typing into,
		// which is the classic defect of every on-screen keyboard: the first
		// tap on a letter blurs the field, the field's caret goes away, and
		// the character arrives nowhere. gift only moves the focus on a press
		// when the node asks for it or when the press hit nothing at all, so
		// declining both is the whole defence. See
		// [TestTappingAKeyDoesNotStealTheFocusFromTheField].
		Interactor: n,
		Focusable:  false,
		// The other half of declining the focus: gift blurs on a press that
		// lands on a node that cannot be focused — see [gift.App.PointerDown]
		// — which without this would make the first tap on a letter key blur
		// the field, withdraw this keyboard and deliver the character to
		// nobody. See [gift.Element.PreservesFocus].
		PreservesFocus: true,
		// The reason this exists at all; see [gift.Element.Obstructs].
		Obstructs: true,
		Clip:      true,
	}
}

// hasPadding reports whether the caller set a padding. The style bits do not
// cover padding, so this reads the value the way the rest of the package does.
func (v KeyboardView) hasPadding() bool { return !v.pad.IsZero() }

// kbHidden is the layouter of a keyboard that is not showing: zero size, no
// children, no allocation. It is a value type, so the element above boxes a
// zero sized struct into the interface and the compiler uses a shared address
// for it rather than allocating.
type kbHidden struct{}

// Layout implements gift.Layouter.
//
// It clears [kbLive], which is the other half of setting it in
// [keyboardNode.Layout]. Without this, [KeyRectForTest] kept answering with the
// geometry of the last keyboard that was on screen after the keyboard had gone
// away: a user's kiosk test would tap a believable point, hit the form behind
// it, and get no input and no diagnosis. A nil store per frame is not an
// allocation and the keyboard is not on the frame path when it is hidden
// anyway; the alternative, clearing it in [KeyboardView.Build], would miss the
// case where nothing rebuilds after the keyboard goes down.
func (kbHidden) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	kbLive = nil
	ctx.ReportOverflow(geom.Size{})
	return c.Constrain(geom.Size{})
}

// keyboardNode is the retained half of a [KeyboardView]: layouter, painter and
// interactor in one object, allocated once per build like every other node in
// this package.
//
// # Why the keys are not child views
//
// Because forty eight of them are. The composed spelling — a VStack of HStacks
// of Buttons — is five rows times up to eleven buttons, each of which is a
// scene node with a node payload, a layouter, a painter, an interactor, an
// element and a label view under it; call it a hundred and forty scene nodes
// and a hundred and forty heap objects per build, five nested layout passes
// over them, and a paint that walks the same tree. Against that, this object
// is one allocation, one layout that fills a fixed array of forty eight
// rectangles, and a paint that emits its operations in one loop.
//
// It is worth being clear about what is given up. A key cannot be styled
// individually, cannot be a [gift.Component], and does not appear in
// gifttest's tree — a test aims at a key by asking this node where it is, not
// by selecting it. That is an acceptable trade for a widget whose cells are
// interchangeable by construction and a bad one for anything else, which is
// why the rest of this package composes and this does not.
type keyboardNode struct {
	fr  frameSpec
	st  styleSpec
	pad geom.Insets

	// st8 is the shared [keyboardState]. It is a pointer rather than the
	// value so that this node, which is replaced on every rebuild, does not
	// carry a stale copy of which page is showing.
	st8 *keyboardState

	req text.Request

	fg, face, pressed, accent, onAccent Color
	keyBorder                           Border

	// rects is the geometry of the last layout, in local space. It is a fixed
	// size array and not a slice because the key count is fixed by
	// construction — see the note on row shapes above [kbLower] — so the
	// layout writes into the node and allocates nothing.
	rects [kbKeyCount]geom.Rect

	// h is the height of the keyboard, and leaving says that it is sliding
	// away: it then lays out at no height and draws itself above its bounds;
	// see [keyboardSlide].
	h       float32
	leaving bool

	// safe is the safe area of the window this node was built for, which
	// tells the way out whether the bleed remembered in [kbSlide] is still
	// the window's; see [keyboardNode.panelBleed].
	safe geom.Insets
}

// kbLive is the keyboard node of the most recent layout pass, or nil.
//
// It exists for [KeyRectForTest] and for nothing else: a key is not a node, so
// a test has no other way to find out where one is. It is a package variable
// for the reason [kbState] is one — there is one keyboard in a process — and
// it is deliberately not read by anything in the frame path.
var kbLive *keyboardNode

// --- layout -----------------------------------------------------------------

// Layout makes the keyboard as wide as it is allowed to be and exactly as tall
// as five rows of keys.
//
// The height is a constant of the metrics and not a share of the window: a key
// has to be a fingertip tall, and a keyboard that grew with the screen would
// make the keys enormous on the 1080p kiosk of section 1 and the form above it
// unusable. The width is greedy on a bounded axis like [BoxView], which is
// what puts the keys edge to edge across the window.
//
// The rectangles are computed here and nowhere else, which is what makes a
// press, a shift and a page switch pure repaints: none of them changes the
// geometry, by construction of the tables.
func (n *keyboardNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	cc := n.fr.apply(c)
	w := cc.Max.W
	if !isFinite(w) {
		w = kbDefaultWidth
	}
	h := n.pad.Vertical() + kbRows*kbKeyHeight + (kbRows-1)*kbGap
	size := cc.Constrain(geom.Sz(w, h))
	n.h = size.H

	kbLive = n
	if n.leaving {
		// Its keys are not on the screen any more as far as anybody asks.
		kbLive = nil
	}
	inner := geom.Rc(n.pad.Left, n.pad.Top, size.W-n.pad.Right, size.H-n.pad.Bottom)
	y := inner.Min.Y
	i := 0
	rows := &kbLower
	for r := range kbRows {
		row := rows[r]
		var total float32
		for k := range row {
			total += row[k].w
		}
		avail := inner.Width() - float32(len(row)-1)*kbGap
		x := inner.Min.X
		for k := range row {
			kw := avail * row[k].w / total
			n.rects[i] = geom.Rc(x, y, x+kw, y+kbKeyHeight)
			x += kw + kbGap
			i++
		}
		y += kbKeyHeight + kbGap
	}
	// A keyboard has no children and asks for nothing it was not offered, so
	// it cannot overflow in the sense of section 7.
	ctx.ReportOverflow(geom.Size{})
	if n.leaving {
		return geom.Sz(size.W, 0)
	}
	return size
}

// --- paint -------------------------------------------------------------------

// Paint draws the panel and then the keys, in the order of section 8, moved
// down by the part of the slide in or out still to go.
func (n *keyboardNode) Paint(ctx *gift.PaintContext) {
	frac, moving := kbSlide.step(ctx.Now())
	if moving {
		ctx.Animate(100 * time.Millisecond)
	} else if n.leaving {
		return
	}
	b := ctx.Bounds()
	if n.leaving {
		// Laid out at no height on the bottom edge: its place is above.
		b.Min.Y -= n.h
	}
	b.Max.Y = b.Min.Y + n.h
	dy := float32(frac) * n.h
	// The panel's bleed is decided before the slide moves the picture, and
	// a panel that bleeds is painted outside the slide's transform, moved
	// by hand, which is the same picture; see [keyboardNode.panelBleed].
	in := n.panelBleed(ctx)
	if in != (geom.Insets{}) {
		n.paintPanel(ctx, b.Translate(geom.Pt(0, dy)), in)
	}
	if dy != 0 {
		ctx.PushTransform(geom.Translate(geom.Pt(0, dy)))
		defer ctx.PopTransform()
	}
	if in == (geom.Insets{}) {
		paintBackground(ctx, n.st, b)
	}
	n.paintKeys(ctx, b)
	paintBorder(ctx, n.st, b)
}

// kbBleed is the set of edges at which the panel of the keyboard reaches past
// the safe area: the bottom it sits on, and the sides it spans. Never the top,
// where its neighbour is the content.
const kbBleed = EdgesLeft | EdgesRight | EdgesBottom

// panelBleed returns how far, per edge, the panel reaches past the bounds of
// the keyboard behind the system bars on this frame, and zero when it does
// not.
//
// It is asked before the slide's transform is pushed, and it is not left to
// [paintBackground], for two reasons.
//
// The first is the slide. [gift.PaintContext.SafeAreaBleed] decides from
// where the node *appears*: its layout bounds through the transform that is
// active when it is asked. Under the translation of a slide the keyboard
// appears below the safe bottom by however far it still has to go, so asked
// there it would bleed on no frame of the slide but the last, and the strip
// behind the navigation bar would show the page under a keyboard rising over
// it and fill in one frame after the keyboard arrived. Asked before the
// translation, the node is where layout put it, on the safe bottom.
//
// The second is the way out. A keyboard sliding away is laid out at no height
// on the bottom edge — see [keyboardSlide] — and an empty rectangle has no
// place in device space for SafeAreaBleed to compare with the safe area, so
// it answers zero for it. The keyboard has to remember instead: every frame
// it is in place it keeps the answer in [kbSlide], with the safe area it was
// given for, and the way out reuses it as long as the window's safe area is
// still the one it was taken in. A rotation during the slide out — the one
// change that could make it wrong — rebuilds, and then the panel paints its
// bounds for the rest of a quarter second rather than a bleed that may be on
// the wrong side.
//
// Zero, which is every window without a safe area, sends Paint down exactly
// the path it always took: the same operations in the same transform.
func (n *keyboardNode) panelBleed(ctx *gift.PaintContext) geom.Insets {
	if n.st.bleed == 0 || n.st.radius > 0 {
		return geom.Insets{}
	}
	if n.leaving {
		if kbSlide.bleedSafe != n.safe {
			return geom.Insets{}
		}
		return kbSlide.bleed
	}
	in := n.st.bleed.of(ctx.SafeAreaBleed())
	kbSlide.bleed, kbSlide.bleedSafe = in, n.safe
	return in
}

// paintPanel is [paintBackground] for a panel that bleeds by in: the shadow
// on the bounds b, which outline the keyboard, and the fill and the glass on b
// grown by in, which reach the edge of the window. The rectangle is final,
// so the style's own bleed is not consulted again.
func (n *keyboardNode) paintPanel(ctx *gift.PaintContext, b geom.Rect, in geom.Insets) {
	st := n.st
	st.bleed = 0
	// Always delegated, never gated here: paintBackground asserts the shadow
	// resolved before its own visibility gate, and an unresolved shadow would
	// read as invisible to a gate in front of it.
	sh := st
	sh.background, sh.grad, sh.material = Color{}, false, render.Material{}
	paintBackground(ctx, sh, b)
	st.shadow = Shadow{}
	paintBackground(ctx, st, b.Outset(in))
}

// paintKeys draws the forty eight key faces and their labels.
//
// The four assertions are the first four statements, in front of every
// visibility gate below them, for the reason [assertResolved] gives and for
// the reason [textFieldNode.paintContent] spells out: an unresolved semantic
// colour is transparent, so each gate would silently drop its operation and
// the keyboard would lay out, hit test and repaint perfectly while drawing
// nothing at all. A fifth colour gated in here belongs in the hand written
// table of TestEveryVisibilityGateIsGuarded; TestTheGateInventoryIsComplete
// notices a new *function* and would not notice a fifth gate in this one.
func (n *keyboardNode) paintKeys(ctx *gift.PaintContext, b geom.Rect) {
	assertResolved(n.face, "the key face of an OnScreenKeyboard")
	assertResolved(n.pressed, "the pressed key face of an OnScreenKeyboard")
	assertResolved(n.accent, "the armed shift colour of an OnScreenKeyboard")
	assertResolved(n.fg, "the key label colour of an OnScreenKeyboard")
	assertResolvedBorder(n.keyBorder, "the key border colour of an OnScreenKeyboard")

	rows := n.st8.rows()
	i := 0
	for r := range kbRows {
		row := rows[r]
		for k := range row {
			key := &row[k]
			rc := n.rects[i].Translate(b.Min)
			fill, label := n.face, n.fg
			switch {
			case i == n.st8.pressed:
				fill = n.pressed
			case key.cmd == kbShift && n.st8.shift:
				fill, label = n.accent, n.onAccent
			}
			if !fill.IsTransparent() {
				ctx.Add(render.Op{
					Kind:         render.OpFillRoundRect,
					Bounds:       rc,
					Color:        fill,
					CornerRadius: kbKeyRadius,
				})
			}
			if n.keyBorder.IsVisible() {
				ctx.Add(render.Op{
					Kind:         render.OpStrokeRoundRect,
					Bounds:       rc,
					Color:        n.keyBorder.Color,
					CornerRadius: kbKeyRadius,
					StrokeWidth:  n.keyBorder.Width,
				})
			}
			if !label.IsTransparent() {
				n.paintLabel(ctx, rc, key.label, label)
			}
			i++
		}
	}
}

// paintLabel centres one label in its key.
//
// The shaping request is the node's own with the text swapped in, so every
// label after the first frame is a cache hit in internal/text and allocates
// nothing; see [textFieldNode.para] for the same argument at greater length.
func (n *keyboardNode) paintLabel(ctx *gift.PaintContext, rc geom.Rect, label string, fg Color) {
	req := n.req
	req.Text = label
	p := text.Default().Layout(req)
	if len(p.Lines) == 0 {
		return
	}
	ln := &p.Lines[0]
	x := roundf(rc.Min.X + (rc.Width()-p.Size.W)/2)
	baseline := roundf(rc.Min.Y + (rc.Height()-p.Size.H)/2 + ln.Baseline)

	first := ctx.GlyphsLen()
	for ri := range ln.Runs {
		run := &ln.Runs[ri]
		id := render.FontID(run.Font.ID())
		for gi := range run.Glyphs {
			g := &run.Glyphs[gi]
			ctx.AppendGlyph(render.Glyph{
				Font: id,
				ID:   render.GlyphID(g.ID),
				Size: run.Size,
				X:    x + g.X,
				Y:    baseline + g.Y,
			})
		}
	}
	if count := ctx.GlyphsLen() - first; count > 0 {
		ctx.Add(render.Op{
			Kind:       render.OpGlyphs,
			Bounds:     rc.Inset(geom.InsetsAll(-kbLabelInset)),
			Color:      fg,
			Glyphs:     first,
			GlyphCount: count,
		})
	}
}

// --- input -------------------------------------------------------------------

// HandleEvent implements gift.Interactor.
//
// # A virtual press is a real press
//
// Nothing here knows what a text field is. A character key calls
// [gift.EventContext.TypeRune] and a command key calls
// [gift.EventContext.KeyDown] and [gift.EventContext.KeyUp], which are the
// same three entry points backend/ebiten calls for a physical keyboard. The
// event is built by gift, delivered to whatever holds the focus and bubbled
// up its ancestors exactly like any other, so no widget can distinguish the
// two and none of them needs a second code path. That is the whole design and
// it is why the test for it asserts on the *field's value*: there is no signal
// to assert on.
//
// A command key sends its press on pointer down and its release on pointer up,
// which is faithful in a way that matters: gift's own key repeat is armed by
// the press and disarmed by the release, so a finger resting on the drawn
// backspace deletes at [gift.KeyRepeatInterval] with no timer in this file.
//
// # Every event is consumed
//
// Including the ones this returns no action for. A keyboard is an opaque
// surface: a press that fell through it would reach whatever the form has
// underneath, and the gap between two keys would be a hole into the
// application.
func (n *keyboardNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	switch e.Kind {
	case gift.EventPointerDown:
		// Deliberately no ctx.RequestFocus. See [KeyboardView.Build].
		i := n.keyAt(ctx, e.Pos)
		n.st8.pressed = i
		ctx.Repaint()
		if i >= 0 {
			n.press(ctx, n.keyOf(i))
		}
		return true

	case gift.EventPointerUp, gift.EventPointerCancel:
		n.st8.pressed = -1
		ctx.Repaint()
		if n.st8.down != gift.KeyOther {
			ctx.KeyUp(n.st8.down, 0)
			n.st8.down = gift.KeyOther
		}
		return true

	case gift.EventPointerMove:
		// Consumed so that a finger that slides over the keys does not reach
		// a scroll container underneath and drag the form out from under it.
		// The key under the finger is *not* re-armed: a keyboard types on the
		// press, and a slide that retargeted would type a whole row.
		return true
	}
	return false
}

// press performs one key.
func (n *keyboardNode) press(ctx *gift.EventContext, k kbKey) {
	switch k.cmd {
	case kbShift:
		n.st8.shift = !n.st8.shift
		return
	case kbPage:
		// Shift is dropped with the page. It applies to letters and the
		// symbol page has none, so a shift that survived the round trip would
		// be an invisible mode.
		n.st8.symbols = !n.st8.symbols
		n.st8.shift = false
		return
	case kbDismiss:
		// The whole gesture, and deliberately no character and no key code:
		// dismissing is not typing. The field hears EventFocusLost, withdraws
		// its request and this keyboard is gone on the next build; see
		// [gift.App.dismissSoftKeyboard].
		ctx.DismissSoftKeyboard()
		return
	case kbKeyCode:
		n.st8.down = k.key
		ctx.KeyDown(k.key, 0)
	}
	if k.r != 0 {
		ctx.TypeRune(k.r)
	}
	if n.st8.shift {
		// Shift is one shot, the way it is on every touch keyboard and the
		// way a physical shift is when it is held for one letter. There is no
		// caps lock: it would need a second visual state and a double tap
		// gesture to reach, and a kiosk form asks for a name and not for a
		// paragraph in capitals.
		n.st8.shift = false
	}
}

// keyOf returns the key at the flat index i on the page that is showing.
func (n *keyboardNode) keyOf(i int) kbKey {
	rows := n.st8.rows()
	for r := range kbRows {
		if i < len(rows[r]) {
			return rows[r][i]
		}
		i -= len(rows[r])
	}
	return kbKey{}
}

// keyAt returns the flat index of the key under the device space point p, or
// -1 for a press that landed in a gap.
//
// The comparison happens in device space, which is where [gift.Event.Pos]
// lives; see [textFieldNode.offsetOf] for why that distinction is not
// academic.
func (n *keyboardNode) keyAt(ctx *gift.EventContext, p geom.Point) int {
	b := ctx.DeviceBounds()
	for i := range n.rects {
		if n.rects[i].Translate(b.Min).Contains(p) {
			return i
		}
	}
	return -1
}

// KeyRectForTest returns the rectangle of the key labelled label on the page
// that is showing, in the local space of the keyboard node, and whether there
// is such a key. It reports false while no keyboard is up, which is the answer
// a test needs: a rectangle from the last keyboard would send a tap into the
// form behind it. Add the origin of the keyboard node's bounds to it to get a
// point to press.
//
// It exists because a key is not a node: there is nothing for a gifttest
// selector to find, by the deliberate design of [keyboardNode]. A test adds
// the origin of the keyboard node's bounds to this and clicks there. It is
// exported rather than hidden in an export_test file because a *user's* test
// of a form on a kiosk needs it for exactly the same reason gift's own tests
// do, and giving them no way to press a key would make the keyboard
// untestable outside this package.
func KeyRectForTest(label string) (geom.Rect, bool) {
	i, ok := kbIndexOf(label)
	if !ok {
		return geom.Rect{}, false
	}
	if kbLive == nil {
		return geom.Rect{}, false
	}
	return kbLive.rects[i], true
}

func kbIndexOf(label string) (int, bool) {
	rows := kbState.rows()
	i := 0
	for r := range kbRows {
		for k := range rows[r] {
			if rows[r][k].label == label {
				return i, true
			}
			i++
		}
	}
	return 0, false
}

// --- modifiers ---------------------------------------------------------------

// Font sets the font the key labels are shaped and drawn with, overriding
// [SetDefaultFont] for this keyboard only.
func (v KeyboardView) Font(f Font) KeyboardView { v.font = f; return v }

// FontSize sets the em size of the key labels in pixels. It does not change
// the size of the keys, which is a touch target and not a text measurement;
// see [kbKeyHeight].
func (v KeyboardView) FontSize(s float32) KeyboardView {
	if !isFinite(s) || s <= 0 {
		panic("gift/ui: OnScreenKeyboard.FontSize must be a finite number greater than zero")
	}
	v.size = s
	return v
}

// Foreground sets the colour of the key labels.
func (v KeyboardView) Foreground(c Color) KeyboardView { v.fg, v.hasFG = c, true; return v }

// KeyFace sets the fill of a key at rest, replacing the themed
// [ColorControl].
func (v KeyboardView) KeyFace(c Color) KeyboardView { v.keyFace, v.hasFace = c, true; return v }

// PressedKeyFace sets the fill of the key under the finger, replacing the
// themed [ColorControlPressed].
func (v KeyboardView) PressedKeyFace(c Color) KeyboardView {
	v.keyPressed, v.hasPressed = c, true
	return v
}

// ShiftColor sets the fill of the shift key while shift is armed, replacing
// the themed [ColorAccent].
func (v KeyboardView) ShiftColor(c Color) KeyboardView {
	v.keyAccent, v.hasAccent = c, true
	return v
}

// Padding sets the same padding on all four edges around the keys, replacing
// the default of [kbPadding].
func (v KeyboardView) Padding(f float32) KeyboardView { v.setPadding(f); return v }

// PaddingInsets sets the padding per edge, replacing any previous padding.
func (v KeyboardView) PaddingInsets(i geom.Insets) KeyboardView { v.setPaddingInsets(i); return v }

// Frame fixes both axes. Pass [geom.Unbounded]() for an axis that should stay
// free; the width then falls back to [kbDefaultWidth] and the height to five
// rows of keys.
//
// Fixing the height does not resize the keys. They stay a fingertip tall and
// the rows are clipped at the bottom, which is the honest outcome of asking
// for less room than a keyboard needs.
func (v KeyboardView) Frame(w, h float32) KeyboardView { v.setFrame(w, h); return v }

// MinWidth raises the minimum width of the keyboard, and the maximum with it
// if that is lower; see [frameSpec].
func (v KeyboardView) MinWidth(f float32) KeyboardView { v.setMinWidth(f); return v }

// MinHeight raises the minimum height of the keyboard; see [frameSpec].
func (v KeyboardView) MinHeight(f float32) KeyboardView { v.setMinHeight(f); return v }

// MaxWidth lowers the maximum width of the keyboard; see [frameSpec].
func (v KeyboardView) MaxWidth(f float32) KeyboardView { v.setMaxWidth(f); return v }

// MaxHeight lowers the maximum height of the keyboard; see [frameSpec].
func (v KeyboardView) MaxHeight(f float32) KeyboardView { v.setMaxHeight(f); return v }

// Background fills the panel behind the keys, replacing the themed
// [ColorSurface].
func (v KeyboardView) Background(b Background) KeyboardView { v.setBackgroundSpec(b); return v }

// Border strokes the inside of the panel, replacing the themed hairline.
func (v KeyboardView) Border(b Border) KeyboardView { v.setBorder(b); return v }

// Shadow draws a blurred copy of the panel behind it. It extends the paint
// bounds but not the layout size and not the hit area.
func (v KeyboardView) Shadow(s Shadow) KeyboardView { v.setShadow(s); return v }

// CornerRadius rounds the panel's background and border. It does not change
// the corners of the keys, which are [kbKeyRadius].
func (v KeyboardView) CornerRadius(f float32) KeyboardView { v.setCornerRadius(f); return v }

// Clip is accepted only as Clip(true), which is what a keyboard already does.
//
// A keyboard that did not clip would draw its bottom row outside its own
// bounds when a [KeyboardView.Frame] gave it less height than five rows need,
// on top of the form it is supposed to sit below, while the obstruction the
// scroll reveal avoids stayed the smaller rectangle. [TextFieldView.Clip]
// refuses the same thing for the same kind of reason.
func (v KeyboardView) Clip(b bool) KeyboardView {
	if !b {
		panic("gift/ui: Clip(false) on an OnScreenKeyboard; a keyboard smaller than its five " +
			"rows would paint them over the form it is sitting below")
	}
	return v
}

// Key sets the reconciliation key of this view among its siblings.
func (v KeyboardView) Key(s string) KeyboardView { v.setKey(s); return v }

// Flex makes the keyboard take a share of the remaining main axis space of its
// parent stack, proportional to f. In the [ZStack] it is meant to live in this
// does nothing; see [Overlay.Flex].
func (v KeyboardView) Flex(f float32) KeyboardView { v.setFlex(f); return v }
