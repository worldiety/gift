package ui

import (
	"fmt"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/layout"
)

var (
	sheetType      = gift.RegisterType("ui.Sheet")
	sheetHostType  = gift.RegisterType("ui.sheetHost")
	sheetPanelType = gift.RegisterType("ui.sheetPanel")
)

// Metrics of a sheet, in logical pixels unless said otherwise.
const (
	// sheetRadius is the corner radius of the top edge of a sheet.
	sheetRadius = float32(14)
	// sheetHeight is the default height of a sheet as a fraction of the
	// height it is presented in: tall enough to work in, short enough that
	// the dimmed screen above says "this goes back where it came from".
	sheetHeight = float32(0.9)
	// sheetMaxWidth is the default widest a sheet gets. A sheet across the
	// whole of a 1920 pixel kiosk panel is a page, not a sheet.
	sheetMaxWidth = float32(720)
)

// SheetView presents one view in a panel that slides up from the bottom edge
// over another view, and takes the input of everything underneath. It is
// created by [Sheet]; the zero value is not useful.
//
//	ui.Sheet(screen, picker).
//	    Presented(open).
//	    OnDismiss(func() { setOpen(false) })
//
// # What it is made of
//
// It is a [ModalView] with a different dialog, and the three things that make
// a modal modal are the same three things here, built from the same parts: the
// scrim that swallows every pointer event outside the panel, the focus trap,
// and the drawing order. What differs is the panel: it is anchored to the
// bottom edge, as wide as the window up to [SheetView.MaxWidth], as tall as
// [SheetView.Height] of it, and its whole height away while it is not
// presented, so that it arrives from below the window rather than from under
// its own footprint.
//
// # Dismissing it
//
// A tap on the scrim, and the escape key, call [SheetView.OnDismiss]. Escape is
// delivered to the sheet whether or not anything in it holds the focus — see
// [gift.Element.KeyFallback] — because a sheet opened by a tap on a touchscreen
// has nothing focused and a key that reached nobody would be a key that does
// nothing. A sheet without OnDismiss can be closed only by the application,
// which is what a sheet that demands an answer wants.
//
// The back button and the back gesture of Android — see [gift.App.Back] — do
// what escape does: they call OnDismiss. A sheet without OnDismiss takes them
// and does nothing, as a dialog that is not cancelable does on Android, so that
// back neither leaves the application nor acts on the screen behind the
// sheet. A [NavigationStack] inside the sheet pops first, and on its root
// screen it lets back through to the sheet: unlike escape, which the stack
// consumes there, back on the first screen of a sheet closes it.
//
// There is no drag to dismiss. The panel is a surface, not a control.
//
// # Presented, not nil
//
// For the reason [ModalView.Presented] gives: a nil sheet is unmounted and
// cannot be seen to leave. Pass the sheet's view and say whether it is up.
//
// # The on-screen keyboard
//
// A [Window] with an [OnScreenKeyboard] among its content lays the rest of
// that content out above the keyboard while it is showing, so a sheet inside
// such a Window sits on the keyboard rather than under it, and a text field at
// the bottom of the sheet stays in sight. The sheet gets shorter by the
// keyboard's height while it is up, because its height is a fraction of the
// space it is given.
//
// # The system bars
//
// A [Window] on a phone presents a sheet in its safe area, above the
// navigation bar, and the sheet's surface reaches on behind the navigation
// bar to the bottom edge of the window — and behind the cutout and the side
// navigation bar of a phone on its side, where the sheet is as wide as the
// safe area — while its content stays inside the safe area. The scrim covers
// the status bar and the navigation bar too. See [sheetPanelNode.Layout].
type SheetView struct {
	base
	content      gift.View
	sheet        gift.View
	onDismiss    func()
	presented    bool
	hasPresented bool
	scrim        Color
	hasScrim     bool
	height       float32
	maxWidth     float32
}

// Sheet returns content with sheet presented over it from the bottom edge. A
// nil sheet is the closed state with nothing mounted; see [SheetView] for why
// [SheetView.Presented] is usually the better way to close one.
//
// content must not be nil.
func Sheet(content, sheet gift.View) SheetView {
	if content == nil {
		panic("gift/ui: Sheet with a nil content view")
	}
	return SheetView{content: content, sheet: sheet}
}

// ViewType implements gift.View.
func (s SheetView) ViewType() gift.TypeID { return sheetType }

