package ui

import (
	"math"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

var (
	popoverType = gift.RegisterType("ui.Popover")
	panelType   = gift.RegisterType("ui.panel")
)

// Metrics of a popover and a menu, in logical pixels: the ones of iOS.
const (
	// panelMargin keeps a panel off the edges of the window.
	panelMargin = float32(8)
	// panelGap is the distance between the anchor and a panel without an
	// arrow, and panelArrow the length of an arrow.
	panelGap   = float32(6)
	panelArrow = float32(10)
	// panelRadius is the corner radius of the card.
	panelRadius = float32(13)
	// panelExit is how long a withdrawn panel takes to shrink back into its
	// anchor; see [gift.Element.OverlayExit].
	panelExit = 140 * time.Millisecond
	// panelAppear bounds the spring a panel grows out of its anchor with.
	panelAppear = 500 * time.Millisecond
)

// PopoverEdge says on which side of its anchor a popover appears.
type PopoverEdge uint8

const (
	// EdgeAuto puts the popover below its anchor when it fits there, above
	// when it fits there instead, and on the larger side otherwise.
	EdgeAuto PopoverEdge = iota
	// EdgeBelow and EdgeAbove prefer one side, and still take the other one
	// when only that fits.
	EdgeBelow
	EdgeAbove
	// EdgeLeading and EdgeTrailing put the popover beside its anchor.
	EdgeLeading
	EdgeTrailing
)

// panelAlign is how a panel lines up with its anchor along the other axis.
type panelAlign uint8

const (
	alignCentre panelAlign = iota
	// alignLeading lines the panel up with the leading edge of the anchor,
	// as a menu of a button does, and with the trailing edge when that is
	// the side with the room.
	alignLeading
)

// panel is the overlay a popover and a menu are drawn in: a card placed
// against the presenter, with an optional arrow, over a scrim that closes it
// when it is tapped. It grows out of its anchor with a spring and shrinks back
// into it when it is withdrawn.
type panel struct {
	content    gift.View
	edge       PopoverEdge
	align      panelAlign
	arrow      bool
	point      geom.Point // an anchor point instead of the presenter
	hasPoint   bool
	background Color
	scrim      Color
	spotlight  bool // the scrim leaves the presenter uncovered
	holeRadius float32
	maxWidth   float32
	padding    float32
	label      string
	onDismiss  func()
	onKey      func(gift.Event) bool
}

func (panel) ViewType() gift.TypeID { return panelType }

func (p panel) Build(*gift.BuildContext) gift.Element {
	n := &panelNode{p: p}
	return gift.Element{
		Key:         "ui.panel",
		Label:       p.label,
		Layouter:    n,
		Painter:     n,
		Interactor:  n,
		Children:    []gift.View{p.content},
		Focusable:   p.onKey != nil,
		KeyFallback: true,
		FocusTrap:   true,
	}
}

// panelState is what a panel keeps across builds: when it appeared.
type panelState struct{ since time.Duration }

type panelNode struct {
	p panel
	// card is the card's rectangle and anchor the presenter's, both in the
	// panel's own space, which is the window's; side is where the card ended
	// up and tip the point the arrow touches, which is also the point the
	// card grows out of.
	card, anchor geom.Rect
	side         PopoverEdge
	tip          geom.Point
}

// Layout places the card against its anchor inside the safe rectangle of the
// window, [safeWindow], rather than inside the window itself. The panel node
// covers the whole window — its scrim and its dismissing press do — but on a
// phone drawn edge to edge the window includes the status bar, the navigation
// bar and, on its side, the cutout of the camera, and a menu that opened
// under the clock or behind the home indicator would have rows nobody can
// read or tap. So every room, every clamp and the widest card are measured
// from the edges of that rectangle: a menu of a button just above a tab bar
// that does not fit below flips above, and one that fits on neither side is
// clamped to stop panelMargin short of the navigation bar, not of the glass.
// Without a safe area the rectangle is the window, and the arithmetic is the
// one it always was.
func (n *panelNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	win := c.Constrain(c.Max)
	r := safeWindow(ctx, win)
	if st := ctx.Retained[panelState](); st.since == 0 {
		st.since = max(ctx.Now(), 1)
		ctx.Animate(panelAppear)
	}
	a, ok := ctx.Presenter()
	if n.p.hasPoint || !ok {
		a = geom.Rc(n.p.point.X, n.p.point.Y, n.p.point.X, n.p.point.Y)
	}
	n.anchor = a
	gap := panelGap
	if n.p.arrow {
		gap = panelArrow
	}
	pad := n.p.padding
	maxW := min(n.p.maxWidth, r.Width()-2*panelMargin)
	room := [...]float32{
		EdgeBelow:    r.Max.Y - panelMargin - (a.Max.Y + gap),
		EdgeAbove:    a.Min.Y - gap - (r.Min.Y + panelMargin),
		EdgeLeading:  a.Min.X - gap - (r.Min.X + panelMargin),
		EdgeTrailing: r.Max.X - panelMargin - (a.Max.X + gap),
	}
	vertical := n.p.edge == EdgeAuto || n.p.edge == EdgeBelow || n.p.edge == EdgeAbove
	measure := func(w, h float32) geom.Size {
		in := ctx.Measure(0, geom.Constraints{Max: geom.Sz(max(0, w-2*pad), max(0, h-2*pad))})
		return geom.Sz(in.W+2*pad, in.H+2*pad)
	}
	var s geom.Size
	side := n.p.edge
	if vertical {
		s = measure(maxW, max(room[EdgeBelow], room[EdgeAbove]))
		first, second := EdgeBelow, EdgeAbove
		if side == EdgeAbove {
			first, second = EdgeAbove, EdgeBelow
		}
		switch {
		case s.H <= room[first]:
			side = first
		case s.H <= room[second]:
			side = second
		case room[first] >= room[second]:
			side = first
		default:
			side = second
		}
		if s.H > room[side] {
			s = measure(maxW, room[side])
		}
	} else {
		other := EdgeTrailing
		if side == EdgeTrailing {
			other = EdgeLeading
		}
		if room[side] < room[other] && room[side] < min(maxW, 200) {
			side = other
		}
		s = measure(min(maxW, room[side]), r.Height()-2*panelMargin)
	}

	var x, y float32
	cx, cy := (a.Min.X+a.Max.X)/2, (a.Min.Y+a.Max.Y)/2
	switch side {
	case EdgeBelow, EdgeAbove:
		x = cx - s.W/2
		if n.p.align == alignLeading {
			x = a.Min.X
			if x+s.W > r.Max.X-panelMargin {
				x = a.Max.X - s.W
			}
		}
		x = clampf(x, r.Min.X+panelMargin, r.Max.X-panelMargin-s.W)
		y = a.Max.Y + gap
		if side == EdgeAbove {
			y = a.Min.Y - gap - s.H
		}
		y = clampf(y, r.Min.Y+panelMargin, r.Max.Y-panelMargin-s.H)
	default:
		y = clampf(cy-s.H/2, r.Min.Y+panelMargin, r.Max.Y-panelMargin-s.H)
		x = a.Max.X + gap
		if side == EdgeLeading {
			x = a.Min.X - gap - s.W
		}
		x = clampf(x, r.Min.X+panelMargin, r.Max.X-panelMargin-s.W)
	}
	n.card = geom.Rc(x, y, x+s.W, y+s.H)
	n.side = side
	inset := panelRadius + panelArrow
	switch side {
	case EdgeBelow:
		n.tip = geom.Pt(clampf(cx, x+inset, x+s.W-inset), y-gap)
	case EdgeAbove:
		n.tip = geom.Pt(clampf(cx, x+inset, x+s.W-inset), y+s.H+gap)
	case EdgeLeading:
		n.tip = geom.Pt(x+s.W+gap, clampf(cy, y+inset, y+s.H-inset))
	default:
		n.tip = geom.Pt(x-gap, clampf(cy, y+inset, y+s.H-inset))
	}
	if !n.p.arrow && n.p.align == alignLeading {
		// A menu grows out of the corner nearest its anchor.
		n.tip = geom.Pt(x, n.tip.Y)
		if x+s.W/2 < cx {
			n.tip.X = x + s.W
		}
	}
	ctx.Place(0, geom.Pt(x+pad, y+pad))
	ctx.ReportOverflow(geom.Size{})
	return win
}

// progress is how far the panel has grown out of its anchor at now: a spring
// with a trace of overshoot on the way in, and an ease back on the way out.
func (n *panelNode) progress(ctx *gift.PaintContext) (f float32, moving bool) {
	if leave, leaving := ctx.OverlayLeaving(); leaving {
		return (1 - leave) * (1 - leave), true
	}
	st := ctx.Retained[panelState]()
	t := (ctx.Now() - st.since).Seconds()
	if st.since == 0 || t >= panelAppear.Seconds() {
		return 1, false
	}
	// Underdamped at a response of 0.32 s and a damping of 0.8: x(t) of a
	// spring from 0 to 1, in closed form.
	const z = 0.8
	w := 2 * math.Pi / 0.32
	wd := w * math.Sqrt(1-z*z)
	e := math.Exp(-z * w * t)
	x := 1 - e*(math.Cos(wd*t)+z*w/wd*math.Sin(wd*t))
	return float32(x), true
}

func (n *panelNode) Paint(ctx *gift.PaintContext) {
	b := ctx.Bounds()
	toDevice := deviceMapping(ctx)
	f, moving := n.progress(ctx)
	alpha := clampf(f, 0, 1)
	if !n.p.scrim.IsTransparent() {
		// The scrim is the panel's own bounds, and those are the whole
		// window, the system bars included: it dims the status bar and the
		// navigation bar with the rest of the screen, as the scrim of a modal
		// does, without a bleed of its own. Only the card keeps to the safe
		// rectangle; see [panelNode.Layout].
		sc := scaleColor(n.p.scrim, alpha)
		if n.p.spotlight {
			// A ring so wide that it covers the window, whose inner contour
			// is the presenter with its corner radius: the scrim with a
			// rounded hole, out of one stroke.
			w := 2 * (b.Width() + b.Height())
			hole := n.anchor
			ctx.Add(render.Op{
				Kind: render.OpStrokeRoundRect, Bounds: hole.Outset(geom.InsetsAll(w)),
				Color: sc, CornerRadius: n.p.holeRadius + w, StrokeWidth: w,
			})
		} else {
			ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: b, Color: sc})
		}
	}
	// Grown out of the tip: scaled about it from a quarter to full size.
	s := 0.25 + 0.75*f
	o := n.tip
	ctx.PushTransform(geom.Translate(geom.Pt(-o.X, -o.Y)).Mul(geom.Scale(s, s)).Mul(geom.Translate(o)))
	card := n.card
	ctx.Add(render.Op{Kind: render.OpShadow, Bounds: card.Translate(geom.Pt(0, 6)), Color: scaleColor(RGBA(0, 0, 0, 70), alpha), CornerRadius: panelRadius, Blur: 24})
	bg := scaleColor(n.p.background, alpha)
	if n.p.arrow {
		n.paintArrow(ctx, bg)
	}
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: card, Color: bg, CornerRadius: panelRadius})
	// Clips are in device space and do not follow the transform: the clip
	// is the card as the transform shows it.
	shown := geom.Rc(o.X+(card.Min.X-o.X)*s, o.Y+(card.Min.Y-o.Y)*s, o.X+(card.Max.X-o.X)*s, o.Y+(card.Max.Y-o.Y)*s)
	ctx.PushClip(toDevice(shown))
	ctx.PaintChildren()
	ctx.PopClip()
	ctx.PopTransform()
	if moving {
		ctx.Animate(50 * time.Millisecond)
	}
}

