package charts

import (
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"
)

// series returns n points with X ascending, some X repeated, and a share of
// missing values, some of them in runs.
func series(r *rand.Rand, n int, missing float64) ([]float64, []float64) {
	xs, ys := make([]float64, n), make([]float64, n)
	x, y := 0.0, 0.0
	for i := range n {
		if r.IntN(10) > 0 {
			x += r.Float64() * 3
		}
		y += r.NormFloat64()
		xs[i], ys[i] = x, y
		if r.Float64() < missing {
			ys[i] = math.NaN()
			if r.IntN(4) == 0 {
				ys[i] = math.Inf(1)
			}
		}
	}
	return xs, ys
}

func points(xs, ys []float64) []Point {
	ps := make([]Point, len(xs))
	for i := range xs {
		ps[i] = Point{xs[i], ys[i]}
	}
	return ps
}

// m4Reference is M4 written from its definition, column by column, without
// the streaming state of the real one.
func m4Reference(ps []Point, lo, hi float64, buckets int) []Point {
	g := newGrid(lo, hi, buckets)
	m4 := func(run []Point) []Point {
		if len(run) == 0 {
			return nil
		}
		l, h := run[0], run[0]
		for _, p := range run {
			if p.Y < l.Y {
				l = p
			}
			if p.Y > h.Y {
				h = p
			}
		}
		if h.X < l.X {
			l, h = h, l
		}
		return appendOnce(nil, run[0], l, h, run[len(run)-1])
	}
	var out []Point
	for start := 0; start < len(ps); {
		col := g.col(ps[start].X)
		end := start
		for end < len(ps) && g.col(ps[end].X) == col {
			end++
		}
		in := ps[start:end]
		first, last := -1, -1
		for i, p := range in {
			if Missing(p.Y) {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first < 0 {
			out = append(out, m4(in)...)
		} else {
			out = append(out, m4(in[:first])...)
			out = append(out, Point{in[first].X, gap})
			var mid []Point
			for _, p := range in[first+1 : max(first+1, last)] {
				if !Missing(p.Y) {
					mid = append(mid, p)
				}
			}
			if len(mid) > 0 {
				ex := m4(mid)
				l, h := ex[0], ex[0]
				for _, p := range mid {
					if p.Y < l.Y {
						l = p
					}
					if p.Y > h.Y {
						h = p
					}
				}
				if h.X < l.X {
					l, h = h, l
				}
				out = appendOnce(out, l, h)
				out = append(out, Point{in[last].X, gap})
			}
			out = append(out, m4(in[last+1:])...)
		}
		start = end
	}
	return compact(out, 0)
}

func same(a, b []Point) bool {
	return slices.EqualFunc(a, b, func(p, q Point) bool {
		return p.X == q.X && (p.Y == q.Y || Missing(p.Y) && Missing(q.Y))
	})
}

func TestM4KeepsTheExtremesOfEveryColumnAndItsGaps(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for trial := range 200 {
		xs, ys := series(r, 1+r.IntN(3000), []float64{0, 0.01, 0.2, 0.7}[trial%4])
		ps := points(xs, ys)
		lo, hi := xs[0], xs[len(xs)-1]
		buckets := 1 + r.IntN(300)
		got := slices.Collect(M4(slices.Values(ps), lo, hi, buckets))
		want := m4Reference(ps, lo, hi, buckets)
		if !same(got, want) {
			t.Fatalf("trial %d, %d points into %d columns:\n got %v\nwant %v", trial, len(ps), buckets, got, want)
		}
		if limit := 12 * buckets; len(got) > limit {
			t.Fatalf("trial %d: %d points out of %d columns", trial, len(got), buckets)
		}
	}
}

func TestTheIndexReducesAsM4Does(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for trial := range 200 {
		xs, ys := series(r, 1+r.IntN(5000), []float64{0, 0.005, 0.1, 0.6}[trial%4])
		d := XY(xs, ys)
		i0 := r.IntN(len(xs))
		i1 := i0 + r.IntN(len(xs)-i0+1)
		if i1 == i0 {
			continue
		}
		lo, hi := xs[i0], xs[i1-1]
		buckets := 1 + r.IntN(200)
		// The range as a half open one of whole X values: the points of the
		// first and the last X that are not in [i0, i1) belong to the grid
		// as well.
		a, b := d.span(lo, hi)
		got := d.columns(nil, a, b, newGrid(lo, hi, buckets))
		want := slices.Collect(M4(slices.Values(points(xs[a:b], ys[a:b])), lo, hi, buckets))
		if !same(got, want) {
			t.Fatalf("trial %d, points %d to %d into %d columns:\n got %v\nwant %v", trial, a, b, buckets, got, want)
		}
	}
}

func TestTheIndexFindsTheExtremesOfAnyRange(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	xs, ys := series(r, 10000, 0.05)
	built := XY(xs, ys)
	grown := XY(nil, nil)
	for i := range xs {
		grown.Append(xs[i], ys[i])
	}
	for range 2000 {
		i0 := r.IntN(len(ys))
		i1 := i0 + r.IntN(len(ys)-i0+1)
		wl, wh := -1, -1
		for i := i0; i < i1; i++ {
			if Missing(ys[i]) {
				continue
			}
			if wl < 0 || ys[i] < ys[wl] {
				wl = i
			}
			if wh < 0 || ys[i] > ys[wh] {
				wh = i
			}
		}
		for name, d := range map[string]*Data{"built": built, "appended": grown} {
			l, h := d.ix.extremes(d.ys, i0, i1)
			if l != wl || h != wh {
				t.Fatalf("%s [%d, %d): extremes %d %d, want %d %d", name, i0, i1, l, h, wl, wh)
			}
		}
	}
}

func TestAZoomedInChartGetsThePointsThemselvesAndOneBeyondEachEdge(t *testing.T) {
	d := XY([]float64{0, 1, 2, 3, 4, 5}, []float64{10, 11, math.NaN(), math.NaN(), 14, 15})
	got := d.reduce(nil, 1.5, 4.5, 100)
	want := []Point{{1, 11}, {2, gap}, {4, 14}, {5, 15}}
	if !same(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAMillionPointsReduceToTheWidthOfTheChart(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 8))
	xs, ys := series(r, 1_000_000, 0.001)
	d := XY(xs, ys)
	lo, hi, _ := d.Extent()
	got := d.reduce(nil, lo, hi, 800)
	if len(got) > 12*800+2 || len(got) < 800 {
		t.Fatalf("%d points for 800 columns", len(got))
	}
	allocs := testing.AllocsPerRun(10, func() { got = d.reduce(got[:0], lo, hi, 800) })
	if allocs > 0 {
		t.Errorf("a reduction into a reused slice allocates %v times", allocs)
	}
}

func TestDataSortsWhatArrivesOutOfOrder(t *testing.T) {
	d := XY([]float64{3, 1, math.NaN(), 2}, []float64{30, 10, 99, 20})
	if d.Len() != 3 || d.At(0) != (Point{1, 10}) || d.At(2) != (Point{3, 30}) {
		t.Fatalf("%v", d.xs)
	}
	d.Append(2.5, 25)
	d.Append(4, 40)
	if got := slices.Clone(d.xs); !slices.Equal(got, []float64{1, 2, 2.5, 3, 4}) {
		t.Fatalf("after appending %v", got)
	}
	if l, h := d.ix.extremes(d.ys, 0, d.Len()); d.ys[l] != 10 || d.ys[h] != 40 {
		t.Fatalf("extremes %d %d", l, h)
	}
	d.DropBefore(2.5)
	if d.Len() != 3 || d.At(0).X != 2.5 {
		t.Fatalf("after dropping %v", d.xs)
	}
}

func TestM4PutsStrayPointsIntoAColumnAndStaysBounded(t *testing.T) {
	ps := []Point{{-5, 1}, {0, 2}, {3, 3}, {2.5, 9}, {20, 4}, {math.NaN(), 7}}
	got := slices.Collect(M4(slices.Values(ps), 0, 10, 10))
	// -5 falls into the first column with 0; 2.5 arrives after 3 and joins
	// its column; 20 is clamped into the last one; the NaN X is left out.
	want := []Point{{-5, 1}, {0, 2}, {3, 3}, {2.5, 9}, {20, 4}}
	if !same(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if n := len(slices.Collect(M4(slices.Values(ps), 5, 5, 0))); n > 4 {
		t.Fatalf("an empty range yields %d points", n)
	}
}

func BenchmarkReduce(b *testing.B) {
	r := rand.New(rand.NewPCG(9, 10))
	for _, n := range []int{10_000, 1_000_000, 10_000_000} {
		xs, ys := series(r, n, 0.001)
		d := XY(xs, ys)
		lo, hi, _ := d.Extent()
		var out []Point
		b.Run(fmtCount(n), func(b *testing.B) {
			for b.Loop() {
				out = d.reduce(out[:0], lo, hi, 1600)
			}
		})
	}
}

func BenchmarkM4Stream(b *testing.B) {
	r := rand.New(rand.NewPCG(11, 12))
	xs, ys := series(r, 1_000_000, 0.001)
	ps := points(xs, ys)
	for b.Loop() {
		for range M4(slices.Values(ps), xs[0], xs[len(xs)-1], 1600) {
		}
	}
}

func fmtCount(n int) string {
	switch {
	case n >= 1_000_000:
		return strconv.Itoa(n/1_000_000) + "M"
	case n >= 1000:
		return strconv.Itoa(n/1000) + "k"
	}
	return strconv.Itoa(n)
}
