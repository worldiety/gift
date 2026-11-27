package ui

import (
	"slices"
	"strconv"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
)

var navigationStackType = gift.RegisterType("ui.NavigationStack")

// Metrics of the title bar, in logical pixels. See [TabBarHeight] for why
// these are constants and not modifiers.
const (
	// NavigationBarHeight is the height of the title bar of a
	// [NavigationStackView].
	//
	// It is above [ControlHitTarget] so that the back affordance inside it
	// can be a full sized target with a little room around it.
	NavigationBarHeight = float32(52)

	// navBackIconSize is the edge of the chevron of the back affordance.
	navBackIconSize = float32(22)
	// navTitleSize is the font size of the title.
	navTitleSize = float32(17)
	// navBarPadding is the horizontal inset of the bar's contents.
	navBarPadding = float32(8)
)

// ScreenSpec is one screen of a [NavigationStackView]. It is created by
// [Screen]; the zero value is not useful.
type ScreenSpec struct {
	title   string
	content gift.View
	key     string
}

// Screen describes one screen of a navigation stack.
//
// title is drawn in the bar while this screen is on top, and is the accessible
// name of the bar. content is the screen itself and ownership of it passes to
// gift.
//
// The reconciliation identity defaults to the title, like [Tab]'s does, and
// for the same reason: a stack that is popped and pushed again has to be able
// to say which screen is which, and the index is exactly the thing that
// changes. Two screens of one stack with the same title need
// [ScreenSpec.Key]. A build with -tags giftdebug rejects duplicate sibling
// keys; a release build does not check, and matches the duplicates by their
// position among themselves, which is the identity the key was meant to
// replace.
func Screen(title string, content gift.View) ScreenSpec {
	if content == nil {
		panic("gift/ui: Screen with a nil content view")
	}
	return ScreenSpec{title: title, content: content}
}

// Key sets the reconciliation identity of this screen, replacing the default
// of the title; see [Screen].
func (s ScreenSpec) Key(v string) ScreenSpec { s.key = v; return s }

func (s ScreenSpec) identity(i int) string {
	switch {
	case s.key != "":
		return s.key
	case s.title != "":
		return s.title
	default:
		return "screen-" + strconv.Itoa(i)
	}
}