// Build implements gift.View.
func (s SheetView) Build(bc *gift.BuildContext) gift.Element {
	if s.sheet == nil {
		// Closed means absent, as for [ModalView].
		return ZStack(s.content).Key(s.key).Flex(s.flex).Build(bc)
	}
	wash := defaultScrimColor
	if s.hasScrim {
		wash = s.scrim
	}
	presented := true
	if s.hasPresented {
		presented = s.presented
	}
	h := s.height
	if h == 0 {
		h = sheetHeight
	}
	w := s.maxWidth
	if w == 0 {
		w = sheetMaxWidth
	}
	return ZStack(
		s.content,
		newLayer("sheet", sheetHost{
			scrim: scrim{color: wash, onTap: s.onDismiss},
			panel: sheetPanel{
				hidden:    !presented,
				child:     s.sheet,
				height:    h,
				maxWidth:  w,
				onDismiss: s.onDismiss,
			},
		}).
			Trap(presented).
			Hidden(!presented),
	).Key(s.key).Flex(s.flex).Build(bc)
}

// --- modifiers -------------------------------------------------------------

// OnDismiss makes a tap on the scrim, the escape key and the back action of
// the platform call fn. A nil fn, the default, makes all three do nothing,
// and back is still taken, so it does not reach what is under the sheet; see
// [ModalView.OnDismiss].
func (s SheetView) OnDismiss(fn func()) SheetView { s.onDismiss = fn; return s }

// Presented says whether the sheet is on the screen right now; see
// [ModalView.Presented]. The default is "presented".
func (s SheetView) Presented(v bool) SheetView { s.presented, s.hasPresented = v, true; return s }

// Scrim sets the wash drawn over the content while the sheet is up; see
// [ModalView.Scrim].
func (s SheetView) Scrim(c Color) SheetView { s.scrim, s.hasScrim = c, true; return s }

// Height sets the height of the panel as a fraction of the height it is
// presented in, in (0, 1]. The default is 0.9.
func (s SheetView) Height(f float32) SheetView {
	if !isFinite(f) || f <= 0 || f > 1 {
		panic(fmt.Sprintf("gift/ui: Sheet.Height(%v) must be a fraction in (0, 1]", f))
	}
	s.height = f
	return s
}

// MaxWidth sets the widest the panel gets. A narrower window gets a panel as
// wide as itself. The default is 720.
func (s SheetView) MaxWidth(px float32) SheetView {
	if !isFinite(px) || px <= 0 {
		panic(fmt.Sprintf("gift/ui: Sheet.MaxWidth(%v) must be a positive number", px))
	}
	s.maxWidth = px
	return s
}

// Key sets the reconciliation key of this view among its siblings.
func (s SheetView) Key(v string) SheetView { s.setKey(v); return s }

// Flex makes the sheet host take a share of the remaining main axis space of
// its parent stack.
func (s SheetView) Flex(v float32) SheetView { s.setFlex(v); return s }

// --- the host ----------------------------------------------------------------

// sheetHost lays the scrim and the panel out over each other, the panel on
// the bottom edge and centred: it is ZStack(scrim, panel).Align(geom.Bottom)
// with one thing a ZStack cannot know, which is that the panel may be larger
// than the rectangle it sits on.
//
// A panel on a phone grows past its own rectangle to the edges of the window
// — see [sheetPanelNode.Layout] — and it grows as a node, not only as a face
// drawn outside it, because the panel is the thing that slides: a node that
// its transition is moving is a layer for as long as it moves, and a layer is
// rasterised at the node's bounds and no larger. A face that reached past the
// panel's bounds would be cut back to the panel for every frame of the slide
// and snap out to the edges of the window on the last one; and the panel's
// whole height away, where [gift.TransitionSpec.Parked] puts it, would not be
// below the window but a navigation bar short of it. So the panel's bounds
// cover the reach, and this host places the panel by the rectangle inside it
// that the sheet's content keeps, which is where a ZStack placed the whole
// panel before there was a reach. Without a safe area the two are the same
// rectangle and this is the ZStack's algorithm to the last operation.
type sheetHost struct {
	scrim scrim
	panel sheetPanel
}

// ViewType implements gift.View.
func (h sheetHost) ViewType() gift.TypeID { return sheetHostType }

// Build implements gift.View.
func (h sheetHost) Build(*gift.BuildContext) gift.Element {
	return gift.Element{
		Layouter: &sheetHostNode{},
		Children: []gift.View{h.scrim, h.panel},
	}
}

// sheetHostNode is the layouter of [sheetHost]. It is the overlay algorithm
// of [ZStack] itself, run through a measurer that answers, for the panel, the
// size of the rectangle the content keeps rather than the panel's own.
type sheetHostNode struct {
	ctx     *gift.LayoutContext
	reach   geom.Insets
	items   [2]layout.Item
	origins [2]geom.Point
}

