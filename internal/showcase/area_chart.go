package showcase

import (
	"sync"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui/charts"
)

func init() {
	register(Demo{Name: "area-chart", Group: "charts", Title: "Area Chart",
		Summary: "The area under a series, faded out towards zero, with its line on top.",
		Width:   640, Height: 300}, areaChartDemo)
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

func areaChartDemo(ctx *gift.Context) gift.View {
	visitors := demoVisitors()
	// snippet:begin
	return charts.Chart(
		charts.Area(visitors).Gradient(true).Opacity(0.4),
		charts.Line(visitors).Width(1.5),
	).YTitle("Visitors a day").Location(chartZone).Height(260).Width(600)
	// snippet:end
}
