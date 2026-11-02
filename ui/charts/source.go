package charts

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"math"
	"time"
)

// Series is the data of a mark: a [*Data] in memory, a [*Source] that is
// asked for what the chart shows, or a [Func].
type Series interface{ series() }

func (*Data) series()   {}
func (*Source) series() {}
func (Func) series()    {}

// Func is a function plotted as a series, the counterpart of Swift Charts'
// LinePlot of a function: the chart evaluates it once a pixel column, so it
// costs the same at any zoom. A NaN or an infinite result is a gap. A chart
// that shows only functions needs [ChartView.XDomain].
type Func func(x float64) float64

// Request is what a chart asks a [Source] for: the points between From and
// To, and how many pixel columns it has across them. More than four points
// a column, the first, the last, the smallest and the largest, are more
// than it can show; [M4] reduces any number of points to that, and a source
// that runs it where the points are, in the database say, sends no more.
//
// The range reaches half the visible width beyond either side, so that the
// chart can be dragged that far before it has to ask again.
type Request struct {
	From, To float64
	Buckets  int
}

// FromTime and ToTime are From and To on a time axis.
func (r Request) FromTime() time.Time { return Time(r.From) }

// ToTime is To on a time axis; see [Request.FromTime].
func (r Request) ToTime() time.Time { return Time(r.To) }

// SourceConfig says where a [Source] gets its points.
type SourceConfig struct {
	// Extent returns the smallest and the largest X there is, which the
	// chart shows before it is zoomed. Without it a chart needs
	// [ChartView.XDomain].
	Extent func(ctx context.Context) (lo, hi float64, err error)
	// Points returns the points of a request, ascending in X; a missing
	// value is a NaN Y. It runs on a goroutine of its own, and ctx is
	// cancelled when the chart wants something else before it is done.
	//
	// It may return every point in the range: the chart reduces them with
	// [M4] as they arrive, in constant memory. It does well not to, when the
	// points are many: the time it takes is the time until the chart shows
	// them, and a pass over a billion points takes seconds.
	Points func(ctx context.Context, r Request) (iter.Seq[Point], error)
	// Time says that X is a time in seconds, for a time axis.
	Time bool
}

// Source is a series that is too large to hold, or that lives elsewhere: a
// column of a time series database, a log on disk, a service. A chart asks
// it for the points of what it shows, at the resolution it shows them, and
// asks again when it is zoomed or dragged beyond what it has, the way a map
// asks for tiles: drill down into a year of measurements, and the chart
// fetches the minutes of the hour it arrives at.
//
// A chart asks on a goroutine of its own and has at most one request of a
// source under way; it cancels one that is no longer wanted. Until the
// answer arrives it shows what it had, scaled.
//
// A Source is made once and kept, like a [*Data]; [Source.Refresh] tells the
// charts that show it that its points changed.
type Source struct {
	cfg   SourceConfig
	rev   uint64
	watch map[string]func()
}

// NewSource returns a source of the points of cfg. It panics without
// cfg.Points.
func NewSource(cfg SourceConfig) *Source {
	if cfg.Points == nil {
		panic("charts: NewSource without SourceConfig.Points")
	}
	return &Source{cfg: cfg}
}

// Refresh makes the charts that show s ask for its extent and its points
// again, after new points have arrived. It must be called on the user
// interface goroutine.
func (s *Source) Refresh() {
	s.rev++
	for _, fn := range s.watch {
		fn()
	}
}

func (s *Source) watchAs(key string, fn func()) {
	if fn == nil {
		delete(s.watch, key)
		return
	}
	if s.watch == nil {
		s.watch = map[string]func(){}
	}
	s.watch[key] = fn
}

// loading is what a chart has of a source: its extent and the points of the
// last answer, and the request under way.
type loading struct {
	rev     uint64
	extent  [2]float64
	hasExt  bool
	extBusy bool
	extErr  error

	have, want Request
	pts        []Point
	busy       bool
	cancel     context.CancelFunc
	gen        uint64
	err        error
}

// covers reports whether the points of r are good for showing [lo, hi]
// across buckets columns: they reach both ends, and their columns are not
// much wider than the chart's nor very much narrower.
func covers(r Request, lo, hi float64, buckets int, extent [2]float64, hasExt bool) bool {
	if r.Buckets == 0 {
		return false
	}
	// What lies beyond the extent need not be had.
	if hasExt {
		lo, hi = max(lo, extent[0]), min(hi, extent[1])
	}
	if r.From > lo || r.To < hi {
		return false
	}
	have := (r.To - r.From) / float64(r.Buckets)
	need := (hi - lo) / float64(max(buckets, 1))
	return have <= need*1.5 && have >= need/4
}

// request is the request for showing [lo, hi] across buckets columns: half
// the visible width more on either side, within the extent.
func request(lo, hi float64, buckets int, extent [2]float64, hasExt bool) Request {
	span := hi - lo
	from, to := lo-span/2, hi+span/2
	if hasExt {
		from, to = max(from, extent[0]), min(to, extent[1])
	}
	if !(to > from) {
		from, to = lo, hi
	}
	n := int(math.Ceil(float64(buckets) * (to - from) / span))
	return Request{From: from, To: to, Buckets: max(n, 1)}
}

// errPanicked wraps a panic of a source's function, which runs on a
// goroutine of its own where it would take the program down.
var errPanicked = errors.New("charts: the source panicked")

func safely(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", errPanicked, r)
		}
	}()
	return fn()
}

// fetchExtent asks s for its extent on a goroutine and hands the answer to
// done on the user interface goroutine, through post.
func fetchExtent(s *Source, post func(func()), done func(lo, hi float64, err error)) {
	go func() {
		var lo, hi float64
		err := safely(func() error {
			var err error
			lo, hi, err = s.cfg.Extent(context.Background())
			return err
		})
		if err == nil && !(hi >= lo) {
			err = fmt.Errorf("charts: the source's extent [%v, %v] is empty", lo, hi)
		}
		post(func() { done(lo, hi, err) })
	}()
}

// fetchPoints asks s for the points of r on a goroutine, reduces them as
// they arrive and hands them to done on the user interface goroutine.
func fetchPoints(ctx context.Context, s *Source, r Request, post func(func()), done func(pts []Point, err error)) {
	go func() {
		var pts []Point
		err := safely(func() error {
			seq, err := s.cfg.Points(ctx, r)
			if err != nil || seq == nil {
				return err
			}
			// A source that does not watch ctx is stopped here, a few
			// thousand points after the chart has lost interest.
			checked := func(yield func(Point) bool) {
				n := 0
				for p := range seq {
					if n++; n&4095 == 0 && ctx.Err() != nil {
						return
					}
					if !yield(p) {
						return
					}
				}
			}
			for p := range M4(checked, r.From, r.To, r.Buckets) {
				pts = append(pts, p)
			}
			return nil
		})
		if ctx.Err() != nil {
			return
		}
		post(func() { done(pts, err) })
	}()
}
