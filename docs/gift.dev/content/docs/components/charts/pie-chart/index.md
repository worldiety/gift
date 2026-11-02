---
title: Pie Chart
weight: 5
---

A pie chart shows the shares of a whole as the slices of a circle, the counterpart of Swift Charts' `SectorMark`.
With an inner radius the circle becomes a ring, a donut, with room in the middle for the total.

{{< gallery-shot alt="A pie of drinks, coffee 58 %, tea 22 %, cocoa 12 % and water 8 %, with the shares written in the slices, and a ring of the power of four machines with 82 kW now in its middle, each with a legend below" >}}

{{< gallery-code >}}

## Usage

A chart with sectors draws them as a circle, clockwise from twelve o'clock in the order they are given, without
axes:

```go
charts.Chart(
	charts.Sector(58).Name("Coffee").Percent(true),
	charts.Sector(22).Name("Tea").Percent(true),
)
```

`InnerRadius(0.6)` leaves the inner 60 % of the radius empty, `AngularInset(2)` leaves two pixels between a slice and
the next, along all its length. `Percent(true)` writes the share of a slice in it, `Label` any text; a text that
does not fit its slice is left out. `Center` on the chart puts a view in the middle of a ring.

The legend stands beside the circle when the chart is wide enough and below it otherwise. A mouse over a slice, or a
tap on it, lifts it out of the circle, fades the others and shows its value and its share.

## API

{{< api pkg="charts" type="SectorMark" >}}

## Related

- [Bar Chart](../bar-chart/) shows shares over time with `Stacking(charts.Normalized)`.
