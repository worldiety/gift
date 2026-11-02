package charts_test

import (
	"context"
	"iter"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/gifttest"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/charts"
)

// TestMain runs the tests on Ebitengine's main thread when the GPU half is
// built in; see gifttest.Main.
func TestMain(m *testing.M) { gifttest.Main(m) }

var size = geom.Sz(600, 300)

func mount(t *testing.T, root func(ctx *gift.Context) gift.View) *gifttest.Harness {
	t.Helper()
	h := gifttest.New(t, gifttest.Options{Root: root, Size: size,
		Font: ui.MustFont(ui.FontQuery{Family: inter.Family}), Theme: ui.LightTheme()})
	h.Settle()
	return h
}

// ramp is n points from 0 to n-1, with y = x.
func ramp(n int) *charts.Data {
	xs, ys := make([]float64, n), make([]float64, n)
	for i := range n {
		xs[i], ys[i] = float64(i), float64(i)
	}
	return charts.XY(xs, ys)
}

// visible records the ranges a chart reports.
type visible struct{ lo, hi []float64 }

func (v *visible) set(lo, hi float64) { v.lo, v.hi = append(v.lo, lo), append(v.hi, hi) }

func (v *visible) last(t *testing.T) (float64, float64) {
	t.Helper()
	if len(v.lo) == 0 {
		t.Fatal("the chart reported no visible range")
	}
	return v.lo[len(v.lo)-1], v.hi[len(v.hi)-1]
}

// vertices returns every vertex of the chart's triangles.
func vertices(h *gifttest.Harness) []render.Vertex {
	var vs []render.Vertex
	l := h.List()
	for _, op := range h.Ops() {
		if op.Kind == render.OpTriangles {
			first, n := op.Triangles()
			vs = append(vs, l.Vertices(first, n)...)
		}
	}
	return vs
}

func TestALineBreaksWhereValuesAreMissing(t *testing.T) {
	ys := []float64{0, 1, 2, 3, math.NaN(), math.NaN(), 6, 7, 8, 9, 10}
	xs := make([]float64, len(ys))
	for i := range xs {
		xs[i] = float64(i)
	}
	d := charts.XY(xs, ys)
	h := mount(t, func(*gift.Context) gift.View { return charts.Chart(charts.Line(d)).Zoomable(false) })
	vs := vertices(h)
	if len(vs) == 0 {
		t.Fatal("no line")
	}
	lo, hi := float32(math.Inf(1)), float32(math.Inf(-1))
	for _, v := range vs {
		lo, hi = min(lo, v.X), max(hi, v.X)
	}
	at := func(x float64) float32 { return lo + (hi-lo)*float32(x/10) }
	left, right := false, false
	for _, v := range vs {
		switch {
		case v.X > at(3.3) && v.X < at(5.7):
			t.Fatalf("a vertex at %v lies in the gap between %v and %v", v.X, at(3), at(6))
		case v.X <= at(3.3):
			left = true
		default:
			right = true
		}
	}
	if !left || !right {
		t.Fatalf("the line is not on both sides of the gap: %v %v", left, right)
	}
}

func TestAValueAloneBetweenGapsIsADot(t *testing.T) {
	d := charts.XY([]float64{0, 1, 2, 3, 4}, []float64{1, math.NaN(), 5, math.NaN(), 2})
	h := mount(t, func(*gift.Context) gift.View { return charts.Chart(charts.Line(d)).Zoomable(false) })
	if n := len(vertices(h)); n < 3*12*3 {
		t.Fatalf("%d vertices for two ends of nothing and a dot", n)
	}
}

func TestTheControlKeyAndTheWheelZoomAboutThePointer(t *testing.T) {
	d := ramp(1000)
	var vis visible
	h := mount(t, func(*gift.Context) gift.View {
		return charts.Chart(charts.Line(d)).OnVisibleChange(vis.set)
	})
	plot := h.Find(gifttest.ByKey("plot")).Bounds()
	h.Wheel(geom.Pt(plot.Min.X+100, 150), geom.Pt(0, 3))
	h.Settle()
	if len(vis.lo) != 0 {
		t.Fatalf("a wheel without a key zoomed to %v", vis.lo)
	}
	h.SetModifiers(gift.ModControl)
	h.Wheel(geom.Pt(plot.Min.X+100, 150), geom.Pt(0, 3))
	h.SetModifiers()
	h.Settle()
	lo, hi := vis.last(t)
	if hi-lo > 999*0.6 || lo <= 0 {
		t.Fatalf("three notches show [%v, %v]", lo, hi)
	}
	h.AssertExists(gifttest.ByKey("charts.reset"))
}

