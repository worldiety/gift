package ui

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/layout"
)

// nodeKind selects the layout algorithm of a [node].
type nodeKind uint8

const (
	kindStack nodeKind = iota
	kindOverlay
	kindBox
	kindScroll
	kindLayer
	// kindRow is the content stack of a [RowView]: a horizontal stack whose
	// labels give way before the row overflows; see [node.layoutRow].
	kindRow
)

// node is the retained half of every container view: it is both the
// gift.Layouter and, when the view has something to draw, the gift.Painter of
// the node.
//
// One object serves both roles on purpose. A build allocates exactly one of
// these per container, and the scratch buffers it carries are what makes
// layout allocation free afterwards: the node lives for as long as the
// retained scene node does, so a frame that only relayouts reuses the buffers
// it grew on the first frame.
//
// The alternative — keeping the scratch inside gift's node payload — would
// require gift to know about stack layout, which the project plan, section 3,
// puts in internal/layout instead.
type node struct {
	kind nodeKind
	spec layout.StackSpec
	fr   frameSpec
	st   styleSpec

	items   []layout.Item
	origins []geom.Point

	// bar is the scroll indicator of a kindScroll node and is unused by every
	// other kind. It sits here rather than in a scroll specific node type
	// because one struct per container is what keeps a build to one
	// allocation; see the type documentation.
	bar scrollBarState

	// kb is one more than the index of the child that is an on-screen
	// keyboard the other children avoid, or zero when there is none. It is
	// read by kindOverlay only and set by [Overlay.Build]; see
	// [node.layoutWindow]. Offset by one so that the zero value, which is
	// what every other container gets, means "no keyboard" without a
	// separate flag.
	kb int

	// plate says that child 0 is the window background of [Window], which
	// keeps the whole box while the other children avoid the keyboard and
	// the safe area. A node with a plate is always laid out by
	// [node.layoutWindow], keyboard or not, because the safe area is decided
	// there.
	plate bool

	// ignoreSafeArea is [Overlay.IgnoresSafeArea]: the children of a Window
	// are laid out in the whole window, bars and all. Read only with plate.
	ignoreSafeArea bool

	// row is the shrink state of a kindRow node and is unused by every other
	// kind, for the reason bar is here.
	row rowShrink

	// refresh is the pull-to-refresh of a refreshable scroll view, and nil
	// for every other; see [ScrollView.Refreshable].
	refresh *refreshSpec

	// ctx is the layout context of the call in progress. It exists so that
	// the node can implement layout.Measurer through a pointer receiver:
	// boxing a wrapper value into the Measurer interface on every layout
	// would allocate once per container per frame.
	ctx *gift.LayoutContext
}

// MeasureChild implements layout.Measurer.
//
// A row narrows the constraints of its labels here, which is how it makes them
// give way without a second stack algorithm; see [node.layoutRow].
func (n *node) MeasureChild(i int, c geom.Constraints) geom.Size {
	if n.kind == kindRow {
		c = n.row.limit(i, c)
	}
	return n.ctx.Measure(i, c)
}

// ChildBaseline implements layout.Baseliner. It forwards the baseline a child
// reported during this pass, or false when the child reported none — which is
// the answer for anything that is not text, and the one case the algorithm
// must not guess at.
func (n *node) ChildBaseline(i int) (float32, bool) { return n.ctx.ChildBaseline(i) }

