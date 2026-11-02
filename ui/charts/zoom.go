package charts

import (
	"math"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui/gesture"
	"github.com/worldiety/gift/ui/motion"
)

// zoomDuration is how long a zoom by a tap, a dragged range or the reset
// button takes.
const zoomDuration = 350 * time.Millisecond

// viewport is the visible range of the x axis and its way to a new one.
type viewport struct {
	lo, hi  float64
	from    [2]float64
	t0      time.Duration
	dur     time.Duration
	moving  bool
	valid   bool
	zoomed  bool
	full    [2]float64
	hasFull bool
	now     time.Duration
	minSpan float64
}

// at returns the range at now and whether it is still on its way.
func (vp *viewport) at(now time.Duration) (lo, hi float64, moving bool) {
	vp.now = now
	if !vp.moving {
		return vp.lo, vp.hi, false
	}
	u := float64(now-vp.t0) / float64(vp.dur)
	if u >= 1 {
		vp.moving = false
		return vp.lo, vp.hi, false
	}
	e := 1 - math.Pow(1-u, 3)
	return vp.from[0] + (vp.lo-vp.from[0])*e, vp.from[1] + (vp.hi-vp.from[1])*e, true
}

// animateTo moves to [lo, hi] over [zoomDuration].
func (vp *viewport) animateTo(now time.Duration, lo, hi float64, zoomed bool) {
	vp.animateFor(now, lo, hi, zoomed, zoomDuration)
}

func (vp *viewport) animateFor(now time.Duration, lo, hi float64, zoomed bool, d time.Duration) {
	cl, ch, _ := vp.at(now)
	if !vp.valid {
		vp.jump(lo, hi, zoomed)
		return
	}
	lo, hi = vp.clamp(lo, hi)
	vp.from, vp.lo, vp.hi = [2]float64{cl, ch}, lo, hi
	vp.t0, vp.dur, vp.moving, vp.zoomed = now, d, true, zoomed && vp.isZoomed(lo, hi)
}

// jump shows [lo, hi] at once.
func (vp *viewport) jump(lo, hi float64, zoomed bool) {
	lo, hi = vp.clamp(lo, hi)
	vp.lo, vp.hi, vp.valid, vp.moving = lo, hi, true, false
	vp.zoomed = zoomed && vp.isZoomed(lo, hi)
}

// show shows a range the program gives.
func (vp *viewport) show(lo, hi float64) {
	if !vp.valid {
		vp.jump(lo, hi, true)
		return
	}
	vp.animateTo(vp.now, lo, hi, true)
}

func (vp *viewport) isZoomed(lo, hi float64) bool {
	if !vp.hasFull {
		return true
	}
	eps := (vp.full[1] - vp.full[0]) * 1e-9
	return lo > vp.full[0]+eps || hi < vp.full[1]-eps
}

// clamp keeps [lo, hi] within the extent and at least the narrowest span
// wide.
func (vp *viewport) clamp(lo, hi float64) (float64, float64) {
	if !vp.hasFull {
		return lo, hi
	}
	full := vp.full[1] - vp.full[0]
	span := min(max(hi-lo, vp.minSpan, full*1e-12), full)
	mid := (lo + hi) / 2
	lo, hi = mid-span/2, mid+span/2
	switch {
	case lo < vp.full[0]:
		lo, hi = vp.full[0], vp.full[0]+span
	case hi > vp.full[1]:
		lo, hi = vp.full[1]-span, vp.full[1]
	}
	return lo, hi
}

// follow takes a new extent: an unzoomed chart shows all of it, and a
// zoomed one that showed the end of the data keeps showing it, so that a
// live series runs on through a zoomed chart.
func (vp *viewport) follow(lo, hi float64) {
	old, had := vp.full, vp.hasFull
	vp.full, vp.hasFull = [2]float64{lo, hi}, true
	switch {
	case !vp.valid || !vp.zoomed && !vp.moving:
		vp.jump(lo, hi, false)
	case vp.zoomed && had && hi > old[1] && vp.hi >= old[1]-(old[1]-old[0])*1e-9:
		d := hi - old[1]
		vp.lo, vp.hi = vp.lo+d, vp.hi+d
		vp.from[0], vp.from[1] = vp.from[0]+d, vp.from[1]+d
	}
}