// paintArrow draws the arrow as a square turned by 45 degrees, centred on the
// card's edge, so that the card covers its inner half: a triangle panelArrow
// tall and twice that wide, without a triangle primitive.
func (n *panelNode) paintArrow(ctx *gift.PaintContext, c Color) {
	t := n.tip
	var centre geom.Point
	switch n.side {
	case EdgeBelow:
		centre = geom.Pt(t.X, t.Y+panelArrow)
	case EdgeAbove:
		centre = geom.Pt(t.X, t.Y-panelArrow)
	case EdgeLeading:
		centre = geom.Pt(t.X-panelArrow, t.Y)
	default:
		centre = geom.Pt(t.X+panelArrow, t.Y)
	}
	half := panelArrow * math.Sqrt2 / 2
	ctx.PushTransform(geom.Translate(geom.Pt(-centre.X, -centre.Y)).Mul(geom.Rotate(math.Pi / 4)).Mul(geom.Translate(centre)))
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(centre.X-half, centre.Y-half, centre.X+half, centre.Y+half), Color: c, CornerRadius: 2})
	ctx.PopTransform()
}

// deviceRect is [deviceMapping] for one rectangle, without the closure, for
// a painter on the frame path, which must not allocate.
func deviceRect(ctx *gift.PaintContext, r geom.Rect) geom.Rect {
	b, db := ctx.Bounds(), ctx.DeviceBounds()
	sx, sy := float32(1), float32(1)
	if b.Width() > 0 {
		sx = db.Width() / b.Width()
	}
	if b.Height() > 0 {
		sy = db.Height() / b.Height()
	}
	return geom.Rc(db.Min.X+(r.Min.X-b.Min.X)*sx, db.Min.Y+(r.Min.Y-b.Min.Y)*sy,
		db.Min.X+(r.Max.X-b.Min.X)*sx, db.Min.Y+(r.Max.Y-b.Min.Y)*sy)
}