// Layout measures the children through the shared algorithms in
// internal/layout and places them.
//
// The constraints the children see are the incoming ones as modified by the
// frame modifiers, and the size reported upwards is the one the frame asks
// for. A Frame that does not fit the incoming constraints therefore wins and
// overflows, which gift passes through unchanged rather than clamping
// silently; see [gift.Layouter].
func (n *node) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	k := ctx.ChildCount()
	n.ensure(k)
	n.ctx = ctx
	cc := n.fr.apply(c)

	var res layout.Result
	switch n.kind {
	case kindStack:
		for i := range k {
			n.items[i].Flex = ctx.ChildFlex(i)
		}
		res = layout.Stack(n.spec, cc, k, n, n.items, n.origins)
	case kindRow:
		res = n.layoutRow(ctx, cc, k)
	case kindScroll:
		res = n.layoutScroll(ctx, cc, k)
	case kindLayer:
		res = n.layoutLayer(ctx, cc, k)
	case kindBox:
		// A Box has no content, so it is greedy: it takes the whole extent
		// on every axis that is bounded, and collapses to its padding on an
		// axis that is not.
		//
		// This is what makes the obvious spelling work. A ZStack hands its
		// children loose constraints, bounded wherever its own are, so
		//
		//	ui.ZStack(ui.Box().Background(c), content)
		//
		// paints the plate behind the content instead of producing a zero
		// sized, invisible box. The minimum-size rule that was here before
		// silently dropped such backgrounds, which cost real debugging time
		// during WU-D.
		//
		// A stack measures an inflexible child with an unbounded main axis —
		// see the project plan, section 7, "Overflow-Modell", rule 1 — so a
		// Box in a VStack collapses on the main axis and fills the cross
		// axis. Give it a height with Frame or MinHeight, or a share of the
		// leftover space with Flex.
		pad := n.spec.Padding
		size := geom.Sz(pad.Horizontal(), pad.Vertical())
		if cc.HasBoundedWidth() {
			size.W = cc.Max.W
		}
		if cc.HasBoundedHeight() {
			size.H = cc.Max.H
		}
		// A Box has no children, so it cannot overflow: it never asks for
		// more than the constraints offer.
		res.Size = cc.Constrain(size)
	default:
		if n.plate || n.kb > 0 && n.kb <= k {
			res = n.layoutWindow(ctx, cc, k)
		} else {
			res = layout.Overlay(n.spec.Padding, n.spec.Alignment, cc, k, n, n.items, n.origins)
		}
	}

	for i := range k {
		ctx.Place(i, n.origins[i])
	}
	// Overflow is reported, never hidden. The children above keep the origins
	// the algorithm gave them even when they do not fit; the project plan,
	// section 7, "Overflow-Modell", rules out clipping them silently, and
	// rule 3 requires the excess to be visible in gift.Diagnostics.
	ctx.ReportOverflow(res.Overflow)
	// Cleared without a defer: a closure would be the only allocation in
	// this function, and a layouter that panics has already put the App into
	// the state the application is expected to report and restart from.
	n.ctx = nil
	return res.Size
}

// Paint draws the node in the fixed order of the project plan, section 8.
// It is installed as the painter only when there is something to draw; see
// [styleSpec.needsPainter].
//
// A scroll container has one more step after the border: its indicator, which
// is an overlay and therefore comes last. See [scrollBarState.paint].
func (n *node) Paint(ctx *gift.PaintContext) {
	paintStyle(ctx, n.st)
	if n.kind == kindScroll {
		if n.refresh != nil {
			n.refresh.paint(ctx)
		}
		n.bar.paint(ctx)
	}
}

// HandleEvent is the interactor of a scroll container: the indicator first,
// gift's gesture afterwards.
//
// It is installed on kindScroll nodes only, and it is the shape
// [gift.ScrollInteractor] documents. The order matters and is not a
// preference: a grabbed thumb has to consume the move before the viewport can
// recognise the same movement as a content drag.
func (n *node) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	if n.bar.handleEvent(ctx, e) {
		return true
	}
	done := gift.ScrollInteractor().HandleEvent(ctx, e)
	if n.refresh != nil {
		n.refresh.handleEvent(ctx, e)
	}
	return done
}

