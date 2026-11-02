---
title: Area Chart
weight: 2
---

An area chart fills the space between a series and zero, the counterpart of Swift Charts' `AreaMark`. With its line
on top it shows a quantity over time: visitors a day, energy used, a queue. The y axis of a chart with an area
always includes zero.

{{< gallery-shot alt="Two area charts: visitors a day from July to October as a line over an area that fades out towards zero, and the power a hall draws over a day as three stacked areas, solar rising to a hill at noon, wind as a band and the grid on top" >}}

{{< gallery-code >}}

## Usage

```go
charts.Chart(
	charts.Area(visitors).Gradient(true).Opacity(0.4),
	charts.Line(visitors).Width(1.5),
)
```

An area and a line of the same series share its colour. `Opacity` sets how opaque the area is, 0.3 by default, so
that the grid behind it shows; `Gradient(true)` fades it out towards zero. Missing values break an area as they
break a line, and `Interpolation` and `MaxGap` work as on a [line](../line-chart/).

## Stacked areas

The areas of several series stack, as in Swift Charts: each lies on the ones before it, which are read at its own
points, between their points in a straight line, so the series need not share their xs. Stacked areas do not lie on
each other and are 0.85 opaque by default. `Stacking(charts.Normalized)` makes every x a whole, for shares over
time; `charts.Unstacked` draws every area from zero. Lines are never stacked.

## API

{{< api pkg="charts" type="AreaMark" >}}

## Related

- [Line Chart](../line-chart/) for the line on top, the chart as a whole and its inspection.
