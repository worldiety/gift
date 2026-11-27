package gift

import (
	"strconv"
	"time"

	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/internal/scene"
)

// This file is the top layer: views that a node presents above everything else
// in the window — a popover, a menu, a context menu — wherever in the tree the
// node is.
//
// # Why the core has to do this
//
// A presentation that is an ordinary child of its presenter cannot work. The
// presenter of a menu is a button in a toolbar inside a scroll view inside a
// tab: the scroll view clips it, every sibling painted after the button paints
// over it, and a tap outside it lands on whatever is there instead of closing
// it. [ui.ModalView] solves this by wrapping the screen it covers, which is the
// right shape for an alert the application raises and the wrong one for a menu
// a control raises: the control would have to reach up to the screen.
//
// So the root owns a second child after the application's own root, the top
// layer, and every node that declares [Element.Overlay] has its view mounted
// there, full window, painted after everything and hit tested before it. The
// declaration stays with the presenter — it is rebuilt when the presenter is
// rebuilt and it goes when the presenter goes — which is SwiftUI's
// .popover(isPresented:) in shape: a property of a view, not a call.

// overlayEntry is one presentation: the node that declares it, its view, the
// key its host is reconciled under, and when it began to leave.
type overlayEntry struct {
	owner scene.Handle
	view  View
	key   string
	exit  time.Duration
	// leaving is the time the presenter withdrew the overlay, or zero while
	// it is presented.
	leaving time.Duration
}

// topLayer is the view of the top layer: one host per presentation.
type topLayer struct{ a *App }

var (
	topLayerType   = RegisterType("gift.topLayer")
	overlayHostTyp = RegisterType("gift.overlayHost")
)

func (topLayer) ViewType() TypeID { return topLayerType }

func (t topLayer) Build(*BuildContext) Element {
	return Element{Key: "gift.top", Layouter: fillLayout{}, Children: t.a.topViews()}
}

// overlayHost is the full window node an overlay is mounted under. Its
// layouter carries what the core knows about the presentation, which is how a
// node inside the overlay finds its presenter; see [LayoutContext.Presenter].
type overlayHost struct {
	key     string
	owner   scene.Handle
	view    View
	leaving time.Duration
	exit    time.Duration
}

func (overlayHost) ViewType() TypeID { return overlayHostTyp }

func (h overlayHost) Build(*BuildContext) Element {
	return Element{
		Key:      h.key,
		Layouter: &overlayHostNode{owner: h.owner, leaving: h.leaving, exit: h.exit},
		Children: []View{h.view},
	}
}

// overlayHostNode lays its overlay out over the whole window and remembers the
// presenter rectangle the overlay saw, so that a layout in which the presenter
// moved can be corrected in the same update; see [App.settleOverlays].
type overlayHostNode struct {
	owner   scene.Handle
	leaving time.Duration
	exit    time.Duration
	seen    geom.Rect
}

// leavingOverlay reports whether nd hosts an overlay on its way out, which is
// still drawn and no longer takes input; see [Element.OverlayExit].
func leavingOverlay(nd *nodeData) bool {
	host, ok := nd.layouter.(*overlayHostNode)
	return ok && host.leaving != 0
}

func (o *overlayHostNode) Layout(ctx *LayoutContext, c geom.Constraints) geom.Size {
	s := c.Constrain(c.Max)
	if ctx.ChildCount() > 0 {
		ctx.Measure(0, geom.Tight(s))
		ctx.Place(0, geom.Point{})
	}
	return s
}

// fillLayout gives every child the whole of its own size.
type fillLayout struct{}

func (fillLayout) Layout(ctx *LayoutContext, c geom.Constraints) geom.Size {
	s := c.Constrain(c.Max)
	for i := range ctx.ChildCount() {
		ctx.Measure(i, geom.Tight(s))
		ctx.Place(i, geom.Point{})
	}
	return s
}

// rootLayout is the layouter of the root node: the application's root as a
// component lays it out, and the top layer over the whole viewport.
type rootLayout struct{}

func (rootLayout) Layout(ctx *LayoutContext, c geom.Constraints) geom.Size {
	if ctx.ChildCount() == 0 {
		return c.Constrain(geom.Size{})
	}
	sz := ctx.Measure(0, c)
	ctx.Place(0, geom.Point{})
	if ctx.ChildCount() > 1 {
		ctx.Measure(1, geom.Tight(c.Max))
		ctx.Place(1, geom.Point{})
	}
	return sz
}

