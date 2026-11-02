package charts

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

var (
	chartType = gift.RegisterType("charts.Chart")
	plotType  = gift.RegisterType("charts.plot")
)

// ChartView is a chart: marks on a pair of axes, with a legend. It is
// created by [Chart].
type ChartView struct {
	marks          []Mark
	xdom, ydom     [2]float64
	hasX, hasY     bool
	xTime          bool
	zero           bool
	xTitle, yTitle string
	xFormat        func(float64) string
	yFormat        func(float64) string
	loc            *time.Location
	cats           []string
	still          bool
	visible        [2]float64
	hasVisible     bool
	onVisible      func(lo, hi float64)
	minSpan        float64
	noInspect      bool
	onInspect      func(x float64, ok bool)
	legend         int8
	leading        bool
	width, height  float32
	budget         int
	key, label     string
	flex           float32
}

// Chart returns a chart of marks, the counterpart of Swift Charts' Chart:
//
//	charts.Chart(
//		charts.Line(hall).Name("Hall"),
//		charts.Line(yard).Name("Yard"),
//		charts.RuleY(25).Label("Limit").Dash(4, 3),
//	).YTitle("°C")
//
// The axes fit the data, the y axis to what is visible, and follow it as
// it changes. A chart with a continuous x axis can be zoomed: pinched,
// double tapped, and with a mouse a range dragged across it, or the wheel
// turned with the control or command key held; zoomed, it is dragged with
// a finger or scrolled sideways on a trackpad. Its data is drawn at the
// resolution of the screen, however many points it has; see [Data] and
// [Source].
//
// A mouse over the chart, or a finger that rests on it, shows the values at
// that point of every series.
func Chart(marks ...Mark) ChartView {
	return ChartView{marks: marks, height: 220, budget: 200_000}
}

// XDomain fixes the x axis to [lo, hi], the whole of the chart when it is
// not zoomed, instead of the extent of the data.
func (v ChartView) XDomain(lo, hi float64) ChartView {
	v.xdom, v.hasX = [2]float64{min(lo, hi), max(lo, hi)}, lo != hi
	return v
}

// XDomainTime is [ChartView.XDomain] for a time axis, which it makes the x
// axis.
func (v ChartView) XDomainTime(from, to time.Time) ChartView {
	v = v.XDomain(Seconds(from), Seconds(to))
	v.xTime = true
	return v
}

// TimeAxis makes the x axis a time axis, for data whose X are seconds
// since the Unix epoch made with [XY] or a [Func]. [Times], [FromTimes] and
// a [Source] with [SourceConfig.Time] make one by themselves.
func (v ChartView) TimeAxis() ChartView { v.xTime = true; return v }

// YDomain fixes the y axis to [lo, hi] instead of fitting it to the visible
// values.
func (v ChartView) YDomain(lo, hi float64) ChartView {
	v.ydom, v.hasY = [2]float64{min(lo, hi), max(lo, hi)}, lo != hi
	return v
}

// YIncludesZero keeps zero on the y axis; a chart with bars or areas always
// does.
func (v ChartView) YIncludesZero(b bool) ChartView { v.zero = b; return v }

// XTitle sets the title of the x axis, under its labels at the trailing
// end, as Swift Charts' chartXAxisLabel does.
func (v ChartView) XTitle(s string) ChartView { v.xTitle = s; return v }

// YTitle sets the title of the y axis, above it.
func (v ChartView) YTitle(s string) ChartView { v.yTitle = s; return v }

// XFormat sets the labels of the x axis and of the inspection.
func (v ChartView) XFormat(fn func(x float64) string) ChartView { v.xFormat = fn; return v }

// YFormat sets the labels of the y axis and of the values in the
// inspection.
func (v ChartView) YFormat(fn func(y float64) string) ChartView { v.yFormat = fn; return v }

// Location sets the time zone of a time axis; the default is time.Local.
func (v ChartView) Location(loc *time.Location) ChartView { v.loc = loc; return v }

// Categories makes the x axis one of names: the X of a point is the index
// of its name, so a bar chart of four products plots the points 0 to 3 and
// labels them with the four names.
//
//	charts.Chart(charts.Bar(charts.XY([]float64{0, 1, 2}, sales))).Categories("Tea", "Coffee", "Cocoa")
func (v ChartView) Categories(names ...string) ChartView { v.cats = names; return v }

