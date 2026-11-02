package charts

import (
	"math"
	"strconv"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/gesture"
)

// plot is the chart itself: the node that lays out the axes and draws the
// marks, with the labels as its children.
type plot struct {
	st   *state
	kids []gift.View
}

func (plot) ViewType() gift.TypeID { return plotType }

func (p plot) Build(*gift.BuildContext) gift.Element {
	n := p.st.node
	return gift.Element{
		Key:        "plot",
		Label:      describe(p.st.v),
		Layouter:   n,
		Painter:    n,
		Interactor: n,
		Children:   p.kids,
		Clip:       true,
	}
}

// surface takes the input of a plot: a child as large as the plot, above
// its labels and below its button, with the gestures. The plot keeps its
// own node to itself, whose one timer drives the zoom, which would replace
// the timer a tap waits on to rule out a double tap.
type surface struct{ n *plotNode }

var surfaceType = gift.RegisterType("charts.surface")

func (surface) ViewType() gift.TypeID { return surfaceType }

func (s surface) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: "input", Label: describe(s.n.st.v), Layouter: s, Interactor: s,
		Focusable: !s.n.st.v.still && len(s.n.st.v.cats) == 0, MultiTouch: gesture.NeedsMultiTouch(s.n.gs)}
}

func (s surface) Layout(_ *gift.LayoutContext, c geom.Constraints) geom.Size { return c.Max }

func (s surface) HandleEvent(ctx *gift.EventContext, e gift.Event) bool { return s.n.input(ctx, e) }

type plotNode struct {
	st   *state
	gs   []gesture.Gesture
	ectx *gift.EventContext
	// invalidate lays the plot out again, from the input on the surface.
	invalidate func()
	mesh       mesh
	ps         []pt
	buf        []pt
	tmp        []pt
}

// frameStep is how often the plot is laid out while the zoom or the y
// axis moves.
const frameStep = 16 * time.Millisecond

// HandleEvent takes the timer of the plot, which moves the zoom and the y
// axis on.
func (n *plotNode) HandleEvent(ctx *gift.EventContext, e gift.Event) bool {
	if e.Kind != gift.EventTimer {
		return false
	}
	ctx.RequestLayout()
	ctx.Repaint()
	return true
}

// gap between a label and what it labels, and around the plot.
const pad = 6

// hidden is where a label goes that is not shown: outside the chart, which
// clips its children.
var hidden = geom.Pt(-1e5, -1e5)