// eased2 is the y axis on its way to a new range, so that it glides when a
// zoom or a drag brings other values into view.
type eased2 struct {
	init     bool
	to, from [2]float64
	v0       [2]float64
	t0       time.Duration
	moving   bool
}

var yspring = motion.Spring{Response: 250 * time.Millisecond, Damping: 1}

func (e *eased2) set(now time.Duration, lo, hi float64) {
	switch {
	case !e.init:
		e.init, e.to = true, [2]float64{lo, hi}
	case lo != e.to[0] || hi != e.to[1]:
		cl, ch, _ := e.at(now)
		e.from, e.to, e.t0, e.moving = [2]float64{cl, ch}, [2]float64{lo, hi}, now, true
	}
}

func (e *eased2) at(now time.Duration) (lo, hi float64, moving bool) {
	if !e.moving {
		return e.to[0], e.to[1], false
	}
	t := now - e.t0
	span := e.to[1] - e.to[0]
	// A thousandth of the axis is less than a pixel of any plot.
	if yspring.Settled(t, e.from[0], e.to[0], 0, span*1e-3) && yspring.Settled(t, e.from[1], e.to[1], 0, span*1e-3) {
		e.moving = false
		return e.to[0], e.to[1], false
	}
	lo, _ = yspring.At(t, e.from[0], e.to[0], 0)
	hi, _ = yspring.At(t, e.from[1], e.to[1], 0)
	return lo, hi, true
}

// inspection is the x a pointer inspects.
type inspection struct {
	on bool
	x  float64
	// at is x moved onto the nearest point of the first series, where the
	// line of the inspection stands.
	at     float64
	touch  bool // a finger holds it: it ends when the finger lifts
	sticky bool // a tap set it: it stays until the next tap
}

// dragState is a drag under way: a pan, a pinch or a range dragged with the
// mouse.
type dragState struct {
	brushing bool
	b0, b1   float64
	panning  bool
	base     [2]float64
	pinching bool
	anchor   float64
}

// --- input -------------------------------------------------------------------------

// gestures returns the recognisers of the plot.
func (n *plotNode) gestures() []gesture.Gesture {
	return []gesture.Gesture{
		gesture.Magnify().OnChanged(n.onMagnify).OnEnded(n.onMagnify).OnCancelled(n.onCancel),
		gesture.Drag().ShouldBegin(n.dragBegins).OnChanged(n.onDrag).OnEnded(n.onDragEnd).OnCancelled(n.onCancel),
		gesture.Tap().Count(2).OnEnded(n.onDoubleTap),
		gesture.Tap().OnEnded(n.onTap),
		gesture.LongPress().OnEnded(n.onLongPress),
		gesture.Hover().OnChanged(n.onHover),
		gesture.Wheel().OnChanged(n.onWheel),
	}
}

// input handles an event of the surface.
func (n *plotNode) input(ctx *gift.EventContext, e gift.Event) bool {
	st := n.st
	st.vp.now = ctx.Now()
	n.ectx = ctx
	defer func() { n.ectx = nil }()
	switch e.Kind {
	case gift.EventPointerUp, gift.EventPointerCancel:
		if st.in.touch {
			n.inspect(0, false)
		}
	case gift.EventKeyDown:
		return n.key(e)
	case gift.EventRune:
		return n.rune(e.Rune)
	}
	return gesture.Handle(ctx, e, n.gs)
}

// zoomable reports whether the chart zooms at all.
func (n *plotNode) zoomable() bool {
	st := n.st
	return !st.v.still && len(st.v.cats) == 0 && st.vp.hasFull && st.plot.Width() > 0
}

// xAt returns the x under the local position px.
func (n *plotNode) xAt(px float32) float64 {
	st := n.st
	lo, hi, _ := st.vp.at(st.vp.now)
	return lo + float64(px-st.plot.Min.X)/float64(st.plot.Width())*(hi-lo)
}

// moved asks for a new layout and a build, after the range changed.
func (n *plotNode) moved() {
	if n.invalidate != nil {
		n.invalidate()
	}
	n.st.bump()
}

// zoomAbout scales the visible range by f about x, at once.
func (n *plotNode) zoomAbout(x, f float64) {
	st := n.st
	lo, hi, _ := st.vp.at(st.vp.now)
	lo, hi = x-(x-lo)*f, x+(hi-x)*f
	st.vp.jump(lo, hi, true)
	st.visibleChanged(st.vp.lo, st.vp.hi)
	n.moved()
}