// deviceMapping returns the function that maps a rectangle in the local space
// of the node being painted into device space, the space clips are in: the
// two are a scale and an offset apart, the density and the scroll offsets
// above the node.
func deviceMapping(ctx *gift.PaintContext) func(geom.Rect) geom.Rect {
	b, db := ctx.Bounds(), ctx.DeviceBounds()
	sx, sy := float32(1), float32(1)
	if b.Width() > 0 {
		sx = db.Width() / b.Width()
	}
	if b.Height() > 0 {
		sy = db.Height() / b.Height()
	}
	return func(r geom.Rect) geom.Rect {
		return geom.Rc(db.Min.X+(r.Min.X-b.Min.X)*sx, db.Min.Y+(r.Min.Y-b.Min.Y)*sy,
			db.Min.X+(r.Max.X-b.Min.X)*sx, db.Min.Y+(r.Max.Y-b.Min.Y)*sy)
	}
}

// HandleEvent closes the panel on a press outside its card and on escape,
// and keeps every pointer event from reaching what is behind it.
func (n *panelNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	switch e.Kind {
	case gift.EventPointerDown, gift.EventContextMenu:
		if !n.card.Contains(e.Pos) && n.p.onDismiss != nil {
			n.p.onDismiss()
		}
		return true
	case gift.EventKeyDown:
		if e.Key == gift.KeyEscape && n.p.onDismiss != nil {
			n.p.onDismiss()
			return true
		}
		return n.p.onKey != nil && n.p.onKey(e)
	case gift.EventKeyUp, gift.EventRune:
		return n.p.onKey != nil && n.p.onKey(e)
	}
	return e.IsPointer()
}

