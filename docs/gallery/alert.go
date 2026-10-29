package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "alert", Group: "dialogs", Width: 560, Height: 360, FullWindow: true}, alertDemo)
}

func alertDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	open := ctx.State("open", true)
	closeAlert := func() { open.Set(false) }

	// The screen behind the alert: an editor with unsaved changes.
	screen := ui.VStack(
		ui.HStack(
			ui.Icon(outline.FileChartBar).Size(22).Foreground(ui.ColorAccent),
			ui.Text("Quarterly report").FontSize(22),
			ui.Spacer(),
			ui.Text("Edited").Foreground(ui.ColorSecondaryLabel),
		).Gap(10).Align(geom.Center),
		ui.HStack(
			ui.Card(
				ui.Text("Revenue grew"),
				ui.Text("In all three regions").Foreground(ui.ColorSecondaryLabel),
			).Header("Summary").Flex(1),
			ui.Card(
				ui.Text("October figures"),
				ui.Text("Due on Friday").Foreground(ui.ColorSecondaryLabel),
			).Header("Open items").Flex(1),
		).Gap(16),
		ui.Spacer(),
	).Gap(16).Padding(24)

	// One button of each role: the cancel choice in the label color, the
	// destructive one in red and the ordinary action in the accent color.
	alert := ui.Alert(
		"Save changes?",
		"Your edits to this report are lost if you close it without saving.",
		ui.AlertDestructive("Discard", closeAlert),
		ui.AlertCancel("Cancel", closeAlert),
		ui.AlertAction("Save", closeAlert),
	)

	// Modal presents the alert over the screen: it dims the screen and
	// blocks its input. Presented keeps the alert mounted, so it animates
	// both in and out.
	return ui.Modal(screen, alert).Presented(ctx.Read(open))
	// snippet:end
}
