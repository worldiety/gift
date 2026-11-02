---
title: Area Chart
weight: 2
---

An area chart fills the space between a series and zero, the counterpart of Swift Charts' `AreaMark`. With its line
on top it shows a quantity over time: visitors a day, energy used, a queue. The y axis of a chart with an area
always includes zero.

{{< gallery-shot alt="Visitors a day from July to October as a blue line over an area that fades out towards zero, rising from about 800 to 1500 with a peak every weekend" >}}

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

## API

{{< api pkg="charts" type="AreaMark" >}}

## Related

- [Line Chart](../line-chart/) for the line on top, the chart as a whole and its inspection.