// CanGoBack implements gift.BackHandler: an open panel — a popover, a menu, a
// context menu — takes the back action of the platform.
//
// It takes it without onDismiss too, and does nothing then, for the reason a
// press outside the card is swallowed without one: the panel is the top of
// what the user sees, and a back that passed through it would pop the screen
// under it, or leave the application, with the panel still open. A panel on
// its way out is no candidate; gift skips an overlay that is leaving, so back
// right after a dismissal reaches what is under it. The node exists only
// while the panel is presented, so there is nothing else to ask.
func (n *panelNode) CanGoBack() bool { return true }

// GoBack implements gift.BackHandler: it dismisses the panel, as escape does.
// A menu always has a dismissal, its own close.
func (n *panelNode) GoBack(*gift.EventContext) {
	if n.p.onDismiss != nil {
		n.p.onDismiss()
	}
}

// safeWindow is the rectangle a node of the top layer places what it shows
// in: the window of size win without the safe area of
// [gift.App.SetSafeArea]. The top layer is laid out against the whole window
// and stays so — its scrims cover the system bars, which is what dims them
// with the rest of the screen — but a toast, the card of a popover and an
// open menu are things to read and to touch, and those stay where the system
// draws nothing on top. A safe area larger than the window leaves a rectangle
// of no size in the middle rather than an inverted one; see [geom.Rect.Inset].
func safeWindow(ctx *gift.LayoutContext, win geom.Size) geom.Rect {
	return geom.Rc(0, 0, win.W, win.H).Inset(ctx.SafeArea())
}

func clampf(v, lo, hi float32) float32 {
	if hi < lo {
		return lo
	}
	return max(lo, min(hi, v))
}

