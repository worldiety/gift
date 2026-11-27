package imageviewer

import (
	"math"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/gesture"
	"github.com/worldiety/gift/ui/motion"
)

var (
	fullscreenType = gift.RegisterType("imageviewer.Fullscreen")
	canvasType     = gift.RegisterType("imageviewer.canvas")
	cardType       = gift.RegisterType("imageviewer.card")
	buttonType     = gift.RegisterType("imageviewer.button")
	emptyType      = gift.RegisterType("imageviewer.empty")
)

// The chrome of a full screen viewer.
const (
	// buttonSize is the touch target of a chrome button and buttonDisc the
	// disc drawn in it.
	buttonSize = float32(44)
	buttonDisc = float32(40)
	iconSize   = float32(24)
	// chromeMargin is the distance of the buttons from the edges.
	chromeMargin = float32(12)
	// arrowsLinger is how long the arrows stay after the mouse stopped, and
	// fadeIn and fadeOut how long they take to appear and to go.
	arrowsLinger = 1500 * time.Millisecond
	fadeIn       = 150 * time.Millisecond
	fadeOut      = 300 * time.Millisecond
	// chromeFade is how long the close button takes to follow a tap.
	chromeFade = 200 * time.Millisecond
	// cardMaxWidth bounds the card on a wide screen.
	cardMaxWidth = float32(640)
)

// FullscreenView is the view of a [Viewer]; see [Fullscreen].
type FullscreenView struct {
	v          *Viewer
	background ui.Color
	spacing    float32
	card       func(asset.ID) gift.View
	wheel      WheelMode
	doubleTap  float64
	maxZoom    float64
	controls   bool
	closable   bool
	key        string
}

// Fullscreen returns the full screen view of v. It belongs on top of
// everything else, the last child of a [ui.ZStack] that also holds the origin:
// it covers the whole stack while the viewer is open and is not there at all
// while it is closed.
//
// # What the user can do
//
//   - Swipe sideways to the previous and the next picture; the pages are
//     [FullscreenView.Spacing] apart, as in Photos. A flick turns the page
//     whatever the distance, a slow drag turns it past half way.
//   - Pinch to zoom about the fingers, with the rubber band and the bounce
//     of [Zoomable]; pan a zoomed picture, and pan on past its edge to turn
//     the page. Double tap to zoom in about the tap and out again.
//   - Drag the picture down to close: it follows the finger and shrinks,
//     the background fades, and it closes on a release past a third of the
//     way or on a downward flick. Pinch it below its fit to close as well.
//   - Swipe up to reveal the card of [FullscreenView.Card], and down to put
//     it away.
//   - Tap to show and hide the close button.
//   - With a mouse: move it to bring up the arrows at the sides, which fade
//     out when it rests; wheel to zoom about the cursor; drag like a finger;
//     double click to zoom.
//   - With keys: left and right turn the page, home and end go to the ends,
//     escape closes, plus, minus and zero zoom, space toggles the zoom, up and
//     down open and close the card.
//   - With Android's back button or back gesture: back puts the card away
//     when it is open and closes the viewer otherwise, as escape does; see
//     [gift.BackHandler].
//
// It opens out of the thumbnail the [Origin] reports and closes back into
// it, while the origin conceals the thumbnail; see [Viewer.SetOrigin].
func Fullscreen(v *Viewer) FullscreenView {
	return FullscreenView{v: v, background: ui.RGB(0, 0, 0), spacing: 20, wheel: WheelZooms, controls: true, closable: true}
}

// Background sets the colour behind the pictures. The default is black.
func (f FullscreenView) Background(c ui.Color) FullscreenView { f.background = c; return f }

// Spacing sets the gap between two pages. The default is 20.
func (f FullscreenView) Spacing(px float32) FullscreenView { f.spacing = max(0, px); return f }

// Card sets the content of the card a swipe up reveals below the picture:
// details, metadata, actions. It is built for the entry shown and rebuilt
// when the page turns. Nil, the default, has no card.
func (f FullscreenView) Card(fn func(asset.ID) gift.View) FullscreenView { f.card = fn; return f }

// Wheel sets what the wheel does. The default is [WheelZooms].
func (f FullscreenView) Wheel(m WheelMode) FullscreenView { f.wheel = m; return f }

// DoubleTapZoom sets the zoom of a double tap; see
// [ZoomableView.DoubleTapZoom].
func (f FullscreenView) DoubleTapZoom(z float64) FullscreenView { f.doubleTap = z; return f }

// MaxZoom sets the largest zoom; see [ZoomableView.MaxZoom].
func (f FullscreenView) MaxZoom(z float64) FullscreenView { f.maxZoom = z; return f }

// ShowsControls sets whether the close button and the arrows are drawn. The
// gestures and the keys work either way.
func (f FullscreenView) ShowsControls(b bool) FullscreenView { f.controls = b; return f }

// Closable sets whether the user can close the viewer with a gesture or a
// key; [Viewer.Close] works either way. A kiosk that is nothing but the
// viewer turns it off.
func (f FullscreenView) Closable(b bool) FullscreenView { f.closable = b; return f }

