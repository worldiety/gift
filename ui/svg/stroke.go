package svg

import (
	"math"

	"golang.org/x/image/vector"

	"github.com/worldiety/gift/internal/iconsvg"
)

// pt is a point on the canvas.
type pt struct{ x, y float64 }

// poly is a subpath flattened to a polyline on the canvas.
type poly struct {
	pts    []pt
	closed bool
}

// tolerance is how far a flattened curve may stray from the curve, in
// pixels.
const tolerance = 0.1

// flatten converts ops, transformed by m, into polylines on the canvas.
func flatten(ops []iconsvg.Op, m matrix) []poly {
	var out []poly
	var cur poly
	// After a Z the path goes on from where the subpath began.
	var restart pt
	hasRestart := false
	at := func(p [2]float64) pt {
		x, y := m.apply(p[0], p[1])
		return pt{clampCoord(x), clampCoord(y)}
	}
	flush := func() {
		if len(cur.pts) > 0 {
			out = append(out, cur)
		}
		cur = poly{}
	}
	begun := func() bool {
		if len(cur.pts) == 0 && hasRestart {
			cur.pts = append(cur.pts, restart)
		}
		return len(cur.pts) > 0
	}
	for _, op := range ops {
		switch op.Kind {
		case 'M':
			flush()
			hasRestart = false
			cur.pts = append(cur.pts, at(op.P[0]))
		case 'L':
			if begun() {
				cur.pts = append(cur.pts, at(op.P[0]))
			}
		case 'C':
			if !begun() {
				continue
			}
			p0, c1, c2, p := cur.pts[len(cur.pts)-1], at(op.P[0]), at(op.P[1]), at(op.P[2])
			// The bound on the error of uniform subdivision of a cubic.
			ax, ay := p0.x-2*c1.x+c2.x, p0.y-2*c1.y+c2.y
			bx, by := c1.x-2*c2.x+p.x, c1.y-2*c2.y+p.y
			d := math.Sqrt(max(ax*ax+ay*ay, bx*bx+by*by))
			n := min(max(int(math.Ceil(math.Sqrt(0.75*d/tolerance))), 1), 256)
			for i := 1; i <= n; i++ {
				t := float64(i) / float64(n)
				u := 1 - t
				a, b, c, e := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
				cur.pts = append(cur.pts, pt{a*p0.x + b*c1.x + c*c2.x + e*p.x, a*p0.y + b*c1.y + c*c2.y + e*p.y})
			}
		case 'Z':
			if len(cur.pts) > 0 {
				restart, hasRestart = cur.pts[0], true
				cur.closed = true
				flush()
			}
		}
	}
	flush()
	return out
}