// Zoomable sets whether the chart can be zoomed and dragged; it can by
// default.
func (v ChartView) Zoomable(b bool) ChartView { v.still = !b; return v }

// Visible shows [lo, hi] of the x axis, the part of the chart zoomed into,
// and follows it when it changes. With [ChartView.OnVisibleChange] the
// visible range belongs to the program: two charts that are given the same
// range and set it when it changes zoom together.
func (v ChartView) Visible(lo, hi float64) ChartView {
	v.visible, v.hasVisible = [2]float64{min(lo, hi), max(lo, hi)}, lo != hi
	return v
}

// OnVisibleChange sets the function called when the visible range of the x
// axis changes by zooming or dragging, with the range it moves to.
func (v ChartView) OnVisibleChange(fn func(lo, hi float64)) ChartView { v.onVisible = fn; return v }

// MinSpan sets the narrowest range of x a zoom goes down to. The default is
// eight points of the densest [Data], and a millionth of the chart for a
// [Source].
func (v ChartView) MinSpan(dx float64) ChartView { v.minSpan = dx; return v }

// Inspection sets whether a pointer over the chart shows the values there;
// it does by default.
func (v ChartView) Inspection(b bool) ChartView { v.noInspect = !b; return v }

// OnInspect sets the function called when the inspected x changes, and
// with false when the inspection ends: to show the moment in a second chart,
// a map or a table.
func (v ChartView) OnInspect(fn func(x float64, ok bool)) ChartView { v.onInspect = fn; return v }

// Legend shows or hides the legend. By default it shows when two or more
// marks have a name.
func (v ChartView) Legend(b bool) ChartView {
	v.legend = -1
	if b {
		v.legend = 1
	}
	return v
}

// YAxisLeading puts the y axis on the leading side; Swift Charts and this
// chart put it on the trailing side by default.
func (v ChartView) YAxisLeading(b bool) ChartView { v.leading = b; return v }

// Height sets the height; the default is 220.
func (v ChartView) Height(px float32) ChartView { v.height = max(px, 40); return v }

// Width sets the width; by default the chart is as wide as it may be.
func (v ChartView) Width(px float32) ChartView { v.width = max(px, 40); return v }

// Budget sets how many points the chart draws at most, all marks together;
// the default is 200,000. A chart of many series draws each of them at a
// coarser resolution rather than more points than this.
func (v ChartView) Budget(points int) ChartView { v.budget = max(points, 1000); return v }

// Label sets the accessible name.
func (v ChartView) Label(s string) ChartView { v.label = s; return v }

// Key sets the reconciliation key.
func (v ChartView) Key(s string) ChartView { v.key = s; return v }

// Flex sets the flex factor in a stack.
func (v ChartView) Flex(f float32) ChartView { v.flex = f; return v }

// ViewType implements gift.View.
func (ChartView) ViewType() gift.TypeID { return chartType }

// Build implements gift.View.
func (v ChartView) Build(*gift.BuildContext) gift.Element {
	return gift.Element{Key: v.key, Flex: v.flex, Layouter: passLayout{},
		Children: []gift.View{gift.Component("charts.chart", v.body)}}
}

type passLayout struct{}

func (passLayout) Layout(ctx *gift.LayoutContext, c geom.Constraints) geom.Size {
	s := ctx.Measure(0, c)
	ctx.Place(0, geom.Point{})
	return s
}

// isTime reports whether the x axis is a time axis: the domain was given as
// times or a series is a time series.
func (v ChartView) isTime() bool {
	if v.xTime {
		return true
	}
	for _, m := range v.marks {
		switch d := m.spec().data.(type) {
		case *Data:
			if d.time {
				return true
			}
		case *Source:
			if d.cfg.Time {
				return true
			}
		}
	}
	return false
}

func (v ChartView) location() *time.Location {
	if v.loc != nil {
		return v.loc
	}
	return time.Local
}

// formatX labels a single x, as the inspection shows it.
func (v ChartView) formatX(x, span float64) string {
	switch {
	case v.xFormat != nil:
		return v.xFormat(x)
	case len(v.cats) > 0:
		if i := int(math.Round(x)); i >= 0 && i < len(v.cats) {
			return v.cats[i]
		}
		return ""
	case v.isTime():
		return timeValue(Time(x).In(v.location()), span)
	}
	return valueFormat(x)
}

func (v ChartView) formatY(y float64) string {
	if v.yFormat != nil && !Missing(y) {
		return v.yFormat(y)
	}
	return valueFormat(y)
}