// Key sets the reconciliation key.
func (f FullscreenView) Key(s string) FullscreenView { f.key = s; return f }

// ViewType implements gift.View.
func (FullscreenView) ViewType() gift.TypeID { return fullscreenType }

// Build implements gift.View.
func (f FullscreenView) Build(*gift.BuildContext) gift.Element {
	key := "imageviewer"
	if f.key != "" {
		key = f.key
	}
	return gift.Element{
		Key:      f.key,
		Layouter: fill{},
		Children: []gift.View{gift.Component(key, f.body)},
	}
}

// body is the component of the viewer: nothing while it is closed, the canvas
// while it is not. The revision state is what the model bumps to rebuild it.
func (f FullscreenView) body(ctx *gift.Context) gift.View {
	rev := ctx.State("rev", uint64(0))
	ctx.Read(rev)
	v := f.v
	v.bump = func() { rev.Set(rev.Get() + 1) }
	if v.phase == closed {
		v.inval = nil
		return empty{}
	}
	v.spacing, v.closable, v.hasCard = float64(f.spacing), f.closable, f.card != nil
	var kids []gift.View
	if f.card != nil && v.has(v.index) {
		kids = append(kids, cardHost{v: v, content: f.card(v.id)})
	}
	if f.controls {
		kids = append(kids,
			button{v: v, role: roleClose},
			button{v: v, role: rolePrev},
			button{v: v, role: roleNext},
		)
	}
	return canvas{f: f, kids: kids}
}

// fill gives its one child everything.
type fill struct{}

func (fill) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	s := c.Constrain(c.Max)
	if ctx.ChildCount() > 0 {
		ctx.Measure(0, geom.Tight(s))
		ctx.Place(0, geom.Point{})
	}
	return s
}

// empty is the closed viewer: no size, no input.
type empty struct{}

func (empty) ViewType() gift.TypeID { return emptyType }
func (empty) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Layouter: fill{}}
}

// --- the canvas --------------------------------------------------------------

type canvas struct {
	f    FullscreenView
	kids []gift.View
}

func (canvas) ViewType() gift.TypeID { return canvasType }

func (c canvas) Build(*gift.BuildContext) gift.Element {
	n := &canvasNode{f: c.f, v: c.f.v}
	n.gs = []gesture.Gesture{
		gesture.Magnify().OnChanged(n.onMagnify).OnEnded(n.onMagnify).OnCancelled(n.onCancel),
		gesture.Drag().OnChanged(n.onDrag).OnEnded(n.onDragEnd).OnCancelled(n.onCancel),
		gesture.Tap().Count(2).OnEnded(n.onDoubleTap),
		gesture.Tap().OnEnded(n.onTap),
		gesture.Hover().OnChanged(n.onHover),
		gesture.Wheel().OnChanged(n.onWheel),
	}
	return gift.Element{
		Key:         "imageviewer",
		Label:       "Picture viewer",
		Layouter:    n,
		Painter:     n,
		Interactor:  n,
		Children:    c.kids,
		Focusable:   true,
		KeyFallback: true,
		FocusTrap:   true,
		MultiTouch:  true,
	}
}

type canvasNode struct {
	f   FullscreenView
	v   *Viewer
	gs  []gesture.Gesture
	now time.Duration
	ctx *gift.EventContext
}

func (n *canvasNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	v := n.v
	size := c.Constrain(c.Max)
	now := ctx.Now()
	v.inval = ctx.Invalidator()
	v.vw, v.vh, v.density = float64(size.W), float64(size.H), ctx.Density()
	v.sync()
	if v.askOpen {
		v.askOpen = false
		v.heroT0 = now
		ctx.Animate(heroDuration())
	}
	if v.askClose {
		v.askClose = false
		n.beginClose(now)
		ctx.After(heroDuration())
	}
	if v.askTurn != 0 && v.phase != closing {
		off, _ := v.pg.at(now)
		v.pg.settle(now)
		v.turn(now, v.askTurn, off, 0)
		v.askTurn = 0
		ctx.Animate(time.Second)
	}
	v.settleHero(now)
	if v.outDir != 0 {
		if _, moving := v.pg.at(now); !moving {
			v.outDir = 0
		}
	}

	// The pictures: the current one with the detail its zoom needs, its
	// neighbours ahead of time.
	z := v.zoom.z
	if v.zoom.mode == springing {
		z = v.zoom.to[0]
	}
	for d := -1; d <= 1; d++ {
		src := v.source(v.index + d)
		v.st[d+1] = requestStages(ctx, src, size.W, size.H, v.density, z, longestEdge(src), d != 0)
	}
	aspect := v.st[1].aspect()
	v.zoom.setViewport(v.vw, v.vh, aspect)
	v.zoom.maxZoom = maxZoom(n.f.maxZoom, longestEdge(v.source(v.index)), size, v.density, aspect)
	v.out.setViewport(v.vw, v.vh, v.out.aspect)

	// The children: the card's host, which places the card itself, then the
	// three buttons.
	k := 0
	if v.hasCard && k < ctx.ChildCount() {
		w := min(size.W, cardMaxWidth)
		h := v.cardHeight()
		ctx.Measure(k, geom.Tight(geom.Sz(w, h)))
		ctx.Place(k, geom.Pt((size.W-w)/2, size.H-h))
		k++
	}
	if n.f.controls {
		at := [3]geom.Point{
			{X: chromeMargin, Y: chromeMargin},
			{X: chromeMargin / 2, Y: (size.H - buttonSize) / 2},
			{X: size.W - buttonSize - chromeMargin/2, Y: (size.H - buttonSize) / 2},
		}
		for i := range 3 {
			if k >= ctx.ChildCount() {
				break
			}
			ctx.Measure(k, geom.Tight(geom.Sz(buttonSize, buttonSize)))
			ctx.Place(k, at[i])
			k++
		}
	}
	ctx.ReportOverflow(geom.Size{})
	return size
}

