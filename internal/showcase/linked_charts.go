package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/charts"
)

func init() {
	register(Demo{Name: "linked-charts", Group: "charts", Title: "Linked Charts",
		Summary: "Two charts that zoom and inspect together, as the panels of a dashboard.",
		Width:   640, Height: 520}, linkedChartsDemo)
}

func linkedChartsDemo(ctx *gift.Context) gift.View {
	temps, energy := demoTemperatures(), demoEnergy()
	hall, grid := temps[0], energy[2]
	// snippet:begin
	visible := ctx.State("visible", [2]float64{})
	inspect := ctx.State("inspect", [2]float64{}) // x, and 1 while inspected
	ctx.Read(visible)
	ctx.Read(inspect)
	link := func(c charts.ChartView) charts.ChartView {
		if v := visible.Get(); v[1] > v[0] {
			c = c.Visible(v[0], v[1])
		}
		in := inspect.Get()
		return c.OnVisibleChange(func(lo, hi float64) { visible.Set([2]float64{lo, hi}) }).
			Inspect(in[0], in[1] == 1).
			OnInspect(func(x float64, on bool) {
				if on {
					inspect.Set([2]float64{x, 1})
				} else {
					inspect.Set([2]float64{})
				}
			})
	}
	return ui.VStack(
		link(charts.Chart(charts.Line(hall).Name("Hall 2"))).YTitle("°C").Legend(false).Location(chartZone).Height(220).Width(600),
		link(charts.Chart(charts.Area(grid).Name("Grid").Gradient(true), charts.Line(grid))).YTitle("kW").Location(chartZone).Height(220).Width(600),
	).Gap(16).Align(geom.Leading)
	// snippet:end
}