// ensure sizes the scratch buffers to k children, reusing the backing arrays
// whenever they are large enough. A frame in which the child count did not
// change never reallocates.
func (n *node) ensure(k int) {
	if cap(n.items) < k {
		n.items = make([]layout.Item, k)
		n.origins = make([]geom.Point, k)
		return
	}
	n.items = n.items[:k]
	n.origins = n.origins[:k]
}

// layoutScroll is the stack algorithm with the scroll axis unbounded.
//
// # Why the axis is measured unbounded
//
// A scroll container exists precisely so that its content may be larger than
// it is. Measuring the content against the viewport extent would make a tall
// column report the viewport height and there would be nothing to scroll; so
// the scroll axis is [geom.Unbounded] and the cross axis keeps the viewport's
// bound. That is also rule 1 of the overflow model of the project plan,
// section 7, applied to the one container where it is not merely a safety
// property but the whole point.
//
// # Why this is not an overflow
//
// Content taller than the viewport is the normal, intended state here, and
// [gift.Diagnostics.OverflowNodes] must stay zero for it — otherwise the one
// number that says "some container in this scene is lying about its size"
// would be noisy in exactly the scenes it is needed for, and
// [gifttest.Harness.AssertNoOverflow] would be useless in a gallery.
//
// The distinction is not "how much bigger" but *whether the content is
// reachable*. Rule 3 of the overflow model is about content that keeps honest
// positions nobody can ever see; here the content keeps honest positions and
// the user reaches all of them by scrolling. Mechanically it falls out of the
// measurement: the scroll axis is unbounded, and an unbounded axis cannot
// overflow because nothing was exceeded. The excess is reported through
// [gift.LayoutContext.ReportScrollContent] instead, where it is a content
// extent rather than a complaint.
//
// The *cross* axis is a different matter and is reported as an ordinary
// overflow. A child wider than a vertical scroller really is cut off and
// really is unreachable, because there is no horizontal offset to move it
// into view. That is the honest line between the two.
func (n *node) layoutScroll(ctx *gift.LayoutContext, cc geom.Constraints, k int) layout.Result {
	for i := range k {
		n.items[i].Flex = ctx.ChildFlex(i)
	}
	free := cc
	if n.spec.Axis == layout.Horizontal {
		free.Min.W, free.Max.W = 0, geom.Unbounded()
	} else {
		free.Min.H, free.Max.H = 0, geom.Unbounded()
	}
	res := layout.Stack(n.spec, free, k, n, n.items, n.origins)

	// The content extent is what the children came to; the viewport is what
	// the incoming constraints allow. gift derives the viewport from the size
	// returned below, so only the content has to be reported.
	content := res.Size
	ctx.ReportScrollContent(float64(mainExtent(n.spec.Axis, content)), 0)
	res.Size = cc.Constrain(content)
	return res
}

