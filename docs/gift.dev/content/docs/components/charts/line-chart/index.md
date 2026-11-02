---
title: Line Chart
weight: 1
---

A line chart draws series as lines over a shared pair of axes, the counterpart of Swift Charts' `LineMark`. The
axes fit the data, the legend names the series, and a pointer over the chart shows the values there. A value that
is missing breaks the line: the sensor in the yard below was off for an hour and a half, and the chart shows the
hole instead of a straight line across it.

{{< gallery-shot alt="Two lines of temperatures over a day, Hall 2 at about 20 °C with a dot and the note Gate open at half past five, and Yard from 5 to 13 °C with a gap after one o'clock, under a dashed limit at 23 °C, with hours on the x axis and a legend" >}}

{{< gallery-code >}}

## Usage

Make the data once and keep it, as a program keeps any state; a build only declares the chart:

```go
hall := charts.Times(times, values)          // or charts.XY(xs, ys), charts.FromTimes(rows, fn)

charts.Chart(
	charts.Line(hall).Name("Hall 2"),
	charts.RuleY(23).Label("Limit").Dash(4, 3),
).YTitle("°C")
```

Each series takes the next colour of the palette, the accent colour first; `Color` sets one. `Width` sets the
width of the line, `Dash` draws it in dashes, `Symbols(true)` puts a dot on each point where the points are apart.

## Missing values

A missing value is `math.NaN()` (or an infinity): the line breaks there, and a value alone between two missing ones
is a dot, so that it shows. Data that leaves its missing values out, a recording that stopped at night, breaks
where two points are further apart than `MaxGap`:

```go
charts.Line(meter).MaxGapDuration(15 * time.Minute)
```

The inspection says so as well: a series without a value at the inspected moment shows a dash.

## Interpolation

`Interpolation` sets how the line runs between points where they are apart: `Linear`, `Monotone` (a smooth curve
that never overshoots its points, for a measurement), and `StepAfter`, `StepBefore`, `StepMiddle` (a value that holds
until the next one, the state of a switch). Where there are several points to a pixel column, the line is their
[M4](../chart-zoom/#too-many-points) and the interpolation has nothing to do.

## Annotations

`charts.Annotation(x, y, text)`, or `AnnotationTime` on a time axis, writes a text at a point with a dot on it:
what happened then. `Position` puts the text above, below, before or after the point, and the annotation leaves
the plot when its point does. Rules take a `Label` of their own.

## Inspection

A mouse over the chart, a finger that rests on it or taps it, shows a line at the nearest point and a box with the
time and the value of every series there. `OnInspect` hears of it, to show the same moment elsewhere;
`Inspection(false)` turns it off.

## API

{{< api pkg="charts" type="ChartView" >}}

{{< api pkg="charts" type="LineMark" >}}

{{< api pkg="charts" type="RuleMark" >}}

{{< api pkg="charts" type="AnnotationMark" >}}

## Related

- [Area Chart](../area-chart/) fills the area under a line.
- [Chart Zoom](../chart-zoom/) zooms into a line of millions of points.
- [Linked Charts](../linked-charts/) inspect and zoom together.
