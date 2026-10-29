package main

import (
	"fmt"

	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "slider", Group: "basic", Width: 560, Height: 300}, sliderDemo)
}

func sliderDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	volume := ctx.State("volume", 0.6)
	rating := ctx.State("rating", 3.0)
	heat := ctx.State("heat", 72.0)
	v, r, h := ctx.Read(volume), ctx.Read(rating), ctx.Read(heat)

	row := func(label string, slider ui.SliderView, value string) gift.View {
		return ui.HStack(
			ui.Text(label).MinWidth(90),
			slider.Frame(240, geom.Unbounded()),
			ui.Text(value).Foreground(ui.ColorSecondaryLabel).MinWidth(48),
		).Gap(16).Align(geom.Center)
	}

	return ui.VStack(
		row("Volume",
			ui.Slider(v, volume.Set).Label("Volume"),
			fmt.Sprintf("%.0f %%", v*100)),
		row("Rating",
			ui.Slider(r, rating.Set).Range(1, 5).Step(1).Label("Rating"),
			fmt.Sprintf("%.0f of 5", r)),
		row("Heat",
			ui.Slider(h, heat.Set).Range(0, 100).Tint(ui.ColorDanger).Label("Heat"),
			fmt.Sprintf("%.0f °C", h)),
		row("Disabled",
			ui.Slider(0.3, nil).Disabled(true),
			"30 %"),
	).Gap(8)
	// snippet:end
}