// cardHeight is the height of the card: half the screen, but not more than
// a comfortable panel.
func (v *Viewer) cardHeight() float32 { return float32(math.Min(v.vh*0.5, 420)) }

// fitRect is the rectangle a picture of the given aspect fits at in a
// viewport of w by h, centred.
func fitRect(w, h, aspect float64) geom.Rect {
	m := zoomer{vw: w, vh: h, aspect: aspect, z: 1}
	return m.rect(1, 0, 0)
}

// coverRect is the rectangle of a picture of the given aspect that covers r,
// centred on it: the picture as a cropping tile shows it.
func coverRect(r geom.Rect, aspect float64) geom.Rect {
	if aspect <= 0 {
		return r
	}
	w, h := float64(r.Width()), float64(r.Height())
	if w/h > aspect {
		h = w / aspect
	} else {
		w = h * aspect
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	return geom.Rc(cx-float32(w/2), cy-float32(h/2), cx+float32(w/2), cy+float32(h/2))
}

func lerpRect(a, b geom.Rect, f float64) geom.Rect {
	l := func(x, y float32) float32 { return float32(motion.Lerp(float64(x), float64(y), f)) }
	return geom.Rc(l(a.Min.X, b.Min.X), l(a.Min.Y, b.Min.Y), l(a.Max.X, b.Max.X), l(a.Max.Y, b.Max.Y))
}

// fade scales a premultiplied colour by f.
func fade(c ui.Color, f float64) ui.Color {
	k := float32(clamp(f, 0, 1))
	return ui.Color{R: c.R * k, G: c.G * k, B: c.B * k, A: c.A * k}
}

// pageRect is where page d, relative to the current one, is drawn at now:
// its zoom, the position of the pages and a swipe. The card does not move it;
// it is a sheet that slides over the picture.
//
// The pages that are not current are fitted with their own aspect, which a
// neighbour knows once its preview has arrived.
func (v *Viewer) pageRect(now time.Duration, d int, off float64, swipeT geom.Point) geom.Rect {
	var r geom.Rect
	switch {
	case d == 0:
		r = v.zoom.rectAt(now)
	case d == v.outDir:
		r = v.out.rectAt(now)
	default:
		r = fitRect(v.vw, v.vh, v.st[d+1].aspect())
	}
	r = r.Translate(geom.Pt(float32(float64(d)*v.pitch()-off), 0))
	if d == 0 && (v.sw.active || v.sw.back) {
		r = v.sw.apply(r, swipeT, v.vh)
	}
	return r
}

func (n *canvasNode) Paint(ctx *gift.PaintContext) {
	v := n.v
	now := ctx.Now()
	b := ctx.Bounds()
	db := ctx.DeviceBounds()
	// The thumbnail's frame is in device space; the pictures are laid out
	// in the viewport, with the origin at the canvas's top left corner.
	toViewport := func(r geom.Rect) geom.Rect { return r.Translate(geom.Pt(-db.Min.X, -db.Min.Y)) }
	off, pmoving := v.pg.at(now)
	swT, smoving := v.sw.at(now)
	cq, cmoving := v.cd.at(now)
	alpha := 1 - v.sw.progress(v.vh, swT)
	moving := pmoving || smoving || cmoving

	switch v.phase {
	case opening, closing:
		f, hmoving := v.heroProgress(now)
		moving = moving || hmoving
		n.paintHero(ctx, b, f, toViewport)
	default:
		ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: b, Color: fade(n.f.background, alpha)})
		_, _, _, zmoving := v.zoom.at(now)
		_, _, _, omoving := v.out.at(now)
		moving = moving || zmoving || v.outDir != 0 && omoving
		ctx.PushClip(db)
		// The current page first: it has the first claim on the frame's
		// upload budget. A neighbour that is not on screen is prepared
		// instead of drawn, so that its upload is done before a swipe shows
		// it — but only while nothing moves. Creating a texture the size of
		// the screen costs a Raspberry Pi 5 a frame of 40 ms however the
		// upload is spread, measured; in a frame of a page that is settling
		// that is a visible hitch, in a frame of a page at rest it is
		// nothing anybody can see.
		still := !moving && !v.dragging && !v.pinching
		for _, d := range [...]int{0, -1, 1} {
			if !v.has(v.index + d) {
				continue
			}
			r := v.pageRect(now, d, off, swT)
			if r.Max.X <= 0 || r.Min.X >= float32(v.vw) {
				if still {
					v.st[d+1].prepare(ctx)
				}
				continue
			}
			v.st[d+1].draw(ctx, r.Translate(b.Min), ui.OpaqueWhite, 0)
		}
		ctx.PopClip()
		if v.hasCard && cq > 0 {
			// The sheet: a panel with rounded top corners over the picture,
			// and the grabber that says it can be dragged.
			h := v.cardHeight()
			w := min(b.Width(), cardMaxWidth)
			x := b.Min.X + (b.Width()-w)/2
			y := b.Max.Y - float32(cq)*h
			ctx.Add(render.Op{Kind: render.OpShadow, Bounds: geom.Rc(x, y, x+w, b.Max.Y+16), Color: ui.RGBA(0, 0, 0, 90), CornerRadius: 16, Blur: 12})
			ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(x, y, x+w, b.Max.Y+16), Color: ui.RGBA(28, 28, 30, 245), CornerRadius: 16})
			cx := x + w/2
			ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(cx-18, y+6, cx+18, y+11), Color: ui.RGBA(255, 255, 255, 90), CornerRadius: 2.5})
		}
	}
	ctx.PaintChildren()
	if moving || n.chromeMoving(now) {
		ctx.Animate(100 * time.Millisecond)
	}
}

