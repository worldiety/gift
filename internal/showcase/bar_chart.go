package showcase

import (
	"sync"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/ui/charts"
)

func init() {
	register(Demo{Name: "bar-chart", Group: "charts", Title: "Bar Chart",
		Summary: "Bars for categories or over time, with a rule for the average.",
		Width:   640, Height: 300}, barChartDemo)
}

var demoCoffee = sync.OnceValue(func() *charts.Data {
	return charts.XY([]float64{0, 1, 2, 3, 4, 5, 6}, []float64{142, 168, 155, 181, 204, 96, 71})
})

func barChartDemo(ctx *gift.Context) gift.View {
	cups := demoCoffee()
	// snippet:begin
	return charts.Chart(
		charts.Bar(cups).Name("Cups"),
		charts.RuleY(145).Label("Average").Dash(4, 3),
	).Categories("Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun").
		YTitle("Cups of coffee").Height(260).Width(600)
	// snippet:end
}
