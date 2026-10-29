package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "sheet", Group: "dialogs", Width: 560, Height: 520, FullWindow: true}, sheetDemo)
}

func sheetDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// The application owns whether the sheet is up. The demo starts with it
	// presented, so the screenshot shows the settled sheet.
	open := ctx.State("open", true)
	notify := ctx.State("notify", true)

	// The screen underneath. While the sheet is up it is dimmed by the
	// scrim and takes no input.
	screen := ui.VStack(
		ui.HStack(
			ui.Text("Orders").FontSize(22),
			ui.Spacer(),
			ui.Button(ui.Text("Filter"), func() { open.Set(true) }).CornerRadius(8),
		).Align(geom.Center),
		ui.Card(
			ui.List(
				ui.Row("Order 1042").Subtitle("Ready for pickup").Value("$18.40"),
				ui.Row("Order 1041").Subtitle("In the oven").Value("$32.10"),
				ui.Row("Order 1040").Subtitle("Delivered").Value("$9.90"),
			),
		).Padding(0),
	).Gap(16).Padding(24)

	// The sheet: a title bar with a button that closes it, and its content.
	// The title bar is inset like the rows, so the two line up.
	filter := ui.VStack(
		ui.HStack(
			ui.Text("Filter").FontSize(20),
			ui.Spacer(),
			ui.Button(ui.Text("Done"), func() { open.Set(false) }).
				CornerRadius(8).MinHeight(ui.ControlHitTarget),
		).Align(geom.Center).PaddingInsets(geom.Insets{Left: ui.RowPadding, Right: ui.RowPadding}),
		ui.List(
			ui.Row("Status").Icon(outline.Filter).Value("Open orders").
				Chevron(outline.AngleRight).OnTap(func() {}),
			ui.Row("Period").Icon(outline.CalendarWeek).Value("This week").
				Chevron(outline.AngleRight).OnTap(func() {}),
			ui.Row("Notify me").Icon(outline.Bell).
				Accessory(ui.Toggle(ctx.Read(notify), notify.Set).Label("Notify me")),
		),
	).Gap(12).Padding(16)

	// Presented keeps the sheet mounted while it is down, so it slides out
	// instead of vanishing. A tap on the scrim or escape calls OnDismiss.
	return ui.Sheet(screen, filter).
		Height(0.5).
		Presented(ctx.Read(open)).
		OnDismiss(func() { open.Set(false) })
	// snippet:end
}
