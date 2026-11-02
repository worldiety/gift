package charts

import (
	"math"
	"sort"
)

// Stacking is how bars and areas of several series share an x, the
// counterpart of Swift Charts' MarkStackingMethod and of grouping bars with
// its position modifier.
type Stacking uint8

const (
	// Stacked puts the bars and the areas of the series on top of each
	// other, positive values upwards and negative ones downwards, the way
	// Swift Charts does by default.
	Stacked Stacking = iota
	// Normalized stacks them to a whole: every stack is as high as the
	// axis, and the axis shows shares.
	Normalized
	// Grouped puts the bars of the series side by side; areas are not
	// stacked.
	Grouped
	// Unstacked draws every series from zero, the later over the earlier.
	Unstacked
)

// bar is a bar as the plot draws it, in the units of the axes.
type bar struct {
	mark   int
	x0, x1 float64
	y0, y1 float64
	value  float64
	thin   bool
}

// stackKey is where bars stack: the x of a point, or the pixel column of a
// bar too thin to draw on its own.
type stackKey struct {
	x   float64
	col int
}

// layoutBars works out the bars of every bar mark from the points it shows,
// stacked, normalized or grouped. Bars narrower than two device pixels
// stack by pixel column: each series shows the largest and the smallest of
// its values in the column, which over a year of days is a skyline that
// does not blur.
func (st *state) layoutBars(lo, hi float64) {
	st.bars = st.bars[:0]
	var marks []int
	for i, m := range st.v.marks {
		if m.spec().kind == kindBar {
			marks = append(marks, i)
		}
	}
	if len(marks) == 0 || !(hi > lo) || st.plot.Width() <= 0 {
		return
	}
	unit := math.Inf(1)
	for _, i := range marks {
		unit = min(unit, st.barStep(st.v.marks[i].spec(), &st.cache[i]))
	}
	frac := float64(st.v.marks[marks[0]].spec().width)
	stacking := st.v.stacking
	if len(marks) == 1 && stacking == Grouped {
		stacking = Stacked
	}
	perPx := (hi - lo) / float64(st.plot.Width()*st.density)
	full := unit * frac
	each := full
	if stacking == Grouped {
		each = full / float64(len(marks))
	}
	thin := each/perPx < 2

	if st.sums == nil {
		st.sums = map[stackKey][2]float64{}
	}
	clear(st.sums)
	type entry struct {
		key   stackKey
		x     float64
		value float64
	}
	var entries [][]entry
	for g, i := range marks {
		var es []entry
		c := &st.cache[i]
		if thin {
			// The largest and the smallest value of each column.
			cols := map[int][2]float64{}
			var order []int
			for _, p := range c.pts {
				if Missing(p.Y) || p.X < lo-unit || p.X > hi+unit {
					continue
				}
				col := int(math.Floor((p.X - lo) / perPx))
				e, ok := cols[col]
				if !ok {
					order = append(order, col)
					e = [2]float64{math.Inf(-1), math.Inf(1)}
				}
				cols[col] = [2]float64{max(e[0], p.Y), min(e[1], p.Y)}
			}
			sort.Ints(order)
			for _, col := range order {
				e := cols[col]
				x := lo + (float64(col)+0.5)*perPx
				if e[0] > 0 {
					es = append(es, entry{stackKey{col: col + 1}, x, e[0]})
				}
				if e[1] < 0 {
					es = append(es, entry{stackKey{col: -col - 1}, x, e[1]})
				}
			}
		} else {
			for _, p := range c.pts {
				if Missing(p.Y) || p.X < lo-unit || p.X > hi+unit {
					continue
				}
				x := p.X
				if stacking == Grouped {
					x += (float64(g) - float64(len(marks)-1)/2) * each
				}
				es = append(es, entry{stackKey{x: p.X}, x, p.Y})
			}
		}
		entries = append(entries, es)
	}
	if stacking == Normalized {
		for _, es := range entries {
			for _, e := range es {
				s := st.sums[e.key]
				s[0] += math.Abs(e.value)
				st.sums[e.key] = s
			}
		}
	}
	totals := st.sums
	running := map[stackKey][2]float64{}
	w := each
	if thin {
		w = perPx
	}
	for g, i := range marks {
		for _, e := range entries[g] {
			b := bar{mark: i, x0: e.x - w/2, x1: e.x + w/2, value: e.value, thin: thin}
			v := e.value
			switch stacking {
			case Stacked, Normalized:
				if stacking == Normalized {
					if t := totals[e.key][0]; t > 0 {
						v = math.Abs(v) / t
					} else {
						v = 0
					}
				}
				r := running[e.key]
				if v >= 0 {
					b.y0, b.y1 = r[0], r[0]+v
					r[0] += v
				} else {
					b.y0, b.y1 = r[1]+v, r[1]
					r[1] += v
				}
				running[e.key] = r
			default:
				b.y0, b.y1 = min(0, v), max(0, v)
			}
			st.bars = append(st.bars, b)
		}
	}
}

// stackAreas works out the lower and the upper edge of every area when
// areas stack: each area lies on the ones before it, which are read at its
// own points, between their points in a straight line.
func (st *state) stackAreas() {
	var marks []int
	for i, m := range st.v.marks {
		c := &st.cache[i]
		c.lower, c.upper = c.lower[:0], c.upper[:0]
		if m.spec().kind == kindArea {
			marks = append(marks, i)
		}
	}
	stacking := st.v.stacking
	if len(marks) < 2 || stacking != Stacked && stacking != Normalized {
		for _, i := range marks {
			st.cache[i].stacked = false
		}
		return
	}
	for k, i := range marks {
		c := &st.cache[i]
		c.stacked = true
		for _, p := range c.pts {
			if Missing(p.Y) {
				c.lower, c.upper = append(c.lower, gap), append(c.upper, gap)
				continue
			}
			below := 0.0
			for _, j := range marks[:k] {
				below += valueAt(st.cache[j].pts, p.X)
			}
			top := below + p.Y
			if stacking == Normalized {
				total := 0.0
				for _, j := range marks {
					if j == i {
						total += math.Abs(p.Y)
					} else {
						total += math.Abs(valueAt(st.cache[j].pts, p.X))
					}
				}
				if total > 0 {
					below, top = below/total, top/total
				} else {
					below, top = 0, 0
				}
			}
			c.lower, c.upper = append(c.lower, below), append(c.upper, top)
		}
	}
}

// valueAt reads the series ps, ascending in x, at x: between two points on
// the straight line, zero beyond its ends and next to a missing value.
func valueAt(ps []Point, x float64) float64 {
	i := sort.Search(len(ps), func(k int) bool { return ps[k].X >= x })
	switch {
	case i < len(ps) && ps[i].X == x:
		if Missing(ps[i].Y) {
			return 0
		}
		return ps[i].Y
	case i == 0 || i == len(ps):
		return 0
	}
	a, b := ps[i-1], ps[i]
	if Missing(a.Y) || Missing(b.Y) || b.X == a.X {
		return 0
	}
	return a.Y + (b.Y-a.Y)*(x-a.X)/(b.X-a.X)
}
