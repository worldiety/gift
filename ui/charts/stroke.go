package charts

import (
	"math"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/render"
)

// pt is a position on the plot in logical pixels. Positions are worked out
// in float64 and clipped to the plot before they become float32, so that a
// value of 1e300 or a neighbour a million widths away cannot break a line.
type pt struct{ x, y float64 }

// mesh collects the triangles of one colour for one operation.
type mesh struct {
	vs []render.Vertex
	// feather is one device pixel in logical pixels: the soft edge of a
	// line, from its colour to nothing.
	feather float64
}

func (m *mesh) tri(a, b, c pt, ca, cb, cc render.Color) {
	m.vs = append(m.vs,
		render.Vertex{X: float32(a.x), Y: float32(a.y), Color: ca},
		render.Vertex{X: float32(b.x), Y: float32(b.y), Color: cb},
		render.Vertex{X: float32(c.x), Y: float32(c.y), Color: cc})
}

func (m *mesh) quad(a, b, c, d pt, ca, cb, cc, cd render.Color) {
	m.tri(a, b, c, ca, cb, cc)
	m.tri(a, c, d, ca, cc, cd)
}

// emit adds the triangles as one operation and empties the mesh.
func (m *mesh) emit(ctx *gift.PaintContext, bounds geom.Rect, c render.Color) {
	if len(m.vs) < 3 {
		m.vs = m.vs[:0]
		return
	}
	first := ctx.VerticesLen()
	for _, v := range m.vs {
		ctx.AppendVertex(v)
	}
	ctx.Add(render.Op{Kind: render.OpTriangles, Bounds: bounds, Color: c, Glyphs: first, GlyphCount: uint32(len(m.vs))})
	m.vs = m.vs[:0]
}

// stroke draws the polyline ps of width w: joins mitred where the line
// bends gently, and where it turns sharply, as the column of an M4 does,
// the segments run on by half the width past the corner so that the corner
// is filled. The edges are soft over one device pixel.
func (m *mesh) stroke(ps []pt, w float64, c render.Color) {
	ps = dedupe(ps)
	if len(ps) == 1 {
		m.dot(ps[0], w*0.75, c)
		return
	}
	hw := max(w/2-m.feather/2, m.feather/4)
	start := 0
	for start < len(ps)-1 {
		// One strip runs from start up to the first sharp corner.
		end := start + 1
		for end < len(ps)-1 && !sharp(ps[end-1], ps[end], ps[end+1]) {
			end++
		}
		m.strip(ps[start:end+1], hw, start > 0, end < len(ps)-1, c)
		start = end
	}
}

// strip strokes a polyline without sharp corners; extendStart and extendEnd
// run its ends on past a sharp corner.
func (m *mesh) strip(ps []pt, hw float64, extendStart, extendEnd bool, c render.Color) {
	n := len(ps)
	f := m.feather
	clear := render.Color{}
	var pl, pr, plo, pro pt
	for i := 0; i < n; i++ {
		p := ps[i]
		var nx, ny, scale float64
		switch {
		case i == 0:
			nx, ny = normal(ps[0], ps[1])
			scale = 1
			if extendStart {
				dx, dy := ny, -nx
				p = pt{p.x - dx*hw, p.y - dy*hw}
			}
		case i == n-1:
			nx, ny = normal(ps[n-2], ps[n-1])
			scale = 1
			if extendEnd {
				dx, dy := ny, -nx
				p = pt{p.x + dx*hw, p.y + dy*hw}
			}
		default:
			ax, ay := normal(ps[i-1], ps[i])
			bx, by := normal(ps[i], ps[i+1])
			nx, ny = ax+bx, ay+by
			l := math.Hypot(nx, ny)
			nx, ny = nx/l, ny/l
			scale = 1 / max(nx*bx+ny*by, 0.4)
		}
		l := pt{p.x + nx*hw*scale, p.y + ny*hw*scale}
		r := pt{p.x - nx*hw*scale, p.y - ny*hw*scale}
		lo := pt{p.x + nx*(hw+f)*scale, p.y + ny*(hw+f)*scale}
		ro := pt{p.x - nx*(hw+f)*scale, p.y - ny*(hw+f)*scale}
		if i > 0 {
			m.quad(pl, l, r, pr, c, c, c, c)
			m.quad(plo, lo, l, pl, clear, clear, c, c)
			m.quad(pr, r, ro, pro, c, c, clear, clear)
		}
		pl, pr, plo, pro = l, r, lo, ro
	}
}

// sharp reports whether the line turns by more than about 110° at b.
func sharp(a, b, c pt) bool {
	ux, uy := b.x-a.x, b.y-a.y
	vx, vy := c.x-b.x, c.y-b.y
	lu, lv := math.Hypot(ux, uy), math.Hypot(vx, vy)
	return (ux*vx+uy*vy)/(lu*lv) < -0.35
}

func normal(a, b pt) (float64, float64) {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	return -dy / l, dx / l
}

