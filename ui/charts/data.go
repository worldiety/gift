package charts

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"time"
)

// Point is one value of a series: Y at X. A Y that is NaN or infinite is a
// missing value, where a line breaks; see [Missing].
//
// On a time axis X is a time in seconds since the Unix epoch, as [Seconds]
// converts it.
type Point struct {
	X, Y float64
}

// Missing reports whether y is a missing value: NaN or infinite. Data from a
// sensor that was off, a gap in a recording, a value that failed to convert
// are written as math.NaN(), and a line or an area is not drawn across them.
func Missing(y float64) bool { return math.IsNaN(y) || math.IsInf(y, 0) }

// Seconds is t as an X of a time axis: seconds since the Unix epoch, with
// the fraction. A float64 keeps a microsecond apart for the next 200 years.
func Seconds(t time.Time) float64 {
	return float64(t.Unix()) + float64(t.Nanosecond())/1e9
}

// Time is the inverse of [Seconds].
func Time(x float64) time.Time {
	s, f := math.Modf(x)
	return time.Unix(int64(s), int64(math.Round(f*1e9)))
}

// Data is a series in memory: points ascending in X, the counterpart of the
// data collection of a Swift Charts mark. A chart never draws more of it than
// its width can show. Data keeps an index of the minima and maxima of its
// values, so that the chart reduces any part of it to the four points per
// pixel column that [M4] keeps in a time that depends on the width of the
// chart and not on the number of points: a million points zoom as smoothly
// as a hundred.
//
// The index costs at most half a byte per point. Data takes the slices it
// is made of and does not copy them unless they are out of order; change
// them only through its methods.
//
// Data belongs to the user interface goroutine, like a state. A worker that
// receives new values hands them over with [gift.App.Post] and appends them
// there; the charts that show the data follow.
type Data struct {
	xs, ys []float64
	time   bool
	ix     index
	rev    uint64
	watch  map[string]func()
}

// XY returns the data of xs and ys, which must be of the same length. Points
// out of order are sorted by X, in a copy; points whose X is NaN are left out.
func XY(xs, ys []float64) *Data {
	if len(xs) != len(ys) {
		panic(fmt.Sprintf("charts: XY with %d xs and %d ys", len(xs), len(ys)))
	}
	d := &Data{}
	d.set(xs, ys)
	return d
}

// Times returns the data of a time series, for a time axis.
func Times(ts []time.Time, ys []float64) *Data {
	if len(ts) != len(ys) {
		panic(fmt.Sprintf("charts: Times with %d times and %d ys", len(ts), len(ys)))
	}
	xs := make([]float64, len(ts))
	for i, t := range ts {
		xs[i] = Seconds(t)
	}
	d := &Data{time: true}
	d.set(xs, ys)
	return d
}

// From returns the data of items, with the X and Y of each from xy, the way
// a Swift Charts mark plots a collection of structs.
//
//	charts.From(sales, func(s Sale) (float64, float64) { return float64(s.Week), s.Amount })
func From[T any](items []T, xy func(T) (float64, float64)) *Data {
	xs, ys := make([]float64, len(items)), make([]float64, len(items))
	for i, it := range items {
		xs[i], ys[i] = xy(it)
	}
	d := &Data{}
	d.set(xs, ys)
	return d
}

// FromTimes is [From] for a time series.
func FromTimes[T any](items []T, xy func(T) (time.Time, float64)) *Data {
	xs, ys := make([]float64, len(items)), make([]float64, len(items))
	for i, it := range items {
		var t time.Time
		t, ys[i] = xy(it)
		xs[i] = Seconds(t)
	}
	d := &Data{time: true}
	d.set(xs, ys)
	return d
}

// set takes xs and ys, sorted into a copy when they are not ascending.
func (d *Data) set(xs, ys []float64) {
	sorted := true
	for i, x := range xs {
		if math.IsNaN(x) || i > 0 && x < xs[i-1] {
			sorted = false
			break
		}
	}
	if !sorted {
		xs, ys = sortedCopy(xs, ys)
	}
	if len(xs) > math.MaxInt32 {
		panic(fmt.Sprintf("charts: %d points are more than Data holds", len(xs)))
	}
	d.xs, d.ys = xs, ys
	d.ix.build(d.ys)
}