func (n *plotNode) onMagnify(v gesture.MagnifyValue) {
	if !n.zoomable() {
		return
	}
	st := n.st
	d := &st.drag
	at := v.Common.Local(v.Location)
	if v.Phase == gesture.Began || !d.pinching {
		lo, hi, _ := st.vp.at(st.vp.now)
		d.pinching, d.base = true, [2]float64{lo, hi}
		d.anchor = n.xAt(v.Common.Local(v.StartAnchor).X)
	}
	span := (d.base[1] - d.base[0]) / math.Max(v.Magnification, 1e-6)
	f := float64(at.X-st.plot.Min.X) / float64(st.plot.Width())
	lo := d.anchor - f*span
	st.vp.jump(lo, lo+span, true)
	if v.Phase == gesture.Ended {
		d.pinching = false
		st.visibleChanged(st.vp.lo, st.vp.hi)
	}
	n.moved()
}

func (n *plotNode) dragBegins(v gesture.DragValue) bool {
	st := n.st
	switch {
	case st.in.touch:
		return true
	case !n.zoomable():
		return false
	case v.Device == gift.PointerMouse:
		return true
	}
	// A finger drags a zoomed chart sideways; anything else goes on to a
	// scroll view around it.
	return st.vp.zoomed && math.Abs(float64(v.Translation.X)) > math.Abs(float64(v.Translation.Y))
}

func (n *plotNode) onDrag(v gesture.DragValue) {
	st := n.st
	d := &st.drag
	at := v.Common.Local(v.Location)
	switch {
	case st.in.touch:
		n.inspect(n.xAt(at.X), true)
	case v.Device == gift.PointerMouse:
		if !d.brushing {
			d.brushing = true
			d.b0 = n.xAt(v.Common.Local(v.StartLocation).X)
		}
		d.b1 = n.xAt(at.X)
		n.inspect(0, false)
		n.moved()
	default:
		if !d.panning {
			lo, hi, _ := st.vp.at(st.vp.now)
			d.panning, d.base = true, [2]float64{lo, hi}
		}
		k := (d.base[1] - d.base[0]) / float64(st.plot.Width())
		dx := float64(v.Translation.X) * k
		st.vp.jump(d.base[0]-dx, d.base[1]-dx, true)
		n.moved()
	}
}

func (n *plotNode) onDragEnd(v gesture.DragValue) {
	st := n.st
	d := &st.drag
	switch {
	case d.brushing:
		d.brushing = false
		a, b := min(d.b0, d.b1), max(d.b0, d.b1)
		lo, hi, _ := st.vp.at(st.vp.now)
		if px := (b - a) / (hi - lo) * float64(st.plot.Width()); px >= 8 {
			st.vp.animateTo(st.vp.now, a, b, true)
			st.visibleChanged(st.vp.lo, st.vp.hi)
		}
		n.moved()
	case d.panning:
		d.panning = false
		// The chart glides on to where the flick would have taken it.
		k := (d.base[1] - d.base[0]) / float64(st.plot.Width())
		dx := float64(v.PredictedEndTranslation.X) * k
		st.vp.animateFor(st.vp.now, d.base[0]-dx, d.base[1]-dx, true, 500*time.Millisecond)
		st.visibleChanged(st.vp.lo, st.vp.hi)
		n.moved()
	}
}

func (n *plotNode) onCancel() {
	d := &n.st.drag
	d.brushing, d.panning, d.pinching = false, false, false
	n.moved()
}

func (n *plotNode) onDoubleTap(v gesture.TapValue) {
	if !n.zoomable() {
		return
	}
	st := n.st
	x := n.xAt(v.Common.Local(v.Location).X)
	lo, hi, _ := st.vp.at(st.vp.now)
	st.vp.animateTo(st.vp.now, x-(x-lo)/2, x+(hi-x)/2, true)
	st.visibleChanged(st.vp.lo, st.vp.hi)
	n.moved()
}

func (n *plotNode) onTap(v gesture.TapValue) {
	st := n.st
	if st.v.noInspect || v.Device == gift.PointerMouse {
		return
	}
	at := v.Common.Local(v.Location)
	if st.in.on && st.in.sticky {
		n.inspect(0, false)
		return
	}
	if st.plot.Contains(at) {
		n.inspect(n.xAt(at.X), true)
		st.in.sticky = true
	}
}