// paintHero draws the transition between the thumbnail and the screen.
func (n *canvasNode) paintHero(ctx *gift.PaintContext, b geom.Rect, f float64, toViewport func(geom.Rect) geom.Rect) {
	v := n.v
	st := v.st[1]
	aspect := st.aspect()
	full := fitRect(v.vw, v.vh, aspect)
	if v.phase == opening {
		bg := clamp(f, 0, 1)
		ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: b, Color: fade(n.f.background, bg)})
		if !v.heroHas {
			k := float32(0.94 + 0.06*bg)
			c := geom.Pt(float32(v.vw/2), float32(v.vh/2))
			r := geom.Rc(c.X+(full.Min.X-c.X)*k, c.Y+(full.Min.Y-c.Y)*k, c.X+(full.Max.X-c.X)*k, c.Y+(full.Max.Y-c.Y)*k)
			st.draw(ctx, r.Translate(b.Min), fade(ui.OpaqueWhite, bg), 0)
			return
		}
		from := toViewport(v.heroFrom)
		r := lerpRect(from, full, f)
		radius := float32(motion.Lerp(float64(v.heroR), 0, clamp(f, 0, 1)))
		n.drawCover(ctx, st, r.Translate(b.Min), radius)
		return
	}
	// Closing.
	bg := v.heroAlpha * (1 - clamp(f, 0, 1))
	ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: b, Color: fade(n.f.background, bg)})
	if !v.heroHas {
		k := float32(1 - 0.06*clamp(f, 0, 1))
		s := v.heroStart
		c := geom.Pt((s.Min.X+s.Max.X)/2, (s.Min.Y+s.Max.Y)/2)
		r := geom.Rc(c.X+(s.Min.X-c.X)*k, c.Y+(s.Min.Y-c.Y)*k, c.X+(s.Max.X-c.X)*k, c.Y+(s.Max.Y-c.Y)*k)
		st.draw(ctx, r.Translate(b.Min), fade(ui.OpaqueWhite, 1-clamp(f, 0, 1)), 0)
		return
	}
	to := toViewport(v.heroFrom)
	radius := float32(motion.Lerp(0, float64(v.heroR), clamp(f, 0, 1)))
	if v.heroClip {
		img := lerpRect(v.heroImage, coverRect(to, aspect), f)
		clip := lerpRect(geom.Rc(0, 0, float32(v.vw), float32(v.vh)), to, f)
		// Clips are in device space: the viewport rectangle scaled by the
		// density and moved to where the canvas is.
		b, db := ctx.Bounds(), ctx.DeviceBounds()
		k := db.Width() / max(b.Width(), 1)
		ctx.PushClip(geom.Rc(db.Min.X+clip.Min.X*k, db.Min.Y+clip.Min.Y*k, db.Min.X+clip.Max.X*k, db.Min.Y+clip.Max.Y*k))
		st.draw(ctx, img.Translate(b.Min), ui.OpaqueWhite, 0)
		ctx.PopClip()
		return
	}
	n.drawCover(ctx, st, lerpRect(v.heroStart, to, f).Translate(b.Min), radius)
}

// drawCover draws the sharpest stage that can be drawn cropped into r.
func (n *canvasNode) drawCover(ctx *gift.PaintContext, st stages, r geom.Rect, radius float32) {
	_ = st.detail.DrawCover(ctx, r, ui.OpaqueWhite, radius) ||
		st.screen.DrawCover(ctx, r, ui.OpaqueWhite, radius) ||
		st.preview.DrawCover(ctx, r, ui.OpaqueWhite, radius)
}