// anchorLayout takes the size of its one child.
type anchorLayout struct{}

func (anchorLayout) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	if ctx.ChildCount() == 0 {
		return c.Constrain(geom.Size{})
	}
	s := ctx.Measure(0, c)
	ctx.Place(0, geom.Point{})
	return s
}

// --- popover -----------------------------------------------------------------

// PopoverView shows content in a card next to an anchor, above everything
// else in the window. It is created by [Popover].
type PopoverView struct {
	anchor, content gift.View
	presented       bool
	onDismiss       func()
	edge            PopoverEdge
	arrow           bool
	background      Color
	hasBackground   bool
	maxWidth        float32
	padding         float32
	key, label      string
	flex            float32
}

// Popover returns anchor with a popover of content that is shown while
// [PopoverView.Presented] is true.
//
//	open := ctx.State("info", false)
//	ui.Popover(
//		ui.Button(ui.Text("Info"), func() { open.Set(true) }),
//		ui.Text("Opening hours: 9 to 17"),
//	).Presented(ctx.Read(open)).OnDismiss(func() { open.Set(false) })
//
// The popover is a card with an arrow that points at the anchor, below it when
// it fits there and above otherwise, kept inside the window — inside the part
// of it the system bars of a phone leave free, [gift.App.SafeRect]. It grows
// out of the arrow with a spring when it appears and shrinks back when it is
// withdrawn, as on iOS. It lives in the top layer, so no clipping scroll view
// and no sibling hides it; see [gift.Element.Overlay].
//
// A press outside the card, or escape, calls [PopoverView.OnDismiss]; the
// application then withdraws the popover by presenting it no longer. The
// press itself goes nowhere else, so the first tap next to an open popover
// only closes it. While it is open, the keyboard focus is trapped inside it.
func Popover(anchor, content gift.View) PopoverView {
	return PopoverView{anchor: anchor, content: content, arrow: true, maxWidth: 320, padding: 16}
}

// Presented shows the popover while b is true.
func (v PopoverView) Presented(b bool) PopoverView { v.presented = b; return v }

// OnDismiss sets the function called on a press outside the popover and on
// escape. Without it the popover stays until the application withdraws it.
func (v PopoverView) OnDismiss(fn func()) PopoverView { v.onDismiss = fn; return v }

// Edge sets the preferred side; see [PopoverEdge].
func (v PopoverView) Edge(e PopoverEdge) PopoverView { v.edge = e; return v }

// Arrow sets whether the card points at its anchor. It does by default.
func (v PopoverView) Arrow(b bool) PopoverView { v.arrow = b; return v }

// Background sets the colour of the card. The default is [ColorSurface].
func (v PopoverView) Background(c Color) PopoverView {
	v.background, v.hasBackground = c, true
	return v
}

// MaxWidth caps the width of the card. The default is 320.
func (v PopoverView) MaxWidth(w float32) PopoverView { v.maxWidth = max(1, w); return v }

// Padding sets the space between the card and its content. The default is 16.
func (v PopoverView) Padding(f float32) PopoverView { v.padding = max(0, f); return v }

// Key sets the reconciliation key.
func (v PopoverView) Key(s string) PopoverView { v.key = s; return v }

// Flex sets the flex factor of the anchor in a stack.
func (v PopoverView) Flex(f float32) PopoverView { v.flex = f; return v }

// Label sets the accessible name of the popover.
func (v PopoverView) Label(s string) PopoverView { v.label = s; return v }

// ViewType implements gift.View.
func (PopoverView) ViewType() gift.TypeID { return popoverType }

// Build implements gift.View.
func (v PopoverView) Build(*gift.BuildContext) gift.Element {
	e := gift.Element{Key: v.key, Layouter: anchorLayout{}, Children: []gift.View{v.anchor}, Flex: v.flex}
	if v.presented && v.content != nil {
		bg := ColorSurface
		if v.hasBackground {
			bg = v.background
		}
		e.Overlay = panel{
			content:    v.content,
			edge:       v.edge,
			arrow:      v.arrow,
			background: ResolveColor(bg),
			maxWidth:   v.maxWidth,
			padding:    v.padding,
			label:      v.label,
			onDismiss:  v.onDismiss,
		}
		e.OverlayExit = panelExit
	}
	return e
}
