// Package charts draws charts after Swift Charts: marks on a pair of axes,
// with a legend, an inspection of the values under the pointer, and zoom.
//
//	charts.Chart(
//		charts.Area(hall).Gradient(true),
//		charts.Line(hall).Name("Hall 2"),
//		charts.Line(yard).Name("Yard").Interpolation(charts.Monotone),
//		charts.RuleY(23).Label("Limit").Dash(4, 3),
//	).YTitle("°C")
//
// # Marks
//
// A chart is made of marks, drawn in the order they are given: [Line],
// [Area], [Bar] and [Scatter] draw a series, [RuleY] and [RuleX] a line at
// one value, [BandY] and [BandX] a shaded range, [Annotation] a text at a
// point. They are Swift Charts' LineMark, AreaMark, BarMark, PointMark,
// RuleMark, RectangleMark and the annotation modifier. The axes, the grid
// and the legend follow from the marks; [ChartView] has the modifiers of the
// chart as a whole, from [ChartView.XDomain] to [ChartView.Categories].
//
// Bars and areas of several series stack, as in Swift Charts, or stand side
// by side; see [ChartView.Stacking]. A chart of [Sector] marks is a pie, or a
// ring with [SectorMark.InnerRadius], instead of a pair of axes.
//
// # Data
//
// A series is a [*Data] in memory, made with [XY], [Times], [From] or
// [FromTimes], a [*Source] that is asked for what the chart shows, or a
// [Func]. A missing value is a NaN, see [Missing]: a line and an area break
// there, and a value alone between two missing ones is a dot. Data whose
// missing values are left out rather than written breaks with
// [LineMark.MaxGap] where two points are too far apart.
//
// # Zoom
//
// A chart with a continuous x axis zooms in: two fingers pinch it, a double
// tap zooms in twice where it lands, the mouse drags out the range to zoom
// into, and the wheel zooms with the control or command key held. A zoomed
// chart is dragged with a finger, scrolled sideways on a trackpad and moved
// with the arrow keys, and a button in its corner, the 0 key and the Home
// key show all of it again. The y axis fits what is visible; with
// [ChartView.ZoomY] the zoom takes it as well. A program sets the visible
// range with [ChartView.Visible] and the inspected x with
// [ChartView.Inspect], and learns of them through
// [ChartView.OnVisibleChange] and [ChartView.OnInspect], which is how two
// charts zoom and inspect together.
//
// # Too many points
//
// A chart draws no more than its width can show. Of the points of a pixel
// column a line shows the first and the last, the smallest and the largest:
// the M4 aggregation, which draws the same pixels as all the points and is
// [M4] here. What that costs depends on where the points are.
//
//   - [Data] keeps an index of the extremes of its values, a segment tree
//     of half a byte a point, so that it reduces any range to its columns in
//     O(width · log n): ten million points in under two milliseconds.
//   - A [Source] runs on a goroutine of its own; the chart reduces what it
//     yields as it arrives, in constant memory, cancels a request it no
//     longer needs, and shows what it had until the answer is there.
//   - A [Func] is evaluated once a column.
//   - [ChartView.Budget] bounds the points of all marks together, so that a
//     chart of a hundred series draws each coarser instead of all of them
//     in full.
//
// Values that would break the drawing — far beyond the axis, 1e300, a
// neighbour a million widths away — are clipped before they become
// coordinates, and a [Source] that panics shows its error instead of
// taking the program down.
package charts