// beginClose starts the transition back from wherever the picture is now.
func (n *canvasNode) beginClose(now time.Duration) {
	v := n.v
	if v.phase == closed || v.phase == closing {
		return
	}
	v.settleHero(now)
	off, _ := v.pg.at(now)
	swT, _ := v.sw.at(now)
	r := v.pageRect(now, 0, off, swT)
	v.heroAlpha = 1 - v.sw.progress(v.vh, swT)
	v.heroClip = v.zoom.zoomed() && !v.sw.active
	v.heroImage = r
	v.heroStart = r
	if v.heroClip {
		v.heroStart = r.Intersect(geom.Rc(0, 0, float32(v.vw), float32(v.vh)))
	}
	v.heroHas = false
	if v.origin != nil {
		v.heroFrom, v.heroR, v.heroHas = v.origin.ThumbnailFrame(v.id)
	}
	v.phase, v.heroT0 = closing, now
	v.sw, v.mode, v.dragging, v.pinching = swipe{}, dragNone, false, false
}

// --- the chrome --------------------------------------------------------------

// chromeAlpha is the opacity of the close button at now: it follows a tap.
func (v *Viewer) chromeAlpha(now time.Duration) float64 {
	f := clamp(float64(now-v.chromeT0)/float64(chromeFade), 0, 1)
	if v.chromeT0 == 0 {
		f = 1
	}
	if v.chrome {
		return f
	}
	return 1 - f
}

// arrowsAlpha is the opacity of the arrows at now: they come with a moving
// mouse and go when it rests.
func (v *Viewer) arrowsAlpha(now time.Duration) float64 {
	if v.hoverAt == 0 && !v.hovering {
		return 0
	}
	in := clamp(float64(now-v.hoverFrom)/float64(fadeIn), 0, 1)
	out := 1 - clamp(float64(now-v.hoverAt-arrowsLinger)/float64(fadeOut), 0, 1)
	return math.Min(in, out)
}

func (n *canvasNode) chromeMoving(now time.Duration) bool {
	v := n.v
	if now-v.chromeT0 < chromeFade {
		return true
	}
	return v.hoverAt != 0 && now-v.hoverAt < arrowsLinger+fadeOut
}

type role uint8

const (
	roleClose role = iota
	rolePrev
	roleNext
)

// button is a chrome button: a dark disc with a white symbol, faded in and
// out by the viewer. A button that is not visible lets every event through to
// the picture behind it.
type button struct {
	v    *Viewer
	role role
}

func (button) ViewType() gift.TypeID { return buttonType }

func (b button) Build(*gift.BuildContext) gift.Element {
	n := &buttonNode{b: b}
	key, label := "viewer-close", "Close"
	switch b.role {
	case rolePrev:
		key, label = "viewer-previous", "Previous picture"
	case roleNext:
		key, label = "viewer-next", "Next picture"
	}
	return gift.Element{Key: key, Label: label, Layouter: n, Painter: n, Interactor: n}
}

type buttonNode struct{ b button }

func (n *buttonNode) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size {
	return c.Constrain(geom.Sz(buttonSize, buttonSize))
}

// alpha is the opacity of the button at now.
func (n *buttonNode) alpha(now time.Duration) float64 {
	v := n.b.v
	if v.phase != open && v.phase != opening || v.mode == dragSwipe || v.pinching {
		return 0
	}
	if f, _ := v.heroProgress(now); v.phase == opening {
		if a := clamp(f, 0, 1); a < 1 {
			return 0
		}
	}
	switch n.b.role {
	case roleClose:
		if !v.closable {
			return 0
		}
		return math.Max(v.chromeAlpha(now), v.arrowsAlpha(now))
	case rolePrev:
		if !v.has(v.index - 1) {
			return 0
		}
	case roleNext:
		if !v.has(v.index + 1) {
			return 0
		}
	}
	return v.arrowsAlpha(now)
}

func (n *buttonNode) Paint(ctx *gift.PaintContext) {
	a := n.alpha(ctx.Now())
	if a <= 0 {
		return
	}
	b := ctx.Bounds()
	c := geom.Pt((b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2)
	disc := geom.Rc(c.X-buttonDisc/2, c.Y-buttonDisc/2, c.X+buttonDisc/2, c.Y+buttonDisc/2)
	bg := ui.RGBA(0, 0, 0, 110)
	if ctx.Interaction().Pressed {
		bg = ui.RGBA(0, 0, 0, 170)
	}
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: disc, Color: fade(bg, a), CornerRadius: buttonDisc / 2})
	sym := outline.Close
	switch n.b.role {
	case rolePrev:
		sym = outline.ChevronLeft
	case roleNext:
		sym = outline.ChevronRight
	}
	icon := geom.Rc(c.X-iconSize/2, c.Y-iconSize/2, c.X+iconSize/2, c.Y+iconSize/2)
	sym.Draw(ctx, icon, fade(ui.OpaqueWhite, a), int(math.Round(float64(iconSize*n.b.v.density))))
}