func (n *plotNode) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	st := n.st
	v := st.v
	now := ctx.Now()
	w := v.width
	if w == 0 {
		w = c.Max.W
		if math.IsInf(float64(w), 1) || w <= 0 {
			w = 480
		}
	}
	size := c.Constrain(geom.Sz(w, v.height))
	st.size, st.density = size, ctx.Density()
	if st.polar() {
		n.layoutPolar(ctx, size)
		return size
	}
	p := st.plan

	if lo, hi, ok := st.extent(); ok {
		st.vp.minSpan = st.minSpan(hi - lo)
		st.vp.follow(lo, hi)
	}
	lo, hi, xMoving := st.vp.at(now)

	// The labels decide the room around the plot.
	loose := geom.Constraints{Max: geom.Sz(size.W, size.H)}
	var yw, xh, lh float32
	for i := p.y[0]; i < p.y[1]; i++ {
		yw = max(yw, ctx.Measure(i, loose).W)
	}
	for i := p.x[0]; i < p.x[1]; i++ {
		xh = max(xh, ctx.Measure(i, loose).H)
	}
	if xh == 0 {
		xh = labelSize + 3
	}
	var xTitle, yTitle geom.Size
	if p.xTitle >= 0 {
		xTitle = ctx.Measure(p.xTitle, loose)
	}
	if p.yTitle >= 0 {
		yTitle = ctx.Measure(p.yTitle, loose)
	}
	legend := n.layoutLegend(ctx, size.W)
	if len(legend) > 0 {
		last := legend[len(legend)-1]
		lh = last.Max.Y + pad
	}
	top := float32(labelSize/2 + 2)
	if yTitle.H > 0 {
		top += yTitle.H + 2
	}
	bottom := size.H - xh - pad - lh
	if xTitle.H > 0 {
		bottom -= xTitle.H + 2
	}
	left, right := float32(0), size.W-yw-pad
	if v.leading {
		left, right = yw+pad, size.W
	}
	if len(v.cats) == 0 && !v.leading {
		// The first label of the x axis is centred on the edge.
		left = 2
	}
	plot := geom.Rect{Min: geom.Pt(left, top), Max: geom.Pt(max(right, left+1), max(bottom, top+1))}
	st.plot, st.lo, st.hi = plot, lo, hi

	n.reduce(ctx, lo, hi)
	ylo, yhi, yMoving := st.ys.at(now)
	if st.yZoomed() {
		ylo, yhi, yMoving = st.yvp.at(now)
	}
	st.ylo, st.yhi = ylo, yhi

	xt := st.xTicks(lo, hi, plot.Width())
	yf := v.yFormat
	if st.normal && yf == nil {
		yf = func(y float64) string { return strconv.Itoa(int(math.Round(y*100))) + " %" }
	}
	yt := numberTicks(ylo, yhi, plot.Height(), 36, yf)
	st.valueLabels()
	if !sameTicks(xt, st.builtX) || !sameTicks(yt, st.builtY) || !sameValues(st.values, st.builtValues) {
		st.xt, st.yt = xt, yt
		ctx.RequestBuild()
	}

	// The x labels, centred under their ticks, the ones that would collide
	// with the one before left out.
	edge := float32(math.Inf(-1))
	for k, i := 0, p.x[0]; i < p.x[1]; k, i = k+1, i+1 {
		s := ctx.ChildSize(i)
		x := float32(st.plotX(st.builtX[k].v))
		at := geom.Pt(min(max(x-s.W/2, 0), size.W-s.W), plot.Max.Y+pad/2)
		if x < plot.Min.X-1 || x > plot.Max.X+1 || at.X < edge+pad {
			ctx.Place(i, hidden)
			continue
		}
		ctx.Place(i, at)
		edge = at.X + s.W
	}
	for k, i := 0, p.y[0]; i < p.y[1]; k, i = k+1, i+1 {
		s := ctx.ChildSize(i)
		y := float32(st.plotY(st.builtY[k].v))
		if y < plot.Min.Y-s.H/2 || y > plot.Max.Y+s.H/2 {
			ctx.Place(i, hidden)
			continue
		}
		x := plot.Max.X + pad
		if v.leading {
			x = yw - s.W
		}
		ctx.Place(i, geom.Pt(x, y-s.H/2))
	}
	if p.yTitle >= 0 {
		x := size.W - yTitle.W
		if v.leading {
			x = 0
		}
		ctx.Place(p.yTitle, geom.Pt(x, 0))
	}
	if p.xTitle >= 0 {
		ctx.Place(p.xTitle, geom.Pt(plot.Max.X-xTitle.W, plot.Max.Y+pad/2+xh+2))
	}
	legendTop := size.H - lh + pad
	st.legendAt = st.legendAt[:0]
	for k, i := 0, p.legend[0]; i < p.legend[1]; k, i = k+1, i+1 {
		r := legend[k].Translate(geom.Pt(0, legendTop))
		ctx.Place(i, geom.Pt(r.Min.X+14, r.Min.Y))
		st.legendAt = append(st.legendAt, r)
	}
	n.placeMarkLabels(ctx, plot)
	n.placeValues(ctx, plot)
	n.placeNotes(ctx, plot)
	n.placeCallout(ctx, plot)
	if p.reset >= 0 {
		ctx.Measure(p.reset, loose)
		ctx.Place(p.reset, geom.Pt(plot.Min.X+pad, plot.Min.Y+pad))
	}
	if p.status >= 0 {
		s := ctx.Measure(p.status, geom.Constraints{Max: geom.Sz(plot.Width(), plot.Height())})
		ctx.Place(p.status, geom.Pt(plot.Min.X+(plot.Width()-s.W)/2, plot.Min.Y+(plot.Height()-s.H)/2))
	}
	if p.surface >= 0 {
		ctx.Measure(p.surface, geom.Constraints{Min: size, Max: size})
		ctx.Place(p.surface, geom.Point{})
	}
	n.invalidate = ctx.Invalidator()
	if xMoving || yMoving {
		// The timer fires when the clock has moved, so a frame that is
		// drawn again at the same moment lays nothing out.
		ctx.After(frameStep)
		ctx.Animate(frameStep)
	}
	return size
}

