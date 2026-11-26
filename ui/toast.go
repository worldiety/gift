package ui

import (
	"math"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

var (
	toastType      = gift.RegisterType("ui.Toast")
	toastLayerType = gift.RegisterType("ui.toastLayer")
	toastCardType  = gift.RegisterType("ui.toastCard")
)

// Metrics and timings of a toast: those of the capsules iOS shows for a
// copied link or a connected headset.
const (
	toastMargin   = float32(12)
	toastMinH     = float32(50)
	toastPadX     = float32(20)
	toastPadY     = float32(10)
	toastIconSize = float32(22)
	toastGap      = float32(12)
	toastMaxW     = float32(420)
	toastExit     = 280 * time.Millisecond
	toastAppear   = 600 * time.Millisecond
	toastDuration = 2 * time.Second
)

// ToastEdge is the edge of the window a toast appears at.
type ToastEdge uint8

const (
	// ToastTop is the default, where iOS shows its capsules.
	ToastTop ToastEdge = iota
	// ToastBottom is where Android shows its toasts, and out of the way of
	// a title bar.
	ToastBottom
)

// ToastView shows a short message above everything for a moment. It is
// created by [Toast].
type ToastView struct {
	content   gift.View
	title     string
	subtitle  string
	icon      Symbol
	hasIcon   bool
	presented bool
	onDismiss func()
	onTap     func()
	duration  time.Duration
	edge      ToastEdge
	key       string
	flex      float32
}

// Toast returns content with a message that slides in at the top of the
// window while it is [ToastView.Presented], stays for two seconds and goes
// again: a capsule with an optional symbol, a title and an optional subtitle,
// as iOS shows "Copied" or a connected headset.
//
//	copied := ctx.State("copied", false)
//	ui.Toast(screen, "Link copied").Icon(outline.Link).
//		Presented(ctx.Read(copied)).OnDismiss(func() { copied.Set(false) })
//
// The toast takes no room and blocks nothing around it. When its time is up
// it calls [ToastView.OnDismiss], whose job is to stop presenting it; a
// finger that pushes it back to its edge dismisses it early, a tap calls
// [ToastView.OnTap] and dismisses it, and while a finger holds it it stays.
// A new title is a new toast, which slides in afresh.
func Toast(content gift.View, title string) ToastView {
	return ToastView{content: content, title: title, duration: toastDuration}
}

// Presented shows the toast while b is true.
func (v ToastView) Presented(b bool) ToastView { v.presented = b; return v }

// OnDismiss sets the function called when the toast's time is up or it was
// swiped or tapped away; it should stop presenting it.
func (v ToastView) OnDismiss(fn func()) ToastView { v.onDismiss = fn; return v }

// OnTap sets the function called when the toast is tapped, before it is
// dismissed.
func (v ToastView) OnTap(fn func()) ToastView { v.onTap = fn; return v }

// Subtitle sets a second, smaller line under the title.
func (v ToastView) Subtitle(s string) ToastView { v.subtitle = s; return v }

// Icon sets the symbol in front of the text.
func (v ToastView) Icon(s Symbol) ToastView { v.icon, v.hasIcon = s, true; return v }

// Duration sets how long the toast stays; zero keeps it until it is swiped
// away or no longer presented.
func (v ToastView) Duration(d time.Duration) ToastView { v.duration = max(d, 0); return v }

// Edge sets the edge of the window the toast appears at.
func (v ToastView) Edge(e ToastEdge) ToastView { v.edge = e; return v }

// Key sets the reconciliation key.
func (v ToastView) Key(s string) ToastView { v.key = s; return v }

// Flex sets the flex factor in a stack.
func (v ToastView) Flex(f float32) ToastView { v.flex = f; return v }

// ViewType implements gift.View.
func (v ToastView) ViewType() gift.TypeID { return toastType }

// Build implements gift.View.
func (v ToastView) Build(*gift.BuildContext) gift.Element {
	e := gift.Element{Key: v.key, Flex: v.flex, Layouter: anchorLayout{}}
	if v.content != nil {
		e.Children = []gift.View{v.content}
	}
	if v.presented && v.title != "" {
		e.Overlay = toastLayer{v: v}
		e.OverlayExit = toastExit
	}
	return e
}

// toastLayer covers the window, as everything in the top layer does, but has
// no interactor: a press beside the capsule goes to whatever is under it.
type toastLayer struct{ v ToastView }

func (toastLayer) ViewType() gift.TypeID { return toastLayerType }

func (l toastLayer) Build(*gift.BuildContext) gift.Element {
	return gift.Element{
		Key:      "ui.toastLayer",
		Layouter: toastLayerLayout{edge: l.v.edge},
		// The key carries the message, so that a new one is a new card with
		// its own entrance and its own time.
		Children: []gift.View{toastCard{v: l.v}},
	}
}

type toastLayerLayout struct{ edge ToastEdge }

// Layout places the capsule at its edge of the safe rectangle, not of the
// window: the layer covers the whole window, as the top layer does, but on a
// phone drawn edge to edge the top of the window is the status bar and the
// bottom the navigation bar, and a toast that slid in there would sit under
// the clock or under the home indicator, half covered and out of reach of the
// finger that swipes it away. So the margin is measured from the edge of
// [safeWindow], the width is the safe width, and a phone on its side centres
// the capsule between its cutout and its navigation bar rather than between
// the edges of the glass. Without a safe area the rectangle is the window and
// the arithmetic is the one it always was.
func (l toastLayerLayout) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	win := c.Constrain(c.Max)
	r := safeWindow(ctx, win)
	// A safe rectangle narrower than the two margins — a safe area larger
	// than the window, which a backend may report for a frame while a phone
	// folds — offers the capsule no width rather than a negative one, which
	// constraints may not have.
	s := ctx.Measure(0, geom.Constraints{Max: geom.Sz(max(0, min(r.Width()-2*toastMargin, toastMaxW)), r.Height())})
	y := r.Min.Y + toastMargin
	if l.edge == ToastBottom {
		y = r.Max.Y - toastMargin - s.H
	}
	ctx.Place(0, geom.Pt(r.Min.X+(r.Width()-s.W)/2, y))
	ctx.ReportOverflow(geom.Size{})
	return win
}

