package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/font/inter"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	// The input scrolls the strip by one and a half wheel notches and holds a drag on the
	// list, so the screenshot shows both viewports moved and the list's bar.
	register(Demo{Name: "scroll-view", Group: "layout", Width: 720, Height: 420, Input: `{"steps":[
		{"op":"waitFrames","frames":4},
		{"op":"wheel","x":500,"y":210,"dx":-1.5},
		{"op":"waitFrames","frames":2},
		{"op":"pointerDown","x":198,"y":300},
		{"op":"waitFrames","frames":2},
		{"op":"pointerMove","x":198,"y":260},
		{"op":"waitFrames","frames":2},
		{"op":"pointerMove","x":198,"y":200},
		{"op":"waitFrames","frames":10}
	]}`}, scrollViewDemo)
}

func scrollViewDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	bold := ui.MustFont(ui.FontQuery{Family: inter.Family, Weight: ui.WeightBold})
	people := []string{"Ada", "Brian", "Chen", "Dana", "Emil", "Farah", "Greta", "Hugo", "Ines", "Jonas"}

	// A vertical list. The Frame bounds the viewport; without a bound on
	// the scroll axis the view grows with its content and nothing scrolls.
	rows := make([]gift.View, 0, len(people))
	for _, name := range people {
		rows = append(rows, ui.HStack(
			ui.ZStack(
				ui.Box().Background(ui.Fade(ui.ColorAccent, 0.25)).CornerRadius(16),
				ui.Text(name[:1]).Font(bold).Foreground(ui.ColorAccent),
			).Align(geom.Center).Frame(32, 32),
			ui.VStack(
				ui.Text(name).Font(bold),
				ui.Text("Sent you a message").FontSize(13).Foreground(ui.ColorSecondaryLabel),
			).Gap(2),
		).Gap(12).Align(geom.Center))
	}
	inbox := ui.VScroll(rows...).
		Gap(12).Padding(12).
		Frame(260, 280).
		Background(ui.ColorSurface).CornerRadius(12).
		Border(ui.Border{Width: 1, Color: ui.ColorSeparator}).
		// The bar is drawn by the container; this one is thinner, inset
		// from the edge and in the accent color.
		ScrollBar(ui.ScrollBar{Width: 6, Margin: 4, Thumb: ui.ColorAccent})

	// A horizontal strip. MaxWidth bounds the viewport along X; the cut
	// tiles at both ends tell the user there is more, so it has no bar.
	tiles := make([]gift.View, 0, 8)
	for i := range 8 {
		tiles = append(tiles, ui.ZStack(
			ui.Box().Background(ui.Fade(ui.ColorAccent, 0.6+0.05*float32(i))).CornerRadius(10),
			ui.Text(people[i]).Foreground(ui.ColorOnAccent).Padding(8),
		).Align(geom.BottomLeading).Frame(96, 72))
	}
	strip := ui.HScroll(tiles...).Gap(8).MaxWidth(300).
		ScrollBar(ui.ScrollBar{Hidden: true})

	return ui.HStack(
		inbox,
		ui.VStack(
			ui.Text("Albums").Font(bold),
			strip,
			ui.Text("Drag, fling or use the wheel.").FontSize(13).Foreground(ui.ColorSecondaryLabel),
		).Gap(10),
	).Gap(24).Align(geom.Center)
	// snippet:end
}