// reduce works out what every mark shows of [lo, hi] at the width of the
// plot, fits the y axis to it and notes what the sources lack.
func (n *plotNode) reduce(ctx *gift.LayoutContext, lo, hi float64) {
	st := n.st
	v := st.v
	series := 0
	for _, m := range v.marks {
		if m.spec().data != nil {
			series++
		}
	}
	buckets := int(math.Ceil(float64(st.plot.Width() * st.density)))
	if series > 0 {
		buckets = min(buckets, v.budget/(4*series))
	}
	buckets = max(buckets, 16)
	st.need = [3]float64{lo, hi, float64(buckets)}

	ymin, ymax := math.Inf(1), math.Inf(-1)
	take := func(y float64) {
		if !Missing(y) {
			ymin, ymax = min(ymin, y), max(ymax, y)
		}
	}
	zero := v.zero
	areas := 0
	for _, m := range v.marks {
		if m.spec().kind == kindArea {
			areas++
		}
	}
	areaStack := areas > 1 && (v.stacking == Stacked || v.stacking == Normalized)
	empty, busy, fetch := true, false, false
	st.errText = ""
	// Every series takes the next colour, and marks of the same series, an
	// area with its line on top, share one.
	colors := map[Series]render.Color{}
	for i, m := range v.marks {
		s := m.spec()
		c := &st.cache[i]
		switch {
		case s.hasColor:
			c.color = ui.ResolveColor(s.color)
		case s.kind <= kindScatter:
			key := s.data
			if _, ok := key.(Func); ok {
				key = funcKey(i)
			}
			col, ok := colors[key]
			if !ok {
				col = seriesColor(len(colors))
				colors[key] = col
			}
			c.color = col
		case s.kind == kindRuleX || s.kind == kindRuleY:
			c.color = ui.ResolveColor(ui.ColorSecondaryLabel)
		case s.kind == kindNote:
			c.color = ui.ResolveColor(ui.ColorLabel)
		default:
			c.color = ui.ResolveColor(ui.ColorAccent)
		}
		switch s.kind {
		case kindRuleY:
			take(s.a)
			continue
		case kindNote:
			take(s.b)
			continue
		case kindBandY:
			take(s.a)
			take(s.b)
			continue
		case kindRuleX, kindBandX:
			continue
		case kindBar, kindArea:
			zero = true
		}
		switch d := s.data.(type) {
		case *Data:
			key := cacheKey{lo: lo, hi: hi, buckets: buckets, rev: d.rev, src: d}
			if c.key != key {
				c.pts, c.key = d.reduce(c.pts[:0], lo, hi, buckets), key
			}
		case *Source:
			ld := st.loads[d]
			if ld == nil {
				fetch = true
				continue
			}
			c.pts = ld.pts
			busy = busy || ld.busy || ld.extBusy
			if ld.err != nil {
				st.errText = ld.err.Error()
			} else if ld.extErr != nil {
				st.errText = ld.extErr.Error()
			}
			if !covers(ld.have, lo, hi, buckets, ld.extent, ld.hasExt) &&
				!((ld.busy || ld.err != nil) && covers(ld.want, lo, hi, buckets, ld.extent, ld.hasExt)) {
				fetch = true
			}
			if !ld.hasExt && !ld.extBusy && ld.extErr == nil && d.cfg.Extent != nil {
				fetch = true
			}
		case Func:
			c.pts = c.pts[:0]
			for k := range buckets + 1 {
				x := lo + (hi-lo)*float64(k)/float64(buckets)
				y := d(x)
				if Missing(y) {
					y = gap
				}
				c.pts = append(c.pts, Point{x, y})
			}
			c.key = cacheKey{}
		default:
			continue
		}
		// Bars and stacked areas are fitted below, as they are stacked.
		own := s.kind != kindBar && !(s.kind == kindArea && areaStack)
		for _, p := range c.pts {
			if p.X >= lo && p.X <= hi {
				if own {
					take(p.Y)
				}
				if !Missing(p.Y) {
					empty = false
				}
			}
		}
	}
	st.layoutBars(lo, hi)
	for _, b := range st.bars {
		if b.x1 >= lo && b.x0 <= hi {
			take(b.y0)
			take(b.y1)
		}
	}
	st.stackAreas()
	for i := range v.marks {
		c := &st.cache[i]
		if !c.stacked {
			continue
		}
		for k, p := range c.pts {
			if p.X >= lo && p.X <= hi {
				take(c.lower[k])
				take(c.upper[k])
			}
		}
	}
	st.normal = v.stacking == Normalized && (len(st.bars) > 0 || areaStack)
	if fetch {
		ctx.RequestBuild()
	}
	if st.empty != empty || st.busy != busy {
		st.empty, st.busy = empty, busy
		ctx.RequestBuild()
	}
	if len(v.marks) == 0 {
		st.empty = false
	}

	var tlo, thi float64
	switch {
	case st.normal:
		tlo, thi = 0, 1
	case v.hasY:
		tlo, thi = v.ydom[0], v.ydom[1]
	case math.IsInf(ymin, 1):
		tlo, thi = 0, 1
	default:
		if zero {
			ymin, ymax = min(ymin, 0), max(ymax, 0)
		}
		tlo, thi = niceDomain(ymin, ymax, max(int(st.plot.Height()/36), 1))
	}
	st.ys.set(ctx.Now(), tlo, thi)
}