// --- state ---------------------------------------------------------------------

// state is what a chart keeps across builds: the visible range, what it
// has of its sources, and what the last layout worked out for the next
// build, which makes the labels.
type state struct {
	v    ChartView
	tok  gift.Token
	bump func()
	path string

	loads   map[*Source]*loading
	watched map[Series]bool

	vp       viewport
	given    [2]float64
	hasGiven bool

	// From the last layout.
	size        geom.Size
	plot        geom.Rect
	density     float32
	lo, hi      float64
	ylo, yhi    float64
	xt, yt      []tick
	builtX      []tick
	builtY      []tick
	need        [3]float64
	cache       []markCache
	ys          eased2
	empty, busy bool
	errText     string

	plan        plan
	legendAt    []geom.Rect
	callout     geom.Rect
	calloutRows []geom.Rect

	in   inspection
	drag dragState
	// node is the plot's node, which the surface hands its input to.
	node *plotNode
}

// markCache is what a mark showed in the last layout: its points, reduced
// for the visible range and the width of the plot.
type markCache struct {
	pts        []Point
	key        cacheKey
	color      render.Color
	nearest    Point
	hasNearest bool
}

type cacheKey struct {
	lo, hi  float64
	buckets int
	rev     uint64
	src     any
}

// body is the chart: a plot node with its labels as children.
func (v ChartView) body(ctx *gift.Context) gift.View {
	mem := ctx.State("state", (*state)(nil))
	rev := ctx.State("rev", 0)
	ctx.Read(rev)
	st := mem.Get()
	if st == nil {
		st = &state{loads: map[*Source]*loading{}, watched: map[Series]bool{}, path: ctx.Path()}
		st.tok = ctx.Request()
		st.bump = func() { rev.Set(rev.Get() + 1) }
		st.node = &plotNode{st: st}
		st.node.gs = st.node.gestures()
		mem.Set(st)
		ctx.OnUnmount("charts.stop", st.stop)
	}
	st.v = v
	if len(st.cache) != len(v.marks) {
		st.cache = make([]markCache, len(v.marks))
	}
	st.watch()
	if v.hasVisible && (!st.hasGiven || st.given != v.visible) {
		st.given, st.hasGiven = v.visible, true
		st.vp.show(v.visible[0], v.visible[1])
	}
	st.fetch()
	st.inspectValues()
	return plot{st: st, kids: st.children()}
}

// watch follows the data and the sources of the marks, and stops following
// the ones that are gone.
func (st *state) watch() {
	now := map[Series]bool{}
	for _, m := range st.v.marks {
		switch d := m.spec().data.(type) {
		case *Data:
			now[d] = true
		case *Source:
			now[d] = true
		}
	}
	notify := func() { st.tok.Post(st.bump) }
	for s := range now {
		if !st.watched[s] {
			switch d := s.(type) {
			case *Data:
				d.watchAs(st.path, notify)
			case *Source:
				d.watchAs(st.path, notify)
			}
		}
	}
	for s := range st.watched {
		if !now[s] {
			unwatch(s, st.path)
			if src, ok := s.(*Source); ok {
				if ld := st.loads[src]; ld != nil && ld.cancel != nil {
					ld.cancel()
				}
				delete(st.loads, src)
			}
		}
	}
	st.watched = now
}

func unwatch(s Series, path string) {
	switch d := s.(type) {
	case *Data:
		d.watchAs(path, nil)
	case *Source:
		d.watchAs(path, nil)
	}
}

// stop lets go of everything when the chart is unmounted.
func (st *state) stop() {
	for s := range st.watched {
		unwatch(s, st.path)
	}
	for _, ld := range st.loads {
		if ld.cancel != nil {
			ld.cancel()
		}
	}
}