func TestADoubleTapZoomsInAndTheResetButtonOut(t *testing.T) {
	d := ramp(1000)
	var vis visible
	h := mount(t, func(*gift.Context) gift.View {
		return charts.Chart(charts.Line(d)).OnVisibleChange(vis.set)
	})
	h.AssertNone(gifttest.ByKey("charts.reset"))
	c := h.Find(gifttest.ByKey("plot")).Center()
	h.TapAt(c)
	h.Advance(80 * time.Millisecond)
	h.TapAt(c)
	h.Advance(time.Second)
	h.Settle()
	lo, hi := vis.last(t)
	if math.Abs((hi-lo)-999.0/2) > 1 {
		t.Fatalf("a double tap shows [%v, %v]", lo, hi)
	}
	h.Find(gifttest.ByKey("charts.reset")).Click()
	h.Advance(time.Second)
	h.Settle()
	lo, hi = vis.last(t)
	if lo != 0 || hi != 999 {
		t.Fatalf("reset shows [%v, %v]", lo, hi)
	}
	h.AssertNone(gifttest.ByKey("charts.reset"))
}

func TestAMouseDragsARangeToZoomInto(t *testing.T) {
	d := ramp(1000)
	var vis visible
	h := mount(t, func(*gift.Context) gift.View {
		return charts.Chart(charts.Line(d)).OnVisibleChange(vis.set)
	})
	plot := h.Find(gifttest.ByKey("plot")).Bounds()
	y := plot.Min.Y + 100
	h.MoveTo(geom.Pt(plot.Min.X+100, y))
	h.PressAt(geom.Pt(plot.Min.X+100, y))
	for x := float32(110); x <= 200; x += 10 {
		h.MoveTo(geom.Pt(plot.Min.X+x, y))
		h.Frame()
	}
	h.ReleaseAt(geom.Pt(plot.Min.X+200, y))
	h.Advance(time.Second)
	h.Settle()
	lo, hi := vis.last(t)
	if hi-lo > 300 || hi-lo < 100 || lo < 100 {
		t.Fatalf("a range dragged over a sixth of the chart shows [%v, %v]", lo, hi)
	}
}

func TestAFingerDragsAZoomedChartAndTwoPinchIt(t *testing.T) {
	d := ramp(1000)
	var vis visible
	h := mount(t, func(*gift.Context) gift.View {
		return charts.Chart(charts.Line(d)).OnVisibleChange(vis.set)
	})
	c := h.Find(gifttest.ByKey("plot")).Center()
	h.Pinch(c, 60, 240, 300*time.Millisecond)
	h.Settle()
	lo, hi := vis.last(t)
	if span := hi - lo; span > 999/3 || span < 999/5 {
		t.Fatalf("spreading the fingers four times shows [%v, %v]", lo, hi)
	}
	h.Fling(c, geom.Pt(c.X+150, c.Y), 300*time.Millisecond)
	h.Advance(time.Second)
	h.Settle()
	lo2, hi2 := vis.last(t)
	if !(lo2 < lo) || math.Abs((hi2-lo2)-(hi-lo)) > 1e-6 {
		t.Fatalf("a drag to the right moved [%v, %v] to [%v, %v]", lo, hi, lo2, hi2)
	}
}

