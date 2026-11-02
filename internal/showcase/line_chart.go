package showcase

import (
	"math"
	"sync"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui/charts"
)

func init() {
	register(Demo{Name: "line-chart", Group: "charts", Title: "Line Chart",
		Summary: "Series over time as lines, broken where values are missing, with a limit.",
		Width:   640, Height: 320}, lineChartDemo)
}

var demoTemperatures = sync.OnceValue(func() [2]*charts.Data {
	var ts []time.Time
	var hall, yard []float64
	for i := range 24 * 12 {
		t := chartDay.Add(time.Duration(i) * 5 * time.Minute)
		h := float64(i) / 12
		ts = append(ts, t)
		hall = append(hall, 20.5+2.6*math.Sin((h-9)/24*2*math.Pi)+0.25*noise(i))
		y := 9 + 4.5*math.Sin((h-10)/24*2*math.Pi) + 0.4*noise(i+100)
		// The sensor in the yard was off for an hour and a half.
		if h >= 13 && h < 14.5 {
			y = math.NaN()
		}
		yard = append(yard, y)
	}
	return [2]*charts.Data{charts.Times(ts, hall), charts.Times(ts, yard)}
})

func lineChartDemo(ctx *gift.Context) gift.View {
	d := demoTemperatures()
	hall, yard := d[0], d[1]
	// snippet:begin
	return charts.Chart(
		charts.Line(hall).Name("Hall 2"),
		charts.Line(yard).Name("Yard").Interpolation(charts.Monotone),
		charts.RuleY(23).Label("Limit 23 °C").Dash(4, 3),
	).YTitle("°C").Location(chartZone).Height(280).Width(600)
	// snippet:end
}
