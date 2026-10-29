package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "badge", Group: "basic", Width: 480, Height: 320}, badgeDemo)
}

func badgeDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	return ui.VStack(
		ui.HStack(
			ui.Badge("3"),
			ui.Badge("42"),
			ui.Badge("128"),
			ui.Badge("New"),
			ui.Badge("Offline").Color(ui.ColorDanger),
			ui.Badge("Beta").Color(ui.ColorControl).Foreground(ui.ColorLabel),
		).Gap(8).Align(geom.Center).PaddingInsets(geom.Insets{Left: 16, Right: 16}),
		ui.List(
			ui.Row("Inbox").Accessory(ui.Badge("12")).Key("inbox"),
			ui.Row("Updates").Subtitle("Two need a restart").Accessory(ui.Badge("2")).Key("updates"),
			ui.Row("Printer").Accessory(ui.Badge("Jammed").Color(ui.ColorDanger)).Key("printer"),
		).Frame(360, geom.Unbounded()),
	).Gap(20)
	// snippet:end
}