// layoutLayer is the layout of one screen of a navigation container: fill the
// area, and stretch the single child to fill it too.
//
// # Why this is not the overlay algorithm
//
// A [layer] is not a box that happens to hold one child, it *is* the screen —
// the whole of the tab, or the whole of the area above a navigation bar — and
// a screen that shrink-wrapped its content would be the wrong answer twice
// over. Its own size would be the width of the widest label on it, so a
// background behind it would not reach the edges of the window; and its
// child's would be the same, so a [VScroll] inside a tab would be as wide as
// its longest row rather than as wide as the tab.
//
// A stack measures its children with a *loose* cross axis — see
// layout.Stack — so neither of those falls out of the ordinary machinery, and
// there is no "stretch" cross alignment in this project to ask for. There does
// not need to be one here: a layer has exactly one child, and the question
// "where does the child sit inside the leftover space" has no leftover space
// to be about.
//
// # An unbounded axis is refused
//
// A layer must fill what it is given, and an axis with no finite maximum gives
// it nothing to fill. The earlier version of this function shrink-wrapped such
// an axis, and that was a silent defect of the worst class this project has:
// the scrim of a [ModalView] is a layer child with no content of its own, so
// on an unbounded axis it measured zero — while the alert above it sized and
// painted exactly as usual. The dialog was on the screen, looked right, and a
// tap went straight through it to the "Delete all" button underneath. Nothing
// reported it: the size equals the constraint on an unbounded axis, so the
// overflow is zero, there is no visual difference, and there is no diagnostic.
//
// A panic is therefore the only honest answer. "Documented and not fixed" was
// not available, because [ModalView] promises without qualification that the
// scrim swallows every pointer event; and a diagnostic that only fires under
// giftdebug would let a kiosk ship with a confirmation dialog that is
// decoration. The message names the composition, because the composition is
// always the same shape — a navigation container inside something that
// measures its children loosely, which in this package means a scroll
// container or an inflexible child of a stack.
func (n *node) layoutLayer(ctx *gift.LayoutContext, cc geom.Constraints, k int) layout.Result {
	if !cc.HasBoundedWidth() || !cc.HasBoundedHeight() {
		panic("gift/ui: a navigation container was measured with an unbounded " +
			"axis. ui.TabBar, ui.NavigationStack, ui.Modal and ui.Sheet are " +
			"screens: they fill the area they are given, and there is nothing " +
			"to fill here. " +
			"The two ways to get here are putting one inside a scroll container, " +
			"which is not a place a screen belongs, and making it an inflexible " +
			"child of a stack, where the stack measures it with an unbounded main " +
			"axis — ui.VStack(nav) rather than ui.VStack(nav.Flex(1)). Give it a " +
			"Flex, or wrap it in a view with a Frame that is finite on both axes, " +
			"such as ui.ZStack(nav).Frame(w, h). This is a panic " +
			"and not a silently smaller screen because the scrim of a ui.Modal " +
			"would measure zero on the unbounded axis while the alert above it " +
			"drew normally, and every tap would go through a dialog that is on " +
			"the screen")
	}
	fill := cc.Constrain(cc.Max)

	size := fill
	if k > 0 {
		child := ctx.Measure(0, geom.Constraints{Min: fill, Max: cc.Max})
		n.origins[0] = geom.Point{}
		// The child may refuse the minimum — a Frame smaller than the screen
		// does — and it may exceed the maximum, which is rule 1 of the
		// overflow model of the project plan, section 7: gift passes an
		// oversized child through rather than clamping it silently.
		if child.W > size.W {
			size.W = child.W
		}
		if child.H > size.H {
			size.H = child.H
		}
	}
	outer := cc.Constrain(size)
	return layout.Result{Size: outer, Overflow: geom.Sz(
		clampLow(size.W-outer.W), clampLow(size.H-outer.H))}
}