// xTicks returns the ticks of the x axis for [lo, hi] over px pixels.
func (st *state) xTicks(lo, hi float64, px float32) []tick {
	v := st.v
	switch {
	case len(v.cats) > 0:
		var ts []tick
		for i, c := range v.cats {
			if x := float64(i); x >= lo-0.5 && x <= hi+0.5 {
				ts = append(ts, tick{x, c})
			}
		}
		return ts
	case v.isTime():
		return timeTicks(lo, hi, px, 72, v.location(), v.xFormat)
	}
	return numberTicks(lo, hi, px, 72, v.xFormat)
}

func sameTicks(a, b []tick) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// layoutLegend measures the legend entries and flows them into lines of
// width w, a swatch before each name; it returns their rectangles from the
// top of the legend.
func (n *plotNode) layoutLegend(ctx *gift.LayoutContext, w float32) []geom.Rect {
	p := n.st.plan
	var rs []geom.Rect
	x, y, lineH := float32(0), float32(0), float32(0)
	for i := p.legend[0]; i < p.legend[1]; i++ {
		s := ctx.Measure(i, geom.Constraints{Max: geom.Sz(w-14, 100)})
		ew := 14 + s.W
		if x > 0 && x+ew > w {
			x, y = 0, y+lineH+4
		}
		rs = append(rs, geom.Rect{Min: geom.Pt(x, y), Max: geom.Pt(x+ew, y+s.H)})
		x += ew + 14
		lineH = max(lineH, s.H)
	}
	return rs
}

// placeMarkLabels puts the label of a rule at its trailing end and that of
// a band in its leading top corner.
func (n *plotNode) placeMarkLabels(ctx *gift.LayoutContext, plot geom.Rect) {
	st := n.st
	p := st.plan
	loose := geom.Constraints{Max: geom.Sz(plot.Width(), plot.Height())}
	for k, i := 0, p.labels[0]; i < p.labels[1]; k, i = k+1, i+1 {
		s := st.v.marks[p.labelOf[k]].spec()
		size := ctx.Measure(i, loose)
		var at geom.Point
		switch s.kind {
		case kindRuleY:
			y := float32(st.plotY(s.a))
			at = geom.Pt(plot.Max.X-size.W-4, y-size.H-2)
		case kindRuleX:
			at = geom.Pt(float32(st.plotX(s.a))+4, plot.Min.Y+2)
		case kindBandX:
			at = geom.Pt(float32(st.plotX(s.a))+4, plot.Min.Y+2)
		case kindBandY:
			at = geom.Pt(plot.Min.X+4, float32(st.plotY(s.b))+2)
		}
		at.X = min(max(at.X, plot.Min.X), plot.Max.X-size.W)
		at.Y = min(max(at.Y, plot.Min.Y), plot.Max.Y-size.H)
		ctx.Place(i, at)
	}
}

// placeCallout puts the values of the inspection in a box beside the line
// of the inspected x, on the side with room.
func (n *plotNode) placeCallout(ctx *gift.LayoutContext, plot geom.Rect) {
	st := n.st
	p := st.plan
	st.callout = geom.Rect{}
	st.calloutAt = st.calloutAt[:0]
	if p.callout[1] == p.callout[0] {
		return
	}
	const inset = 8
	var w, h float32
	loose := geom.Constraints{Max: geom.Sz(min(plot.Width()-2*inset, 260), plot.Height())}
	sizes := make([]geom.Size, 0, p.callout[1]-p.callout[0])
	for i := p.callout[0]; i < p.callout[1]; i++ {
		s := ctx.Measure(i, loose)
		if i > p.callout[0] {
			s.W += 14
		}
		sizes = append(sizes, s)
		w = max(w, s.W)
		h += s.H + 2
	}
	w, h = w+2*inset, h-2+2*inset
	x := float32(st.plotX(st.in.at))
	left := x + 10
	if left+w > plot.Max.X {
		left = x - 10 - w
	}
	left = max(left, plot.Min.X)
	box := geom.Rect{Min: geom.Pt(left, plot.Min.Y+4), Max: geom.Pt(left+w, plot.Min.Y+4+h)}
	st.callout = box
	y := box.Min.Y + inset
	for k, i := 0, p.callout[0]; i < p.callout[1]; k, i = k+1, i+1 {
		s := sizes[k]
		at := geom.Pt(box.Min.X+inset, y)
		if k > 0 {
			st.calloutAt = append(st.calloutAt, geom.Rect{Min: at, Max: at.Add(geom.Pt(s.W, s.H))})
			at.X += 14
			ctx.Measure(i, geom.Constraints{Min: geom.Sz(w-2*inset-14, 0), Max: geom.Sz(w-2*inset-14, s.H)})
		}
		ctx.Place(i, at)
		y += s.H + 2
	}
}

// --- paint -------------------------------------------------------------------------

