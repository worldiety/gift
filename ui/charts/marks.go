package charts

import (
	"time"

	"github.com/worldiety/gift/render"
	"github.com/worldiety/gift/ui"
)

// Mark is what a chart draws: a [LineMark], [AreaMark], [BarMark],
// [ScatterMark], [RuleMark] or [BandMark], the building blocks of Swift
// Charts. The marks of a chart are drawn in the order they are given, all on
// the same axes.
type Mark interface{ spec() markSpec }

type kind uint8

const (
	kindLine kind = iota
	kindArea
	kindBar
	kindScatter
	kindRuleX
	kindRuleY
	kindBandX
	kindBandY
)

// markSpec is the declaration of a mark of any kind; each kind uses the
// fields it needs.
type markSpec struct {
	kind     kind
	data     Series
	name     string
	color    ui.Color
	hasColor bool
	width    float32
	dash     [2]float32
	interp   Interpolation
	maxGap   float64
	opacity  float32
	gradient bool
	symbols  bool
	radius   float32
	a, b     float64
	label    string
}

// Interpolation is how a line or an area runs from one point to the next.
type Interpolation uint8

const (
	// Linear runs straight from point to point.
	Linear Interpolation = iota
	// Monotone runs in a smooth curve that does not overshoot: between two
	// points it never goes above the higher or below the lower one, which is
	// what a smoothed measurement must not do.
	Monotone
	// StepAfter holds each value until the next point and steps there: the
	// reading of a meter, the state of a switch.
	StepAfter
	// StepBefore steps at each point to its value, which held since the
	// point before.
	StepBefore
	// StepMiddle steps halfway between two points.
	StepMiddle
)

// --- line ----------------------------------------------------------------------

// LineMark draws a series as a line, the counterpart of Swift Charts'
// LineMark. A missing value breaks the line, and so does a distance between
// two points beyond [LineMark.MaxGap]; a value alone between two gaps is a
// dot.
type LineMark struct{ s markSpec }

// Line returns a line through the points of s.
//
//	charts.Line(temperatures).Name("Hall 2")
func Line(s Series) LineMark { return LineMark{markSpec{kind: kindLine, data: s, width: 2}} }

// Name names the series in the legend and in the inspection.
func (m LineMark) Name(s string) LineMark { m.s.name = s; return m }

// Color sets the colour; by default each series takes the next colour of the
// chart's palette, the accent colour first.
func (m LineMark) Color(c ui.Color) LineMark { m.s.color, m.s.hasColor = c, true; return m }

// Width sets the width of the line in logical pixels; the default is 2.
func (m LineMark) Width(px float32) LineMark { m.s.width = max(px, 0.5); return m }

// Dash draws the line in dashes on pixels long with off pixels between
// them, for a forecast, a target, a series that is less certain.
func (m LineMark) Dash(on, off float32) LineMark { m.s.dash = [2]float32{on, off}; return m }

// Interpolation sets how the line runs between points; the default is
// [Linear]. Smoothing and steps apply where the points are apart; where
// there are several to a pixel column, the line is the [M4] of the column.
func (m LineMark) Interpolation(i Interpolation) LineMark { m.s.interp = i; return m }

// MaxGap breaks the line between two points that are further apart than dx,
// for a series whose missing values are left out rather than written as
// NaN: a recording that stopped at night. On a time axis dx is in seconds;
// [MaxGapDuration] takes a duration.
func (m LineMark) MaxGap(dx float64) LineMark { m.s.maxGap = dx; return m }

// MaxGapDuration is [LineMark.MaxGap] on a time axis.
func (m LineMark) MaxGapDuration(d time.Duration) LineMark { return m.MaxGap(d.Seconds()) }

// Symbols draws a dot on every point when the points are far enough apart
// to tell them apart, as Swift Charts' symbol modifier does.
func (m LineMark) Symbols(b bool) LineMark { m.s.symbols = b; return m }

func (m LineMark) spec() markSpec { return m.s }

// --- area ----------------------------------------------------------------------