// layoutWindow is the overlay algorithm of a [Window] and of every [ZStack]
// that holds an on-screen keyboard. Two things are decided here that the plain
// overlay does not know about, and both are the same move: some children are
// laid out in a smaller rectangle than the box, and the rest keep the box.
//
//   - The safe area. A Window laid out against the whole window — the root of
//     the tree — measures and places every child but its background plate
//     inside the window without the safe area of [gift.App.SetSafeArea], so
//     that nothing is drawn under the status bar, the navigation bar or the
//     cutout of a phone. The plate keeps the whole window and the page is
//     painted behind the bars.
//   - The keyboard. The keyboard is laid out against that safe rectangle, and
//     every other child in the band of it the keyboard leaves free. It is the
//     "adjust resize" of the mobile platforms, and [Overlay.AvoidKeyboard]
//     says why it is the default.
//
// # The algorithm
//
// The keyboard is measured first, with exactly the constraints the other
// children get before it reserves anything, because its height is the one
// number the rest depends on. A hidden keyboard measures zero — see
// [kbHidden] — and a window without a safe area has an inset of zero, and
// with both at zero nothing below differs from [layout.Overlay] by a single
// pixel: the constraints, the size and the origins are the same expressions
// with zeroes added. That is what keeps every screen of a desktop or kiosk
// application without a focused field, and every golden taken of one, exactly
// as it was.
//
// With a safe area, the children other than the plate are measured with the
// constraints of the box deflated by it and placed with its top and left edge
// added, which is padding by another name; the keyboard among them, so it sits
// on the safe bottom edge, above the navigation bar of a phone and not behind
// it. With a keyboard up, the others are measured with the maximum height of
// that safe rectangle lowered by the keyboard's, and aligned inside the band
// between the keyboard and the opposite edge rather than inside the whole box.
// A flexible layout therefore re-flows into the smaller band — a scroll
// container gets a shorter viewport, a control pinned to the bottom of a
// screen moves up to sit on the keyboard — and a scroll reveal of the focused
// field needs no help, because there is nothing left to obstruct.
//
// The keyboard's reserve is counted inside the safe rectangle, and that is
// the rule "the larger of the two, not the sum" in another form: the bottom of
// a phone with its keyboard up loses the keyboard plus the navigation bar it
// sits on, which is exactly what is covered, and never the navigation bar
// twice.
//
// # When the safe area is not applied
//
// Only a Window, and only one given the whole window: the constraints it lays
// out against have the size of [gift.LayoutContext.Viewport] as their maximum.
// The root of a tree is laid out against exactly that. A Window nested in
// something smaller — a preview of a screen in a gallery, a test in a framed
// box, a second Window inside the first one, which is already in the safe
// rectangle — is not at the edge of the screen, or not all of it is, and
// insetting it as well would pad it twice. [Overlay.IgnoresSafeArea] turns it
// off for the one that is.
//
// # When nothing is reserved for the keyboard
//
// Three cases, and each falls back to plain overlap rather than to a guess:
//
//   - the height is unbounded, so there is no "remaining" height to hand out;
//   - the overlay's vertical alignment is neither the top nor the bottom edge.
//     A keyboard aligned to the middle would split the box into two bands, and
//     a child cannot be laid out into two bands;
//   - the keyboard is hidden, which is the zero reserve above.
//
// The window background of [Window] is the one child that keeps the whole box,
// so that the page is painted behind the keyboard and behind the bars too. A
// keyboard with a translucent or rounded panel would otherwise show a strip of
// unpainted framebuffer, which is exactly the defect [Window] exists to
// prevent; a phone would show one under its status bar.
//
// It allocates nothing: the scratch buffers are the node's, as for every other
// kind.
func (n *node) layoutWindow(ctx *gift.LayoutContext, cc geom.Constraints, k int) layout.Result {
	pad := n.spec.Padding
	align := n.spec.Alignment
	kb := -1
	if n.kb > 0 && n.kb <= k {
		kb = n.kb - 1
	}
	plate := n.plate && k > 0
	inner := cc.Deflate(pad).Loosen()

	// The safe area applies to a Window that fills the window and nowhere
	// else; see "When the safe area is not applied" above. Zero is the
	// desktop, and every expression below then reduces to the plain overlay.
	var s geom.Insets
	if plate && !n.ignoreSafeArea && cc.Max == ctx.Viewport() {
		s = ctx.SafeArea()
	}
	safe := inner.Deflate(s)

	var kbs geom.Size
	if kb >= 0 {
		kbs = n.MeasureChild(kb, safe)
		n.items[kb].Size = kbs
	}

	var reserve float32
	if kb >= 0 && safe.HasBoundedHeight() && (align.Y == 0 || align.Y == 1) && kbs.H > 0 {
		reserve = kbs.H
	}
	rest := safe
	rest.Max.H = clampLow(safe.Max.H - reserve)

	// What every child needs of the inside of the box to be shown whole: its
	// own size, plus the safe area for everything but the plate, plus the
	// keyboard for everything in the band.
	w, h := kbs.W, kbs.H
	if kb >= 0 {
		w, h = kbs.W+s.Horizontal(), kbs.H+s.Vertical()
	}
	for i := range k {
		if i == kb {
			continue
		}
		fill := plate && i == 0
		c := rest
		if fill {
			c = inner
		}
		sz := n.MeasureChild(i, c)
		n.items[i].Size = sz
		ew, eh := sz.W, sz.H
		if !fill {
			ew += s.Horizontal()
			eh += reserve + s.Vertical()
		}
		if ew > w {
			w = ew
		}
		if eh > h {
			h = eh
		}
	}

	content := geom.Sz(w, h).Outset(pad)
	outer := cc.Constrain(content)
	inside := geom.Sz(
		clampLow(outer.W-pad.Horizontal()),
		clampLow(outer.H-pad.Vertical()),
	)
	// The safe rectangle inside the padding, and in it the band the keyboard
	// leaves free: above it when it sits on the bottom edge, below it when it
	// sits on the top one. With no safe area and no reserve both are the
	// whole inside, which is the plain overlay.
	safeX, safeY := pad.Left+s.Left, pad.Top+s.Top
	safeW := clampLow(inside.W - s.Horizontal())
	safeH := clampLow(inside.H - s.Vertical())
	bandTop := safeY
	if align.Y == 0 {
		bandTop += reserve
	}
	bandH := clampLow(safeH - reserve)
	for i := range k {
		sz := n.items[i].Size
		switch {
		case plate && i == 0:
			n.origins[i] = geom.Pt(
				pad.Left+(inside.W-sz.W)*align.X,
				pad.Top+(inside.H-sz.H)*align.Y)
		case i == kb:
			n.origins[i] = geom.Pt(
				safeX+(safeW-sz.W)*align.X,
				safeY+(safeH-sz.H)*align.Y)
		default:
			n.origins[i] = geom.Pt(
				safeX+(safeW-sz.W)*align.X,
				bandTop+(bandH-sz.H)*align.Y)
		}
	}
	return layout.Result{Size: outer, Overflow: geom.Sz(
		overflowBy(content.W, outer.W), overflowBy(content.H, outer.H))}
}