func (n *plotNode) Paint(ctx *gift.PaintContext) {
	st := n.st
	v := st.v
	if st.polar() {
		n.paintPolar(ctx)
		return
	}
	b := ctx.Bounds()
	db := ctx.DeviceBounds()
	at := func(r geom.Rect) geom.Rect { return r.Translate(b.Min) }
	plot := at(st.plot)
	// Clips are in device space, which the transform of the node maps to;
	// for a node that is not rotated that is a scale and a shift.
	device := func(r geom.Rect) geom.Rect {
		sx, sy := float32(1), float32(1)
		if b.Width() > 0 && b.Height() > 0 {
			sx, sy = db.Width()/b.Width(), db.Height()/b.Height()
		}
		return geom.Rc(db.Min.X+(r.Min.X-b.Min.X)*sx, db.Min.Y+(r.Min.Y-b.Min.Y)*sy,
			db.Min.X+(r.Max.X-b.Min.X)*sx, db.Min.Y+(r.Max.Y-b.Min.Y)*sy)
	}
	dens := max(st.density, 0.5)
	hair := 1 / dens
	snap := func(f float32) float32 { return float32(math.Round(float64(f*dens))) / dens }
	m := &n.mesh
	m.feather = float64(hair)
	ox, oy := float64(b.Min.X), float64(b.Min.Y)

	grid := ui.ResolveColor(ui.ColorSeparator)
	faint := ui.ResolveColor(ui.Fade(ui.ColorSeparator, 0.5))

	ctx.PushClip(device(plot))
	for i, mk := range v.marks {
		s := mk.spec()
		c := st.cache[i].color
		switch s.kind {
		case kindBandX, kindBandY:
			r := plot
			if s.kind == kindBandX {
				r.Min.X, r.Max.X = float32(ox+st.plotX(s.a)), float32(ox+st.plotX(s.b))
			} else {
				r.Min.Y, r.Max.Y = float32(oy+st.plotY(s.b)), float32(oy+st.plotY(s.a))
			}
			ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: r.Intersect(plot), Color: scale(c, s.opacity)})
		}
	}
	for _, t := range st.yt {
		y := snap(float32(oy + st.plotY(t.v)))
		ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(plot.Min.X, y-hair/2, plot.Max.X, y+hair/2), Color: grid})
	}
	if len(v.cats) == 0 {
		for _, t := range st.xt {
			x := snap(float32(ox + st.plotX(t.v)))
			ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(x-hair/2, plot.Min.Y, x+hair/2, plot.Max.Y), Color: faint})
		}
	}
	clip := rect{float64(plot.Min.X) - 12, float64(plot.Min.Y) - 1e5, float64(plot.Max.X) + 12, float64(plot.Max.Y) + 1e5}
	for i, mk := range v.marks {
		s := mk.spec()
		c := &st.cache[i]
		switch s.kind {
		case kindLine:
			n.pieces(c.pts, s, ox, oy, func(ps []pt) {
				n.buf = polylines(ps, nil, clip, n.buf, func(q []pt) {
					if s.dash[0] > 0 {
						m.dash(q, float64(s.width), float64(s.dash[0]), float64(s.dash[1]), c.color)
					} else {
						m.stroke(q, float64(s.width), c.color)
					}
				})
			})
			m.emit(ctx, plot, c.color)
			if s.symbols {
				n.symbols(ctx, c, ox, oy, 6)
			}
		case kindArea:
			if c.stacked {
				n.stackedArea(ctx, s, c, ox, oy, plot)
				continue
			}
			base := n.baseline(oy)
			top := scale(c.color, s.opacity)
			fill := func(float64) render.Color { return top }
			if s.gradient {
				// Full at the top of the plot, nothing at the base.
				y0, y1 := float64(plot.Min.Y), base
				fill = func(y float64) render.Color {
					if y1 == y0 {
						return top
					}
					return scale(top, float32(min(max((y1-y)/(y1-y0), 0), 1)))
				}
			}
			n.pieces(c.pts, s, ox, oy, func(ps []pt) {
				n.buf = polylines(ps, nil, clip, n.buf, func(q []pt) {
					if len(q) == 1 {
						m.dot(q[0], 1.5, top)
						return
					}
					m.area(q, base, fill)
				})
			})
			m.emit(ctx, plot, top)
		case kindBar:
			n.paintBars(ctx, i, s, c, ox, oy, plot)
		case kindScatter:
			n.symbols(ctx, c, ox, oy, s.width)
		case kindRuleY, kindRuleX:
			var a, b pt
			if s.kind == kindRuleY {
				y := float64(snap(float32(oy + st.plotY(s.a))))
				a, b = pt{float64(plot.Min.X), y}, pt{float64(plot.Max.X), y}
			} else {
				x := float64(snap(float32(ox + st.plotX(s.a))))
				a, b = pt{x, float64(plot.Min.Y)}, pt{x, float64(plot.Max.Y)}
			}
			if s.dash[0] > 0 {
				m.dash([]pt{a, b}, float64(s.width), float64(s.dash[0]), float64(s.dash[1]), c.color)
			} else {
				m.stroke([]pt{a, b}, float64(s.width), c.color)
			}
			m.emit(ctx, plot, c.color)
		}
	}
	n.paintNotes(ctx, ox, oy, plot)
	n.paintInspection(ctx, ox, oy, plot)
	if d := st.drag; d.brushing {
		x0, x1 := float32(ox+st.plotX(min(d.b0, d.b1))), float32(ox+st.plotX(max(d.b0, d.b1)))
		r := geom.Rc(x0, plot.Min.Y, x1, plot.Max.Y)
		if v.zoomY {
			r.Min.Y, r.Max.Y = float32(oy+st.plotY(max(d.c0, d.c1))), float32(oy+st.plotY(min(d.c0, d.c1)))
			ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: r, Color: ui.ResolveColor(ui.ColorAccent), StrokeWidth: hair})
		}
		accent := ui.ResolveColor(ui.ColorAccent)
		ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: r, Color: scale(accent, 0.15)})
		ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(x0-hair/2, plot.Min.Y, x0+hair/2, plot.Max.Y), Color: accent})
		ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(x1-hair/2, plot.Min.Y, x1+hair/2, plot.Max.Y), Color: accent})
	}
	ctx.PopClip()

	// The base line of the plot, a little stronger than the grid.
	ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(plot.Min.X, snap(plot.Max.Y)-hair/2, plot.Max.X, snap(plot.Max.Y)+hair/2),
		Color: ui.ResolveColor(ui.Fade(ui.ColorSecondaryLabel, 0.5))})

	for k, r := range st.legendAt {
		mk := v.marks[st.plan.legendOf[k]].spec()
		n.swatch(ctx, at(r), mk.kind, st.cache[st.plan.legendOf[k]].color)
	}
	if !st.callout.IsEmpty() {
		box := at(st.callout)
		ctx.Add(render.Op{Kind: render.OpShadow, Bounds: box.Translate(geom.Pt(0, 2)), Color: render.RGBA(0, 0, 0, 40), CornerRadius: 8, Blur: 8})
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: box, Color: ui.ResolveColor(ui.ColorSurface), CornerRadius: 8})
		ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: box, Color: grid, CornerRadius: 8, StrokeWidth: hair})
		for k, row := range st.calloutRows() {
			if k < len(st.calloutAt) {
				n.swatch(ctx, at(st.calloutAt[k]), v.marks[row.mark].spec().kind, st.cache[row.mark].color)
			}
		}
	}
	ctx.PaintChildren()
}