// polyBounds is the box of the polylines, grown by pad.
func polyBounds(ps []poly, pad float64) [4]float64 {
	b := [4]float64{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, p := range ps {
		for _, q := range p.pts {
			b[0], b[1], b[2], b[3] = min(b[0], q.x), min(b[1], q.y), max(b[2], q.x), max(b[3], q.y)
		}
	}
	return [4]float64{b[0] - pad, b[1] - pad, b[2] + pad, b[3] + pad}
}

// dash cuts the polylines into dashes of the lengths of pattern, scaled by
// k, starting offset into the pattern.
func dash(ps []poly, pattern []float64, offset, k float64) []poly {
	total := 0.0
	pat := make([]float64, 0, 2*len(pattern))
	for _, d := range pattern {
		pat = append(pat, d*k)
		total += d * k
	}
	if len(pat)%2 == 1 {
		pat = append(pat, pat...)
		total *= 2
	}
	if total <= 0 {
		return ps
	}
	var out []poly
	for _, p := range ps {
		pts := p.pts
		if p.closed && len(pts) > 0 {
			pts = append(append([]pt(nil), pts...), pts[0])
		}
		// Where in the pattern the subpath begins.
		pos := math.Mod(offset, total)
		if pos < 0 {
			pos += total
		}
		idx := 0
		for pos >= pat[idx] {
			pos -= pat[idx]
			idx = (idx + 1) % len(pat)
		}
		on := idx%2 == 0
		var cur []pt
		if on && len(pts) > 0 {
			cur = append(cur, pts[0])
		}
		for i := 1; i < len(pts); i++ {
			a, b := pts[i-1], pts[i]
			seg := math.Hypot(b.x-a.x, b.y-a.y)
			t := 0.0
			for seg-t > 0 {
				left := pat[idx] - pos
				step := min(left, seg-t)
				t += step
				pos += step
				q := pt{a.x + (b.x-a.x)*t/seg, a.y + (b.y-a.y)*t/seg}
				if on {
					cur = append(cur, q)
				}
				if pos >= pat[idx]-1e-9 {
					if on && len(cur) > 1 {
						out = append(out, poly{pts: cur})
					}
					cur = nil
					idx = (idx + 1) % len(pat)
					pos = 0
					on = !on
					if on {
						cur = append(cur, q)
					}
				}
			}
		}
		if on && len(cur) > 1 {
			out = append(out, poly{pts: cur})
		}
	}
	return out
}

// stamper strokes polylines as a union of convex stamps: a quad for every
// segment, a join at every corner and a cap at every free end, each a closed
// contour of the same orientation, so that the nonzero rule of the
// rasteriser fills exactly their union. The idea is internal/icon's.
type stamper struct {
	z      *vector.Rasterizer
	ox, oy float64
}

// convex adds a convex polygon, turned to positive orientation.
func (s *stamper) convex(ps ...pt) {
	area := 0.0
	for i := range ps {
		a, b := ps[i], ps[(i+1)%len(ps)]
		area += a.x*b.y - b.x*a.y
	}
	if area == 0 {
		return
	}
	if area < 0 {
		for i, j := 0, len(ps)-1; i < j; i, j = i+1, j-1 {
			ps[i], ps[j] = ps[j], ps[i]
		}
	}
	s.z.MoveTo(float32(ps[0].x+s.ox), float32(ps[0].y+s.oy))
	for _, p := range ps[1:] {
		s.z.LineTo(float32(p.x+s.ox), float32(p.y+s.oy))
	}
	s.z.ClosePath()
}

func (s *stamper) disc(c pt, r float64) {
	n := min(max(int(math.Ceil(2*math.Pi*r/1.5)), 8), 256)
	ps := make([]pt, n)
	for i := range n {
		a := 2 * math.Pi * float64(i) / float64(n)
		ps[i] = pt{c.x + r*math.Cos(a), c.y + r*math.Sin(a)}
	}
	s.convex(ps...)
}

// stroke stamps the polyline pts of half width hw.
func (s *stamper) stroke(pts []pt, closed bool, hw float64, capStyle, join string, miter float64) {
	// Points on top of each other make no segment.
	q := pts[:0:0]
	for _, p := range pts {
		if len(q) == 0 || math.Hypot(p.x-q[len(q)-1].x, p.y-q[len(q)-1].y) > 1e-6 {
			q = append(q, p)
		}
	}
	if closed && len(q) > 2 && q[0] == q[len(q)-1] {
		q = q[:len(q)-1]
	}
	if len(q) == 1 {
		// A subpath of no length shows as a dot with a round cap and a
		// square with a square one.
		switch capStyle {
		case "round":
			s.disc(q[0], hw)
		case "square":
			p := q[0]
			s.convex(pt{p.x - hw, p.y - hw}, pt{p.x + hw, p.y - hw}, pt{p.x + hw, p.y + hw}, pt{p.x - hw, p.y + hw})
		}
		return
	}
	n := len(q)
	segs := n - 1
	if closed {
		segs = n
	}
	for i := range segs {
		a, b := q[i], q[(i+1)%n]
		nx, ny := normal(a, b, hw)
		s.convex(pt{a.x + nx, a.y + ny}, pt{b.x + nx, b.y + ny}, pt{b.x - nx, b.y - ny}, pt{a.x - nx, a.y - ny})
	}
	// Joins at the corners.
	for i := range n {
		if !closed && (i == 0 || i == n-1) {
			continue
		}
		prev, cur, next := q[(i+n-1)%n], q[i], q[(i+1)%n]
		s.join(prev, cur, next, hw, join, miter)
	}
	if !closed {
		s.cap(q[1], q[0], hw, capStyle)
		s.cap(q[n-2], q[n-1], hw, capStyle)
	}
}

func normal(a, b pt, hw float64) (float64, float64) {
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return 0, 0
	}
	return -dy / l * hw, dx / l * hw
}

// join stamps the join at cur between the segments from prev and to next.
func (s *stamper) join(prev, cur, next pt, hw float64, join string, miter float64) {
	if join == "round" {
		s.disc(cur, hw)
		return
	}
	n0x, n0y := normal(prev, cur, hw)
	n1x, n1y := normal(cur, next, hw)
	// The outer side of the turn.
	cross := (cur.x-prev.x)*(next.y-cur.y) - (cur.y-prev.y)*(next.x-cur.x)
	if cross > 0 {
		n0x, n0y, n1x, n1y = -n0x, -n0y, -n1x, -n1y
	}
	a, b := pt{cur.x + n0x, cur.y + n0y}, pt{cur.x + n1x, cur.y + n1y}
	if join != "bevel" {
		// The miter tip is where the two outer edges meet.
		mx, my := n0x+n1x, n0y+n1y
		ml := math.Hypot(mx, my)
		if ml > 0 {
			cos := (n0x*n1x + n0y*n1y) / (hw * hw)
			length := hw / math.Sqrt(max((1+cos)/2, 1e-12))
			if length/hw <= miter {
				tip := pt{cur.x + mx/ml*length, cur.y + my/ml*length}
				s.convex(cur, a, tip, b)
				return
			}
		}
	}
	s.convex(cur, a, b)
}

// cap stamps the cap at end, the segment coming from from.
func (s *stamper) cap(from, end pt, hw float64, capStyle string) {
	switch capStyle {
	case "round":
		s.disc(end, hw)
	case "square":
		dx, dy := end.x-from.x, end.y-from.y
		l := math.Hypot(dx, dy)
		if l == 0 {
			return
		}
		ux, uy := dx/l*hw, dy/l*hw
		nx, ny := normal(from, end, hw)
		s.convex(pt{end.x + nx, end.y + ny}, pt{end.x + nx + ux, end.y + ny + uy},
			pt{end.x - nx + ux, end.y - ny + uy}, pt{end.x - nx, end.y - ny})
	}
}