func (rootLayout) Paint(ctx *PaintContext) { ctx.PaintChildren() }

// topViews returns the hosts of the presentations, in the order they were
// presented, so that a later one is painted over an earlier one.
func (a *App) topViews() []View {
	a.topBuf = a.topBuf[:0]
	for _, e := range a.overlays {
		a.topBuf = append(a.topBuf, overlayHost{key: e.key, owner: e.owner, view: e.view, leaving: e.leaving, exit: e.exit})
	}
	return a.topBuf
}

// declareOverlay records what the element of node h declares: an overlay, or
// none. A withdrawn overlay with an exit time stays, without input, for that
// long; see [Element.OverlayExit].
func (a *App) declareOverlay(h scene.Handle, nd *nodeData, v View, exit time.Duration, hidden bool) {
	if v == nil && !nd.presents {
		return
	}
	i := a.overlayIndex(h)
	if v != nil && !hidden {
		nd.presents = true
		if i < 0 {
			a.overlays = append(a.overlays, overlayEntry{
				owner: h, view: v, exit: exit,
				key: "overlay:" + strconv.FormatUint(uint64(h.Key()), 10),
			})
		} else {
			e := &a.overlays[i]
			e.view, e.exit, e.leaving = v, exit, 0
		}
		a.topDirty = true
		return
	}
	nd.presents = false
	if i < 0 {
		return
	}
	a.withdrawOverlay(i, !hidden)
}

// withdrawOverlay ends presentation i: at once, or after its exit time when
// animate is set and it has one.
func (a *App) withdrawOverlay(i int, animate bool) {
	e := &a.overlays[i]
	if animate && e.exit > 0 {
		if e.leaving == 0 {
			e.leaving = max(a.in.now, 1)
			a.topDirty = true
			if a.topNode.IsZero() {
				return
			}
			a.animate(a.topNode, e.exit+time.Millisecond)
		}
		return
	}
	a.overlays = append(a.overlays[:i], a.overlays[i+1:]...)
	a.topDirty = true
}

func (a *App) overlayIndex(h scene.Handle) int {
	for i := range a.overlays {
		if a.overlays[i].owner == h {
			return i
		}
	}
	return -1
}

// forgetOverlay drops the presentation of a node that is being unmounted, at
// once: the presenter is gone and so is anything that could animate it out.
func (a *App) forgetOverlay(h scene.Handle) {
	if i := a.overlayIndex(h); i >= 0 {
		a.overlays = append(a.overlays[:i], a.overlays[i+1:]...)
		a.topDirty = true
	}
}

// hideOverlaysIn withdraws, at once, the presentations of every node in the
// subtree at h, which is becoming hidden: an inactive tab does not keep a menu
// open over the active one.
func (a *App) hideOverlaysIn(h scene.Handle) {
	for i := 0; i < len(a.overlays); {
		if o := a.overlays[i].owner; a.store.Valid(o) && a.isAncestor(h, o) {
			a.data(o).presents = false
			a.overlays = append(a.overlays[:i], a.overlays[i+1:]...)
			a.topDirty = true
			continue
		}
		i++
	}
}

// tickOverlays ends the presentations whose exit time is over.
func (a *App) tickOverlays(now time.Duration) {
	for i := 0; i < len(a.overlays); {
		if e := a.overlays[i]; e.leaving != 0 && now-e.leaving >= e.exit {
			a.overlays = append(a.overlays[:i], a.overlays[i+1:]...)
			a.topDirty = true
			continue
		}
		i++
	}
}