// pieces calls fn with every run of the points of a line or an area that is
// not broken by a missing value or by a distance beyond the mark's MaxGap,
// in plot coordinates and interpolated as the mark says.
func (n *plotNode) pieces(pts []Point, s markSpec, ox, oy float64, fn func([]pt)) {
	st := n.st
	cur := n.ps[:0]
	flush := func() {
		if len(cur) > 0 {
			fn(n.interpolate(cur, s.interp))
		}
		cur = cur[:0]
	}
	prev := math.NaN()
	for _, p := range pts {
		if Missing(p.Y) {
			flush()
			prev = math.NaN()
			continue
		}
		if s.maxGap > 0 && !math.IsNaN(prev) && p.X-prev > s.maxGap {
			flush()
		}
		prev = p.X
		cur = append(cur, pt{finite(ox + st.plotX(p.X)), finite(oy + st.plotY(p.Y))})
	}
	flush()
	n.ps = cur
}

// interpolate returns ps with the steps or the curve of the interpolation
// in between, where the points are far enough apart for it to show.
func (n *plotNode) interpolate(ps []pt, i Interpolation) []pt {
	if i == Linear || len(ps) < 2 {
		return ps
	}
	if (ps[len(ps)-1].x-ps[0].x)/float64(len(ps)-1) < 3 {
		return ps
	}
	out := n.tmp[:0]
	switch i {
	case StepAfter, StepBefore, StepMiddle:
		for k := range ps {
			if k > 0 {
				a, b := ps[k-1], ps[k]
				switch i {
				case StepAfter:
					out = append(out, pt{b.x, a.y})
				case StepBefore:
					out = append(out, pt{a.x, b.y})
				default:
					mid := (a.x + b.x) / 2
					out = append(out, pt{mid, a.y}, pt{mid, b.y})
				}
			}
			out = append(out, ps[k])
		}
	case Monotone:
		out = monotone(out, ps)
	}
	n.tmp = out
	return out
}