// AreaMark fills the space between a series and zero, the counterpart of
// Swift Charts' AreaMark. Missing values break it like a line.
type AreaMark struct{ s markSpec }

// Area returns the area under the points of s.
func Area(s Series) AreaMark {
	return AreaMark{markSpec{kind: kindArea, data: s, opacity: 0.3}}
}

// Name names the series in the legend and in the inspection.
func (m AreaMark) Name(s string) AreaMark { m.s.name = s; return m }

// Color sets the colour; see [LineMark.Color].
func (m AreaMark) Color(c ui.Color) AreaMark { m.s.color, m.s.hasColor = c, true; return m }

// Opacity sets how opaque the area is, 0.3 by default, so that a line on it
// and the grid behind it show.
func (m AreaMark) Opacity(f float32) AreaMark { m.s.opacity = min(max(f, 0), 1); return m }

// Gradient fades the area out towards zero.
func (m AreaMark) Gradient(b bool) AreaMark { m.s.gradient = b; return m }

// Interpolation sets how the edge of the area runs between points.
func (m AreaMark) Interpolation(i Interpolation) AreaMark { m.s.interp = i; return m }

// MaxGap breaks the area where two points are further apart than dx; see
// [LineMark.MaxGap].
func (m AreaMark) MaxGap(dx float64) AreaMark { m.s.maxGap = dx; return m }

// MaxGapDuration is [AreaMark.MaxGap] on a time axis.
func (m AreaMark) MaxGapDuration(d time.Duration) AreaMark { return m.MaxGap(d.Seconds()) }

func (m AreaMark) spec() markSpec { return m.s }

// --- bar -----------------------------------------------------------------------

// BarMark draws a bar from zero to each value, the counterpart of Swift
// Charts' BarMark. Bars are as wide as the points are apart, less a gap;
// when they would be narrower than two pixels, each pixel column shows the
// highest and the lowest bar that falls into it, so a year of daily values
// becomes a skyline instead of a blur.
type BarMark struct{ s markSpec }

// Bar returns a bar for every point of s.
func Bar(s Series) BarMark {
	return BarMark{markSpec{kind: kindBar, data: s, width: 0.8, radius: 2}}
}

// Name names the series in the legend and in the inspection.
func (m BarMark) Name(s string) BarMark { m.s.name = s; return m }

// Color sets the colour; see [LineMark.Color].
func (m BarMark) Color(c ui.Color) BarMark { m.s.color, m.s.hasColor = c, true; return m }

// Width sets the share of the distance between two points a bar takes, 0.8
// by default.
func (m BarMark) Width(f float32) BarMark { m.s.width = min(max(f, 0.05), 1); return m }

// CornerRadius rounds the corners of the bars; the default is 2.
func (m BarMark) CornerRadius(px float32) BarMark { m.s.radius = max(px, 0); return m }

func (m BarMark) spec() markSpec { return m.s }

// --- scatter -----------------------------------------------------------------------

// ScatterMark draws a dot on every point, the counterpart of Swift Charts'
// PointMark. More dots than the chart has pixel columns are not all drawn:
// it then shows the [M4] points of each column, which keeps the outliers.
type ScatterMark struct{ s markSpec }

// Scatter returns a dot for every point of s.
func Scatter(s Series) ScatterMark {
	return ScatterMark{markSpec{kind: kindScatter, data: s, width: 7}}
}

// Name names the series in the legend and in the inspection.
func (m ScatterMark) Name(s string) ScatterMark { m.s.name = s; return m }

// Color sets the colour; see [LineMark.Color].
func (m ScatterMark) Color(c ui.Color) ScatterMark { m.s.color, m.s.hasColor = c, true; return m }

// Size sets the diameter of the dots; the default is 7.
func (m ScatterMark) Size(px float32) ScatterMark { m.s.width = max(px, 1); return m }

func (m ScatterMark) spec() markSpec { return m.s }

// --- rule ----------------------------------------------------------------------

