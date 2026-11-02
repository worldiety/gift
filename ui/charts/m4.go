package charts

import (
	"iter"
	"math"
	"slices"
	"sort"
)

// gap is the Y of a break: a point that is not drawn and that a line and an
// area do not cross.
var gap = math.NaN()

// M4 reduces points, ascending in X, to what a line chart of buckets pixel
// columns between lo and hi can show: for every column the first and the
// last point, and the ones with the smallest and the largest value, in
// order. Drawn as a line, the result covers the same pixels as all the
// points would; this is the M4 aggregation of Jugel et al., "M4: A
// Visualization-Oriented Time Series Data Aggregation" (VLDB 2014).
//
// It keeps the missing values of [Missing] where they are: a column with a
// missing value yields the four points of the values before it, a break,
// the smallest and the largest value between the first and the last missing
// value of the column, another break and the four points of the values
// after it, so a line breaks in the column where the data does. A break is a
// Point whose Y is NaN, at the X of the missing value. That is twelve points
// a column at the very most, however many arrive.
//
// It is one pass with constant memory, so it reduces a billion points from
// a database as well as a thousand, and it is what a [Source] can run where
// its points are, to send the chart only what it can show. A point before lo
// or after hi falls into the first or the last column, and one that arrives
// out of order into the column of the point before it. Fewer than one
// bucket counts as one, and an empty range puts every point into the one
// column.
func M4(points iter.Seq[Point], lo, hi float64, buckets int) iter.Seq[Point] {
	return func(yield func(Point) bool) {
		g := newGrid(lo, hi, buckets)
		var (
			c     cell
			cur   = -1
			out   []Point
			broke bool
		)
		flush := func() bool {
			out = c.emit(out[:0])
			for _, p := range out {
				if Missing(p.Y) {
					if broke {
						continue
					}
					broke = true
				} else {
					broke = false
				}
				if !yield(p) {
					return false
				}
			}
			return true
		}
		for p := range points {
			if math.IsNaN(p.X) {
				continue
			}
			b := max(g.col(p.X), cur)
			if b != cur && cur >= 0 {
				if !flush() {
					return
				}
				c = cell{}
			}
			cur = b
			c.add(p)
		}
		if cur >= 0 {
			flush()
		}
	}
}

// grid is the division of [lo, hi] into pixel columns.
type grid struct {
	lo, scale float64
	n         int
}

func newGrid(lo, hi float64, buckets int) grid {
	n := max(buckets, 1)
	g := grid{lo: lo, n: n}
	if hi > lo && !math.IsInf(hi-lo, 0) {
		g.scale = float64(n) / (hi - lo)
	}
	return g
}

// col returns the column of x, clamped to the grid.
func (g grid) col(x float64) int {
	f := (x - g.lo) * g.scale
	switch {
	case !(f > 0):
		return 0
	case f >= float64(g.n):
		return g.n - 1
	}
	return int(f)
}

// run is the M4 of values without a missing one between them.
type run struct {
	n                   int
	first, last, lo, hi Point
}

func (r *run) add(p Point) {
	if r.n == 0 {
		r.first, r.lo, r.hi = p, p, p
	} else {
		if p.Y < r.lo.Y {
			r.lo = p
		}
		if p.Y > r.hi.Y {
			r.hi = p
		}
	}
	r.last = p
	r.n++
}

// merge takes the extremes of o; the first and the last point of a run that
// is merged no longer matter.
func (r *run) merge(o run) {
	switch {
	case o.n == 0:
	case r.n == 0:
		*r = o
	default:
		if o.lo.Y < r.lo.Y {
			r.lo = o.lo
		}
		if o.hi.Y > r.hi.Y {
			r.hi = o.hi
		}
		r.n += o.n
	}
}

// emit appends the first point, the extremes in order of X and the last
// point, each once.
func (r run) emit(dst []Point) []Point {
	if r.n == 0 {
		return dst
	}
	a, b := r.lo, r.hi
	if b.X < a.X {
		a, b = b, a
	}
	return appendOnce(dst, r.first, a, b, r.last)
}

// extremes appends the smallest and the largest value in order of X, each
// once: the part of a column between its first and its last missing value.
func (r run) extremes(dst []Point) []Point {
	if r.n == 0 {
		return dst
	}
	a, b := r.lo, r.hi
	if b.X < a.X {
		a, b = b, a
	}
	return appendOnce(dst, a, b)
}

