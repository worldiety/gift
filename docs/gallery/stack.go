package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "stack", Group: "layout", Width: 640, Height: 520}, stackDemo)
}

func stackDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	bold := ui.MustFont(ui.FontQuery{Family: inter.Family, Weight: ui.WeightBold})

	// A toolbar: the Spacer takes the free width and pushes the button to
	// the trailing edge.
	toolbar := ui.HStack(
		ui.Icon(outline.Inbox).Size(20),
		ui.Text("Inbox").Font(bold),
		ui.Spacer(),
		ui.Button(ui.Text("Compose").Foreground(ui.ColorOnAccent), func() {}).
			Background(ui.ColorAccent).CornerRadius(8),
	).Gap(10).Align(geom.Center).
		Padding(12).Background(ui.ColorSurface).CornerRadius(12).
		Border(ui.Border{Width: 1, Color: ui.ColorSeparator})

	// Align moves the children of a VStack along X.
	bar := func(w float32) gift.View {
		return ui.Box().Frame(w, 10).Background(ui.ColorAccent).CornerRadius(5)
	}
	aligned := func(name string, a geom.Alignment) ui.Stack {
		return ui.VStack(
			ui.Text(name).FontSize(13).Foreground(ui.ColorSecondaryLabel),
			bar(90), bar(50), bar(120),
		).Gap(6).Align(a).Padding(12).Background(ui.ColorControl).CornerRadius(10)
	}

	// Flex splits the free width of the row: one, two and one share.
	share := func(label string, flex float32) ui.Stack {
		return ui.VStack(ui.Text(label).Foreground(ui.ColorOnAccent)).
			Align(geom.Center).Padding(10).
			Background(ui.Fade(ui.ColorAccent, 0.4+0.3*flex)).CornerRadius(8).
			Flex(flex)
	}

	// AlignBaseline puts labels of different sizes on one line.
	reading := ui.HStack(
		ui.Text("21").Font(bold).FontSize(44),
		ui.Text("°C").FontSize(20),
		ui.Text("Living room").Foreground(ui.ColorSecondaryLabel),
	).Gap(6).AlignBaseline()

	return ui.VStack(
		toolbar,
		ui.HStack(
			aligned("Leading", geom.Leading).Flex(1),
			aligned("Center", geom.Center).Flex(1),
			aligned("Trailing", geom.Trailing).Flex(1),
		).Gap(8),
		ui.HStack(share("Flex 1", 1), share("Flex 2", 2), share("Flex 1", 1)).Gap(8),
		reading,
	).Gap(16).Frame(500, geom.Unbounded())
	// snippet:end
}