// fetch asks the sources for what the last layout found missing: an
// extent, or points for the visible range.
func (st *state) fetch() {
	for _, m := range st.v.marks {
		src, ok := m.spec().data.(*Source)
		if !ok {
			continue
		}
		ld := st.loads[src]
		if ld == nil {
			ld = &loading{rev: src.rev}
			st.loads[src] = ld
		}
		if ld.rev != src.rev {
			// New points: ask again, and show the old ones until they come.
			ld.rev, ld.hasExt, ld.extErr, ld.err, ld.have = src.rev, false, nil, nil, Request{}
		}
		if !ld.hasExt && !ld.extBusy && ld.extErr == nil && src.cfg.Extent != nil {
			ld.extBusy = true
			fetchExtent(src, st.tok.Post, func(lo, hi float64, err error) {
				ld.extBusy, ld.extErr = false, err
				if err == nil {
					ld.extent, ld.hasExt = [2]float64{lo, hi}, true
				}
				st.bump()
			})
		}
		lo, hi, buckets := st.need[0], st.need[1], int(st.need[2])
		if buckets == 0 || !(hi > lo) {
			continue
		}
		// What is had, what is under way and what failed is not asked for
		// again; a failed request is asked again when the chart wants
		// something else, or when the source has news.
		if covers(ld.have, lo, hi, buckets, ld.extent, ld.hasExt) ||
			(ld.busy || ld.err != nil) && covers(ld.want, lo, hi, buckets, ld.extent, ld.hasExt) {
			continue
		}
		if ld.cancel != nil {
			ld.cancel()
		}
		r := request(lo, hi, buckets, ld.extent, ld.hasExt)
		c, cancel := context.WithCancel(context.Background())
		ld.gen++
		gen := ld.gen
		ld.want, ld.busy, ld.cancel = r, true, cancel
		fetchPoints(c, src, r, st.tok.Post, func(pts []Point, err error) {
			if ld.gen != gen {
				return
			}
			ld.busy, ld.cancel, ld.err = false, nil, err
			if err == nil {
				ld.pts, ld.have = pts, r
			}
			st.bump()
		})
	}
}

// --- children --------------------------------------------------------------------

// The children of a plot, in this order; plan records where each group
// starts.
type plan struct {
	x, y, legend, labels, callout [2]int
	xTitle, yTitle, surface       int
	reset, status                 int
	// labelOf is the mark of each rule or band label.
	labelOf []int
	// legendOf is the mark of each legend entry.
	legendOf []int
}

const (
	labelSize = 11
	titleSize = 12
)

func (st *state) children() []gift.View {
	v := st.v
	var kids []gift.View
	p := plan{xTitle: -1, yTitle: -1, surface: -1, reset: -1, status: -1}
	group := func(r *[2]int, add func()) {
		r[0] = len(kids)
		add()
		r[1] = len(kids)
	}
	axis := func(t tick) gift.View {
		return ui.Text(t.label).FontSize(labelSize).Foreground(ui.ColorSecondaryLabel).MaxLines(1)
	}
	st.builtX, st.builtY = st.xt, st.yt
	group(&p.x, func() {
		for _, t := range st.xt {
			kids = append(kids, axis(t))
		}
	})
	group(&p.y, func() {
		for _, t := range st.yt {
			kids = append(kids, axis(t))
		}
	})
	if v.xTitle != "" {
		p.xTitle = len(kids)
		kids = append(kids, ui.Text(v.xTitle).FontSize(titleSize).Foreground(ui.ColorSecondaryLabel).MaxLines(1))
	}
	if v.yTitle != "" {
		p.yTitle = len(kids)
		kids = append(kids, ui.Text(v.yTitle).FontSize(titleSize).Foreground(ui.ColorSecondaryLabel).MaxLines(1))
	}
	group(&p.legend, func() {
		if !st.showLegend() {
			return
		}
		for i, m := range v.marks {
			s := m.spec()
			if s.name != "" && s.kind <= kindScatter {
				p.legendOf = append(p.legendOf, i)
				kids = append(kids, ui.Text(s.name).FontSize(labelSize).Foreground(ui.ColorSecondaryLabel).MaxLines(1))
			}
		}
	})
	group(&p.labels, func() {
		for i, m := range v.marks {
			s := m.spec()
			if s.label != "" && s.kind >= kindRuleX {
				p.labelOf = append(p.labelOf, i)
				kids = append(kids, ui.Text(s.label).FontSize(labelSize).Foreground(ui.ColorSecondaryLabel).MaxLines(1))
			}
		}
	})
	group(&p.callout, func() {
		if !st.in.on {
			return
		}
		kids = append(kids, ui.Text(v.formatX(st.in.at, st.hi-st.lo)).FontSize(labelSize).Foreground(ui.ColorSecondaryLabel).MaxLines(1))
		for i, m := range v.marks {
			s := m.spec()
			c := &st.cache[i]
			if s.kind > kindScatter || !c.hasNearest {
				continue
			}
			name := s.name
			if name == "" {
				name = "Value"
			}
			kids = append(kids, ui.HStack(
				ui.Text(name).FontSize(titleSize).Foreground(ui.ColorSecondaryLabel).MaxLines(1),
				ui.Spacer(),
				ui.Text(v.formatY(c.nearest.Y)).FontSize(titleSize).Foreground(ui.ColorLabel).MaxLines(1),
			).Gap(12))
		}
	})
	p.surface = len(kids)
	kids = append(kids, surface{n: st.node})
	if !v.still && st.vp.zoomed && len(v.cats) == 0 {
		p.reset = len(kids)
		kids = append(kids, ui.Button(ui.Icon(outline.ZoomOut).Size(16).Foreground(ui.ColorLabel), st.resetZoom).
			Background(ui.Fade(ui.ColorSurface, 0.9)).Border(ui.Border{Width: 1, Color: ui.ColorSeparator}).
			CornerRadius(14).Padding(6).Label("Reset Zoom").Key("charts.reset"))
	}
	if msg := st.statusText(); msg != "" {
		p.status = len(kids)
		kids = append(kids, ui.Text(msg).FontSize(titleSize).Foreground(ui.ColorSecondaryLabel).MaxLines(2))
	}
	st.plan = p
	return kids
}