// appendOnce appends the points of ps that are not among the ones before
// them: the first point of a run is often its smallest or its largest too.
func appendOnce(dst []Point, ps ...Point) []Point {
	for i, p := range ps {
		if !slices.Contains(ps[:i], p) {
			dst = append(dst, p)
		}
	}
	return dst
}

// cell is one pixel column: the run before its first missing value, the
// extremes between its first and its last missing value, and the run after
// its last missing value.
type cell struct {
	pre, mid, post run
	missing        bool
	gx0, gx1       float64
}

func (c *cell) add(p Point) {
	switch {
	case Missing(p.Y):
		if !c.missing {
			c.missing, c.gx0 = true, p.X
		} else {
			c.mid.merge(c.post)
		}
		c.post, c.gx1 = run{}, p.X
	case c.missing:
		c.post.add(p)
	default:
		c.pre.add(p)
	}
}

func (c cell) emit(dst []Point) []Point {
	dst = c.pre.emit(dst)
	if !c.missing {
		return dst
	}
	dst = append(dst, Point{c.gx0, gap})
	if c.mid.n > 0 {
		dst = c.mid.extremes(dst)
		dst = append(dst, Point{c.gx1, gap})
	}
	return c.post.emit(dst)
}

// reduce appends what a chart of buckets columns between lo and hi shows of
// d: the points themselves when there are at most four a column, the [M4]
// of each column otherwise, and in both cases the nearest point beyond
// either end, so that a line runs on to the edge of the plot. It costs
// O(buckets · log n), whatever the number of points.
func (d *Data) reduce(dst []Point, lo, hi float64, buckets int) []Point {
	n := len(d.xs)
	if n == 0 {
		return dst
	}
	i0, i1 := d.span(lo, hi)
	if i1-i0 <= 4*max(buckets, 1) {
		return d.raw(dst, max(i0-1, 0), min(i1+1, n))
	}
	if i0 > 0 {
		dst = d.raw(dst, i0-1, i0)
	}
	dst = d.columns(dst, i0, i1, newGrid(lo, hi, buckets))
	if i1 < n {
		dst = d.raw(dst, i1, i1+1)
	}
	return dst
}

// raw appends the points [i0, i1), a missing one as a break.
func (d *Data) raw(dst []Point, i0, i1 int) []Point {
	n := len(dst)
	for i := i0; i < i1; i++ {
		y := d.ys[i]
		if Missing(y) {
			y = gap
		}
		dst = append(dst, Point{d.xs[i], y})
	}
	return compact(dst, n)
}

// compact removes the breaks of dst[from:] that follow a break: one break
// says all there is to say.
func compact(dst []Point, from int) []Point {
	w := max(from, 1)
	for r := w; r < len(dst); r++ {
		if Missing(dst[r].Y) && Missing(dst[w-1].Y) {
			continue
		}
		dst[w] = dst[r]
		w++
	}
	if w > len(dst) {
		return dst
	}
	return dst[:w]
}

// columns appends the M4 of the points [i0, i1), column by column of g,
// with the index answering for each column. The result is that of [M4]
// over the same points.
func (d *Data) columns(dst []Point, i0, i1 int, g grid) []Point {
	for i := i0; i < i1; {
		j := g.col(d.xs[i])
		e := i + sort.Search(i1-i, func(k int) bool { return g.col(d.xs[i+k]) > j })
		n := len(dst)
		dst = compact(d.cell(i, e).emit(dst), n)
		i = e
	}
	return dst
}

// cell returns the column of the points [i0, i1) from the index.
func (d *Data) cell(i0, i1 int) cell {
	first, last, ok := d.ix.missing(i0, i1)
	if !ok {
		return cell{pre: d.run(i0, i1)}
	}
	c := cell{pre: d.run(i0, first), post: d.run(last+1, i1), missing: true, gx0: d.xs[first], gx1: d.xs[last]}
	if l, h := d.ix.extremes(d.ys, first+1, last); l >= 0 {
		c.mid = run{n: 1, lo: d.At(l), hi: d.At(h)}
	}
	return c
}

// run returns the run of the points [i0, i1), which have no missing value.
func (d *Data) run(i0, i1 int) run {
	if i0 >= i1 {
		return run{}
	}
	l, h := d.ix.extremes(d.ys, i0, i1)
	return run{n: i1 - i0, first: d.At(i0), last: d.At(i1 - 1), lo: d.At(l), hi: d.At(h)}
}
