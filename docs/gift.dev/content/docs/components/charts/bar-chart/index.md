---
title: Bar Chart
weight: 3
---

A bar chart draws a bar from zero to each value, the counterpart of Swift Charts' `BarMark`: values of
categories, as below, or values over time, a day of a year.

{{< gallery-shot alt="Seven bars of cups of coffee from Monday to Sunday, highest on Friday at about 200, lowest on Sunday at about 70, with a dashed rule for the average at 145" >}}

{{< gallery-code >}}

## Usage

`Categories` makes the x axis one of names: the X of a point is the index of its name.

```go
cups := charts.XY([]float64{0, 1, 2, 3, 4, 5, 6}, sold)
charts.Chart(charts.Bar(cups)).Categories("Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun")
```

Over a continuous axis, numbers or times, a bar is as wide as the points are apart, `Width` of it, 0.8 by default.
When that is less than two pixels — a year of daily values on a small screen — each pixel column shows the highest
and the lowest bar that falls into it, so the chart stays a skyline and does not turn into a blur.

## API

{{< api pkg="charts" type="BarMark" >}}

## Related

- [Line Chart](../line-chart/) for the chart as a whole, its rules and its inspection.
