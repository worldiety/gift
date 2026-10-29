package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "shadow-and-border", Group: "styling", Width: 1040, Height: 260, Settle: 30}, shadowAndBorderDemo)
}

func shadowAndBorderDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// A tile is a surface with a title and a caption. The modifiers that
	// differ per tile are applied by the caller.
	tile := func(title, caption string) ui.Stack {
		return ui.VStack(
			ui.Text(title),
			ui.Text(caption).FontSize(12).Foreground(ui.ColorSecondaryLabel),
		).Gap(4).Padding(16).Frame(160, 96).
			Background(ui.ColorSurface).CornerRadius(12)
	}

	// A hairline one logical pixel wide in the separator color, like a card.
	hairline := tile("Hairline", "Width 1, separator").
		Border(ui.Border{Width: 1, Color: ui.ColorSeparator})

	// A thicker border in the accent color. It lies inside the bounds, so
	// the tile keeps its size.
	accent := tile("Accent", "Width 2, accent").
		Border(ui.Border{Width: 2, Color: ui.ColorAccent})

	// A soft shadow below the tile. It does not change the layout.
	soft := tile("Soft shadow", "Blur 16, offset 4").
		Shadow(ui.Shadow{Blur: 16, OffsetY: 4, Color: ui.RGBA(0, 0, 0, 60)})

	// A negative spread tucks the shadow under the tile, so only the
	// offset side shows.
	lifted := tile("Lifted", "Spread -6, offset 12").
		Shadow(ui.Shadow{Blur: 20, Spread: -6, OffsetY: 12, Color: ui.RGBA(0, 0, 0, 90)})

	// A picture clipped to its rounded corners, with a border drawn on top.
	// The caption is a second layer of a ZStack.
	picture := ui.ZStack(
		ui.Image(pictures[0]).Fit(ui.FitCover).Frame(160, 96).
			CornerRadius(12).Clip(true).
			Border(ui.Border{Width: 2, Color: ui.ColorSurface}).
			Shadow(ui.Shadow{Blur: 16, OffsetY: 4, Color: ui.RGBA(0, 0, 0, 60)}),
		ui.VStack(
			ui.Text("Clipped picture").Foreground(ui.RGB(255, 255, 255)),
			ui.Text("Radius 12, border 2").FontSize(12).Foreground(ui.RGBA(255, 255, 255, 210)),
		).Gap(4).Padding(16),
	)

	return ui.HStack(hairline, accent, soft, lifted, picture).
		Gap(24).Align(geom.Alignment{Y: 0.5})
	// snippet:end
}