// RuleMark is a line across the whole plot at one value, the counterpart of
// Swift Charts' RuleMark: a limit, a target, the moment something happened.
type RuleMark struct{ s markSpec }

// RuleY returns a horizontal line at y.
func RuleY(y float64) RuleMark { return RuleMark{markSpec{kind: kindRuleY, a: y, width: 1}} }

// RuleX returns a vertical line at x.
func RuleX(x float64) RuleMark { return RuleMark{markSpec{kind: kindRuleX, a: x, width: 1}} }

// RuleTime returns a vertical line at t, on a time axis.
func RuleTime(t time.Time) RuleMark { return RuleX(Seconds(t)) }

// Label sets a text at the end of the line.
func (m RuleMark) Label(s string) RuleMark { m.s.label = s; return m }

// Color sets the colour; the default is that of secondary text.
func (m RuleMark) Color(c ui.Color) RuleMark { m.s.color, m.s.hasColor = c, true; return m }

// Width sets the width of the line; the default is 1.
func (m RuleMark) Width(px float32) RuleMark { m.s.width = max(px, 0.5); return m }

// Dash draws the line in dashes; see [LineMark.Dash].
func (m RuleMark) Dash(on, off float32) RuleMark { m.s.dash = [2]float32{on, off}; return m }

func (m RuleMark) spec() markSpec { return m.s }

// --- band ----------------------------------------------------------------------

// BandMark shades a range across the whole plot, the counterpart of a Swift
// Charts RectangleMark that spans the plot: the range a value should stay
// in, the hours a machine was down.
type BandMark struct{ s markSpec }

// BandY returns a horizontal band from lo to hi.
func BandY(lo, hi float64) BandMark {
	return BandMark{markSpec{kind: kindBandY, a: min(lo, hi), b: max(lo, hi), opacity: 0.12}}
}

// BandX returns a vertical band from lo to hi.
func BandX(lo, hi float64) BandMark {
	return BandMark{markSpec{kind: kindBandX, a: min(lo, hi), b: max(lo, hi), opacity: 0.12}}
}

// BandTime returns a vertical band from one time to another, on a time
// axis.
func BandTime(from, to time.Time) BandMark { return BandX(Seconds(from), Seconds(to)) }

// Label sets a text in the top leading corner of the band.
func (m BandMark) Label(s string) BandMark { m.s.label = s; return m }

// Color sets the colour, shown at [BandMark.Opacity]; the default is the
// accent colour.
func (m BandMark) Color(c ui.Color) BandMark { m.s.color, m.s.hasColor = c, true; return m }

// Opacity sets how opaque the band is, 0.12 by default.
func (m BandMark) Opacity(f float32) BandMark { m.s.opacity = min(max(f, 0), 1); return m }

func (m BandMark) spec() markSpec { return m.s }

// --- colours -----------------------------------------------------------------

// palette are the colours of the series after the first, which takes the
// accent colour: the system colours of iOS, in their light and their dark
// variants.
var palette = [2][]render.Color{
	{ui.RGB(52, 199, 89), ui.RGB(255, 149, 0), ui.RGB(175, 82, 222), ui.RGB(255, 59, 48),
		ui.RGB(50, 173, 230), ui.RGB(255, 204, 0), ui.RGB(255, 45, 85), ui.RGB(88, 86, 214), ui.RGB(162, 132, 94)},
	{ui.RGB(48, 209, 88), ui.RGB(255, 159, 10), ui.RGB(191, 90, 242), ui.RGB(255, 69, 58),
		ui.RGB(100, 210, 255), ui.RGB(255, 214, 10), ui.RGB(255, 55, 95), ui.RGB(94, 92, 230), ui.RGB(172, 142, 104)},
}

// seriesColor is the colour of the i-th series that has none of its own.
func seriesColor(i int) render.Color {
	if i == 0 {
		return ui.ResolveColor(ui.ColorAccent)
	}
	dark := 0
	if ui.CurrentTheme().IsDark() {
		dark = 1
	}
	p := palette[dark]
	return p[(i-1)%len(p)]
}
