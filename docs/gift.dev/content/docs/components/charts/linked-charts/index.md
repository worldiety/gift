---
title: Linked Charts
weight: 7
---

Charts of one moment belong together: the temperature of a hall and the power it draws, the speed of a machine and
its vibration. Linked, they zoom together and inspect together: a range dragged across one shows in both, and a
pointer over either shows the values of the same moment in both, as the panels of a dashboard do.

{{< gallery-shot alt="Two charts of a day, the temperature of hall 2 and the power drawn from the grid, one above the other with the same hours" >}}

{{< gallery-code >}}

## Usage

A chart tells of its visible range through `OnVisibleChange` and of its inspection through `OnInspect`; it is told
of them through `Visible` and `Inspect`. Keep the two in states, give them to every chart and set them from every
chart, and the charts follow each other. A chart that is told the range it has just reported, or the moment it is
inspecting, keeps what it is doing, so a finger that holds the inspection of one chart keeps holding it.

A range that comes as a step of a pinch or a drag is shown at once, so that the other chart follows frame by frame;
one that jumps somewhere else, a button that shows the last week say, glides there.

## Related

- [Chart Zoom](../chart-zoom/) for the gestures of the zoom.
- [Line Chart](../line-chart/) for the inspection.