// monotone samples the monotone cubic of Fritsch and Carlson through ps,
// which never overshoots its points.
func monotone(out, ps []pt) []pt {
	k := len(ps)
	d := make([]float64, k-1)
	for i := range k - 1 {
		d[i] = (ps[i+1].y - ps[i].y) / (ps[i+1].x - ps[i].x)
	}
	m := make([]float64, k)
	m[0], m[k-1] = d[0], d[k-2]
	for i := 1; i < k-1; i++ {
		if d[i-1]*d[i] <= 0 {
			m[i] = 0
		} else {
			m[i] = 3 * (ps[i+1].x - ps[i-1].x) /
				((2*ps[i+1].x-ps[i].x-ps[i-1].x)/d[i-1] + (ps[i+1].x-2*ps[i].x+ps[i-1].x)/d[i])
		}
	}
	for i := range k - 1 {
		a, b := ps[i], ps[i+1]
		h := b.x - a.x
		steps := min(max(int(h/3), 1), 24)
		for s := range steps {
			t := float64(s) / float64(steps)
			t2, t3 := t*t, t*t*t
			y := (2*t3-3*t2+1)*a.y + (t3-2*t2+t)*h*m[i] + (-2*t3+3*t2)*b.y + (t3-t2)*h*m[i+1]
			out = append(out, pt{a.x + h*t, y})
		}
	}
	return append(out, ps[k-1])
}

// baseline is where an area and a bar start: zero, or the edge of the plot
// nearest to it.
func (n *plotNode) baseline(oy float64) float64 {
	st := n.st
	return oy + st.plotY(min(max(0, st.ylo), st.yhi))
}

// symbols draws a dot of diameter d on every point that is far enough from
// the one before to be told apart.
func (n *plotNode) symbols(ctx *gift.PaintContext, c *markCache, ox, oy float64, d float32) {
	st := n.st
	r := d / 2
	last := float32(math.Inf(-1))
	plot := st.plot.Translate(geom.Pt(float32(ox), float32(oy)))
	for _, p := range c.pts {
		if Missing(p.Y) {
			continue
		}
		x, y := float32(ox+st.plotX(p.X)), float32(oy+st.plotY(p.Y))
		if x < plot.Min.X-r || x > plot.Max.X+r || y < plot.Min.Y-r || y > plot.Max.Y+r {
			continue
		}
		if c.key.buckets > 0 && x-last < 1/st.density && len(c.pts) > 4*c.key.buckets {
			continue
		}
		last = x
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(x-r, y-r, x+r, y+r), Color: c.color, CornerRadius: r})
	}
}

// paintBars draws the bars of mark i as layoutBars worked them out.
func (n *plotNode) paintBars(ctx *gift.PaintContext, i int, s markSpec, c *markCache, ox, oy float64, plot geom.Rect) {
	st := n.st
	hair := 1 / st.density
	for _, b := range st.bars {
		if b.mark != i {
			continue
		}
		x0, x1 := float32(ox+st.plotX(b.x0)), float32(ox+st.plotX(b.x1))
		if x1 < plot.Min.X || x0 > plot.Max.X {
			continue
		}
		y0, y1 := float32(oy+st.plotY(b.y1)), float32(oy+st.plotY(b.y0))
		if b.thin {
			x0 = float32(math.Round(float64(x0*st.density))) / st.density
			r := geom.Rc(x0, y0, x0+hair, max(y1, y0+hair))
			ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: r, Color: c.color})
			continue
		}
		// Stacked segments and grouped bars are kept a hair apart.
		r := geom.Rc(x0, y0, x1, y1)
		if st.v.stacking != Unstacked {
			r = geom.Rc(x0+hair/2, y0+hair/2, x1-hair/2, y1-hair/2)
		}
		if r.Width() <= 0 || r.Height() <= 0 {
			continue
		}
		rad := min(s.radius, r.Width()/4, r.Height()/2)
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: r, Color: c.color, CornerRadius: rad})
	}
}

// stackedArea fills a stacked area between its lower and its upper edge.
func (n *plotNode) stackedArea(ctx *gift.PaintContext, s markSpec, c *markCache, ox, oy float64, plot geom.Rect) {
	st := n.st
	m := &n.mesh
	col := scale(c.color, 0.85)
	if s.opaque {
		col = scale(c.color, s.opacity)
	}
	var top, bottom []pt
	flush := func() {
		m.band(top, bottom, col)
		top, bottom = top[:0], bottom[:0]
	}
	prev := math.NaN()
	for k, p := range c.pts {
		if Missing(p.Y) || k >= len(c.upper) || Missing(c.upper[k]) {
			flush()
			prev = math.NaN()
			continue
		}
		if s.maxGap > 0 && !math.IsNaN(prev) && p.X-prev > s.maxGap {
			flush()
		}
		prev = p.X
		x := finite(ox + st.plotX(p.X))
		top = append(top, pt{x, finite(oy + st.plotY(c.upper[k]))})
		bottom = append(bottom, pt{x, finite(oy + st.plotY(c.lower[k]))})
	}
	flush()
	m.emit(ctx, plot, col)
}

