---
title: Charts
weight: 8
---

Charts after [Swift Charts](https://developer.apple.com/documentation/charts): marks on a pair of axes that fit the
data, a legend, the values under the pointer, and zoom. They live in package
[`ui/charts`](https://github.com/worldiety/gift/tree/main/ui/charts).

- [Line Chart](line-chart/): series as lines, broken where values are missing, with rules for limits.
- [Area Chart](area-chart/): the area under a series, faded out towards zero.
- [Bar Chart](bar-chart/): bars for categories or over time, stacked or grouped, with their values.
- [Scatter Chart](scatter-chart/): a dot for every point, with bands for ranges, zoomed in both axes.
- [Pie Chart](pie-chart/): shares of a whole as the slices of a pie or a ring.
- [Chart Zoom](chart-zoom/): drilling into millions of points, in memory or from a database.
- [Linked Charts](linked-charts/): charts that zoom and inspect together, as a dashboard.

A chart is made of marks, drawn in the order they are given:

| Swift Charts      | gift                                        |
|-------------------|---------------------------------------------|
| `LineMark`        | `charts.Line(series)`                       |
| `AreaMark`        | `charts.Area(series)`                       |
| `BarMark`         | `charts.Bar(series)`                        |
| `PointMark`       | `charts.Scatter(series)`                    |
| `RuleMark`        | `charts.RuleY(y)`, `charts.RuleX(x)`        |
| `RectangleMark`   | `charts.BandY(lo, hi)`, `charts.BandX(lo, hi)` |
| `SectorMark`      | `charts.Sector(value)`                      |
| `.annotation`     | `charts.Annotation(x, y, text)`, `BarMark.ValueLabels` |
| `MarkStackingMethod`, `position(by:)` | `ChartView.Stacking`  |
| `LinePlot` of a function | `charts.Line(charts.Func(f))`        |

A series is a `*charts.Data` in memory, a `*charts.Source` the chart asks for what it shows, or a `charts.Func`.
However many points it has, a chart draws no more than its width can show; see [Chart Zoom](chart-zoom/).