// MeasureChild implements layout.Measurer.
//
// The reach is not passed down to the panel but worked out again here from
// the same constraints by the same rule, [sheetReach]: the panel's height is
// on the bottom edge of the safe area or not whatever the panel says, and
// whether it is as wide as the safe area is told by the size it answers,
// which is wider than the constraints exactly when it reaches at the sides.
func (n *sheetHostNode) MeasureChild(i int, c geom.Constraints) geom.Size {
	sz := n.ctx.Measure(i, c)
	if i != 1 {
		return sz
	}
	r := sheetReach(n.ctx, c.Max, c.Max.W)
	if r == (geom.Insets{}) {
		return sz
	}
	own := geom.Sz(sz.W, sz.H-r.Bottom)
	if sz.W > c.Max.W {
		// Spanning: the content is exactly as wide as the constraints.
		own.W = c.Max.W
	} else {
		r.Left, r.Right = 0, 0
	}
	n.reach = r
	return own
}

// Layout implements gift.Layouter.
func (n *sheetHostNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	k := min(ctx.ChildCount(), len(n.items))
	n.ctx, n.reach = ctx, geom.Insets{}
	res := layout.Overlay(geom.Insets{}, geom.Bottom, c, k, n, n.items[:], n.origins[:])
	if k > 1 {
		// The panel's origin is its face's, the reach to the left of the
		// content; the content is where the overlay put the whole panel.
		n.origins[1].X -= n.reach.Left
	}
	for i := range k {
		ctx.Place(i, n.origins[i])
	}
	ctx.ReportOverflow(res.Overflow)
	n.ctx = nil
	return res.Size
}

// sheetReach is how far a sheet presented in band reaches past it to the edges
// of the window, when it is w wide: the safe area of the bottom edge where band
// is the safe area's height, and of the sides where it is the safe area's
// width and the sheet spans it. Zero without a safe area, which is every
// desktop and every kiosk. See [sheetPanelNode.Layout].
func sheetReach(ctx *gift.LayoutContext, band geom.Size, w float32) geom.Insets {
	s := ctx.SafeArea()
	if s == (geom.Insets{}) {
		return geom.Insets{}
	}
	vp := ctx.Viewport()
	var r geom.Insets
	if band.H == vp.H-s.Vertical() {
		r.Bottom = s.Bottom
	}
	if band.W == vp.W-s.Horizontal() && w == band.W {
		r.Left, r.Right = s.Left, s.Right
	}
	return r
}

// --- the panel ---------------------------------------------------------------

// sheetPanel is the node the sheet hangs under: the thing that moves, as
// [modalDialog] is for a modal, and the thing that sizes the sheet.
type sheetPanel struct {
	hidden    bool
	child     gift.View
	height    float32
	maxWidth  float32
	onDismiss func()
}

// ViewType implements gift.View.
func (p sheetPanel) ViewType() gift.TypeID { return sheetPanelType }

// Build implements gift.View.
//
// The panel has two children, and the content is not inside the surface: the
// face is an empty [ZStack] with the surface colour and a uniform radius, and
// the content is its sibling, painted after it and so on top of it. The face
// is sheetRadius taller than the panel: the extra strip hangs below the
// window edge, so only the top corners are seen rounded. A radius per corner
// would be a new field in every style of this package for one caller.
//
// The two are siblings so that the face can be made larger than the content
// without the content noticing, which is what a phone with a navigation bar
// asks for; see [sheetPanelNode.Layout]. Without a safe area the display list
// is the one a face with the content inside it drew: a container without a
// clip adds nothing to the list but its background, and the content is
// measured and placed exactly as the face's padding and alignment did it.
func (p sheetPanel) Build(bc *gift.BuildContext) gift.Element {
	face := ZStack().
		Background(ColorSurface).
		CornerRadius(sheetRadius).
		Key("face")
	return gift.Element{
		Key:      "panel",
		Layouter: &sheetPanelNode{height: p.height, maxWidth: p.maxWidth},
		Children: []gift.View{face, p.child},
		Hidden:   p.hidden,
		// The whole height of the panel, so it starts and ends below the
		// window edge rather than a third of the way up it.
		Transition: gift.TransitionSpec{
			Parked:   geom.Pt(0, 1),
			Duration: ControlAnimation,
		},
		Interactor:  sheetKeys{onDismiss: p.onDismiss},
		KeyFallback: true,
	}
}

// sheetPanelNode sizes the panel from the space it is presented in.
type sheetPanelNode struct {
	height   float32
	maxWidth float32
}

