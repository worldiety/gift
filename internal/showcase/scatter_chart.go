package showcase

import (
	"sync"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui/charts"
)

func init() {
	register(Demo{Name: "scatter-chart", Group: "charts", Title: "Scatter Chart",
		Summary: "A dot for every point, for two measurements against each other.",
		Width:   640, Height: 320}, scatterChartDemo)
}

var demoMachines = sync.OnceValue(func() [2]*charts.Data {
	var ax, ay, bx, by []float64
	for i := range 160 {
		load := 20 + 75*(0.5+0.5*noise(i*3))
		ax = append(ax, load)
		ay = append(ay, 32+0.38*load+3*noise(i*7))
		load = 20 + 75*(0.5+0.5*noise(i*5+1))
		bx = append(bx, load)
		by = append(by, 28+0.22*load+2.5*noise(i*11))
	}
	return [2]*charts.Data{charts.XY(ax, ay), charts.XY(bx, by)}
})

func scatterChartDemo(ctx *gift.Context) gift.View {
	d := demoMachines()
	press, lathe := d[0], d[1]
	// snippet:begin
	return charts.Chart(
		charts.Scatter(press).Name("Press"),
		charts.Scatter(lathe).Name("Lathe").Size(6),
		charts.BandY(40, 55).Label("Normal"),
	).XTitle("Load %").YTitle("°C").Zoomable(false).Height(280).Width(600)
	// snippet:end
}
