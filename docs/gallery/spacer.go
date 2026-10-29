package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "spacer", Group: "layout", Width: 520, Height: 480}, spacerDemo)
}

func spacerDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	section := func(title string, content gift.View) gift.View {
		return ui.VStack(
			ui.Text(title).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			content,
		).Gap(6).Align(geom.Leading)
	}
	marker := func(s string) gift.View {
		return ui.Text(s).Foreground(ui.ColorOnAccent).
			PaddingInsets(geom.Insets{Top: 4, Right: 10, Bottom: 4, Left: 10}).
			Background(ui.ColorAccent).CornerRadius(8)
	}

	// One spacer pushes the actions to the trailing edge.
	toolbar := ui.HStack(
		ui.Icon(outline.ChevronLeft).Size(20),
		ui.Text("Inbox"),
		ui.Spacer(),
		ui.Icon(outline.Search).Size(20),
		ui.Icon(outline.Cog).Size(20),
	).Gap(12).Align(geom.Center).Padding(12).
		Background(ui.ColorSurface).CornerRadius(12)

	// Flex splits the free space: one part before B, three parts after it.
	ratio := ui.HStack(
		marker("A"),
		ui.Spacer().Flex(1),
		marker("B"),
		ui.Spacer().Flex(3),
		marker("C"),
	).Align(geom.Center).Padding(12).
		Background(ui.ColorSurface).CornerRadius(12)

	// In a VStack with a fixed height, a spacer moves the footer down.
	card := ui.VStack(
		ui.Text("Backup finished"),
		ui.Text("1,284 files were copied.").Foreground(ui.ColorSecondaryLabel),
		ui.Spacer(),
		ui.HStack(
			ui.Spacer(),
			ui.Text("Show files").Foreground(ui.ColorAccent),
		),
	).Gap(4).Align(geom.Leading).Padding(16).Frame(geom.Unbounded(), 140).
		Background(ui.ColorSurface).CornerRadius(12)

	return ui.VStack(
		section("Push to the edge", toolbar),
		section("Split by flex 1 : 3", ratio),
		section("Push to the bottom", card),
	).Gap(16).Align(geom.Leading).Frame(360, geom.Unbounded())
	// snippet:end
}