func (n *plotNode) onLongPress(v gesture.LongPressValue) {
	st := n.st
	if st.v.noInspect {
		return
	}
	at := v.Common.Local(v.Location)
	n.inspect(n.xAt(at.X), true)
	st.in.touch = true
}

func (n *plotNode) onHover(v gesture.HoverValue) {
	st := n.st
	if st.v.noInspect || st.drag.brushing {
		return
	}
	at := v.Common.Local(v.Location)
	if v.Inside && st.plot.Contains(at) {
		n.inspect(n.xAt(at.X), true)
		return
	}
	if st.in.on && !st.in.sticky {
		n.inspect(0, false)
	}
}

// inspect shows the values at x, or ends the inspection.
func (n *plotNode) inspect(x float64, on bool) {
	st := n.st
	if st.in.on == on && (!on || st.in.x == x) {
		return
	}
	st.in = inspection{on: on, x: x}
	if st.v.onInspect != nil {
		st.v.onInspect(x, on)
	}
	n.moved()
}

// wheelNotch is the factor of one notch of a wheel turned with the control
// or command key held.
const wheelNotch = 1.25

func (n *plotNode) onWheel(v gesture.WheelValue) bool {
	if !n.zoomable() {
		return false
	}
	st := n.st
	at := v.Common.Local(v.Location)
	zoomMod := v.Mods.Has(gift.ModControl) || v.Mods.Has(gift.ModMeta)
	switch {
	case zoomMod && v.Precise:
		n.zoomAbout(n.xAt(at.X), math.Exp2(-float64(v.Delta.Y)*0.05))
	case zoomMod:
		if v.Delta.Y == 0 {
			return false
		}
		n.zoomAbout(n.xAt(at.X), math.Pow(wheelNotch, -float64(v.Delta.Y)))
	case v.Precise && st.vp.zoomed && math.Abs(float64(v.Delta.X)) > math.Abs(float64(v.Delta.Y)):
		// A trackpad drags a zoomed chart sideways, the content following
		// the fingers.
		lo, hi, _ := st.vp.at(st.vp.now)
		dx := float64(v.Delta.X*gift.DefaultScrollPreciseWheelStep) * (hi - lo) / float64(st.plot.Width())
		st.vp.jump(lo-dx, hi-dx, true)
		st.visibleChanged(st.vp.lo, st.vp.hi)
		n.moved()
	default:
		return false
	}
	return true
}

func (n *plotNode) key(e gift.Event) bool {
	if !n.zoomable() {
		return false
	}
	st := n.st
	lo, hi, _ := st.vp.at(st.vp.now)
	step := (hi - lo) / 5
	switch e.Key {
	case gift.KeyLeft:
		st.vp.animateTo(st.vp.now, lo-step, hi-step, true)
	case gift.KeyRight:
		st.vp.animateTo(st.vp.now, lo+step, hi+step, true)
	case gift.KeyHome:
		st.resetZoom()
		return true
	default:
		return false
	}
	st.visibleChanged(st.vp.lo, st.vp.hi)
	n.moved()
	return true
}

func (n *plotNode) rune(r rune) bool {
	if !n.zoomable() {
		return false
	}
	st := n.st
	lo, hi, _ := st.vp.at(st.vp.now)
	mid := (lo + hi) / 2
	switch r {
	case '+', '=':
		st.vp.animateTo(st.vp.now, mid-(hi-lo)/4, mid+(hi-lo)/4, true)
	case '-', '_':
		st.vp.animateTo(st.vp.now, mid-(hi-lo), mid+(hi-lo), true)
	case '0':
		st.resetZoom()
		return true
	default:
		return false
	}
	st.visibleChanged(st.vp.lo, st.vp.hi)
	n.moved()
	return true
}

// plotX maps x to a local position on the plot.
func (st *state) plotX(x float64) float64 {
	return float64(st.plot.Min.X) + (x-st.lo)/(st.hi-st.lo)*float64(st.plot.Width())
}

// plotY maps y to a local position on the plot.
func (st *state) plotY(y float64) float64 {
	return float64(st.plot.Max.Y) - (y-st.ylo)/(st.yhi-st.ylo)*float64(st.plot.Height())
}
