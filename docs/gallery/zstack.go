package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "zstack", Group: "layout", Width: 720, Height: 320}, zstackDemo)
}

func zstackDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// A plate behind content. The unframed Box is greedy and fills the
	// whole box; the alignment centers the text on it.
	plate := ui.ZStack(
		ui.Box().Background(ui.ColorAccent).CornerRadius(14),
		ui.Text("Centered").Foreground(ui.ColorOnAccent),
	).Align(geom.Center).Frame(150, 110)

	// A caption over a picture, here a gradient plate, in the bottom
	// leading corner. The padding of the text column keeps it off the edge.
	caption := ui.ZStack(
		ui.Box().
			Background(ui.LinearGradient(ui.Fade(ui.ColorAccent, 0.3), ui.ColorAccent)).
			CornerRadius(14),
		ui.VStack(
			ui.Text("Harbor").FontSize(17).Foreground(ui.ColorOnAccent),
			ui.Text("12 photos").FontSize(13).Foreground(ui.ColorOnAccent),
		).Gap(2).Padding(12),
	).Align(geom.BottomLeading).Frame(150, 110)

	// A badge on the top trailing corner of an icon tile. Without a frame,
	// the overlay is as large as its largest child, the padded tile.
	tile := ui.ZStack(
		ui.Box().Background(ui.ColorControl).CornerRadius(16),
		ui.Icon(outline.Bell).Size(32),
	).Align(geom.Center).Frame(80, 80)
	badge := ui.ZStack(
		ui.VStack(tile).Padding(8),
		ui.Badge("3").Color(ui.ColorDanger),
	).Align(geom.TopTrailing)

	return ui.HStack(
		zstackTile(plate, "Plate"),
		zstackTile(caption, "Caption"),
		zstackTile(ui.VStack(badge).Align(geom.Center).Frame(150, 110), "Badge"),
	).Gap(20)
	// snippet:end
}

// zstackTile puts a caption under one variant.
func zstackTile(v gift.View, title string) gift.View {
	return ui.VStack(
		v,
		ui.Text(title).FontSize(13).Foreground(ui.ColorSecondaryLabel),
	).Gap(8).Align(geom.Center)
}
