package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/asset"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "image", Group: "basic", Width: 760, Height: 280, Settle: 30}, imageDemo)
}

func imageDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	wide := pictures[2] // a 360 x 200 picture
	well := ui.Fade(ui.ColorLabel, 0.06)

	caption := func(pic gift.View, s string) gift.View {
		return ui.VStack(
			pic,
			ui.Text(s).FontSize(13).Foreground(ui.ColorSecondaryLabel),
		).Gap(8).Align(geom.Alignment{X: 0.5})
	}

	return ui.HStack(
		caption(ui.Image(wide).Fit(ui.FitContain).Frame(112, 112).Background(well), "FitContain"),
		caption(ui.Image(wide).Fit(ui.FitCover).Frame(112, 112).Background(well), "FitCover"),
		caption(ui.Image(wide).Fit(ui.FitStretch).Frame(112, 112).Background(well), "FitStretch"),
		caption(ui.Image(pictures[1]).Fit(ui.FitCover).Frame(112, 112).
			CornerRadius(20).Clip(true), "Rounded"),
		caption(ui.Image(asset.File("missing.png")).Frame(112, 112).
			CornerRadius(20).Placeholder(well), "Placeholder"),
	).Gap(20)
	// snippet:end
}