// NavigationStackView is a stack of screens within one place: a title bar with
// a back affordance and, under it, the screen on top of the stack. It is
// created by [NavigationStack]; the zero value is not useful.
//
//	ui.NavigationStack(pop, screens...)
//
// # It holds no stack
//
// The name says "stack" and the type stores none. The screens are an argument,
// exactly like the tabs of a [TabBarView] and the children of a [VStack]: the
// application owns the list, a push is appending to it and a pop is dropping
// the last element. There is no Push method and there is deliberately no
// internal history, because a navigation state that lives inside a widget is a
// second source of truth next to the application's own — and the first thing
// any real application wants is to restore it, to deep link into it, or to
// refuse to leave a screen. All three are trivial when the list is the
// application's and impossible when it is not. This is the one-way data flow
// of the project plan, section 5, applied to navigation.
//
// The consequence to keep in mind is that *the back affordance is a request*.
// It calls onBack and nothing happens until the application shortens its list
// and rebuilds. A screen that must not be left simply does not shorten it.
//
// # A covered screen stays mounted and is not drawn
//
// The same decision as [TabBarView], for the same reasons and with the same
// costs; the long form of the argument is there. The screen below the one on
// top keeps its state, its scroll offset and its half typed text, costs
// nothing per frame because it is not painted and not hit tested, and becomes
// visible again when it is popped back to — with the list still where the user
// left it.
//
// "Where the user left it" is meant literally, and it is the one promise here
// that took work rather than falling out of the flag. A fling in flight when
// the screen is covered used to keep ticking: measured at 140 of 600 frames
// and an offset that travelled from 400 to 1395 document units, so the list
// came back a thousand pixels past where the finger left it. The fling is now
// stopped at the moment the screen is hidden, offset intact; see
// [gift.App.stopHiddenWork].
//
// The cost this type has and a tab bar does not: a stack that grows without
// bound keeps every screen it ever pushed. Ten levels of drill-down is ten
// live subtrees. That is the application's list and the application's problem,
// and it is the honest consequence of not owning the history.
//
// # The back affordance
//
// A chevron and the previous screen's title, at the leading edge of the bar,
// as a [ButtonView] of at least [ControlHitTarget] on both axes. It is absent
// on the root screen — there is nowhere to go — which is also why popping the
// root is impossible rather than merely discouraged: no affordance offers it
// and the escape key below refuses it.
//
// Escape pops as well, and it is honestly a secondary path. gift delivers keys
// to the focused node and bubbles them upwards, so a focused escape reaches the
// stack when the focus is *inside* it; with nothing focused at all — a touch
// panel with a keyboard plugged in, where nothing was ever focused — the stack
// declares [gift.Element.KeyFallback] and the unfocused escape is delivered to
// it. Two limits follow from the fallback's rule. While a [Modal] or a
// [Sheet] is open its focus trap bounds the search, so an escape cannot pop
// the screen behind it. And a stack placed *inside* a sheet is the last
// fallback in document order, so it takes escape before the sheet does and
// consumes it even on its root screen: the sheet's [SheetView.OnDismiss] never
// sees escape, and such a sheet is closed by a tap on the scrim, by the back
// action of the platform (below) or by the application.
//
// # The back button and the back gesture of the platform
//
// Android's system back — the button, and the gesture from the edge of the
// screen — pops as well, through [gift.App.Back], and it differs from escape
// in exactly one place: the root screen. Escape is consumed there, for the
// reason [navBackKey.HandleEvent] gives. Back is declined there, because back
// has a third party escape does not have: the operating system, which leaves
// the application when nobody in it goes back. A stack on its root screen
// that took the back action and did nothing would trap the user in the
// application; one that declines lets the action reach the stack or the
// [Sheet] around it and, on the very first screen of the application, the
// system. A pushed screen takes it even when onBack is nil, for the reason a
// nil onBack still draws the affordance: the application said there is a
// screen under this one, and leaving the application from the second screen
// because the pop was not wired up would be the worst way to find out.
//
// Back does not need the focus inside the stack, or a fallback flag: gift
// takes the last [gift.BackHandler] in document order inside the active focus
// trap that answers true, and the focused node and its ancestors decide only
// between handlers side by side — the first of them that answers true goes
// back unless that last one lies inside it. So a stack nested in another pops
// before the outer one, also while the focus is inside the outer one but
// outside the inner — a tab of a [TabBar] that holds the focus does not keep
// the stack of its tab from popping — and a stack on its root screen, which
// declines, hides nothing before it. See [gift.App.Back] for the route.
//
// gift has no edge swipe of its own; the system's back gesture arrives as
// the back action above. A horizontal drag near the edge of the screen is the
// gesture a horizontally scrolling gallery already owns, and gift's pointer
// arbitration is "whoever steals the pointer last wins" rather than an arena
// — see [gift.EventContext.StealPointer] — so the two could not be told apart
// without building one.
type NavigationStackView struct {
	base
	onBack  func()
	screens []ScreenSpec
	backSym Symbol
	still   bool
}

// NavigationStack returns a navigation stack showing the last of screens, with
// a back affordance that calls onBack.
//
// An empty screens list panics: a navigation stack with nothing in it has no
// title, no content and no meaning, and the application state that produced it
// is broken in a way a blank screen would hide. A nil onBack makes the back
// affordance inert but still drawn, which is the same choice [Button] makes
// for a nil action.
//
// The screens slice belongs to gift from this call onwards.
func NavigationStack(onBack func(), screens ...ScreenSpec) NavigationStackView {
	if len(screens) == 0 {
		panic("gift/ui: NavigationStack with no screens")
	}
	return NavigationStackView{onBack: onBack, screens: screens}
}

// ViewType implements gift.View.
func (v NavigationStackView) ViewType() gift.TypeID { return navigationStackType }