// toastCard is the capsule: symbol, title and subtitle, its entrance and
// exit, and the finger.
type toastCard struct{ v ToastView }

func (toastCard) ViewType() gift.TypeID { return toastCardType }

func (c toastCard) Build(bc *gift.BuildContext) gift.Element {
	v := c.v
	var kids []gift.View
	if v.hasIcon {
		kids = append(kids, Icon(v.icon).Size(toastIconSize).Foreground(ColorLabel))
	}
	kids = append(kids, Text(v.title).FontSize(15).Foreground(ColorLabel).MaxLines(1))
	if v.subtitle != "" {
		kids = append(kids, Text(v.subtitle).FontSize(13).Foreground(ColorSecondaryLabel).MaxLines(1))
	}
	label := v.title
	if v.subtitle != "" {
		label += ", " + v.subtitle
	}
	n := &toastNode{v: v, bg: ResolveColor(ColorSurface), shadow: ResolveColor(RGBA(0, 0, 0, 60))}
	// The capsule enters from beyond the edge of the window, which is the
	// safe area further out than the edge of the safe rectangle it rests
	// against; a change of the safe area rebuilds every scope, so this is
	// read where it is current.
	if v.edge == ToastBottom {
		n.beyond = bc.SafeArea().Bottom
	} else {
		n.beyond = bc.SafeArea().Top
	}
	return gift.Element{
		Key:      "toast-" + v.title + "\x00" + v.subtitle,
		Label:    label,
		Layouter: n, Painter: n, Interactor: n,
		Children: kids,
	}
}

// toastState is what a card keeps across builds: when it appeared, and the
// finger on it.
type toastState struct {
	since    time.Duration
	pressed  bool
	from     float32
	dy       float32
	released time.Duration
	dyAtUp   float32
}

type toastNode struct {
	v          ToastView
	bg, shadow Color
	// beyond is the safe area at the toast's edge: how much further the
	// edge of the window is than the edge the card rests against.
	beyond float32
}

func (n *toastNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	st := ctx.Retained[toastState]()
	if st.since == 0 {
		st.since = max(ctx.Now(), 1)
		ctx.Animate(toastAppear)
		if n.v.duration > 0 {
			ctx.After(n.v.duration)
		}
	}
	icon := 0
	x := toastPadX
	if n.v.hasIcon {
		s := ctx.Measure(0, geom.Tight(geom.Sz(toastIconSize, toastIconSize)))
		x += s.W + toastGap
		icon = 1
	}
	maxText := max(c.Max.W-x-toastPadX, 0)
	title := ctx.Measure(icon, geom.Constraints{Max: geom.Sz(maxText, c.Max.H)})
	textW, textH := title.W, title.H
	var sub geom.Size
	if ctx.ChildCount() > icon+1 {
		sub = ctx.Measure(icon+1, geom.Constraints{Max: geom.Sz(maxText, c.Max.H)})
		textW, textH = max(textW, sub.W), textH+sub.H
	}
	h := max(toastMinH, textH+2*toastPadY)
	w := x + textW + toastPadX
	if n.v.hasIcon {
		ctx.Place(0, geom.Pt(toastPadX, (h-toastIconSize)/2))
	}
	y := (h - textH) / 2
	ctx.Place(icon, geom.Pt(x, y))
	if sub.H > 0 {
		ctx.Place(icon+1, geom.Pt(x, y+title.H))
	}
	return c.Constrain(geom.Sz(w, h))
}

