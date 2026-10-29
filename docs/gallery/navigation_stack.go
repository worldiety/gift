package main

import (
	"github.com/worldiety/gift"
	"github.com/worldiety/gift/geom"
	"github.com/worldiety/gift/icon/outline"
	"github.com/worldiety/gift/ui"
)

func init() {
	register(Demo{Name: "navigation-stack", Group: "navigation", Width: 560, Height: 330, FullWindow: true},
		navigationStackDemo)
}

func navigationStackDemo(ctx *gift.Context) gift.View {
	// snippet:begin
	// The application owns the navigation state. Here it is the title of the
	// pushed screen, or "" on the root screen. The demo starts pushed.
	open := ctx.State("open", "Display")
	nightShift := ctx.State("nightShift", true)

	push := func(title string) func() { return func() { open.Set(title) } }

	settings := ui.VScroll(
		ui.Card(
			ui.List(
				ui.Row("Display").Icon(outline.Sun).Chevron(outline.AngleRight).
					OnTap(push("Display")),
				ui.Row("Notifications").Icon(outline.Bell).Chevron(outline.AngleRight).
					OnTap(push("Notifications")),
				ui.Row("Privacy").Icon(outline.Lock).Chevron(outline.AngleRight).
					OnTap(push("Privacy")),
			),
		).Padding(0),
	).Padding(16).Flex(1)

	display := ui.VScroll(
		ui.Card(
			ui.List(
				ui.Row("Brightness").Icon(outline.Sun).Value("80 %"),
				ui.Row("Night shift").Icon(outline.Moon).Accessory(
					ui.Toggle(ctx.Read(nightShift), nightShift.Set).Label("Night shift"),
				),
				ui.Row("Text size").Icon(outline.TextSize).Value("Default"),
			),
		).Padding(0).Header("Screen"),
		ui.Text("Night shift warms the colors of the display after sunset.").
			FontSize(13).Foreground(ui.ColorSecondaryLabel),
	).Gap(12).Padding(16).Flex(1)

	// The screens are derived from the state: the root, and the pushed
	// screen on top of it. A push sets the state, a pop clears it.
	screens := []ui.ScreenSpec{ui.Screen("Settings", settings)}
	switch ctx.Read(open) {
	case "Display":
		screens = append(screens, ui.Screen("Display", display))
	case "Notifications", "Privacy":
		screens = append(screens, ui.Screen(ctx.Read(open), ui.VStack(
			ui.Text("Nothing to configure.").Foreground(ui.ColorSecondaryLabel),
		).Padding(16).Align(geom.Center).Flex(1)))
	}

	return ui.NavigationStack(func() { open.Set("") }, screens...).
		BackIcon(outline.AngleLeft)
	// snippet:end
}
