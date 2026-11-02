---
title: Chart Zoom
weight: 6
---

Every chart with a continuous x axis can be zoomed into, and however many points it has, it draws no more than its
width can show. The upper chart below holds a million points of a vibration sensor in memory; the lower one asks a
source for a year of one value a second, 31 million points, and asks again, for the minutes of the hour it arrives
at, as it is zoomed into, the way a map asks for tiles.

{{< gallery-shot alt="Two charts: a million points of a signal as a dense band with single spikes and a gap, and a year of temperatures from a source as a band rising from January to July and falling to December" >}}

{{< gallery-code >}}

## Zooming

| Input                          | What it does                                                    |
|--------------------------------|-----------------------------------------------------------------|
| Two fingers                    | zoom about their centre                                         |
| Double tap                     | zooms in twice about the tap                                    |
| Mouse, dragged                 | zooms into the range dragged across                             |
| Wheel with control or command  | zooms about the cursor; so does a trackpad pinch                |
| One finger, on a zoomed chart  | drags it sideways; flicked, it glides on                        |
| Trackpad, sideways             | drags a zoomed chart                                            |
| Arrow keys, plus, minus        | drag, zoom about the middle                                     |
| The button in the corner, 0, Home | show all of it again                                         |

The y axis fits what is visible and glides to it. A finger that drags up or down a chart that is not zoomed goes on
to the scroll view around it, and so does the wheel without a key, so a chart in a long page does not trap it.

`ZoomY(true)` lets the zoom take the y axis as well: two fingers scale both axes, the mouse drags out a rectangle,
and a zoomed chart is dragged any way.

`Visible(lo, hi)` sets the visible range and `OnVisibleChange` hears of it: two charts that are given the same range
and set it when either changes zoom together, as [Linked Charts](../linked-charts/) shows. `MinSpan` sets how far a zoom goes, by default down to eight points
of the densest data; `Zoomable(false)` turns zooming off.

## Too many points

Of the points that fall into a pixel column, a line shows four: the first and the last, the lowest and the
highest. That is the M4 aggregation of Jugel et al., and drawn as a line it covers exactly the pixels all the
points would: the knocks in the signal above are single points among a million, and each of them shows. Missing
values are kept, so a gap is a gap at any zoom. What the reduction costs depends on where the points are:

- **`*charts.Data`**, in memory, keeps an index of the extremes of its values, half a byte a point. A chart reduces
  any range of it to its columns in O(width · log n): ten million points in under two milliseconds on an Apple
  M-series processor, and a zoom is as smooth with them as with a hundred.
- **`*charts.Source`** runs on a goroutine of its own. The chart reduces what it yields as it arrives, in constant
  memory, has at most one request of it under way and cancels one it no longer needs; until the answer arrives it
  shows what it had.
- **`charts.Func`** is evaluated once a pixel column.

`Budget` bounds the points of all marks together, 200,000 by default: a chart of a hundred series draws each of
them coarser rather than more. Values far beyond the axis are clipped before they become coordinates, a value of
1e300 included.

## Live data

`Append` adds a point, in a few comparisons when it comes after the last; `DropBefore` lets the old ones go, for a
chart of the last minutes. A chart that is not zoomed follows the data, and a zoomed one that shows its end keeps
showing it. Data belongs to the user interface goroutine; a worker hands new values over with `App.Post`.

```go
app.Post(func() {
	signal.AppendTime(t, v)
	signal.DropBeforeTime(t.Add(-10 * time.Minute))
})
```

## Sources

A source answers two questions on a goroutine of the chart's: how far its data reaches, for the chart before it is
zoomed, and which points there are between two times, at the resolution the chart can show. The request reaches
half a width beyond either side, so that a drag does not have to wait. A source may return every point in the
range — the chart reduces them as they arrive — but does better to reduce them where they are with `charts.M4`,
so that only what the chart can show leaves the database. This one reads a column of the time series database of
[nago](https://github.com/worldiety/nago/tree/main/pkg/ndb/tsdb), whose iterator yields points in order:

```go
src := charts.NewSource(charts.SourceConfig{
	Time: true,
	Extent: func(ctx context.Context) (float64, float64, error) {
		return first, last, nil // the first and the last time of the column, in seconds
	},
	Points: func(ctx context.Context, r charts.Request) (iter.Seq[charts.Point], error) {
		raw := func(yield func(charts.Point) bool) {
			for p := range col.IterF64(r.FromTime().UnixMilli(), r.ToTime().UnixMilli()) {
				if !yield(charts.Point{X: float64(p.X) / 1000, Y: float64(p.Y)}) {
					return
				}
			}
		}
		return charts.M4(raw, r.From, r.To, r.Buckets), nil
	},
})

charts.Chart(charts.Line(src).Name("Hall 2"))
```

`charts.M4` keeps missing values where they are, which an M4 that only compares values does not. `Refresh` tells the
charts that show a source that its points changed. A source that fails, or panics, shows its error in the chart.

## API

{{< api pkg="charts" type="Data" >}}

{{< api pkg="charts" type="Source" >}}

{{< api pkg="charts" type="SourceConfig" decl="true" >}}

{{< api pkg="charts" type="Request" decl="true" >}}

{{< api-funcs pkg="charts" names="M4,Missing,Seconds,Time" >}}

## Related

- [Line Chart](../line-chart/) for the chart as a whole and its inspection.
- [Map](../../media/map/) zooms the same way and asks for tiles as a chart asks a source.
