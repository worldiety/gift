---
title: Scatter Chart
weight: 4
---

A scatter chart draws a dot for every point, the counterpart of Swift Charts' `PointMark`: two measurements against
each other, the temperature of two machines over their load below, with the range a temperature should stay in as
a band.

{{< gallery-shot alt="Dots of the temperature of a press and of a lathe against their load from 20 to 95 percent, the press rising from 40 to 70 °C and the lathe from 33 to 50 °C, over a shaded band labelled Normal from 40 to 55 °C" >}}

{{< gallery-code >}}

## Usage

```go
charts.Chart(
	charts.Scatter(press).Name("Press"),
	charts.BandY(40, 55).Label("Normal"),
).XTitle("Load %").YTitle("°C").Zoomable(false)
```

`Size` sets the diameter of the dots, 7 by default. A scatter of more points than the chart has pixel columns does
not draw them all: it shows the [M4](../chart-zoom/#too-many-points) points of each column, the first, the last,
the lowest and the highest, which keeps the outliers.

`BandY` and `BandX` shade a range across the whole plot, the counterpart of a Swift Charts `RectangleMark` that
spans it; `BandTime` takes two times. A band takes the accent colour at 12 % by default.

## API

{{< api pkg="charts" type="ScatterMark" >}}

{{< api pkg="charts" type="BandMark" >}}

## Related

- [Line Chart](../line-chart/) for the chart as a whole and its inspection.