// Build implements gift.View.
//
// A vertical stack of the bar and a flexible content area holding one [layer]
// per screen, all hidden but the last. The NavigationStack node is that stack;
// there is no wrapper node.
func (v NavigationStackView) Build(*gift.BuildContext) gift.Element {
	top := len(v.screens) - 1
	e := gift.Element{Key: v.key, Flex: v.flex, Layouter: anchorLayout{},
		Children: []gift.View{gift.Component("ui.navigation", v.body)}}
	// The escape key. The stack node is an ancestor of every screen, so a key
	// the focused node did not consume bubbles here; see [App.deliver].
	// Declaring an interactor and not declaring Focusable is the same thing
	// ui.OnScreenKeyboard does: it makes the node reachable by a bubbling
	// event without putting it in the tab order and without letting a tap on
	// the background of a screen land on it — there is no background, the
	// node draws nothing.
	//
	// It does mean a tap anywhere inside the stack that nothing else handled
	// arrives here. That is harmless: the handler answers false to everything
	// that is not the escape key, so the event keeps bubbling exactly as it
	// would have.
	e.Interactor = navBackKey{stack: v, canPop: top > 0}
	// And the kiosk half of the escape key. Bubbling from the focused node
	// reaches this handler only when something *is* focused, which on a touch
	// panel with no keyboard attached is never: gift moves the focus on a
	// press and there is nothing to press that takes it. This flag says
	// "deliver an unfocused key here", so escape from a keyboard plugged into
	// a kiosk pops the screen whether or not the user has ever touched a
	// control. See [gift.Element.KeyFallback] for the rule when stacks are
	// nested, which is the same last-in-document-order rule as the focus
	// trap's.
	e.KeyFallback = true
	return e
}

// navMemory is what a stack remembers of its last build: the screens it
// showed, and the one a pop took away while it slides out.
type navMemory struct {
	ids     []string
	screens []ScreenSpec
	leaving *leavingScreen
}

// leavingScreen is a screen a pop took away, which stays mounted, hidden
// and out of input, until it has slid out at the trailing edge.
type leavingScreen struct {
	id     string
	screen ScreenSpec
	until  time.Time
}

// body is the stack: the bar and the screens, the one on top shown.
func (v NavigationStackView) body(ctx *gift.Context) gift.View {
	mem := ctx.State("memory", (*navMemory)(nil))
	rev := ctx.State("rev", 0)
	ctx.Read(rev)
	m := mem.Get()
	if m == nil {
		m = &navMemory{}
		mem.Set(m)
	}
	top := len(v.screens) - 1
	ids := make([]string, len(v.screens))
	for i, s := range v.screens {
		ids[i] = s.identity(i)
	}
	// A pop is the list of the last build cut short. Its top screen is kept
	// for the length of its exit, so that it slides out while the screen it
	// uncovers slides in beside it, as a push does the other way round.
	if !v.still && len(m.ids) > len(ids) && slices.Equal(m.ids[:len(ids)], ids) {
		last := len(m.ids) - 1
		m.leaving = &leavingScreen{id: m.ids[last], screen: m.screens[last], until: time.Now().Add(ControlAnimation)}
	}
	if m.leaving != nil && (v.still || slices.Contains(ids, m.leaving.id)) {
		m.leaving = nil
	}
	m.ids, m.screens = ids, slices.Clone(v.screens)

	layers := make([]gift.View, 0, len(v.screens)+2)
	for i, s := range v.screens {
		l := newLayer(ids[i], s.content).
			Hidden(i != top).
			Parked(geom.Pt(-1, 0)).
			Entry(geom.Pt(1, 0)).
			Still(v.still)
		if m.leaving == nil {
			l = l.Return(geom.Pt(-navReturn, 0))
		}
		layers = append(layers, l)
	}
	if lv := m.leaving; lv != nil {
		layers = append(layers,
			newLayer(lv.id, lv.screen.content).Hidden(true).Parked(geom.Pt(1, 0)),
			navSweep{after: max(time.Until(lv.until), time.Millisecond), done: func() {
				m.leaving = nil
				rev.Set(rev.Get() + 1)
			}})
	}
	// The screens are cut at the edges of the stack, so that a screen that
	// slides out does not slide over what is beside it, a sidebar say. Where
	// the stack lies on the edge of the safe area the cut is at the edge of
	// the window instead; see [navContent].
	return VStack(
		v.bar(top),
		navContent{layers: layers},
	).Flex(1)
}

// navContent is the content area of a navigation stack: the [ZStack] of its
// screens, cut at its own edges for painting and for input.
//
// # Why the paint cut reaches the window's edge
//
// The cut exists for the slide of a push and a pop: a screen on its way out
// must not be drawn over a sidebar beside the stack, or over the bar above
// it. On a phone the content area of a stack that fills the window ends on
// the safe bottom edge, above the navigation bar of the system, and an
// application is free to put a [TabBar] at the bottom of a screen. That bar
// paints its surface down to the edge of the window — see
// [gift.PaintContext.SafeAreaBleed] — and a cut at the content area took
// the part behind the navigation bar away again.
//
// So the area declares [gift.Element.ClipBleedsSafeArea]: at each edge where
// it lies on the edge of the safe rectangle the paint cut is at the edge of
// the window, where nothing is beside the stack that the slide could cover.
// Input stays cut at the area, and without a safe area, on every desktop and
// kiosk, nothing changes at all. The view type is the ZStack's own, so the
// node is the one it always was.
type navContent struct {
	layers []gift.View
}