// overflowBy is how far content exceeds permitted, zero when it does not and
// zero for an unbounded or NaN difference, which is the rule of the excess in
// internal/layout: an infinity must not travel into the diagnostics.
func overflowBy(content, permitted float32) float32 {
	d := content - permitted
	if !(d > 0) || !isFinite(d) {
		return 0
	}
	return d
}

// clampLow returns v or zero, whichever is larger.
func clampLow(v float32) float32 {
	if v > 0 {
		return v
	}
	return 0
}

// mainExtent returns the component of s along ax.
func mainExtent(ax layout.Axis, s geom.Size) float32 {
	if ax == layout.Horizontal {
		return s.W
	}
	return s.H
}

// element builds the gift.Element of a container view.
func element(b base, kind nodeKind, gap float32, axis layout.Axis, cross layout.CrossAlign, children []gift.View) gift.Element {
	// The one place a container reads the theme; see [styleSpec.resolved].
	b.style = b.style.resolved()
	n := &node{
		kind: kind,
		spec: layout.StackSpec{Axis: axis, Gap: gap, Padding: b.pad, Alignment: b.align, CrossAlign: cross},
		fr:   b.frame,
		st:   b.style,
	}
	var p gift.Painter
	if b.style.needsPainter() {
		p = n
	}
	return gift.Element{
		Key:      b.key,
		Flex:     b.flex,
		Layouter: n,
		Painter:  p,
		Children: children,
		// The paint clip and the input clip come from this one field. The
		// painter pushes the full bounds (see paintStyle) and gift clips hit
		// testing to the same rectangle, so a clipped child cannot be
		// invisible and clickable at the same time. The paint clip has the
		// corners of the background.
		Clip:       b.style.clip,
		ClipRadius: b.style.radius,
	}
}