func sortedCopy(xs, ys []float64) ([]float64, []float64) {
	order := make([]int, 0, len(xs))
	for i, x := range xs {
		if !math.IsNaN(x) {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(a, b int) bool { return xs[order[a]] < xs[order[b]] })
	sx, sy := make([]float64, len(order)), make([]float64, len(order))
	for i, k := range order {
		sx[i], sy[i] = xs[k], ys[k]
	}
	return sx, sy
}

// Len returns the number of points.
func (d *Data) Len() int { return len(d.xs) }

// At returns point i.
func (d *Data) At(i int) Point { return Point{d.xs[i], d.ys[i]} }

// Extent returns the smallest and the largest X, and false for empty data.
func (d *Data) Extent() (lo, hi float64, ok bool) {
	if len(d.xs) == 0 {
		return 0, 0, false
	}
	return d.xs[0], d.xs[len(d.xs)-1], true
}

// Append adds a point. One at or after the last X costs a few comparisons;
// one before it is inserted in order, which moves the points after it and
// rebuilds the index, so a program that receives values out of order sorts a
// batch before it appends it.
func (d *Data) Append(x, y float64) {
	if math.IsNaN(x) {
		return
	}
	n := len(d.xs)
	if n == 0 || x >= d.xs[n-1] {
		if n >= math.MaxInt32 {
			panic("charts: Data is full")
		}
		d.xs, d.ys = append(d.xs, x), append(d.ys, y)
		d.ix.push(d.ys, n)
	} else {
		i := sort.SearchFloat64s(d.xs, x)
		for i < n && d.xs[i] == x {
			i++
		}
		d.xs, d.ys = slices.Insert(d.xs, i, x), slices.Insert(d.ys, i, y)
		d.ix.build(d.ys)
	}
	d.changed()
}

// AppendTime is [Data.Append] for a time series.
func (d *Data) AppendTime(t time.Time, y float64) { d.Append(Seconds(t), y) }

// DropBefore removes the points before x, for a chart of the last minutes of
// a live signal. It rebuilds the index, so a program calls it now and then,
// when a second or so has gathered, and not for every point it appends.
func (d *Data) DropBefore(x float64) {
	i := sort.SearchFloat64s(d.xs, x)
	if i == 0 {
		return
	}
	d.xs, d.ys = d.xs[i:], d.ys[i:]
	d.ix.build(d.ys)
	d.changed()
}

// DropBeforeTime is [Data.DropBefore] for a time series.
func (d *Data) DropBeforeTime(t time.Time) { d.DropBefore(Seconds(t)) }

func (d *Data) changed() {
	d.rev++
	for _, fn := range d.watch {
		fn()
	}
}

// watchAs calls fn after every change, until it is called again with the
// same key and nil.
func (d *Data) watchAs(key string, fn func()) {
	if fn == nil {
		delete(d.watch, key)
		return
	}
	if d.watch == nil {
		d.watch = map[string]func(){}
	}
	d.watch[key] = fn
}

// span returns the indices of the points from the first at or after lo to
// the last at or before hi, as a half open range.
func (d *Data) span(lo, hi float64) (int, int) {
	i0 := sort.SearchFloat64s(d.xs, lo)
	i1 := sort.Search(len(d.xs), func(i int) bool { return d.xs[i] > hi })
	return i0, max(i0, i1)
}

// nearest returns the index of the point whose X is nearest to x, and false
// for empty data.
func (d *Data) nearest(x float64) (int, bool) {
	n := len(d.xs)
	if n == 0 {
		return 0, false
	}
	i := sort.SearchFloat64s(d.xs, x)
	switch {
	case i == 0:
		return 0, true
	case i == n:
		return n - 1, true
	case x-d.xs[i-1] <= d.xs[i]-x:
		return i - 1, true
	}
	return i, true
}

// step is the typical distance between neighbouring points: the median of a
// sample of them, which a bar takes its width from.
func (d *Data) step() float64 {
	n := len(d.xs)
	if n < 2 {
		return 0
	}
	const sample = 64
	stride := max(1, (n-1)/sample)
	var ds []float64
	for i := 1; i < n; i += stride {
		if dx := d.xs[i] - d.xs[i-1]; dx > 0 {
			ds = append(ds, dx)
		}
	}
	if len(ds) == 0 {
		return 0
	}
	slices.Sort(ds)
	return ds[len(ds)/2]
}
