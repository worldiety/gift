package showcase

import (
	"math"
	"sync"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/charts"
)

func init() {
	register(Demo{Name: "area-chart", Group: "charts", Title: "Area Chart",
		Summary: "The area under a series, faded out towards zero, and areas stacked into a whole.",
		Width:   640, Height: 560}, areaChartDemo)
}

var demoVisitors = sync.OnceValue(func() *charts.Data {
	var ts []time.Time
	var ys []float64
	for i := range 120 {
		ts = append(ts, chartDay.AddDate(0, 0, i-119))
		weekend := chartDay.AddDate(0, 0, i-119).Weekday()%6 == 0
		v := 820 + 4*float64(i) + 90*noise(i)
		if weekend {
			v += 260
		}
		ys = append(ys, v)
	}
	return charts.Times(ts, ys)
})

// demoEnergy is the power a hall draws from its roof, its turbine and the
// grid over a day, in kilowatts.
var demoEnergy = sync.OnceValue(func() [3]*charts.Data {
	var ts []time.Time
	var solar, wind, grid []float64
	for i := range 24 * 4 {
		h := float64(i) / 4
		ts = append(ts, chartDay.Add(time.Duration(i)*15*time.Minute))
		s := math.Max(0, 38*math.Sin((h-6.5)/13*math.Pi))
		w := 12 + 6*math.Sin(h/5) + 2*noise(i)
		load := 55 + 30*math.Exp(-(h-11)*(h-11)/18) + 3*noise(i+40)
		solar, wind = append(solar, s), append(wind, w)
		grid = append(grid, math.Max(0, load-s-w))
	}
	return [3]*charts.Data{charts.Times(ts, solar), charts.Times(ts, wind), charts.Times(ts, grid)}
})

func areaChartDemo(ctx *gift.Context) gift.View {
	visitors := demoVisitors()
	e := demoEnergy()
	solar, wind, grid := e[0], e[1], e[2]
	caption := func(s string) gift.View {
		return ui.Text(s).FontSize(13).Foreground(ui.ColorSecondaryLabel)
	}
	// snippet:begin
	return ui.VStack(
		caption("An area with its line, faded out"),
		charts.Chart(
			charts.Area(visitors).Gradient(true).Opacity(0.4),
			charts.Line(visitors).Width(1.5),
		).YTitle("Visitors a day").Location(chartZone).Height(220).Width(600),
		caption("Areas stacked into the power a hall draws"),
		charts.Chart(
			charts.Area(solar).Name("Solar"),
			charts.Area(wind).Name("Wind"),
			charts.Area(grid).Name("Grid"),
		).YTitle("kW").Location(chartZone).Height(240).Width(600),
	).Gap(10).Align(geom.Leading)
	// snippet:end
}