func (navContent) ViewType() gift.TypeID { return zstackType }

func (c navContent) Build(bc *gift.BuildContext) gift.Element {
	e := ZStack(c.layers...).Flex(1).Clip(true).Key("content").Build(bc)
	e.ClipBleedsSafeArea = true
	return e
}

// navSweep takes the leaving screen away once it has slid out, from a timer
// of its node; it lays out to nothing and draws nothing.
type navSweep struct {
	after time.Duration
	done  func()
}

var navSweepType = gift.RegisterType("ui.navSweep")

func (navSweep) ViewType() gift.TypeID { return navSweepType }

func (s navSweep) Build(*gift.BuildContext) gift.Element {
	n := &navSweepNode{s: s}
	return gift.Element{Key: "sweep", Layouter: n, Interactor: n}
}

type navSweepNode struct {
	s     navSweep
	armed bool
}

func (n *navSweepNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	if !n.armed {
		n.armed = true
		ctx.After(n.s.after)
	}
	return c.Constrain(geom.Size{})
}

func (n *navSweepNode) HandleEvent(_ *gift.EventContext, e gift.Event) bool {
	if e.Kind != gift.EventTimer {
		return false
	}
	n.s.done()
	return true
}

// navReturn is how far towards the leading edge a screen waits before it is
// uncovered by a pop, as a fraction of its own width.
//
// # The three places a screen of a stack can be, and why there are three
//
// A push slides the arriving screen in from the trailing edge while the screen
// it covers slides a full width out to the leading one. A full width and not
// an eighth: the two then move in lockstep and their edges meet, so neither is
// ever drawn over the other. That matters here in a way it does not in a tab
// bar, because a screen in this package draws no background of its own — the
// cards do — so two overlapping screens are two screens legible *through* each
// other. The alternative, giving every screen an opaque background, was built
// and measured and then taken out again: the window of this project's own demo
// has no background at all, so painting one only while a screen moves is a
// visible flash at both ends of every transition, and painting one always
// changes the resting picture of every golden image in the repository. The
// geometry is the cheaper fix.
//
// A pop cannot be the mirror image of that, and the reason is worth stating
// rather than hiding. The screen being popped is *unmounted* — the application
// shortened its list, which is what a pop is in this design; see
// [NavigationStackView] — and an unmounted node has no state, no bounds and
// nothing to paint, so nothing in gift can animate it out. Only the revealed
// screen moves. If it came back from the full width it was parked at, the
// window would be empty for the first frame of every pop, so it comes back
// from here instead: a short slide, with a strip of this width at the trailing
// edge showing whatever is behind the stack until it arrives. Moving where it
// waits is invisible, because a parked screen is not painted;
// [gift.TransitionSpec.Return] is that move.
//
// That was the whole story until the stack learned to keep the popped screen
// itself: [NavigationStackView.body] remembers the screens of its last build
// and, after a pop, keeps the one taken away as a hidden layer parked at the
// trailing edge until it has slid there, so a pop is the mirror image of a
// push after all. The short return is what remains for a screen uncovered
// without such a layer, a stack that is popped by more than one screen.
const navReturn = float32(0.08)

