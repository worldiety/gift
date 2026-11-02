---
title: Charts
weight: 8
---

Charts after [Swift Charts](https://developer.apple.com/documentation/charts): marks on a pair of axes that fit the
data, a legend, the values under the pointer, and zoom. They live in package
[`ui/charts`](https://github.com/worldiety/gift/tree/main/ui/charts).

- [Line Chart](line-chart/): series as lines, broken where values are missing, with rules for limits.
- [Area Chart](area-chart/): the area under a series, faded out towards zero.
- [Bar Chart](bar-chart/): bars for categories or over time.
- [Scatter Chart](scatter-chart/): a dot for every point, with bands for ranges.
- [Chart Zoom](chart-zoom/): drilling into millions of points, in memory or from a database.

A chart is made of marks, drawn in the order they are given:

| Swift Charts      | gift                                        |
|-------------------|---------------------------------------------|
| `LineMark`        | `charts.Line(series)`                       |
| `AreaMark`        | `charts.Area(series)`                       |
| `BarMark`         | `charts.Bar(series)`                        |
| `PointMark`       | `charts.Scatter(series)`                    |
| `RuleMark`        | `charts.RuleY(y)`, `charts.RuleX(x)`        |
| `RectangleMark`   | `charts.BandY(lo, hi)`, `charts.BandX(lo, hi)` |
| `LinePlot` of a function | `charts.Line(charts.Func(f))`        |

A series is a `*charts.Data` in memory, a `*charts.Source` the chart asks for what it shows, or a `charts.Func`.
However many points it has, a chart draws no more than its width can show; see [Chart Zoom](chart-zoom/).
