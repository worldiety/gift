package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/icon/solid"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "icon", Group: "basic", Width: 560, Height: 280}, iconDemo)
}

func iconDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	caption := func(s string) gift.View {
		return ui.Text(s).FontSize(13).Foreground(ui.ColorSecondaryLabel).MinWidth(64)
	}

	return ui.VStack(
		ui.HStack(
			caption("Outline"),
			ui.Icon(outline.Home),
			ui.Icon(outline.Search),
			ui.Icon(outline.Bell),
			ui.Icon(outline.Envelope),
			ui.Icon(outline.Cog),
			ui.Icon(outline.User),
		).Gap(16).Align(geom.Center),
		ui.HStack(
			caption("Solid"),
			ui.Icon(solid.Heart).Foreground(ui.ColorDanger),
			ui.Icon(solid.Star).Foreground(ui.ColorAccent),
			ui.Icon(solid.CheckCircle).Foreground(ui.ColorAccent),
			ui.Icon(solid.Bell).Foreground(ui.ColorSecondaryLabel),
			ui.Icon(solid.Bell).
				Foreground(ui.ColorOnAccent).
				Padding(6).
				Background(ui.ColorAccent).
				CornerRadius(8),
		).Gap(16).Align(geom.Center),
		ui.HStack(
			caption("Sizes"),
			ui.Icon(outline.Star).Size(16),
			ui.Icon(outline.Star),
			ui.Icon(outline.Star).Size(24),
			ui.Icon(outline.Star).Size(32),
			ui.Icon(outline.Star).Size(40),
		).Gap(16).Align(geom.Center),
	).Gap(20)
	// snippet:end
}
