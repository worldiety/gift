package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "segmented-control", Group: "basic", Width: 520, Height: 360}, segmentedControlDemo)
}

func segmentedControlDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	period := ctx.State("period", 1)
	view := ctx.State("view", 0)

	labeled := func(caption string, control gift.View) gift.View {
		return ui.VStack(
			ui.Text(caption).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			control,
		).Gap(6).Align(geom.Alignment{X: 0})
	}

	return ui.VStack(
		labeled("Default", ui.SegmentedControl(ctx.Read(period), []string{"Day", "Week", "Month", "Year"}, period.Set).
			Label("Period")),
		labeled("Capsule", ui.SegmentedControl(ctx.Read(view), []string{"List", "Grid", "Map"}, view.Set).
			Capsule(true).
			Label("View")),
		labeled("Disabled", ui.SegmentedControl(0, []string{"Low", "Medium", "High"}, nil).
			Disabled(true)),
	).Gap(18).Frame(360, geom.Unbounded())
	// snippet:end
}