func TestTheMouseInspectsTheValuesUnderIt(t *testing.T) {
	a := charts.XY([]float64{0, 1, 2, 3}, []float64{10, 11, 12, 13})
	b := charts.XY([]float64{0, 1, 2, 3}, []float64{20, math.NaN(), 22, 23})
	var xs []float64
	var ends int
	h := mount(t, func(*gift.Context) gift.View {
		return charts.Chart(charts.Line(a).Name("Hall"), charts.Line(b).Name("Yard")).
			OnInspect(func(x float64, ok bool) {
				if ok {
					xs = append(xs, x)
				} else {
					ends++
				}
			})
	})
	plot := h.Find(gifttest.ByKey("plot")).Bounds()
	h.MoveTo(geom.Pt(plot.Min.X+plot.Width()/3+3, plot.Min.Y+50))
	h.Settle()
	h.AssertExists(gifttest.ByText("11"))
	h.AssertExists(gifttest.ByText("–"))
	if len(xs) == 0 {
		t.Fatal("no inspection")
	}
	h.MoveTo(geom.Pt(-10, -10))
	h.Settle()
	h.AssertNone(gifttest.ByText("11"))
	if ends == 0 {
		t.Fatal("the inspection did not end")
	}
}

// recorder is a source whose requests a test can see.
type recorder struct {
	mu   sync.Mutex
	reqs []charts.Request
}

func (r *recorder) source(lo, hi float64) *charts.Source {
	return charts.NewSource(charts.SourceConfig{
		Extent: func(context.Context) (float64, float64, error) { return lo, hi, nil },
		Points: func(ctx context.Context, q charts.Request) (iter.Seq[charts.Point], error) {
			r.mu.Lock()
			r.reqs = append(r.reqs, q)
			r.mu.Unlock()
			// Every point of the range, a hundred thousand of them: the chart
			// has to reduce them itself.
			return func(yield func(charts.Point) bool) {
				const n = 100_000
				for i := range n + 1 {
					x := q.From + (q.To-q.From)*float64(i)/n
					if !yield(charts.Point{X: x, Y: math.Sin(x / 10)}) {
						return
					}
				}
			}, nil
		},
	})
}

func (r *recorder) requests() []charts.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]charts.Request(nil), r.reqs...)
}

// await runs frames until done says so, for the goroutines of a source.
func await(t *testing.T, h *gifttest.Harness, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
		h.Frame()
	}
	h.Settle()
}

func TestASourceIsAskedForWhatIsShownAndAgainWhenZoomed(t *testing.T) {
	var rec recorder
	src := rec.source(0, 1000)
	var vis visible
	h := mount(t, func(*gift.Context) gift.View {
		return charts.Chart(charts.Line(src)).OnVisibleChange(vis.set)
	})
	await(t, h, "the first answer", func() bool {
		return len(rec.requests()) > 0 && h.Exists(gifttest.ByText("1k")) && !h.Exists(gifttest.ByText("Loading…"))
	})
	first := rec.requests()[0]
	plot := h.Find(gifttest.ByKey("plot")).Bounds()
	if first.From != 0 || first.To != 1000 || first.Buckets < int(plot.Width()*0.8) || first.Buckets > 2*int(plot.Width()) {
		t.Fatalf("the first request is %+v for a plot %v wide", first, plot.Width())
	}
	h.SetModifiers(gift.ModControl)
	h.Wheel(plot.Min.Add(geom.Pt(plot.Width()/2, 50)), geom.Pt(0, 10))
	h.SetModifiers()
	await(t, h, "the zoomed answer", func() bool { return len(rec.requests()) > 1 })
	lo, hi := vis.last(t)
	last := rec.requests()[len(rec.requests())-1]
	if last.From > lo || last.To < hi || last.To-last.From > 3*(hi-lo) {
		t.Fatalf("showing [%v, %v] asked for %+v", lo, hi, last)
	}
}

func TestASourceThatFailsSaysSo(t *testing.T) {
	src := charts.NewSource(charts.SourceConfig{
		Extent: func(context.Context) (float64, float64, error) { return 0, 1, nil },
		Points: func(context.Context, charts.Request) (iter.Seq[charts.Point], error) { panic("disk on fire") },
	})
	h := mount(t, func(*gift.Context) gift.View { return charts.Chart(charts.Line(src)) })
	await(t, h, "the error", func() bool { return h.Exists(gifttest.ByTextContains("disk on fire")) })
}

func TestAppendedPointsWidenTheChart(t *testing.T) {
	d := ramp(11)
	h := mount(t, func(*gift.Context) gift.View { return charts.Chart(charts.Line(d)) })
	h.AssertNone(gifttest.ByText("40"))
	for x := 11.0; x <= 40; x++ {
		d.Append(x, x)
	}
	h.Settle()
	h.AssertExists(gifttest.ByText("40"))
}