// reconcileTop brings the top layer up to date with the presentations. The
// layer exists only while something is presented, so an application that
// presents nothing has not one node more than it builds.
func (a *App) reconcileTop() {
	if !a.topDirty || a.root == nil || !a.store.Valid(a.root.node) {
		return
	}
	a.topDirty = false
	root := a.data(a.root.node)
	if len(root.children) == 0 {
		// Not built yet; the first build of the root reconciles the layer.
		return
	}
	switch {
	case len(a.overlays) == 0 && !a.topNode.IsZero():
		a.unmount(a.root.node, a.topNode)
		root.children = root.children[:1]
		a.store.SetChildren(a.root.node, root.children)
		a.topNode = scene.Handle{}
		a.markNeedsLayout(a.root.node)
	case len(a.overlays) > 0 && a.topNode.IsZero():
		h := a.mountChild(a.root.node, a.describe(topLayer{a: a}), a.root)
		root.children = append(root.children[:1], h)
		a.store.SetChildren(a.root.node, root.children)
		a.topNode = h
		a.markNeedsLayout(a.root.node)
	case len(a.overlays) > 0:
		nd := a.data(a.topNode)
		views := a.topViews()
		nd.childViews = views
		a.reconcileChildren(a.topNode, nd, views, a.root)
		a.markNeedsLayout(a.topNode)
	}
}

// settleOverlays lays the top layer out again when a presenter moved in the
// layout that just ran: an overlay is placed against where its presenter was
// when the overlay was laid out, and in a layout that also moved the presenter
// that was the previous frame's rectangle.
func (a *App) settleOverlays() bool {
	if len(a.overlays) == 0 || a.topNode.IsZero() || !a.store.Valid(a.topNode) {
		return false
	}
	moved := false
	for _, c := range a.data(a.topNode).children {
		host, ok := a.data(c).layouter.(*overlayHostNode)
		if !ok || !a.store.Valid(host.owner) {
			continue
		}
		if r := a.NodeDeviceBounds(NodeRef{host.owner}); r != host.seen {
			a.markNeedsLayout(c)
			a.markSubtreeForLayout(c, 0)
			moved = true
		}
	}
	return moved
}

// markSubtreeForLayout marks every node below h for layout, because what
// changed for them — where their presenter is — is not a constraint and would
// not reach them through the usual cache.
func (a *App) markSubtreeForLayout(h scene.Handle, depth int) {
	if depth > scene.MaxDepth {
		return
	}
	for _, c := range a.data(h).children {
		a.markNeedsLayout(c)
		a.markSubtreeForLayout(c, depth+1)
	}
}

// hostOf returns the overlay host the node h is inside, or nil.
func (a *App) hostOf(h scene.Handle) *overlayHostNode {
	for depth := 0; !h.IsZero() && a.store.Valid(h); depth++ {
		if depth > scene.MaxDepth {
			return nil
		}
		n := a.store.Get(h)
		if host, ok := n.Payload.layouter.(*overlayHostNode); ok {
			return host
		}
		h = n.Parent
	}
	return nil
}

// presenter is the body of the Presenter methods.
func (a *App) presenter(h scene.Handle) (geom.Rect, bool) {
	host := a.hostOf(h)
	if host == nil || !a.store.Valid(host.owner) {
		return geom.Rect{}, false
	}
	r := a.NodeDeviceBounds(NodeRef{host.owner})
	host.seen = r
	return r, true
}

// Presenter returns the rectangle, in device space, of the node that presents
// the overlay the node being laid out is in, and whether it is in one; see
// [Element.Overlay]. A popover places itself against it.
//
// The top layer is laid out after the application, so the rectangle is the
// presenter's in this frame; when the presenter moved in the very layout that
// is running, the overlay is laid out once more in the same update with the
// new rectangle.
func (l *LayoutContext) Presenter() (geom.Rect, bool) { return l.app.presenter(l.node) }

// Presenter is [LayoutContext.Presenter] for the node being painted.
func (p *PaintContext) Presenter() (geom.Rect, bool) { return p.app.presenter(p.cur) }

// Presenter is [LayoutContext.Presenter] for the node receiving the event.
func (c *EventContext) Presenter() (geom.Rect, bool) { return c.app.presenter(c.cur) }

// OverlayLeaving reports how far the overlay the node being painted is in has
// gone on its way out, from zero to one, and whether it is leaving at all: a
// withdrawn overlay stays on the screen for [Element.OverlayExit] so that it
// can animate away, and takes no input meanwhile.
func (p *PaintContext) OverlayLeaving() (float32, bool) {
	host := p.app.hostOf(p.cur)
	if host == nil || host.leaving == 0 || host.exit <= 0 {
		return 0, false
	}
	f := float32(p.app.in.now-host.leaving) / float32(host.exit)
	return min(max(f, 0), 1), true
}