func (n *buttonNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	if n.alpha(ctx.Now()) < 0.3 {
		return false
	}
	switch e.Kind {
	case gift.EventPointerDown:
		return true
	case gift.EventPointerUp:
		if !e.Inside || e.Dragged {
			return true
		}
		v := n.b.v
		switch n.b.role {
		case roleClose:
			v.Close()
		case rolePrev:
			v.Previous()
		case roleNext:
			v.Next()
		}
		v.hoverAt = ctx.Now()
		return true
	case gift.EventPointerMove:
		// The arrows stay while the mouse rests on them.
		if e.Device == gift.PointerMouse {
			n.b.v.hoverAt = ctx.Now()
		}
	}
	return false
}

// --- input -------------------------------------------------------------------

func (n *canvasNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	v := n.v
	n.now, n.ctx = ctx.Now(), ctx
	defer func() { n.ctx = nil }()
	v.settleHero(n.now)
	switch v.phase {
	case closed:
		return false
	case closing:
		if e.Kind == gift.EventTimer {
			if _, moving := v.heroProgress(n.now); moving {
				ctx.After(20 * time.Millisecond)
			} else {
				v.finishClose()
			}
		}
		// Nothing reaches what is behind a closing viewer.
		return e.IsPointer()
	case opening:
		if e.IsPointer() && e.Kind != gift.EventPointerMove {
			return true
		}
	}
	switch e.Kind {
	case gift.EventPointerDown:
		ctx.RequestFocus()
	case gift.EventKeyDown:
		return n.key(e)
	case gift.EventRune:
		return zoomKey(&v.zoom, n.now, e.Rune, n.zoomChanged)
	}
	return gesture.Handle(ctx, e, n.gs)
}

func (n *canvasNode) moved() {
	n.ctx.Repaint()
	n.ctx.Animate(time.Second)
}

func (n *canvasNode) zoomChanged() {
	n.moved()
	n.ctx.RequestLayout()
}

func (n *canvasNode) close(now time.Duration) {
	n.beginClose(now)
	n.ctx.After(heroDuration())
	n.moved()
}

func (n *canvasNode) onMagnify(m gesture.MagnifyValue) {
	v := n.v
	c := m.Local(m.Location)
	switch m.Phase {
	case gesture.Began:
		if v.mode == dragSwipe || v.mode == dragCard {
			return
		}
		if off, _ := v.pg.at(n.now); off != 0 {
			v.pg.settle(n.now)
			v.pg.spring(n.now, v.pg.off, 0)
		}
		if !v.zoom.gesturing {
			v.zoom.begin(n.now)
		}
		v.pinching, v.mag, v.anchor = true, 1, m.Local(m.StartAnchor)
		fallthrough
	case gesture.Changed:
		if !v.pinching {
			return
		}
		v.zoom.pinch(v.anchor, c, m.Magnification/v.mag)
		v.mag, v.anchor = m.Magnification, c
		n.moved()
	case gesture.Ended:
		if !v.pinching {
			return
		}
		v.pinching = false
		if v.dragging && v.mode == dragPan {
			return
		}
		n.releaseZoom(0, 0, v.anchor)
	}
}

// releaseZoom lets go of the zoom, or closes a picture pinched well below its
// fit.
func (n *canvasNode) releaseZoom(vx, vy float64, anchor geom.Point) {
	v := n.v
	if v.closable && v.zoom.z < 0.75 {
		n.close(n.now)
		return
	}
	v.zoom.end(n.now, vx, vy, anchor)
	n.zoomChanged()
}

func (n *canvasNode) onDrag(d gesture.DragValue) {
	v := n.v
	if d.Phase == gesture.Began {
		n.beginDrag(d)
		return
	}
	delta := d.Translation.Sub(v.trans)
	v.trans = d.Translation
	if v.pinching {
		return
	}
	dx, dy := float64(delta.X), float64(delta.Y)
	pitch := v.pitch()
	hasPrev, hasNext := v.has(v.index-1), v.has(v.index+1)
	switch v.mode {
	case dragPage:
		v.pg.raw -= dx
		v.pg.show(hasPrev, hasNext, pitch, v.vw)
	case dragPan:
		if v.pg.raw != 0 {
			r := v.pg.raw - dx
			if r*v.pg.raw < 0 {
				// Back across the resting place: the rest pans.
				dx, r = -r, 0
			} else {
				dx = 0
			}
			v.pg.raw = r
			v.pg.show(hasPrev, hasNext, pitch, v.vw)
		}
		v.zoom.pan(dx, dy, func(rest float64) bool {
			if rest < 0 && !hasNext || rest > 0 && !hasPrev {
				return false
			}
			v.pg.raw -= rest
			v.pg.show(hasPrev, hasNext, pitch, v.vw)
			return true
		})
	case dragSwipe:
		v.sw.t = v.sw.t.Add(delta)
	case dragCard:
		v.cd.drag(-dy / float64(v.cardHeight()))
		v.relayoutCard()
	}
	n.moved()
}

