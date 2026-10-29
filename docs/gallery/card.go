package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "card", Group: "layout", Width: 760, Height: 420}, cardDemo)
}

func cardDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	volume := ctx.State("volume", 0.6)
	sync := ctx.State("sync", true)

	// A plain card: no header, the default padding. A card is as wide as its
	// content; the Spacer makes the row, and so the card, fill the column.
	note := ui.Card(
		ui.HStack(
			ui.Icon(outline.CheckCircle).Size(22).Foreground(ui.ColorAccent),
			ui.VStack(
				ui.Text("Backup finished"),
				ui.Text("1,284 files copied to the external drive").
					FontSize(13).Foreground(ui.ColorSecondaryLabel),
			).Gap(2),
			ui.Spacer(),
		).Gap(12).Align(geom.Center),
	)

	// A header captions the content. The children stack vertically.
	account := ui.Card(
		ui.Text("Signed in as kiosk@example.com"),
		ui.HStack(
			ui.Text("Sync settings"),
			ui.Spacer(),
			ui.Toggle(ctx.Read(sync), sync.Set).Label("Sync settings"),
		).Align(geom.Center),
		ui.HStack(
			ui.Icon(outline.VolumeDown).Size(18).Foreground(ui.ColorSecondaryLabel),
			ui.Slider(ctx.Read(volume), volume.Set).Label("Volume").Flex(1),
			ui.Icon(outline.VolumeUp).Size(18).Foreground(ui.ColorSecondaryLabel),
		).Gap(10).Align(geom.Center),
	).Header("Account")

	// A list brings its own row padding, so the card's content padding is
	// zero. The header keeps its inset.
	display := ui.Card(
		ui.List(
			ui.Row("Brightness").Icon(outline.Sun).Value("80 %"),
			ui.Row("Night shift").Icon(outline.Moon).Value("Off"),
			ui.Row("Text size").Icon(outline.TextSize).Value("Default"),
			ui.Row("Screen lock").Icon(outline.LockTime).Value("5 min"),
			ui.Row("Wallpaper").Icon(outline.Image).Chevron(outline.AngleRight).
				OnTap(func() {}),
		).SeparatorInsets(ui.RowPadding+22+12, 0),
	).Padding(0).Header("Display")

	return ui.HStack(
		ui.VStack(note, account).Gap(16).Flex(1),
		ui.VStack(display).Flex(1),
	).Gap(16).MinWidth(680)
	// snippet:end
}