func TestAnEmptyChartSaysSo(t *testing.T) {
	h := mount(t, func(*gift.Context) gift.View { return charts.Chart(charts.Line(charts.XY(nil, nil))) })
	h.AssertExists(gifttest.ByText("No Data"))
}

func TestCategoriesNameTheBars(t *testing.T) {
	d := charts.XY([]float64{0, 1, 2}, []float64{3, 5, 4})
	h := mount(t, func(*gift.Context) gift.View {
		return charts.Chart(charts.Bar(d)).Categories("Tea", "Coffee", "Cocoa")
	})
	for _, name := range []string{"Tea", "Coffee", "Cocoa"} {
		h.AssertExists(gifttest.ByText(name))
	}
	h.AssertOps("bars", 3, func(op render.Op) bool { return op.Kind == render.OpFillRoundRect })
}

func TestAbsurdValuesStayOnThePlot(t *testing.T) {
	d := charts.XY([]float64{0, 1, 2, 3, 4}, []float64{1, 1e300, -1e300, 2, math.MaxFloat64})
	h := mount(t, func(*gift.Context) gift.View {
		return charts.Chart(charts.Line(d), charts.Area(d)).YDomain(0, 3).Zoomable(false)
	})
	vs := vertices(h)
	if len(vs) == 0 {
		t.Fatal("nothing drawn")
	}
	for _, v := range vs {
		if math.IsNaN(float64(v.X)) || math.IsNaN(float64(v.Y)) || math.Abs(float64(v.X)) > 1e6 || math.Abs(float64(v.Y)) > 1e6 {
			t.Fatalf("a vertex at %v, %v", v.X, v.Y)
		}
	}
}

func TestTheTimeAxisLabelsHoursAndTheDay(t *testing.T) {
	zone := time.FixedZone("CET", 3600)
	day := time.Date(2026, 10, 30, 0, 0, 0, 0, zone)
	ts := []time.Time{day, day.Add(23 * time.Hour)}
	d := charts.Times(ts, []float64{1, 2})
	h := mount(t, func(*gift.Context) gift.View { return charts.Chart(charts.Line(d)).Location(zone) })
	h.AssertExists(gifttest.ByText("30 Oct"))
	h.AssertExists(gifttest.ByText("12:00"))
}

func TestStackedBarsCarryTheirValues(t *testing.T) {
	a := charts.XY([]float64{0, 1}, []float64{98, 112})
	b := charts.XY([]float64{0, 1}, []float64{31, 38})
	h := mount(t, func(*gift.Context) gift.View {
		return charts.Chart(charts.Bar(a).ValueLabels(true), charts.Bar(b).ValueLabels(true)).Categories("Mon", "Tue")
	})
	for _, v := range []string{"98", "112", "31", "38"} {
		h.AssertExists(gifttest.ByText(v))
	}
	// Two stacks of two segments.
	h.AssertOps("segments", 4, func(op render.Op) bool { return op.Kind == render.OpFillRoundRect })
}

func TestANormalizedChartShowsShares(t *testing.T) {
	a := charts.XY([]float64{0}, []float64{1})
	b := charts.XY([]float64{0}, []float64{3})
	h := mount(t, func(*gift.Context) gift.View {
		return charts.Chart(charts.Bar(a).ValueLabels(true), charts.Bar(b).ValueLabels(true)).Stacking(charts.Normalized)
	})
	h.AssertExists(gifttest.ByText("25 %"))
	h.AssertExists(gifttest.ByText("75 %"))
	h.AssertExists(gifttest.ByText("100 %"))
}