// dedupe drops points that lie on the one before, in place.
func dedupe(ps []pt) []pt {
	out := ps[:0]
	for i, p := range ps {
		if i > 0 {
			q := out[len(out)-1]
			if math.Abs(p.x-q.x) < 1e-3 && math.Abs(p.y-q.y) < 1e-3 {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// dot draws a disc of radius r as a fan with a soft rim: a value alone
// between two missing ones, which a line would not show at all.
func (m *mesh) dot(p pt, r float64, c render.Color) {
	const n = 12
	clear := render.Color{}
	f := m.feather
	for k := range n {
		a0 := 2 * math.Pi * float64(k) / n
		a1 := 2 * math.Pi * float64(k+1) / n
		i0 := pt{p.x + math.Cos(a0)*r, p.y + math.Sin(a0)*r}
		i1 := pt{p.x + math.Cos(a1)*r, p.y + math.Sin(a1)*r}
		o0 := pt{p.x + math.Cos(a0)*(r+f), p.y + math.Sin(a0)*(r+f)}
		o1 := pt{p.x + math.Cos(a1)*(r+f), p.y + math.Sin(a1)*(r+f)}
		m.tri(p, i0, i1, c, c, c)
		m.quad(i0, o0, o1, i1, c, clear, clear, c)
	}
}

// dash cuts ps into its dashes, on and off logical pixels long, and strokes
// them.
func (m *mesh) dash(ps []pt, w, on, off float64, c render.Color) {
	if on <= 0 || off <= 0 {
		m.stroke(ps, w, c)
		return
	}
	var cur []pt
	pos, drawing := 0.0, true
	for i := 1; i < len(ps); i++ {
		a, b := ps[i-1], ps[i]
		seg := math.Hypot(b.x-a.x, b.y-a.y)
		t := 0.0
		if drawing && len(cur) == 0 {
			cur = append(cur, a)
		}
		for t < seg {
			limit := on
			if !drawing {
				limit = off
			}
			step := min(limit-pos, seg-t)
			t += step
			pos += step
			p := pt{a.x + (b.x-a.x)*t/seg, a.y + (b.y-a.y)*t/seg}
			if drawing {
				cur = append(cur, p)
			}
			if pos >= limit {
				if drawing && len(cur) > 1 {
					m.stroke(cur, w, c)
				}
				cur = cur[:0]
				drawing, pos = !drawing, 0
				if drawing {
					cur = append(cur, p)
				}
			}
		}
	}
	if drawing && len(cur) > 1 {
		m.stroke(cur, w, c)
	}
}

// area fills between the polyline ps and the horizontal line at base, in
// the colour fill gives each height: a vertical gradient, which a triangle
// reproduces exactly when its corners have the colours of their heights. A
// segment that crosses the base is split there.
func (m *mesh) area(ps []pt, base float64, fill func(y float64) render.Color) {
	for i := 1; i < len(ps); i++ {
		a, b := ps[i-1], ps[i]
		if (a.y-base)*(b.y-base) < 0 {
			t := (base - a.y) / (b.y - a.y)
			c := pt{a.x + (b.x-a.x)*t, base}
			m.trapezoid(a, c, base, fill)
			m.trapezoid(c, b, base, fill)
			continue
		}
		m.trapezoid(a, b, base, fill)
	}
}

// band fills between two polylines of the same xs, a stacked area between
// the one below it and its own top.
func (m *mesh) band(top, bottom []pt, c render.Color) {
	for i := 1; i < len(top) && i < len(bottom); i++ {
		m.quad(top[i-1], top[i], bottom[i], bottom[i-1], c, c, c, c)
	}
}

func (m *mesh) trapezoid(a, b pt, base float64, fill func(y float64) render.Color) {
	fb := fill(base)
	m.quad(a, b, pt{b.x, base}, pt{a.x, base}, fill(a.y), fill(b.y), fb, fb)
}

// clipSegment clips the segment from a to b to r, Liang–Barsky, and reports
// whether anything is left of it.
func clipSegment(a, b pt, r rect) (pt, pt, bool) {
	t0, t1 := 0.0, 1.0
	dx, dy := b.x-a.x, b.y-a.y
	for _, e := range [4][2]float64{{-dx, a.x - r.x0}, {dx, r.x1 - a.x}, {-dy, a.y - r.y0}, {dy, r.y1 - a.y}} {
		p, q := e[0], e[1]
		if p == 0 {
			if q < 0 {
				return a, b, false
			}
			continue
		}
		t := q / p
		if p < 0 {
			if t > t1 {
				return a, b, false
			}
			t0 = max(t0, t)
		} else {
			if t < t0 {
				return a, b, false
			}
			t1 = min(t1, t)
		}
	}
	return pt{a.x + dx*t0, a.y + dy*t0}, pt{a.x + dx*t1, a.y + dy*t1}, true
}

// rect is a rectangle in float64 logical pixels.
type rect struct{ x0, y0, x1, y1 float64 }

func (r rect) inset(d float64) rect { return rect{r.x0 + d, r.y0 + d, r.x1 - d, r.y1 - d} }

func (r rect) contains(p pt) bool { return p.x >= r.x0 && p.x <= r.x1 && p.y >= r.y0 && p.y <= r.y1 }

func rectOf(g geom.Rect) rect {
	return rect{float64(g.Min.X), float64(g.Min.Y), float64(g.Max.X), float64(g.Max.Y)}
}

// polylines splits ps at the breaks, if there are any, clips every piece to
// r and calls fn with each piece that is left; a piece that leaves r and
// comes back is two. buf is reused for the pieces.
func polylines(ps []pt, breaks []bool, r rect, buf []pt, fn func([]pt)) []pt {
	cur := buf[:0]
	flush := func() {
		if len(cur) > 0 {
			fn(cur)
		}
		cur = cur[:0]
	}
	broken := func(i int) bool { return breaks != nil && breaks[i] }
	for i := range ps {
		if broken(i) {
			flush()
			continue
		}
		if i == 0 || broken(i-1) {
			if r.contains(ps[i]) {
				cur = append(cur, ps[i])
			}
			continue
		}
		a, b := ps[i-1], ps[i]
		ca, cb, ok := clipSegment(a, b, r)
		if !ok {
			flush()
			continue
		}
		if ca != a || len(cur) == 0 {
			flush()
			cur = append(cur, ca)
		}
		cur = append(cur, cb)
		if cb != b {
			flush()
		}
	}
	flush()
	return cur
}
