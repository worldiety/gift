package charts

import (
	"math"
	"strconv"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// polar reports whether the chart is a pie: it has sectors.
func (st *state) polar() bool {
	for _, m := range st.v.marks {
		if m.spec().kind == kindSector {
			return true
		}
	}
	return false
}

// slice is a sector as the plot draws it: its angles, clockwise from twelve
// o'clock, and its share.
type slice struct {
	mark   int
	a0, a1 float64
	share  float64
	inner  float64
	inset  float64
	value  float64
}

// slices works out the angles of the sectors with a positive value.
func (st *state) slices() []slice {
	total := 0.0
	for _, m := range st.v.marks {
		if s := m.spec(); s.kind == kindSector && s.a > 0 && !Missing(s.a) {
			total += s.a
		}
	}
	st.pie = st.pie[:0]
	if total <= 0 {
		return st.pie
	}
	at := 0.0
	for i, m := range st.v.marks {
		s := m.spec()
		if s.kind != kindSector || !(s.a > 0) || Missing(s.a) {
			continue
		}
		share := s.a / total
		st.pie = append(st.pie, slice{mark: i, a0: at * 2 * math.Pi, a1: (at + share) * 2 * math.Pi,
			share: share, inner: float64(s.inner), inset: float64(s.inset), value: s.a})
		at += share
	}
	return st.pie
}

func percent(share float64) string {
	p := share * 100
	if p > 0 && p < 1 {
		return "<1 %"
	}
	return strconv.Itoa(int(math.Round(p))) + " %"
}

// polarChildren are the children of a pie: the legend, the labels in the
// slices, the centre, the inspection, the surface and a status.
func (st *state) polarChildren() []gift.View {
	v := st.v
	var kids []gift.View
	p := plan{xTitle: -1, yTitle: -1, surface: -1, reset: -1, status: -1, center: -1}
	group := func(r *[2]int, add func()) {
		r[0] = len(kids)
		add()
		r[1] = len(kids)
	}
	pie := st.slices()
	group(&p.legend, func() {
		if v.legend == -1 {
			return
		}
		for _, sl := range pie {
			if s := v.marks[sl.mark].spec(); s.name != "" {
				p.legendOf = append(p.legendOf, sl.mark)
				kids = append(kids, ui.Text(s.name).FontSize(labelSize).Foreground(ui.ColorSecondaryLabel).MaxLines(1))
			}
		}
	})
	group(&p.labels, func() {
		for _, sl := range pie {
			s := v.marks[sl.mark].spec()
			text := s.label
			if s.percent {
				text = percent(sl.share)
			}
			if text == "" {
				continue
			}
			p.labelOf = append(p.labelOf, sl.mark)
			kids = append(kids, ui.Text(text).FontSize(titleSize).Foreground(ui.RGB(255, 255, 255)).MaxLines(1))
		}
	})
	if v.center != nil {
		p.center = len(kids)
		kids = append(kids, v.center)
	}
	group(&p.callout, func() {
		if !st.in.on || st.in.sector < 0 || st.in.sector >= len(pie) {
			return
		}
		sl := pie[st.in.sector]
		s := v.marks[sl.mark].spec()
		name := s.name
		if name == "" {
			name = "Value"
		}
		kids = append(kids,
			ui.Text(name).FontSize(labelSize).Foreground(ui.ColorSecondaryLabel).MaxLines(1),
			ui.HStack(
				ui.Text(v.formatY(sl.value)).FontSize(titleSize).Foreground(ui.ColorLabel).MaxLines(1),
				ui.Text(percent(sl.share)).FontSize(titleSize).Foreground(ui.ColorSecondaryLabel).MaxLines(1),
			).Gap(8))
	})
	p.surface = len(kids)
	kids = append(kids, surface{n: st.node})
	if len(pie) == 0 {
		p.status = len(kids)
		kids = append(kids, ui.Text("No Data").FontSize(titleSize).Foreground(ui.ColorSecondaryLabel))
	}
	st.plan = p
	return kids
}

// layoutPolar lays out a pie: the circle as large as the room leaves it,
// the legend beside it when the chart is wide and below it otherwise.
func (n *plotNode) layoutPolar(ctx *gift.LayoutContext, size geom.Size) {
	st := n.st
	p := st.plan
	pie := st.slices()
	for k, sl := range pie {
		s := st.v.marks[sl.mark].spec()
		c := seriesColor(k)
		if s.hasColor {
			c = ui.ResolveColor(s.color)
		}
		st.cache[sl.mark].color = c
	}
	loose := geom.Constraints{Max: size}
	var sizes []geom.Size
	var lw, lh float32
	for i := p.legend[0]; i < p.legend[1]; i++ {
		s := ctx.Measure(i, loose)
		sizes = append(sizes, s)
		lw, lh = max(lw, s.W+14), lh+s.H+4
	}
	area := geom.Rect{Max: geom.Pt(size.W, size.H)}
	st.legendAt = st.legendAt[:0]
	side := len(sizes) > 0 && size.W-lw-2*pad >= size.H
	switch {
	case side:
		area.Max.X = size.W - lw - 2*pad
		y := (size.H - lh) / 2
		for k, i := 0, p.legend[0]; i < p.legend[1]; k, i = k+1, i+1 {
			r := geom.Rect{Min: geom.Pt(area.Max.X+2*pad, y), Max: geom.Pt(area.Max.X+2*pad+sizes[k].W+14, y+sizes[k].H)}
			ctx.Place(i, geom.Pt(r.Min.X+14, r.Min.Y))
			st.legendAt = append(st.legendAt, r)
			y += sizes[k].H + 4
		}
	case len(sizes) > 0:
		legend := n.layoutLegend(ctx, size.W)
		h := legend[len(legend)-1].Max.Y
		area.Max.Y = size.H - h - pad
		for k, i := 0, p.legend[0]; i < p.legend[1]; k, i = k+1, i+1 {
			r := legend[k].Translate(geom.Pt(0, area.Max.Y+pad))
			ctx.Place(i, geom.Pt(r.Min.X+14, r.Min.Y))
			st.legendAt = append(st.legendAt, r)
		}
	}
	const pop = 6
	r := max(min(area.Width(), area.Height())/2-pop, 1)
	c := geom.Pt(area.Min.X+area.Width()/2, area.Min.Y+area.Height()/2)
	st.plot = geom.Rect{Min: geom.Pt(c.X-r, c.Y-r), Max: geom.Pt(c.X+r, c.Y+r)}

	for k, i := 0, p.labels[0]; i < p.labels[1]; k, i = k+1, i+1 {
		s := ctx.Measure(i, loose)
		var sl slice
		for _, x := range pie {
			if x.mark == p.labelOf[k] {
				sl = x
			}
		}
		mid := (sl.a0 + sl.a1) / 2
		// In a pie a little beyond the middle of the radius, in a ring in
		// the middle of the ring.
		rr := float64(r) * 0.62
		if sl.inner > 0 {
			rr = float64(r) * (1 + sl.inner) / 2
		}
		at := polarPoint(c, rr, mid)
		// A label that does not fit its slice is left out.
		arc := (sl.a1 - sl.a0) * rr
		if float64(s.W) > arc*1.4 || float64(s.H) > float64(r)*(1-sl.inner)*0.9 {
			ctx.Place(i, hidden)
			continue
		}
		ctx.Place(i, geom.Pt(at.X-s.W/2, at.Y-s.H/2))
	}
	if p.center >= 0 {
		in := float32(1)
		for _, sl := range pie {
			in = min(in, float32(sl.inner))
		}
		d := 2 * r * in * 0.8
		s := ctx.Measure(p.center, geom.Constraints{Max: geom.Sz(max(d, 1), max(d, 1))})
		ctx.Place(p.center, geom.Pt(c.X-s.W/2, c.Y-s.H/2))
	}
	st.callout = geom.Rect{}
	if p.callout[1] > p.callout[0] {
		const inset = 8
		var w, h float32
		var ss []geom.Size
		for i := p.callout[0]; i < p.callout[1]; i++ {
			s := ctx.Measure(i, loose)
			ss = append(ss, s)
			w, h = max(w, s.W), h+s.H+2
		}
		w, h = w+2*inset, h-2+2*inset
		at := st.in.pos.Add(geom.Pt(14, 14))
		at.X = min(max(at.X, 0), size.W-w)
		at.Y = min(max(at.Y, 0), size.H-h)
		st.callout = geom.Rect{Min: at, Max: at.Add(geom.Pt(w, h))}
		y := at.Y + inset
		for k, i := 0, p.callout[0]; i < p.callout[1]; k, i = k+1, i+1 {
			ctx.Place(i, geom.Pt(at.X+inset, y))
			y += ss[k].H + 2
		}
	}
	if p.status >= 0 {
		s := ctx.Measure(p.status, loose)
		ctx.Place(p.status, geom.Pt(c.X-s.W/2, c.Y-s.H/2))
	}
	if p.surface >= 0 {
		ctx.Measure(p.surface, geom.Constraints{Min: size, Max: size})
		ctx.Place(p.surface, geom.Point{})
	}
	n.invalidate = ctx.Invalidator()
}

// polarPoint is the point at radius r and angle a, clockwise from twelve
// o'clock, about c.
func polarPoint(c geom.Point, r, a float64) geom.Point {
	return geom.Pt(c.X+float32(r*math.Sin(a)), c.Y-float32(r*math.Cos(a)))
}

// paintPolar draws the slices; the one inspected stands out of the circle
// and the others fade.
func (n *plotNode) paintPolar(ctx *gift.PaintContext) {
	st := n.st
	b := ctx.Bounds()
	m := &n.mesh
	m.feather = 1 / float64(max(st.density, 0.5))
	plot := st.plot.Translate(b.Min)
	c := pt{float64(plot.Min.X + plot.Width()/2), float64(plot.Min.Y + plot.Height()/2)}
	r := float64(plot.Width() / 2)
	sel := -1
	if st.in.on {
		sel = st.in.sector
	}
	for k, sl := range st.slices() {
		col := st.cache[sl.mark].color
		at := c
		if sel >= 0 && k != sel {
			col = scale(col, 0.4)
		}
		if k == sel {
			mid := (sl.a0 + sl.a1) / 2
			at = pt{c.x + 6*math.Sin(mid), c.y - 6*math.Cos(mid)}
		}
		m.sector(at, r*sl.inner, r, sl.a0, sl.a1, sl.inset, col)
		m.emit(ctx, plot.Outset(geom.Insets{Left: 8, Top: 8, Right: 8, Bottom: 8}), col)
	}
	for k, rr := range st.legendAt {
		n.swatch(ctx, rr.Translate(b.Min), kindSector, st.cache[st.plan.legendOf[k]].color)
	}
	if !st.callout.IsEmpty() {
		box := st.callout.Translate(b.Min)
		ctx.Add(render.Op{Kind: render.OpShadow, Bounds: box.Translate(geom.Pt(0, 2)), Color: render.RGBA(0, 0, 0, 40), CornerRadius: 8, Blur: 8})
		ctx.Add(render.Op{Kind: render.OpFillRoundRect, Bounds: box, Color: ui.ResolveColor(ui.ColorSurface), CornerRadius: 8})
		ctx.Add(render.Op{Kind: render.OpStrokeRoundRect, Bounds: box, Color: ui.ResolveColor(ui.ColorSeparator), CornerRadius: 8, StrokeWidth: 1 / max(st.density, 0.5)})
	}
	ctx.PaintChildren()
}

// sectorAt returns the slice under the local position p, and false
// outside the ring.
func (st *state) sectorAt(p geom.Point) (int, bool) {
	c := geom.Pt(st.plot.Min.X+st.plot.Width()/2, st.plot.Min.Y+st.plot.Height()/2)
	r := float64(st.plot.Width() / 2)
	dx, dy := float64(p.X-c.X), float64(p.Y-c.Y)
	d := math.Hypot(dx, dy)
	a := math.Atan2(dx, -dy)
	if a < 0 {
		a += 2 * math.Pi
	}
	for k, sl := range st.pie {
		if d <= r+6 && d >= r*sl.inner && a >= sl.a0 && a < sl.a1 {
			return k, true
		}
	}
	return -1, false
}

// sector fills the slice from a0 to a1 between the radii r0 and r1 about
// c, with soft edges, its sides inset by inset/2 pixels each so that two
// slices are inset pixels apart all along.
func (m *mesh) sector(c pt, r0, r1, a0, a1, inset float64, col render.Color) {
	clear := render.Color{}
	f := m.feather
	sweep := a1 - a0
	steps := max(int(math.Ceil(sweep*r1/3)), 2)
	// The sides move in parallel by half the inset: at radius r that is an
	// angle of inset/2/r.
	side := func(r float64) (float64, float64) {
		if inset <= 0 || r <= 0 {
			return a0, a1
		}
		d := min(inset/2/r, sweep/2*0.95)
		return a0 + d, a1 - d
	}
	at := func(r, a float64) pt { return pt{c.x + r*math.Sin(a), c.y - r*math.Cos(a)} }
	o0, o1 := side(r1)
	oo0, oo1 := side(r1 + f)
	i0, i1 := side(r0)
	center := c
	if r0 <= 0 && inset > 0 && sweep < 2*math.Pi {
		// The tip of a pie slice moves out along its middle.
		mid := (a0 + a1) / 2
		d := min(inset/2/math.Max(math.Sin(sweep/2), 0.05), r1*0.3)
		center = pt{c.x + d*math.Sin(mid), c.y - d*math.Cos(mid)}
	}
	for k := range steps {
		t0, t1 := float64(k)/float64(steps), float64(k+1)/float64(steps)
		a, b := at(r1, o0+(o1-o0)*t0), at(r1, o0+(o1-o0)*t1)
		ao, bo := at(r1+f, oo0+(oo1-oo0)*t0), at(r1+f, oo0+(oo1-oo0)*t1)
		var ia, ib pt
		if r0 > 0 {
			ia, ib = at(r0, i0+(i1-i0)*t0), at(r0, i0+(i1-i0)*t1)
			m.quad(ia, a, b, ib, col, col, col, col)
			ii0, ii1 := side(r0 - f)
			iao, ibo := at(r0-f, ii0+(ii1-ii0)*t0), at(r0-f, ii0+(ii1-ii0)*t1)
			m.quad(iao, ia, ib, ibo, clear, col, col, clear)
		} else {
			m.tri(center, a, b, col, col, col)
		}
		m.quad(a, ao, bo, b, col, clear, clear, col)
	}
	if sweep >= 2*math.Pi-1e-9 {
		return
	}
	// The soft sides.
	for _, e := range [2]struct {
		a   float64
		dir float64
	}{{o0, -1}, {o1, 1}} {
		inner := center
		if r0 > 0 {
			ai, _ := side(r0)
			if e.dir > 0 {
				_, ai = side(r0)
			}
			inner = at(r0, ai)
		}
		outer := at(r1, e.a)
		// The normal of the side, pointing out of the slice.
		nx, ny := math.Cos(e.a)*e.dir, math.Sin(e.a)*e.dir
		m.quad(inner, outer, pt{outer.x + nx*f, outer.y + ny*f}, pt{inner.x + nx*f, inner.y + ny*f}, col, col, clear, clear)
	}
}