// offset is how far the card is from its place towards its edge's outside,
// negative for the top edge, and how opaque it is.
func (n *toastNode) offset(ctx *gift.PaintContext, st *toastState, h float32) (dy, alpha float32, moving bool) {
	away := -(h + toastMargin + 8 + n.beyond)
	if n.v.edge == ToastBottom {
		away = -away
	}
	if leave, leaving := ctx.OverlayLeaving(); leaving {
		e := leave * leave
		return st.dy + (away-st.dy)*e, 1 - leave, true
	}
	if st.pressed {
		return st.dy, 1, false
	}
	// In from outside on a spring with a trace of overshoot, and back from
	// wherever a finger left it.
	t := (ctx.Now() - st.since).Seconds()
	from := away
	if st.released > st.since {
		t, from = (ctx.Now() - st.released).Seconds(), st.dyAtUp
	}
	if t >= toastAppear.Seconds() {
		return 0, 1, false
	}
	const z = 0.75
	w := 2 * math.Pi / 0.45
	wd := w * math.Sqrt(1-z*z)
	e := math.Exp(-z * w * t)
	f := float32(1 - e*(math.Cos(wd*t)+z*w/wd*math.Sin(wd*t)))
	return from * (1 - f), clampf(f*2, 0, 1), true
}

func (n *toastNode) Paint(ctx *gift.PaintContext) {
	st := ctx.Retained[toastState]()
	b := ctx.Bounds()
	dy, alpha, moving := n.offset(ctx, st, b.Height())
	ctx.PushTransform(geom.Translate(geom.Pt(0, dy)))
	r := b.Height() / 2
	ctx.Add(render.Op{Kind: render.OpShadow, Bounds: b.Translate(geom.Pt(0, 4)), Color: scaleColor(n.shadow, alpha), CornerRadius: r, Blur: 20})
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: b, Color: scaleColor(n.bg, alpha), CornerRadius: r})
	ctx.PaintChildren()
	ctx.PopTransform()
	if moving {
		ctx.Animate(50 * time.Millisecond)
	}
}

func (n *toastNode) dismiss() {
	if n.v.onDismiss != nil {
		n.v.onDismiss()
	}
}

// HandleEvent follows a finger along the axis of the toast's edge: towards
// the outside freely, inwards against a rubber band. Pushed far enough out
// it dismisses the toast; let go short of that, it springs back and its time
// starts again.
func (n *toastNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	st := ctx.Retained[toastState]()
	out := float32(-1)
	if n.v.edge == ToastBottom {
		out = 1
	}
	switch e.Kind {
	case gift.EventTimer:
		if st.pressed {
			return true
		}
		n.dismiss()
		return true
	case gift.EventPointerDown:
		st.pressed, st.from, st.dy = true, e.Pos.Y, 0
		ctx.Repaint()
		return true
	case gift.EventPointerMove:
		if !st.pressed {
			return false
		}
		d := e.Pos.Y - st.from
		if d*out < 0 {
			d = d / (1 + absf(d)/20) // inwards: a rubber band
		}
		st.dy = d
		ctx.Repaint()
		return true
	case gift.EventPointerUp, gift.EventPointerCancel:
		if !st.pressed {
			return false
		}
		st.pressed = false
		switch {
		case st.dy*out > 16:
			n.dismiss()
		case e.Kind == gift.EventPointerUp && !e.Dragged && e.Inside:
			if n.v.onTap != nil {
				n.v.onTap()
			}
			n.dismiss()
		default:
			st.released, st.dyAtUp, st.dy = max(e.Time, st.since+1), st.dy, 0
			ctx.Animate(toastAppear)
			if n.v.duration > 0 {
				ctx.After(n.v.duration)
			}
		}
		ctx.Repaint()
		return true
	}
	return false
}