// beginDrag decides what a drag moves: a zoomed picture pans, and a picture
// at its fit pages sideways and swipes vertically, by the direction the
// finger took — the 45 degree split of Apple's photo transition sample.
func (n *canvasNode) beginDrag(d gesture.DragValue) {
	v := n.v
	v.dragging, v.trans = true, d.Translation
	t := d.Translation
	vertical := math.Abs(float64(t.Y)) > math.Abs(float64(t.X))
	v.pg.settle(n.now)
	switch {
	case d.Fingers > 1 || v.zoom.zoomed() || v.zoom.gesturing:
		v.mode = dragPan
		if !v.zoom.gesturing {
			v.zoom.begin(n.now)
		}
	case vertical && v.cd.open():
		v.mode = dragCard
		v.cd.settle(n.now)
	case vertical && t.Y > 0 && v.closable:
		v.mode = dragSwipe
		v.sw = swipe{active: true, anchor: d.Local(d.StartLocation)}
	case vertical && t.Y < 0 && v.hasCard:
		v.mode = dragCard
		v.cd.settle(n.now)
	case vertical:
		v.mode = dragNone
	default:
		v.mode = dragPage
	}
}

func (n *canvasNode) onDragEnd(d gesture.DragValue) {
	v := n.v
	v.dragging = false
	mode := v.mode
	v.mode = dragNone
	if v.pinching {
		return
	}
	vx, vy := float64(d.Velocity.X), float64(d.Velocity.Y)
	hasPrev, hasNext := v.has(v.index-1), v.has(v.index+1)
	switch mode {
	case dragPage:
		dir := v.pg.release(-vx, v.pitch(), hasPrev, hasNext)
		v.turn(n.now, dir, v.pg.off, -vx)
	case dragPan:
		if v.pg.raw != 0 {
			dir := v.pg.release(-vx, v.pitch(), hasPrev, hasNext)
			v.zoom.end(n.now, 0, vy, d.Local(d.Location))
			v.turn(n.now, dir, v.pg.off, -vx)
		} else {
			n.releaseZoom(vx, vy, d.Local(d.Location))
		}
	case dragSwipe:
		p := v.sw.progress(v.vh, v.sw.t)
		if vy > dismissSpeed || vy > -dismissSpeed && p > dismissProgress {
			n.close(n.now)
			return
		}
		v.sw.back, v.sw.active = true, false
		v.sw.t0, v.sw.from, v.sw.v0 = n.now, v.sw.t, d.Velocity
	case dragCard:
		h := float64(v.cardHeight())
		q := v.cd.q
		target := 0.0
		if -vy/h > 1.5 || vy/h <= 1.5 && q > 0.5 {
			target = 1
		}
		v.cd.animate(n.now, target, -vy/h)
		v.relayoutCard()
	}
	n.moved()
}

func (n *canvasNode) onCancel() {
	v := n.v
	v.dragging, v.pinching = false, false
	switch v.mode {
	case dragSwipe:
		v.sw.back, v.sw.active = true, false
		v.sw.t0, v.sw.from, v.sw.v0 = n.now, v.sw.t, geom.Point{}
	case dragPage, dragPan:
		v.pg.spring(n.now, v.pg.off, 0)
	}
	if v.zoom.gesturing {
		v.zoom.end(n.now, 0, 0, geom.Pt(float32(v.vw/2), float32(v.vh/2)))
	}
	v.mode = dragNone
	n.moved()
}

func (n *canvasNode) onTap(gesture.TapValue) {
	v := n.v
	if v.cd.open() {
		v.cd.settle(n.now)
		v.cd.animate(n.now, 0, 0)
		v.relayoutCard()
		n.moved()
		return
	}
	v.chrome, v.chromeT0 = !v.chrome, n.now
	n.moved()
}

func (n *canvasNode) onDoubleTap(t gesture.TapValue) {
	n.v.zoom.toggle(n.now, t.Local(t.Location), n.f.doubleTap)
	n.zoomChanged()
}

func (n *canvasNode) onHover(h gesture.HoverValue) {
	v := n.v
	if !h.Inside {
		v.hovering = false
		v.hoverAt = n.now - arrowsLinger
		n.moved()
		return
	}
	if !v.hovering || n.now-v.hoverAt > arrowsLinger+fadeOut {
		v.hoverFrom = n.now
	}
	v.hovering, v.hoverAt = true, n.now
	n.ctx.Repaint()
	n.ctx.Animate(arrowsLinger + fadeOut)
}

func (n *canvasNode) onWheel(w gesture.WheelValue) bool {
	v := n.v
	mode := n.f.wheel
	if mode == WheelOff || mode == WheelWithModifier && !w.Mods.Has(gift.ModControl) && !w.Mods.Has(gift.ModMeta) {
		return false
	}
	// A sideways scroll of a trackpad turns the page of a picture at its
	// fit.
	if math.Abs(float64(w.Delta.X)) > math.Abs(float64(w.Delta.Y)) && !v.zoom.zoomed() {
		v.wheelX += w.Delta.X
		if math.Abs(float64(v.wheelX)) >= 2 {
			dir := 1
			if v.wheelX > 0 {
				dir = -1
			}
			v.wheelX = 0
			off, _ := v.pg.at(n.now)
			v.pg.settle(n.now)
			v.turn(n.now, dir, off, 0)
			n.moved()
		}
		return true
	}
	if w.Delta.Y == 0 {
		return true
	}
	v.zoom.scale(n.now, math.Exp(wheelZoom*float64(w.Delta.Y)), w.Local(w.Location))
	n.zoomChanged()
	return true
}

