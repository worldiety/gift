package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "gradient", Group: "styling", Width: 840, Height: 300}, gradientDemo)
}

func gradientDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// An accent plate whose color thins out towards the bottom.
	accent := ui.VStack(
		ui.Icon(outline.ChartLineUp).Size(22).Foreground(ui.ColorOnAccent),
		ui.Text("Weekly report").FontSize(15).Foreground(ui.ColorOnAccent),
		ui.Text("12 new entries").FontSize(13).Foreground(ui.ColorOnAccent),
	).Gap(6).Padding(16).
		Background(ui.LinearGradient(ui.ColorAccent, ui.Fade(ui.ColorAccent, 0.75))).
		CornerRadius(14).Frame(160, 120)

	// A quiet sheen: from the surface color to the control face.
	sheen := ui.VStack(
		ui.Text("Storage").FontSize(15),
		ui.Text("38 GB free").FontSize(13).Foreground(ui.ColorSecondaryLabel),
	).Gap(6).Padding(16).
		Background(ui.LinearGradient(ui.ColorSurface, ui.ColorControl)).
		Border(ui.Border{Width: 1, Color: ui.ColorSeparator}).
		CornerRadius(14).Frame(160, 120)

	// A gradient from a transparent color fades out what is under it. The
	// overlay repeats the corner radius of the card it lies on.
	fade := ui.ZStack(
		ui.VStack(
			ui.Text("Release notes"),
			ui.Text("Faster startup"),
			ui.Text("New file picker"),
			ui.Text("Fewer redraws"),
		).Gap(4).Padding(16),
		ui.Box().
			Background(ui.LinearGradient(ui.Fade(ui.ColorSurface, 0), ui.ColorSurface)).
			CornerRadius(14),
	).Background(ui.ColorSurface).
		Border(ui.Border{Width: 1, Color: ui.ColorSeparator}).
		CornerRadius(14).Frame(160, 120)

	// A tinted glow that fades from a translucent color to nothing.
	glow := ui.VStack(
		ui.Icon(outline.Bell).Size(22).Foreground(ui.ColorDanger),
		ui.Text("3 alerts").FontSize(15),
	).Gap(6).Padding(16).
		Background(ui.LinearGradient(ui.Fade(ui.ColorDanger, 0.3), ui.Fade(ui.ColorDanger, 0))).
		CornerRadius(14).Frame(160, 120)

	return ui.HStack(
		gradientTile(accent, "Accent"),
		gradientTile(sheen, "Sheen"),
		gradientTile(fade, "Fade out"),
		gradientTile(glow, "Glow"),
	).Gap(20)
	// snippet:end
}

// gradientTile puts a caption under one variant.
func gradientTile(v gift.View, title string) gift.View {
	return ui.VStack(
		v,
		ui.Text(title).FontSize(13).Foreground(ui.ColorSecondaryLabel),
	).Gap(8).Align(geom.Center)
}