// barStep is the distance between two bars in x: one category, the step of
// the data, or the distance of the points shown.
func (st *state) barStep(s markSpec, c *markCache) float64 {
	if len(st.v.cats) > 0 {
		return 1
	}
	if d, ok := s.data.(*Data); ok {
		if step := d.step(); step > 0 {
			return step
		}
	}
	var prev float64
	best := math.Inf(1)
	for i, p := range c.pts {
		if i > 0 && p.X > prev {
			best = min(best, p.X-prev)
		}
		prev = p.X
	}
	if math.IsInf(best, 1) {
		return (st.hi - st.lo) / 10
	}
	return best
}

// swatch draws the colour of a series in front of its name: a dot for a
// line or a scatter, a square for an area or a bar.
func (n *plotNode) swatch(ctx *gift.PaintContext, r geom.Rect, k kind, c render.Color) {
	cy := r.Min.Y + r.Height()/2
	sw := geom.Rc(r.Min.X+1, cy-4, r.Min.X+9, cy+4)
	rad := float32(4)
	if k == kindBar || k == kindArea || k == kindSector {
		rad = 2
	}
	ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: sw, Color: c, CornerRadius: rad})
}

// paintInspection draws the line of the inspected x and a ringed dot on
// the nearest point of every series.
func (n *plotNode) paintInspection(ctx *gift.PaintContext, ox, oy float64, plot geom.Rect) {
	st := n.st
	if !st.in.on {
		return
	}
	hair := 1 / max(st.density, 0.5)
	x := float32(math.Round(float64((float32(ox+st.plotX(st.in.at)))*st.density))) / st.density
	ctx.Add(render.Op{Kind: render.OpFillRect, Bounds: geom.Rc(x-hair/2, plot.Min.Y, x+hair/2, plot.Max.Y),
		Color: ui.ResolveColor(ui.ColorSecondaryLabel)})
	ring := ui.ResolveColor(ui.ColorSurface)
	for i, mk := range st.v.marks {
		c := &st.cache[i]
		if k := mk.spec().kind; k > kindScatter || k == kindBar || c.stacked || !c.hasNearest || Missing(c.nearest.Y) {
			continue
		}
		px, py := float32(ox+st.plotX(c.nearest.X)), float32(oy+st.plotY(c.nearest.Y))
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(px-5.5, py-5.5, px+5.5, py+5.5), Color: ring, CornerRadius: 5.5})
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: geom.Rc(px-3.5, py-3.5, px+3.5, py+3.5), Color: c.color, CornerRadius: 3.5})
	}
}

// funcKey stands for a [Func] among the series, which as a function is no
// key of a map.
type funcKey int

func (funcKey) series() {}

// finite keeps a position of a value far off the plot, 1e300 say, at a
// distance a line can still be clipped at.
func finite(f float64) float64 { return min(max(f, -1e7), 1e7) }

// scale returns c at f of its opacity; the colour is premultiplied.
func scale(c render.Color, f float32) render.Color {
	return render.Color{R: c.R * f, G: c.G * f, B: c.B * f, A: c.A * f}
}

// inspectValues finds the point of every series nearest to the inspected
// x, for the callout the build makes.
func (st *state) inspectValues() {
	st.in.at = st.in.x
	snapped := false
	for i, m := range st.v.marks {
		c := &st.cache[i]
		c.hasNearest = false
		if !st.in.on {
			continue
		}
		x := st.in.x
		switch d := m.spec().data.(type) {
		case *Data:
			if k, ok := d.nearest(x); ok {
				c.nearest, c.hasNearest = d.At(k), true
			}
		case *Source:
			if ld := st.loads[d]; ld != nil {
				c.nearest, c.hasNearest = nearestOf(ld.pts, x)
			}
		case Func:
			c.nearest, c.hasNearest = Point{x, d(x)}, true
		}
		if c.hasNearest && !snapped && m.spec().kind <= kindScatter {
			st.in.at, snapped = c.nearest.X, true
		}
	}
}

// nearestOf returns the point of ps nearest to x that is not a break.
func nearestOf(ps []Point, x float64) (Point, bool) {
	best, ok := Point{}, false
	for _, p := range ps {
		if Missing(p.Y) {
			continue
		}
		if !ok || math.Abs(p.X-x) < math.Abs(best.X-x) {
			best, ok = p, true
		}
	}
	return best, ok
}
