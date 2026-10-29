package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "list", Group: "lists", Width: 820, Height: 520}, listDemo)
}

func listDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	notify := ctx.State("notify", true)

	// A grouped settings list: section headers, rows with icons, a switch
	// and a chevron. The separators start at the text, after the icon.
	settings := ui.Card(
		ui.List(
			ui.Section("General"),
			ui.Row("Notifications").Icon(outline.Bell).
				Accessory(ui.Toggle(ctx.Read(notify), notify.Set)),
			ui.Row("Brightness").Icon(outline.Sun).Value("60 %"),
			ui.Row("Language").Icon(outline.Language).Value("English").
				Chevron(outline.AngleRight).OnTap(func() {}),
			ui.Section("About"),
			ui.Row("Version").Icon(outline.InfoCircle).Value("1.4.0"),
			ui.Row("Updates").Icon(outline.Lock).
				Subtitle("Managed by your administrator").Disabled(true),
		).SeparatorInsets(ui.RowPadding+22+12, 0).Frame(340, geom.Unbounded()),
	).Padding(0).Header("Settings")

	// Rows without icons and full-width separators read like a table.
	printers := ui.Card(
		ui.List(
			ui.Row("Front desk").Subtitle("Ready").Value("A4"),
			ui.Row("Warehouse").Subtitle("Out of paper").Value("Letter"),
			ui.Row("Label printer").Subtitle("Offline").Value("62 mm"),
		).SeparatorInsets(0, 0).Frame(300, geom.Unbounded()),
	).Padding(0).Header("Printers")

	return ui.HStack(
		settings,
		printers,
	).Gap(24)
	// snippet:end
}