func (st *state) showLegend() bool {
	switch st.v.legend {
	case 1:
		return true
	case -1:
		return false
	}
	n := 0
	for _, m := range st.v.marks {
		if s := m.spec(); s.name != "" && s.kind <= kindScatter {
			n++
		}
	}
	return n >= 2
}

func (st *state) statusText() string {
	switch {
	case st.errText != "" && st.empty:
		return st.errText
	case st.busy && st.empty:
		return "Loading…"
	case st.empty && st.size.W > 0:
		return "No Data"
	}
	return ""
}

// resetZoom shows the whole chart again.
func (st *state) resetZoom() {
	lo, hi, ok := st.extent()
	if !ok {
		return
	}
	st.vp.animateTo(st.vp.now, lo, hi, false)
	st.visibleChanged(lo, hi)
	st.bump()
}

// visibleChanged tells the program where the chart zooms to.
func (st *state) visibleChanged(lo, hi float64) {
	if st.v.onVisible != nil {
		st.given = [2]float64{lo, hi}
		st.v.onVisible(lo, hi)
	}
}

// --- extent -------------------------------------------------------------------------

// extent is the whole x axis: the given domain, the categories, or what the
// data spans, with room for half a bar at either end.
func (st *state) extent() (lo, hi float64, ok bool) {
	v := st.v
	switch {
	case v.hasX:
		return v.xdom[0], v.xdom[1], true
	case len(v.cats) > 0:
		return -0.5, float64(len(v.cats)) - 0.5, true
	}
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, m := range v.marks {
		s := m.spec()
		var a, b float64
		var have bool
		switch d := s.data.(type) {
		case *Data:
			a, b, have = d.Extent()
			if have && s.kind == kindBar {
				pad := d.step() / 2
				a, b = a-pad, b+pad
			}
		case *Source:
			if ld := st.loads[d]; ld != nil && ld.hasExt {
				a, b, have = ld.extent[0], ld.extent[1], true
			}
		}
		if have {
			lo, hi = min(lo, a), max(hi, b)
		}
	}
	if math.IsInf(lo, 0) {
		return 0, 0, false
	}
	if lo == hi {
		pad := 0.5
		if v.isTime() {
			pad = 1800
		}
		lo, hi = lo-pad, hi+pad
	}
	return lo, hi, true
}

// minSpan is the narrowest range a zoom goes down to.
func (st *state) minSpan(full float64) float64 {
	if st.v.minSpan > 0 {
		return st.v.minSpan
	}
	m := math.Inf(1)
	for _, mk := range st.v.marks {
		if d, ok := mk.spec().data.(*Data); ok {
			if s := d.step(); s > 0 {
				m = min(m, 8*s)
			}
		}
	}
	if math.IsInf(m, 1) {
		m = full * 1e-6
	}
	if st.v.isTime() {
		m = max(m, 0.01)
	}
	return max(m, full*1e-12)
}

// --- helpers ------------------------------------------------------------------------

func describe(v ChartView) string {
	if v.label != "" {
		return v.label
	}
	names := []string{}
	for _, m := range v.marks {
		if s := m.spec(); s.name != "" {
			names = append(names, s.name)
		}
	}
	if len(names) == 0 {
		return "Chart"
	}
	return "Chart of " + strings.Join(names, ", ")
}