func (n *canvasNode) key(e gift.Event) bool {
	v := n.v
	turn := func(dir int) bool {
		if !v.has(v.index + dir) {
			return true
		}
		off, _ := v.pg.at(n.now)
		v.pg.settle(n.now)
		v.turn(n.now, dir, off, 0)
		n.moved()
		return true
	}
	switch e.Key {
	case gift.KeyEscape:
		n.back()
		return true
	case gift.KeyLeft:
		return turn(-1)
	case gift.KeyRight:
		return turn(1)
	case gift.KeyHome:
		v.jump(0)
		return true
	case gift.KeyEnd:
		v.jump(v.coll.Len() - 1)
		return true
	case gift.KeySpace, gift.KeyEnter:
		if e.Repeat {
			return true
		}
		v.zoom.toggle(n.now, geom.Pt(float32(v.vw/2), float32(v.vh/2)), n.f.doubleTap)
		n.zoomChanged()
		return true
	case gift.KeyUp, gift.KeyDown:
		if !v.hasCard {
			return false
		}
		target := 1.0
		if e.Key == gift.KeyDown {
			target = 0
		}
		v.cd.settle(n.now)
		v.cd.animate(n.now, target, 0)
		v.relayoutCard()
		n.moved()
		return true
	}
	return false
}

// back is what escape and the platform's back action do: put the card away
// when it is open, else close the viewer when it may be closed. A viewer made
// with Closable(false) swallows both, so that neither reaches what is behind
// it. It runs with n.now and n.ctx set.
func (n *canvasNode) back() {
	v := n.v
	if v.cd.open() {
		v.cd.settle(n.now)
		v.cd.animate(n.now, 0, 0)
		v.relayoutCard()
		n.moved()
		return
	}
	if v.closable {
		n.close(n.now)
	}
}

// CanGoBack implements [gift.BackHandler]. The viewer takes the back action
// whenever it is on the screen, whatever back then does:
//
//   - Open or opening, back puts the card away or closes the viewer, as
//     escape does, or does nothing at all for a viewer that is not closable.
//     That last one is a deliberate block: the viewer covers everything and
//     is a focus trap, and a back that went past it would pop or leave what
//     the person cannot see, as an Android dialog that is not cancelable
//     keeps back to itself.
//   - Closing, back does nothing either. The picture is already on its way
//     into its thumbnail, and a second back during that short flight — a
//     double press, or the gesture repeated impatiently — must not go on to
//     pop the navigation stack or leave the application underneath, which is
//     what would happen if the viewer declined while it is still drawn.
//   - Closed, the viewer declines; its component has then replaced the
//     canvas with an empty node anyway.
//
// The phase moves on without a rebuild — the hero transition is driven by the
// canvas's own timers — so this reads the viewer's phase as it is now rather
// than anything captured at build time. It is a field read: no allocation.
func (n *canvasNode) CanGoBack() bool { return n.v.phase != closed }

// GoBack implements [gift.BackHandler]: back as escape does it, through the
// same animation path. It settles a finished opening first, as
// [canvasNode.HandleEvent] does for every event, so that a back right after
// the picture arrived is treated as one on an open viewer.
func (n *canvasNode) GoBack(ctx *gift.EventContext) {
	v := n.v
	n.now, n.ctx = ctx.Now(), ctx
	defer func() { n.ctx = nil }()
	v.settleHero(n.now)
	if v.phase == closed || v.phase == closing {
		return
	}
	n.back()
}

// --- the card ----------------------------------------------------------------

// cardHost is the slot of the card: the open card's rectangle, in which it
// places the card as far down as the card has been put away. Below the
// screen, a closed card is not hit by anything.
//
// It has its own node so that it can lay itself out on every frame of the
// card's spring, from a timer of its own: the canvas's timer belongs to its
// gestures.
type cardHost struct {
	v       *Viewer
	content gift.View
}

func (cardHost) ViewType() gift.TypeID { return cardType }

func (c cardHost) Build(*gift.BuildContext) gift.Element {
	n := &cardNode{v: c.v}
	return gift.Element{Key: "viewer-card", Layouter: n, Interactor: n, Children: []gift.View{c.content}, Clip: true}
}

type cardNode struct{ v *Viewer }

func (n *cardNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	v := n.v
	v.cardInval = ctx.Invalidator()
	size := c.Constrain(c.Max)
	q, moving := v.cd.at(ctx.Now())
	if ctx.ChildCount() > 0 {
		ctx.Measure(0, geom.Tight(size))
		ctx.Place(0, geom.Pt(0, size.H*float32(1-clamp(q, 0, 1.2))))
	}
	if moving {
		ctx.After(time.Second / 60)
	}
	return size
}

// HandleEvent lays the card out again when its timer fires and lets every
// other event through to the canvas.
func (n *cardNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	if e.Kind == gift.EventTimer {
		ctx.RequestLayout()
		return true
	}
	return false
}

// relayoutCard lays the card out again after its position changed.
func (v *Viewer) relayoutCard() {
	if v.cardInval != nil {
		v.cardInval()
	}
}
