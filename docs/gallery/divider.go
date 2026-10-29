package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "divider", Group: "basic", Width: 480, Height: 360}, dividerDemo)
}

func dividerDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	row := func(title, value string) gift.View {
		return ui.HStack(
			ui.Text(title),
			ui.Spacer(),
			ui.Text(value).Foreground(ui.ColorSecondaryLabel),
		).PaddingInsets(geom.Insets{Top: 12, Right: 16, Bottom: 12, Left: 16})
	}

	return ui.VStack(
		// Inset dividers line up with the text of the rows.
		ui.VStack(
			row("Display", "Automatic"),
			ui.Divider().Inset(16, 0),
			row("Brightness", "80%"),
			ui.Divider().Inset(16, 0),
			row("Sleep", "After 5 minutes"),
		).Background(ui.ColorSurface).CornerRadius(12),

		// In an HStack a divider needs a flex to get a width.
		ui.HStack(
			ui.Divider().Flex(1),
			ui.Text("or").Foreground(ui.ColorSecondaryLabel),
			ui.Divider().Flex(1),
		).Gap(12).Align(geom.Center),

		// A thicker line in the accent color.
		ui.Divider().Thickness(3).Color(ui.ColorAccent),
	).Gap(24).Frame(320, geom.Unbounded())
	// snippet:end
}