// Layout takes Height of the available height and up to MaxWidth of the
// available width, lays the content out in that, and lays the face out
// sheetRadius taller than that and as far past it as the panel reaches.
//
// It needs bounded constraints, for the reason a modal does: a sheet in an
// unbounded axis would be zero tall, and the scrim under it zero sized, so
// every tap would go through a sheet that is drawn on the screen.
//
// # The system bars
//
// A sheet inside a [Window] on a phone is presented in the safe area of
// [gift.App.SetSafeArea], so its bottom edge is the top of the navigation bar
// and not the bottom of the window. Its content belongs there — a button at
// the bottom of a sheet must not sit under the home indicator — but its
// surface does not: a sheet that stopped at the navigation bar would show a
// strip of the dimmed screen under itself, a sheet floating above the bottom
// of the phone. So the face grows past the panel to the edge of the window,
// at the bottom by the safe area of the bottom edge on top of the strip it
// already hangs down by, and, where the sheet is as wide as the safe area —
// a phone on its side, with the cutout at one end and the navigation bar at
// the other — at the sides by the safe area of each side. The content keeps
// the rectangle the panel had without a safe area; the panel itself grows by
// the reach, and its size says so, because the panel is the thing that
// slides and a sliding node is drawn no larger than its bounds — see
// [sheetHost], which places the panel by the content's rectangle and so
// keeps everything laid out against the sheet where it was. Its whole height
// away, where it waits while it is not presented, is then below the window
// and not a navigation bar short of it.
//
// This is a change of layout and not the paint-only bleed of the bars — see
// [Stack.BleedSafeArea] — because the surface is rounded, and a rounded
// background never bleeds: the shape that reaches the edge of the window has
// to be laid out there.
//
// Whether the panel is at the edge of the window is told from its
// constraints, the way [Window] tells whether it fills the window: the space a
// sheet is presented in is the space the Window gave its content, and that is
// the safe rectangle exactly when its height, or its width, is the viewport's
// less the safe area. A sheet in a nested Window, or in a frame of its own, is
// not at the edge of the window and its face stays the panel's. A sheet sitting
// on a drawn keyboard is presented in a shorter band, and its face does not
// grow at the bottom, where the keyboard is; at the sides it does. The rule is
// [sheetReach], and [sheetHost] asks it the same question.
func (n *sheetPanelNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	if !c.HasBoundedWidth() || !c.HasBoundedHeight() {
		panic("gift/ui: a Sheet measured with an unbounded axis; put it where it gets the whole " +
			"window, as the outermost view inside ui.Window, the way ui.Modal is placed")
	}
	size := c.Constrain(geom.Sz(min(c.Max.W, n.maxWidth), roundf(c.Max.H*n.height)))
	ext := sheetReach(ctx, c.Max, size.W)

	ctx.Measure(0, geom.Tight(geom.Sz(size.W+ext.Horizontal(), size.H+sheetRadius+ext.Bottom)))
	ctx.Place(0, geom.Point{})
	// The constraints a face with the content inside it gave that content:
	// the panel's rectangle, loosened, at its top left corner, which is now
	// to the right of the reach.
	got := ctx.Measure(1, geom.Loose(size))
	ctx.Place(1, geom.Pt(ext.Left, 0))
	// The face's strip under the bottom edge is the rounding trick of
	// [sheetPanel.Build], and the reach is the window's edge, not content
	// that does not fit. Content that does not fit is.
	ctx.ReportOverflow(geom.Sz(clampLow(got.W-size.W), clampLow(got.H-size.H)))
	if ext == (geom.Insets{}) {
		return size
	}
	return geom.Sz(size.W+ext.Horizontal(), size.H+ext.Bottom)
}

// sheetKeys is the interactor of the panel: the escape key and, as a
// [gift.BackHandler], the back action of the platform.
type sheetKeys struct{ onDismiss func() }

// HandleEvent implements gift.Interactor.
func (k sheetKeys) HandleEvent(_ *gift.EventContext, e gift.Event) bool {
	if e.Kind != gift.EventKeyDown || e.Key != gift.KeyEscape {
		return false
	}
	// Consumed even without a handler, for the reason [navBackKey] gives:
	// an escape that travelled out of the sheet would act on a screen the
	// user cannot reach from here.
	if k.onDismiss != nil {
		k.onDismiss()
	}
	return true
}

// CanGoBack implements gift.BackHandler: a presented sheet takes the back
// action, with or without OnDismiss. A sheet that is not presented is not
// asked: the panel carries [gift.Element.Hidden] itself, and gift skips hidden
// nodes on both routes of the back action, the focused path included.
//
// Without OnDismiss it is a block, deliberately: back that passed through a
// sheet which demands an answer would pop the screen behind it, or leave the
// application, while the question is still on the screen. It is the escape
// key's rule above, for the same reason.
func (k sheetKeys) CanGoBack() bool { return true }

// GoBack implements gift.BackHandler: it asks the application to dismiss the
// sheet, as a tap on the scrim and escape do.
func (k sheetKeys) GoBack(*gift.EventContext) {
	if k.onDismiss != nil {
		k.onDismiss()
	}
}
