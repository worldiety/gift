package showcase

import (
	"context"
	"iter"
	"math"
	"sync"
	"time"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/charts"
)

func init() {
	register(Demo{Name: "chart-zoom", Group: "charts", Title: "Chart Zoom",
		Summary: "A million points in memory and a year of seconds from a source, zoomed into.",
		Width:   640, Height: 560, Settle: 30}, chartZoomDemo)
}

// demoSignal is a million points of a vibration sensor, with a gap.
var demoSignal = sync.OnceValue(func() *charts.Data {
	const n = 1_000_000
	xs, ys := make([]float64, n), make([]float64, n)
	start := charts.Seconds(chartDay)
	for i := range n {
		xs[i] = start + float64(i)*0.05
		t := float64(i) / n
		y := math.Sin(float64(i)*0.013)*(1+2*t) + 0.6*noise(i)
		if i%97_003 == 0 {
			y += 6 // a knock
		}
		if i > 610_000 && i < 640_000 {
			y = math.NaN()
		}
		ys[i] = y
	}
	d := charts.XY(xs, ys)
	return d
})

// demoYear is a source of a year of one value a second, 31 million points
// that exist nowhere: it computes them at the resolution a chart asks for,
// as a database that runs M4 next to its data answers.
var demoYear = sync.OnceValue(func() *charts.Source {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, chartZone)
	lo, hi := charts.Seconds(from), charts.Seconds(from.AddDate(1, 0, 0))
	value := func(x float64) float64 {
		day := (x - lo) / 86400
		return 15 + 10*math.Sin((day-110)/365*2*math.Pi) + 4*math.Sin(day*2*math.Pi) + 0.8*noise(int(x))
	}
	return charts.NewSource(charts.SourceConfig{
		Time:   true,
		Extent: func(context.Context) (float64, float64, error) { return lo, hi, nil },
		Points: func(ctx context.Context, r charts.Request) (iter.Seq[charts.Point], error) {
			step := max(1, math.Floor((r.To-r.From)/float64(r.Buckets*4)))
			return func(yield func(charts.Point) bool) {
				for x := math.Ceil(r.From/step) * step; x <= r.To; x += step {
					if !yield(charts.Point{X: x, Y: value(x)}) {
						return
					}
				}
			}, nil
		},
	})
})

func chartZoomDemo(ctx *gift.Context) gift.View {
	signal, year := demoSignal(), demoYear()
	caption := func(s string) gift.View {
		return ui.Text(s).FontSize(13).Foreground(ui.ColorSecondaryLabel)
	}
	// snippet:begin
	return ui.VStack(
		caption("A million points in memory: pinch, double tap or drag a range"),
		charts.Chart(charts.Line(signal).Width(1)).TimeAxis().Location(chartZone).Height(220).Width(600),
		caption("A year of one value a second, fetched as it is zoomed into"),
		charts.Chart(charts.Line(year).Width(1).Name("Outside")).YTitle("°C").Location(chartZone).Height(220).Width(600),
	).Gap(10).Align(geom.Leading)
	// snippet:end
}