// bar builds the title bar of the screen at index top.
func (v NavigationStackView) bar(top int) gift.View {
	title := Text(v.screens[top].title).
		FontSize(navTitleSize).
		Foreground(ColorLabel).
		MaxLines(1).
		Align(AlignCenter).
		Flex(1)

	var leading gift.View
	if top > 0 {
		back := Text(v.screens[top-1].title).
			FontSize(navTitleSize).
			Foreground(ColorAccent).
			MaxLines(1)
		var label gift.View = back
		if !v.backSym.IsZero() {
			label = HStack(
				Icon(v.backSym).Size(navBackIconSize).Foreground(ColorAccent),
				back,
			).Gap(2).Align(geom.Center)
		}
		leading = Button(label, v.onBack).
			Key("back").
			MinWidth(ControlHitTarget).
			MinHeight(ControlHitTarget).
			Padding(4).
			Background(ColorClear).
			Border(Border{}).
			CornerRadius(0).
			PressedStyle(ButtonStyle{Background: ColorControlPressed, CornerRadius: 8}).
			Label("Back")
	} else {
		// A spacer of the same minimum width as the affordance would be,
		// so that the centred title does not jump sideways when a screen is
		// pushed. It is not a Spacer with a Flex — that would take the whole
		// remainder — but a fixed, empty box.
		leading = Box().Frame(ControlHitTarget, 0)
	}

	return VStack(
		HStack(
			leading,
			title,
			// The trailing counterweight. Without it the flexible title is
			// centred inside the space that is left over after the back
			// affordance, which is not the middle of the bar.
			Box().Frame(ControlHitTarget, 0),
		).
			Frame(geom.Unbounded(), NavigationBarHeight).
			PaddingInsets(geom.Insets{Left: navBarPadding, Right: navBarPadding}).
			Align(geom.Center),
		Box().Frame(geom.Unbounded(), 1).Background(ColorSeparator),
	).Background(ColorSurface).
		// The surface reaches the top of the window behind the status bar,
		// and the sides behind a cutout; the title stays below them. See
		// [Stack.BleedSafeArea].
		BleedSafeArea(EdgesLeft | EdgesRight | EdgesTop).
		Key("bar")
}

// navBackKey is the interactor of the stack node: the escape key and, as a
// [gift.BackHandler], the back action of the platform.
//
// It is a value type and is stored in the interface by value, so the element
// of a navigation stack allocates nothing beyond the stack node itself, and
// asking it whether it can go back is reading a bool.
type navBackKey struct {
	stack  NavigationStackView
	canPop bool
}

// HandleEvent implements gift.Interactor.
func (n navBackKey) HandleEvent(_ *gift.EventContext, e gift.Event) bool {
	if e.Kind != gift.EventKeyDown || e.Key != gift.KeyEscape {
		return false
	}
	if !n.canPop {
		// The root screen. The key is still consumed, because an escape that
		// travelled out of a navigation stack to whatever encloses it would
		// close a modal the user cannot see from here, or dismiss something
		// else entirely. "Back on the root screen does nothing" is a
		// deliberate stop and not an omission.
		return true
	}
	if n.stack.onBack != nil {
		n.stack.onBack()
	}
	return true
}

// CanGoBack implements gift.BackHandler: a stack takes the back action while
// there is a screen to pop, and only then.
//
// This is where back and escape part ways. On the root screen escape is
// consumed and back is declined: what is around the stack — an outer stack,
// a sheet, and finally the operating system, which leaves the application —
// gets it instead. See [NavigationStackView], "The back button and the back
// gesture of the platform".
func (n navBackKey) CanGoBack() bool { return n.canPop }

// GoBack implements gift.BackHandler. It asks the application to pop, exactly
// as the back affordance and escape do, and nothing happens until the
// application shortens its list.
//
// A nil onBack takes the action and does nothing. The screen on top was
// pushed, and a back that fell through to the system would leave the whole
// application from the second screen of it; doing nothing is the same answer
// the inert affordance gives.
func (n navBackKey) GoBack(*gift.EventContext) {
	if n.stack.onBack != nil {
		n.stack.onBack()
	}
}

// --- modifiers -------------------------------------------------------------

// Animated turns the slide of a push and a pop on or off; it is on unless
// turned off. A stack beside a sidebar turns it off, as the settings of macOS
// do: a page there replaces the one beside the list without moving.
func (v NavigationStackView) Animated(b bool) NavigationStackView { v.still = !b; return v }

// BackIcon sets the symbol drawn before the previous screen's title in the
// back affordance. The conventional choice is a leading chevron:
//
//	ui.NavigationStack(pop, screens...).BackIcon(outline.AngleLeft)
//
// It is a parameter and not a built in glyph because package ui cannot import
// an icon package — gift/icon/outline imports *this* package to declare its
// variables as [Symbol], so the dependency only runs one way; see [Symbol].
// Without it the affordance is the previous title alone, which is legible and
// is what an application that imports no icons gets.
func (v NavigationStackView) BackIcon(s Symbol) NavigationStackView { v.backSym = s; return v }

// Key sets the reconciliation key of this view among its siblings.
func (v NavigationStackView) Key(s string) NavigationStackView { v.setKey(s); return v }

// Flex makes the stack take a share of the remaining main axis space of its
// parent stack. A navigation stack inside a [TabBarView] is already given the
// whole tab and needs none.
func (v NavigationStackView) Flex(s float32) NavigationStackView { v.setFlex(s); return v }
