package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "modal", Group: "dialogs", Width: 560, Height: 360, FullWindow: true}, modalDemo)
}

func modalDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// The application owns the presentation. The demo starts with the
	// dialog open.
	open := ctx.State("open", true)

	screen := ui.VStack(
		ui.Text("Downloads").FontSize(22),
		ui.Card(
			ui.List(
				ui.Row("Report.pdf").Icon(outline.FilePdf).Value("2.4 MB"),
				ui.Row("Holiday.jpg").Icon(outline.Image).Value("5.1 MB"),
				ui.Row("Notes.txt").Icon(outline.File).Value("12 KB"),
			),
		).Padding(0),
		ui.Button(ui.Text("Clear downloads"), func() { open.Set(true) }).CornerRadius(10),
		ui.Spacer(),
	).Gap(16).Padding(24)

	// The modal is any view. Here it is a card of its own.
	dialog := ui.VStack(
		ui.Icon(outline.TrashBin).Size(28).Foreground(ui.ColorDanger),
		ui.Text("Clear downloads?").FontSize(17),
		ui.Text("The three files are moved to the trash.").
			FontSize(14).Foreground(ui.ColorSecondaryLabel),
		ui.HStack(
			ui.Button(ui.Text("Cancel"), func() { open.Set(false) }).
				CornerRadius(10).MinHeight(ui.ControlHitTarget).Flex(1),
			ui.Button(ui.Text("Clear").Foreground(ui.ColorDanger), func() { open.Set(false) }).
				CornerRadius(10).MinHeight(ui.ControlHitTarget).Flex(1),
		).Gap(8),
	).Gap(10).Align(geom.Center).Padding(20).
		Frame(300, geom.Unbounded()).
		Background(ui.ColorSurface).
		CornerRadius(14).
		Shadow(ui.Shadow{Blur: 24, OffsetY: 8, Color: ui.Fade(ui.ColorLabel, 0.25)})

	// Presented keeps the dialog mounted while it is closed, so it animates
	// out as well as in. A tap on the scrim closes it.
	return ui.Modal(screen, dialog).
		Presented(ctx.Read(open)).
		OnDismiss(func() { open.Set(false) })
	// snippet:end
}