func TestAPieShowsSharesAndASliceUnderThePointer(t *testing.T) {
	h := mount(t, func(*gift.Context) gift.View {
		return charts.Chart(
			charts.Sector(75).Name("Coffee").Percent(true),
			charts.Sector(25).Name("Tea").Percent(true),
		)
	})
	h.AssertExists(gifttest.ByText("75 %"))
	h.AssertExists(gifttest.ByText("25 %"))
	h.AssertCount(gifttest.ByText("Coffee"), 1)
	// The first slice runs clockwise from twelve to nine o'clock: to the
	// right of the middle is in it.
	plot := h.Find(gifttest.ByKey("plot")).Bounds()
	pie := h.Find(gifttest.ByText("75 %")).Bounds()
	h.MoveTo(geom.Pt(pie.Min.X+pie.Width()/2, pie.Min.Y+pie.Height()/2))
	h.Settle()
	h.AssertExists(gifttest.ByText("75"))
	h.AssertCount(gifttest.ByText("Coffee"), 2)
	h.MoveTo(geom.Pt(plot.Max.X+20, plot.Max.Y+20))
	h.Settle()
	h.AssertNone(gifttest.ByText("75"))
}

func TestAnAnnotationStandsAtItsPoint(t *testing.T) {
	d := ramp(11)
	h := mount(t, func(*gift.Context) gift.View {
		return charts.Chart(charts.Line(d), charts.Annotation(5, 5, "Gate open").Position(charts.Bottom))
	})
	h.AssertCount(gifttest.ByText("Gate open"), 1)
	note := h.Find(gifttest.ByText("Gate open")).Bounds()
	plot := h.Find(gifttest.ByKey("plot")).Bounds()
	if mid := plot.Min.Y + plot.Height()/2; note.Min.Y < mid-20 || note.Min.Y > mid+40 {
		t.Fatalf("the annotation of the middle stands at %v of a plot %v", note, plot)
	}
}

func TestZoomYTakesARectangle(t *testing.T) {
	d := ramp(1001)
	h := mount(t, func(*gift.Context) gift.View { return charts.Chart(charts.Scatter(d)).ZoomY(true) })
	h.AssertExists(gifttest.ByText("1k"))
	plot := h.Find(gifttest.ByKey("plot")).Bounds()
	at := func(fx, fy float32) geom.Point {
		return geom.Pt(plot.Min.X+plot.Width()*fx, plot.Min.Y+plot.Height()*fy)
	}
	h.MoveTo(at(0.3, 0.2))
	h.PressAt(at(0.3, 0.2))
	for f := float32(0.25); f <= 0.4; f += 0.05 {
		h.MoveTo(at(0.3+f/2, f))
		h.Frame()
	}
	h.ReleaseAt(at(0.5, 0.4))
	h.Advance(time.Second)
	h.Settle()
	h.AssertNone(gifttest.ByText("1k"))
	h.AssertExists(gifttest.ByKey("charts.reset"))
	h.Find(gifttest.ByKey("charts.reset")).Click()
	h.Advance(time.Second)
	h.Settle()
	h.AssertExists(gifttest.ByText("1k"))
}

func TestTwoChartsInspectTogether(t *testing.T) {
	a, b := ramp(11), ramp(11)
	h := gifttest.New(t, gifttest.Options{Root: func(ctx *gift.Context) gift.View {
		in := ctx.State("in", [2]float64{})
		ctx.Read(in)
		link := func(c charts.ChartView) charts.ChartView {
			return c.Inspect(in.Get()[0], in.Get()[1] == 1).OnInspect(func(x float64, on bool) {
				if on {
					in.Set([2]float64{x, 1})
				} else {
					in.Set([2]float64{})
				}
			})
		}
		return ui.VStack(
			link(charts.Chart(charts.Line(a).Name("A"))).Height(140).Key("top"),
			link(charts.Chart(charts.Line(b).Name("B"))).Height(140).Key("bottom"),
		)
	}, Size: geom.Sz(600, 300), Font: ui.MustFont(ui.FontQuery{Family: inter.Family}), Theme: ui.LightTheme()})
	h.Settle()
	top := h.Find(gifttest.ByKey("top")).Bounds()
	h.MoveTo(geom.Pt(top.Min.X+top.Width()*0.3, top.Min.Y+40))
	h.Settle()
	// Both callouts name their series; B's is in the chart below.
	h.AssertExists(gifttest.ByText("A"))
	bottom := h.Find(gifttest.ByKey("bottom"))
	if bottom.Find(gifttest.ByText("B")).IsZero() {
		t.Fatal("the chart below does not inspect")
	}
}
