---
title: Bar Chart
weight: 3
---

A bar chart draws a bar from zero to each value, the counterpart of Swift Charts' `BarMark`: values of
categories, as below, or values over time, a day of a year. The bars of several series stack, as in Swift Charts,
or stand side by side.

{{< gallery-shot alt="Two bar charts of the cups of coffee, tea and cocoa sold from Monday to Sunday: stacked, with the cups written in the segments of coffee and tea and a dashed rule for the average, and grouped, three bars to a day" >}}

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

## Stacking and grouping

Bars of several series at the same x stack by default: positive values upwards, negative ones downwards.
`Stacking` on the chart says otherwise:

| Stacking            | What it does                                                          |
|---------------------|-----------------------------------------------------------------------|
| `charts.Stacked`    | on top of each other, the default, as Swift Charts' `.standard`       |
| `charts.Normalized` | every stack as high as the axis, which shows shares in per cent       |
| `charts.Grouped`    | side by side, as Swift Charts' `position(by:)`                        |
| `charts.Unstacked`  | every series from zero, the later over the earlier                    |

Bars too thin to draw on their own stack by pixel column: each series shows the largest of its values in the
column, which over many points is an upper bound of the stack rather than a blur.

## Values

`ValueLabels(true)` writes its value on every bar of a mark: in the segment when the bars stack, above the bar
otherwise, below it when it is negative; a normalized stack shows its share. A label that does not fit is left out,
and so are all of them when there are more than a hundred bars to read.

## API

{{< api pkg="charts" type="BarMark" >}}

## Related

- [Line Chart](../line-chart/) for the chart as a whole, its rules and its inspection.
