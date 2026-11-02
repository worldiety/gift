package showcase

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
	"github.com/worldiety/gift/ui/charts"
)

func init() {
	register(Demo{Name: "pie-chart", Group: "charts", Title: "Pie Chart",
		Summary: "Shares of a whole as the slices of a pie or a ring.",
		Width:   640, Height: 300}, pieChartDemo)
}

func pieChartDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	pie := charts.Chart(
		charts.Sector(58).Name("Coffee").Percent(true),
		charts.Sector(22).Name("Tea").Percent(true),
		charts.Sector(12).Name("Cocoa").Percent(true),
		charts.Sector(8).Name("Water").Percent(true),
	).Height(240).Width(290)
	ring := charts.Chart(
		charts.Sector(31).Name("Press").InnerRadius(0.62).AngularInset(2),
		charts.Sector(24).Name("Lathe").InnerRadius(0.62).AngularInset(2),
		charts.Sector(18).Name("Mill").InnerRadius(0.62).AngularInset(2),
		charts.Sector(9).Name("Light").InnerRadius(0.62).AngularInset(2),
	).Center(ui.VStack(
		ui.Text("82 kW").FontSize(20),
		ui.Text("now").FontSize(12).Foreground(ui.ColorSecondaryLabel),
	).Align(geom.Center)).Height(240).Width(290)
	return ui.HStack(pie, ring).Gap(20)
	// snippet:end
}
