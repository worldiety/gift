package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "box", Group: "layout", Width: 720, Height: 360}, boxDemo)
}

func boxDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	tile := func(caption string, box ui.BoxView) gift.View {
		return ui.VStack(
			box.Frame(112, 80),
			ui.Text(caption).FontSize(13).Foreground(ui.ColorSecondaryLabel),
		).Gap(10).Align(geom.Center)
	}

	return ui.VStack(
		ui.HStack(
			// A plain fill in a semantic color.
			tile("Fill", ui.Box().Background(ui.ColorAccent).CornerRadius(12)),
			// A vertical gradient, from the top to the bottom.
			tile("Gradient", ui.Box().
				Background(ui.LinearGradient(ui.ColorAccent, ui.Fade(ui.ColorAccent, 0.15))).
				CornerRadius(12)),
			// A border is stroked inside the bounds.
			tile("Border", ui.Box().
				Background(ui.ColorSurface).
				Border(ui.Border{Width: 2, Color: ui.ColorSeparator}).
				CornerRadius(12)),
			// A shadow is painted outside the bounds and takes no space.
			tile("Shadow", ui.Box().
				Background(ui.ColorSurface).
				Shadow(ui.Shadow{Blur: 20, OffsetY: 6, Color: ui.RGBA(0, 0, 0, 170)}).
				CornerRadius(12)),
			// Half the height as radius makes a capsule.
			tile("Capsule", ui.Box().Background(ui.Fade(ui.ColorAccent, 0.35)).CornerRadius(40)),
		).Gap(28),

		// Flex shares a stack's leftover width between boxes.
		ui.HStack(
			ui.Box().Background(ui.ColorAccent).Flex(3),
			ui.Box().Background(ui.Fade(ui.ColorAccent, 0.55)).Flex(2),
			ui.Box().Background(ui.Fade(ui.ColorAccent, 0.25)).Flex(1),
			ui.Box().Background(ui.ColorControl).Flex(2),
		).Gap(3).Frame(geom.Unbounded(), 12),
	).Gap(28)
	// snippet:end
}
