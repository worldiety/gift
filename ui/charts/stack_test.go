package charts

import (
	"math"
	"testing"

	"github.com/worldiety/gift/geom"
)

// stackState returns the state of a chart of marks as a layout of a plot
// 600 by 200 pixels over [lo, hi] leaves it, with every series reduced.
func stackState(v ChartView, lo, hi float64) *state {
	st := &state{v: v, cache: make([]markCache, len(v.marks)), density: 1}
	st.plot = geom.Rc(0, 0, 600, 200)
	st.lo, st.hi = lo, hi
	for i, m := range v.marks {
		if d, ok := m.spec().data.(*Data); ok {
			st.cache[i].pts = d.reduce(nil, lo, hi, 600)
		}
	}
	return st
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestBarsStackUpAndDown(t *testing.T) {
	a := XY([]float64{0, 1}, []float64{3, -2})
	b := XY([]float64{0, 1}, []float64{4, -5})
	st := stackState(Chart(Bar(a), Bar(b)), -0.5, 1.5)
	st.layoutBars(-0.5, 1.5)
	if len(st.bars) != 4 {
		t.Fatalf("%d bars", len(st.bars))
	}
	want := [][2]float64{{0, 3}, {-2, 0}, {3, 7}, {-7, -2}}
	for i, b := range st.bars {
		if !near(b.y0, want[i][0]) || !near(b.y1, want[i][1]) {
			t.Errorf("bar %d from %v to %v, want %v", i, b.y0, b.y1, want[i])
		}
	}
}

func TestNormalizedBarsMakeAWhole(t *testing.T) {
	a := XY([]float64{0}, []float64{1})
	b := XY([]float64{0}, []float64{3})
	st := stackState(Chart(Bar(a), Bar(b)).Stacking(Normalized), -0.5, 0.5)
	st.layoutBars(-0.5, 0.5)
	if !near(st.bars[0].y1, 0.25) || !near(st.bars[1].y0, 0.25) || !near(st.bars[1].y1, 1) {
		t.Fatalf("%+v", st.bars)
	}
}

func TestGroupedBarsStandSideBySide(t *testing.T) {
	a := XY([]float64{0, 1}, []float64{3, 4})
	b := XY([]float64{0, 1}, []float64{5, 6})
	st := stackState(Chart(Bar(a), Bar(b)).Stacking(Grouped), -0.5, 1.5)
	st.layoutBars(-0.5, 1.5)
	first, second := st.bars[0], st.bars[2]
	if !(first.x1 <= second.x0+1e-9) || first.y0 != 0 || second.y0 != 0 {
		t.Fatalf("the bars of x 0 are %+v and %+v", first, second)
	}
	if w := first.x1 - first.x0; math.Abs(w-0.4) > 1e-6 {
		t.Errorf("a grouped bar is %v wide, want half of 0.8", w)
	}
}

func TestThinBarsStackByPixelColumn(t *testing.T) {
	n := 20000
	xs, ys := make([]float64, n), make([]float64, n)
	for i := range n {
		xs[i], ys[i] = float64(i), float64(i%7)
	}
	a, b := XY(xs, ys), XY(xs, ys)
	st := stackState(Chart(Bar(a), Bar(b)), 0, float64(n))
	st.layoutBars(0, float64(n))
	if len(st.bars) == 0 || len(st.bars) > 2*600+4 {
		t.Fatalf("%d bars for 600 columns", len(st.bars))
	}
	for _, b := range st.bars {
		if !b.thin || b.y1 > 12+1e-9 {
			t.Fatalf("%+v", b)
		}
	}
}

func TestStackedAreasLieOnEachOther(t *testing.T) {
	a := XY([]float64{0, 2, 4}, []float64{1, 1, 1})
	b := XY([]float64{1, 3}, []float64{2, math.NaN()})
	st := stackState(Chart(Area(a), Area(b)), 0, 4)
	st.stackAreas()
	lo, hi := st.cache[1].lower, st.cache[1].upper
	if !near(lo[0], 1) || !near(hi[0], 3) || !Missing(hi[1]) {
		t.Fatalf("the upper area is %v to %v", lo, hi)
	}
	st = stackState(Chart(Area(a), Area(b)).Stacking(Normalized), 0, 4)
	st.stackAreas()
	if !near(st.cache[1].upper[0], 1) || !near(st.cache[1].lower[0], 1.0/3) {
		t.Fatalf("normalized: %v to %v", st.cache[1].lower, st.cache[1].upper)
	}
}

func TestValueAtReadsBetweenPoints(t *testing.T) {
	ps := []Point{{0, 0}, {2, 4}, {3, gap}, {4, 8}}
	for _, c := range []struct{ x, want float64 }{{1, 2}, {2, 4}, {2.5, 0}, {-1, 0}, {5, 0}} {
		if got := valueAt(ps, c.x); !near(got, c.want) {
			t.Errorf("at %v: %v, want %v", c.x, got, c.want)
		}
	}
}
