package showcase

import (
	"sync"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/charts"
)

func init() {
	register(Demo{Name: "bar-chart", Group: "charts", Title: "Bar Chart",
		Summary: "Bars for categories or over time, stacked or side by side, with their values.",
		Width:   640, Height: 560}, barChartDemo)
}

// demoDrinks are the cups of coffee, tea and cocoa sold on each day of a
// week.
var demoDrinks = sync.OnceValue(func() [3]*charts.Data {
	days := []float64{0, 1, 2, 3, 4, 5, 6}
	return [3]*charts.Data{
		charts.XY(days, []float64{98, 112, 104, 121, 138, 61, 44}),
		charts.XY(days, []float64{31, 38, 35, 40, 42, 22, 18}),
		charts.XY(days, []float64{13, 18, 16, 20, 24, 13, 9}),
	}
})

func barChartDemo(ctx *gift.Context) gift.View {
	d := demoDrinks()
	coffee, tea, cocoa := d[0], d[1], d[2]
	week := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	caption := func(s string) gift.View {
		return ui.Text(s).FontSize(13).Foreground(ui.ColorSecondaryLabel)
	}
	// snippet:begin
	return ui.VStack(
		caption("Stacked, as Swift Charts stacks by default"),
		charts.Chart(
			charts.Bar(coffee).Name("Coffee").ValueLabels(true),
			charts.Bar(tea).Name("Tea").ValueLabels(true),
			charts.Bar(cocoa).Name("Cocoa"),
			charts.RuleY(145).Label("Average").Dash(4, 3),
		).Categories(week...).YTitle("Cups").Height(240).Width(600),
		caption("Grouped"),
		charts.Chart(
			charts.Bar(coffee).Name("Coffee"),
			charts.Bar(tea).Name("Tea"),
			charts.Bar(cocoa).Name("Cocoa"),
		).Categories(week...).Stacking(charts.Grouped).Height(220).Width(600),
	).Gap(10).Align(geom.Leading)
	// snippet:end
}
